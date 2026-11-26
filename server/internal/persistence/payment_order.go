// 本文件保存经过账号归属核验的人民币订单和不可变报价，不操作角色资产。
package persistence

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"math"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"minimir/internal/devconfig"
	"minimir/internal/payment"
	p "minimir/internal/protocol"
)

const paymentOrderCollection = "payment_orders" // 保存本地原单，支付和履约证据均永久保留。

// PaymentOrderState 区分原单建立、渠道绑定、支付核验与实际履约。
type PaymentOrderState string

const (
	PaymentOrderCreated   PaymentOrderState = "created"   // 原单可由可信渠道受理。
	PaymentOrderBound     PaymentOrderState = "bound"     // 绑定渠道订单，尚无成功核验证明。
	PaymentOrderVerified  PaymentOrderState = "verified"  // 已核验支付且可进入待履约队列。
	PaymentOrderReview    PaymentOrderState = "review"    // 原证明已保留，渠道对账出现需阻止新履约的分歧。
	PaymentOrderFulfilled PaymentOrderState = "fulfilled" // 原资产事务已经提交。
)

// PaymentOrderQuote 固定由服务端签发的商品、应付分数与履约量。
type PaymentOrderQuote struct {
	Kind         payment.Kind `bson:"kind"`          // 充值、现金特权卡或现金托管卡。
	ProductID    uint32       `bson:"product_id"`    // 可信商品的目录编号。
	AmountFen    int64        `bson:"amount_fen"`    // 人民币应付金额，单位为分。
	YuanbaoUnits int64        `bson:"yuanbao_units"` // 充值发放的非绑定元宝最小单位，现金卡为零。
	CardDays     uint32       `bson:"card_days"`     // 现金卡的权限天数，充值为零。
	RuleVersion  uint64       `bson:"rule_version"`  // 原商品规则版本。
	ConfigHash   string       `bson:"config_hash"`   // 原报价的完整规则摘要。
	ProviderID   string       `bson:"provider_id"`   // 受信渠道的配置身份。
	MerchantID   string       `bson:"merchant_id"`   // 原订单的商户作用域。
}

// PaymentOrder 保存本地商户原单、永久受益人、原报价和两阶段提交证据。
type PaymentOrder struct {
	ID                string                      `bson:"_id"`                           // 服务端为账号及原请求生成的本地商户订单号。
	RequestID         string                      `bson:"request_id"`                    // 原购买请求身份，成功或未知提交都沿用该值。
	AccountID         string                      `bson:"account_id"`                    // 发起支付的已认证账号。
	RoleID            string                      `bson:"role_id"`                       // 获得元宝或权益的原角色。
	OriginServerID    uint64                      `bson:"origin_server_id"`              // 角色永久入口，不因合服而变化。
	Quote             PaymentOrderQuote           `bson:"quote"`                         // 初次受理冻结的服务端报价。
	Revision          int64                       `bson:"revision"`                      // 包括渠道核验和资产提交的状态修订。
	State             PaymentOrderState           `bson:"state"`                         // 当前业务处理阶段。
	ExternalOrderID   string                      `bson:"external_order_id,omitempty"`   // 可信渠道受理后绑定的原单号。
	Verification      *PaymentOrderVerification   `bson:"verification,omitempty"`        // 首个成功核验支付的永久证据。
	Reconciliation    *PaymentOrderReconciliation `bson:"reconciliation,omitempty"`      // 最近一次有效对账观察。
	AssetOperationRef string                      `bson:"asset_operation_ref,omitempty"` // 实际履约事务的永久回执主键。
	CreatedAt         time.Time                   `bson:"created_at"`                    // 本地原单首次受理时刻。
	UpdatedAt         time.Time                   `bson:"updated_at"`                    // 本地最近状态变更时刻。
	VerifiedAt        time.Time                   `bson:"verified_at,omitempty"`         // 首次核验成功时刻，用于履约队列排序。
	FulfilledAt       time.Time                   `bson:"fulfilled_at,omitempty"`        // 原资产事务的服务端时刻。
}

