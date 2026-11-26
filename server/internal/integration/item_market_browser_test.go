//go:build creator_item_market

// 本文件用实际保存的市场触屏界面验证独立 MongoDB 中的成交、退款托管和原操作重连。
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

// TestItemMarketBrowserMongoWS 验证同一真实构建在缺省关闭与工作簿候选启用时的行为。
func TestItemMarketBrowserMongoWS(t *testing.T) {
	webRoot, fixturePath := os.Getenv("MINIMIR_ITEM_MARKET_BROWSER_WEB_ROOT"), os.Getenv("MINIMIR_ITEM_MARKET_BROWSER_FIXTURE")
	if webRoot == "" || fixturePath == "" {
		t.Skip("需要独立 Creator Web Mobile 实构及道具市场候选")
	}
	repo, err := filepath.Abs("../../..")
	must(t, err)
	work := filepath.Join(repo, ".work")
	build, fixture := itemMarketBrowserInput(t, work, webRoot), itemMarketBrowserInput(t, work, fixturePath)
	bundle, err := os.ReadFile(filepath.Join(build, "assets", "main", "index.js"))
	must(t, err)
	bundleSHA := sha256.Sum256(bundle)
	raw, err := os.ReadFile(fixture)
	must(t, err)
	var candidate devconfig.Fixture
	must(t, json.Unmarshal(raw, &candidate))
	sha := sha256.Sum256(raw)
	candidate.Hash = hex.EncodeToString(sha[:])
	must(t, candidate.Validate())
	if candidate.ItemMarket == nil {
		t.Fatal("浏览器验收需要来自唯一源的道具市场规则")
	}
	for _, run := range []struct {
		name    string
		enabled bool
	}{{"default-disabled", false}, {"listing-bid-buy-escrow-reconnect", true}} {
		t.Run(run.name, func(t *testing.T) {
			verifyItemMarketBrowser(t, repo, work, build, fixture, candidate, hex.EncodeToString(bundleSHA[:]), run.enabled)
		})
	}
}

// itemMarketBrowserInput 只接受任务工作目录内真实构建和候选配置的完整路径。
func itemMarketBrowserInput(t *testing.T, root, supplied string) string {
	t.Helper()
	actual, err := filepath.EvalSymlinks(supplied)
	must(t, err)
	actual, err = filepath.Abs(actual)
	must(t, err)
	local, err := filepath.Rel(root, actual)
	must(t, err)
	if local == "." || !filepath.IsLocal(local) {
		t.Fatal("道具市场触屏只使用隔离构建和候选文件")
	}
	return actual
}

type marketBrowserActor struct {
	account string // 只子进程接收的临时账号。
	pass    string // 不写入报告的临时密码。
	name    string // 正常选角中显示的角色名。
	role    string // Mongo 用于核对永久对账的角色身份。
}

func startItemMarketBrowserActor(t *testing.T, endpoint, prefix string, origin uint64) marketBrowserActor {
	t.Helper()
	actor := marketBrowserActor{account: "market_" + domain.ID()[:11], pass: domain.ID()}
	actor.pass = actor.pass[:16]
	login := &pb.LoginResult{}
	rpc(t, endpoint, "", p.MsgC2SLocalRegister, &pb.LocalLoginRequest{Account: actor.account, Password: actor.pass}, login, 0)
	role := createDirectoryRole(t, endpoint, login.AccountToken, origin, prefix)
	actor.name, actor.role = role.Name, domain.Hex(role.Key.RoleId)
	return actor
}

