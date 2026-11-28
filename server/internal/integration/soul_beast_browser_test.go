//go:build creator_soul_beast

// 本文件用 Creator 构建的真实触屏操作核对魂兽的 MongoDB 收藏及原回执。
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

// TestSoulBeastCreatorTouchMongo 用测试碎片打开名录，其余操作均由触屏使用线上协议完成。
func TestSoulBeastCreatorTouchMongo(t *testing.T) {
	webRoot := os.Getenv("MINIMIR_SOUL_BROWSER_WEB_ROOT")
	if webRoot == "" {
		t.Skip("需要已构建的魂兽 Creator 隔离包")
	}
	repo, err := filepath.Abs("../../..")
	must(t, err)
	build, err := creatorBrowserBuildPath(repo, webRoot)
	must(t, err)
	entry, err := os.ReadFile(filepath.Join(build, "assets", "main", "index.js"))
	must(t, err)
	bundleHash := persistence.Hash(entry)
	trial := startSoulTrial(t, nil)
	player := trial.create(t, "魂兽触屏", time.Now().Add(-time.Hour), false)
	roleID := domain.Hex(player.summary.Key.RoleId)
	update, err := trial.store.DB.Collection("roles").UpdateByID(trial.ctx, roleID,
		bson.M{"$set": bson.M{"soul_collection": domain.SoulCollection{Fragments: 30}}})
	must(t, err)
	if update.ModifiedCount != 1 {
		t.Fatal("没有建立隔离测试碎片余额")
	}
	upstream, err := url.Parse(trial.endpoint)
	must(t, err)
	upstream.Path = ""
	mux := http.NewServeMux()
	mux.Handle("/account/rpc", http.StripPrefix("/account", httputil.NewSingleHostReverseProxy(upstream)))
	mux.Handle("/", http.FileServer(http.Dir(build)))
	web := httptest.NewServer(mux)
	defer web.Close()
	evidence, err := os.MkdirTemp(filepath.Join(repo, ".work"), "soul-beast-browser-")
	must(t, err)
	t.Log("魂兽触屏验收目录 " + evidence)
	fixtureHash := trial.store.Cfg.Fixture.Hash
	manifest := map[string]any{"baseUrl": web.URL, "endpoint": trial.route.assignment.GameWSURL, "role": player.summary.Name,
		"region": trial.route.origin.RegionName, "server": trial.route.origin.Name,
		"fixtureSha256": fixtureHash, "bundleSha256": bundleHash}
	raw, err := json.Marshal(manifest)
	must(t, err)
	input := filepath.Join(evidence, "manifest.json")
	must(t, os.WriteFile(input, raw, 0600))
	ctx, cancel := context.WithTimeout(trial.ctx, 180*time.Second)
	defer cancel()
	node := exec.CommandContext(ctx, "node", filepath.Join(repo, "tools/client/verify_soul_beast_browser.cjs"), input)
	node.Dir = repo
	node.Env = append(os.Environ(), "MINIMIR_SOUL_BROWSER_ACCOUNT="+player.account, "MINIMIR_SOUL_BROWSER_PASSWORD="+player.password)
	output, err := node.CombinedOutput()
	if err != nil {
		t.Fatalf("魂兽触屏验收失败 %v\n%s", err, output)
	}
	raw, err = os.ReadFile(filepath.Join(evidence, "soul-beast-browser-report.json"))
	must(t, err)
	var proof struct {
		Passed                      bool   // 所有触屏步骤及网络对拍通过。
		FixtureSha256, BundleSha256 string // 当前配置与真正下载的实包摘要。
		Receipts                    []struct {
			Operation string // 客户端实际收到的操作编号。
			Action    uint32 // 客户端实际收到的原动作。
			Delta     int64  `json:",string"` // 原回执的有符号碎片变化。
		}
	}
	must(t, json.Unmarshal(raw, &proof))
	if !proof.Passed || proof.FixtureSha256 != fixtureHash || proof.BundleSha256 != bundleHash || len(proof.Receipts) != 6 {
		t.Fatal("触屏报告未对应实际包、夹具或原资产操作", proof)
	}
	for i, receipt := range proof.Receipts {
		var op persistence.Operation
		must(t, trial.store.DB.Collection("operations").FindOne(trial.ctx,
			bson.M{"_id": "asset:" + roleID + ":" + receipt.Operation}).Decode(&op))
		if op.Soul == nil || op.Kind != persistence.AssetSoulAction || op.Code != 0 || op.Soul.Action != receipt.Action ||
			op.Soul.Delta != receipt.Delta || op.Soul.Collection.Revision != int64(i+1) {
			t.Fatal("触屏原回执与角色永久账本不同", receipt, op.Soul)
		}
	}
	role, err := trial.store.Role(trial.ctx, roleID)
	must(t, err)
	if role.Soul == nil || role.Soul.Fragments != 19 || role.Soul.Revision != 6 || len(role.Soul.Beasts) != 0 || role.Soul.Attached != 0 {
		t.Fatal("实包合成升星再分解后的最终收藏不一致", role.Soul)
	}
}
