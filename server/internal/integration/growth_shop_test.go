// 本文件用真实 MongoDB/WebSocket 核验成长商城的选择币种、角色限购、到账和重放。
package integration

import (
	"encoding/json"
	"errors"
	"os"
	"slices"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"google.golang.org/protobuf/proto"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
	"minimir/internal/wire"
)

// TestGrowthShopMongoWS 实买祝福油和书页各两个币种，核对限购、原单和原用油入口。
func TestGrowthShopMongoWS(t *testing.T) {
	trial := startGrowthShopTrial(t, true)
	balance := domain.Wallet{Gold: 5000, BoundGold: 50, Yuanbao: 1550, BoundYuanbao: 4200}
	player := createGrowthShopPlayer(t, trial, "商城联机", balance, false)
	roleID := domain.Hex(player.summary.Key.RoleId)
	c := trial.attach(t, &player, false)
	shop := queryGrowthShopTest(t, c, 0, 0)
	oilID, pageID := trial.store.Cfg.Fixture.LuckOil.ItemID, trial.store.Cfg.Fixture.SkillEconomy.PageItemID
	if len(shop.Offers) != 5 || shop.Offers[0].ItemId != oilID || shop.Offers[2].ItemId != pageID ||
		shop.Offers[2].PriceUnits != 50 || shop.Offers[3].PriceUnits != 100 || shop.Offers[4].Currency != 0 {
		t.Fatal("原工作簿的祝福油、书页或待定商品未投影到真实目录", shop)
	}
	expectGrowthShopQueryError(c, 5, 1, p.CodeGrowthShopUnavailable)
	expectGrowthShopQueryError(c, 1, 2, p.CodeVendorFunds)
	oldSkill := c.send(p.MsgC2SSkillAssetPreview, &pb.SkillAssetPreviewRequest{Kind: 1, OfferId: 3, Quantity: 1})
	travelExpectError(c, p.MsgC2SSkillAssetPreview, oldSkill, p.CodeSkillPurchaseBlocked)
	// 原技能书的合法报价继续由旧入口受理。
	book := c.send(p.MsgC2SSkillAssetPreview, &pb.SkillAssetPreviewRequest{Kind: 1, OfferId: 8, Quantity: 1})
	if frame := travelUntilWithin(c, 5*time.Second, func(f wire.Frame) bool { return f.Request == book }); frame.ID != p.MsgS2CSkillAssetPreview || frame.Error != 0 {
		t.Fatal("商城拾取原书页价格后，原合法技能书报价失效", frame.ID, frame.Error)
	}
	old := queryGrowthShopTest(t, c, 1, 1)
	bound := queryGrowthShopTest(t, c, 2, 2)
	expectGrowthShopCommit(t, c, growthShopPurchase(c, old), p.CodeGrowthShopQuoteChanged)
	changed := growthShopPurchase(c, bound)
	changed.Count = 1
	expectGrowthShopCommit(t, c, changed, p.CodeGrowthShopQuoteChanged)
	// 确认实际落库后丢弃原连接，不用响应决定是否已经成功扣款。
	request := growthShopPurchase(c, bound)
	c.send(p.MsgC2SGrowthShopCommit, request)
	stored := waitGrowthShopReceipt(t, trial, roleID, domain.Hex(request.Operation.OpId))
	binding := c.binding
	c.close()
	c = trial.attach(t, &player, false)
	replay := c.asset(p.MsgC2SGrowthShopCommit, request)
	if c.binding == binding || replay.ErrorCode != 0 || replay.GrowthShop == nil || !proto.Equal(replay.GrowthShop, stored.GrowthShop.Proto()) ||
		replay.GrowthShop.BalanceUnits != 200 || replay.GrowthShop.Terms.TotalPriceUnits != 4000 || !replay.GrowthShop.Items[0].Bound {
		t.Fatal("重连后未取回原绑定元宝交易，或原价格被重算", replay)
	}
	expectGrowthShopQueryError(c, 1, 2, p.CodeGrowthShopLimit)
	ordinary := expectGrowthShopCommit(t, c, growthShopPurchase(c, queryGrowthShopTest(t, c, 1, 1)), 0)
	if ordinary.GrowthShop.Terms.DailyBoughtBefore != 2 || ordinary.GrowthShop.BalanceUnits != 550 || !ordinary.GrowthShop.Items[0].Bound {
		t.Fatal("切换币种绕过共享限购或导致普通元宝产品未绑定", ordinary)
	}
	expectGrowthShopQueryError(c, 2, 1, p.CodeGrowthShopLimit)
	for _, buy := range []struct{ product, count uint32 }{{3, 3}, {4, 2}} {
		expectGrowthShopCommit(t, c, growthShopPurchase(c, queryGrowthShopTest(t, c, buy.product, buy.count)), 0)
	}
	read, err := trial.store.Role(trial.ctx, roleID)
	must(t, err)
	var oils, pages int64
	for _, item := range read.Items {
		if item.SourceKind != "growth_shop" {
			continue
		}
		def, _ := trial.store.Cfg.Fixture.Item(item.ItemID)
		if !item.Bound || persistence.CanTrade(item, def) || item.SourceID == "" {
			t.Fatal("持特权也必须按商城独立来源绑定，不能出售", item)
		}
		switch item.ItemID {
		case oilID:
			oils += item.Quantity
		case pageID:
			pages += item.Quantity
		}
	}
	if oils != 3 || pages != 5 || read.Wallet != (domain.Wallet{Gold: 5000, BoundGold: 50, Yuanbao: 400}) ||
		read.GrowthShop == nil || len(read.GrowthShop.Counts) != 2 {
		t.Fatal("原余额与四次实际绑定发货未守恒", read.Wallet, oils, pages, read.GrowthShop)
	}
	weaponIndex := slices.IndexFunc(c.inventory.Items, func(item *pb.ItemView) bool { return item.ItemId == 218 })
	if weaponIndex < 0 {
		t.Fatal("原角色缺少试用已购绑定油的木剑")
	}
	wood := c.inventory.Items[weaponIndex]
	ref := &pb.WeaponOilItemRef{InstanceId: wood.InstanceId, Version: wood.Version}
	use := weaponOilQuoteWS(c, ref, domain.Bytes(domain.ID()), true)
	if use.Oil.SourceKind != "growth_shop" || !use.OutputBound {
		t.Fatal("原武器成长未能自然选中购买的绑定油", use)
	}
	used := c.asset(p.MsgC2SWeaponOilCommit, weaponOilCommitWS(c, use, ref))
	if used.ErrorCode != 0 || !used.GetWeaponOil().GetSuccess() || used.GetWeaponOil().GetWeapon().GetCurrentLuck() != 1 ||
		!used.GetWeaponOil().GetWeapon().GetBound() {
		t.Fatal("商城油未沿原成长链消费并绑定实际武器", used)
	}
	count, err := trial.store.DB.Collection("audit_outbox").CountDocuments(trial.ctx,
		bson.M{"role_id": roleID, "kind": persistence.AssetGrowthShopCommit, "result_code": 0})
	must(t, err)
	if count != 4 {
		t.Fatal("原操作重放或异常报价增加了成功入账", count)
	}
	tight := createGrowthShopPlayer(t, trial, "商城满包", balance, true)
	blocked := trial.attach(t, &tight, false)
	expectGrowthShopQueryError(blocked, 1, 1, p.CodeEquipmentBagFull)
	full, err := trial.store.Role(trial.ctx, domain.Hex(tight.summary.Key.RoleId))
	must(t, err)
	if full.GrowthShop != nil || full.Wallet != balance || len(full.Items) != full.Capacity {
		t.Fatal("满包失败时写入了商城额度或扣款", full.GrowthShop, full.Wallet)
	}
	t.Log("两种币种实际买到 3 瓶绑定油和 5 张绑定书页，商城限量跨币种，原回执重放只有四笔成交，原用油使木剑升到幸运 +1")
}

