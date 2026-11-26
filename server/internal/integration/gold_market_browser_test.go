//go:build creator_gold_market

// 本文件用实际 Chromium 触屏和独立 MongoDB 对账金币挂单、部分撮合、撤余单及各种托管。
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
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
)

// TestGoldMarketBrowserMongoWS 在同一 Creator 包上比较缺省关闭和显式审核参数的隔离候选。
func TestGoldMarketBrowserMongoWS(t *testing.T) {
	webRoot, fixtureFile := os.Getenv("MINIMIR_GOLD_MARKET_BROWSER_WEB_ROOT"), os.Getenv("MINIMIR_GOLD_MARKET_BROWSER_FIXTURE")
	if webRoot == "" || fixtureFile == "" {
		t.Skip("需要独立 Creator Web Mobile 实构和明确的金币市场隔离参数")
	}
	repo, err := filepath.Abs("../../..")
	must(t, err)
	work := filepath.Join(repo, ".work")
	build, fixture := goldMarketBrowserInput(t, work, webRoot), goldMarketBrowserInput(t, work, fixtureFile)
	bundle, err := os.ReadFile(filepath.Join(build, "assets", "main", "index.js"))
	must(t, err)
	bundleDigest := sha256.Sum256(bundle)
	raw, err := os.ReadFile(fixture)
	must(t, err)
	var candidate devconfig.Fixture
	must(t, json.Unmarshal(raw, &candidate))
	fixtureDigest := sha256.Sum256(raw)
	candidate.Hash = hex.EncodeToString(fixtureDigest[:])
	must(t, candidate.Validate())
	market := candidate.GoldMarket
	if market == nil || market.ReferencePriceYuanbaoUnits != 100 || market.GoldPerLot != 10000 || market.OrderExpirySeconds != 120 || candidate.ItemMarket == nil {
		t.Fatal("需要确认的显式隔离样本（参考100、期限120秒）和保留的道具市场")
	}
	for _, name := range []string{"default-disabled", "two-client-match-cancel-claim-reconnect"} {
		t.Run(name, func(t *testing.T) {
			verifyGoldMarketBrowser(t, repo, work, build, fixture, candidate, hex.EncodeToString(bundleDigest[:]), name != "default-disabled")
		})
	}
}

// goldMarketBrowserInput 拒绝隔离工作目录之外的实构或候选文件。
func goldMarketBrowserInput(t *testing.T, work, supplied string) string {
	t.Helper()
	resolved, err := filepath.EvalSymlinks(supplied)
	must(t, err)
	resolved, err = filepath.Abs(resolved)
	must(t, err)
	relative, err := filepath.Rel(work, resolved)
	must(t, err)
	if relative == "." || !filepath.IsLocal(relative) {
		t.Fatal("金币触屏验收只接受隔离工作目录")
	}
	return resolved
}

type goldBrowserActor struct {
	account string // 一次性测试账号。
	pass    string // 仅子进程读取。
	name    string // 正常选角中显示的姓名。
	role    string // 用于仅核对本测试库的持久角色。
}

func newGoldMarketBrowserActor(t *testing.T, endpoint, prefix string, origin uint64) goldBrowserActor {
	t.Helper()
	actor := goldBrowserActor{account: "goldui_" + domain.ID()[:10], pass: domain.ID()[:16]}
	login := &pb.LoginResult{}
	rpc(t, endpoint, "", p.MsgC2SLocalRegister, &pb.LocalLoginRequest{Account: actor.account, Password: actor.pass}, login, 0)
	role := createDirectoryRole(t, endpoint, login.AccountToken, origin, prefix)
	actor.name, actor.role = role.Name, domain.Hex(role.Key.RoleId)
	return actor
}

