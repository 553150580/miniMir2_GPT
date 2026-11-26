// 本文件用真实 WebSocket 和隔离 MongoDB 验证金币委托、撮合、双币交付与共用税余数。
package integration

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"google.golang.org/protobuf/proto"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
	"minimir/internal/wire"
)

const (
	goldTestPlace  uint32 = iota + 1 // 挂单只确认原冻结和委托。
	goldTestCancel                   // 原角色按订单身份撤销剩余委托。
	goldTestClaim                    // 原角色领取完整的互斥币种托管项。
)

// TestGoldMarketDefaultClosedMongoWS 验证主夹具没有开放未审定的行情或新单。
func TestGoldMarketDefaultClosedMongoWS(t *testing.T) {
	_, _, endpoint, route := startChallengePlayerDeathServer(t, func(*devconfig.Config) {})
	c, _, _ := joinWorldBossTestRole(t, endpoint, route)
	t.Cleanup(c.close)
	state := goldStateWS(c, 1)
	if state.Enabled || state.CanTrade || state.Rules != nil || len(state.Orders) != 0 || len(state.Escrows) != 0 {
		t.Fatal("没有隔离输入的主规则提前开放金币市场", state)
	}
}

// TestGoldMarketLifecycleMongoWS 核对按价时优先成交、部分余量、到期撤单与跨市场收税。
func TestGoldMarketLifecycleMongoWS(t *testing.T) {
	ctx, store, endpoint, route := startChallengePlayerDeathServer(t, func(cfg *devconfig.Config) {
		configureGoldMarketWS(t, cfg)
	})
	readRole := func(role *marketTestRole) domain.Role {
		t.Helper()
		stored, err := store.Role(ctx, domain.Hex(role.key.RoleId))
		must(t, err)
		return stored
	}
	newAccount := func() string {
		t.Helper()
		login := &pb.LoginResult{}
		rpc(t, endpoint, "", p.MsgC2SLocalRegister,
			&pb.LocalLoginRequest{Account: "goldws_" + domain.ID()[:8], Password: "test_123456"}, login, 0)
		return login.AccountToken
	}
	seed := func(token string, slot uint32, items ...domain.Item) *marketTestRole {
		t.Helper()
		role := &pb.RoleSummary{}
		rpc(t, endpoint, token, p.MsgC2SCreateRole, &pb.CreateRoleRequest{OriginServerId: route.origin.OriginID,
			Slot: slot, Job: 0, Name: "金币" + domain.ID()[:6], OpId: domain.Bytes(domain.ID())}, role, 0)
		prepared := &marketTestRole{key: role.Key}
		initial := readRole(prepared)
		// 只在首次附着前准备隔离钱包、有效资格和一件原来源道具，此后金币和元宝变化均由真实市场事务提交。
		updated, err := store.DB.Collection("roles").UpdateByID(ctx, initial.ID, bson.M{"$set": bson.M{
			"items": append(initial.Items, items...), "privilege_until": time.Now().Add(time.Hour),
			"wallet": domain.Wallet{Gold: 50_000, BoundGold: 88_888, Yuanbao: 1_000, BoundYuanbao: 9_999},
		}})
		must(t, err)
		if updated.ModifiedCount != 1 {
			t.Fatal("无法准备独立账号市场前态", role.Name)
		}
		prepared.attach = func() {
			if prepared.client != nil {
				prepared.client.close()
			}
			ticket := directoryTicket(t, endpoint, token, prepared.key, route)
			c := join(t, directoryTicketAddress(t, ticket), ticket.Ticket)
			t.Cleanup(c.close)
			c.auto(false, 1)
			prepared.client = c
		}
		prepared.attach()
		return prepared
	}
	sellerToken, buyerToken := newAccount(), newAccount()
	item := domain.Item{ID: domain.ID(), ItemID: 35, Quantity: 1, Version: 1, SourceKind: "monster_drop", SourceID: domain.ID()}
	firstSeller, secondSeller, partialSeller := seed(sellerToken, 1, item), seed(sellerToken, 2), seed(sellerToken, 3)
	buyer, buyerAlt := seed(buyerToken, 1), seed(buyerToken, 2)
	seedMarketTaxWithItemTrade(t, ctx, store, firstSeller, buyer, item)

	state := goldStateWS(buyer.client, 2)
	rules := state.Rules
	if !state.Enabled || !state.CanTrade || rules.GetReferenceUnitPriceYuanbaoUnits() != 100 ||
		rules.GetMinUnitPriceYuanbaoUnits() != 80 || rules.GetMaxUnitPriceYuanbaoUnits() != 120 || rules.GetGoldPerLot() != 10_000 ||
		rules.GetConfigHash() != store.Cfg.Fixture.Hash || rules.GetOrderDurationSeconds() != 120 || rules.GetSellerCommissionBp() != 1000 ||
		rules.GetMaxOrdersPerSide() != 5 || rules.GetPriceStepYuanbaoUnits() != 1 || len(rules.GetTermsHash()) != 64 {
		t.Fatal("金币目录缺少独立权限、候选参考价及工作簿精度", state)
	}
	invalid := goldTermsForTest(rules, persistence.GoldMarketBuy, 1, 99)
	invalid.TermsHash = strings.Repeat("f", 64)
	goldActionWS(buyer.client, invalid, p.CodeOperationConflict)
	goldActionWS(buyer.client, goldTermsForTest(rules, persistence.GoldMarketBuy, 1, 121), p.CodeOperationConflict)
	goldActionWS(buyer.client, goldTermsForTest(rules, persistence.GoldMarketBuy, 9, 120), p.CodeVendorFunds)
	goldActionWS(firstSeller.client, goldTermsForTest(rules, persistence.GoldMarketSell, 6, 91), p.CodeVendorFunds)

	// 先挂小贵价、再挂同账号最优价、再挂两笔同价。真实成交须按价时优先并跳过同账号。
	partial := goldActionWS(partialSeller.client, goldTermsForTest(rules, persistence.GoldMarketSell, 2, 101), 0)
	self := goldActionWS(buyerAlt.client, goldTermsForTest(rules, persistence.GoldMarketSell, 1, 80), 0)
	first := goldActionWS(firstSeller.client, goldTermsForTest(rules, persistence.GoldMarketSell, 1, 91), 0)
	second := goldActionWS(secondSeller.client, goldTermsForTest(rules, persistence.GoldMarketSell, 1, 91), 0)
	if partial.GoldDelta != -20_000 || first.GoldDelta != -10_000 {
		t.Fatal("卖单没有只冻结原非绑金币", partial, first)
	}
	for _, receipt := range []*pb.GoldMarketReceipt{partial, self, first, second} {
		order := goldDurableOrder(t, ctx, store, receipt.OrderId, func(o persistence.GoldMarketOrder) bool { return !o.PendingMatch })
		if order.State != persistence.GoldMarketActive || order.ExpiresAt.Sub(order.AcceptedAt) != 120*time.Second {
			t.Fatal("原委托没有按已确认客户端所见的显式期限入簿", order)
		}
	}
	buyRequest := goldTermsForTest(rules, persistence.GoldMarketBuy, 3, 110)
	buy := goldActionWS(buyer.client, buyRequest, 0)
	if buy.YuanbaoDeltaUnits != -330 || buy.GoldDelta != 0 || buy.FilledLots != 0 || buy.RemainingLots != 3 {
		t.Fatal("挂单回执必须只扣原报限价，并把成交交给撮合", buy)
	}
	goldDurableOrder(t, ctx, store, buy.OrderId, func(o persistence.GoldMarketOrder) bool { return o.State == persistence.GoldMarketFilled })
	var facts []persistence.GoldMarketFill
	cursor, err := store.DB.Collection("gold_market_fills").Find(ctx, bson.M{"buy_order_id": domain.Hex(buy.OrderId)},
		options.Find().SetSort(bson.D{{Key: "sequence", Value: 1}}))
	must(t, err)
	must(t, cursor.All(ctx, &facts))
	wants := []struct {
		order       *pb.GoldMarketReceipt // 原在簿卖单的身份。
		price       int64                 // 一手实际结算价，使用在簿价格。
		tax         int64                 // 本次卖方的实际佣金。
		improvement int64                 // 该笔应退买家原冻结元宝的差价。
	}{{first, 91, 10, 19}, {second, 91, 9, 19}, {partial, 101, 10, 9}}
	if len(facts) != len(wants) {
		t.Fatalf("三手买入应产生三笔跨账号在簿成交：%+v", facts)
	}
	for index, fact := range facts {
		want := wants[index]
		if fact.MakerOrderID != domain.Hex(want.order.OrderId) || fact.TakerOrderID != domain.Hex(buy.OrderId) ||
			fact.UnitPrice != want.price || fact.Gross != want.price || fact.Tax != want.tax || fact.BuyerRefund != want.improvement ||
			fact.Gold != 10_000 || fact.Lots != 1 || fact.Buyer.AccountID == fact.Seller.AccountID {
			t.Fatalf("第 %d 笔未按价时优先、在簿价、价差或共用佣金结算：%+v", index+1, fact)
		}
	}
	left := goldDurableOrder(t, ctx, store, partial.OrderId, func(o persistence.GoldMarketOrder) bool { return o.RemainingLots == 1 })
	if left.FrozenGold != 10_000 || left.State != persistence.GoldMarketActive {
		t.Fatal("部分成交后没有为余下的一手继续原路冻结", left)
	}
	selfOrder := goldDurableOrder(t, ctx, store, self.OrderId, func(o persistence.GoldMarketOrder) bool { return o.RemainingLots == 1 })
	if selfOrder.FrozenGold != 10_000 || selfOrder.State != persistence.GoldMarketActive {
		t.Fatal("同账号最优价不应产生自成交或丢失原冻结资产", selfOrder)
	}

	// 买方收到逐笔实际金币和价差，卖方原成交净收可在卡到期后领取。
	expireMarketTestPrivilege(t, ctx, store, partialSeller)
	for _, entry := range []struct {
		role  *marketTestRole       // 原出售角色。
		order *pb.GoldMarketReceipt // 原在簿订单。
		net   int64                 // 原逐笔成交的扣税净收。
	}{{firstSeller, first, 81}, {secondSeller, second, 82}, {partialSeller, partial, 91}} {
		claims := goldEscrowsWS(entry.role.client, entry.order.OrderId, persistence.GoldMarketClaimProceeds, 1)
		if claims[0].YuanbaoUnits != entry.net || claims[0].Gold != 0 || claims[0].Reason != persistence.GoldMarketReasonFill {
			t.Fatal("卖方原待领币种或净收错误", claims[0])
		}
		goldActionWS(entry.role.client, goldClaimForTest(claims[0]), 0)
	}
	var oldClaimRequest *pb.GoldMarketAction
	var oldClaimReceipt *pb.GoldMarketReceipt
	for index, claim := range goldEscrowsWS(buyer.client, buy.OrderId, persistence.GoldMarketClaimGold, 3) {
		if claim.YuanbaoUnits != 0 || claim.Gold != 10_000 || claim.Reason != persistence.GoldMarketReasonFill {
			t.Fatal("买方没有收到原成交的非绑金币", claim)
		}
		request := goldClaimForTest(claim)
		receipt := goldActionWS(buyer.client, request, 0)
		if index == 0 {
			oldClaimRequest, oldClaimReceipt = request, receipt
		}
	}
	var difference int64
	for _, claim := range goldEscrowsWS(buyer.client, buy.OrderId, persistence.GoldMarketClaimYuanbaoRefund, 3) {
		if claim.Gold != 0 || claim.Reason != persistence.GoldMarketReasonPriceDifference {
			t.Fatal("价差退款串错币种或原路原因", claim)
		}
		difference += goldActionWS(buyer.client, goldClaimForTest(claim), 0).YuanbaoDeltaUnits
	}
	if difference != 47 || goldStateWS(buyer.client, 1).GetRules().GetReferenceUnitPriceYuanbaoUnits() != 100 {
		t.Fatal("价差退款总额错误或少量成交错误覆盖候选参考价", difference)
	}

	// 只缩短独立原订单的截止触发实际排程，仍由原角色领回剩余币种。
	deadline, err := store.DB.Collection("gold_orders").UpdateOne(ctx, bson.M{"_id": left.ID, "revision": left.Revision},
		bson.M{"$set": bson.M{"expires_at": time.Now().Add(3 * time.Second).UTC().Truncate(time.Millisecond)}})
	must(t, err)
	if deadline.ModifiedCount != 1 {
		t.Fatal("独立余单截止无法定位", left.ID)
	}
	expireMarketTestPrivilege(t, ctx, store, buyerAlt)
	if goldStateWS(buyerAlt.client, 3).CanTrade {
		t.Fatal("过期角色仍可新增金币委托")
	}
	goldActionWS(buyerAlt.client, goldTermsForTest(rules, persistence.GoldMarketSell, 1, 91), p.CodeLineForbidden)
	selfView := goldOwnOrderWS(buyerAlt.client, self.OrderId)
	goldActionWS(buyerAlt.client, &pb.GoldMarketAction{Action: goldTestCancel, OrderId: selfView.OrderId, OrderVersion: selfView.Version}, 0)
	for _, refund := range []struct {
		role   *marketTestRole // 原退款角色。
		order  []byte          // 原冻结的委托编号。
		reason uint32          // 原到期或撤销原因。
	}{{partialSeller, partial.OrderId, persistence.GoldMarketReasonExpiry}, {buyerAlt, self.OrderId, persistence.GoldMarketReasonCancel}} {
		claim := goldEscrowsWS(refund.role.client, refund.order, persistence.GoldMarketClaimGoldReturn, 1)[0]
		if claim.Reason != refund.reason || claim.Gold != 10_000 || claim.YuanbaoUnits != 0 {
			t.Fatal("原冻结金币没有因到期或撤单原路退回", claim)
		}
		goldActionWS(refund.role.client, goldClaimForTest(claim), 0)
	}

	// 原两笔操作跨健康新票返回首个回执，新操作重复领回则被拒绝。
	beforeReplay := readRole(buyer)
	oldBinding := buyer.client.binding
	buyer.attach()
	if buyer.client.binding == oldBinding {
		t.Fatal("市场原角色健康重连未取得新围栏")
	}
	if got := goldActionWS(buyer.client, buyRequest, 0); !proto.Equal(got, buy) {
		t.Fatal("跨会话重放挂单未返回原冻结回执", got)
	}
	if got := goldActionWS(buyer.client, oldClaimRequest, 0); !proto.Equal(got, oldClaimReceipt) {
		t.Fatal("跨会话重放领款未返回原金币回执", got)
	}
	duplicate := proto.Clone(oldClaimRequest).(*pb.GoldMarketAction)
	duplicate.Operation = nil
	goldActionWS(buyer.client, duplicate, p.CodeOperationConflict)
	if actual := readRole(buyer); actual.Wallet != beforeReplay.Wallet {
		t.Fatal("重放或新操作领回重复改变原钱包", actual.Wallet)
	}
	assertGoldMarketBalances(t, ctx, store, []domain.Role{readRole(firstSeller), readRole(secondSeller), readRole(partialSeller), readRole(buyer), readRole(buyerAlt)})
}

