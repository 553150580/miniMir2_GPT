//go:build creator_mining

// 本文件以独立 Creator 实包、真实 HTTP/WebSocket 和 MongoDB 验证矿区触屏。
package integration

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
)

// TestMiningBrowserMongoWS 验证矿区采集、主动回收设置和真实城镇 NPC 的收购账本。
func TestMiningBrowserMongoWS(t *testing.T) {
	webRoot := os.Getenv("MINIMIR_MINING_BROWSER_WEB_ROOT")
	mode := os.Getenv("MINIMIR_MINING_BROWSER_MODE")
	if webRoot == "" || os.Getenv("MINIMIR_MONGO_E2E") != "1" || os.Getenv("MINIMIR_INTEGRATION_TEST_MONGO_URI") == "" {
		t.Skip("触屏挖矿需要独立 Creator Web Mobile 与隔离 MongoDB")
	}
	if mode != "mining" && mode != "default" {
		t.Fatal("浏览器验收需要明确独立矿区包或默认关闭包")
	}
	repo, err := filepath.Abs("../../..")
	must(t, err)
	work := filepath.Join(repo, ".work")
	build := miningBrowserInput(t, work, webRoot)
	bundle, err := os.ReadFile(filepath.Join(build, "assets", "main", "index.js"))
	must(t, err)
	bundleDigest := sha256.Sum256(bundle)
	fixtureFile := filepath.Join(repo, "config", "development", "first-loop.json")
	enabled := mode == "mining"
	if enabled {
		fixtureFile = miningBrowserInput(t, work, os.Getenv("MINIMIR_MINING_BROWSER_FIXTURE"))
	}
	fixtureBytes, err := os.ReadFile(fixtureFile)
	must(t, err)
	var fixture devconfig.Fixture
	must(t, json.Unmarshal(fixtureBytes, &fixture))
	fixtureDigest := sha256.Sum256(fixtureBytes)
	fixture.Hash = hex.EncodeToString(fixtureDigest[:])
	must(t, fixture.Validate())
	if (fixture.Mining != nil) != enabled {
		t.Fatal("隔离与默认夹具的矿区开关与预期不同")
	}

	ctx, store, account, route := startChallengePlayerDeathServer(t, func(cfg *devconfig.Config) {
		cfg.Fixture, cfg.FixtureFile = fixture, fixtureFile
	})
	login := &pb.LoginResult{}
	name, password := "minetouch_"+domain.ID()[:9], domain.ID()[:16]
	rpc(t, account, "", p.MsgC2SLocalRegister, &pb.LocalLoginRequest{Account: name, Password: password}, login, 0)
	role := createDirectoryRole(t, account, login.AccountToken, route.origin.OriginID, "触屏矿工")
	roleID := domain.Hex(role.Key.RoleId)
	var tool domain.Item
	var shop *devconfig.NPCShop
	var town *devconfig.MapTemplate
	var vendor devconfig.TownNPC
	var fallback, unbound domain.Item
	metadata := map[string]any{
		"enabled": enabled, "normalMap": fixture.Map.ID, "ruleVersion": fixture.RuleVersion, "miner": role.Name,
		"region": route.origin.RegionName, "server": route.origin.Name, "endpoint": route.assignment.GameWSURL,
		"fixtureSha256": fixture.Hash, "clientBundleSha256": hex.EncodeToString(bundleDigest[:]),
	}
	if enabled {
		first, deep := fixture.Mining.Layers[0], fixture.Mining.Layers[1]
		definition := fixture.Mining.Tools[0]
		durability := definition.DurabilityU
		if recycle := fixture.Mining.AutoRecycle; recycle != nil {
			// 触屏需要先后两段真实随机出矿，给验收原件预置足量耐久，商品售价和工作簿规则不变。
			durability = max(durability, 32*fixture.Mining.ToolWearU)
			names := make([]string, 0, len(recycle.ItemIDs))
			for _, id := range recycle.ItemIDs {
				item, exists := fixture.Item(id)
				if !exists {
					t.Fatal("矿石自动回收缺少公开矿种", id)
				}
				names = append(names, item.Name)
			}
			metadata["miningRecycle"] = map[string]any{"itemIds": recycle.ItemIDs, "names": names}
		}
		tool = domain.Item{ID: domain.ID(), ItemID: definition.ItemID, Quantity: 1, Version: 1,
			DurabilityU: durability, MaxDurabilityU: durability, OriginalDurabilityU: durability,
			SourceKind: "npc_shop", SourceID: domain.ID()}
		town, vendor, _ = miningVendor(t, &fixture, tool.ItemID)
		shop = fixture.NPCShopByRole(vendor.Role)
		fallback, unbound = miningBrowserSampleOres(t, &fixture, shop)
		// 买锄由联机专项验收。两件铜矿仅为绑定分账和舍入边界的隔离前态。
		change, err := store.DB.Collection("roles").UpdateByID(ctx, roleID, bson.M{
			"$set": bson.M{"level": first.MinimumLevel}, "$push": bson.M{"items": bson.M{"$each": []domain.Item{tool, fallback, unbound}}},
		})
		must(t, err)
		if change.ModifiedCount != 1 {
			t.Fatal("没有准备隔离触屏角色工具")
		}
		metadata["mineMap"], metadata["deepMap"], metadata["toolId"], metadata["toolInstance"] = first.Map.ID, deep.Map.ID, tool.ItemID, tool.ID
		metadata["toolFull"], metadata["toolWear"], metadata["minLevel"], metadata["deepLevel"] = tool.DurabilityU, fixture.Mining.ToolWearU, first.MinimumLevel, deep.MinimumLevel
		metadata["town"] = map[string]any{"map": town.ID, "npc": vendor.ID, "name": vendor.Name, "rates": shop.MineralBuyback,
			"approach": miningBrowserApproach(t, town, vendor), "fallback": miningBrowserOreView(fallback), "unbound": miningBrowserOreView(unbound)}
	}
	before, err := store.Role(ctx, roleID)
	must(t, err)
	upstream, err := url.Parse(account)
	must(t, err)
	upstream.Path = ""
	mux := http.NewServeMux()
	mux.Handle("/account/rpc", http.StripPrefix("/account", httputil.NewSingleHostReverseProxy(upstream)))
	mux.Handle("/", http.FileServer(http.Dir(build)))
	web := httptest.NewServer(mux)
	defer web.Close()
	metadata["baseUrl"] = web.URL
	evidence, err := os.MkdirTemp(work, "mining-browser-")
	must(t, err)
	manifest := filepath.Join(evidence, "manifest.json")
	raw, err := json.Marshal(metadata)
	must(t, err)
	must(t, os.WriteFile(manifest, raw, 0600))
	t.Log("挖矿触屏验收目录 " + evidence)
	browserCtx, stop := context.WithTimeout(ctx, 200*time.Second)
	defer stop()
	node := exec.CommandContext(browserCtx, "node", filepath.Join(repo, "tools/client/verify_mining_browser.cjs"), manifest)
	node.Dir = repo
	node.Env = append(os.Environ(), "MINIMIR_MINING_BROWSER_ACCOUNT="+name, "MINIMIR_MINING_BROWSER_PASSWORD="+password)
	log, err := node.CombinedOutput()
	if err != nil {
		t.Fatalf("矿区触屏未完成：%v\n%s", err, log)
	}
	t.Logf("%s", log)
	if enabled {
		verifyMiningBrowserAssets(t, ctx, store, evidence, before, tool, &fixture, shop, town, vendor, fallback, unbound)
	}
}