func verifyGoldMarketBrowser(t *testing.T, repo, work, build, fixture string, candidate devconfig.Fixture, bundleSHA string, enabled bool) {
	t.Helper()
	ctx, store, account, route := startChallengePlayerDeathServer(t, func(cfg *devconfig.Config) {
		if cfg.Fixture.RuleVersion != candidate.RuleVersion {
			t.Fatal("隔离金币市场的普通玩法版本不同", cfg.Fixture.RuleVersion, candidate.RuleVersion)
		}
		if enabled {
			cfg.Fixture, cfg.FixtureFile = candidate, fixture
		}
		if !enabled && cfg.Fixture.GoldMarket != nil {
			t.Fatal("缺省关闭验收必须使用没有金币市场参数的真实默认配置")
		}
		for i := range cfg.Fixture.Monsters {
			creature := &cfg.Fixture.Monsters[i]
			creature.HP, creature.MoveMS, creature.AttackMin, creature.AttackMax = 5000, 60_000, 0, 0
		}
	})
	seller := newGoldMarketBrowserActor(t, account, "金币卖家", route.origin.OriginID)
	buyer := newGoldMarketBrowserActor(t, account, "金币买家", route.origin.OriginID)
	initialSeller := domain.Wallet{Gold: 50000, BoundGold: 70000, BoundYuanbao: 77}
	initialBuyer := domain.Wallet{BoundGold: 88000, Yuanbao: 1000, BoundYuanbao: 113}
	now := time.Now()
	for _, participant := range []struct {
		actor  goldBrowserActor
		wallet domain.Wallet
	}{{seller, initialSeller}, {buyer, initialBuyer}} {
		seed, err := store.DB.Collection("roles").UpdateByID(ctx, participant.actor.role, bson.M{"$set": bson.M{
			"wallet": participant.wallet, "privilege_since": now.Add(-time.Hour), "privilege_until": now.Add(time.Hour),
		}})
		must(t, err)
		if seed.ModifiedCount != 1 {
			t.Fatal("未准备隔离角色卡期和实际钱包")
		}
	}
	upstream, err := url.Parse("http://" + store.Cfg.AccountAddress)
	must(t, err)
	mux := http.NewServeMux()
	mux.Handle("/account/rpc", http.StripPrefix("/account", httputil.NewSingleHostReverseProxy(upstream)))
	mux.Handle("/", http.FileServer(http.Dir(build)))
	web := httptest.NewServer(mux)
	defer web.Close()
	evidence, err := os.MkdirTemp(work, "gold-market-browser-")
	must(t, err)
	manifest := filepath.Join(evidence, "manifest.json")
	raw, err := json.Marshal(map[string]any{"baseUrl": web.URL, "endpoint": route.assignment.GameWSURL,
		"region": route.origin.RegionName, "server": route.origin.Name, "seller": seller.name, "buyer": buyer.name,
		"ruleVersion": candidate.RuleVersion, "enabled": enabled, "clientBundleSha256": bundleSHA, "fixtureSha256": store.Cfg.Fixture.Hash})
	must(t, err)
	must(t, os.WriteFile(manifest, raw, 0600))
	t.Logf("金币市场触屏验收目录 %s", evidence)
	browserCtx, stop := context.WithTimeout(ctx, 150*time.Second)
	defer stop()
	command := exec.CommandContext(browserCtx, "node", filepath.Join(repo, "tools/client/verify_gold_market_browser.cjs"), manifest)
	command.Dir = repo
	command.Env = append(os.Environ(), "MINIMIR_GOLD_MARKET_SELLER_ACCOUNT="+seller.account, "MINIMIR_GOLD_MARKET_SELLER_PASSWORD="+seller.pass,
		"MINIMIR_GOLD_MARKET_BUYER_ACCOUNT="+buyer.account, "MINIMIR_GOLD_MARKET_BUYER_PASSWORD="+buyer.pass)
	log, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("金币市场触屏验收失败：%v\n%s", err, log)
	}
	t.Logf("%s", log)
	s, err := store.Role(ctx, seller.role)
	must(t, err)
	b, err := store.Role(ctx, buyer.role)
	must(t, err)
	ops := store.DB.Collection("operations")
	n, err := ops.CountDocuments(ctx, bson.M{"kind": persistence.AssetGoldMarket})
	must(t, err)
	if !enabled {
		if n != 0 || s.Wallet != initialSeller || b.Wallet != initialBuyer {
			t.Fatal("默认关闭仍操作了钱包", n, s.Wallet, b.Wallet)
		}
		return
	}
	if n != 7 || s.Wallet.Gold != 40000 || s.Wallet.Yuanbao != 81 || b.Wallet.Gold != 10000 || b.Wallet.Yuanbao != 910 ||
		s.Wallet.BoundGold != initialSeller.BoundGold || s.Wallet.BoundYuanbao != initialSeller.BoundYuanbao ||
		b.Wallet.BoundGold != initialBuyer.BoundGold || b.Wallet.BoundYuanbao != initialBuyer.BoundYuanbao {
		t.Fatal("原钱包或金额守恒不正确", n, s.Wallet, b.Wallet)
	}
	var result struct {
		Passed    bool
		Orders    map[string]string
		SellerOp  string
		Reconnect struct{ SameRequest, SameReceipt bool }
	}
	raw, err = os.ReadFile(filepath.Join(evidence, "gold-market-browser-report.json"))
	must(t, err)
	must(t, json.Unmarshal(raw, &result))
	if !result.Passed || !result.Reconnect.SameRequest || !result.Reconnect.SameReceipt {
		t.Fatal("未看到同一原挂单重放与真实对账", result)
	}
	for _, expected := range []struct {
		who, role string
		state     uint32
	}{{"seller", seller.role, persistence.GoldMarketCancelled}, {"buyer", buyer.role, persistence.GoldMarketFilled}} {
		var observed persistence.GoldMarketOrder
		must(t, store.DB.Collection("gold_orders").FindOne(ctx, bson.M{"_id": result.Orders[expected.who]}).Decode(&observed))
		if observed.State != expected.state || observed.Owner.RoleID != expected.role {
			t.Fatal("原触屏订单阶段或角色不符", expected, observed)
		}
	}
	var original persistence.Operation
	must(t, ops.FindOne(ctx, bson.M{"_id": "asset:" + seller.role + ":" + result.SellerOp, "kind": persistence.AssetGoldMarket}).Decode(&original))
	if original.Code != 0 {
		t.Fatal("原卖单未关联唯一成功的冻结操作", original)
	}
}
