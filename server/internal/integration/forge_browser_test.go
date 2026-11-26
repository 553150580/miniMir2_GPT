//go:build creator_forge

// 本文件用独立 Creator 实包、真实 HTTP/WebSocket 和 MongoDB 核对打造触屏路径。
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

// TestForgeBrowserMongoWS 依据原实包触屏确认，核对两个钱包、原选材和唯一永久成品。
func TestForgeBrowserMongoWS(t *testing.T) {
	webRoot, mode := os.Getenv("MINIMIR_FORGE_BROWSER_WEB_ROOT"), os.Getenv("MINIMIR_FORGE_BROWSER_MODE")
	if webRoot == "" || os.Getenv("MINIMIR_MONGO_E2E") != "1" || os.Getenv("MINIMIR_INTEGRATION_TEST_MONGO_URI") == "" {
		t.Skip("触屏打造需要独立 Creator Web Mobile 与隔离 MongoDB")
	}
	if mode != "forge" && mode != "default" {
		t.Fatal("浏览器验收必须指定独立打造包或原默认包")
	}
	repo, err := filepath.Abs("../../..")
	must(t, err)
	work := filepath.Join(repo, ".work")
	build := forgeBrowserInput(t, work, webRoot)
	bundle, err := os.ReadFile(filepath.Join(build, "assets", "main", "index.js"))
	must(t, err)
	bundleDigest := sha256.Sum256(bundle)
	fixtureFile := filepath.Join(repo, "config", "development", "first-loop.json")
	enabled := mode == "forge"
	if enabled {
		fixtureFile = forgeBrowserInput(t, work, os.Getenv("MINIMIR_FORGE_BROWSER_FIXTURE"))
	}
	payload, err := os.ReadFile(fixtureFile)
	must(t, err)
	var fixture devconfig.Fixture
	must(t, json.Unmarshal(payload, &fixture))
	digest := sha256.Sum256(payload)
	fixture.Hash = hex.EncodeToString(digest[:])
	must(t, fixture.Validate())
	if (fixture.Forge != nil) != enabled {
		t.Fatal("实包模式与服务端图纸打造开关不一致")
	}
	ctx, store, account, route := startChallengePlayerDeathServer(t, func(cfg *devconfig.Config) {
		cfg.Fixture, cfg.FixtureFile = fixture, fixtureFile
	})
	login := &pb.LoginResult{}
	name, password := "forgetouch_"+domain.ID()[:9], domain.ID()[:16]
	rpc(t, account, "", p.MsgC2SLocalRegister, &pb.LocalLoginRequest{Account: name, Password: password}, login, 0)
	role := createDirectoryRole(t, account, login.AccountToken, route.origin.OriginID, "触屏打造")
	roleID := domain.Hex(role.Key.RoleId)
	metadata := map[string]any{"enabled": enabled, "role": role.Name, "region": route.origin.RegionName, "server": route.origin.Name,
		"endpoint": route.assignment.GameWSURL, "normalMap": fixture.Map.ID, "ruleVersion": fixture.RuleVersion,
		"fixtureSha256": fixture.Hash, "clientBundleSha256": hex.EncodeToString(bundleDigest[:])}
	var original forgeBrowserInputs
	if enabled {
		original = seedForgeBrowserAssets(t, ctx, store, roleID, &fixture)
		metadata["blueprint"], metadata["ores"], metadata["extra"] = original.blueprint, original.ores, original.extra
		metadata["output"] = map[string]any{"itemId": original.output.ID, "name": original.output.Name,
			"quality": original.recipe.Quality, "slot": original.output.EquipmentSlot, "requiredLevel": *original.output.RequiredLevel}
		metadata["price"] = map[string]string{"gold": "3000", "boundGold": "27000"}
		metadata["walletAfter"] = map[string]string{"gold": "9000", "boundGold": "0", "yuanbao": "0", "boundYuanbao": "0"}
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
	evidence, err := os.MkdirTemp(work, "forge-browser-")
	must(t, err)
	manifest := filepath.Join(evidence, "manifest.json")
	raw, err := json.Marshal(metadata)
	must(t, err)
	must(t, os.WriteFile(manifest, raw, 0600))
	t.Log("打造触屏验收目录 " + evidence)
	browserCtx, stop := context.WithTimeout(ctx, 190*time.Second)
	defer stop()
	node := exec.CommandContext(browserCtx, "node", filepath.Join(repo, "tools/client/verify_forge_browser.cjs"), manifest)
	node.Dir = repo
	node.Env = append(os.Environ(), "MINIMIR_FORGE_BROWSER_ACCOUNT="+name, "MINIMIR_FORGE_BROWSER_PASSWORD="+password)
	log, err := node.CombinedOutput()
	if err != nil {
		t.Fatalf("打造触屏未完成：%v\n%s", err, log)
	}
	t.Logf("%s", log)
	verifyForgeBrowserAssets(t, ctx, store, evidence, before, original, enabled)
}

// forgeBrowserInputs 固定测试前态，触屏必须逐件选择这些实际原矿。
type forgeBrowserInputs struct {
	blueprint domain.Item           // 已领取原来源身份的隔离图纸。
	ores      []domain.Item         // 预置明确 95 以上平均纯度的绑定黑铁。
	extra     domain.Item           // 保持未消费的原背包旁证。
	output    devconfig.Item        // 原候选装备定义。
	recipe    devconfig.ForgeRecipe // 原确切金币与黑铁件数。
}

func seedForgeBrowserAssets(t *testing.T, ctx context.Context, store *persistence.Store, roleID string, fixture *devconfig.Fixture) forgeBrowserInputs {
	t.Helper()
	if len(fixture.Forge.Recipes) != 1 || fixture.Forge.Recipes[0].OreCount != 4 || fixture.Forge.Recipes[0].Gold != 30_000 {
		t.Fatal("本轮触屏隔离输入需要原配方的四块黑铁和 30,000 金币")
	}
	recipe := fixture.Forge.Recipes[0]
	output, found := fixture.Item(recipe.OutputItemID)
	if !found || output.RequiredLevel == nil {
		t.Fatal("触屏缺少候选成品模板", recipe)
	}
	blueprint := domain.Item{ID: domain.ID(), ItemID: recipe.BlueprintItemID, Quantity: 1, Version: 3,
		Bound: true, Quality: recipe.Quality, SourceKind: "personal_boss", SourceID: domain.ID()}
	newOre := func(purity uint32, bound bool) domain.Item {
		return domain.Item{ID: domain.ID(), ItemID: fixture.Forge.OreItemID, Quantity: 1, Version: int64(purity),
			OrePurity: purity, Bound: bound, SourceKind: "mining", SourceID: domain.ID()}
	}
	ores := []domain.Item{newOre(94, true), newOre(95, true), newOre(99, true), newOre(100, true)}
	extra := newOre(20, false)
	items := append([]domain.Item{blueprint}, ores...)
	items = append(items, extra)
	change, err := store.DB.Collection("roles").UpdateByID(ctx, roleID, bson.M{
		"$set": bson.M{"level": *output.RequiredLevel, "wallet": domain.Wallet{Gold: 12_000, BoundGold: 27_000}, "items": items},
	})
	must(t, err)
	if change.ModifiedCount != 1 {
		t.Fatal("未给独立触屏角色安装明确的图纸、原矿和钱包前态")
	}
	return forgeBrowserInputs{blueprint: blueprint, ores: ores, extra: extra, output: output, recipe: recipe}
}

// forgeBrowserInput 解析真实路径并拒绝默认试玩静态目录或项目外文件。
func forgeBrowserInput(t *testing.T, work, supplied string) string {
	t.Helper()
	resolved, err := filepath.EvalSymlinks(supplied)
	must(t, err)
	resolved, err = filepath.Abs(resolved)
	must(t, err)
	relative, err := filepath.Rel(work, resolved)
	must(t, err)
	if relative == "." || !filepath.IsLocal(relative) {
		t.Fatal("只允许独立验收的客户端与夹具")
	}
	return resolved
}

// verifyForgeBrowserAssets 用真实持久回执独立核对浏览器报告和所有变动资产。
func verifyForgeBrowserAssets(t *testing.T, ctx context.Context, store *persistence.Store, evidence string,
	before domain.Role, original forgeBrowserInputs, enabled bool) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(evidence, "forge-browser-report.json"))
	must(t, err)
	var report struct {
		Passed   bool     `json:"passed"`
		Enabled  bool     `json:"enabled"`
		Canceled []string `json:"canceled"`
		Selected []string `json:"selected"`
		Commit   struct {
			OpID    string `json:"op"`
			QuoteID string `json:"quote"`
		} `json:"commit"`
		Result struct {
			Instance string `json:"instance"`
			Luck     uint32 `json:"luck"`
		} `json:"result"`
	}
	must(t, json.Unmarshal(raw, &report))
	if !report.Passed || report.Enabled != enabled {
		t.Fatal("触屏报告未通过或不符合独立实包模式")
	}
	after, err := store.Role(ctx, before.ID)
	must(t, err)
	if !enabled {
		if report.Commit.OpID != "" || after.EconomicSeq != before.EconomicSeq || after.Wallet != before.Wallet || !slices.Equal(after.Items, before.Items) {
			t.Fatal("默认实包不应制造打造账、扣费或修改背包")
		}
		return
	}
	if !slices.Equal(report.Canceled, []string{"first", "second"}) || len(report.Selected) != len(original.ores) || len(report.Commit.OpID) != 32 {
		t.Fatal("两个确认取消、原件选择或操作身份缺少触屏证据")
	}
	for i, ore := range original.ores {
		if report.Selected[i] != ore.ID {
			t.Fatal("触屏未选择顺序一致的原矿", i, report.Selected[i])
		}
	}
	id := "asset:" + after.ID + ":" + report.Commit.OpID
	var operation persistence.Operation
	must(t, store.DB.Collection("operations").FindOne(ctx, bson.M{"_id": id, "role_id": after.ID}).Decode(&operation))
	receipt := operation.Forge
	wantSpend := domain.Wallet{Gold: 3000, BoundGold: 27_000}
	if operation.Kind != persistence.AssetForgeCommit || operation.Code != 0 || receipt == nil || !receipt.Success || receipt.QuoteID != report.Commit.QuoteID ||
		receipt.Plan.Blueprint != original.blueprint || !slices.Equal(receipt.Plan.Ores, original.ores) || !receipt.Plan.AllowBound ||
		receipt.Plan.Spend != wantSpend || receipt.Plan.SuccessPPM != 1_000_000 || !receipt.Plan.OutputBound || receipt.Output == nil {
		t.Fatal("永久回执与原报价、投入、绑定及保底成功条件不一致", operation)
	}
	var audit struct {
		Kind  string                          `bson:"kind"`
		Forge *persistence.ForgeResultReceipt `bson:"forge"`
	}
	must(t, store.DB.Collection("audit_outbox").FindOne(ctx, bson.M{"_id": id, "role_id": after.ID}).Decode(&audit))
	if audit.Kind != operation.Kind || !reflect.DeepEqual(audit.Forge, receipt) {
		t.Fatal("原打造审计与永久回执不一致")
	}
	output := *receipt.Output
	if output.ID != report.Result.Instance || output.ItemID != original.output.ID || output.SourceKind != "forge_craft" || output.SourceID != receipt.QuoteID ||
		output.Quality != original.recipe.Quality || !output.Bound || output.InitialLuck != report.Result.Luck || output.InitialLuck > 3 ||
		output.DurabilityU != original.output.OriginalDurabilityU || output.MaxDurabilityU != output.DurabilityU || output.OriginalDurabilityU != output.DurabilityU {
		t.Fatal("浏览器成品与真实打造结果、出处、绑定、品质、初始幸运、耐久不一致", output)
	}
	wantWallet := before.Wallet
	wantWallet.Gold -= wantSpend.Gold
	wantWallet.BoundGold -= wantSpend.BoundGold
	if !slices.Equal(after.Items, []domain.Item{original.extra, output}) || after.EconomicSeq != before.EconomicSeq+1 ||
		after.SourceSeq != before.SourceSeq || after.Wallet != wantWallet {
		t.Fatal("原投入、未选黑铁、成品、水位或钱包最终对账不一致", after.Items, after.Wallet)
	}
}
