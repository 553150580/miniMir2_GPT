//go:build creator_recycle_oil

// 本文件通过独立 Creator 实包和真实 MongoDB 校验装备分解时所见原概率、绑定及永久结果。
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

const recycleOilBrowserMaxAttempts = 24 // 有限隔离库存供按真实概率逐件验证，命中后立即停止。

// TestRecycleOilBrowserMongoWS 用现实触屏继续分解，直到服务器按原概率确认一瓶真实祝福油。
func TestRecycleOilBrowserMongoWS(t *testing.T) {
	webRoot, mode := os.Getenv("MINIMIR_RECYCLE_BROWSER_WEB_ROOT"), os.Getenv("MINIMIR_RECYCLE_BROWSER_MODE")
	if webRoot == "" || os.Getenv("MINIMIR_MONGO_E2E") != "1" || os.Getenv("MINIMIR_INTEGRATION_TEST_MONGO_URI") == "" {
		t.Skip("装备分解触屏验收需要独立 Creator Web Mobile 与已核验的 MongoDB 副本集")
	}
	if mode != "recycle" && mode != "default" {
		t.Fatal("必须指定装备分解候选或默认模式")
	}
	repo, err := filepath.Abs("../../..")
	must(t, err)
	work := filepath.Join(repo, ".work")
	build := recycleOilBrowserInput(t, work, webRoot)
	bundle, err := os.ReadFile(filepath.Join(build, "assets", "main", "index.js"))
	must(t, err)
	bundleHash := persistence.Hash(bundle)
	enabled := mode == "recycle"
	fixtureFile := filepath.Join(repo, "config", "development", "first-loop.json")
	if enabled {
		fixtureFile = recycleOilBrowserInput(t, work, os.Getenv("MINIMIR_RECYCLE_BROWSER_FIXTURE"))
	}
	raw, err := os.ReadFile(fixtureFile)
	must(t, err)
	var fixture devconfig.Fixture
	must(t, json.Unmarshal(raw, &fixture))
	fixture.Hash = persistence.Hash(raw)
	must(t, fixture.Validate())
	if enabled && (fixture.EquipmentRecycleOil == nil || fixture.EquipmentRecycleOil.Denominator != 30 || fixture.LuckOil == nil) ||
		!enabled && (fixture.EquipmentRecycleOil != nil || fixture.RuleVersion != 30) {
		t.Fatal("客户端验收必须匹配工作簿分母30或未开启产油的规则30")
	}
	ctx, store, account, route := startChallengePlayerDeathServer(t, func(cfg *devconfig.Config) {
		cfg.Fixture, cfg.FixtureFile = fixture, fixtureFile
	})
	login, role := &pb.LoginResult{}, &pb.RoleSummary{}
	name, password := "rec_touch_"+domain.ID()[:9], domain.ID()[:16]
	rpc(t, account, "", p.MsgC2SLocalRegister, &pb.LocalLoginRequest{Account: name, Password: password}, login, 0)
	rpc(t, account, login.AccountToken, p.MsgC2SCreateRole, &pb.CreateRoleRequest{
		OriginServerId: route.origin.OriginID, Slot: 1, Job: 1, Name: "触屏分解" + domain.ID()[:5], OpId: domain.Bytes(domain.ID()),
	}, role, 0)
	roleID := domain.Hex(role.Key.RoleId)
	preview, attempts, definition := seedRecycleOilBrowser(t, ctx, store, roleID, &fixture, enabled)
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
	// 程序只需知道隔离入口及待选原实例，临时凭据仅由子进程环境传递。
	manifestData := map[string]any{"enabled": enabled, "role": role.Name, "region": route.origin.RegionName, "server": route.origin.Name,
		"endpoint": route.assignment.GameWSURL, "baseUrl": web.URL, "normalMap": fixture.Map.ID, "ruleVersion": fixture.RuleVersion,
		"fixtureSha256": fixture.Hash, "clientBundleSha256": bundleHash, "weaponName": definition.Name, "preview": preview, "attempts": attempts}
	evidence, err := os.MkdirTemp(work, "recycle-oil-browser-")
	must(t, err)
	manifest := filepath.Join(evidence, "manifest.json")
	raw, err = json.Marshal(manifestData)
	must(t, err)
	must(t, os.WriteFile(manifest, raw, 0600))
	t.Log("分解祝福油触屏验收目录 " + evidence)
	browserCtx, stop := context.WithTimeout(ctx, 220*time.Second)
	defer stop()
	node := exec.CommandContext(browserCtx, "node", filepath.Join(repo, "tools/client/verify_recycle_oil_browser.cjs"), manifest)
	node.Dir = repo
	node.Env = append(os.Environ(), "MINIMIR_RECYCLE_BROWSER_ACCOUNT="+name, "MINIMIR_RECYCLE_BROWSER_PASSWORD="+password)
	log, err := node.CombinedOutput()
	if err != nil {
		t.Fatalf("装备分解触屏验收失败：%v\n%s", err, log)
	}
	t.Logf("%s", log)
	verifyRecycleOilBrowser(t, ctx, store, evidence, before, preview, attempts, definition, &fixture, bundleHash)
}

