//go:build creator_wing

// 本文件用已构建的 Creator 手机触屏入口验证角色翅膀，并逐笔对拍独立 MongoDB 原凭证。
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

// wingTouchState 是浏览器看到的原实例和可变外观，版本保持精确字符串。
type wingTouchState struct {
	WingID           string
	Rank, Tier, Luck uint32
	Hidden           bool
	Version          string
}

// wingTouchReport 只接收触屏运行回收的字段，资产事实仍以独立数据库为准。
type wingTouchReport struct {
	Passed, Enabled, DefaultEntryHidden, OldEquipmentVisible, BoundCancel, BoundConfirmed bool
	FixtureSha256, ClientBundleSha256                                                     string
	Operations                                                                            []struct {
		wingTouchState
		Op     string
		Action uint32
		Spent  int64
	}
	Final      wingTouchState
	Attributes struct {
		Before, After uint32
		Enabled       bool
	}
}

// TestWingCreatorTouchMongo 验证默认旧角色栏及候选包从原选材到已解锁外观的真实触屏流程。
func TestWingCreatorTouchMongo(t *testing.T) {
	webRoot, mode := os.Getenv("MINIMIR_WING_BROWSER_WEB_ROOT"), os.Getenv("MINIMIR_WING_BROWSER_MODE")
	if webRoot == "" || os.Getenv("MINIMIR_MONGO_E2E") != "1" || os.Getenv("MINIMIR_INTEGRATION_TEST_MONGO_URI") == "" {
		t.Skip("翅膀触屏验收需要独立 Creator Web Mobile 和已核验的 MongoDB 副本集")
	}
	if mode != "wing" && mode != "default" {
		t.Fatal("请选择默认或隔离翅膀包")
	}
	repo, err := filepath.Abs("../../..")
	must(t, err)
	work := filepath.Join(repo, ".work")
	build, err := creatorBrowserBuildPath(repo, webRoot)
	must(t, err)
	bundle, err := os.ReadFile(filepath.Join(build, "assets", "main", "index.js"))
	must(t, err)
	bundleHash := persistence.Hash(bundle)
	enabled := mode == "wing"
	fixture, fixtureFile := readWingTouchFixture(t, repo, enabled)
	ctx, store, account, route := startChallengePlayerDeathServer(t, func(cfg *devconfig.Config) {
		cfg.Fixture, cfg.FixtureFile = fixture, fixtureFile
	})

	// 唯一武器、原背包容量及绑定羽毛都是隔离前态，成长和外观须经真实游戏服提交。
	login, role := &pb.LoginResult{}, &pb.RoleSummary{}
	name, password := "wing_touch_"+domain.ID()[:9], domain.ID()[:16]
	rpc(t, account, "", p.MsgC2SLocalRegister, &pb.LocalLoginRequest{Account: name, Password: password}, login, 0)
	rpc(t, account, login.AccountToken, p.MsgC2SCreateRole, &pb.CreateRoleRequest{
		OriginServerId: route.origin.OriginID, Slot: 1, Job: 0, Name: "触屏翅膀" + domain.ID()[:5], OpId: domain.Bytes(domain.ID()),
	}, role, 0)
	before, weapon, feather := seedWingTouchRole(t, ctx, store, domain.Hex(role.Key.RoleId), &fixture)
	upstream, err := url.Parse(account)
	must(t, err)
	upstream.Path = ""
	mux := http.NewServeMux()
	mux.Handle("/account/rpc", http.StripPrefix("/account", httputil.NewSingleHostReverseProxy(upstream)))
	mux.Handle("/", http.FileServer(http.Dir(build)))
	web := httptest.NewServer(mux)
	defer web.Close()

	evidence, err := os.MkdirTemp(work, "wing-browser-")
	must(t, err)
	manifest := filepath.Join(evidence, "manifest.json")
	definition, known := fixture.Item(weapon.ItemID)
	if !known {
		t.Fatal("缺少旧角色栏的原木剑定义")
	}
	data := map[string]any{"enabled": enabled, "role": role.Name, "region": route.origin.RegionName, "server": route.origin.Name,
		"endpoint": route.assignment.GameWSURL, "baseUrl": web.URL, "normalMap": fixture.Map.ID, "ruleVersion": fixture.RuleVersion,
		"fixtureSha256": fixture.Hash, "clientBundleSha256": bundleHash, "weapon": weapon, "weaponName": definition.Name,
		"feather": feather, "expectedLuckPPM": wingTouchLuck(&fixture, weapon), "bagCount": len(before.Items), "capacity": before.Capacity}
	raw, err := json.Marshal(data)
	must(t, err)
	must(t, os.WriteFile(manifest, raw, 0600))
	t.Log("翅膀触屏验收目录 " + evidence)

	browserCtx, stop := context.WithTimeout(ctx, 215*time.Second)
	defer stop()
	node := exec.CommandContext(browserCtx, "node", filepath.Join(repo, "tools/client/verify_wing_browser.cjs"), manifest)
	node.Dir = repo
	node.Env = append(os.Environ(), "MINIMIR_WING_BROWSER_ACCOUNT="+name, "MINIMIR_WING_BROWSER_PASSWORD="+password)
	log, err := node.CombinedOutput()
	if err != nil {
		t.Fatalf("翅膀触屏验收失败：%v\n%s", err, log)
	}
	t.Logf("%s", log)
	verifyWingTouchLedger(t, ctx, store, evidence, before, weapon, feather, &fixture, bundleHash)
}

