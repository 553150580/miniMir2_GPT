// 本文件在独立 MongoDB 副本集验收支付原单、渠道去重与对账修订保护。
package persistence

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	"minimir/internal/payment"
	p "minimir/internal/protocol"
)

// TestPaymentOrderAuthorityMongo 使用真实账号、角色及交易索引验收订单边界。
func TestPaymentOrderAuthorityMongo(t *testing.T) {
	ctx, s := paymentOrderMongo(t)
	a, b := paymentOrderActor(t, ctx, s), paymentOrderActor(t, ctx, s)
	quote := PaymentOrderQuote{Kind: payment.KindRecharge, ProductID: 101, AmountFen: 600, YuanbaoUnits: 6000,
		RuleVersion: s.Cfg.Fixture.RuleVersion, ConfigHash: s.Cfg.Fixture.Hash, ProviderID: "test-channel", MerchantID: "test-merchant"}
	requestID := domain.ID()
	_, err := s.CreatePaymentOrder(ctx, a, requestID, quote)
	paymentOrderTestCode(t, err, p.CodeLineForbidden)
	// 只在独立测试实例装配可信报价。充值档位和渠道凭据均不写入默认产品。
	s.Cfg.Fixture.Payment = &devconfig.PaymentConfig{CashCardProducts: []devconfig.CashCardProduct{
		{ID: 1, Kind: devconfig.RoleCardKindPrivilege, Days: 30, PriceFen: 3000},
	}}
	wrong := a
	wrong.RoleID = b.RoleID
	_, err = s.CreatePaymentOrder(ctx, wrong, requestID, quote)
	paymentOrderTestCode(t, err, p.CodeUnauthenticated)
	forged := quote
	forged.YuanbaoUnits++
	_, err = s.CreatePaymentOrder(ctx, a, requestID, forged)
	paymentOrderTestCode(t, err, p.CodeInvalidArgument)
	order, err := s.CreatePaymentOrder(ctx, a, requestID, quote)
	paymentOrderTestOK(t, err)
	if order.AccountID != a.AccountID || order.RoleID != a.RoleID || order.OriginServerID != s.Cfg.OriginID || order.State != PaymentOrderCreated {
		t.Fatalf("支付原单永久归属异常 %+v", order)
	}
	// 相同操作键可重放原单。配置关闭也能查询已有承诺。
	priorCatalog := s.Cfg.Fixture.Payment
	s.Cfg.Fixture.Payment = nil
	replay, err := s.CreatePaymentOrder(ctx, a, requestID, quote)
	paymentOrderTestOK(t, err)
	if replay.ID != order.ID || replay.Revision != order.Revision || replay.Quote != order.Quote {
		t.Fatalf("原操作没有返回首次订单 %+v", replay)
	}
	s.Cfg.Fixture.Payment = priorCatalog
	repriced := quote
	repriced.AmountFen, repriced.YuanbaoUnits = 650, 6500
	_, err = s.CreatePaymentOrder(ctx, a, requestID, repriced)
	paymentOrderTestCode(t, err, p.CodeOperationConflict)
	card := quote
	card.Kind, card.ProductID, card.AmountFen, card.YuanbaoUnits, card.CardDays = payment.KindHostingCard, 1, 3000, 0, 30
	_, err = s.CreatePaymentOrder(ctx, a, domain.ID(), card)
	paymentOrderTestCode(t, err, p.CodeOperationConflict)
	card.Kind = payment.KindPrivilegeCard
	_, err = s.CreatePaymentOrder(ctx, a, domain.ID(), card)
	paymentOrderTestOK(t, err)

	other, err := s.CreatePaymentOrder(ctx, b, requestID, quote)
	paymentOrderTestOK(t, err)
	if other.ID == order.ID {
		t.Fatal("不同账号的原操作键不能获得同一本地订单")
	}
	_, err = s.BindPaymentOrder(ctx, order.ID, quote.ProviderID, "wrong-merchant", "external-1")
	paymentOrderTestCode(t, err, p.CodeOperationConflict)
	order, err = s.BindPaymentOrder(ctx, order.ID, quote.ProviderID, quote.MerchantID, "external-1")
	paymentOrderTestOK(t, err)
	_, err = s.BindPaymentOrder(ctx, order.ID, quote.ProviderID, quote.MerchantID, "different-order")
	paymentOrderTestCode(t, err, p.CodeOperationConflict)
	_, err = s.BindPaymentOrder(ctx, other.ID, quote.ProviderID, quote.MerchantID, "external-1")
	paymentOrderTestCode(t, err, p.CodeOperationConflict)
	other, err = s.BindPaymentOrder(ctx, other.ID, quote.ProviderID, quote.MerchantID, "external-2")
	paymentOrderTestOK(t, err)

	paidAt := bossTime(time.Now().Add(-time.Second))
	actual := paymentOrderTestPaid(order, "txn-1", paidAt)
	verifier := paymentOrderTestVerifier(t, order, func(context.Context, payment.OrderQuery) (payment.ProviderOrder, error) { return actual, nil })
	_, err = s.RecordVerifiedPayment(ctx, payment.VerifiedPayment{})
	paymentOrderTestCode(t, err, p.CodeInvalidArgument)
	proof, err := verifier.Verify(ctx, order.Expected())
	paymentOrderTestOK(t, err)
	verified, err := s.RecordVerifiedPayment(ctx, proof)
	paymentOrderTestOK(t, err)
	verifiedReplay, err := s.RecordVerifiedPayment(ctx, proof)
	paymentOrderTestOK(t, err)
	if !verified.hasVerifiedPayment() || verifiedReplay.Revision != verified.Revision || verified.Verification.TransactionID != "txn-1" ||
		!verifiedReplay.VerifiedAt.Equal(verified.VerifiedAt) {
		t.Fatalf("同一交易重放异常 %+v / %+v", verified, verifiedReplay)
	}
	duplicate := paymentOrderTestVerifier(t, other, func(context.Context, payment.OrderQuery) (payment.ProviderOrder, error) {
		return paymentOrderTestPaid(other, "txn-1", paidAt), nil
	})
	_, err = s.ReconcilePaymentOrder(ctx, duplicate, other.ID)
	paymentOrderTestCode(t, err, p.CodeOperationConflict)
	stillBound, err := s.GetPaymentOrder(ctx, other.ID)
	paymentOrderTestOK(t, err)
	if stillBound.State != PaymentOrderBound || stillBound.Verification != nil {
		t.Fatalf("同一实付交易绑定到第二张订单 %+v", stillBound)
	}

	// 不同商户的同名渠道单号和交易号应互不干扰。
	separate := quote
	separate.MerchantID = "another-merchant"
	scoped, err := s.CreatePaymentOrder(ctx, b, domain.ID(), separate)
	paymentOrderTestOK(t, err)
	scoped, err = s.BindPaymentOrder(ctx, scoped.ID, separate.ProviderID, separate.MerchantID, "external-1")
	paymentOrderTestOK(t, err)
	scopedVerifier := paymentOrderTestVerifier(t, scoped, func(context.Context, payment.OrderQuery) (payment.ProviderOrder, error) {
		return paymentOrderTestPaid(scoped, "txn-1", paidAt), nil
	})
	scoped, err = s.ReconcilePaymentOrder(ctx, scopedVerifier, scoped.ID)
	paymentOrderTestOK(t, err)
	if !scoped.hasVerifiedPayment() {
		t.Fatalf("不同商户未独立核验 %+v", scoped)
	}

	t.Run("原证明与渠道错单隔离", func(t *testing.T) {
		original := actual
		actual.AmountFen++
		review, err := s.ReconcilePaymentOrder(ctx, verifier, verified.ID)
		if !errors.Is(err, payment.ErrOrderMismatch) || review.State != PaymentOrderReview || review.hasVerifiedPayment() ||
			review.Verification.TransactionID != verified.Verification.TransactionID || review.Reconciliation.Status != PaymentReconciliationMismatch {
			t.Fatalf("错单未按原证明进入复核 %+v %v", review, err)
		}
		delayed, err := s.RecordVerifiedPayment(ctx, proof)
		paymentOrderTestOK(t, err)
		if delayed.State != PaymentOrderReview {
			t.Fatal("旧的渠道证明不能解除已提交的复核")
		}
		actual = original
		recovered, err := s.ReconcilePaymentOrder(ctx, verifier, verified.ID)
		paymentOrderTestOK(t, err)
		if !recovered.hasVerifiedPayment() || !recovered.VerifiedAt.Equal(verified.VerifiedAt) {
			t.Fatalf("真实重查没有恢复原证明 %+v", recovered)
		}
		actual.TransactionID = "another-payment"
		review, err = s.ReconcilePaymentOrder(ctx, verifier, verified.ID)
		if !errors.Is(err, payment.ErrOrderMismatch) || review.State != PaymentOrderReview || review.Verification.TransactionID != "txn-1" {
			t.Fatalf("以新交易号替代了首次核验 %+v %v", review, err)
		}
	})

	t.Run("迟到查单不可覆盖已提交修订", func(t *testing.T) {
		queried, release := make(chan struct{}), make(chan struct{})
		late := paymentOrderTestVerifier(t, other, func(ctx context.Context, _ payment.OrderQuery) (payment.ProviderOrder, error) {
			close(queried)
			select {
			case <-release:
			case <-ctx.Done():
				return payment.ProviderOrder{}, ctx.Err()
			}
			refund := paymentOrderTestPaid(other, "txn-2", paidAt)
			refund.State = payment.ProviderRefunded
			return refund, nil
		})
		result := make(chan error, 1)
		go func() {
			_, err := s.ReconcilePaymentOrder(ctx, late, other.ID)
			result <- err
		}()
		select {
		case <-queried:
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
		matching := paymentOrderTestVerifier(t, other, func(context.Context, payment.OrderQuery) (payment.ProviderOrder, error) {
			return paymentOrderTestPaid(other, "txn-2", paidAt), nil
		})
		_, err := s.ReconcilePaymentOrder(ctx, matching, other.ID)
		close(release)
		paymentOrderTestOK(t, err)
		paymentOrderTestCode(t, <-result, p.CodeOperationConflict)
		confirmed, err := s.GetPaymentOrder(ctx, other.ID)
		paymentOrderTestOK(t, err)
		if !confirmed.hasVerifiedPayment() {
			t.Fatalf("迟到的查单覆盖了已提交的支付证明 %+v", confirmed)
		}
		refunded := paymentOrderTestVerifier(t, other, func(context.Context, payment.OrderQuery) (payment.ProviderOrder, error) {
			observation := paymentOrderTestPaid(other, "txn-2", paidAt)
			observation.State = payment.ProviderRefunded
			return observation, nil
		})
		observed, err := s.ReconcilePaymentOrder(ctx, refunded, other.ID)
		if !errors.Is(err, payment.ErrRefunded) || observed.State != PaymentOrderReview || observed.hasVerifiedPayment() ||
			observed.Reconciliation.Status != PaymentReconciliationRefunded || observed.Verification.TransactionID != "txn-2" {
			t.Fatalf("已核验未履约原单未按渠道退款阻断 %+v %v", observed, err)
		}
	})
	role, err := s.Role(ctx, a.RoleID)
	paymentOrderTestOK(t, err)
	if role.Wallet.Yuanbao != 0 || role.Wallet.BoundYuanbao != 0 {
		t.Fatalf("渠道核验阶段写入了角色资产 %+v", role.Wallet)
	}
}

// paymentOrderMongo 只接受验收工具指定的专属副本集，为当前用例分配随机数据库。
func paymentOrderMongo(t *testing.T) (context.Context, *Store) {
	t.Helper()
	uri := os.Getenv("MINIMIR_INTEGRATION_TEST_MONGO_URI")
	if os.Getenv("MINIMIR_MONGO_E2E") != "1" || uri == "" {
		t.Skip("需要独立 MongoDB 副本集")
	}
	cfg, err := devconfig.Load("../../../conf/dev/server.json")
	paymentOrderTestOK(t, err)
	cfg.MongoURI, cfg.Database = uri, "minimir_dev_game_test_payment_order_"+domain.ID()[:12]
	ctx, cancel := context.WithTimeout(t.Context(), 35*time.Second)
	t.Cleanup(cancel)
	s, err := Open(ctx, cfg)
	paymentOrderTestOK(t, err)
	t.Cleanup(func() {
		cleanup, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		defer s.Close(cleanup)
		paymentOrderTestOK(t, s.DB.Drop(cleanup))
	})
	return ctx, s
}

// paymentOrderActor 建立持久账号和角色，使用账号层权限，无需附着游戏连接。
func paymentOrderActor(t *testing.T, ctx context.Context, s *Store) Auth {
	t.Helper()
	name := "pay_" + domain.ID()[:8]
	login, err := s.Login(ctx, name, "test_pay_123456", true)
	paymentOrderTestOK(t, err)
	auth, err := s.Authenticate(ctx, login.AccountToken)
	paymentOrderTestOK(t, err)
	role, err := s.CreateRole(ctx, auth, &pb.CreateRoleRequest{OriginServerId: s.Cfg.OriginID, Slot: 1, Name: name, OpId: domain.Bytes(domain.ID())})
	paymentOrderTestOK(t, err)
	auth.RoleID = domain.Hex(role.Key.RoleId)
	return auth
}

// paymentOrderTestChannel 只在测试中模拟经过渠道身份认证的主动查单。
type paymentOrderTestChannel func(context.Context, payment.OrderQuery) (payment.ProviderOrder, error)

// QueryOrder 调用该测试显式注入的渠道观察。
func (channel paymentOrderTestChannel) QueryOrder(ctx context.Context, query payment.OrderQuery) (payment.ProviderOrder, error) {
	return channel(ctx, query)
}

// paymentOrderTestVerifier 使用订单原商户构造隔离查单器。
func paymentOrderTestVerifier(t *testing.T, order PaymentOrder, channel paymentOrderTestChannel) *payment.Verifier {
	t.Helper()
	verifier, err := payment.NewVerifier(order.Quote.ProviderID, order.Quote.MerchantID, channel)
	paymentOrderTestOK(t, err)
	return verifier
}

// paymentOrderTestPaid 生成仅供测试使用的一致查单结果。
func paymentOrderTestPaid(order PaymentOrder, transactionID string, paidAt time.Time) payment.ProviderOrder {
	expected := order.Expected()
	return payment.ProviderOrder{OrderID: expected.ID, MerchantID: expected.MerchantID, ExternalOrderID: expected.ExternalOrderID,
		BeneficiaryReference: expected.BeneficiaryReference(), PurchaseKind: expected.PurchaseKind, ProductID: expected.ProductID,
		AmountFen: expected.AmountFen, Currency: expected.Currency, State: payment.ProviderPaid, TransactionID: transactionID, PaidAt: paidAt}
}

// paymentOrderTestOK 在原调用处报告失败。
func paymentOrderTestOK(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

// paymentOrderTestCode 验证失败使用预期的业务代码。
func paymentOrderTestCode(t *testing.T, err error, expected uint32) {
	t.Helper()
	if err == nil || Code(err) != expected {
		t.Fatalf("取得 %v，预期业务码 %d", err, expected)
	}
}
