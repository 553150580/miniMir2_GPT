//go:build creator_body_training

// 本文件对照 Creator 构建的炼体触屏结果和 MongoDB 角色账本。
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

// TestBodyTrainingCreatorTouchMongo 触屏验证各阶段炼体候选，默认包显示原只读详情。
func TestBodyTrainingCreatorTouchMongo(t *testing.T) {
	rawRoot, mode := os.Getenv("MINIMIR_BODY_TRAINING_BROWSER_WEB_ROOT"), os.Getenv("MINIMIR_BODY_TRAINING_BROWSER_MODE")
	if rawRoot == "" {
		t.Skip("需要原 Creator 实构和隔离账号")
	}
	if mode != "default" && mode != "candidate" && mode != "hits" && mode != "mastery" {
		t.Fatal("须指定一个隔离炼体候选或默认包")
	}
	repo, err := filepath.Abs("../../..")
	must(t, err)
	build, err := creatorBrowserBuildPath(repo, rawRoot)
	must(t, err)
	source, err := os.ReadFile(filepath.Join(build, "assets", "main", "index.js"))
	must(t, err)
	bundleHash := persistence.Hash(source)
	enabled, hits, mastery := mode != "default", mode == "hits", mode == "mastery"
	var trial worldBossRewardTrial
	var player worldBossRewardPlayer
	switch mode {
	case "mastery":
		trial, player = startBodyMasteryTrial(t, nil)
	case "hits":
		trial, player = startBodyHitsTrial(t, true, nil)
	default:
		trial, player = startBodyTrainingTrial(t, enabled)
	}
	fixtureHash := trial.store.Cfg.Fixture.Hash
	upstream, err := url.Parse(trial.endpoint)
	must(t, err)
	upstream.Path = ""
	mux := http.NewServeMux()
	mux.Handle("/account/rpc", http.StripPrefix("/account", httputil.NewSingleHostReverseProxy(upstream)))
	mux.Handle("/", http.FileServer(http.Dir(build)))
	web := httptest.NewServer(mux)
	defer web.Close()
	evidence, err := os.MkdirTemp(filepath.Join(repo, ".work"), "body-training-browser-")
	must(t, err)
	t.Log("炼体触屏验收目录 " + evidence)
	manifest := map[string]any{"baseUrl": web.URL, "endpoint": trial.route.assignment.GameWSURL, "role": player.summary.Name,
		"region": trial.route.origin.RegionName, "server": trial.route.origin.Name, "enabled": enabled, "hits": hits, "mastery": mastery,
		"fixtureSha256": fixtureHash, "bundleSha256": bundleHash}
	raw, err := json.Marshal(manifest)
	must(t, err)
	input := filepath.Join(evidence, "manifest.json")
	must(t, os.WriteFile(input, raw, 0600))
	ctx, cancel := context.WithTimeout(trial.ctx, 180*time.Second)
	defer cancel()
	node := exec.CommandContext(ctx, "node", filepath.Join(repo, "tools/client/verify_body_training_browser.cjs"), input)
	node.Dir = repo
	node.Env = append(os.Environ(), "MINIMIR_BODY_TRAINING_BROWSER_ACCOUNT="+player.account, "MINIMIR_BODY_TRAINING_BROWSER_PASSWORD="+player.password)
	output, err := node.CombinedOutput()
	if err != nil {
		t.Fatalf("炼体触屏验收失败 %v\n%s", err, output)
	}
	raw, err = os.ReadFile(filepath.Join(evidence, "body-training-browser-report.json"))
	must(t, err)
	var result struct {
		Passed, Enabled, Hits, Mastery bool   // 是否通过、开放以及实际应用的候选。
		FixtureSha256, BundleSha256    string // 实际配置及下载实包的摘要。
		Receipts                       []struct {
			Operation, Quote string // 浏览器看到的操作和报价身份。
			Track            uint32 // 本次成交的系别。
		}
	}
	must(t, json.Unmarshal(raw, &result))
	if !result.Passed || result.Enabled != enabled || result.Hits != hits || result.Mastery != mastery ||
		result.FixtureSha256 != fixtureHash || result.BundleSha256 != bundleHash {
		t.Fatal("触屏报告未对应实际包或夹具", result)
	}
	roleID := domain.Hex(player.summary.Key.RoleId)
	role, err := trial.store.Role(trial.ctx, roleID)
	must(t, err)
	expected := 0
	if enabled {
		expected = 2
		levels, wallet := domain.BodyTrainingState{Health: 1, Mana: 1}, domain.Wallet{Gold: 3000}
		switch mode {
		case "hits":
			levels, wallet = domain.BodyTrainingState{Damage: 1, Absorption: 1}, domain.Wallet{Gold: 2500}
		case "mastery":
			expected = 3
			levels, wallet = domain.BodyTrainingState{Critical: 1, Speed: 1, Resistance: 1},
				domain.Wallet{Gold: 5000, BoundGold: 1000, Yuanbao: 550}
		}
		if role.BodyTrainingLevels() != levels || role.Wallet != wallet {
			t.Fatal("浏览器升级、费用与数据库未对账", role.BodyTrainingLevels(), role.Wallet)
		}
		for _, receipt := range result.Receipts {
			var original persistence.Operation
			must(t, trial.store.DB.Collection("operations").FindOne(trial.ctx,
				bson.M{"_id": "asset:" + roleID + ":" + receipt.Operation}).Decode(&original))
			if original.BodyTraining == nil || original.BodyTraining.QuoteID != receipt.Quote ||
				original.BodyTraining.Proto().GetTerms().GetTrack() != receipt.Track {
				t.Fatal("浏览器原回执未对应数据库持久化操作")
			}
		}
	} else if role.BodyTraining != nil || role.Wallet.Gold != 5000 || role.Wallet.BoundGold != 1000 {
		t.Fatal("默认包应保留旧角色结构和原钱包")
	}
	audits, err := trial.store.DB.Collection("audit_outbox").CountDocuments(trial.ctx,
		bson.M{"role_id": roleID, "kind": persistence.AssetBodyTrainingCommit})
	must(t, err)
	if len(result.Receipts) != expected || audits != int64(expected) || role.EconomicSeq != int64(expected) {
		t.Fatal("永久账本笔数与触屏操作不一致", result.Receipts, audits)
	}
}