// readWingTouchFixture 保证默认仍使用规则30，候选只接收项目隔离目录编码后的实际配置。
func readWingTouchFixture(t *testing.T, repo string, enabled bool) (devconfig.Fixture, string) {
	t.Helper()
	fixtureFile := filepath.Join(repo, "config", "development", "first-loop.json")
	if enabled {
		input := os.Getenv("MINIMIR_WING_BROWSER_FIXTURE")
		if input == "" {
			t.Fatal("缺少隔离翅膀配置")
		}
		resolved, err := filepath.EvalSymlinks(input)
		must(t, err)
		fixtureFile, err = filepath.Abs(resolved)
		must(t, err)
		relative, err := filepath.Rel(filepath.Join(repo, ".work", "m4-wing-projection"), fixtureFile)
		must(t, err)
		if relative == "." || !filepath.IsLocal(relative) {
			t.Fatal("只接受本次独立翅膀投影")
		}
	}
	raw, err := os.ReadFile(fixtureFile)
	must(t, err)
	var fixture devconfig.Fixture
	must(t, json.Unmarshal(raw, &fixture))
	fixture.Hash = persistence.Hash(raw)
	must(t, fixture.Validate())
	if enabled && (fixture.Wings == nil || len(fixture.Wings.Ranks) != 3 || !slices.Contains(fixture.Wings.AppearanceTiers, uint32(3))) ||
		!enabled && (fixture.Wings != nil || fixture.RuleVersion != 30) {
		t.Fatal("网页包与隔离三阶或默认规则30不匹配")
	}
	return fixture, fixtureFile
}

