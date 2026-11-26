//go:build creator_role_cards

// 本文件以同一个实构客户端验证已装配及缺省双月卡规则，核对持久回执、钱包和原角色归属。
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
	"strconv"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
)

// TestRoleCardBrowserMongoWS 通过保存的触屏入口购买两种月卡、领取绑定元宝并追溯未知提交。
func TestRoleCardBrowserMongoWS(t *testing.T) {
	webRoot, fixturePath := os.Getenv("MINIMIR_ROLE_CARD_BROWSER_WEB_ROOT"), os.Getenv("MINIMIR_ROLE_CARD_BROWSER_FIXTURE")
	if webRoot == "" || fixturePath == "" {
		t.Skip("需要独立 Creator Web Mobile 实构和双月卡候选配置")
	}
	repo, err := filepath.Abs("../../..")
	must(t, err)
	work := filepath.Join(repo, ".work")
	build, fixturePath := roleCardBrowserInput(t, work, webRoot), roleCardBrowserInput(t, work, fixturePath)
	bundle, err := os.ReadFile(filepath.Join(build, "assets", "main", "index.js"))
	must(t, err)
	bundleDigest := sha256.Sum256(bundle)
	content, err := os.ReadFile(fixturePath)
	must(t, err)
	var candidate devconfig.Fixture
	must(t, json.Unmarshal(content, &candidate))
	fixtureDigest := sha256.Sum256(content)
	candidate.Hash = hex.EncodeToString(fixtureDigest[:])
	must(t, candidate.Validate())
	if candidate.RoleCards == nil || len(candidate.RoleCards.Products) != 2 {
		t.Fatal("需从已验证候选文件读取完整双月卡配置")
	}
	for _, enabled := range []bool{false, true} {
		name := "default-disabled"
		if enabled {
			name = "role-card-purchase-claim-reconnect"
		}
		t.Run(name, func(t *testing.T) {
			verifyRoleCardBrowser(t, repo, work, build, fixturePath, candidate, hex.EncodeToString(bundleDigest[:]), enabled)
		})
	}
}

// roleCardBrowserInput 只从任务专属工作目录读取实际构建或隔离发布数据。
func roleCardBrowserInput(t *testing.T, work, supplied string) string {
	t.Helper()
	resolved, err := filepath.EvalSymlinks(supplied)
	must(t, err)
	resolved, err = filepath.Abs(resolved)
	must(t, err)
	relative, err := filepath.Rel(work, resolved)
	must(t, err)
	if relative == "." || !filepath.IsLocal(relative) {
		t.Fatal("双月卡触屏验收只接受工作区的隔离产物", resolved)
	}
	return resolved
}

