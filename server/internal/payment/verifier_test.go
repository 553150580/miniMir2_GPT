// 本文件用隔离适配器检查主动查单的可信边界、原订单匹配和证明不可被请求构造。
package payment_test

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"strings"
	"testing"
	"time"

	"minimir/internal/payment"
)

// queryFunc 仅供测试记录主动查询及独立的渠道返回。
type queryFunc func(context.Context, payment.OrderQuery) (payment.ProviderOrder, error)

func (fn queryFunc) QueryOrder(ctx context.Context, query payment.OrderQuery) (payment.ProviderOrder, error) {
	return fn(ctx, query)
}

// testOrder 返回一个会在渠道受理后持久绑定的原订单。
func testOrder() payment.ExpectedOrder {
	return payment.ExpectedOrder{
		ID: strings.Repeat("1", 32), ProviderID: "channel-test", MerchantID: "merchant-a", ExternalOrderID: "order-from-channel",
		AccountID: strings.Repeat("2", 32), OriginServerID: 17, RoleID: strings.Repeat("3", 32),
		PurchaseKind: payment.KindPrivilegeCard, ProductID: 2, AmountFen: 7800, Currency: payment.CurrencyCNY,
	}
}

// testObservation 表示适配器已经认证的渠道查单响应，本测试随后改变各字段验证拒绝条件。
func testObservation(order payment.ExpectedOrder) payment.ProviderOrder {
	return payment.ProviderOrder{
		OrderID: order.ID, MerchantID: "merchant-a", ExternalOrderID: "order-from-channel",
		BeneficiaryReference: order.BeneficiaryReference(), PurchaseKind: payment.KindPrivilegeCard, ProductID: 2, AmountFen: 7800,
		Currency: payment.CurrencyCNY, State: payment.ProviderPaid, TransactionID: "transaction-from-channel",
		PaidAt: time.Now().Add(-time.Minute).In(time.FixedZone("回执时区", 8*3600)),
	}
}