// configureGoldMarketWS 在默认运行下设置无发布意义的测试数值；专门验收可载入工作目录中的正式投影文件。
func configureGoldMarketWS(t *testing.T, cfg *devconfig.Config) {
	t.Helper()
	if cfg.Fixture.GoldMarket != nil {
		t.Fatal("默认环境已经包含未审定的金币市场")
	}
	path := os.Getenv("MINIMIR_GOLD_MARKET_TEST_FIXTURE")
	if path == "" {
		cfg.Fixture.GoldMarket = &devconfig.GoldMarketConfig{GoldPerLot: 10_000, PriceStepYuanbaoUnits: 1,
			MaxBuyOrdersPerAccountRun: 5, MaxSellOrdersPerAccountRun: 5, TaxDivisor: 10, OrderExpirySeconds: 120,
			ReferencePriceYuanbaoUnits: 100, PriceWindowLots: 5_000, PriceLimitBP: 2_000}
		must(t, cfg.Fixture.Validate())
		return
	}
	work, err := filepath.Abs("../../../.work")
	must(t, err)
	actual, err := filepath.EvalSymlinks(path)
	must(t, err)
	relative, err := filepath.Rel(work, actual)
	must(t, err)
	if relative == "." || !filepath.IsLocal(relative) {
		t.Fatal("候选夹具必须来自本项目独立工作目录", actual)
	}
	raw, err := os.ReadFile(actual)
	must(t, err)
	var candidate devconfig.Fixture
	must(t, json.Unmarshal(raw, &candidate))
	checksum := sha256.Sum256(raw)
	candidate.Hash = hex.EncodeToString(checksum[:])
	must(t, candidate.Validate())
	if candidate.GoldMarket == nil || candidate.ItemMarket == nil {
		t.Fatal("隔离配置须启用金币市场并保留道具市场")
	}
	cfg.Fixture, cfg.FixtureFile = candidate, actual
	t.Logf("金币市场实际加载候选夹具 SHA-256=%s", candidate.Hash)
}