// verifyRoleCardBrowser 启动隔离节点，浏览器只用正常入口，随后核对全部持久结果。
func verifyRoleCardBrowser(t *testing.T, repo, work, build, path string, candidate devconfig.Fixture, bundleDigest string, enabled bool) {
	t.Helper()
	ctx, store, endpoint, route := startChallengePlayerDeathServer(t, func(cfg *devconfig.Config) {
		if cfg.Fixture.RuleVersion != candidate.RuleVersion {
			t.Fatal("双月卡候选必须使用同一客户端规则及资源版本", candidate.RuleVersion, cfg.Fixture.RuleVersion)
		}
		if enabled {
			cfg.Fixture, cfg.FixtureFile = candidate, path
		} else {
			cfg.Fixture.RoleCards = nil
		}
		// 浏览器钱包断言只观察购卡和日领，减缓隔离普通怪的活动。
		for i := range cfg.Fixture.Monsters {
			creature := &cfg.Fixture.Monsters[i]
			creature.HP, creature.MoveMS, creature.AttackMin, creature.AttackMax = 5000, 60_000, 0, 0
		}
	})
	var privilege, hosting devconfig.RoleCardProduct
	for _, item := range candidate.RoleCards.Products {
		switch item.Kind {
		case devconfig.RoleCardKindPrivilege:
			privilege = item
		case devconfig.RoleCardKindHosting:
			hosting = item
		}
	}
	initialWallet := domain.Wallet{Yuanbao: privilege.PriceYuanbaoUnits + hosting.PriceYuanbaoUnits + 251, BoundYuanbao: 113}
	account, password := "cardui_"+domain.ID()[:10], domain.ID()[:16]
	login := &pb.LoginResult{}
	rpc(t, endpoint, "", p.MsgC2SLocalRegister, &pb.LocalLoginRequest{Account: account, Password: password}, login, 0)
	role := createDirectoryRole(t, endpoint, login.AccountToken, route.origin.OriginID, "月卡触屏")
	roleID := domain.Hex(role.Key.RoleId)
	sibling := &pb.RoleSummary{}
	rpc(t, endpoint, login.AccountToken, p.MsgC2SCreateRole, &pb.CreateRoleRequest{OriginServerId: route.origin.OriginID,
		Slot: 2, Job: 0, Name: "隔离同号" + domain.ID()[:5], OpId: domain.Bytes(domain.ID())}, sibling, 0)
	siblingID := domain.Hex(sibling.Key.RoleId)
	beforeSibling, err := store.Role(ctx, siblingID)
	must(t, err)
	seed, err := store.DB.Collection("roles").UpdateByID(ctx, roleID, bson.M{"$set": bson.M{"wallet": initialWallet}})
	must(t, err)
	if seed.ModifiedCount != 1 {
		t.Fatal("隔离角色未获得验收所需的原始钱包")
	}
	upstream, err := url.Parse("http://" + store.Cfg.AccountAddress)
	must(t, err)
	mux := http.NewServeMux()
	mux.Handle("/account/rpc", http.StripPrefix("/account", httputil.NewSingleHostReverseProxy(upstream)))
	mux.Handle("/", http.FileServer(http.Dir(build)))
	web := httptest.NewServer(mux)
	defer web.Close()
	evidence, err := os.MkdirTemp(work, "role-card-browser-")
	must(t, err)
	manifest := filepath.Join(evidence, "manifest.json")
	// 报告从实际运行配置取得价格和钱值，一次性账号认证留在子进程环境。
	raw, err := json.Marshal(map[string]any{"baseUrl": web.URL, "endpoint": route.assignment.GameWSURL,
		"region": route.origin.RegionName, "server": route.origin.Name, "roleName": role.Name, "enabled": enabled,
		"ruleVersion": candidate.RuleVersion, "clientBundleSha256": bundleDigest, "fixtureSha256": candidate.Hash,
		"privilegeProduct": privilege.ID, "hostingProduct": hosting.ID, "privilegeUnits": strconv.FormatInt(privilege.PriceYuanbaoUnits, 10),
		"hostingUnits": strconv.FormatInt(hosting.PriceYuanbaoUnits, 10), "dailyUnits": strconv.FormatInt(candidate.RoleCards.DailyPrivilegeUnits, 10),
		"initialYuanbao": strconv.FormatInt(initialWallet.Yuanbao, 10), "initialBoundYuanbao": strconv.FormatInt(initialWallet.BoundYuanbao, 10)})
	must(t, err)
	must(t, os.WriteFile(manifest, raw, 0600))
	t.Logf("双月卡触屏验收目录 %s", evidence)
	browserCtx, stop := context.WithTimeout(ctx, 130*time.Second)
	defer stop()
	command := exec.CommandContext(browserCtx, "node", filepath.Join(repo, "tools/client/verify_role_card_browser.cjs"), manifest)
	command.Dir = repo
	command.Env = append(os.Environ(), "MINIMIR_ROLE_CARD_BROWSER_ACCOUNT="+account, "MINIMIR_ROLE_CARD_BROWSER_PASSWORD="+password)
	log, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("双月卡触屏验收失败：%v\n%s", err, log)
	}
	t.Logf("%s", log)
	stored, err := store.Role(ctx, roleID)
	must(t, err)
	other, err := store.Role(ctx, siblingID)
	must(t, err)
	if other.Wallet != beforeSibling.Wallet || !other.PrivilegeUntil.IsZero() || !other.HostingCardUntil.IsZero() || other.PrivilegeClaimDay != "" {
		t.Fatal("双月卡权益或资产串到同账号其它角色", other.Wallet, other.PrivilegeUntil, other.HostingCardUntil)
	}
	ops := store.DB.Collection("operations")
	filter := bson.M{"role_id": roleID, "kind": bson.M{"$in": bson.A{persistence.AssetBuyRoleCard, persistence.AssetClaimRoleCardDaily}}}
	count, err := ops.CountDocuments(ctx, filter)
	must(t, err)
	if !enabled {
		if count != 0 || stored.Wallet != initialWallet || !stored.PrivilegeUntil.IsZero() || !stored.HostingCardUntil.IsZero() {
			t.Fatal("缺省规则的隐藏入口写入了卡期或扣款", count, stored.Wallet)
		}
		return
	}
	var report struct {
		Passed    bool
		Purchase  map[string]struct{ OpID, QuoteID string }
		Daily     struct{ OpID, OperatingDay, GrantedUnits string }
		Wallet    struct{ YuanbaoUnits, BoundYuanbaoUnits string }
		Reconnect struct{ SameRequest, SameReceipt bool }
	}
	raw, err = os.ReadFile(filepath.Join(evidence, "role-card-browser-report.json"))
	must(t, err)
	must(t, json.Unmarshal(raw, &report))
	if !report.Passed || !report.Reconnect.SameRequest || !report.Reconnect.SameReceipt || count != 3 {
		t.Fatal("购卡、日领及重放未在 Mongo 确认恰好三笔原操作", count, report)
	}
	for _, entry := range []struct {
		Name    string
		Product devconfig.RoleCardProduct
		Until   time.Time
	}{{"privilege", privilege, stored.PrivilegeUntil}, {"hosting", hosting, stored.HostingCardUntil}} {
		observed := report.Purchase[entry.Name]
		var op persistence.Operation
		must(t, ops.FindOne(ctx, bson.M{"_id": "asset:" + roleID + ":" + observed.OpID, "kind": persistence.AssetBuyRoleCard}).Decode(&op))
		if op.Code != 0 || op.CardPurchase == nil || op.CardPurchase.Quote.QuoteID != observed.QuoteID ||
			op.CardPurchase.Quote.ProductID != entry.Product.ID || op.CardPurchase.Quote.PriceYuanbaoUnits != entry.Product.PriceYuanbaoUnits ||
			!op.CardPurchase.ValidUntil.Equal(entry.Until) {
			t.Fatal("购买的触屏确认、卡期或价格不对应原 Mongo 回执", entry.Name, op.CardPurchase)
		}
	}
	var daily persistence.Operation
	must(t, ops.FindOne(ctx, bson.M{"_id": "asset:" + roleID + ":" + report.Daily.OpID, "kind": persistence.AssetClaimRoleCardDaily}).Decode(&daily))
	if daily.Code != 0 || daily.CardDaily == nil || daily.CardDaily.OperatingDay != report.Daily.OperatingDay || stored.PrivilegeClaimDay != report.Daily.OperatingDay ||
		daily.CardDaily.GrantedBoundYuanbaoUnits != candidate.RoleCards.DailyPrivilegeUnits ||
		report.Daily.GrantedUnits != strconv.FormatInt(daily.CardDaily.GrantedBoundYuanbaoUnits, 10) ||
		stored.Wallet.Yuanbao != initialWallet.Yuanbao-privilege.PriceYuanbaoUnits-hosting.PriceYuanbaoUnits ||
		stored.Wallet.BoundYuanbao != initialWallet.BoundYuanbao+candidate.RoleCards.DailyPrivilegeUnits ||
		strconv.FormatInt(stored.Wallet.Yuanbao, 10) != report.Wallet.YuanbaoUnits || strconv.FormatInt(stored.Wallet.BoundYuanbao, 10) != report.Wallet.BoundYuanbaoUnits {
		t.Fatal("绑定日领或两种钱包与原角色 Mongo 结果不一致", daily.CardDaily, stored.Wallet, report)
	}
}