// recycleOilBrowserInput 只允许使用 .work 内本次已独立构建的产物与明确候选。
func recycleOilBrowserInput(t *testing.T, work, supplied string) string {
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
		t.Fatal("只接受 .work 内的独立实包和夹具")
	}
	return resolved
}

// seedRecycleOilBrowser 只准备可确认的正耐久旧物，实际抽样、原件销毁和发油都由服务器结算。
func seedRecycleOilBrowser(t *testing.T, ctx context.Context, store *persistence.Store, roleID string, fixture *devconfig.Fixture,
	enabled bool) (domain.Item, []domain.Item, devconfig.Item) {
	t.Helper()
	const woodenSwordID = 218
	definition, found := fixture.Item(woodenSwordID)
	if !found || !definition.EquipmentRecycleKnown() || !definition.IsEquipment || definition.EquipmentSlot != devconfig.WeaponSlot {
		t.Fatal("隔离角色缺少已确认可分解的原木剑")
	}
	newSword := func() domain.Item {
		return persistence.InitializeItemDurability(domain.Item{ID: domain.ID(), ItemID: woodenSwordID, Version: 1,
			Quantity: 1, InitialLuck: 3, Bound: true, SourceKind: "monster_drop", SourceID: "1"}, fixture)
	}
	preview := newSword()
	if !preview.HasValidDurability() || preview.DurabilityU <= 0 {
		t.Fatal("分解浏览器前态须为有效正耐久装备")
	}
	items, attempts := []domain.Item{preview}, []domain.Item{}
	if enabled {
		for range recycleOilBrowserMaxAttempts {
			candidate := newSword()
			candidate.CurrentLuck, candidate.LuckOilToSix, candidate.LuckOilToNine = 9, 100, 3000
			attempts = append(attempts, candidate)
		}
		items = append(items, attempts...)
	}
	change, err := store.DB.Collection("roles").UpdateByID(ctx, roleID, bson.M{"$set": bson.M{
		"capacity": len(items), "items": items, "equipped": []domain.Equipped{},
	}})
	must(t, err)
	if change.ModifiedCount != 1 {
		t.Fatal("预置隔离角色的原始背包失败")
	}
	return preview, attempts, definition
}

