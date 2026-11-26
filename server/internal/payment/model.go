// 本包定义支付渠道主动查单与已核验支付事实。渠道通知不在此处解释为支付结果。
package payment

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"math"
	"strconv"
	"time"
)

// Kind 标识原订单的商品种类，同编号的不同种类不可相互替代。
type Kind string

const (
	KindRecharge      Kind = "recharge"       // 向指定角色新增非绑定元宝。
	KindPrivilegeCard Kind = "privilege_card" // 人民币直购指定角色的特权卡期。
	KindHostingCard   Kind = "hosting_card"   // 人民币直购指定角色的托管卡期。
	CurrencyCNY            = "CNY"            // 当前玩法支持的实付币种，金额单位为分。
)

// ExpectedOrder 是服务端持久订单的不可变核验输入，带上可信渠道受理后绑定的外部订单号。
type ExpectedOrder struct {
	ID              string // 本地永久订单编号。
	ProviderID      string // 可信渠道的配置身份。
	MerchantID      string // 此单原商户作用域。
	ExternalOrderID string // 原绑定的渠道订单编号。
	AccountID       string // 本单所属的原账号。
	OriginServerID  uint64 // 角色所属的永久入口服，不随合服变化。
	RoleID          string // 本单固定的受益角色。
	PurchaseKind    Kind   // 原商品种类。
	ProductID       uint32 // 原服务端商品编号。
	AmountFen       int64  // 原人民币报价，按分记账。
	Currency        string // 原实付币种。
}

// BeneficiaryReference 生成渠道可携带的订单绑定标记，无需向渠道传递游戏账号或角色编号。
// 已认证的渠道结果或渠道原受理记录应带回该标记，不能用本地期望值填补缺失字段。
func (order ExpectedOrder) BeneficiaryReference() string {
	if !orderID(order.ID) || !orderID(order.AccountID) || !orderID(order.RoleID) ||
		order.OriginServerID == 0 || order.OriginServerID > math.MaxInt64 {
		return ""
	}
	scope := "minimir:payment:beneficiary:v1:" + order.ID + ":" + order.AccountID + ":" +
		strconv.FormatUint(order.OriginServerID, 10) + ":" + order.RoleID
	digest := sha256.Sum256([]byte(scope))
	return hex.EncodeToString(digest[:])
}

// orderID 检查项目生成的规范本地订单、账号或角色编号。
func orderID(value string) bool {
	if len(value) != 32 {
		return false
	}
	for _, char := range value {
		if (char < '0' || char > '9') && (char < 'a' || char > 'f') {
			return false
		}
	}
	return true
}

// OrderQuery 仅提供渠道查单身份，不把本地期望金额、商品或受益人交给渠道适配器当结果使用。
type OrderQuery struct {
	OrderID         string // 本地服务端下发给渠道的原订单号。
	MerchantID      string // 当前查单必须使用的商户作用域。
	ExternalOrderID string // 按可信创建结果绑定的原渠道订单号。
}

// ProviderState 由实际渠道查单状态映射，未识别状态保持为零。
type ProviderState uint8

const (
	ProviderPending  ProviderState = iota + 1 // 尚无可用于履约的成功支付。
	ProviderPaid                              // 已正式支付且渠道支持按此状态履约。
	ProviderClosed                            // 已关闭或取消。
	ProviderRefunded                          // 已退款或撤销，不得用于新履约。
)

// ProviderOrder 只承载经过渠道身份认证的主动查单事实。
type ProviderOrder struct {
	OrderID              string        // 渠道查询确认的商户原单号。
	MerchantID           string        // 渠道查询确认的商户作用域。
	ExternalOrderID      string        // 渠道查询确认的原渠道订单号。
	BeneficiaryReference string        // 原订单的受益角色绑定标记。
	PurchaseKind         Kind          // 渠道查询确认的商品类别。
	ProductID            uint32        // 渠道查询确认的商品编号。
	AmountFen            int64         // 渠道查询确认的已实付金额，按分记账。
	Currency             string        // 渠道查询确认的实付币种。
	State                ProviderState // 正式查询返回的支付状态。
	TransactionID        string        // 支付完成后由渠道返回的唯一交易号。
	PaidAt               time.Time     // 渠道确认的支付时刻。
}

// Provider 由部署时确认为可信的渠道适配器实现。查询必须独立于通知、客户端或管理端传值。
// 适配器遵循查询上下文、核验渠道响应的来源和完整性，并从其确认的原交易提取全部字段。
type Provider interface {
	QueryOrder(ctx context.Context, query OrderQuery) (ProviderOrder, error)
}

// VerifiedPayment 是只可由本包核验器创建的已付证明，下游仍须核对当前持久订单与幂等身份。
type VerifiedPayment struct {
	order         ExpectedOrder // 第一次主动查单已完整匹配的原订单。
	transactionID string        // 同渠道同商户下唯一的确切交易编号。
	paidAt        time.Time     // 查单确认的原支付时刻。
}

// Valid 阻止将零值或无确认结果传给履约。
func (p VerifiedPayment) Valid() bool {
	return p.order.ID != "" && p.transactionID != "" && !p.paidAt.IsZero()
}

// Order 返回副本，调用方的修改不会改变已经核验的原订单。
func (p VerifiedPayment) Order() ExpectedOrder { return p.order }

// TransactionID 返回与已核验商户作用域配合使用的原渠道交易号。
func (p VerifiedPayment) TransactionID() string { return p.transactionID }

// PaidAt 返回已核验的渠道支付时刻。
func (p VerifiedPayment) PaidAt() time.Time { return p.paidAt }