// miningBrowserSampleOres 为绑定矿保留原样本回退，并单独验证奇数纯度非绑定铜矿。
func miningBrowserSampleOres(t *testing.T, fixture *devconfig.Fixture, shop *devconfig.NPCShop) (domain.Item, domain.Item) {
	t.Helper()
	for _, ore := range fixture.Mining.Ores {
		if ore.Key != "copper" {
			continue
		}
		rate, ok := shop.MineralBuybackFor(ore.ItemID)
		if !ok || rate.Rounding != "floor_per_ore" || rate.Numerator != 1 || rate.Denominator != 2 {
			t.Fatal("隔离浏览器缺少逐块取整的铜矿样本规则")
		}
		makeOre := func(purity uint32, bound bool) domain.Item {
			return domain.Item{ID: domain.ID(), ItemID: ore.ItemID, Quantity: 1, Version: 1, OrePurity: purity,
				Bound: bound, SourceKind: "mining", SourceID: domain.ID()}
		}
		return makeOre(11, true), makeOre(13, false)
	}
	t.Fatal("隔离浏览器缺少铜矿模板")
	return domain.Item{}, domain.Item{}
}

// miningBrowserApproach 提供从真实城镇地图得出的八个相邻格中的可通行候选。
func miningBrowserApproach(t *testing.T, town *devconfig.MapTemplate, npc devconfig.TownNPC) []devconfig.Point {
	t.Helper()
	var points []devconfig.Point
	for _, offset := range []devconfig.Point{{X: 1}, {Y: 1}, {X: -1}, {Y: -1}, {X: 1, Y: 1}, {X: -1, Y: 1}, {X: 1, Y: -1}, {X: -1, Y: -1}} {
		point := devconfig.Point{X: npc.Position.X + offset.X, Y: npc.Position.Y + offset.Y}
		if town.Walkable(point) {
			points = append(points, point)
		}
	}
	if len(points) == 0 {
		t.Fatal("隔离商人缺少可步行靠近的邻格")
	}
	return points
}

