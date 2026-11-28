// 本文件在隔离游戏服加载工作簿原五档锚点，实测边界等级报价、支付和重连重放。
package integration

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
	"google.golang.org/protobuf/proto"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
)

// workbookHostingReceipt 保存实际角色所见锚点、成交之前和之后的钱包及同一原事务。
type workbookHostingReceipt struct {
	Level         uint32                              `json:"level"`               // 实际角色等级，包含价格边界。
	QuoteAfterMap *pb.HostingQuoteResult              `json:"after_map,omitempty"` // 50 级角色换图和穿装后看到的参考锚点。
	Before        domain.Wallet                       `json:"before"`              // 购时前的四钱包。
	After         domain.Wallet                       `json:"after"`               // 购时及重放后的四钱包。
	Receipt       *persistence.HostingPurchaseReceipt `json:"receipt"`             // MongoDB 实际保存的原报价与到账。
}

// workbookHostingTrial 持有这一个隔离价格版本对应的实际账号、区服和资产数据库。
type workbookHostingTrial struct {
	ctx      context.Context    // 隔离游戏节点生命周期。
	store    *persistence.Store // 验证实际定价来源和永久资产事实。
	endpoint string             // 真实账号服 RPC。
	route    directoryTestEntry // 本轮独立入口服的正常签票路由。
}

// TestHostingPurchaseWorkbookBandsMongoWS 将候选表的九个边界样本一路核对到客户端和 MongoDB。
func TestHostingPurchaseWorkbookBandsMongoWS(t *testing.T) {
	file := os.Getenv("MINIMIR_HOSTING_PRICE_TEST_FIXTURE")
	if os.Getenv("MINIMIR_MONGO_E2E") != "1" || os.Getenv("MINIMIR_INTEGRATION_TEST_MONGO_URI") == "" || file == "" {
		t.Skip("五档工作簿报价需本轮隔离价表与项目专属副本集")
	}
	raw, err := os.ReadFile(file)
	must(t, err)
	var fixture devconfig.Fixture
	must(t, json.Unmarshal(raw, &fixture))
	must(t, fixture.Validate())
	fixture.Hash = persistence.Hash(raw)
	if len(fixture.HostingTimePrices) != 1 || fixture.HostingTimePrices[0].PurchaseStepSeconds != 30 {
		t.Fatal("本轮通过一次完整生效版本检查工作簿五档和 30 秒测试粒度")
	}
	version := fixture.HostingTimePrices[0]
	ctx, store, endpoint, route := startChallengePlayerDeathServer(t, func(cfg *devconfig.Config) { cfg.Fixture = fixture })
	trial := workbookHostingTrial{ctx: ctx, store: store, endpoint: endpoint, route: route}
	var receipts []workbookHostingReceipt
	for _, band := range version.Bands {
		receipts = append(receipts, verifyWorkbookHostingPrice(t, trial, version, band, band.LevelMin))
		if band.LevelMax != 0 {
			receipts = append(receipts, verifyWorkbookHostingPrice(t, trial, version, band, band.LevelMax))
		}
	}
	result, err := json.MarshalIndent(map[string]any{"fixture_sha256": fixture.Hash, "version": version.Version, "receipts": receipts}, "", "  ")
	must(t, err)
	must(t, os.WriteFile(filepath.Join(filepath.Dir(file), "hosting-price-receipts.json"), append(result, '\n'), 0o644))
	t.Logf("工作簿五档共 %d 个真实边界角色已付款，固定版本 %d", len(receipts), version.Version)
}

