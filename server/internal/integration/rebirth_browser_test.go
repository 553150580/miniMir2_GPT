//go:build creator_rebirth

// 本文件把真实 Creator 触屏报价、确认及回执与 MongoDB 原状态对账。
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
	"slices"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"minimir/internal/domain"
	"minimir/internal/persistence"
)

// TestRebirthCreatorTouchMongo 在同源的默认和候选构建上检查入口和原子资产结果。
func TestRebirthCreatorTouchMongo(t *testing.T) {
	webRoot, mode := os.Getenv("MINIMIR_REBIRTH_BROWSER_WEB_ROOT"), os.Getenv("MINIMIR_REBIRTH_BROWSER_MODE")
	if webRoot == "" {
		t.Skip("需要隔离 Creator 构建目录")
	}
	if mode != "default" && mode != "candidate" {
		t.Fatal("必须指定默认包或转生候选包")
	}
	repo, err := filepath.Abs("../../..")
	must(t, err)
	build, err := creatorBrowserBuildPath(repo, webRoot)
	must(t, err)
	bundle, err := os.ReadFile(filepath.Join(build, "assets", "main", "index.js"))
	must(t, err)
	enabled := mode == "candidate"
	trial, role := startRebirthTrial(t, enabled)
	fixture := &trial.store.Cfg.Fixture
	roleID := domain.Hex(role.summary.Key.RoleId)
	challenge, proof, material := "", uint32(0), uint32(0)
	if enabled {
		stage := fixture.Rebirth.Stages[0]
		c := trial.attach(t, &role, true)
		challenge = winRebirthPersonalBoss(t, c, stage.ChallengeTemplateID)
		c.close()
		proof, material = stage.CredentialItemID, stage.MaterialItemID
	}
	before, err := trial.store.Role(trial.ctx, roleID)
	must(t, err)
	upstream, err := url.Parse(trial.endpoint)
	must(t, err)
	upstream.Path = ""
	mux := http.NewServeMux()
	mux.Handle("/account/rpc", http.StripPrefix("/account", httputil.NewSingleHostReverseProxy(upstream)))
	mux.Handle("/", http.FileServer(http.Dir(build)))
	web := httptest.NewServer(mux)
	defer web.Close()
	evidence, err := os.MkdirTemp(filepath.Join(repo, ".work"), "rebirth-browser-")
	must(t, err)
	t.Log("转生触屏验收目录 " + evidence)
	bundleHash := persistence.Hash(bundle)
	manifest := map[string]any{"baseUrl": web.URL, "endpoint": trial.route.assignment.GameWSURL, "role": role.summary.Name,
		"region": trial.route.origin.RegionName, "server": trial.route.origin.Name, "enabled": enabled, "fixtureSha256": fixture.Hash,
		"clientBundleSha256": bundleHash, "challengeId": challenge, "credentialItemId": proof, "materialItemId": material}
	raw, err := json.Marshal(manifest)
	must(t, err)
	input := filepath.Join(evidence, "manifest.json")
	must(t, os.WriteFile(input, raw, 0600))
	ctx, cancel := context.WithTimeout(trial.ctx, 200*time.Second)
	defer cancel()
	node := exec.CommandContext(ctx, "node", filepath.Join(repo, "tools/client/verify_rebirth_browser.cjs"), input)
	node.Dir = repo
	node.Env = append(os.Environ(), "MINIMIR_REBIRTH_BROWSER_ACCOUNT="+role.account, "MINIMIR_REBIRTH_BROWSER_PASSWORD="+role.password)
	output, err := node.CombinedOutput()
	if err != nil {
		t.Fatalf("转生触屏验收失败 %v\n%s", err, output)
	}
	raw, err = os.ReadFile(filepath.Join(evidence, "rebirth-browser-report.json"))
	must(t, err)
	var report struct {
		Passed, Enabled                   bool   // 浏览器通过结果和默认开关。
		FixtureSha256, ClientBundleSha256 string // 对应夹具和实际下载的客户端摘要。
		Receipt                           *struct {
			Op, QuoteID, ChallengeID string // 原操作、原报价、原首领。
		}
	}
	must(t, json.Unmarshal(raw, &report))
	if !report.Passed || report.Enabled != enabled || report.FixtureSha256 != fixture.Hash || report.ClientBundleSha256 != bundleHash {
		t.Fatal("触屏报告与构建或夹具不符", report)
	}
	after, err := trial.store.Role(trial.ctx, roleID)
	must(t, err)
	var expected uint32
	if enabled {
		expected = 1
		if report.Receipt == nil || report.Receipt.ChallengeID != challenge || after.Level != 51 || after.PendingExperience != 0 ||
			after.Wallet.Gold != 800 || after.Wallet.BoundGold != 0 || !slices.Equal(before.Equipped, after.Equipped) {
			t.Fatal("触屏成交后角色的等级、装备、钱款或胜利身份不对")
		}
		var operation persistence.Operation
		must(t, trial.store.DB.Collection("operations").FindOne(trial.ctx,
			bson.M{"_id": "asset:" + roleID + ":" + report.Receipt.Op}).Decode(&operation))
		if operation.Rebirth == nil || operation.Rebirth.QuoteID != report.Receipt.QuoteID {
			t.Fatal("浏览器原回执未落入对应永久操作")
		}
	} else if report.Receipt != nil || before.ProgressionSnapshot() != after.ProgressionSnapshot() || before.Wallet != after.Wallet {
		t.Fatal("关闭候选时默认包改变了等级或钱包")
	}
	count, err := trial.store.DB.Collection("audit_outbox").CountDocuments(trial.ctx, bson.M{"role_id": roleID, "kind": persistence.AssetRebirthCommit})
	must(t, err)
	if count != int64(expected) || after.RebirthCount != expected || after.EconomicSeq != before.EconomicSeq+int64(expected) {
		t.Fatal("实际成交与审计笔数不一致", count, after.RebirthCount)
	}
}
