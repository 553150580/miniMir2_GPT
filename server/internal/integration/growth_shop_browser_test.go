//go:build creator_growth_shop

// 本文件将真实商城触屏回执与独立 MongoDB 角色资产逐笔对账。
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
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"minimir/internal/domain"
	"minimir/internal/persistence"
)

// TestGrowthShopCreatorTouchMongo 实测隔离购买和默认入口关闭，钱包按原回执对账。
func TestGrowthShopCreatorTouchMongo(t *testing.T) {
	root, mode := os.Getenv("MINIMIR_GROWTH_SHOP_BROWSER_WEB_ROOT"), os.Getenv("MINIMIR_GROWTH_SHOP_BROWSER_MODE")
	if root == "" {
		t.Skip("需要已构建的 Creator 隔离或默认实包")
	}
	if mode != "candidate" && mode != "default" {
		t.Fatal("商城验收须明确隔离或默认模式")
	}
	enabled := mode == "candidate"
	repo, err := filepath.Abs("../../..")
	must(t, err)
	web, err := creatorBrowserBuildPath(repo, root)
	must(t, err)
	bundle, err := os.ReadFile(filepath.Join(web, "assets", "main", "index.js"))
	must(t, err)
	bundleHash := persistence.Hash(bundle)
	trial := startGrowthShopTrial(t, enabled)
	wallet := domain.Wallet{Gold: 500, BoundGold: 25, Yuanbao: 1300, BoundYuanbao: 500}
	player := createGrowthShopPlayer(t, trial, "商城触屏", wallet, false)
	upstream, err := url.Parse(trial.endpoint)
	must(t, err)
	upstream.Path = ""
	mux := http.NewServeMux()
	mux.Handle("/account/rpc", http.StripPrefix("/account", httputil.NewSingleHostReverseProxy(upstream)))
	mux.Handle("/", http.FileServer(http.Dir(web)))
	browser := httptest.NewServer(mux)
	defer browser.Close()
	notes, err := os.MkdirTemp(filepath.Join(repo, ".work"), "growth-shop-browser-")
	must(t, err)
	t.Log("商城触屏验收目录 " + notes)
	fixtureHash := trial.store.Cfg.Fixture.Hash
	manifest := map[string]any{"baseUrl": browser.URL, "endpoint": trial.route.assignment.GameWSURL, "role": player.summary.Name,
		"region": trial.route.origin.RegionName, "server": trial.route.origin.Name, "enabled": enabled,
		"fixtureSha256": fixtureHash, "bundleSha256": bundleHash}
	raw, err := json.Marshal(manifest)
	must(t, err)
	input := filepath.Join(notes, "manifest.json")
	must(t, os.WriteFile(input, raw, 0600))
	ctx, cancel := context.WithTimeout(trial.ctx, 180*time.Second)
	defer cancel()
	node := exec.CommandContext(ctx, "node", filepath.Join(repo, "tools/client/verify_growth_shop_browser.cjs"), input)
	node.Dir = repo
	node.Env = append(os.Environ(), "MINIMIR_GROWTH_SHOP_BROWSER_ACCOUNT="+player.account, "MINIMIR_GROWTH_SHOP_BROWSER_PASSWORD="+player.password)
	output, err := node.CombinedOutput()
	if err != nil {
		t.Fatalf("商城触屏验收失败 %v\n%s", err, output)
	}
	raw, err = os.ReadFile(filepath.Join(notes, "growth-shop-browser-report.json"))
	must(t, err)
	var proof struct {
		Passed, Enabled             bool   // 触屏用例和目录的实际状态。
		FixtureSha256, BundleSha256 string // 运行夹具和浏览器实际下载的包。
		Receipts                    []struct {
			Operation string // 客户端收到的原经济操作编号。
			Quote     string // 服务端保存的原购买条款身份。
			Product   uint32 // 实际使用的工作簿商品编号。
			Total     int64  `json:",string"` // 浏览器收到的实际扣款。
		}
	}
	must(t, json.Unmarshal(raw, &proof))
	if !proof.Passed || proof.Enabled != enabled || proof.FixtureSha256 != fixtureHash || proof.BundleSha256 != bundleHash {
		t.Fatal("触屏报告未对应实际夹具与下载的实包", proof)
	}
	roleID := domain.Hex(player.summary.Key.RoleId)
	role, err := trial.store.Role(trial.ctx, roleID)
	must(t, err)
	if !enabled {
		if len(proof.Receipts) != 0 || role.GrowthShop != nil || role.Wallet != wallet {
			t.Fatal("默认包写入了隔离商城数据", role.GrowthShop, role.Wallet)
		}
		return
	}
	for _, outcome := range proof.Receipts {
		var op persistence.Operation
		must(t, trial.store.DB.Collection("operations").FindOne(trial.ctx,
			bson.M{"_id": "asset:" + roleID + ":" + outcome.Operation}).Decode(&op))
		if op.GrowthShop == nil || op.Kind != persistence.AssetGrowthShopCommit || op.Code != 0 || op.GrowthShop.Quote.ID != outcome.Quote ||
			op.GrowthShop.Quote.Terms.ProductID != outcome.Product || op.GrowthShop.Quote.Terms.Total != outcome.Total {
			t.Fatal("触屏一次确认与 MongoDB 原购买事实不同", outcome)
		}
	}
	if len(proof.Receipts) != 2 || role.GrowthShop == nil || len(role.GrowthShop.Counts) != 2 ||
		role.Wallet != (domain.Wallet{Gold: 500, BoundGold: 25, Yuanbao: 300, BoundYuanbao: 300}) {
		t.Fatal("两种币种购买后的实际限额与钱包未对账", role.GrowthShop, role.Wallet)
	}
}
