//go:build creator_wing_oil

// 本文件用实际 Creator 触屏把原隐藏翅膀、原武器和一次用油与隔离 MongoDB 原账对拍。
package integration

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
)

// wingOilTouchReport 保存触屏实际所见，永久消费和目标版本独立查询原账。
type wingOilTouchReport struct {
	Passed, Enabled, DefaultWingHidden, DefaultWeaponHidden, OldEquipmentVisible bool
	BoundCancelled, BoundGranted, PermissionPreserved, ReopenedOff               bool
	FixtureSha256, ClientBundleSha256                                            string
	Operation                                                                    *struct {
		Op, WingID, Version, Oil string
		Rank, Tier, Luck, Pity   uint32
		Hidden, OilBound         bool
	}
	Attributes struct {
		Before, After uint32
		Enabled       bool
	}
}

// TestWingOilCreatorTouchMongo 仅启动隔离运行服和真实 Web Mobile，候选与默认包分别验收。
func TestWingOilCreatorTouchMongo(t *testing.T) {
	webRoot, mode := os.Getenv("MINIMIR_WING_OIL_BROWSER_WEB_ROOT"), os.Getenv("MINIMIR_WING_OIL_BROWSER_MODE")
	if webRoot == "" || os.Getenv("MINIMIR_MONGO_E2E") != "1" || os.Getenv("MINIMIR_INTEGRATION_TEST_MONGO_URI") == "" {
		t.Skip("翅膀用油触屏需独立 Creator Web Mobile 与已核验的 MongoDB 副本集")
	}
	if mode != "wing-oil" && mode != "default" {
		t.Fatal("须指定默认或独立翅膀用油候选")
	}
	repo, err := filepath.Abs("../../..")
	must(t, err)
	work := filepath.Join(repo, ".work")
	build, err := creatorBrowserBuildPath(repo, webRoot)
	must(t, err)
	bundle, err := os.ReadFile(filepath.Join(build, "assets", "main", "index.js"))
	must(t, err)
	bundleHash := persistence.Hash(bundle)
	enabled := mode == "wing-oil"
	fixture, fixtureFile := readWingOilTouchFixture(t, repo, enabled)
	ctx, store, account, route := startChallengePlayerDeathServer(t, func(cfg *devconfig.Config) {
		cfg.Fixture, cfg.FixtureFile = fixture, fixtureFile
	})
	login, role := &pb.LoginResult{}, &pb.RoleSummary{}
	name, password := "w_oil_touch_"+domain.ID()[:8], domain.ID()[:16]
	rpc(t, account, "", p.MsgC2SLocalRegister, &pb.LocalLoginRequest{Account: name, Password: password}, login, 0)
	rpc(t, account, login.AccountToken, p.MsgC2SCreateRole, &pb.CreateRoleRequest{
		OriginServerId: route.origin.OriginID, Slot: 1, Job: 0, Name: "翼油触屏" + domain.ID()[:5], OpId: domain.Bytes(domain.ID()),
	}, role, 0)
	before, weapon, bound, free := seedWingOilTouchRole(t, ctx, store, domain.Hex(role.Key.RoleId), &fixture)
	definition, known := fixture.Item(weapon.ItemID)
	if !known {
		t.Fatal("缺少原角色栏的木剑定义")
	}

	upstream, err := url.Parse(account)
	must(t, err)
	upstream.Path = ""
	mux := http.NewServeMux()
	mux.Handle("/account/rpc", http.StripPrefix("/account", httputil.NewSingleHostReverseProxy(upstream)))
	mux.Handle("/", http.FileServer(http.Dir(build)))
	web := httptest.NewServer(mux)
	defer web.Close()
	evidence, err := os.MkdirTemp(work, "wing-oil-browser-")
	must(t, err)
	manifest := filepath.Join(evidence, "manifest.json")
	attributesBefore, attributesAfter := wingOilTouchAttributes(&fixture)
	data := map[string]any{"enabled": enabled, "role": role.Name, "region": route.origin.RegionName, "server": route.origin.Name,
		"endpoint": route.assignment.GameWSURL, "baseUrl": web.URL, "normalMap": fixture.Map.ID, "ruleVersion": fixture.RuleVersion,
		"fixtureSha256": fixture.Hash, "clientBundleSha256": bundleHash, "weapon": weapon, "weaponName": definition.Name,
		"wing": before.Wing, "boundOil": bound, "freeOil": free, "attributesBefore": attributesBefore, "attributesAfter": attributesAfter}
	raw, err := json.Marshal(data)
	must(t, err)
	must(t, os.WriteFile(manifest, raw, 0600))
	t.Log("翅膀用油触屏验收目录 " + evidence)
	browserCtx, stop := context.WithTimeout(ctx, 215*time.Second)
	defer stop()
	node := exec.CommandContext(browserCtx, "node", filepath.Join(repo, "tools/client/verify_wing_oil_browser.cjs"), manifest)
	node.Dir = repo
	node.Env = append(os.Environ(), "MINIMIR_WING_OIL_BROWSER_ACCOUNT="+name, "MINIMIR_WING_OIL_BROWSER_PASSWORD="+password)
	log, err := node.CombinedOutput()
	if err != nil {
		t.Fatalf("翅膀用油触屏验收失败：%v\n%s", err, log)
	}
	t.Logf("%s", log)
	verifyWingOilTouchLedger(t, ctx, store, evidence, before, bound, &fixture, bundleHash)
}