func verifyItemMarketBrowser(t *testing.T, repo, work, build, fixture string, candidate devconfig.Fixture, bundleSHA string, enabled bool) {
	t.Helper()
	ctx, store, accountURL, route := startChallengePlayerDeathServer(t, func(cfg *devconfig.Config) {
		if cfg.Fixture.RuleVersion != candidate.RuleVersion {
			t.Fatal("候选与客户端普通玩法资源版本不同", candidate.RuleVersion, cfg.Fixture.RuleVersion)
		}
		if enabled {
			cfg.Fixture, cfg.FixtureFile = candidate, fixture
			rules := *candidate.ItemMarket
			// 台上两小时和二十四小时规则从原候选读取，仅在此隔离时钟缩短等待。
			rules.AuctionSeconds, rules.BuyoutSeconds = 25, 85
			rules.ExtendWhenRemainingBelowSeconds, rules.ExtendToRemainingSeconds = 3, 3
			cfg.Fixture.ItemMarket = &rules
		} else {
			cfg.Fixture.ItemMarket = nil
		}
		for i := range cfg.Fixture.Monsters {
			creature := &cfg.Fixture.Monsters[i]
			creature.HP, creature.MoveMS, creature.AttackMin, creature.AttackMax = 5000, 60_000, 0, 0
		}
	})
	seller := startItemMarketBrowserActor(t, accountURL, "交易卖家", route.origin.OriginID)
	buyer := startItemMarketBrowserActor(t, accountURL, "交易买家", route.origin.OriginID)
	const itemID = 35
	definition, ok := candidate.Item(itemID)
	if !ok || !definition.Stackable || definition.MaxStack < 5 {
		t.Fatal("触屏验收需要既有的可堆叠物品35")
	}
	asset := domain.Item{ID: domain.ID(), ItemID: itemID, Quantity: 5, Version: 1, SourceKind: "monster_drop", SourceID: domain.ID()}
	now := time.Now()
	for _, participant := range []struct {
		actor  marketBrowserActor
		wallet domain.Wallet
	}{{seller, domain.Wallet{BoundYuanbao: 77}}, {buyer, domain.Wallet{Yuanbao: 1000, BoundYuanbao: 113}}} {
		fields := bson.M{"wallet": participant.wallet, "privilege_since": now.Add(-time.Hour), "privilege_until": now.Add(time.Hour)}
		if participant.actor == seller {
			role, err := store.Role(ctx, seller.role)
			must(t, err)
			fields["items"] = append(role.Items, asset)
		}
		seed, err := store.DB.Collection("roles").UpdateByID(ctx, participant.actor.role, bson.M{"$set": fields})
		must(t, err)
		if seed.ModifiedCount != 1 {
			t.Fatal("无法准备本用例的原始卡期与钱包")
		}
	}
	upstream, err := url.Parse("http://" + store.Cfg.AccountAddress)
	must(t, err)
	mux := http.NewServeMux()
	mux.Handle("/account/rpc", http.StripPrefix("/account", httputil.NewSingleHostReverseProxy(upstream)))
	mux.Handle("/", http.FileServer(http.Dir(build)))
	web := httptest.NewServer(mux)
	defer web.Close()
	evidence, err := os.MkdirTemp(work, "item-market-browser-")
	must(t, err)
	manifest := filepath.Join(evidence, "manifest.json")
	raw, err := json.Marshal(map[string]any{"baseUrl": web.URL, "endpoint": route.assignment.GameWSURL,
		"region": route.origin.RegionName, "server": route.origin.Name, "seller": seller.name, "buyer": buyer.name,
		"ruleVersion": candidate.RuleVersion, "enabled": enabled, "clientBundleSha256": bundleSHA, "fixtureSha256": candidate.Hash,
		"itemName": definition.Name, "itemId": itemID, "sourceId": asset.SourceID})
	must(t, err)
	must(t, os.WriteFile(manifest, raw, 0600))
	t.Logf("道具市场触屏验收目录 %s", evidence)
	browserCtx, stop := context.WithTimeout(ctx, 155*time.Second)
	defer stop()
	command := exec.CommandContext(browserCtx, "node", filepath.Join(repo, "tools/client/verify_item_market_browser.cjs"), manifest)
	command.Dir = repo
	command.Env = append(os.Environ(), "MINIMIR_MARKET_SELLER_ACCOUNT="+seller.account, "MINIMIR_MARKET_SELLER_PASSWORD="+seller.pass,
		"MINIMIR_MARKET_BUYER_ACCOUNT="+buyer.account, "MINIMIR_MARKET_BUYER_PASSWORD="+buyer.pass)
	log, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("道具市场触屏验收失败：%v\n%s", err, log)
	}
	t.Logf("%s", log)
	sold, err := store.Role(ctx, seller.role)
	must(t, err)
	bought, err := store.Role(ctx, buyer.role)
	must(t, err)
	ops := store.DB.Collection("operations")
	orders := store.DB.Collection("item_orders")
	newOps, err := ops.CountDocuments(ctx, bson.M{"kind": persistence.AssetItemMarket})
	must(t, err)
	if !enabled {
		if newOps != 0 || bought.Wallet.Yuanbao != 1000 || marketBrowserItemCount(sold, asset.SourceID) != 5 {
			t.Fatal("禁用客户端改动了道具市场资产", newOps, bought.Wallet)
		}
		return
	}
	if newOps != 11 || sold.Wallet.Yuanbao != 49 || sold.Wallet.BoundYuanbao != 77 ||
		bought.Wallet.Yuanbao != 946 || bought.Wallet.BoundYuanbao != 113 ||
		marketBrowserItemCount(sold, asset.SourceID) != 1 || marketBrowserItemCount(bought, asset.SourceID) != 3 {
		t.Fatal("市场前端的交易、领取或资金不符合唯一永久回执", newOps, sold.Wallet, bought.Wallet,
			marketBrowserItemCount(sold, asset.SourceID), marketBrowserItemCount(bought, asset.SourceID))
	}
	var observed struct {
		Passed    bool
		OrderIDs  map[string]string
		DirectOp  string
		Reconnect struct{ SameRequest, SameReceipt bool }
	}
	raw, err = os.ReadFile(filepath.Join(evidence, "item-market-browser-report.json"))
	must(t, err)
	must(t, json.Unmarshal(raw, &observed))
	if !observed.Passed || !observed.Reconnect.SameRequest || !observed.Reconnect.SameReceipt {
		t.Fatal("浏览器没有确认原购买与持久回放", observed)
	}
	for label, phase := range map[string]uint32{"auction": persistence.ItemMarketSold, "direct": persistence.ItemMarketDirect, "cancel": persistence.ItemMarketCancelled} {
		var order persistence.ItemMarketOrder
		must(t, orders.FindOne(ctx, bson.M{"_id": observed.OrderIDs[label]}).Decode(&order))
		if order.Phase != phase || order.Seller.RoleID != seller.role {
			t.Fatal("浏览器观察的订单阶段或卖家不正确", label, order)
		}
	}
	var direct persistence.Operation
	must(t, ops.FindOne(ctx, bson.M{"_id": "asset:" + buyer.role + ":" + observed.DirectOp, "kind": persistence.AssetItemMarket}).Decode(&direct))
	if direct.Code != 0 {
		t.Fatal("重连后产生了额外扣款或原直购未提交", direct)
	}
}

func marketBrowserItemCount(role domain.Role, source string) int64 {
	var quantity int64
	for _, item := range role.Items {
		if item.SourceID == source && item.ItemID == 35 && !item.Bound {
			quantity += item.Quantity
		}
	}
	return quantity
}