// miningBrowserOreView 只传递触屏定位原件所需的公开身份和属性。
func miningBrowserOreView(item domain.Item) map[string]any {
	return map[string]any{"item": item.ItemID, "instance": item.ID, "purity": item.OrePurity, "bound": item.Bound}
}

// miningBrowserInput 拒绝把主试玩静态目录或外部资料作为触屏验收目标。
func miningBrowserInput(t *testing.T, work, supplied string) string {
	t.Helper()
	resolved, err := filepath.EvalSymlinks(supplied)
	must(t, err)
	resolved, err = filepath.Abs(resolved)
	must(t, err)
	relative, err := filepath.Rel(work, resolved)
	must(t, err)
	if relative == "." || !filepath.IsLocal(relative) {
		t.Fatal("只能使用本轮独立触屏输入")
	}
	return resolved
}

// verifyMiningBrowserAssets 核对原工具、实际出矿来源、收购回执和两个钱包的持久增量。
func verifyMiningBrowserAssets(t *testing.T, ctx context.Context, store *persistence.Store, evidence string, before domain.Role, tool domain.Item,
	fixture *devconfig.Fixture, shop *devconfig.NPCShop, town *devconfig.MapTemplate, vendor devconfig.TownNPC, fallback, unbound domain.Item) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(evidence, "mining-browser-report.json"))
	must(t, err)
	var report struct {
		Stopped struct {
			Attempts int64 `json:"attempts"`
		} `json:"stopped"`
		Ore  miningBrowserOre   `json:"ore"`
		Auto *miningBrowserAuto `json:"auto"`
		Town struct {
			Approach struct {
				Target devconfig.Point `json:"target"`
			} `json:"approach"`
		} `json:"town"`
		Sales []miningBrowserSale `json:"sales"`
	}
	must(t, json.Unmarshal(raw, &report))
	after, err := store.Role(ctx, before.ID)
	must(t, err)
	if (report.Auto != nil) != (fixture.Mining.AutoRecycle != nil) {
		t.Fatal("矿石自动回收配置与触屏实际路径不一致")
	}
	attempts, commands := report.Stopped.Attempts, int64(2)
	if report.Auto != nil {
		attempts = report.Auto.Stopped.Attempts
		if attempts <= report.Stopped.Attempts {
			t.Fatal("开启回收后未新增开采轮次")
		}
		commands += 2
	}
	wear := fixture.Mining.ToolWearU
	toolIndex := slices.IndexFunc(after.Items, func(item domain.Item) bool { return item.ID == tool.ID })
	if report.Stopped.Attempts < 1 || toolIndex < 0 || after.Items[toolIndex].DurabilityU != tool.DurabilityU-attempts*wear ||
		after.Items[toolIndex].Version != tool.Version+attempts || len(report.Sales) != 2 || after.EconomicSeq != before.EconomicSeq+commands {
		var durability int64 = -1
		if toolIndex >= 0 {
			durability = after.Items[toolIndex].DurabilityU
		}
		t.Fatalf("锄头和收购水位不符：判定 %d 次，锄头耐久 %d→%d，出售 %d 笔，经济水位 %d→%d",
			attempts, tool.DurabilityU, durability, len(report.Sales), before.EconomicSeq, after.EconomicSeq)
	}
	sources := verifyMiningBrowserSources(t, ctx, store, before, after, tool, wear, attempts, town, report.Town.Approach.Target)
	firstResult := slices.IndexFunc(sources[:report.Stopped.Attempts], func(source persistence.Operation) bool {
		return source.Mining.Ore != nil && source.Mining.Ore.ID == report.Ore.Instance
	})
	if firstResult < 0 || !report.Ore.matches(*sources[firstResult].Mining.Ore) || !report.Ore.Bound || sources[firstResult].Mining.AutoRecycle != nil {
		t.Fatal("界面实际留存的矿石和原来源回执不一致", report.Ore)
	}
	mined := *sources[firstResult].Mining.Ore
	for _, source := range sources[:report.Stopped.Attempts] {
		if source.Mining.AutoRecycle != nil {
			t.Fatal("未开启矿石自动回收时提前收走原矿", source.SourceSeq)
		}
	}
	rate, configured := shop.MineralBuybackFor(mined.ItemID)
	naturalSale := configured && int64(mined.OrePurity)*int64(rate.Numerator)/int64(rate.Denominator) > 0
	bound := mined
	if !naturalSale {
		bound = fallback
	}
	unsold := fallback
	if !naturalSale {
		unsold = mined
	}
	if !slices.Contains(after.Items, unsold) {
		t.Fatal("本次未选中的原矿实例发生变化", unsold)
	}
	if report.Sales[0].Supplied == naturalSale || !report.Sales[1].Supplied {
		t.Fatal("触屏验收没有正确区分实际出矿与隔离前态")
	}
	boundPrice := verifyMiningBrowserSale(t, ctx, store, after, report.Sales[0], bound, shop, vendor)
	unboundPrice := verifyMiningBrowserSale(t, ctx, store, after, report.Sales[1], unbound, shop, vendor)
	wantWallet := before.Wallet
	if report.Auto != nil {
		wantWallet = verifyMiningBrowserAuto(t, ctx, store, fixture, before, after, sources[report.Stopped.Attempts:], report.Auto, mined, fallback, unbound)
	}
	wantWallet.BoundGold += boundPrice
	wantWallet.Gold += unboundPrice
	if after.Wallet != wantWallet {
		t.Fatal("两笔收购的钱包增量与纯度计算不一致", after.Wallet, wantWallet)
	}
}

