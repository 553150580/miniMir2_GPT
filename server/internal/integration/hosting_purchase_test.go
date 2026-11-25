// 本文件通过真实 WebSocket 和隔离 MongoDB 验证金币补时、钱包扣款和重连重放。
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

// TestHostingPurchaseQuoteWalletAndReconnectMongoWS 仅在测试服启用报价，验证原始回执无法重复扣费。
func TestHostingPurchaseQuoteWalletAndReconnectMongoWS(t *testing.T) {
	const full = uint32(24 * 3600)
	const startingSeconds = int64(23*3600 - 15)
	ctx, store, endpoint, route := startChallengePlayerDeathServer(t, enableHostingTestPrices)
	login := &pb.LoginResult{}
	rpc(t, endpoint, "", p.MsgC2SLocalRegister, &pb.LocalLoginRequest{Account: "hostbuy_" + domain.ID()[:8], Password: "test_123456"}, login, 0)
	role := createDirectoryRole(t, endpoint, login.AccountToken, route.origin.OriginID, "托管补时")
	roleID := domain.Hex(role.Key.RoleId)
	// 仅为隔离报价指定原钱包和余量，实际扣费、加时和回执完全由游戏请求提交。
	seedWallet := domain.Wallet{BoundGold: 10, Gold: 100_000}
	seed, err := store.DB.Collection("roles").UpdateOne(ctx, bson.M{"_id": roleID}, bson.M{"$set": bson.M{
		"wallet": seedWallet, "hosting_seconds": startingSeconds,
	}})
	must(t, err)
	if seed.ModifiedCount != 1 {
		t.Fatal("测试角色原资产没有初始化")
	}
	attach := func() *client {
		t.Helper()
		ticket := directoryTicket(t, endpoint, login.AccountToken, role.Key, route)
		c := join(t, directoryTicketAddress(t, ticket), ticket.Ticket)
		c.auto(false, 1)
		return c
	}
	c := attach()
	defer func() {
		if c != nil {
			c.close()
		}
	}()
	before, err := store.Role(ctx, roleID)
	must(t, err)
	if before.Wallet != seedWallet || before.HostingSeconds != startingSeconds {
		t.Fatal("初始余额与报价场景不一致", before.Wallet, before.HostingSeconds)
	}
	if quote := hostingQuoteWS(c, 0); quote.QuoteVersion != 71 || quote.BandMaxLevel != 19 || quote.GranularitySeconds != 15 ||
		quote.GoldPerHour != 3602 || quote.RequestedSeconds != 0 || len(quote.QuoteId) != 0 || quote.ErrorCode != 0 {
		t.Fatalf("零秒未返回只读价格档位：%v", quote)
	}
	for _, invalid := range []uint32{13, full + 15} {
		if quote := hostingQuoteWS(c, invalid); quote.ErrorCode != p.CodeInvalidArgument || len(quote.QuoteId) != 0 {
			t.Fatalf("无效秒数取得了报价：%d %v", invalid, quote)
		}
	}
	staleScene := c.send(p.MsgC2SHostingQuote, &pb.HostingQuoteRequest{SceneEpoch: c.entry.SceneEpoch ^ 1, RequestedSeconds: 15})
	travelExpectError(c, p.MsgC2SHostingQuote, staleScene, p.CodeInvalidArgument)

	forged := &pb.BuyHostingTimeRequest{RequestedSeconds: 15, QuoteId: domain.Bytes(domain.ID()), Operation: c.operation()}
	if result := c.asset(p.MsgC2SBuyHostingTime, forged); result.ErrorCode != p.CodeHostingPurchaseQuoteStale || result.HostingPurchase != nil {
		t.Fatal("伪造报价未被拒绝", result)
	} else {
		c.inventoryAt(result.NewRevision)
	}
	quote := hostingQuoteWS(c, 15)
	if quote.ErrorCode != 0 || len(quote.QuoteId) != 16 || quote.RoleRevision != c.inventory.Revision || quote.SceneEpoch != c.entry.SceneEpoch ||
		quote.TotalGold != 16 || quote.BoundGoldCost != 10 || quote.GoldCost != 6 || quote.BeforeSeconds != startingSeconds {
		t.Fatal("报价取整或钱包拆分不一致", quote)
	}
	first := &pb.BuyHostingTimeRequest{RequestedSeconds: 15, QuoteId: quote.QuoteId, Operation: c.operation()}
	firstResult := c.asset(p.MsgC2SBuyHostingTime, first)
	if firstResult.ErrorCode != 0 || !proto.Equal(firstResult.GetHostingPurchase().GetQuote(), quote) || firstResult.GetHostingPurchase().GetBalanceSeconds() != 23*3600 {
		t.Fatal("首笔回执与原确认页不一致", firstResult)
	}
	c.inventoryAt(firstResult.NewRevision)
	longQuote := hostingQuoteWS(c, full)
	last := &pb.BuyHostingTimeRequest{RequestedSeconds: full, QuoteId: longQuote.QuoteId, Operation: c.operation()}
	lastResult := c.asset(p.MsgC2SBuyHostingTime, last)
	if lastResult.ErrorCode != 0 || !proto.Equal(lastResult.GetHostingPurchase().GetQuote(), longQuote) ||
		longQuote.TotalGold != 86_448 || lastResult.GetHostingPurchase().GetBalanceSeconds() != 47*3600 {
		t.Fatal("23小时余额未能再购24小时", lastResult)
	}
	c.inventoryAt(lastResult.NewRevision)
	if blocked := hostingQuoteWS(c, 15); blocked.ErrorCode != p.CodeHostingPurchaseBalance || len(blocked.QuoteId) != 0 {
		t.Fatal("超过购买前余额门槛后仍取得报价", blocked)
	}

	// 健康重连清除临时报价，原操作只从 MongoDB 恢复原扣款与到账快照。
	c.close()
	c = nil
	resumed := attach()
	defer resumed.close()
	replay := resumed.asset(p.MsgC2SBuyHostingTime, first)
	if replay.ErrorCode != 0 || !proto.Equal(replay.HostingPurchase, firstResult.HostingPurchase) {
		t.Fatal("新连接没有返回原操作成交回执", replay)
	}
	mutated := proto.Clone(first).(*pb.BuyHostingTimeRequest)
	mutated.RequestedSeconds *= 2
	if replay := resumed.asset(p.MsgC2SBuyHostingTime, mutated); replay.ErrorCode != p.CodeOperationConflict || replay.HostingPurchase != nil {
		t.Fatal("相同操作编号携带变化的请求未被拒绝", replay)
	}
	final, err := store.Role(ctx, roleID)
	must(t, err)
	if final.HostingSeconds != 47*3600 || final.Wallet.BoundGold != 0 || final.Wallet.Gold != 13_546 ||
		final.EconomicSeq != before.EconomicSeq+3 || final.SourceSeq != before.SourceSeq || final.ClaimedSeconds != before.ClaimedSeconds {
		t.Fatalf("重放改变了资产、日领或来源水位：余额=%d 金币=%+v 经济=%d 来源=%d 已领取=%d",
			final.HostingSeconds, final.Wallet, final.EconomicSeq, final.SourceSeq, final.ClaimedSeconds)
	}
	var audit struct {
		Purchase *persistence.HostingPurchaseReceipt `bson:"hosting_purchase"` // 随角色事务写入的原报价、钱包拆分与到账。
	}
	must(t, store.DB.Collection("audit_outbox").FindOne(ctx, bson.M{"_id": "asset:" + roleID + ":" + domain.Hex(first.Operation.OpId)}).Decode(&audit))
	if !proto.Equal(audit.Purchase.Proto(), firstResult.HostingPurchase) {
		t.Fatal("永久审计与原成交回执不一致", audit.Purchase)
	}
}