// Expected 投影查单所需的完整原支付条款，金额和受益人均来自已存订单。
func (order PaymentOrder) Expected() payment.ExpectedOrder {
	return payment.ExpectedOrder{ID: order.ID, ProviderID: order.Quote.ProviderID, MerchantID: order.Quote.MerchantID,
		ExternalOrderID: order.ExternalOrderID, AccountID: order.AccountID, OriginServerID: order.OriginServerID,
		RoleID: order.RoleID, PurchaseKind: order.Quote.Kind, ProductID: order.Quote.ProductID,
		AmountFen: order.Quote.AmountFen, Currency: payment.CurrencyCNY}
}

// CreatePaymentOrder 使用账号授权和可信服务报价建立原单，原请求重试读回首个订单。
// a 携带已认证账号代次以及所选角色，requestID 在该账号下标识原购买操作，quote 由服务端商品目录生成。
func (s *Store) CreatePaymentOrder(ctx context.Context, a Auth, requestID string, quote PaymentOrderQuote) (PaymentOrder, error) {
	if !lowerHex(a.AccountID, 16) || a.AuthVersion <= 0 || !lowerHex(a.RoleID, 16) || !lowerHex(requestID, 16) || !quote.valid() {
		return PaymentOrder{}, Fail(p.CodeInvalidArgument, "支付原单参数无效")
	}
	// 原请求在账号内唯一，即使客户端换角也无法用原键建立第二张订单。
	id := paymentOrderID(a.AccountID, requestID)
	var result PaymentOrder // 已存在的原单，或本次原子建立的新原单。
	err := s.tx(ctx, func(tc context.Context) error {
		if err := s.accountGuard(tc, a); err != nil {
			return err
		}
		existing, err := s.readPaymentOrder(tc, id)
		if err == nil {
			if existing.RequestID != requestID || existing.AccountID != a.AccountID || existing.RoleID != a.RoleID || existing.Quote != quote {
				return Fail(p.CodeOperationConflict, "支付原请求已绑定其他条款")
			}
			result = existing
			return nil
		}
		if !errors.Is(err, mongo.ErrNoDocuments) {
			return err
		}
		if err = s.validateNewPaymentQuote(quote); err != nil {
			return err
		}
		// 账号授权围栏和角色永久归属核验都在原单写入事务内。
		var role struct {
			Origin uint64 `bson:"origin_server_id"` // 此角色在原单上唯一有效的永久入口服。
		}
		filter := bson.M{"_id": a.RoleID, "account_id": a.AccountID}
		err = s.DB.Collection("roles").FindOne(tc, filter, options.FindOne().SetProjection(bson.M{"origin_server_id": 1})).Decode(&role)
		if err != nil {
			return authorityReadError(err, p.CodeUnauthenticated, "支付角色不属于当前账号")
		}
		if role.Origin == 0 || role.Origin > math.MaxInt64 {
			return Fail(p.CodeOperationConflict, "支付角色的永久入口无效")
		}
		at := bossTime(time.Now())
		result = PaymentOrder{ID: id, RequestID: requestID, AccountID: a.AccountID, RoleID: a.RoleID, OriginServerID: role.Origin,
			Quote: quote, Revision: 1, State: PaymentOrderCreated, CreatedAt: at, UpdatedAt: at}
		_, err = s.DB.Collection(paymentOrderCollection).InsertOne(tc, result)
		return err
	})
	if err != nil {
		return PaymentOrder{}, err
	}
	return result, nil
}

// GetPaymentOrder 按本地商户原单号查询内部对账与履约资料。
func (s *Store) GetPaymentOrder(ctx context.Context, orderID string) (PaymentOrder, error) {
	if !lowerHex(orderID, 16) {
		return PaymentOrder{}, Fail(p.CodeInvalidArgument, "支付订单编号无效")
	}
	return s.readPaymentOrder(ctx, orderID)
}

// readPaymentOrder 加入调用者的现有事务，以便读证据与后续状态条件写使用同一快照。
func (s *Store) readPaymentOrder(ctx context.Context, orderID string) (PaymentOrder, error) {
	var order PaymentOrder // 保持 MongoDB 原单的资料和确切修订。
	err := s.DB.Collection(paymentOrderCollection).FindOne(ctx, bson.M{"_id": orderID}).Decode(&order)
	return order, err
}

