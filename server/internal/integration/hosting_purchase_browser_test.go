// 本文件以真实 Creator 客户端的购买、取消和重连输入核对永久补时事务。
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
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
)

// TestHostingPurchaseBrowserMongoWS 在默认规则和隔离报价规则各跑一遍保存的触屏入口。
func TestHostingPurchaseBrowserMongoWS(t *testing.T) {
	webRoot := os.Getenv("MINIMIR_HOSTING_BROWSER_WEB_ROOT")
	if webRoot == "" {
		t.Skip("需要独立 Creator Web Mobile 构建")
	}
	repo, err := filepath.Abs("../../..")
	must(t, err)
	work := filepath.Join(repo, ".work")
	build, err := filepath.EvalSymlinks(webRoot)
	must(t, err)
	build, err = filepath.Abs(build)
	must(t, err)
	relative, err := filepath.Rel(work, build)
	must(t, err)
	if relative == "." || !filepath.IsLocal(relative) {
		t.Fatal("只能以工作区内的独立 Creator 构建验收")
	}
	_, err = os.Stat(filepath.Join(build, "index.html"))
	must(t, err)
	for _, candidate := range []struct {
		name    string // 隔离服务器和浏览器报告的场景名称。
		enabled bool   // 是否安装仅供测试的价格配置。
	}{{"default-disabled", false}, {"configured-purchase-reconnect", true}} {
		t.Run(candidate.name, func(t *testing.T) {
			verifyHostingPurchaseBrowser(t, repo, build, work, candidate.enabled)
		})
	}
}

// verifyHostingPurchaseBrowser 启动双节点和静态站点，浏览器完全通过常规登录与游戏连接访问它们。
func verifyHostingPurchaseBrowser(t *testing.T, repo, build, work string, enabled bool) {
	t.Helper()
	ctx, store, endpoint, route := startChallengePlayerDeathServer(t, func(cfg *devconfig.Config) {
		if enabled {
			enableHostingTestPrices(cfg)
		}
	})
	account, password := "hostui_"+domain.ID()[:10], domain.ID()[:16]
	login := &pb.LoginResult{}
	rpc(t, endpoint, "", p.MsgC2SLocalRegister, &pb.LocalLoginRequest{Account: account, Password: password}, login, 0)
	role := createDirectoryRole(t, endpoint, login.AccountToken, route.origin.OriginID, "托管触屏")
	roleID := domain.Hex(role.Key.RoleId)
	seedWallet := domain.Wallet{BoundGold: 10, Gold: 100_000}
	const balance = int64(23*3600 - 15)
	seed, err := store.DB.Collection("roles").UpdateOne(ctx, bson.M{"_id": roleID}, bson.M{"$set": bson.M{"wallet": seedWallet, "hosting_seconds": balance}})
	must(t, err)
	if seed.ModifiedCount != 1 {
		t.Fatal("未准备好隔离角色的初始钱包和托管余量")
	}
	upstream, err := url.Parse("http://" + store.Cfg.AccountAddress)
	must(t, err)
	mux := http.NewServeMux()
	mux.Handle("/account/rpc", http.StripPrefix("/account", httputil.NewSingleHostReverseProxy(upstream)))
	mux.Handle("/", http.FileServer(http.Dir(build)))
	web := httptest.NewServer(mux)
	defer web.Close()
	evidence, err := os.MkdirTemp(work, "hosting-purchase-browser-")
	must(t, err)
	manifest := filepath.Join(evidence, "manifest.json")
	// 一次性凭据仅通过子进程环境传递，不写入浏览器报告。
	raw, err := json.Marshal(map[string]any{"baseUrl": web.URL, "endpoint": route.assignment.GameWSURL,
		"region": route.origin.RegionName, "server": route.origin.Name, "roleName": role.Name,
		"ruleVersion": store.Cfg.Fixture.RuleVersion, "enabled": enabled})
	must(t, err)
	must(t, os.WriteFile(manifest, raw, 0600))
	t.Logf("托管补时触屏验收目录 %s", evidence)
	browserCtx, stop := context.WithTimeout(ctx, 150*time.Second)
	defer stop()
	command := exec.CommandContext(browserCtx, "node", filepath.Join(repo, "tools/client/verify_hosting_purchase_browser.cjs"), manifest)
	command.Dir = repo
	command.Env = append(os.Environ(), "MINIMIR_HOSTING_BROWSER_ACCOUNT="+account, "MINIMIR_HOSTING_BROWSER_PASSWORD="+password)
	log, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("托管补时触屏验收失败：%v\n%s", err, log)
	}
	t.Logf("%s", log)
	stored, err := store.Role(ctx, roleID)
	must(t, err)
	opQuery := bson.M{"role_id": roleID, "kind": persistence.AssetBuyHostingTime}
	operations := store.DB.Collection("operations")
	count, err := operations.CountDocuments(ctx, opQuery)
	must(t, err)
	if !enabled {
		if count != 0 || stored.HostingSeconds != balance || stored.Wallet != seedWallet {
			t.Fatal("默认闭合入口改变了原托管时长或钱包", stored.HostingSeconds, stored.Wallet, count)
		}
		return
	}
	var op persistence.Operation // 浏览器确认的一笔原操作，断线前后共用同一身份。
	must(t, operations.FindOne(ctx, opQuery).Decode(&op))
	if op.HostingPurchase == nil {
		t.Fatal("浏览器付款未留下永久补时回执", op.Code)
	}
	quote := op.HostingPurchase.Quote
	var interaction struct {
		Purchase struct {
			OpID             string // 浏览器首个付款及重放请求共用的操作标识。
			QuoteID          string // 触屏确认页接受的原报价标识。
			RequestedSeconds uint32 // 界面选择并实际发送的原时长。
			BalanceSeconds   int64  // 重连后浏览器观察到的实际到账余额。
			SameRequest      bool   // 重新连接后原付款载荷逐字节不变。
			SameReceipt      bool   // MongoDB 重放的业务回执逐字节不变。
		}
	}
	browserReport, err := os.ReadFile(filepath.Join(evidence, "hosting-purchase-browser-report.json"))
	must(t, err)
	must(t, json.Unmarshal(browserReport, &interaction))
	observed := interaction.Purchase
	if op.ID != "asset:"+roleID+":"+observed.OpID || quote.QuoteID != observed.QuoteID ||
		quote.RequestedSeconds != observed.RequestedSeconds || stored.HostingSeconds != observed.BalanceSeconds || !observed.SameRequest || !observed.SameReceipt {
		t.Fatal("浏览器触屏后的原请求、报价或到账与永久操作不一致", observed, op.ID)
	}
	if count != 1 || op.Code != 0 || quote.RequestedSeconds != 30 || quote.BoundGoldCost != 10 || quote.GoldCost != 21 ||
		stored.Wallet.BoundGold != 0 || stored.Wallet.Gold != seedWallet.Gold-21 || stored.HostingSeconds != balance+30 ||
		op.HostingPurchase.BalanceSeconds != stored.HostingSeconds {
		t.Fatalf("取消或重连后成交不唯一，或原报价与资产不一致：笔数=%d 回执=%+v 钱包=%+v 余额=%d",
			count, op.HostingPurchase, stored.Wallet, stored.HostingSeconds)
	}
}