// seedMarketTaxWithItemTrade 先走真实道具直购，令同角色的本金税余数从 0 变成 9。
func seedMarketTaxWithItemTrade(t *testing.T, ctx context.Context, store *persistence.Store, seller, buyer *marketTestRole, item domain.Item) {
	t.Helper()
	listed := marketActionWS(seller.client, &pb.ItemMarketAction{Action: marketListAction, ItemInstanceId: domain.Bytes(item.ID), ItemVersion: 1,
		Quantity: 1, UnitPriceYuanbaoUnits: 9}, 0)
	result, err := store.DB.Collection("item_orders").UpdateOne(ctx, bson.M{"_id": domain.Hex(listed.OrderId), "revision": listed.OrderVersion},
		bson.M{"$set": bson.M{"auction_ends_at": time.Now().Add(2 * time.Second).UTC().Truncate(time.Millisecond)}})
	must(t, err)
	if result.ModifiedCount != 1 {
		t.Fatal("道具市场验收需缩短原隔离订单截止", listed)
	}
	view := marketOrderWS(buyer.client, listed.OrderId, persistence.ItemMarketDirect)
	debit := marketActionWS(buyer.client, termsForMarketAction(view, marketDirectBuyAction, 1), 0)
	if debit.YuanbaoDeltaUnits != -9 || debit.TaxYuanbaoUnits != 0 {
		t.Fatal("原道具实际成交未产生不足最小单位的卖方余数", debit)
	}
	proceeds := marketActionWS(seller.client, itemMarketClaimForTest(marketEscrowWS(seller.client, listed.OrderId, persistence.ItemMarketClaimProceeds)), 0)
	if proceeds.YuanbaoDeltaUnits != 9 {
		t.Fatal("实际道具净收与金币撮合前的共有余数不符", proceeds)
	}
}

