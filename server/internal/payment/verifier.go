// 本文件按订单确立的商户和原条款主动查单，只对完整匹配的成功支付签发本地证明。
package payment

import (
	"context"
	"errors"
	"time"
)

var (
	ErrUnconfigured    = errors.New("支付渠道未装配")      // 无实际商户或可信渠道适配器。
	ErrInvalidOrder    = errors.New("原支付订单未完整绑定")   // 服务端原单缺失或不是当前商户。
	ErrOrderMismatch   = errors.New("渠道结果与原支付订单不符") // 订单、资金或受益归属的可信数据有冲突。
	ErrProviderFailure = errors.New("渠道支付结果暂不可确认")  // 未知结果保留原单重查，避免向上层传播渠道原错误。
	ErrNotPaid         = errors.New("原订单尚未支付")      // 继续按原单查证。
	ErrClosed          = errors.New("原订单已关闭")       // 原单不可按当前观察履约。
	ErrRefunded        = errors.New("原支付已撤销或退款")    // 原单不可据此生成成功支付证明。
)

// Verifier 持有固定的实际渠道、商户及查单能力，通知请求无法替换这些设置。
type Verifier struct {
	providerID string   // 当前已配置的渠道身份。
	merchantID string   // 渠道下独立校验的原商户。
	provider   Provider // 唯一可提供可信查单结果的适配器。
}

// NewVerifier 只接受显式注入的渠道和商户，不生成可用的默认渠道。
func NewVerifier(providerID, merchantID string, adapter Provider) (*Verifier, error) {
	if adapter == nil || !channelID(providerID, 32) || !channelID(merchantID, 128) {
		return nil, ErrUnconfigured
	}
	return &Verifier{providerID: providerID, merchantID: merchantID, provider: adapter}, nil
}

// Verify 只核验已绑定渠道订单的持久原单。执行主动查询时不替换期望条款或受益账号。
func (v *Verifier) Verify(ctx context.Context, order ExpectedOrder) (VerifiedPayment, error) {
	if v == nil || v.provider == nil {
		return VerifiedPayment{}, ErrUnconfigured
	}
	beneficiary := order.BeneficiaryReference()
	if beneficiary == "" || order.ProviderID != v.providerID || order.MerchantID != v.merchantID ||
		!channelID(order.ExternalOrderID, 128) || order.Currency != CurrencyCNY || order.AmountFen <= 0 ||
		order.ProductID == 0 || !supportedKind(order.PurchaseKind) {
		return VerifiedPayment{}, ErrInvalidOrder
	}
	if err := ctx.Err(); err != nil {
		return VerifiedPayment{}, err
	}
	actual, err := v.provider.QueryOrder(ctx, OrderQuery{OrderID: order.ID, MerchantID: v.merchantID, ExternalOrderID: order.ExternalOrderID})
	if canceled := ctx.Err(); canceled != nil {
		return VerifiedPayment{}, canceled
	}
	if err != nil {
		return VerifiedPayment{}, ErrProviderFailure
	}
	if actual.OrderID != order.ID || actual.MerchantID != v.merchantID || actual.ExternalOrderID != order.ExternalOrderID {
		return VerifiedPayment{}, ErrOrderMismatch
	}
	switch actual.State {
	case ProviderPending:
		return VerifiedPayment{}, ErrNotPaid
	case ProviderClosed:
		return VerifiedPayment{}, ErrClosed
	case ProviderRefunded:
		return VerifiedPayment{}, ErrRefunded
	case ProviderPaid:
		// 只有支付成功才具有完整的实付信息和渠道交易号。
	default:
		return VerifiedPayment{}, ErrProviderFailure
	}
	if actual.AmountFen != order.AmountFen || actual.Currency != order.Currency || actual.ProductID != order.ProductID ||
		actual.PurchaseKind != order.PurchaseKind || actual.BeneficiaryReference != beneficiary {
		return VerifiedPayment{}, ErrOrderMismatch
	}
	if !channelID(actual.TransactionID, 128) || actual.PaidAt.IsZero() ||
		actual.PaidAt.UnixMilli() <= 0 || actual.PaidAt.After(time.Now()) {
		return VerifiedPayment{}, ErrProviderFailure
	}
	return VerifiedPayment{order: order, transactionID: actual.TransactionID, paidAt: actual.PaidAt.UTC().Truncate(time.Millisecond)}, nil
}

// supportedKind 保留首阶段已确认的充值及两类现金卡，不沿用元宝商品或战令入口。
func supportedKind(kind Kind) bool {
	return kind == KindRecharge || kind == KindPrivilegeCard || kind == KindHostingCard
}

// channelID 约束渠道身份的长度及不可见字符，不对原字符串做会改变订单范围的归一化。
func channelID(value string, maxLen int) bool {
	if len(value) == 0 || len(value) > maxLen {
		return false
	}
	for _, char := range value {
		if char <= ' ' || char > '~' {
			return false
		}
	}
	return true
}