// startGrowthShopTrial 在独占库中加载原隔离目录，默认包路径仍由旧配置提供。
func startGrowthShopTrial(t *testing.T, enabled bool) worldBossRewardTrial {
	t.Helper()
	ctx, store, endpoint, route := startChallengePlayerDeathServer(t, func(cfg *devconfig.Config) {
		if !enabled {
			return
		}
		path := os.Getenv("MINIMIR_GROWTH_SHOP_TEST_FIXTURE")
		raw, err := os.ReadFile(path)
		must(t, err)
		var f devconfig.Fixture
		must(t, json.Unmarshal(raw, &f))
		must(t, f.Validate())
		if f.GrowthShop == nil {
			t.Fatal("隔离配置缺少成长商城")
		}
		f.Hash = persistence.Hash(raw)
		cfg.Fixture, cfg.FixtureFile = f, path
	})
	return worldBossRewardTrial{ctx: ctx, store: store, endpoint: endpoint, route: route}
}

// createGrowthShopPlayer 只准备独立测试钱包与可选的满包容量，材料通过真实购买取得。
func createGrowthShopPlayer(t *testing.T, trial worldBossRewardTrial, name string, wallet domain.Wallet, full bool) worldBossRewardPlayer {
	t.Helper()
	player := trial.create(t, name, time.Now().Add(time.Hour), full)
	update, err := trial.store.DB.Collection("roles").UpdateByID(trial.ctx, domain.Hex(player.summary.Key.RoleId), bson.M{"$set": bson.M{"wallet": wallet}})
	must(t, err)
	if update.ModifiedCount != 1 {
		t.Fatal("隔离角色未建立商城测试钱包")
	}
	return player
}

