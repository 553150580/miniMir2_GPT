// 本文件通过隔离 MongoDB 和真实 WebSocket 验证月卡确认、角色钱包和上海日领取。
package integration

import (
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"google.golang.org/protobuf/proto"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
	"minimir/internal/wire"
)

// TestHostingRoleCardsPurchaseMongoWS 验证真实购卡续期、原回执和免费领时差额。
func TestHostingRoleCardsPurchaseMongoWS(t *testing.T) {
	const privilegeID, hostingID = 7, 8
	const month = 30 * 24 * time.Hour
	const freeHours = int64(6 * 3600)
	ctx, store, endpoint, route := startChallengePlayerDeathServer(t, func(*devconfig.Config) {})
	login := &pb.LoginResult{}
	rpc(t, endpoint, "", p.MsgC2SLocalRegister, &pb.LocalLoginRequest{Account: "cardws_" + domain.ID()[:8], Password: "test_123456"}, login, 0)
	// 同账号的两个角色分别持有可支付和仅绑定的余额，权益必须独立。
	first := createDirectoryRole(t, endpoint, login.AccountToken, route.origin.OriginID, "月卡甲")
	second := &pb.RoleSummary{}
	rpc(t, endpoint, login.AccountToken, p.MsgC2SCreateRole, &pb.CreateRoleRequest{OriginServerId: route.origin.OriginID,
		Slot: 2, Job: 0, Name: "月卡乙" + domain.ID()[:5], OpId: domain.Bytes(domain.ID())}, second, 0)
	firstWallet := domain.Wallet{Yuanbao: 155_000, BoundYuanbao: 11_000, Gold: 9_000, BoundGold: 1_000}
	secondWallet := domain.Wallet{Yuanbao: 1, BoundYuanbao: 150_000, Gold: 90_000, BoundGold: 20_000}
	for _, seed := range []struct {
		role   *pb.RoleSummary // 隔离测试中已正常建角的角色身份。
		wallet domain.Wallet   // 仅在角色首次附着前写入的测试钱包。
	}{{first, firstWallet}, {second, secondWallet}} {
		update, err := store.DB.Collection("roles").UpdateByID(ctx, domain.Hex(seed.role.GetKey().GetRoleId()), bson.M{"$set": bson.M{"wallet": seed.wallet}})
		must(t, err)
		if update.ModifiedCount != 1 {
			t.Fatal("测试钱包未初始化", seed.role.GetName())
		}
	}
	attach := func(role *pb.RoleSummary) *client {
		t.Helper()
		ticket := directoryTicket(t, endpoint, login.AccountToken, role.Key, route)
		joined := join(t, directoryTicketAddress(t, ticket), ticket.Ticket)
		t.Cleanup(joined.close)
		joined.auto(false, 1)
		return joined
	}
	readRole := func(role *pb.RoleSummary) domain.Role {
		t.Helper()
		current, err := store.Role(ctx, domain.Hex(role.GetKey().GetRoleId()))
		must(t, err)
		return current
	}
	c, other := attach(first), attach(second)
	catalog := roleCardStateWS(c, 0)
	if catalog.ErrorCode != 0 || catalog.Quote != nil || len(catalog.Products) != 2 || catalog.DailyPrivilegeUnits != 2000 ||
		catalog.PrivilegeUntilUnixMs != 0 || catalog.HostingUntilUnixMs != 0 || catalog.PrivilegeClaimable {
		t.Fatal("新角色目录、每日金额或初始卡状态错误", catalog)
	}
	for index, expected := range []*pb.RoleCardProductView{
		{ProductId: privilegeID, Kind: devconfig.RoleCardKindPrivilege, Days: 30, PriceYuanbaoUnits: 30_000},
		{ProductId: hostingID, Kind: devconfig.RoleCardKindHosting, Days: 30, PriceYuanbaoUnits: 45_000},
	} {
		if !proto.Equal(catalog.Products[index], expected) {
			t.Fatal("目录未按工作簿开放两款非绑元宝月卡", catalog.Products)
		}
	}
	if unavailable := roleCardStateWS(c, 1); unavailable.ErrorCode != p.CodeLineForbidden || len(unavailable.GetQuote().GetQuoteId()) != 0 {
		t.Fatal("现金商品不应取得游戏内报价", unavailable)
	}
	if blocked := roleCardStateWS(other, privilegeID); blocked.ErrorCode != p.CodeVendorFunds || len(blocked.GetQuote().GetQuoteId()) != 0 {
		t.Fatal("绑定元宝与金币不应组成非绑元宝报价", blocked)
	}
	if result := roleCardAssetWS(c, p.MsgC2SClaimRoleCardDaily, &pb.ClaimRoleCardDailyRequest{OperatingDay: catalog.OperatingDay, Operation: c.operation()}); result.ErrorCode != p.CodeLineForbidden || result.RoleCardDaily != nil {
		t.Fatal("未持有特权卡即获得日领奖励", result)
	}
	claimHosting := func(expected int64, code uint32) {
		t.Helper()
		result := roleCardAssetWS(c, p.MsgC2SClaimHostingTime, &pb.ClaimHostingTimeRequest{BenefitKind: 0, Operation: c.operation()})
		current := readRole(first)
		if result.ErrorCode != code || current.HostingSeconds != expected || current.ClaimedSeconds != expected {
			t.Fatalf("免费领取或购卡补差错误：操作=%v，余额=%d，已领=%d", result, current.HostingSeconds, current.ClaimedSeconds)
		}
	}
	claimHosting(freeHours, 0)
	forged := &pb.BuyRoleCardRequest{ProductId: privilegeID, QuoteId: domain.Bytes(domain.ID()), Operation: c.operation()}
	if result := roleCardAssetWS(c, p.MsgC2SBuyRoleCard, forged); result.ErrorCode != p.CodeOperationConflict || result.RoleCardPurchase != nil {
		t.Fatal("未签发的报价已提交购卡", result)
	}
	quote := func(product uint32) *pb.RoleCardQuoteView {
		t.Helper()
		state := roleCardStateWS(c, product)
		q := state.Quote
		if state.ErrorCode != 0 || len(q.GetQuoteId()) != 16 || q.GetProduct().GetProductId() != product ||
			q.GetRuleVersion() != c.entry.ResourceVersion || q.GetConfigHash() != store.Cfg.Fixture.Hash || q.GetExpiresAtUnixMs() <= time.Now().UnixMilli() {
			t.Fatal("没有收到原角色的有效权威报价", state)
		}
		return q
	}
	privilegeQuote := quote(privilegeID)
	if stolen := roleCardAssetWS(other, p.MsgC2SBuyRoleCard, &pb.BuyRoleCardRequest{ProductId: privilegeID, QuoteId: privilegeQuote.QuoteId, Operation: other.operation()}); stolen.ErrorCode != p.CodeOperationConflict || stolen.RoleCardPurchase != nil {
		t.Fatal("同账号其他角色可使用原角色报价", stolen)
	}
	confirm := func(q *pb.RoleCardQuoteView, balance int64) (*pb.BuyRoleCardRequest, *pb.RoleCardPurchaseReceipt) {
		t.Helper()
		request := &pb.BuyRoleCardRequest{ProductId: q.Product.ProductId, QuoteId: q.QuoteId, Operation: c.operation()}
		result := roleCardAssetWS(c, p.MsgC2SBuyRoleCard, request)
		receipt := result.RoleCardPurchase
		if result.ErrorCode != 0 || !proto.Equal(receipt.GetQuote(), q) || receipt.GetYuanbaoBalanceUnits() != balance ||
			receipt.GetNewUntilUnixMs() != max(receipt.GetEffectiveAtUnixMs(), q.PreviousUntilUnixMs)+month.Milliseconds() {
			t.Fatal("成交没有还原原报价、账户扣款及续期锚点", result)
		}
		return request, receipt
	}
	buyPrivilege, privilegeReceipt := confirm(privilegeQuote, firstWallet.Yuanbao-30_000)
	afterPrivilege := roleCardStateWS(c, 0)
	if !afterPrivilege.PrivilegeClaimable || afterPrivilege.PrivilegeUntilUnixMs != privilegeReceipt.NewUntilUnixMs || afterPrivilege.HostingUntilUnixMs != 0 {
		t.Fatal("特权月卡不应开启托管期", afterPrivilege)
	}
	day, err := time.Parse("2006-01-02", afterPrivilege.OperatingDay)
	must(t, err)
	stale := &pb.ClaimRoleCardDailyRequest{OperatingDay: day.AddDate(0, 0, -1).Format("2006-01-02"), Operation: c.operation()}
	if result := roleCardAssetWS(c, p.MsgC2SClaimRoleCardDaily, stale); result.ErrorCode != p.CodeOperationConflict || result.RoleCardDaily != nil {
		t.Fatal("过时上海运营日仍可领取", result)
	}
	daily := &pb.ClaimRoleCardDailyRequest{OperatingDay: afterPrivilege.OperatingDay, Operation: c.operation()}
	dailyResult := roleCardAssetWS(c, p.MsgC2SClaimRoleCardDaily, daily)
	reward := dailyResult.RoleCardDaily
	if dailyResult.ErrorCode != 0 || reward.GetOperatingDay() != daily.OperatingDay || reward.GetGrantedUnits() != 2000 ||
		reward.GetBoundYuanbaoBalanceUnits() != firstWallet.BoundYuanbao+2000 ||
		daily.OperatingDay != time.UnixMilli(reward.GetAcceptedAtUnixMs()).UTC().Add(8*time.Hour).Format("2006-01-02") {
		t.Fatal("按上海日领取的绑定钱包回执错误", dailyResult)
	}
	duplicate := &pb.ClaimRoleCardDailyRequest{OperatingDay: daily.OperatingDay, Operation: c.operation()}
	if result := roleCardAssetWS(c, p.MsgC2SClaimRoleCardDaily, duplicate); result.ErrorCode != p.CodeOperationConflict || result.RoleCardDaily != nil {
		t.Fatal("同一上海日生成了第二笔绑定元宝", result)
	}
	_, hostingReceipt := confirm(quote(hostingID), firstWallet.Yuanbao-75_000)
	hostingState := roleCardStateWS(c, 0)
	if hostingState.PrivilegeClaimable || hostingState.PrivilegeUntilUnixMs != privilegeReceipt.NewUntilUnixMs || hostingState.HostingUntilUnixMs != hostingReceipt.NewUntilUnixMs {
		t.Fatal("托管卡购买改变了特权期限或日领资格", hostingState)
	}
	claimHosting(2*freeHours, 0)
	renewalQuote := quote(hostingID)
	if renewalQuote.PreviousUntilUnixMs != hostingReceipt.NewUntilUnixMs {
		t.Fatal("续卡报价未保留原托管到期时刻", renewalQuote)
	}
	_, renewed := confirm(renewalQuote, firstWallet.Yuanbao-120_000)
	claimHosting(2*freeHours, p.CodeOperationConflict)

	// 旧会话的两笔成功操作在新票后只能读出各自原回执，账户和命令水位不得再推进。
	beforeReplay := readRole(first)
	oldBinding := c.binding
	c.close()
	c = attach(first)
	if c.binding == oldBinding {
		t.Fatal("重连没有轮换会话围栏", c.binding)
	}
	if replay := roleCardAssetWS(c, p.MsgC2SBuyRoleCard, buyPrivilege); replay.ErrorCode != 0 || !proto.Equal(replay.RoleCardPurchase, privilegeReceipt) {
		t.Fatal("跨会话购买重放未返回原成交", replay)
	}
	if replay := roleCardAssetWS(c, p.MsgC2SClaimRoleCardDaily, daily); replay.ErrorCode != 0 || !proto.Equal(replay.RoleCardDaily, reward) {
		t.Fatal("跨会话日领重放未返回原到账", replay)
	}
	mutated := proto.Clone(buyPrivilege).(*pb.BuyRoleCardRequest)
	mutated.ProductId = hostingID
	if altered := roleCardAssetWS(c, p.MsgC2SBuyRoleCard, mutated); altered.ErrorCode != p.CodeOperationConflict || altered.RoleCardPurchase != nil {
		t.Fatal("同一操作编号可改换购买商品", altered)
	}
	final, unaffected := readRole(first), readRole(second)
	wallet := firstWallet
	wallet.Yuanbao -= 120_000
	wallet.BoundYuanbao += 2_000
	if final.Wallet != wallet || final.Wallet != beforeReplay.Wallet || final.EconomicSeq != beforeReplay.EconomicSeq || final.SourceSeq != beforeReplay.SourceSeq ||
		final.PrivilegeClaimDay != daily.OperatingDay || final.PrivilegeUntil.UnixMilli() != privilegeReceipt.NewUntilUnixMs || final.HostingCardUntil.UnixMilli() != renewed.NewUntilUnixMs ||
		final.HostingSeconds != 2*freeHours || unaffected.Wallet != secondWallet || !unaffected.PrivilegeUntil.IsZero() || !unaffected.HostingCardUntil.IsZero() {
		t.Fatalf("跨角色或跨会话影响了既有钱包、权益及原时长：角色一=%+v，角色二=%+v", final.Wallet, unaffected.Wallet)
	}
	var purchaseAudit struct {
		Receipt *persistence.RoleCardPurchaseReceipt `bson:"role_card_purchase"` // 同事务保存的原报价与买卡回执。
	}
	var dailyAudit struct {
		Receipt *persistence.RoleCardDailyReceipt `bson:"role_card_daily"` // 同事务保存的原每日领取回执。
	}
	must(t, store.DB.Collection("audit_outbox").FindOne(ctx, bson.M{"_id": "asset:" + final.ID + ":" + domain.Hex(buyPrivilege.Operation.OpId)}).Decode(&purchaseAudit))
	must(t, store.DB.Collection("audit_outbox").FindOne(ctx, bson.M{"_id": "asset:" + final.ID + ":" + domain.Hex(daily.Operation.OpId)}).Decode(&dailyAudit))
	if !proto.Equal(purchaseAudit.Receipt.Proto(), privilegeReceipt) || !proto.Equal(dailyAudit.Receipt.Proto(), reward) {
		t.Fatal("并置的 Mongo 审计未保留原交易与原日领回执", purchaseAudit.Receipt, dailyAudit.Receipt)
	}
}

// roleCardStateWS 用独立请求编号关联角色目录与权威报价。
func roleCardStateWS(c *client, product uint32) *pb.RoleCardState {
	c.t.Helper()
	request := c.send(p.MsgC2SRoleCardQuery, &pb.RoleCardQuery{SceneEpoch: c.entry.SceneEpoch, ProductId: product})
	frame := travelUntilWithin(c, 5*time.Second, func(f wire.Frame) bool {
		return f.ID == p.MsgS2CRoleCardState && f.Request == request
	})
	result := &pb.RoleCardState{}
	must(c.t, proto.Unmarshal(frame.Payload, result))
	if result.SceneEpoch != c.entry.SceneEpoch || result.RoleRevision < c.inventory.Revision {
		c.t.Fatal("月卡状态未关联当前场景和角色修订", result)
	}
	return result
}

// roleCardAssetWS 等待对应提交和完整角色快照，非资产拒绝不额外等库存。
func roleCardAssetWS(c *client, id uint16, request proto.Message) *pb.OperationResult {
	c.t.Helper()
	result := c.asset(id, request)
	if result.NewRevision > c.inventory.Revision {
		c.inventoryAt(result.NewRevision)
	}
	return result
}
