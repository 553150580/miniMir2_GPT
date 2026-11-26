//go:build creator_weapon_oil

// 本文件通过独立 Creator 实包、真实 WebSocket 和 MongoDB 验证武器祝福油触屏入口。
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
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
)

// TestWeaponOilBrowserMongoWS 让玩家真实点击已穿武器，再独立核对原操作和资产。
func TestWeaponOilBrowserMongoWS(t *testing.T) {
	webRoot, mode := os.Getenv("MINIMIR_OIL_BROWSER_WEB_ROOT"), os.Getenv("MINIMIR_OIL_BROWSER_MODE")
	if webRoot == "" || os.Getenv("MINIMIR_MONGO_E2E") != "1" || os.Getenv("MINIMIR_INTEGRATION_TEST_MONGO_URI") == "" {
		t.Skip("祝福油触屏验收需要独立 Creator Web Mobile 与已核验的 MongoDB 副本集")
	}
	if mode != "oil" && mode != "default" {
		t.Fatal("必须指定武器祝福油候选模式或默认模式")
	}
	repo, err := filepath.Abs("../../..")
	must(t, err)
	work := filepath.Join(repo, ".work")
	build := weaponOilBrowserInput(t, work, webRoot)
	bundle, err := os.ReadFile(filepath.Join(build, "assets", "main", "index.js"))
	must(t, err)
	bundleHash := persistence.Hash(bundle)
	enabled := mode == "oil"
	fixtureFile := filepath.Join(repo, "config", "development", "first-loop.json")
	if enabled {
		fixtureFile = weaponOilBrowserInput(t, work, os.Getenv("MINIMIR_OIL_BROWSER_FIXTURE"))
	}
	raw, err := os.ReadFile(fixtureFile)
	must(t, err)
	var fixture devconfig.Fixture
	must(t, json.Unmarshal(raw, &fixture))
	fixture.Hash = persistence.Hash(raw)
	must(t, fixture.Validate())
	if (fixture.LuckOil != nil) != enabled {
		t.Fatal("实包验收模式与服务端用油规则不符")
	}
	ctx, store, account, route := startChallengePlayerDeathServer(t, func(cfg *devconfig.Config) {
		cfg.Fixture, cfg.FixtureFile = fixture, fixtureFile
	})
	login, role := &pb.LoginResult{}, &pb.RoleSummary{}
	name, password := "oiltouch_"+domain.ID()[:9], domain.ID()[:16]
	rpc(t, account, "", p.MsgC2SLocalRegister, &pb.LocalLoginRequest{Account: name, Password: password}, login, 0)
	rpc(t, account, login.AccountToken, p.MsgC2SCreateRole, &pb.CreateRoleRequest{
		OriginServerId: route.origin.OriginID, Slot: 1, Job: 1, Name: "触屏祝福" + domain.ID()[:5], OpId: domain.Bytes(domain.ID()),
	}, role, 0)
	roleID := domain.Hex(role.Key.RoleId)
	bag, worn, oil, weaponName, oilName := seedWeaponOilBrowser(t, ctx, store, roleID, &fixture)
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
	// 证据清单只含本次隔离地址、实例和构建身份，临时凭据仅经环境传给浏览器进程。
	metadata := map[string]any{"enabled": enabled, "role": role.Name, "region": route.origin.RegionName, "server": route.origin.Name,
		"endpoint": route.assignment.GameWSURL, "baseUrl": web.URL, "normalMap": fixture.Map.ID, "ruleVersion": fixture.RuleVersion,
		"fixtureSha256": fixture.Hash, "clientBundleSha256": bundleHash, "weaponName": weaponName, "oilName": oilName, "bag": bag, "worn": worn, "oil": oil}
	evidence, err := os.MkdirTemp(work, "weapon-oil-browser-")
	must(t, err)
	manifest := filepath.Join(evidence, "manifest.json")
	raw, err = json.Marshal(metadata)
	must(t, err)
	must(t, os.WriteFile(manifest, raw, 0600))
	t.Log("武器祝福油触屏验收目录 " + evidence)
	browserCtx, stop := context.WithTimeout(ctx, 200*time.Second)
	defer stop()
	node := exec.CommandContext(browserCtx, "node", filepath.Join(repo, "tools/client/verify_weapon_oil_browser.cjs"), manifest)
	node.Dir = repo
	node.Env = append(os.Environ(), "MINIMIR_OIL_BROWSER_ACCOUNT="+name, "MINIMIR_OIL_BROWSER_PASSWORD="+password)
	log, err := node.CombinedOutput()
	if err != nil {
		t.Fatalf("祝福油触屏验收失败：%v\n%s", err, log)
	}
	t.Logf("%s", log)
	verifyWeaponOilBrowserAssets(t, ctx, store, evidence, before, bag, worn, oil, enabled, fixture.Hash, bundleHash)
}