// seedWingTouchRole 选同一件原木剑置入武器栏并添加原绑定羽毛；正常装备位和容量照旧。
func seedWingTouchRole(t *testing.T, ctx context.Context, store *persistence.Store, roleID string,
	fixture *devconfig.Fixture) (domain.Role, domain.Item, *domain.Item) {
	t.Helper()
	initial, err := store.Role(ctx, roleID)
	must(t, err)
	at := slices.IndexFunc(initial.Items, func(item domain.Item) bool { return item.ItemID == 218 })
	if at < 0 || len(initial.Equipped) != 0 {
		t.Fatal("隔离角色缺少原木剑或已有穿戴")
	}
	weapon := persistence.InitializeItemDurability(initial.Items[at], fixture)
	if weapon.DurabilityU <= 0 || !weapon.HasValidDurability() {
		t.Fatal("原旧武器需要有效耐久才能核对权威幸运")
	}
	items := slices.Delete(initial.Items, at, at+1)
	var feather *domain.Item
	if fixture.Wings != nil {
		weapon.CurrentLuck = 4
		var quantity int64
		for _, rank := range fixture.Wings.Ranks {
			quantity += rank.Feathers
		}
		feather = &domain.Item{ID: domain.ID(), ItemID: fixture.Wings.FeatherItemID, Quantity: quantity, Bound: true,
			Version: 1, SourceKind: "world_boss", SourceID: "wing_creator_fixture"}
		items = append(items, *feather)
	}
	equipped := []domain.Equipped{{Slot: devconfig.WeaponSlot, Item: weapon}}
	change, err := store.DB.Collection("roles").UpdateByID(ctx, roleID, bson.M{"$set": bson.M{"items": items, "equipped": equipped}})
	must(t, err)
	if change.ModifiedCount != 1 {
		t.Fatal("隔离角色的旧装备和原羽毛前态没有保存")
	}
	before, err := store.Role(ctx, roleID)
	must(t, err)
	if before.Wing != nil || before.Capacity != initial.Capacity || len(before.Items) > before.Capacity {
		t.Fatal("未激活时不应存在独立翅膀或修改背包容量")
	}
	return before, weapon, feather
}

// wingTouchLuck 从工作簿投影得出旧原武器的幸运，激活时翅膀仍从零开始。
func wingTouchLuck(fixture *devconfig.Fixture, weapon domain.Item) uint32 {
	if fixture.CombatLuck == nil {
		return 0
	}
	return fixture.CombatLuck.WeaponWingPPM[weapon.EffectiveLuck()]
}