// verifyWorkbookHostingPrice 验证对应档位的版本、2 倍小时价、整笔取整、两钱包和唯一入账。
func verifyWorkbookHostingPrice(t *testing.T, trial workbookHostingTrial, version devconfig.HostingTimePriceVersion, band devconfig.HostingTimePriceBand, level uint32) workbookHostingReceipt {
	t.Helper()
	const startingSeconds = int64(123)
	hourly := 2 * band.GRefGoldPerHour
	cost := (hourly + 119) / 120 // 当前隔离粒度 30 秒，服务端须按整笔向上取整。
	bound := cost / 3
	remaining := min(int64(17), cost-1)
	opening := domain.Wallet{Gold: cost - bound + remaining, BoundGold: bound}
	login := &pb.LoginResult{}
	rpc(t, trial.endpoint, "", p.MsgC2SLocalRegister, &pb.LocalLoginRequest{Account: "hostband_" + domain.ID()[:8], Password: domain.ID()[:16]}, login, 0)
	role := createDirectoryRole(t, trial.endpoint, login.AccountToken, trial.route.origin.OriginID, "补时档位")
	roleID := domain.Hex(role.Key.RoleId)
	updated, err := trial.store.DB.Collection("roles").UpdateByID(trial.ctx, roleID, bson.M{"$set": bson.M{
		"level": level, "wallet": opening, "hosting_seconds": startingSeconds,
	}})
	must(t, err)
	if updated.ModifiedCount != 1 {
		t.Fatal("未保存隔离测试钱包与等级", level, opening)
	}
	attach := func() *client {
		key := directoryTicket(t, trial.endpoint, login.AccountToken, role.Key, trial.route)
		client := join(t, directoryTicketAddress(t, key), key.Ticket)
		client.auto(false, 1)
		return client
	}
	c := attach()
	defer c.close()
	discovery := hostingQuoteWS(c, 0)
	checkRate := func(quote *pb.HostingQuoteResult) {
		t.Helper()
		if quote.QuoteVersion != version.Version || quote.BandMinLevel != band.LevelMin || quote.BandMaxLevel != band.LevelMax ||
			quote.ReferenceGoldPerHour != band.GRefGoldPerHour || quote.GoldPerHour != hourly || quote.GranularitySeconds != version.PurchaseStepSeconds ||
			quote.RuleVersion != trial.store.Cfg.Fixture.RuleVersion || quote.ConfigHash != trial.store.Cfg.Fixture.Hash {
			t.Fatal("当前等级未获得工作簿原档位及版本价格", level, quote)
		}
	}
	checkRate(discovery)
	if discovery.ErrorCode != 0 || discovery.RequestedSeconds != 0 || len(discovery.QuoteId) != 0 {
		t.Fatal("只查看价格签发了资产凭据或提示失败", discovery)
	}
	var changed *pb.HostingQuoteResult
	if level == 50 {
		// 正常穿装和切换普通图，当前单价仍由已生效版本及等级决定。
		equipPersonalBossAutoFixture(t, c, &trial.store.Cfg.Fixture, 218)
		travelToBaseline(c, 2, 6)
		travelAcknowledge(c, false)
		changed = hostingQuoteWS(c, 0)
		checkRate(changed)
		if changed.ErrorCode != 0 || changed.SceneEpoch == discovery.SceneEpoch || changed.RoleRevision <= discovery.RoleRevision {
			t.Fatal("没有观察到换图、真实穿装和同一价格版本同时生效", changed)
		}
	}
	quote := hostingQuoteWS(c, version.PurchaseStepSeconds)
	checkRate(quote)
	if quote.ErrorCode != 0 || quote.TotalGold != cost || quote.BoundGoldCost != bound || quote.GoldCost != cost-bound ||
		quote.BeforeSeconds != startingSeconds || len(quote.QuoteId) != 16 || quote.SceneEpoch != c.entry.SceneEpoch || quote.RoleRevision != c.inventory.Revision {
		t.Fatal("当前五档真实报价拆分或整笔舍入不符", level, quote)
	}
	before, err := trial.store.Role(trial.ctx, roleID)
	must(t, err)
	if before.Wallet != opening || before.Level != level || before.HostingSeconds != startingSeconds {
		t.Fatal("只读报价改变了原角色资产", level, before.Wallet)
	}
	request := &pb.BuyHostingTimeRequest{RequestedSeconds: quote.RequestedSeconds, QuoteId: quote.QuoteId, Operation: c.operation()}
	accepted := roleCardAssetWS(c, p.MsgC2SBuyHostingTime, request)
	if accepted.ErrorCode != 0 || !proto.Equal(accepted.GetHostingPurchase().GetQuote(), quote) || accepted.GetHostingPurchase().GetBalanceSeconds() != startingSeconds+30 {
		t.Fatal("原固定价格未实际扣款或到账", level, accepted)
	}
	c.close()
	resumed := attach()
	defer resumed.close()
	if replay := resumed.asset(p.MsgC2SBuyHostingTime, request); replay.ErrorCode != 0 || !proto.Equal(replay.HostingPurchase, accepted.HostingPurchase) {
		t.Fatal("新连接未重放原固定价格成交", level, replay)
	}
	insufficient := hostingQuoteWS(resumed, version.PurchaseStepSeconds)
	checkRate(insufficient)
	if insufficient.ErrorCode != p.CodeHostingPurchaseFunds || len(insufficient.QuoteId) != 0 {
		t.Fatal("售后钱包资金不足仍签发补时报价", level, insufficient)
	}
	after, err := trial.store.Role(trial.ctx, roleID)
	must(t, err)
	if after.Wallet != (domain.Wallet{Gold: remaining}) || after.Level != level || after.HostingSeconds != startingSeconds+30 ||
		after.ClaimedSeconds != before.ClaimedSeconds || after.EconomicSeq != before.EconomicSeq+1 {
		t.Fatal("两钱包、日领或已保存的补时原请求不守恒", level, after.Wallet, after.HostingSeconds)
	}
	key := "asset:" + roleID + ":" + domain.Hex(request.Operation.OpId)
	var op persistence.Operation
	var audit struct {
		Purchase *persistence.HostingPurchaseReceipt `bson:"hosting_purchase"` // 同一资产事务写入的原审计。
	}
	must(t, trial.store.DB.Collection("operations").FindOne(trial.ctx, bson.M{"_id": key, "kind": persistence.AssetBuyHostingTime, "role_id": roleID}).Decode(&op))
	must(t, trial.store.DB.Collection("audit_outbox").FindOne(trial.ctx, bson.M{"_id": key}).Decode(&audit))
	if op.Code != 0 || !proto.Equal(op.HostingPurchase.Proto(), accepted.HostingPurchase) || !proto.Equal(audit.Purchase.Proto(), accepted.HostingPurchase) {
		t.Fatal("原补时、持久经济回执与审计无法对应", level, key)
	}
	return workbookHostingReceipt{Level: level, QuoteAfterMap: changed, Before: before.Wallet, After: after.Wallet, Receipt: op.HostingPurchase}
}