// paymentOrderID 以分域摘要为已认证账号与原请求获得稳定、长度固定的本地商户单号。
func paymentOrderID(accountID, requestID string) string {
	digest := sha256.Sum256([]byte("minimir:payment:order:v1:" + accountID + ":" + requestID))
	return hex.EncodeToString(digest[:16])
}

// valid 检查冻结条款的结构与货币转换，既有已受理订单继续使用原条款。
func (quote PaymentOrderQuote) valid() bool {
	if quote.ProductID == 0 || quote.AmountFen <= 0 || quote.RuleVersion == 0 || quote.RuleVersion > math.MaxInt64 ||
		!lowerHex(quote.ConfigHash, 32) || !validPaymentChannelID(quote.ProviderID, 32) || !validPaymentChannelID(quote.MerchantID, 128) {
		return false
	}
	switch quote.Kind {
	case payment.KindRecharge:
		return quote.CardDays == 0 && quote.AmountFen <= math.MaxInt64/10 && quote.YuanbaoUnits == quote.AmountFen*10
	case payment.KindPrivilegeCard, payment.KindHostingCard:
		return quote.YuanbaoUnits == 0 && (quote.CardDays == 30 || quote.CardDays == 90 || quote.CardDays == 365)
	default:
		return false
	}
}

// validateNewPaymentQuote 按当前快照核验新报价，已受理订单重放绕过新版本配置以保留原承诺。
func (s *Store) validateNewPaymentQuote(quote PaymentOrderQuote) error {
	fixture := &s.Cfg.Fixture
	if fixture.Payment == nil {
		return Fail(p.CodeLineForbidden, "当前未装配支付商品")
	}
	if quote.RuleVersion != fixture.RuleVersion || quote.ConfigHash != fixture.Hash {
		return Fail(p.CodeOperationConflict, "原支付报价的商品版本已变")
	}
	if quote.Kind == payment.KindRecharge {
		// 充值档位由可信调用方独立装配，此处核验原报价的整数换算和版本。
		return nil
	}
	product, ok := fixture.CashCardProduct(quote.ProductID)
	if !ok || product.Days != quote.CardDays || product.PriceFen != quote.AmountFen ||
		(quote.Kind == payment.KindPrivilegeCard && product.Kind != devconfig.RoleCardKindPrivilege) ||
		(quote.Kind == payment.KindHostingCard && product.Kind != devconfig.RoleCardKindHosting) {
		return Fail(p.CodeOperationConflict, "原现金双卡商品条款不一致")
	}
	return nil
}

// paymentOrderIndexes 使渠道单号和真实交易号只能绑定一张本地原单，并支持有界履约扫描。
func paymentOrderIndexes() []mongo.IndexModel {
	return []mongo.IndexModel{
		{Keys: bson.D{{Key: "account_id", Value: 1}, {Key: "request_id", Value: 1}},
			Options: options.Index().SetName("payment_account_request").SetUnique(true)},
		{Keys: bson.D{{Key: "quote.provider_id", Value: 1}, {Key: "quote.merchant_id", Value: 1}, {Key: "external_order_id", Value: 1}},
			Options: options.Index().SetName("payment_external_order").SetUnique(true).
				SetPartialFilterExpression(bson.M{"external_order_id": bson.M{"$type": "string"}})},
		{Keys: bson.D{{Key: "quote.provider_id", Value: 1}, {Key: "quote.merchant_id", Value: 1}, {Key: "verification.transaction_id", Value: 1}},
			Options: options.Index().SetName("payment_provider_transaction").SetUnique(true).
				SetPartialFilterExpression(bson.M{"verification.transaction_id": bson.M{"$type": "string"}})},
		{Keys: bson.D{{Key: "state", Value: 1}, {Key: "origin_server_id", Value: 1}, {Key: "verified_at", Value: 1}, {Key: "_id", Value: 1}},
			Options: options.Index().SetName("payment_pending_fulfillment")},
	}
}

// validPaymentChannelID 拒绝空值、超长或不可见字符，保持渠道原编号的精确身份。
func validPaymentChannelID(value string, maximum int) bool {
	if len(value) == 0 || len(value) > maximum {
		return false
	}
	for _, char := range value {
		if char <= ' ' || char > '~' {
			return false
		}
	}
	return true
}
