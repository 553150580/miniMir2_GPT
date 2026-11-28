//go:build creator_rewarded_ad

// 本文件用实际 Creator Web Mobile 构建和隔离渠道核对托管入口、到账与账本。
package integration

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/netip"
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
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
	"minimir/internal/rewardedad"
)

// TestRewardedAdCreatorTouchMongo 从实际触屏入口进入，并逐笔核对原收益。
func TestRewardedAdCreatorTouchMongo(t *testing.T) {
	root, mode := os.Getenv("MINIMIR_REWARDED_AD_BROWSER_WEB_ROOT"), os.Getenv("MINIMIR_REWARDED_AD_BROWSER_MODE")
	if root == "" {
		t.Skip("需要 Creator Web Mobile 实包")
	}
	if mode != "candidate" && mode != "default" {
		t.Fatal("请指定默认或隔离候选触屏验收")
	}
	enabled := mode == "candidate"
	repo, err := filepath.Abs("../../..")
	must(t, err)
	web, err := creatorBrowserBuildPath(repo, root)
	must(t, err)
	bundle, err := os.ReadFile(filepath.Join(web, "assets", "main", "index.js"))
	must(t, err)
	bundleHash := persistence.Hash(bundle)
	var ctx context.Context
	var store *persistence.Store
	var account string
	var route directoryTestEntry
	var trial rewardedAdTrial
	if enabled {
		trial = startRewardedAdTrial(t)
		ctx, store, account, route = trial.ctx, trial.store, trial.rpcURL(), trial.routes[1]
	} else {
		ctx, store, account, route = startChallengePlayerDeathServer(t, func(*devconfig.Config) {})
	}
	username, password := "advideo_"+domain.ID()[:9], "isolation_video_6633"
	login := &pb.LoginResult{}
	rpc(t, account, "", p.MsgC2SLocalRegister, &pb.LocalLoginRequest{Account: username, Password: password}, login, 0)
	player := createDirectoryRole(t, account, login.AccountToken, route.origin.OriginID, "触屏视频")
	role := domain.Hex(player.Key.RoleId)
	const balance = 24*60*60 + 300 // 原角色持有余额，实际广告到账后可超过 24 小时。
	_, err = store.DB.Collection("roles").UpdateByID(ctx, role, bson.M{"$set": bson.M{"hosting_seconds": balance}})
	must(t, err)
	before, err := store.Role(ctx, role)
	must(t, err)
	upstream, err := url.Parse(account)
	must(t, err)
	upstream.Path = ""
	mux := http.NewServeMux()
	mux.Handle("/account/rpc", http.StripPrefix("/account", httputil.NewSingleHostReverseProxy(upstream)))
	mux.Handle("/", http.FileServer(http.Dir(web)))
	browser := httptest.NewServer(mux)
	defer browser.Close()
	var emulatorURL, emulatorKey string
	if enabled {
		emulatorKey = domain.ID() + domain.ID()
		emulator := newRewardedAdTouchEmulator(trial, emulatorKey, before)
		defer emulator.Close()
		emulatorURL = emulator.URL
	}

	notes, err := os.MkdirTemp(filepath.Join(repo, ".work"), "rewarded-ad-browser-")
	must(t, err)
	t.Log("激励视频触屏验收目录 " + notes)
	fixtureHash := store.Cfg.Fixture.Hash
	manifest := map[string]any{"baseUrl": browser.URL, "endpoint": route.assignment.GameWSURL,
		"role": player.Name, "region": route.origin.RegionName, "server": route.origin.Name, "enabled": enabled,
		"fixtureSha256": fixtureHash, "bundleSha256": bundleHash, "startingSeconds": balance}
	raw, err := json.Marshal(manifest)
	must(t, err)
	input := filepath.Join(notes, "manifest.json")
	must(t, os.WriteFile(input, raw, 0600))
	deadline, cancel := context.WithTimeout(ctx, 150*time.Second)
	defer cancel()
	node := exec.CommandContext(deadline, "node", filepath.Join(repo, "tools/client/verify_rewarded_ad_browser.cjs"), input)
	node.Dir = repo
	node.Env = append(os.Environ(), "MINIMIR_REWARDED_AD_BROWSER_ACCOUNT="+username, "MINIMIR_REWARDED_AD_BROWSER_PASSWORD="+password,
		"MINIMIR_REWARDED_AD_TOUCH_EMULATOR="+emulatorURL, "MINIMIR_REWARDED_AD_TOUCH_KEY="+emulatorKey)
	output, err := node.CombinedOutput()
	if err != nil {
		t.Fatalf("激励视频触屏验收失败 %v\n%s", err, output)
	}
	raw, err = os.ReadFile(filepath.Join(notes, "rewarded-ad-browser-report.json"))
	must(t, err)
	var proof struct {
		Passed, Enabled             bool   // 用户触屏流程结束且已对比原余额。
		FixtureSha256, BundleSha256 string // 校对实际下载的脚本和服务端规则。
		Attempt                     string // 玩家触屏触发的原观看编号。
		GrantFrames                 int    // 已观察到的原余额 WebSocket 消息。
	}
	must(t, json.Unmarshal(raw, &proof))
	if !proof.Passed || proof.Enabled != enabled || proof.FixtureSha256 != fixtureHash || proof.BundleSha256 != bundleHash {
		t.Fatal("触屏报告未对应实际夹具和构建", proof)
	}
	after, err := store.Role(ctx, role)
	must(t, err)
	if after.Wallet != before.Wallet || after.EconomicSeq != before.EconomicSeq || after.ClaimDay != before.ClaimDay ||
		after.ClaimedSeconds != before.ClaimedSeconds {
		t.Fatal("浏览器广告修改了原钱包或日领额度", after.Wallet, before.Wallet)
	}
	count, err := store.DB.Collection("operations").CountDocuments(ctx, bson.M{"kind": "rewarded_ad_grant"})
	must(t, err)
	if !enabled {
		if proof.Attempt != "" || proof.GrantFrames != 0 || count != 0 || after.HostingSeconds != balance {
			t.Fatal("默认包触发了视频权益", proof, after.HostingSeconds)
		}
		return
	}
	var receipt persistence.Operation
	must(t, store.DB.Collection("operations").FindOne(ctx, bson.M{"_id": "rewarded_ad:" + proof.Attempt}).Decode(&receipt))
	if count != 1 || proof.GrantFrames != 1 || after.HostingSeconds != balance+3600 || receipt.RewardedAd == nil ||
		receipt.RoleID != role || receipt.RewardedAd.Origin != route.origin.OriginID || receipt.RewardedAd.Seconds != 3600 ||
		receipt.RewardedAd.Before != balance || receipt.RewardedAd.Balance != after.HostingSeconds {
		t.Fatal("触屏原回执、原运行服余额或到账次数不一致", proof, receipt.RewardedAd, after.HostingSeconds)
	}
}