// readWingOilTouchFixture 拒绝其它环境输入并对拍翅膀功能开关。
func readWingOilTouchFixture(t *testing.T, repo string, enabled bool) (devconfig.Fixture, string) {
	t.Helper()
	fixtureFile := filepath.Join(repo, "config", "development", "first-loop.json")
	if enabled {
		input := os.Getenv("MINIMIR_WING_OIL_BROWSER_FIXTURE")
		if input == "" {
			t.Fatal("缺少本轮隔离翅膀用油配置")
		}
		resolved, err := filepath.EvalSymlinks(input)
		must(t, err)
		fixtureFile, err = filepath.Abs(resolved)
		must(t, err)
		relative, err := filepath.Rel(filepath.Join(repo, ".work", "m4-wing-oil-projection"), fixtureFile)
		must(t, err)
		if relative == "." || !filepath.IsLocal(relative) {
			t.Fatal("只接受本轮隔离翅膀用油配置")
		}
	}
	raw, err := os.ReadFile(fixtureFile)
	must(t, err)
	var fixture devconfig.Fixture
	must(t, json.Unmarshal(raw, &fixture))
	fixture.Hash = persistence.Hash(raw)
	must(t, fixture.Validate())
	if enabled && (fixture.Wings == nil || !fixture.Wings.AllowLuckOil || fixture.LuckOil == nil || fixture.CombatLuck == nil) ||
		!enabled && (fixture.RuleVersion != 30 || fixture.Wings != nil || fixture.LuckOil != nil || fixture.CombatLuck != nil) {
		t.Fatal("默认或翅膀用油实包的对应业务装配不符")
	}
	return fixture, fixtureFile
}

// seedWingOilTouchRole 仅准备一个初始隔离角色及两种绑定原油，点击后的操作必须来自实际游戏服。
func seedWingOilTouchRole(t *testing.T, ctx context.Context, store *persistence.Store, roleID string,
	fixture *devconfig.Fixture) (domain.Role, domain.Item, *domain.Item, *domain.Item) {
	t.Helper()
	initial, err := store.Role(ctx, roleID)
	must(t, err)
	index := slices.IndexFunc(initial.Items, func(item domain.Item) bool { return item.ItemID == 218 })
	if index < 0 || len(initial.Equipped) != 0 {
		t.Fatal("原初始角色没有待穿的唯一木剑")
	}
	weapon := persistence.InitializeItemDurability(initial.Items[index], fixture)
	if weapon.DurabilityU <= 0 || !weapon.HasValidDurability() {
		t.Fatal("原穿戴武器需要有效耐久")
	}
	items := slices.Delete(initial.Items, index, index+1)
	var bound, free *domain.Item
	if fixture.Wings != nil {
		weapon.InitialLuck, weapon.CurrentLuck = 3, 3
		bound = &domain.Item{ID: domain.ID(), ItemID: fixture.LuckOil.ItemID, Quantity: 2, Version: 1, Bound: true}
		free = &domain.Item{ID: domain.ID(), ItemID: fixture.LuckOil.ItemID, Quantity: 3, Version: 1}
		items = append(items, *bound, *free)
		wing := domain.WingState{OwnerRoleID: roleID, ID: domain.ID(), Rank: 3, AppearanceTier: 1, Hidden: true,
			Luck: 3, OilToSix: 100, OilToNine: 0, Version: 4}
		_, err := store.DB.Collection("role_wings").InsertOne(ctx, wing)
		must(t, err)
	}
	equipped := []domain.Equipped{{Slot: devconfig.WeaponSlot, Item: weapon}}
	change, err := store.DB.Collection("roles").UpdateByID(ctx, roleID, bson.M{"$set": bson.M{"items": items, "equipped": equipped}})
	must(t, err)
	if change.ModifiedCount != 1 {
		t.Fatal("未能建立隔离角色前态")
	}
	before, err := store.Role(ctx, roleID)
	must(t, err)
	if before.Capacity != initial.Capacity || len(before.Items) > before.Capacity || (before.Wing != nil) != (fixture.Wings != nil) {
		t.Fatal("隔离翅膀不应使用原装备或背包位置")
	}
	return before, weapon, bound, free
}

// wingOilTouchAttributes 得到原武器和原翅膀共同贡献的确切百万分率。
func wingOilTouchAttributes(fixture *devconfig.Fixture) (uint32, uint32) {
	if fixture.CombatLuck == nil {
		return 0, 0
	}
	table := fixture.CombatLuck.WeaponWingPPM
	return table[3] + table[3], table[3] + table[4]
}