// goldStateWS 读取当前场景的市场范围页，避免把异步市场消息误作当前查询结果。
func goldStateWS(c *client, scope uint32) *pb.GoldMarketState {
	c.t.Helper()
	request := c.send(p.MsgC2SGoldMarketQuery, &pb.GoldMarketQuery{SceneEpoch: c.entry.SceneEpoch, Scope: scope, PageSize: 20})
	frame := travelUntilWithin(c, 4*time.Second, func(f wire.Frame) bool { return f.ID == p.MsgS2CGoldMarketState && f.Request == request })
	state := &pb.GoldMarketState{}
	must(c.t, proto.Unmarshal(frame.Payload, state))
	if state.ErrorCode != 0 || state.SceneEpoch != c.entry.SceneEpoch || state.Scope != scope {
		c.t.Fatal("金币市场页未返回当前场景的请求身份", state)
	}
	return state
}

func goldTermsForTest(rules *pb.GoldMarketRulesView, side uint32, lots, price int64) *pb.GoldMarketAction {
	return &pb.GoldMarketAction{Action: goldTestPlace, Side: side, Lots: lots, UnitPriceYuanbaoUnits: price,
		TermsHash: rules.TermsHash, ExpectedDurationSeconds: rules.OrderDurationSeconds}
}

func goldClaimForTest(claim *pb.GoldMarketEscrowView) *pb.GoldMarketAction {
	return &pb.GoldMarketAction{Action: goldTestClaim, EscrowId: claim.EscrowId, EscrowVersion: claim.Version}
}