// newRewardedAdTouchEmulator 仅供测试的 Node 宿主使用。认证密钥和完播签名始终在测试进程内。
func newRewardedAdTouchEmulator(trial rewardedAdTrial, key string, role domain.Role) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || subtle.ConstantTimeCompare([]byte(r.Header.Get("X-Isolation-Proof")), []byte(key)) != 1 {
			http.NotFound(w, r)
			return
		}
		var terms struct {
			AttemptID   string // 触屏已发起的原观看编号。
			Placement   string // 宿主收到的隔离广告位。
			Provider    string // 宿主收到的隔离渠道。
			Beneficiary string // 原签票的角色绑定标记。
		}
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&terms) != nil {
			http.Error(w, "invalid playback", http.StatusBadRequest)
			return
		}
		ticket, err := trial.store.RewardedAdAttempt(r.Context(), terms.AttemptID)
		if err != nil || ticket.RoleID != role.ID || ticket.AccountID != role.AccountID || ticket.OriginServerID != role.Origin ||
			ticket.Rules.Provider != terms.Provider || ticket.Rules.Placement != terms.Placement ||
			ticket.Expected().BeneficiaryReference() != terms.Beneficiary {
			http.Error(w, "playback is unbound", http.StatusUnauthorized)
			return
		}
		completion := rewardedad.Completion{AttemptID: ticket.ID, Placement: ticket.Rules.Placement, Beneficiary: terms.Beneficiary,
			EventID: "creator-" + ticket.ID, DeviceID: "creator-touch-test", ClientIP: netip.MustParseAddr("192.0.2.66"),
			CompletedAt: time.Now().UTC(), Completed: true}
		notice, err := json.Marshal(completion)
		if err == nil {
			var status int
			status, err = postProofOrigin(r.Context(), trial.account, ticket.ID, trial.provider.signature(notice), notice)
			if err == nil && status == http.StatusOK {
				w.WriteHeader(http.StatusNoContent)
				return
			}
		}
		http.Error(w, "isolation callback failed", http.StatusBadGateway)
	}))
}
