// 本文件把已核验的现金支付交给当前运行服，原子写入角色、订单、权益历史与事后对账证据。
package persistence

import (
	"context"
	"math"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	"minimir/internal/payment"
	p "minimir/internal/protocol"
)

const paymentGrantKind = "payment_grant" // 与玩家命令、战斗收益流水及其他资金来源区分。

// PaymentCardGrant 保存单张现金卡的原卡期与整期续费结果。
type PaymentCardGrant struct {
	Kind          uint32    `bson:"kind"`                     // 原商品对应的角色卡种。
	Days          uint32    `bson:"days"`                     // 经确认现金订单购买的完整天数。
	PreviousSince time.Time `bson:"previous_since,omitempty"` // 更新前的连续期起点。
	PreviousUntil time.Time `bson:"previous_until,omitempty"` // 更新前的连续期终点。
	ValidSince    time.Time `bson:"valid_since"`              // 更新后的连续期起点。
	ValidUntil    time.Time `bson:"valid_until"`              // 更新后的连续期终点。
}

// PaymentGrantReceipt 是已投放或现金卡已生效的永久结果，包含可核对的四钱包快照。
type PaymentGrantReceipt struct {
	OrderID        string            `bson:"order_id"`         // 服务端冻结的原订单。
	AccountID      string            `bson:"account_id"`       // 支付受益账号。
	OriginServerID uint64            `bson:"origin_server_id"` // 角色永久入口服。
	Kind           payment.Kind      `bson:"kind"`             // 充值、现金特权卡或现金托管卡。
	ProductID      uint32            `bson:"product_id"`       // 原配置中的商品身份。
	RuleVersion    uint64            `bson:"rule_version"`     // 成交时已冻结的规则版本。
	ConfigHash     string            `bson:"config_hash"`      // 原价格与权益来自相同配置快照。
	AmountFen      int64             `bson:"amount_fen"`       // 支付的人民币分。
	YuanbaoAdded   int64             `bson:"yuanbao_added"`    // 仅充值新增到角色的非绑定元宝最小单位。
	WalletBefore   domain.Wallet     `bson:"wallet_before"`    // 原四钱包。
	WalletAfter    domain.Wallet     `bson:"wallet_after"`     // 实际履约后的四钱包。
	Card           *PaymentCardGrant `bson:"card,omitempty"`   // 仅现金购卡确认的角色卡期。
	PaidAt         time.Time         `bson:"paid_at"`          // 渠道已确认的付款时刻。
	GrantedAt      time.Time         `bson:"granted_at"`       // 权益或元宝首次实际入账的时刻。
}

// FulfillPaymentOrder 使用持久订单作为唯一输入，当前运行服可为在线和离线角色履约。
func (s *Store) FulfillPaymentOrder(ctx context.Context, id string) (domain.Role, Operation, error) {
	ctx, stop := context.WithTimeout(ctx, transactionTimeout)
	defer stop()
	if s.Cfg.RunID == 0 || s.Epoch <= 0 || s.runGuardSlots == nil {
		return domain.Role{}, Operation{}, Fail(p.CodeRouteChanged, "支付履约需要有效运行服")
	}
	initial, err := s.GetPaymentOrder(ctx, id)
	if err != nil {
		return domain.Role{}, Operation{}, err
	}
	var out domain.Role
	var receipt Operation
	err = s.guardedTx(ctx, Auth{AccountID: initial.AccountID}, func(tc context.Context) error {
		order, err := s.readPaymentOrder(tc, id)
		if err != nil {
			return err
		}
		if order.AccountID != initial.AccountID || order.RoleID != initial.RoleID || order.OriginServerID != initial.OriginServerID {
			return Fail(p.CodeOperationConflict, "支付订单的原受益身份已改变")
		}
		if order.State != PaymentOrderFulfilled && !order.hasVerifiedPayment() {
			return Fail(p.CodeOperationConflict, "支付订单尚未完成核验")
		}
		fence, err := s.paymentRoleFence(tc, order)
		if err != nil {
			return err
		}
		role, err := s.Role(tc, order.RoleID)
		if err != nil {
			return err
		}
		if role.AccountID != order.AccountID || role.Origin != order.OriginServerID {
			return Fail(p.CodeOperationConflict, "支付订单和原角色归属不一致")
		}
		opID := "payment:" + order.ID
		if order.State == PaymentOrderFulfilled {
			receipt, err = s.readPaymentGrant(tc, order, opID)
			out = role
			return err
		}
		at := bossTime(time.Now())
		next, grant, err := applyPaymentGrant(role, order, at)
		if err != nil {
			return err
		}
		receipt = Operation{ID: opID, Digest: Hash([]byte(opID)), RoleID: role.ID, Revision: next.Revision,
			CommandSeq: role.EconomicSeq, SourceSeq: role.SourceSeq, Kind: paymentGrantKind, Created: at, PaymentGrant: &grant}
		audit := bson.M{"_id": opID, "kind": paymentGrantKind, "role_id": role.ID, "account_id": role.AccountID,
			"origin_server_id": role.Origin, "run_server_id": s.Cfg.RunID, "order_id": order.ID,
			"provider_id": order.Quote.ProviderID, "transaction_id": order.Verification.TransactionID, "revision": next.Revision,
			"payment_grant": grant, "at": at}
		if err := s.writeSettlement(tc, fence, next, role.Revision, receipt, audit); err != nil {
			return err
		}
		if grant.Card != nil {
			if err := s.insertRoleCardPeriod(tc, next, opID, grant.Card.Kind, at, grant.Card.PreviousSince, grant.Card.PreviousUntil); err != nil {
				return err
			}
		}
		out = next
		return s.markPaymentOrderFulfilled(tc, order, opID, at)
	})
	if err != nil {
		return domain.Role{}, Operation{}, err
	}
	return out, receipt, nil
}

