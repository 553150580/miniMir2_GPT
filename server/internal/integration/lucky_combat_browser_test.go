//go:build creator_combat_luck

// 本文件以独立 Creator 实包、真实 WebSocket 和 MongoDB 核验原装备入口的幸运属性。
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
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
)

// TestLuckyCombatBrowserMongoWS 核验角色战斗属性既能显示真实穿戴变化，也能在默认规则关闭时隐藏。
func TestLuckyCombatBrowserMongoWS(t *testing.T) {
	webRoot, mode := os.Getenv("MINIMIR_LUCK_BROWSER_WEB_ROOT"), os.Getenv("MINIMIR_LUCK_BROWSER_MODE")
	if webRoot == "" || os.Getenv("MINIMIR_MONGO_E2E") != "1" || os.Getenv("MINIMIR_INTEGRATION_TEST_MONGO_URI") == "" {
		t.Skip("幸运触屏验收需要独立 Creator Web Mobile 与隔离 MongoDB")
	}
	if mode != "luck" && mode != "default" {
		t.Fatal("必须指定幸运候选模式或默认配置模式")
	}
	repo, err := filepath.Abs("../../..")
	must(t, err)
	work := filepath.Join(repo, ".work")
	build := luckyCombatBrowserInput(t, work, webRoot)
	bundle, err := os.ReadFile(filepath.Join(build, "assets", "main", "index.js"))
	must(t, err)
	bundleDigest := sha256.Sum256(bundle)
	bundleHash := hex.EncodeToString(bundleDigest[:])
	fixtureFile := filepath.Join(repo, "config", "development", "first-loop.json")
	enabled := mode == "luck"
	if enabled {
		fixtureFile = luckyCombatBrowserInput(t, work, os.Getenv("MINIMIR_LUCK_BROWSER_FIXTURE"))
	}
	raw, err := os.ReadFile(fixtureFile)
	must(t, err)
	digest := sha256.Sum256(raw)
	var fixture devconfig.Fixture
	must(t, json.Unmarshal(raw, &fixture))
	fixture.Hash = hex.EncodeToString(digest[:])
	must(t, fixture.Validate())
	if (fixture.CombatLuck != nil) != enabled {
		t.Fatal("实包验收模式与服务端幸运配置不一致")
	}

	ctx, store, account, route := startChallengePlayerDeathServer(t, func(cfg *devconfig.Config) {
		cfg.Fixture, cfg.FixtureFile = fixture, fixtureFile
	})
	login, role := &pb.LoginResult{}, &pb.RoleSummary{}
	name, password := "lucktouch_"+domain.ID()[:9], domain.ID()[:16]
	rpc(t, account, "", p.MsgC2SLocalRegister, &pb.LocalLoginRequest{Account: name, Password: password}, login, 0)
	rpc(t, account, login.AccountToken, p.MsgC2SCreateRole, &pb.CreateRoleRequest{
		OriginServerId: route.origin.OriginID, Slot: 1, Job: 1, Name: "触屏幸运" + domain.ID()[:5], OpId: domain.Bytes(domain.ID()),
	}, role, 0)
	roleID := domain.Hex(role.Key.RoleId)
	metadata := map[string]any{"enabled": enabled, "role": role.Name, "region": route.origin.RegionName, "server": route.origin.Name,
		"endpoint": route.assignment.GameWSURL, "normalMap": fixture.Map.ID, "ruleVersion": fixture.RuleVersion,
		"fixtureSha256": fixture.Hash, "clientBundleSha256": bundleHash}
	var necklace domain.Equipped
	if enabled {
		necklace = seedLuckyCombatBrowserNecklace(t, ctx, store, roleID, &fixture)
		metadata["item"] = necklace
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
	evidence, err := os.MkdirTemp(work, "luck-browser-")
	must(t, err)
	manifest := filepath.Join(evidence, "manifest.json")
	raw, err = json.Marshal(metadata)
	must(t, err)
	must(t, os.WriteFile(manifest, raw, 0600))
	t.Log("幸运触屏验收目录 " + evidence)
	browserCtx, stop := context.WithTimeout(ctx, 190*time.Second)
	defer stop()
	node := exec.CommandContext(browserCtx, "node", filepath.Join(repo, "tools/client/verify_combat_luck_browser.cjs"), manifest)
	node.Dir = repo
	node.Env = append(os.Environ(), "MINIMIR_LUCK_BROWSER_ACCOUNT="+name, "MINIMIR_LUCK_BROWSER_PASSWORD="+password)
	log, err := node.CombinedOutput()
	if err != nil {
		t.Fatalf("幸运触屏验收失败：%v\n%s", err, log)
	}
	t.Logf("%s", log)
	verifyLuckyCombatBrowserAssets(t, ctx, store, evidence, before, necklace, enabled, fixture.Hash, bundleHash)
}

// seedLuckyCombatBrowserNecklace 只为测试新角预置一个绑定幸运项链，正常穿戴从触屏触发。
func seedLuckyCombatBrowserNecklace(t *testing.T, ctx context.Context, store *persistence.Store, roleID string, fixture *devconfig.Fixture) domain.Equipped {
	t.Helper()
	if fixture.CombatLuck.RoleResistancePPM[1] != 200_000 || fixture.CombatLuck.OtherPPM[3] != 44_500 ||
		fixture.Forge == nil || len(fixture.Forge.Recipes) != 1 {
		t.Fatal("幸运触屏验收缺少 20% 抵抗、4.45% 非武器幸运和原图纸候选")
	}
	recipe := fixture.Forge.Recipes[0]
	definition, found := fixture.Item(recipe.OutputItemID)
	if !found || definition.RequiredLevel == nil || definition.EquipmentSlot != 3 || !slices.Contains(definition.AllowedJobs, uint32(1)) {
		t.Fatal("幸运触屏验收缺少符合职业的原项链定义", recipe)
	}
	item := domain.Item{ID: domain.ID(), ItemID: definition.ID, Version: 3, Quantity: 1,
		Bound: true, Quality: recipe.Quality, SourceKind: "forge_craft", SourceID: domain.ID(), InitialLuck: 3,
		DurabilityU: definition.OriginalDurabilityU, MaxDurabilityU: definition.OriginalDurabilityU, OriginalDurabilityU: definition.OriginalDurabilityU}
	change, err := store.DB.Collection("roles").UpdateByID(ctx, roleID, bson.M{"$set": bson.M{
		"level": *definition.RequiredLevel, "items": []domain.Item{item}, "equipped": []domain.Equipped{},
	}})
	must(t, err)
	if change.ModifiedCount != 1 {
		t.Fatal("未给隔离角色预置候选幸运项链")
	}
	return domain.Equipped{Slot: definition.EquipmentSlot, Item: item}
}

// luckyCombatBrowserInput 只接受工作目录内的独立构建和候选夹具，避免连接原试玩。
func luckyCombatBrowserInput(t *testing.T, work, supplied string) string {
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
		t.Fatal("只允许 .work 内的独立验收实包和夹具")
	}
	return resolved
}