// goldActionWS 校验真实持久操作的回执与更新后的完整资产视图。
func goldActionWS(c *client, request *pb.GoldMarketAction, code uint32) *pb.GoldMarketReceipt {
	c.t.Helper()
	if request.Operation == nil {
		request.Operation = c.operation()
	}
	result := c.asset(p.MsgC2SGoldMarketAction, request)
	if result.ErrorCode != code || result.Pending || !bytes.Equal(result.OpId, request.Operation.OpId) ||
		(code == 0) != (result.GoldMarket != nil) {
		c.t.Fatalf("金币市场动作 %d 错误，预期错误码 %d：%v", request.Action, code, result)
	}
	if result.NewRevision > c.inventory.Revision {
		c.inventoryAt(result.NewRevision)
	}
	return result.GoldMarket
}

// goldDurableOrder 只读取原持久订单，等待后台真实撮合或入簿推进。
func goldDurableOrder(t *testing.T, ctx context.Context, store *persistence.Store, id []byte, ready func(persistence.GoldMarketOrder) bool) persistence.GoldMarketOrder {
	t.Helper()
	var order persistence.GoldMarketOrder
	for end := time.Now().Add(15 * time.Second); time.Now().Before(end); time.Sleep(100 * time.Millisecond) {
		must(t, store.DB.Collection("gold_orders").FindOne(ctx, bson.M{"_id": domain.Hex(id)}).Decode(&order))
		if ready(order) {
			return order
		}
	}
	t.Fatalf("后台金币委托未推进原订单：%+v", order)
	return order
}