// queryGrowthShopTest 只读原场景在售目录或具体报价。
func queryGrowthShopTest(t *testing.T, c *client, product, count uint32) *pb.GrowthShopState {
	t.Helper()
	id := c.send(p.MsgC2SGrowthShopQuery, &pb.GrowthShopQuery{SceneEpoch: c.entry.SceneEpoch, ProductId: product, Count: count})
	frame := travelUntilWithin(c, 5*time.Second, func(f wire.Frame) bool { return f.Request == id })
	if frame.Error != 0 || frame.ID != p.MsgS2CGrowthShopState {
		t.Fatal("原商城目录或报价请求被拒绝", frame.ID, frame.Error)
	}
	var state pb.GrowthShopState
	must(t, proto.Unmarshal(frame.Payload, &state))
	if state.SceneEpoch != c.entry.SceneEpoch || (state.Quote != nil) != (product != 0) {
		t.Fatal("商城返回了不同场景或无原确认单", &state)
	}
	return &state
}

// expectGrowthShopQueryError 观察指定商品在实际服务器上的报价拒绝。
func expectGrowthShopQueryError(c *client, product, count, code uint32) {
	c.t.Helper()
	id := c.send(p.MsgC2SGrowthShopQuery, &pb.GrowthShopQuery{SceneEpoch: c.entry.SceneEpoch, ProductId: product, Count: count})
	travelExpectError(c, p.MsgC2SGrowthShopQuery, id, code)
}

// growthShopPurchase 提交玩家所见的原科目、单位和份数。
func growthShopPurchase(c *client, state *pb.GrowthShopState) *pb.GrowthShopCommitRequest {
	op := c.operation()
	op.ExpectedRevision = state.RoleRevision
	return &pb.GrowthShopCommitRequest{QuoteId: state.Quote.QuoteId, ProductId: state.Quote.Terms.ProductId,
		Count: state.Quote.Terms.Count, Operation: op}
}

// expectGrowthShopCommit 对拍原返回回执及同期角色资产修订推送。
func expectGrowthShopCommit(t *testing.T, c *client, req *pb.GrowthShopCommitRequest, code uint32) *pb.OperationResult {
	t.Helper()
	out := c.asset(p.MsgC2SGrowthShopCommit, req)
	if out.ErrorCode != code || (out.GrowthShop != nil) != (code == 0) {
		t.Fatal("商城提交的实际结果与预期不一致", out)
	}
	if c.inventory.Revision < out.NewRevision {
		c.inventoryAt(out.NewRevision)
	}
	return out
}

// waitGrowthShopReceipt 等待 MongoDB 确认原商城事务，客户端尚未读取该次响应。
func waitGrowthShopReceipt(t *testing.T, trial worldBossRewardTrial, roleID, operationID string) persistence.Operation {
	t.Helper()
	deadline := time.Now().Add(7 * time.Second)
	var receipt persistence.Operation
	for {
		err := trial.store.DB.Collection("operations").FindOne(trial.ctx, bson.M{"_id": "asset:" + roleID + ":" + operationID}).Decode(&receipt)
		if !errors.Is(err, mongo.ErrNoDocuments) || !time.Now().Before(deadline) {
			must(t, err)
			break
		}
		time.Sleep(35 * time.Millisecond)
	}
	if receipt.Kind != persistence.AssetGrowthShopCommit || receipt.Code != 0 || receipt.GrowthShop == nil {
		t.Fatal("MongoDB 未生成成功的商城原回执", receipt)
	}
	return receipt
}