// verifyLuckyCombatBrowserAssets 独立复核浏览器观察的原穿戴操作与实际持久装备。
func verifyLuckyCombatBrowserAssets(t *testing.T, ctx context.Context, store *persistence.Store, evidence string,
	before domain.Role, necklace domain.Equipped, enabled bool, fixtureHash, bundleHash string) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(evidence, "luck-browser-report.json"))
	must(t, err)
	var report struct {
		Passed             bool   `json:"passed"`             // 真实触屏断言全部通过。
		Enabled            bool   `json:"enabled"`            // 幸运规则模式。
		FixtureSha256      string `json:"fixtureSha256"`      // 参与验收的服务端配置摘要。
		ClientBundleSha256 string `json:"clientBundleSha256"` // 参与验收的客户端主包摘要。
		Equip              struct {
			OpID     string `json:"op"`       // 真实穿戴命令和回执的操作标识。
			Instance string `json:"instance"` // 背包内的原项链实例。
			Slot     uint32 `json:"slot"`     // 客户端申请穿戴的槽位。
		} `json:"equip"`
	}
	must(t, json.Unmarshal(raw, &report))
	if !report.Passed || report.Enabled != enabled || report.FixtureSha256 != fixtureHash || report.ClientBundleSha256 != bundleHash {
		t.Fatal("幸运触屏报告或实包摘要不匹配")
	}
	after, err := store.Role(ctx, before.ID)
	must(t, err)
	if !enabled {
		if report.Equip.OpID != "" || after.EconomicSeq != before.EconomicSeq || after.Wallet != before.Wallet ||
			!slices.Equal(after.Items, before.Items) || !slices.Equal(after.Equipped, before.Equipped) {
			t.Fatal("打开默认规则战斗属性时出现资产操作或状态变化")
		}
		return
	}
	if len(report.Equip.OpID) != 32 || report.Equip.Instance != necklace.Item.ID || report.Equip.Slot != necklace.Slot {
		t.Fatal("原项链穿戴请求与触屏观察不一致", report.Equip)
	}
	original := necklace.Item
	necklace.Item.Version++
	if after.EconomicSeq != before.EconomicSeq+1 || after.Wallet != before.Wallet ||
		!slices.Equal(after.Equipped, []domain.Equipped{necklace}) || len(after.Items) != 0 {
		t.Fatal("原项链穿戴资产、版本或经济水位不一致", after.Equipped, after.Items)
	}
	var operation persistence.Operation
	must(t, store.DB.Collection("operations").FindOne(ctx, bson.M{
		"_id": "asset:" + before.ID + ":" + report.Equip.OpID, "role_id": before.ID,
	}).Decode(&operation))
	receipt := operation.Equipment
	if operation.Kind != "equip" || operation.Code != 0 || receipt == nil || receipt.Slot != necklace.Slot ||
		receipt.Unequip || receipt.ConfigHash != fixtureHash || len(receipt.Moves) != 1 ||
		receipt.Moves[0].Before != original || receipt.Moves[0].After != necklace.Item {
		t.Fatal("原项链的真实永久穿戴回执不一致", operation)
	}
}