func goldOwnOrderWS(c *client, id []byte) *pb.GoldMarketOrderView {
	c.t.Helper()
	for _, order := range goldStateWS(c, 3).Orders {
		if bytes.Equal(order.OrderId, id) && order.Owned {
			return order
		}
	}
	c.t.Fatalf("本人市场列表中缺少原订单 %x", id)
	return nil
}

// goldEscrowsWS 仅从原角色在同一场景的交易仓查找对应订单和币种原项目。
func goldEscrowsWS(c *client, id []byte, kind uint32, count int) []*pb.GoldMarketEscrowView {
	c.t.Helper()
	for end := time.Now().Add(15 * time.Second); time.Now().Before(end); time.Sleep(100 * time.Millisecond) {
		var matches []*pb.GoldMarketEscrowView
		for _, claim := range goldStateWS(c, 4).Escrows {
			if bytes.Equal(claim.OrderId, id) && claim.Kind == kind {
				matches = append(matches, claim)
			}
		}
		if len(matches) == count {
			return matches
		}
	}
	c.t.Fatalf("未观察到金币订单 %x 的 %d 条托管类型 %d", id, count, kind)
	return nil
}

// assertGoldMarketBalances 核对两类市场共同消费的税额、原金币总量和两个绑定钱包。
func assertGoldMarketBalances(t *testing.T, ctx context.Context, store *persistence.Store, roles []domain.Role) {
	t.Helper()
	for index, role := range roles {
		wantGold := []int64{40_000, 40_000, 40_000, 80_000, 50_000}[index]
		wantYuanbao := []int64{1_090, 1_082, 1_091, 708, 1_000}[index]
		if role.Wallet.Gold != wantGold || role.Wallet.Yuanbao != wantYuanbao || role.Wallet.BoundGold != 88_888 || role.Wallet.BoundYuanbao != 9_999 {
			t.Fatalf("两市场统一本金、佣金及绑定钱包不守恒，下标 %d：%+v", index, role.Wallet)
		}
	}
	var tax struct {
		Remainder int64 `bson:"remainder"` // 原卖家账号加永久入口的共同税余数。
	}
	must(t, store.DB.Collection("seller_tax_remainders").FindOne(ctx,
		bson.M{"account_id": roles[0].AccountID, "origin_server_id": roles[0].Origin}).Decode(&tax))
	if tax.Remainder != 2 {
		t.Fatalf("统一总成交额292的累计税余数为 %d", tax.Remainder)
	}
}