// verifyMiningBrowserSources 按连续来源回执分别查证每轮开采和城镇步行触发的安全区绑定。
func verifyMiningBrowserSources(t *testing.T, ctx context.Context, store *persistence.Store, before, after domain.Role, tool domain.Item,
	wear, attempts int64, town *devconfig.MapTemplate, target devconfig.Point) []persistence.Operation {
	t.Helper()
	cursor, err := store.DB.Collection("operations").Find(ctx, bson.M{"role_id": before.ID, "source_seq": bson.M{"$gt": before.SourceSeq}},
		options.Find().SetSort(bson.D{{Key: "source_seq", Value: 1}}))
	must(t, err)
	var sources []persistence.Operation
	must(t, cursor.All(ctx, &sources))
	if int64(len(sources)) != attempts+1 || after.SourceSeq != before.SourceSeq+attempts+1 {
		t.Fatalf("来源应包含 %d 次采矿和一次步行绑定，实际 %d 份回执，水位 %d→%d", attempts, len(sources), before.SourceSeq, after.SourceSeq)
	}
	for i, source := range sources[:len(sources)-1] {
		previousDurability := tool.DurabilityU - int64(i)*wear
		if source.Code != 0 || source.SourceSeq != before.SourceSeq+int64(i)+1 || source.Kind != "mining_attempt" || source.Mining == nil ||
			source.Mining.ToolID != tool.ID || source.Mining.DurabilityBeforeU != previousDurability || source.Mining.DurabilityAfterU != previousDurability-wear {
			t.Fatal("矿轮缺少原工具的连续耐久回执", source.SourceSeq, source.Kind, source.Mining)
		}
	}
	zone, inside := town.SafeZoneAt(target)
	if !inside {
		t.Fatal("隔离商人邻格不在候选城镇安全区内", target)
	}
	walk := sources[len(sources)-1]
	point := domain.RoleBinding{MapID: town.ID, X: int32(zone.Center.X), Y: int32(zone.Center.Y)}
	if walk.Code != 0 || walk.Kind != "safe_zone_binding" || walk.SourceSeq != after.SourceSeq || walk.Binding == nil ||
		walk.Binding.Action != "walk" || walk.Binding.Point != point || walk.Binding.ZoneID != zone.ID ||
		after.Binding == nil || *after.Binding != point || after.BindingSourceSeq != walk.SourceSeq {
		t.Fatal("触屏步行的原安全区绑定回执或来源水位不符", walk.Binding, after.Binding)
	}
	return sources[:len(sources)-1]
}

