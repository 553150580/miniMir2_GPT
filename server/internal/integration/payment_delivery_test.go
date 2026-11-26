// 本文件通过真实账号和游戏 WebSocket 验证核验完成后的在线与离线履约。
package integration

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	"minimir/internal/payment"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
	"minimir/internal/wire"
)

// paymentTestProvider 只在隔离用例接入，返回模拟渠道已保存的事实。
type paymentTestProvider struct{ actual payment.ProviderOrder }

func (provider paymentTestProvider) QueryOrder(_ context.Context, query payment.OrderQuery) (payment.ProviderOrder, error) {
	if query.OrderID != provider.actual.OrderID || query.MerchantID != provider.actual.MerchantID || query.ExternalOrderID != provider.actual.ExternalOrderID {
		return payment.ProviderOrder{}, payment.ErrInvalidOrder
	}
	return provider.actual, nil
}

// TestPaymentDeliveryMongoWS 由账号装配原单，由当前游戏节点后台完成到账并更新在线用户。
func TestPaymentDeliveryMongoWS(t *testing.T) {
	if os.Getenv("MINIMIR_MONGO_E2E") != "1" || os.Getenv("MINIMIR_INTEGRATION_TEST_MONGO_URI") == "" {
		t.Skip("需要核验的隔离 MongoDB 副本集")
	}
	fixtureBytes, err := os.ReadFile(os.Getenv("MINIMIR_CASH_CARD_TEST_FIXTURE"))
	must(t, err)
	fixtureSHA := sha256.Sum256(fixtureBytes)
	var exported struct {
		Payment *devconfig.PaymentConfig `json:"payment"` // 由唯一玩法工作簿生成的现金卡目录。
	}
	must(t, json.Unmarshal(fixtureBytes, &exported))
	if exported.Payment == nil {
		t.Fatal("隔离夹具缺少现金目录")
	}
	ctx, store, endpoint, route := startChallengePlayerDeathServer(t, func(cfg *devconfig.Config) {
		cfg.Fixture.Payment, cfg.Fixture.Hash = exported.Payment, hex.EncodeToString(fixtureSHA[:])
	})
	login := &pb.LoginResult{}
	rpc(t, endpoint, "", p.MsgC2SLocalRegister, &pb.LocalLoginRequest{Account: "payws_" + domain.ID()[:8], Password: "test_123456"}, login, 0)
	first := createDirectoryRole(t, endpoint, login.AccountToken, route.origin.OriginID, "支付甲")
	second := &pb.RoleSummary{}
	rpc(t, endpoint, login.AccountToken, p.MsgC2SCreateRole, &pb.CreateRoleRequest{OriginServerId: route.origin.OriginID,
		Slot: 2, Job: 0, Name: "支付乙" + domain.ID()[:5], OpId: domain.Bytes(domain.ID())}, second, 0)
	attach := func(role *pb.RoleSummary) *client {
		t.Helper()
		ticket := directoryTicket(t, endpoint, login.AccountToken, role.Key, route)
		joined := join(t, directoryTicketAddress(t, ticket), ticket.Ticket)
		t.Cleanup(joined.close)
		joined.auto(false, 1)
		return joined
	}
	readRole := func(summary *pb.RoleSummary) domain.Role {
		t.Helper()
		role, err := store.Role(ctx, domain.Hex(summary.Key.RoleId))
		must(t, err)
		return role
	}
	leased := attach(first)
	firstBefore, secondBefore := readRole(first), readRole(second)
	account, err := store.Authenticate(ctx, login.AccountToken)
	must(t, err)
	confirm := func(role domain.Role, kind payment.Kind, product uint32, amount, yuanbao int64, days uint32) persistence.PaymentOrder {
		t.Helper()
		auth := account
		auth.RoleID = role.ID
		order, err := store.CreatePaymentOrder(ctx, auth, domain.ID(), persistence.PaymentOrderQuote{Kind: kind,
			ProductID: product, AmountFen: amount, CardDays: days, YuanbaoUnits: yuanbao, RuleVersion: store.Cfg.Fixture.RuleVersion,
			ConfigHash: store.Cfg.Fixture.Hash, ProviderID: "isolated-provider", MerchantID: "isolated-cash-cards"})
		must(t, err)
		order, err = store.BindPaymentOrder(ctx, order.ID, order.Quote.ProviderID, order.Quote.MerchantID, domain.ID())
		must(t, err)
		provider := paymentTestProvider{actual: payment.ProviderOrder{OrderID: order.ID, MerchantID: order.Quote.MerchantID,
			ExternalOrderID: order.ExternalOrderID, PurchaseKind: kind, ProductID: product, AmountFen: amount, Currency: payment.CurrencyCNY,
			BeneficiaryReference: order.Expected().BeneficiaryReference(), State: payment.ProviderPaid, TransactionID: domain.ID(), PaidAt: time.Now()}}
		verifier, err := payment.NewVerifier(order.Quote.ProviderID, order.Quote.MerchantID, provider)
		must(t, err)
		order, err = store.ReconcilePaymentOrder(ctx, verifier, order.ID)
		must(t, err)
		return order
	}
	recharge := confirm(firstBefore, payment.KindRecharge, 301, 189, 1890, 0)
	hosting, ok := store.Cfg.Fixture.CashCardProduct(5)
	if !ok {
		t.Fatal("测试目录缺少托管季卡")
	}
	cash := confirm(secondBefore, payment.KindHostingCard, hosting.ID, hosting.PriceFen, 0, hosting.Days)
	// 充值依靠游戏节点后台到账，客户端只等待原钱包推送。
	wallet := &pb.WalletState{}
	travelUntilWithin(leased, 32*time.Second, func(frame wire.Frame) bool {
		if frame.ID != p.MsgS2CWallet || frame.Error != 0 {
			return false
		}
		must(t, proto.Unmarshal(frame.Payload, wallet))
		return wallet.YuanbaoUnits == firstBefore.Wallet.Yuanbao+1890
	})
	firstAfter := readRole(first)
	if firstAfter.Revision != firstBefore.Revision+1 || firstAfter.EconomicSeq != firstBefore.EconomicSeq || firstAfter.SourceSeq != firstBefore.SourceSeq ||
		firstAfter.Wallet.BoundYuanbao != firstBefore.Wallet.BoundYuanbao || firstAfter.Wallet.Gold != firstBefore.Wallet.Gold ||
		firstAfter.Wallet.BoundGold != firstBefore.Wallet.BoundGold {
		t.Fatal("充值影响玩家经济序号、战斗流水或非目标钱包", firstAfter.Wallet)
	}
	deadline := time.Now().Add(25 * time.Second)
	for {
		order, err := store.GetPaymentOrder(ctx, cash.ID)
		must(t, err)
		if order.State == persistence.PaymentOrderFulfilled {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("没有为离线原角色正常开通现金卡")
		}
		time.Sleep(25 * time.Millisecond)
	}
	secondAfter := readRole(second)
	if secondAfter.Wallet != secondBefore.Wallet || secondAfter.Owner != secondBefore.Owner ||
		secondAfter.HostingCardUntil.Sub(secondAfter.HostingCardSince) != time.Duration(hosting.Days)*24*time.Hour || !secondAfter.PrivilegeUntil.IsZero() {
		t.Fatal("现金卡影响钱包、会话或其它卡期", secondAfter.Wallet)
	}
	leased.close()
	if rejoined := attach(first); rejoined.binding == leased.binding || roleCardStateWS(rejoined, 0).PrivilegeUntilUnixMs != 0 {
		t.Fatal("重新入场后的账号或卡状态错误")
	}
	if observed := roleCardStateWS(attach(second), 0); observed.HostingUntilUnixMs != secondAfter.HostingCardUntil.UnixMilli() {
		t.Fatal("原离线角色入场后未观察到实际卡期", observed)
	}
	for _, id := range []string{recharge.ID, cash.ID} {
		order, err := store.GetPaymentOrder(ctx, id)
		must(t, err)
		if order.State != persistence.PaymentOrderFulfilled || order.AssetOperationRef != "payment:"+id {
			t.Fatal("原支付订单未保留唯一履约指针", id)
		}
	}
}