// verifyWingTouchLedger 独立核对五笔触屏操作、原件用量、审计及角色经济总量。
func verifyWingTouchLedger(t *testing.T, ctx context.Context, store *persistence.Store, evidence string, before domain.Role,
	weapon domain.Item, feather *domain.Item, fixture *devconfig.Fixture, bundleHash string) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(evidence, "wing-browser-report.json"))
	must(t, err)
	var report wingTouchReport
	must(t, json.Unmarshal(raw, &report))
	enabled, attackPPM := fixture.Wings != nil, wingTouchLuck(fixture, weapon)
	if !report.Passed || report.Enabled != enabled || !report.OldEquipmentVisible || report.FixtureSha256 != fixture.Hash ||
		report.ClientBundleSha256 != bundleHash || report.Attributes.Enabled != (fixture.CombatLuck != nil) ||
		report.Attributes.Before != attackPPM || report.Attributes.After != attackPPM {
		t.Fatal("浏览器的模式、打包摘要或原装备幸运不匹配", report.Attributes)
	}
	after, err := store.Role(ctx, before.ID)
	must(t, err)
	permanent := store.DB.Collection("role_wings")
	documents, err := permanent.CountDocuments(ctx, bson.M{"_id": before.ID})
	must(t, err)
	operations := store.DB.Collection("operations")
	count, err := operations.CountDocuments(ctx, bson.M{"role_id": before.ID, "kind": persistence.AssetWingAction})
	must(t, err)
	if !enabled {
		if !report.DefaultEntryHidden || len(report.Operations) != 0 || count != 0 || documents != 0 || after.Wing != nil ||
			after.EconomicSeq != before.EconomicSeq || after.Wallet != before.Wallet || !slices.Equal(before.Items, after.Items) ||
			!slices.Equal(before.Equipped, after.Equipped) {
			t.Fatal("默认触屏隐藏入口后改变了原装备或资产")
		}
		return
	}
	if !report.BoundCancel || !report.BoundConfirmed || count != 5 || documents != 1 || len(report.Operations) != 5 || after.Wing == nil {
		t.Fatal("应由真实触屏完成三次成长和两次外观操作", count, documents)
	}
	remain := *feather
	tier := []uint32{1, 1, 3, 1, 1}
	var previous *domain.WingState
	for i, candidate := range report.Operations {
		if len(candidate.Op) != 32 || candidate.Rank != uint32(min(i+1, 3)) || candidate.Tier != tier[i] ||
			candidate.Hidden != (i == 4) || candidate.Luck != 0 || candidate.WingID != report.Final.WingID {
			t.Fatal("界面阶段、外观或原翅膀身份与已确认预期不符", candidate)
		}
		key := bson.M{"_id": "asset:" + before.ID + ":" + candidate.Op, "role_id": before.ID}
		var op persistence.Operation
		must(t, operations.FindOne(ctx, key).Decode(&op))
		receipt := op.Wing
		if op.Kind != persistence.AssetWingAction || op.Code != 0 || op.CommandSeq != before.EconomicSeq+int64(i)+1 ||
			receipt == nil || receipt.Action != candidate.Action || receipt.Config != fixture.Hash || receipt.Rule != fixture.RuleVersion ||
			!reflect.DeepEqual(previous, receipt.Before) || !wingTouchStateMatches(candidate.wingTouchState, receipt.Wing) {
			t.Fatal("触屏原结果未匹配同序、同规则的持久回执", op)
		}
		var audit struct {
			Wing *persistence.WingActionReceipt `bson:"wing"`
		}
		must(t, store.DB.Collection("audit_outbox").FindOne(ctx, key).Decode(&audit))
		if !reflect.DeepEqual(receipt, audit.Wing) {
			t.Fatal("同一原翼回执没有原样写入审计", candidate.Op)
		}
		if i < 3 {
			want := fixture.Wings.Ranks[i].Feathers
			if receipt.Action != 1 || !receipt.AllowBound || len(receipt.QuoteID) != 32 || len(receipt.Feathers) != 1 ||
				candidate.Spent != want || receipt.Feathers[0].Quantity != want || receipt.Feathers[0].Item != remain {
				t.Fatal("逐阶报价没有精确消耗同一份原绑定羽毛", receipt)
			}
			remain.Quantity -= want
			remain.Version++
		} else if receipt.Action != 2 || receipt.AllowBound || receipt.QuoteID != "" || candidate.Spent != 0 || len(receipt.Feathers) != 0 {
			t.Fatal("独立外观操作不应消耗材料", receipt)
		}
		previous = &receipt.Wing
	}
	var original domain.WingState
	must(t, permanent.FindOne(ctx, bson.M{"_id": before.ID}).Decode(&original))
	var roleDocument bson.M
	must(t, store.DB.Collection("roles").FindOne(ctx, bson.M{"_id": before.ID}).Decode(&roleDocument))
	_, embedded := roleDocument["wing"]
	wantItems := slices.DeleteFunc(slices.Clone(before.Items), func(item domain.Item) bool { return item.ID == feather.ID })
	if embedded || remain.Quantity != 0 || original != *after.Wing || original.OwnerRoleID != before.ID ||
		!wingTouchStateMatches(report.Final, original) || after.Capacity != before.Capacity || after.Wallet != before.Wallet ||
		after.EconomicSeq != before.EconomicSeq+count || after.SourceSeq != before.SourceSeq || !slices.Equal(after.Items, wantItems) ||
		!slices.Equal(after.Equipped, before.Equipped) {
		t.Fatal("激活、升阶或外观未保留独立原实例、旧装备及背包守恒", original, after.Items)
	}
	t.Log("原绑定羽毛、独立翅膀、五笔原持久回执和装备幸运对账一致")
}

// wingTouchStateMatches 用真实原实例检查浏览器见到的完整成长和外观。
func wingTouchStateMatches(touch wingTouchState, stored domain.WingState) bool {
	return touch.WingID == stored.ID && touch.Rank == stored.Rank && touch.Tier == stored.AppearanceTier &&
		touch.Luck == stored.Luck && touch.Hidden == stored.Hidden && touch.Version == strconv.FormatInt(stored.Version, 10)
}