// miningBrowserOre 保留浏览器通过真实 WebSocket 读到的单块矿石身份和属性。
type miningBrowserOre struct {
	Item     uint32 `json:"item"`     // 矿石模板编号。
	Instance string `json:"instance"` // 单块矿石实例编号。
	Purity   uint32 `json:"purity"`   // 该次出矿的纯度。
	Bound    bool   `json:"bound"`    // 该次出矿的绑定状态。
}

// matches 核对当前观察与持久回执中同一原矿的身份。
func (o miningBrowserOre) matches(item domain.Item) bool {
	return o.Item == item.ItemID && o.Instance == item.ID && o.Purity == item.OrePurity && o.Bound == item.Bound
}

// miningBrowserWallet 只接受浏览器用十进制字符串记录的四钱包整数。
type miningBrowserWallet struct {
	Gold       int64 `json:"gold,string"`       // 不绑定的金币。
	BoundGold  int64 `json:"boundGold,string"`  // 绑定金币。
	Ingot      int64 `json:"ingot,string"`      // 不绑定的元宝存储单位。
	BoundIngot int64 `json:"boundIngot,string"` // 绑定元宝存储单位。
}

// funds 将触屏样本转换回持久钱包结构以逐栏核对。
func (w miningBrowserWallet) funds() domain.Wallet {
	return domain.Wallet{Gold: w.Gold, BoundGold: w.BoundGold, Yuanbao: w.Ingot, BoundYuanbao: w.BoundIngot}
}

// miningBrowserAuto 记录用户同角色设置回收以及观察到的实际每笔到账。
type miningBrowserAuto struct {
	Canceled      bool     `json:"canceled"`      // 曾取消开启预览且不发操作。
	Enabled       string   `json:"enabled"`       // 首次确认开启的原操作编号。
	Disabled      string   `json:"disabled"`      // 用户确认关闭的原操作编号。
	KeptOriginals []string `json:"keptOriginals"` // 自动回收前就在背包、随后仍在的实例。
	Stopped       struct {
		Attempts int64 `json:"attempts"` // 两段采矿合计的判定轮次。
	} `json:"stopped"`
	Receipts []struct {
		Ore       miningBrowserOre `json:"ore"`              // 本笔回收的原矿。
		Gold      int64            `json:"gold,string"`      // 实收金币。
		BoundGold int64            `json:"boundGold,string"` // 实收绑定金币。
	} `json:"receipts"`
	WalletBefore miningBrowserWallet `json:"walletBefore"` // 开启前的四钱包。
	WalletAfter  miningBrowserWallet `json:"walletAfter"`  // 停止自动回收后的四钱包。
}

// verifyMiningBrowserAuto 对拍两份玩家策略回执以及自动回收轮次的原出矿、到账和保留物品。
func verifyMiningBrowserAuto(t *testing.T, ctx context.Context, store *persistence.Store, fixture *devconfig.Fixture,
	before, after domain.Role, sources []persistence.Operation, auto *miningBrowserAuto, mined, fallback, unbound domain.Item) domain.Wallet {
	t.Helper()
	if !auto.Canceled || before.MiningAutoRecycle || after.MiningAutoRecycle || auto.Enabled == "" || auto.Disabled == "" || auto.Enabled == auto.Disabled ||
		!slices.Equal(auto.KeptOriginals, []string{mined.ID, fallback.ID, unbound.ID}) {
		t.Fatal("实际开启、关闭或保留既有矿石的触屏记录不一致", auto)
	}
	for index, id := range []string{auto.Enabled, auto.Disabled} {
		var op persistence.Operation
		must(t, store.DB.Collection("operations").FindOne(ctx, bson.M{"_id": "asset:" + before.ID + ":" + id}).Decode(&op))
		if op.Code != 0 || op.Kind != persistence.AssetSetMiningAutoRecycle || op.CommandSeq != before.EconomicSeq+int64(index)+1 ||
			op.MiningAutoRecycle == nil || *op.MiningAutoRecycle != (index == 0) {
			t.Fatal("开关原经济回执与真实角色保存不一致", id, op)
		}
	}
	wallet := before.Wallet
	observed := 0
	for _, source := range sources {
		result := source.Mining
		if result.Ore == nil {
			if result.AutoRecycle != nil {
				t.Fatal("没有出矿的轮次不能发回收金币", source.SourceSeq)
			}
			continue
		}
		ore := *result.Ore
		price, allowed := fixture.MiningAutoRecyclePrice(ore.ItemID)
		amount := int64(0)
		if allowed {
			amount = int64(ore.OrePurity) * int64(price.Numerator) / int64(price.Denominator)
		}
		if amount == 0 {
			if result.AutoRecycle != nil || !slices.Contains(after.Items, ore) {
				t.Fatal("不收购或零价的原矿没有保留", source.SourceSeq, ore)
			}
			continue
		}
		want := domain.Wallet{Gold: amount}
		if ore.Bound {
			want = domain.Wallet{BoundGold: amount}
		}
		if observed >= len(auto.Receipts) || result.AutoRecycle == nil || *result.AutoRecycle != want || slices.Contains(after.Items, ore) {
			t.Fatal("候选回收的原矿与持久到账不一致", source.SourceSeq, result)
		}
		record := auto.Receipts[observed]
		if !record.Ore.matches(ore) || record.Gold != want.Gold || record.BoundGold != want.BoundGold {
			t.Fatal("矿石回收的触屏到账与同一原轮次不一致", source.SourceSeq, record)
		}
		observed++
		wallet.Gold += want.Gold
		wallet.BoundGold += want.BoundGold
	}
	if observed == 0 || observed != len(auto.Receipts) || auto.WalletBefore.funds() != before.Wallet || auto.WalletAfter.funds() != wallet {
		t.Fatal("回收不应重记矿石或增加其他钱包", observed, auto.WalletBefore, auto.WalletAfter, wallet)
	}
	return wallet
}