// verifyWingOilTouchLedger 核对实际只扣一瓶原绑定油，未改变原武器和翅膀归属、外观和阶数。
func verifyWingOilTouchLedger(t *testing.T, ctx context.Context, store *persistence.Store, evidence string, before domain.Role,
	bound *domain.Item, fixture *devconfig.Fixture, bundleHash string) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(evidence, "wing-oil-browser-report.json"))
	must(t, err)
	var report wingOilTouchReport
	must(t, json.Unmarshal(raw, &report))
	enabled := fixture.Wings != nil
	attackBefore, attackAfter := wingOilTouchAttributes(fixture)
	if !report.Passed || report.Enabled != enabled || !report.OldEquipmentVisible || report.FixtureSha256 != fixture.Hash ||
		report.ClientBundleSha256 != bundleHash || report.Attributes.Enabled != enabled ||
		report.Attributes.Before != attackBefore || report.Attributes.After != attackAfter {
		t.Fatal("原实包、隔离输入和权威幸运观察不一致", report.Attributes)
	}
	after, err := store.Role(ctx, before.ID)
	must(t, err)
	operationCollection := store.DB.Collection("operations")
	count, err := operationCollection.CountDocuments(ctx, bson.M{"role_id": before.ID, "kind": persistence.AssetWeaponOilCommit})
	must(t, err)
	if after.Capacity != before.Capacity || after.SourceSeq != before.SourceSeq || after.Wallet != before.Wallet ||
		!slices.Equal(after.Equipped, before.Equipped) || after.EconomicSeq != before.EconomicSeq+count {
		t.Fatal("切换目标、开关和查看不应改变原武器、背包容量或永久结算")
	}
	if !enabled {
		if !report.DefaultWingHidden || !report.DefaultWeaponHidden || count != 0 || report.Operation != nil ||
			after.Wing != nil || !slices.Equal(after.Items, before.Items) {
			t.Fatal("默认触屏出现培养入口或产生永久用油")
		}
		return
	}
	actual := report.Operation
	if !report.BoundCancelled || !report.BoundGranted || !report.PermissionPreserved || !report.ReopenedOff ||
		count != 1 || actual == nil || len(actual.Op) != 32 || actual.WingID != before.Wing.ID || actual.Luck != 4 ||
		actual.Pity != 101 || !actual.Hidden || actual.Tier != before.Wing.AppearanceTier || actual.Rank != before.Wing.Rank ||
		actual.Version != strconv.FormatInt(before.Wing.Version+1, 10) || actual.Oil != bound.ID || !actual.OilBound {
		t.Fatal("操作未体现只消费一瓶原绑定油、保留原翅膀及本面板授权", actual)
	}
	var op persistence.Operation
	key := bson.M{"_id": "asset:" + before.ID + ":" + actual.Op, "role_id": before.ID}
	must(t, operationCollection.FindOne(ctx, key).Decode(&op))
	receipt := op.WeaponOil
	wantWing := *before.Wing
	wantWing.Luck, wantWing.OilToSix, wantWing.Version = 4, 101, wantWing.Version+1
	remain := *bound
	remain.Quantity--
	remain.Version++
	if op.Kind != persistence.AssetWeaponOilCommit || op.Code != 0 || op.CommandSeq != before.EconomicSeq+1 ||
		receipt == nil || receipt.Config != fixture.Hash || receipt.Rule != fixture.RuleVersion || !receipt.Success || !receipt.Plan.Guaranteed ||
		!receipt.Plan.AllowBound || receipt.Plan.Equipped || receipt.Plan.Weapon.ID != "" || receipt.Plan.Oil != *bound ||
		receipt.OilConsumed != 1 || receipt.PityUsed != 101 || !reflect.DeepEqual(receipt.Wing, &wantWing) ||
		!reflect.DeepEqual(receipt.Plan.Wing, before.Wing) || !reflect.DeepEqual(receipt.OilRemainder, &remain) {
		t.Fatal("实际用油单没有记录原报价、原目标及原油余量", receipt)
	}
	var audit struct {
		Receipt *persistence.WeaponOilResultReceipt `bson:"weapon_oil"`
	}
	must(t, store.DB.Collection("audit_outbox").FindOne(ctx, key).Decode(&audit))
	wantItems := slices.Clone(before.Items)
	index := slices.Index(wantItems, *bound)
	if index < 0 {
		t.Fatal("原库存缺少被选中的原绑定油")
	}
	wantItems[index] = remain
	if !reflect.DeepEqual(audit.Receipt, receipt) || !reflect.DeepEqual(after.Wing, &wantWing) || !slices.Equal(after.Items, wantItems) {
		t.Fatal("独立原翅膀、同事务审计或原背包油堆不符", after.Wing)
	}
	t.Log("原隐藏翅膀、原绑定油、未变武器与唯一持久用油审计一致")
}