// paymentRoleFence 在原事务检查权威入口路由，并与当前运行服和永久账号上的其他写入竞争。
func (s *Store) paymentRoleFence(ctx context.Context, order PaymentOrder) ([]mongo.ClientBulkWrite, error) {
	current, err := s.liveRoute(ctx, order.OriginServerID)
	if err != nil {
		return nil, err
	}
	if current.run.ID != s.Cfg.RunID || current.run.Owner != s.Instance || current.run.Epoch != s.Epoch {
		return nil, Fail(p.CodeRouteChanged, "支付订单需由原入口当前的游戏所有者履约")
	}
	runWrite, err := s.runFenceWrite(ctx, runGuardGroup(order.AccountID))
	if err != nil {
		return nil, err
	}
	// 渠道已付订单依据持久受益账号发放，与玩家是否在线以及其令牌是否已换代独立。
	writes := make([]mongo.ClientBulkWrite, 1, 5)
	writes[0] = runWrite
	writes = append(writes, mongo.ClientBulkWrite{Database: s.DB.Name(), Collection: "accounts", Model: mongo.NewClientUpdateOneModel().
		SetFilter(bson.M{"_id": order.AccountID}).SetUpdate(bson.M{"$inc": bson.M{"auth_guard_seq": 1}})})
	return writes, nil
}

// readPaymentGrant 沿原订单的唯一指针取回已确认结果，任何不一致都停止入账。
func (s *Store) readPaymentGrant(ctx context.Context, order PaymentOrder, opID string) (Operation, error) {
	var op Operation
	if order.AssetOperationRef != opID || order.Verification == nil {
		return op, Fail(p.CodeOperationConflict, "支付订单的原到账回执不一致")
	}
	err := s.DB.Collection("operations").FindOne(ctx, bson.M{"_id": opID, "kind": paymentGrantKind, "role_id": order.RoleID}).Decode(&op)
	if err != nil {
		return op, authorityReadError(err, p.CodeOperationConflict, "已到账支付订单缺少原回执")
	}
	grant := op.PaymentGrant
	quote := order.Quote
	if grant == nil || grant.OrderID != order.ID || grant.AccountID != order.AccountID || grant.OriginServerID != order.OriginServerID ||
		grant.Kind != quote.Kind || grant.ProductID != quote.ProductID || grant.AmountFen != quote.AmountFen ||
		grant.YuanbaoAdded != quote.YuanbaoUnits || !grant.PaidAt.Equal(order.Verification.PaidAt) ||
		grant.RuleVersion != quote.RuleVersion || grant.ConfigHash != quote.ConfigHash {
		return Operation{}, Fail(p.CodeOperationConflict, "原到账回执与冻结的支付条款不一致")
	}
	return op, nil
}

// applyPaymentGrant 依据冻结的支付条款计算首次到账，保持现有玩家及战斗流水水位。
func applyPaymentGrant(role domain.Role, order PaymentOrder, at time.Time) (domain.Role, PaymentGrantReceipt, error) {
	quote := order.Quote
	if order.Verification == nil || !quote.valid() || role.Revision <= 0 || role.Revision == math.MaxInt64 {
		return role, PaymentGrantReceipt{}, Fail(p.CodeOperationConflict, "支付来源或角色版本异常")
	}
	grant := PaymentGrantReceipt{OrderID: order.ID, AccountID: order.AccountID, OriginServerID: order.OriginServerID,
		Kind: quote.Kind, ProductID: quote.ProductID, RuleVersion: quote.RuleVersion, ConfigHash: quote.ConfigHash,
		AmountFen: quote.AmountFen, WalletBefore: role.Wallet, PaidAt: order.Verification.PaidAt, GrantedAt: at}
	switch quote.Kind {
	case payment.KindRecharge:
		balance, err := add(role.Wallet.Yuanbao, quote.YuanbaoUnits)
		if err != nil {
			return role, PaymentGrantReceipt{}, err
		}
		role.Wallet.Yuanbao, grant.YuanbaoAdded = balance, quote.YuanbaoUnits
	case payment.KindPrivilegeCard, payment.KindHostingCard:
		kind := devconfig.RoleCardKindPrivilege
		if quote.Kind == payment.KindHostingCard {
			kind = devconfig.RoleCardKindHosting
		}
		previousSince, previousUntil := roleCardPeriod(role, kind)
		next, since, until, err := extendRoleCardPeriod(role, kind, quote.CardDays, at)
		if err != nil {
			return role, PaymentGrantReceipt{}, err
		}
		role = next
		grant.Card = &PaymentCardGrant{Kind: kind, Days: quote.CardDays, PreviousSince: previousSince,
			PreviousUntil: previousUntil, ValidSince: since, ValidUntil: until}
	default:
		return role, PaymentGrantReceipt{}, Fail(p.CodeOperationConflict, "不支持原支付商品履约")
	}
	role.Revision++
	grant.WalletAfter = role.Wallet
	return role, grant, nil
}