// miningBrowserSale 是一次触屏报价和确认后记录的原实例及操作身份。
type miningBrowserSale struct {
	Item     uint32 `json:"item"`         // 原矿物品模板编号。
	Instance string `json:"instance"`     // 本次选中且实际出售的背包实例。
	Purity   uint32 `json:"purity"`       // 报价窗口展示的纯度。
	Bound    bool   `json:"bound"`        // 对应原矿和入账钱包的绑定态。
	Supplied bool   `json:"supplied"`     // 此矿来自明确的隔离前态。
	Price    int64  `json:"price,string"` // 界面通过 64 位字符串处理的金币金额。
	Quote    string `json:"quote"`        // 服务端返回的报价编号。
	Op       string `json:"op"`           // 本次触屏提交的唯一永久操作号。
}

// verifyMiningBrowserSale 独立读取实际 Mongo 回执，核对原件已经离包且金额只写入目标钱包。
func verifyMiningBrowserSale(t *testing.T, ctx context.Context, store *persistence.Store, after domain.Role, sale miningBrowserSale,
	item domain.Item, shop *devconfig.NPCShop, vendor devconfig.TownNPC) int64 {
	t.Helper()
	rate, ok := shop.MineralBuybackFor(item.ItemID)
	if !ok || sale.Item != item.ItemID || sale.Instance != item.ID || sale.Purity != item.OrePurity || sale.Bound != item.Bound ||
		slices.ContainsFunc(after.Items, func(row domain.Item) bool { return row.ID == item.ID }) {
		t.Fatal("触屏实际出售的原矿未与背包原实例对齐", sale, item)
	}
	price := int64(item.OrePurity) * int64(rate.Numerator) / int64(rate.Denominator)
	if price <= 0 || sale.Price != price {
		t.Fatal("触屏报价与工作簿的逐块纯度价格不一致", sale, price)
	}
	var receipt persistence.Operation
	must(t, store.DB.Collection("operations").FindOne(ctx, bson.M{"_id": "asset:" + after.ID + ":" + sale.Op}).Decode(&receipt))
	if receipt.Code != 0 || receipt.Kind != persistence.AssetVendor || receipt.Vendor == nil || receipt.Vendor.NPC != vendor.ID ||
		receipt.Vendor.Shop != shop.Key || receipt.Vendor.QuoteID != sale.Quote {
		t.Fatal("报价和城镇 NPC 没有写入同一次原永久操作", sale, receipt)
	}
	want := domain.Wallet{Gold: price}
	if item.Bound {
		want = domain.Wallet{BoundGold: price}
	}
	plan := receipt.Vendor.Plan
	if plan.Sold == nil || *plan.Sold != item || plan.Quantity != 1 || plan.Wallet != want {
		t.Fatal("原出售回执未保留单块纯度、绑态或钱包来源", sale, plan)
	}
	return price
}