// enableHostingTestPrices 只为独立测试提供可复现的取整和钱包输入。
func enableHostingTestPrices(cfg *devconfig.Config) {
	cfg.Fixture.HostingTimePrices = []devconfig.HostingTimePriceVersion{{
		Version: 71, EffectiveFrom: time.Now().UTC().Add(-time.Hour).Truncate(time.Second), PurchaseStepSeconds: 15,
		Bands: []devconfig.HostingTimePriceBand{
			{LevelMin: 1, LevelMax: 19, GRefGoldPerHour: 1801}, {LevelMin: 20, LevelMax: 29, GRefGoldPerHour: 2001},
			{LevelMin: 30, LevelMax: 39, GRefGoldPerHour: 2201}, {LevelMin: 40, LevelMax: 49, GRefGoldPerHour: 2401},
			{LevelMin: 50, GRefGoldPerHour: 2601},
		},
	}}
}

// TestHostingPurchaseDefaultClosedMongoWS 验证试玩默认配置不会开放未审定价格。
func TestHostingPurchaseDefaultClosedMongoWS(t *testing.T) {
	_, _, endpoint, route := startChallengePlayerDeathServer(t, func(*devconfig.Config) {})
	c, _, _ := joinWorldBossTestRole(t, endpoint, route)
	t.Cleanup(c.close)
	if quote := hostingQuoteWS(c, 0); quote.ErrorCode != p.CodeHostingPurchaseUnavailable || quote.QuoteVersion != 0 || len(quote.QuoteId) != 0 {
		t.Fatal("默认运行配置提供了未经审定的价格", quote)
	}
}

// hostingQuoteWS 读取指定场景和请求编号下的只读报价结果。
func hostingQuoteWS(c *client, seconds uint32) *pb.HostingQuoteResult {
	c.t.Helper()
	request := c.send(p.MsgC2SHostingQuote, &pb.HostingQuoteRequest{SceneEpoch: c.entry.SceneEpoch, RequestedSeconds: seconds})
	frame := travelUntilWithin(c, 5*time.Second, func(f wire.Frame) bool {
		return f.ID == p.MsgS2CHostingQuote && f.Request == request
	})
	quote := &pb.HostingQuoteResult{}
	must(c.t, proto.Unmarshal(frame.Payload, quote))
	return quote
}