func TestVerifierQueriesOriginalIdentityAndProducesProof(t *testing.T) {
	order := testOrder()
	observation := testObservation(order)
	calls := 0
	client, err := payment.NewVerifier(order.ProviderID, order.MerchantID, queryFunc(func(_ context.Context, query payment.OrderQuery) (payment.ProviderOrder, error) {
		calls++
		if query != (payment.OrderQuery{OrderID: order.ID, MerchantID: "merchant-a", ExternalOrderID: "order-from-channel"}) {
			t.Fatal("查单未使用服务端已绑定的原三方身份", query)
		}
		return observation, nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	proof, err := client.Verify(t.Context(), order)
	if err != nil || !proof.Valid() || calls != 1 || proof.Order() != order || proof.TransactionID() != observation.TransactionID ||
		proof.PaidAt() != observation.PaidAt.UTC().Truncate(time.Millisecond) {
		t.Fatal("主动查单未形成原支付的完整证明", proof, calls, err)
	}
	copy := proof.Order()
	copy.RoleID = strings.Repeat("4", 32)
	if proof.Order().RoleID != order.RoleID || copy.BeneficiaryReference() == order.BeneficiaryReference() {
		t.Fatal("修改副本改变了证明，或受益身份未随原角色隔离")
	}
}

func TestVerifierRejectsUntrustedStateOrMismatchedFacts(t *testing.T) {
	for _, tc := range []struct {
		name string
		edit func(*payment.ProviderOrder)
		want error
	}{
		{"陌生本地订单", func(o *payment.ProviderOrder) { o.OrderID = strings.Repeat("a", 32) }, payment.ErrOrderMismatch},
		{"其他商户", func(o *payment.ProviderOrder) { o.MerchantID = "merchant-other" }, payment.ErrOrderMismatch},
		{"其他渠道订单", func(o *payment.ProviderOrder) { o.ExternalOrderID = "other" }, payment.ErrOrderMismatch},
		{"其他受益角色", func(o *payment.ProviderOrder) { o.BeneficiaryReference = strings.Repeat("0", 64) }, payment.ErrOrderMismatch},
		{"未返回受益人", func(o *payment.ProviderOrder) { o.BeneficiaryReference = "" }, payment.ErrOrderMismatch},
		{"错额", func(o *payment.ProviderOrder) { o.AmountFen-- }, payment.ErrOrderMismatch},
		{"币种", func(o *payment.ProviderOrder) { o.Currency = "USD" }, payment.ErrOrderMismatch},
		{"商品编号", func(o *payment.ProviderOrder) { o.ProductID++ }, payment.ErrOrderMismatch},
		{"商品种类", func(o *payment.ProviderOrder) { o.PurchaseKind = payment.KindRecharge }, payment.ErrOrderMismatch},
		{"无渠道交易号", func(o *payment.ProviderOrder) { o.TransactionID = "" }, payment.ErrProviderFailure},
		{"交易号含控制字符", func(o *payment.ProviderOrder) { o.TransactionID = "a\nb" }, payment.ErrProviderFailure},
		{"未给支付时刻", func(o *payment.ProviderOrder) { o.PaidAt = time.Time{} }, payment.ErrProviderFailure},
		{"不合法支付时刻", func(o *payment.ProviderOrder) { o.PaidAt = time.Unix(0, 0) }, payment.ErrProviderFailure},
		{"未来支付时刻", func(o *payment.ProviderOrder) { o.PaidAt = time.Now().Add(time.Hour) }, payment.ErrProviderFailure},
		{"待支付", func(o *payment.ProviderOrder) { o.State = payment.ProviderPending }, payment.ErrNotPaid},
		{"已关闭", func(o *payment.ProviderOrder) { o.State = payment.ProviderClosed }, payment.ErrClosed},
		{"已退款", func(o *payment.ProviderOrder) { o.State = payment.ProviderRefunded }, payment.ErrRefunded},
		{"未知状态", func(o *payment.ProviderOrder) { o.State = 255 }, payment.ErrProviderFailure},
	} {
		t.Run(tc.name, func(t *testing.T) {
			order := testOrder()
			observation := testObservation(order)
			tc.edit(&observation)
			client, err := payment.NewVerifier(order.ProviderID, order.MerchantID, queryFunc(func(context.Context, payment.OrderQuery) (payment.ProviderOrder, error) {
				return observation, nil
			}))
			if err != nil {
				t.Fatal(err)
			}
			proof, err := client.Verify(t.Context(), order)
			if proof.Valid() || !errors.Is(err, tc.want) {
				t.Fatal("不匹配或非成功渠道事实产生已付证明", proof, err)
			}
		})
	}
}

func TestVerifierRejectsOrderBeforeQueryUnlessBound(t *testing.T) {
	order := testOrder()
	calls := 0
	client, err := payment.NewVerifier(order.ProviderID, order.MerchantID, queryFunc(func(context.Context, payment.OrderQuery) (payment.ProviderOrder, error) {
		calls++
		return payment.ProviderOrder{}, nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	for _, edit := range []func(*payment.ExpectedOrder){
		func(o *payment.ExpectedOrder) { o.ExternalOrderID = "" },
		func(o *payment.ExpectedOrder) { o.MerchantID = "merchant-other" },
		func(o *payment.ExpectedOrder) { o.ProviderID = "unconfigured" },
		func(o *payment.ExpectedOrder) { o.ID = "from-notification" },
		func(o *payment.ExpectedOrder) { o.AccountID = strings.Repeat("B", 32) },
		func(o *payment.ExpectedOrder) { o.RoleID = "no-such-role" },
		func(o *payment.ExpectedOrder) { o.OriginServerID = math.MaxInt64 + 1 },
		func(o *payment.ExpectedOrder) { o.Currency = "USD" },
		func(o *payment.ExpectedOrder) { o.AmountFen = 0 },
		func(o *payment.ExpectedOrder) { o.ProductID = 0 },
		func(o *payment.ExpectedOrder) { o.PurchaseKind = "unreleased" },
	} {
		changed := order
		edit(&changed)
		proof, err := client.Verify(t.Context(), changed)
		if proof.Valid() || !errors.Is(err, payment.ErrInvalidOrder) {
			t.Fatal("非法原单未被主动查单边界拒绝", changed, err)
		}
	}
	if calls != 0 {
		t.Fatal("未完成绑定的原支付订单被发往渠道", calls)
	}
	if disabled, err := payment.NewVerifier(order.ProviderID, order.MerchantID, nil); disabled != nil || !errors.Is(err, payment.ErrUnconfigured) {
		t.Fatal("没有可信适配器却获得可用渠道", err)
	}
	var unconfigured *payment.Verifier
	if proof, err := unconfigured.Verify(t.Context(), order); proof.Valid() || !errors.Is(err, payment.ErrUnconfigured) {
		t.Fatal("默认状态可生成已付证明", proof, err)
	}
}

func TestVerifierNeverSubstitutesCallerDataOrConfirmsUnknown(t *testing.T) {
	order := testOrder()
	order.AmountFen = 9_007_199_254_740_993
	observation := testObservation(order)
	observation.AmountFen = 9_007_199_254_740_992
	queryError := errors.New("需留在渠道内部的诊断")
	var failure error
	var cancel context.CancelFunc
	client, err := payment.NewVerifier(order.ProviderID, order.MerchantID, queryFunc(func(context.Context, payment.OrderQuery) (payment.ProviderOrder, error) {
		if cancel != nil {
			cancel()
		}
		return observation, failure
	}))
	if err != nil {
		t.Fatal(err)
	}
	if proof, err := client.Verify(t.Context(), order); proof.Valid() || !errors.Is(err, payment.ErrOrderMismatch) {
		t.Fatal("不同的精确金额在比较中被混为同一值", err)
	}
	failure = queryError
	if proof, err := client.Verify(t.Context(), order); proof.Valid() || err != payment.ErrProviderFailure || errors.Is(err, queryError) {
		t.Fatal("未知结果发放证明或原样透出渠道错误", err)
	}
	failure = nil
	observation.AmountFen = order.AmountFen
	ctx, stop := context.WithCancel(t.Context())
	defer stop()
	cancel = stop
	if proof, err := client.Verify(ctx, order); proof.Valid() || !errors.Is(err, context.Canceled) {
		t.Fatal("主动查询期间取消仍确认了支付", err)
	}
	var forged payment.VerifiedPayment
	if err := json.Unmarshal([]byte(`{"order":{"ID":"reported-by-caller"},"transactionID":"fake","paidAt":"2026-01-01T00:00:00Z"}`), &forged); err != nil || forged.Valid() {
		t.Fatal("通知或请求直接填充了本地履约证明", forged, err)
	}
}
