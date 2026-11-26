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
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	p "minimir/internal/protocol"
)

// TestMiningBrowserMongoWS 用已装独立前态的角色测试打开矿区、采集、停止、背包纯度和真实返回。
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
	metadata := map[string]any{
		"enabled": enabled, "normalMap": fixture.Map.ID, "ruleVersion": fixture.RuleVersion, "miner": role.Name,
		"region": route.origin.RegionName, "server": route.origin.Name, "endpoint": route.assignment.GameWSURL,
		"fixtureSha256": fixture.Hash, "clientBundleSha256": hex.EncodeToString(bundleDigest[:]),
	}
	if enabled {
		first, deep := fixture.Mining.Layers[0], fixture.Mining.Layers[1]
		definition := fixture.Mining.Tools[0]
		tool = domain.Item{ID: domain.ID(), ItemID: definition.ItemID, Quantity: 1, Version: 1,
			DurabilityU: definition.DurabilityU, MaxDurabilityU: definition.DurabilityU, OriginalDurabilityU: definition.DurabilityU,
			SourceKind: "npc_shop", SourceID: domain.ID()}
		// 真实商店购买由联机专项验收；此处只准备隔离适龄角色和一把合法工具，用触屏验证采矿。
		change, err := store.DB.Collection("roles").UpdateByID(ctx, roleID, bson.M{
			"$set": bson.M{"level": first.MinimumLevel}, "$push": bson.M{"items": tool},
		})
		must(t, err)
		if change.ModifiedCount != 1 {
			t.Fatal("没有准备隔离触屏角色工具")
		}
		metadata["mineMap"], metadata["deepMap"], metadata["toolId"], metadata["toolInstance"] = first.Map.ID, deep.Map.ID, tool.ItemID, tool.ID
		metadata["toolFull"], metadata["toolWear"], metadata["minLevel"], metadata["deepLevel"] = tool.DurabilityU, fixture.Mining.ToolWearU, first.MinimumLevel, deep.MinimumLevel
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
		verifyMiningBrowserAssets(t, evidence, before, tool, fixture.Mining.ToolWearU, func() domain.Role {
			after, err := store.Role(ctx, roleID)
			must(t, err)
			return after
		})
	}
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

// verifyMiningBrowserAssets 对账同一工具的持久、实际出矿原实例与整轮来源序号。
func verifyMiningBrowserAssets(t *testing.T, evidence string, before domain.Role, tool domain.Item, wear int64, latest func() domain.Role) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(evidence, "mining-browser-report.json"))
	must(t, err)
	var report struct {
		Stopped struct {
			Attempts int64 `json:"attempts"`
		} `json:"stopped"`
		Ore struct {
			Instance string `json:"instance"`
			Purity   uint32 `json:"purity"`
			Bound    bool   `json:"bound"`
		} `json:"ore"`
	}
	must(t, json.Unmarshal(raw, &report))
	after := latest()
	var storedTool, storedOre domain.Item
	for _, item := range after.Items {
		if item.ID == tool.ID {
			storedTool = item
		}
		if item.ID == report.Ore.Instance {
			storedOre = item
		}
	}
	if report.Stopped.Attempts < 1 || storedTool.DurabilityU != tool.DurabilityU-report.Stopped.Attempts*wear ||
		storedOre.OrePurity != report.Ore.Purity || !storedOre.Bound || !report.Ore.Bound || after.Wallet != before.Wallet ||
		after.EconomicSeq != before.EconomicSeq || after.SourceSeq != before.SourceSeq+report.Stopped.Attempts {
		t.Fatal("触屏界面矿石、纯度、绑态或锄头耐久不能与实际原账户对账", storedTool, storedOre)
	}
}