// verifyRecycleOilBrowser 独立核对每次触屏确认、真实随机区间、审计、角色钱包及剩余库存。
func verifyRecycleOilBrowser(t *testing.T, ctx context.Context, store *persistence.Store, evidence string, before domain.Role,
	preview domain.Item, candidates []domain.Item, definition devconfig.Item, fixture *devconfig.Fixture, bundleHash string) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(evidence, "recycle-oil-browser-report.json"))
	must(t, err)
	var report struct {
		Passed, Enabled, Cancelled, DefaultPreviewHidden bool
		FixtureSha256, ClientBundleSha256                string
		Attempts                                         []struct {
			Op, Instance, Oil string
			Luck              uint32
		}
	}
	must(t, json.Unmarshal(raw, &report))
	enabled := fixture.EquipmentRecycleOil != nil
	if !report.Passed || report.Enabled != enabled || !report.Cancelled || report.FixtureSha256 != fixture.Hash || report.ClientBundleSha256 != bundleHash {
		t.Fatal("浏览器报告上的验收模式、取消操作或实包摘要不符")
	}
	after, err := store.Role(ctx, before.ID)
	must(t, err)
	if !enabled {
		if !report.DefaultPreviewHidden || len(report.Attempts) != 0 || after.EconomicSeq != before.EconomicSeq || after.Wallet != before.Wallet ||
			!slices.Equal(after.Items, before.Items) || !slices.Equal(after.Equipped, before.Equipped) {
			t.Fatal("默认包没有保持原分解预览或取消后修改了资产")
		}
		return
	}
	if len(report.Attempts) == 0 || len(report.Attempts) > len(candidates) {
		t.Fatal("触屏消耗的原装备次数超出隔离背包限制", len(report.Attempts))
	}
	var awarded domain.Item
	for i, attempt := range report.Attempts {
		candidate := candidates[i]
		if len(attempt.Op) != 32 || attempt.Instance != candidate.ID || attempt.Luck != 9 {
			t.Fatal("只允许按逐件真实触屏操作的原物品身份核账", attempt)
		}
		var operation persistence.Operation
		id := "asset:" + before.ID + ":" + attempt.Op
		must(t, store.DB.Collection("operations").FindOne(ctx, bson.M{"_id": id, "role_id": before.ID}).Decode(&operation))
		recycling := operation.Recycling
		if operation.Kind != "recycle" || operation.Code != 0 || operation.CommandSeq != before.EconomicSeq+int64(i)+1 || recycling == nil ||
			!slices.Equal(recycling.Recycled, []domain.Item{candidate}) || len(recycling.OilRolls) != 1 ||
			recycling.Gold != 0 || recycling.BoundGold != definition.RecycleGold {
			t.Fatal("实际确认的原分解、绑定金币和已消耗原件不符", operation)
		}
		var audit struct {
			Recycling *persistence.EquipmentRecyclingReceipt `bson:"recycling"`
		}
		must(t, store.DB.Collection("audit_outbox").FindOne(ctx, bson.M{"_id": id, "kind": "recycle"}).Decode(&audit))
		if !reflect.DeepEqual(audit.Recycling, recycling) {
			t.Fatal("装备分解原回执没有同事务进入审计", attempt)
		}
		roll := recycling.OilRolls[0]
		if roll.EquipmentInstanceID != candidate.ID || roll.Luck != 9 || roll.Denominator != 30 || roll.Draw >= roll.Denominator ||
			(roll.Draw < roll.Luck) != (roll.Oil != nil) || (i == len(report.Attempts)-1) != (roll.Oil != nil) {
			t.Fatal("实包没有保留分母30的实际抽样，或命中后继续销毁装备", roll)
		}
		if roll.Oil != nil {
			awarded = *roll.Oil
			if awarded.ID != attempt.Oil || awarded.Bound != candidate.Bound || awarded.ItemID != fixture.LuckOil.ItemID || awarded.Quantity != 1 ||
				awarded.Version != 1 || awarded.SourceKind != "equipment_recycle" || awarded.SourceID != candidate.ID {
				t.Fatal("查看的祝福油不是本次真实绑定的原产物", awarded)
			}
		} else if attempt.Oil != "" {
			t.Fatal("实包出现未确认的祝福油", attempt)
		}
	}
	count, err := store.DB.Collection("operations").CountDocuments(ctx, bson.M{"kind": "recycle", "role_id": before.ID})
	must(t, err)
	wantItems := append([]domain.Item{preview}, candidates[len(report.Attempts):]...)
	wantItems = append(wantItems, awarded)
	wantWallet := before.Wallet
	wantWallet.BoundGold += int64(len(report.Attempts)) * definition.RecycleGold
	if count != int64(len(report.Attempts)) || after.EconomicSeq != before.EconomicSeq+count || after.SourceSeq != before.SourceSeq ||
		after.Wallet != wantWallet || !slices.Equal(after.Items, wantItems) || !slices.Equal(after.Equipped, before.Equipped) {
		t.Fatal("取消或查看原油额外消耗资产，或剩余资产与逐件账本不守恒", after.Items, wantItems)
	}
	t.Logf("按真实分母30确认 %d 件后首次获得原绑定油，剩余武器 %d 件；逐件永久回执、原油及审计一致", len(report.Attempts), len(after.Items)-1)
}