// weaponOilBrowserInput 只加载属于当前工作目录的独立构建和候选配置。
func weaponOilBrowserInput(t *testing.T, work, supplied string) string {
	t.Helper()
	if supplied == "" {
		t.Fatal("缺少独立触屏验收输入")
	}
	resolved, err := filepath.EvalSymlinks(supplied)
	must(t, err)
	resolved, err = filepath.Abs(resolved)
	must(t, err)
	relative, err := filepath.Rel(work, resolved)
	must(t, err)
	if relative == "." || !filepath.IsLocal(relative) {
		t.Fatal("只接受 .work 内的独立验收实包和夹具")
	}
	return resolved
}

// seedWeaponOilBrowser 只预置测试新角的前态，实际扣油及改绑全部走生产入口。
func seedWeaponOilBrowser(t *testing.T, ctx context.Context, store *persistence.Store, roleID string, fixture *devconfig.Fixture) (domain.Item, domain.Item, domain.Item, string, string) {
	t.Helper()
	const woodenSwordID = 218
	definition, found := fixture.Item(woodenSwordID)
	if !found || !definition.IsEquipment || definition.EquipmentSlot != devconfig.WeaponSlot {
		t.Fatal("隔离环境缺少原木剑")
	}
	newSword := func() domain.Item {
		return persistence.InitializeItemDurability(domain.Item{ID: domain.ID(), ItemID: woodenSwordID, Version: 3, Quantity: 1,
			SourceKind: "monster_drop", SourceID: "1"}, fixture)
	}
	bag, worn := newSword(), newSword()
	items := []domain.Item{bag}
	var oil domain.Item
	var oilName string
	if rules := fixture.LuckOil; rules != nil {
		oilDefinition, found := fixture.Item(rules.ItemID)
		if !found || fixture.CombatLuck == nil || fixture.CombatLuck.WeaponWingPPM[3] != 44_000 || fixture.CombatLuck.WeaponWingPPM[4] != 60_000 ||
			fixture.CombatLuck.RoleResistancePPM[1] != 200_000 {
			t.Fatal("隔离环境缺少祝福油与角色幸运的候选配置")
		}
		worn.InitialLuck, worn.CurrentLuck, worn.LuckOilToSix = 3, 3, 100
		oil = domain.Item{ID: domain.ID(), ItemID: oilDefinition.ID, Version: 5, Quantity: 2, Bound: true, SourceKind: "task_reward", SourceID: domain.ID()}
		oilName = oilDefinition.Name
		items = append(items, oil)
	}
	change, err := store.DB.Collection("roles").UpdateByID(ctx, roleID, bson.M{"$set": bson.M{
		"items": items, "equipped": []domain.Equipped{{Slot: devconfig.WeaponSlot, Item: worn}},
	}})
	must(t, err)
	if change.ModifiedCount != 1 {
		t.Fatal("预置隔离角色的背包与穿戴前态失败")
	}
	return bag, worn, oil, definition.Name, oilName
}

