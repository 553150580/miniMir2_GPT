//go:build creator_battle_pass

// 本文件用真实 Creator 包领取已经解锁的战令档位差额，并和独立数据库逐笔对账。
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
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/domain"
	"minimir/internal/payment"
	"minimir/internal/persistence"
)

// TestBattlePassCreatorTouchMongo 确认默认包关闭与实际候选付费差额的触屏交付。
func TestBattlePassCreatorTouchMongo(t *testing.T) {
	root, mode := os.Getenv("MINIMIR_BATTLE_PASS_BROWSER_WEB_ROOT"), os.Getenv("MINIMIR_BATTLE_PASS_BROWSER_MODE")
	if root == "" {
		t.Skip("需要 Creator Web Mobile 实包")
	}
	if mode != "candidate" && mode != "default" {
		t.Fatal("战令触屏必须明确指定候选或默认客户端")
	}
	enabled := mode == "candidate"
	repo, err := filepath.Abs("../../..")
	must(t, err)
	web, err := creatorBrowserBuildPath(repo, root)
	must(t, err)
	bundle, err := os.ReadFile(filepath.Join(web, "assets", "main", "index.js"))
	must(t, err)
	bundleHash := persistence.Hash(bundle)
	trial := startBattlePassTrial(t, enabled)
	player := trial.create(t, "战令触屏", time.Now().Add(time.Hour), false)
	roleID := domain.Hex(player.summary.Key.RoleId)
	day := uint32(0)
	if enabled {
		c := trial.attach(t, &player, true)
		state := qualifyBattlePass(t, trial, player, c)
		day = currentBattlePassIndex(t, state)
		claimBattlePassWS(t, c, &pb.BattlePassClaimRequest{SeasonStart: state.SeasonStart, Day: day, Tier: 1}, 0)
		c.close()
		auth, err := trial.store.Authenticate(trial.ctx, player.token)
		must(t, err)
		auth.RoleID = roleID
		quote, err := trial.store.QuoteBattlePassPayment(trial.ctx, auth, state.SeasonStart, 2, "isolated-touch", "isolated-cash-only")
		must(t, err)
		order, err := trial.store.CreatePaymentOrder(trial.ctx, auth, domain.ID(), quote)
		must(t, err)
		order, err = trial.store.BindPaymentOrder(trial.ctx, order.ID, quote.ProviderID, quote.MerchantID, domain.ID())
		must(t, err)
		verifyBattlePassPayment(t, trial, order, payment.ProviderPaid, nil, 0)
		waitBattlePassPayment(t, trial, order)
	}
	before, err := trial.store.Role(trial.ctx, roleID)
	must(t, err)
	upstream, err := url.Parse(trial.endpoint)
	must(t, err)
	upstream.Path = ""
	mux := http.NewServeMux()
	mux.Handle("/account/rpc", http.StripPrefix("/account", httputil.NewSingleHostReverseProxy(upstream)))
	mux.Handle("/", http.FileServer(http.Dir(web)))
	browser := httptest.NewServer(mux)
	defer browser.Close()
	notes, err := os.MkdirTemp(filepath.Join(repo, ".work"), "battle-pass-browser-")
	must(t, err)
	t.Log("战令触屏验收目录 " + notes)
	fixtureHash := trial.store.Cfg.Fixture.Hash
	manifest := map[string]any{"baseUrl": browser.URL, "endpoint": trial.route.assignment.GameWSURL, "role": player.summary.Name,
		"region": trial.route.origin.RegionName, "server": trial.route.origin.Name, "enabled": enabled, "day": day,
		"fixtureSha256": fixtureHash, "bundleSha256": bundleHash}
	raw, err := json.Marshal(manifest)
	must(t, err)
	input := filepath.Join(notes, "manifest.json")
	must(t, os.WriteFile(input, raw, 0600))
	ctx, cancel := context.WithTimeout(trial.ctx, 150*time.Second)
	defer cancel()
	node := exec.CommandContext(ctx, "node", filepath.Join(repo, "tools/client/verify_battle_pass_browser.cjs"), input)
	node.Dir = repo
	node.Env = append(os.Environ(), "MINIMIR_BATTLE_PASS_BROWSER_ACCOUNT="+player.account, "MINIMIR_BATTLE_PASS_BROWSER_PASSWORD="+player.password)
	output, err := node.CombinedOutput()
	if err != nil {
		t.Fatalf("战令触屏验收失败 %v\n%s", err, output)
	}
	raw, err = os.ReadFile(filepath.Join(notes, "battle-pass-browser-report.json"))
	must(t, err)
	var proof struct {
		Passed, Enabled             bool   // 浏览器实际完成的候选或默认流程。
		FixtureSha256, BundleSha256 string // 实际夹具和下载的客户端包。
		Operation                   string // 浏览器原领取的经济操作编号。
	}
	must(t, json.Unmarshal(raw, &proof))
	if !proof.Passed || proof.Enabled != enabled || proof.FixtureSha256 != fixtureHash || proof.BundleSha256 != bundleHash {
		t.Fatal("触屏报告未对应实际夹具或客户端包", proof)
	}
	after, err := trial.store.Role(trial.ctx, roleID)
	must(t, err)
	if !enabled {
		if proof.Operation != "" || after.Wallet != before.Wallet {
			t.Fatal("默认包触发了候选战令收益", proof, after.Wallet)
		}
		return
	}
	var receipt persistence.Operation
	must(t, trial.store.DB.Collection("operations").FindOne(trial.ctx, bson.M{"_id": "asset:" + roleID + ":" + proof.Operation}).Decode(&receipt))
	if receipt.Code != 0 || receipt.Kind != persistence.AssetBattlePassClaim || receipt.BattlePass == nil || receipt.BattlePass.Day != day ||
		receipt.BattlePass.PreviousTier != 1 || receipt.BattlePass.Tier != 2 {
		t.Fatal("触屏原回执没有提交对应的原日期和档位", receipt.BattlePass)
	}
	assertBattlePassMaterials(t, receipt.BattlePass.Proto(), map[uint32]int64{900001: 8, 900021: 1, 900041: 3, 900051: 5}, 3000, 1000)
	want := before.Wallet
	want.BoundGold += 3000
	want.BoundYuanbao += 1000
	if after.Wallet != want {
		t.Fatal("触屏实际领取后的两种绑定钱包不守恒", after.Wallet, want)
	}
}
