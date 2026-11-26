// 本文件在独立 MongoDB 副本集验收现金订单、四钱包及角色卡期的原子交付。
package persistence

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	"minimir/internal/payment"
	p "minimir/internal/protocol"
)

// paymentGrantTestAdapter 只在隔离测试中提供固定的渠道主动查单事实。
type paymentGrantTestAdapter func(context.Context, payment.OrderQuery) (payment.ProviderOrder, error)

func (f paymentGrantTestAdapter) QueryOrder(ctx context.Context, query payment.OrderQuery) (payment.ProviderOrder, error) {
	return f(ctx, query)
}

func requireGrantTest(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func requireGrantCode(t *testing.T, err error, code uint32) {
	t.Helper()
	if Code(err) != code {
		t.Fatalf("支付返回错误码 %d，预期 %d，原错误 %v", Code(err), code, err)
	}
}

// fixtureForPaymentGrant 允许专项使用工作簿实际产物，常规隔离回归可用等价的六款商品。
func fixtureForPaymentGrant(t *testing.T, fallback devconfig.Fixture) devconfig.Fixture {
	t.Helper()
	var raw []byte
	var err error
	if src := os.Getenv("MINIMIR_CASH_CARD_TEST_FIXTURE"); src != "" {
		raw, err = os.ReadFile(src)
	} else {
		fallback.Payment = &devconfig.PaymentConfig{CashCardProducts: []devconfig.CashCardProduct{
			{ID: 1, Kind: devconfig.RoleCardKindPrivilege, Days: 30, PriceFen: 3000},
			{ID: 2, Kind: devconfig.RoleCardKindPrivilege, Days: 90, PriceFen: 7800},
			{ID: 3, Kind: devconfig.RoleCardKindPrivilege, Days: 365, PriceFen: 29800},
			{ID: 4, Kind: devconfig.RoleCardKindHosting, Days: 30, PriceFen: 4500},
			{ID: 5, Kind: devconfig.RoleCardKindHosting, Days: 90, PriceFen: 11800},
			{ID: 6, Kind: devconfig.RoleCardKindHosting, Days: 365, PriceFen: 46800},
		}}
		raw, err = json.Marshal(fallback)
	}
	requireGrantTest(t, err)
	var fixture devconfig.Fixture
	requireGrantTest(t, json.Unmarshal(raw, &fixture))
	fixture.Hash = Hash(raw)
	requireGrantTest(t, fixture.Validate())
	if fixture.Payment == nil {
		t.Fatal("缺少隔离现金商品目录")
	}
	t.Logf("实际加载现金商品夹具 SHA-256=%s", fixture.Hash)
	return fixture
}

// grantTestQuery 将持久绑定投影为渠道事实。可独立修改事实以覆盖错误金额、角色或交易号。
func grantTestQuery(ctx context.Context, t *testing.T, order PaymentOrder, transaction string,
	adjust func(*payment.ProviderOrder)) (payment.VerifiedPayment, error) {
	t.Helper()
	expected := order.Expected()
	reply := payment.ProviderOrder{OrderID: order.ID, MerchantID: expected.MerchantID, ExternalOrderID: expected.ExternalOrderID,
		BeneficiaryReference: expected.BeneficiaryReference(), PurchaseKind: expected.PurchaseKind,
		ProductID: expected.ProductID, AmountFen: expected.AmountFen, Currency: payment.CurrencyCNY,
		State: payment.ProviderPaid, TransactionID: transaction, PaidAt: order.CreatedAt}
	if adjust != nil {
		adjust(&reply)
	}
	adapter := paymentGrantTestAdapter(func(_ context.Context, query payment.OrderQuery) (payment.ProviderOrder, error) {
		if query != (payment.OrderQuery{OrderID: order.ID, MerchantID: expected.MerchantID, ExternalOrderID: expected.ExternalOrderID}) {
			return payment.ProviderOrder{}, errors.New("隔离适配器只接受持久绑定的原单查询")
		}
		return reply, nil
	})
	verifier, err := payment.NewVerifier(expected.ProviderID, expected.MerchantID, adapter)
	requireGrantTest(t, err)
	return verifier.Verify(ctx, expected)
}

// assertGrantOperation 核对原单盖章、角色流水、四钱包及已到账回执的事务一致性。
func assertGrantOperation(t *testing.T, ctx context.Context, s *Store, original PaymentOrder,
	before, after domain.Role, op Operation, wallet domain.Wallet) PaymentGrantReceipt {
	t.Helper()
	if after.ID != before.ID || after.AccountID != before.AccountID || after.Origin != before.Origin || after.Revision != before.Revision+1 ||
		after.Owner != before.Owner || after.RunServerID != before.RunServerID || after.RunEpoch != before.RunEpoch ||
		after.SessionEpoch != before.SessionEpoch || after.LastEntryAttempt != before.LastEntryAttempt ||
		after.SourceSeq != before.SourceSeq || after.EconomicSeq != before.EconomicSeq || after.Wallet != wallet ||
		after.HostingSeconds != before.HostingSeconds || after.PrivilegeClaimDay != before.PrivilegeClaimDay ||
		!reflect.DeepEqual(after.Items, before.Items) || !reflect.DeepEqual(after.Report, before.Report) {
		t.Fatalf("履约改动了原玩家、战斗或会话流水，或钱包不守恒：%+v", after)
	}
	if op.ID != "payment:"+original.ID || op.Kind != paymentGrantKind || op.RoleID != before.ID ||
		op.Revision != after.Revision || op.CommandSeq != before.EconomicSeq || op.SourceSeq != before.SourceSeq || op.PaymentGrant == nil {
		t.Fatalf("实际到账缺少独立支付回执或保留流水证据：%+v", op)
	}
	grant := *op.PaymentGrant
	if grant.OrderID != original.ID || grant.AccountID != before.AccountID || grant.OriginServerID != before.Origin ||
		grant.Kind != original.Quote.Kind || grant.ProductID != original.Quote.ProductID || grant.AmountFen != original.Quote.AmountFen ||
		grant.RuleVersion != original.Quote.RuleVersion || grant.ConfigHash != original.Quote.ConfigHash ||
		grant.WalletBefore != before.Wallet || grant.WalletAfter != wallet || grant.YuanbaoAdded != wallet.Yuanbao-before.Wallet.Yuanbao ||
		!grant.PaidAt.Equal(original.Verification.PaidAt) || grant.GrantedAt.Before(original.VerifiedAt) {
		t.Fatalf("资金来源、原报价、实收金额或四钱包封账快照不一致：%+v", grant)
	}
	persisted, err := s.GetPaymentOrder(ctx, original.ID)
	requireGrantTest(t, err)
	if persisted.State != PaymentOrderFulfilled || persisted.Quote != original.Quote || persisted.AssetOperationRef != op.ID ||
		persisted.Revision != original.Revision+1 || !reflect.DeepEqual(persisted.Verification, original.Verification) ||
		!persisted.FulfilledAt.Equal(grant.GrantedAt) || !op.Created.Equal(grant.GrantedAt) {
		t.Fatalf("原订单与永久到账证据不同步：%+v", persisted)
	}
	var audit struct {
		Fact PaymentGrantReceipt `bson:"payment_grant"` // 实际资产事务保留的同一来源和钱包快照。
	}
	requireGrantTest(t, s.DB.Collection("audit_outbox").FindOne(ctx, bson.M{"_id": op.ID, "kind": paymentGrantKind,
		"role_id": before.ID, "order_id": original.ID, "transaction_id": original.Verification.TransactionID}).Decode(&audit))
	if !reflect.DeepEqual(audit.Fact, grant) {
		t.Fatal("审计与原支付回执保存了不同的资产事实", audit.Fact, grant)
	}
	return grant
}

// TestPaymentGrantMongo 完整走真钱包和现金两卡的测试渠道核验，覆盖旧运行所有权和重放。
func TestPaymentGrantMongo(t *testing.T) {
	uri := os.Getenv("MINIMIR_INTEGRATION_TEST_MONGO_URI")
	if os.Getenv("MINIMIR_MONGO_E2E") != "1" || uri == "" {
		t.Skip("需要独立 MongoDB 副本集")
	}
	base, err := devconfig.Load("../../../conf/dev/server.json")
	requireGrantTest(t, err)
	base.MongoURI, base.Database = uri, "minimir_dev_game_test_payment_grant_"+domain.ID()[:16]
	cfg := base
	cfg.Fixture = fixtureForPaymentGrant(t, base.Fixture)
	ctx, cancel := context.WithTimeout(t.Context(), 45*time.Second)
	defer cancel()
	account, err := Open(ctx, cfg)
	requireGrantTest(t, err)
	t.Cleanup(func() {
		cleanup, stop := context.WithTimeout(context.Background(), 6*time.Second)
		defer stop()
		defer account.Close(cleanup)
		requireGrantTest(t, account.DB.Drop(cleanup))
	})
	requireGrantTest(t, account.Singleton(ctx, "account"))
	openGame := func(gameConfig devconfig.Config) *Store {
		t.Helper()
		game, err := Open(ctx, gameConfig)
		requireGrantTest(t, err)
		t.Cleanup(func() { game.Close(context.Background()) })
		lease, err := account.ApproveGame(ctx, gameConfig.NodeID, game.Instance, gameConfig.RunID)
		requireGrantTest(t, err)
		game.Epoch = lease.Epoch
		return game
	}
	game := openGame(cfg)
	login := func() Auth {
		t.Helper()
		session, err := account.Login(ctx, "cash_"+domain.ID()[:8], "test_123456", true)
		requireGrantTest(t, err)
		auth, err := account.Authenticate(ctx, session.AccountToken)
		requireGrantTest(t, err)
		return auth
	}
	auth, foreign := login(), login()
	create := func(slot uint32, name string) (*pb.RoleKey, Auth) {
		t.Helper()
		role, err := account.CreateRole(ctx, auth, &pb.CreateRoleRequest{OriginServerId: cfg.OriginID,
			Slot: slot, Job: 0, Name: name, OpId: domain.Bytes(domain.ID())})
		requireGrantTest(t, err)
		selected := auth
		selected.RoleID = domain.Hex(role.Key.RoleId)
		return role.Key, selected
	}
	roleKey, original := create(1, "现金测试甲")
	_, other := create(2, "现金测试乙")
	readRole := func(id string) domain.Role {
		t.Helper()
		role, err := account.Role(ctx, id)
		requireGrantTest(t, err)
		return role
	}
	attach := func(node *Store) domain.Role {
		t.Helper()
		ticket, err := account.IssueTicket(ctx, auth, roleKey)
		requireGrantTest(t, err)
		role, _, err := node.Enter(ctx, ticket.Ticket)
		requireGrantTest(t, err)
		return role
	}
	at := bossTime(time.Now())
	wallet := domain.Wallet{Gold: 135, BoundGold: 246, Yuanbao: 357, BoundYuanbao: 468}
	_, err = account.DB.Collection("roles").UpdateByID(ctx, original.RoleID, bson.M{"$set": bson.M{
		"wallet": wallet, "economic_command_seq": int64(17), "source_seq": int64(43), "hosting_seconds": int64(123),
		"privilege_since": at.Add(-48 * time.Hour), "privilege_until": at.Add(72 * time.Hour),
		"hosting_card_since": at.Add(-96 * time.Hour), "hosting_card_until": at.Add(-24 * time.Hour),
		"privilege_claim_day": "2026-11-20",
	}})
	requireGrantTest(t, err)
	before := readRole(original.RoleID)
	otherBefore := readRole(other.RoleID)
	if before.Owner != "" || before.SessionEpoch != 0 || before.RunEpoch != 0 || before.Wallet != wallet {
		t.Fatal("测试必须从未附着的真实新角色开始", before)
	}

	quote := PaymentOrderQuote{ProviderID: "isolated-payment-grant", MerchantID: "isolated-merchant",
		RuleVersion: cfg.Fixture.RuleVersion, ConfigHash: cfg.Fixture.Hash}
	recharge := quote
	// 这是只由隔离测试调用者装配的可信充值报价，校验 137 分换算 1370 内部单位。
	recharge.Kind, recharge.ProductID, recharge.AmountFen, recharge.YuanbaoUnits = payment.KindRecharge, 20, 137, 1370
	cardQuote := func(id uint32, kind payment.Kind) PaymentOrderQuote {
		t.Helper()
		product, ok := cfg.Fixture.CashCardProduct(id)
		if !ok {
			t.Fatal("原工作簿中找不到测试现金商品", id)
		}
		q := quote
		q.Kind, q.ProductID, q.AmountFen, q.CardDays = kind, product.ID, product.PriceFen, product.Days
		return q
	}
	bind := func(order PaymentOrder) PaymentOrder {
		t.Helper()
		bound, err := account.BindPaymentOrder(ctx, order.ID, quote.ProviderID, quote.MerchantID, "isolated-"+order.ID)
		requireGrantTest(t, err)
		return bound
	}
	openOrder := func(q PaymentOrderQuote) PaymentOrder {
		t.Helper()
		order, err := account.CreatePaymentOrder(ctx, original, domain.ID(), q)
		requireGrantTest(t, err)
		return bind(order)
	}
	verify := func(order PaymentOrder, transaction string) PaymentOrder {
		t.Helper()
		proof, err := grantTestQuery(ctx, t, order, transaction, nil)
		requireGrantTest(t, err)
		result, err := account.RecordVerifiedPayment(ctx, proof)
		requireGrantTest(t, err)
		if result.State != PaymentOrderVerified || result.Verification == nil || result.Verification.TransactionID != transaction {
			t.Fatal("可信核验未进入原单的待履约状态", result)
		}
		return result
	}

	requestID := domain.ID()
	order, err := account.CreatePaymentOrder(ctx, original, requestID, recharge)
	requireGrantTest(t, err)
	_, _, err = game.FulfillPaymentOrder(ctx, order.ID)
	requireGrantCode(t, err, p.CodeOperationConflict)
	foreign.RoleID = original.RoleID
	_, err = account.CreatePaymentOrder(ctx, foreign, domain.ID(), recharge)
	requireGrantCode(t, err, p.CodeUnauthenticated)
	_, err = account.CreatePaymentOrder(ctx, other, requestID, recharge)
	requireGrantCode(t, err, p.CodeOperationConflict)
	alteredCard := cardQuote(2, payment.KindPrivilegeCard)
	alteredCard.AmountFen++
	_, err = account.CreatePaymentOrder(ctx, original, domain.ID(), alteredCard)
	requireGrantCode(t, err, p.CodeOperationConflict)
	order = bind(order)
	_, _, err = game.FulfillPaymentOrder(ctx, order.ID)
	requireGrantCode(t, err, p.CodeOperationConflict)
	wrongRecipient := order.Expected()
	wrongRecipient.RoleID = other.RoleID
	for _, adjust := range []func(*payment.ProviderOrder){
		func(result *payment.ProviderOrder) { result.AmountFen++ },
		func(result *payment.ProviderOrder) {
			result.BeneficiaryReference = wrongRecipient.BeneficiaryReference()
		},
	} {
		proof, err := grantTestQuery(ctx, t, order, "bad-transaction", adjust)
		if !errors.Is(err, payment.ErrOrderMismatch) || proof.Valid() {
			t.Fatal("错误金额或受益角色获得了可信支付证明", err)
		}
		_, err = account.RecordVerifiedPayment(ctx, proof)
		requireGrantCode(t, err, p.CodeInvalidArgument)
	}
	if saved := readRole(original.RoleID); saved.Revision != before.Revision || saved.Wallet != before.Wallet {
		t.Fatal("原单未核验时已经动了原钱包", saved.Wallet, saved.Revision)
	}

	order = verify(order, "isolated-recharge-transaction")
	duplicate := openOrder(recharge)
	proof, err := grantTestQuery(ctx, t, duplicate, order.Verification.TransactionID, nil)
	requireGrantTest(t, err)
	_, err = account.RecordVerifiedPayment(ctx, proof)
	requireGrantCode(t, err, p.CodeOperationConflict)
	_, _, err = game.FulfillPaymentOrder(ctx, duplicate.ID)
	requireGrantCode(t, err, p.CodeOperationConflict)
	queued, err := game.NextVerifiedPayment(ctx, PaymentOrder{})
	requireGrantTest(t, err)
	if queued.ID != order.ID {
		t.Fatal("待履约队列选择了无效或重复交易", queued.ID)
	}

	type repliedGrant struct {
		role domain.Role
		op   Operation
		err  error
	}
	const workers = 6
	start, grants := make(chan struct{}), make(chan repliedGrant, workers)
	for range workers {
		go func() {
			<-start
			role, op, err := game.FulfillPaymentOrder(ctx, order.ID)
			grants <- repliedGrant{role, op, err}
		}()
	}
	close(start)
	first := <-grants
	requireGrantTest(t, first.err)
	wallet.Yuanbao += recharge.YuanbaoUnits
	grant := assertGrantOperation(t, ctx, game, order, before, first.role, first.op, wallet)
	if grant.Card != nil || first.role.SessionEpoch != 0 || !first.role.PrivilegeUntil.Equal(before.PrivilegeUntil) ||
		!first.role.HostingCardUntil.Equal(before.HostingCardUntil) {
		t.Fatal("未附着角色的充值误授卡期或创建会话", grant)
	}
	for range workers - 1 {
		result := <-grants
		requireGrantTest(t, result.err)
		if !reflect.DeepEqual(result.op, first.op) || result.role.Revision != first.role.Revision || result.role.Wallet != wallet {
			t.Fatal("并发履约未回到同一永久回执和唯一到账结果", result.op)
		}
	}
	queued, err = game.NextVerifiedPayment(ctx, PaymentOrder{})
	requireGrantTest(t, err)
	if queued.ID != "" {
		t.Fatal("原到账订单或重复交易仍留在可履约队列", queued)
	}

	beforePrivilege := attach(game)
	privilege := verify(openOrder(cardQuote(2, payment.KindPrivilegeCard)), "isolated-cash-privilege")
	privileged, privilegeOp, err := game.FulfillPaymentOrder(ctx, privilege.ID)
	requireGrantTest(t, err)
	privilegeGrant := assertGrantOperation(t, ctx, game, privilege, beforePrivilege, privileged, privilegeOp, wallet)
	if privilegeGrant.Card == nil || privilegeGrant.Card.Kind != devconfig.RoleCardKindPrivilege || privilegeGrant.Card.Days != 90 ||
		!privilegeGrant.Card.PreviousUntil.Equal(beforePrivilege.PrivilegeUntil) || !privileged.PrivilegeSince.Equal(beforePrivilege.PrivilegeSince) ||
		!privileged.PrivilegeUntil.Equal(beforePrivilege.PrivilegeUntil.Add(90*24*time.Hour)) ||
		!privileged.HostingCardUntil.Equal(beforePrivilege.HostingCardUntil) {
		t.Fatal("连续特权购卡没有延长原卡期或误改托管权益", privilegeGrant.Card)
	}
	hosting := verify(openOrder(cardQuote(6, payment.KindHostingCard)), "isolated-cash-hosting")
	requireGrantTest(t, game.Release(ctx, "game"))
	replacement := openGame(base)
	if replacement.Epoch <= game.Epoch || replacement.Cfg.Fixture.Payment != nil || replacement.Cfg.Fixture.Hash == hosting.Quote.ConfigHash {
		t.Fatal("新所有者未以不同配置和更高代次接管")
	}
	beforeHosting := attach(replacement)
	if beforeHosting.SessionEpoch <= privileged.SessionEpoch || beforeHosting.RunEpoch != replacement.Epoch {
		t.Fatal("绑定没有换到当前运行服代次", beforeHosting.SessionEpoch, beforeHosting.RunEpoch)
	}
	_, _, err = game.FulfillPaymentOrder(ctx, hosting.ID)
	requireGrantCode(t, err, p.CodeRouteChanged)
	hosted, hostingOp, err := replacement.FulfillPaymentOrder(ctx, hosting.ID)
	requireGrantTest(t, err)
	hostingGrant := assertGrantOperation(t, ctx, replacement, hosting, beforeHosting, hosted, hostingOp, wallet)
	if hostingGrant.Card == nil || hostingGrant.Card.Kind != devconfig.RoleCardKindHosting || hostingGrant.Card.Days != 365 ||
		!hostingGrant.Card.PreviousUntil.Equal(beforeHosting.HostingCardUntil) ||
		!hosted.HostingCardSince.Equal(hostingGrant.GrantedAt) || !hosted.HostingCardUntil.Equal(hosted.HostingCardSince.Add(365*24*time.Hour)) ||
		!hosted.PrivilegeUntil.Equal(privileged.PrivilegeUntil) {
		t.Fatal("过期托管卡未按原现金条款从实际履约时刻重开，或改动特权", hostingGrant.Card)
	}

	for _, fact := range []Operation{privilegeOp, hostingOp} {
		var period roleCardPeriodChange
		requireGrantTest(t, account.DB.Collection(roleCardPeriodCollection).FindOne(ctx, bson.M{"_id": fact.ID}).Decode(&period))
		g := fact.PaymentGrant
		if period.Kind != g.Card.Kind || period.RoleID != original.RoleID || period.Origin != cfg.OriginID || period.Revision != fact.Revision ||
			!period.PreviousUntil.Equal(g.Card.PreviousUntil) || !period.PreviousSince.Equal(g.Card.PreviousSince) || !period.EffectiveAt.Equal(g.GrantedAt) {
			t.Fatal("权益历史没有沿原角色、原时钟与到账事务冻结", period)
		}
	}
	for _, existing := range []Operation{first.op, privilegeOp, hostingOp} {
		current, receipt, err := replacement.FulfillPaymentOrder(ctx, existing.PaymentGrant.OrderID)
		requireGrantTest(t, err)
		if !reflect.DeepEqual(receipt, existing) || current.Revision != hosted.Revision || current.Wallet != hosted.Wallet {
			t.Fatal("新运行代次重放改了原回执或角色", receipt)
		}
	}
	count := func(collection string, filter bson.M) int64 {
		t.Helper()
		n, err := account.DB.Collection(collection).CountDocuments(ctx, filter)
		requireGrantTest(t, err)
		return n
	}
	if count(paymentOrderCollection, bson.M{"state": PaymentOrderFulfilled}) != 3 ||
		count(paymentOrderCollection, bson.M{"_id": duplicate.ID, "state": PaymentOrderBound, "verification": nil}) != 1 ||
		count("operations", bson.M{"role_id": original.RoleID, "kind": paymentGrantKind}) != 3 ||
		count("audit_outbox", bson.M{"role_id": original.RoleID, "kind": paymentGrantKind}) != 3 ||
		count(roleCardPeriodCollection, bson.M{"role_id": original.RoleID}) != 2 ||
		!reflect.DeepEqual(readRole(other.RoleID), otherBefore) {
		t.Fatal("原资金、两类卡期或永久审计出现漏记/重记，或跨角色发放")
	}
}