// verifyWeaponOilBrowserAssets 对拍触屏看到的真实报价和 MongoDB 中持久的武器、油堆、操作及审计。
func verifyWeaponOilBrowserAssets(t *testing.T, ctx context.Context, store *persistence.Store, evidence string, before domain.Role,
	bag, worn, oil domain.Item, enabled bool, fixtureHash, bundleHash string) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(evidence, "weapon-oil-browser-report.json"))
	must(t, err)
	var report struct {
		Passed, Enabled                   bool
		FixtureSha256, ClientBundleSha256 string
		Commit                            struct {
			Op, Quote, Instance, Session string
		}
		Result struct {
			Quote, Weapon, Oil string
		}
	}
	must(t, json.Unmarshal(raw, &report))
	if !report.Passed || report.Enabled != enabled || report.FixtureSha256 != fixtureHash || report.ClientBundleSha256 != bundleHash {
		t.Fatal("浏览器报告的模式或实包摘要不一致")
	}
	after, err := store.Role(ctx, before.ID)
	must(t, err)
	if !enabled {
		if report.Commit.Op != "" || after.EconomicSeq != before.EconomicSeq || after.Wallet != before.Wallet ||
			!slices.Equal(after.Items, before.Items) || !slices.Equal(after.Equipped, before.Equipped) {
			t.Fatal("默认包打开详情意外变更了原资产")
		}
		return
	}
	if len(report.Commit.Op) != 32 || len(report.Commit.Quote) != 32 || len(report.Commit.Session) != 32 || report.Commit.Instance != worn.ID ||
		report.Result.Quote != report.Commit.Quote || report.Result.Weapon != worn.ID || report.Result.Oil != oil.ID {
		t.Fatal("触屏原提交和回执没有对应同一报价、武器及绑定原油堆", report.Commit, report.Result)
	}
	changedWeapon, oilRemainder := worn, oil
	changedWeapon.Version++
	changedWeapon.CurrentLuck, changedWeapon.LuckOilToSix, changedWeapon.Bound = 4, 101, true
	oilRemainder.Quantity--
	oilRemainder.Version++
	if after.EconomicSeq != before.EconomicSeq+1 || after.Wallet != before.Wallet ||
		!slices.Equal(after.Equipped, []domain.Equipped{{Slot: devconfig.WeaponSlot, Item: changedWeapon}}) ||
		!slices.Equal(after.Items, []domain.Item{bag, oilRemainder}) {
		t.Fatal("原穿戴武器、背包和经济水位没有只结算一次", after.Equipped, after.Items)
	}
	var operation persistence.Operation
	id := "asset:" + before.ID + ":" + report.Commit.Op
	must(t, store.DB.Collection("operations").FindOne(ctx, bson.M{"_id": id, "role_id": before.ID}).Decode(&operation))
	receipt := operation.WeaponOil
	if operation.Kind != persistence.AssetWeaponOilCommit || operation.Code != 0 || receipt == nil || receipt.Weapon != changedWeapon ||
		receipt.OilRemainder == nil || *receipt.OilRemainder != oilRemainder || receipt.OilConsumed != 1 || !receipt.Success || receipt.PityUsed != 101 ||
		receipt.QuoteID != report.Commit.Quote || receipt.UISession != report.Commit.Session || receipt.Config != fixtureHash ||
		receipt.Plan.Weapon != worn || receipt.Plan.Oil != oil || !receipt.Plan.Equipped || !receipt.Plan.AllowBound || !receipt.Plan.Guaranteed {
		t.Fatal("武器用油的永久原事务回执不一致", operation)
	}
	var audit struct {
		WeaponOil *persistence.WeaponOilResultReceipt `bson:"weapon_oil"`
	}
	must(t, store.DB.Collection("audit_outbox").FindOne(ctx, bson.M{"_id": id, "kind": persistence.AssetWeaponOilCommit}).Decode(&audit))
	if !reflect.DeepEqual(audit.WeaponOil, receipt) {
		t.Fatal("审计中的用油事实与永久回执不一致")
	}
}
