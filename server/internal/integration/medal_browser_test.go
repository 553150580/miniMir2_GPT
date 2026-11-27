//go:build creator_medal

// 本文件对照 Creator 实包触屏收到的勋章回执与隔离数据库原资产。
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
	"strconv"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	"minimir/internal/persistence"
)

// medalTouchSpend 是浏览器收到的实际材料扣量。64 位字段使用字符串。
type medalTouchSpend struct {
	ID       string
	Version  string
	Quantity string
	Bound    bool
}

// medalTouchReceipt 记录原报价、操作号和服务端确认后的新档位。
type medalTouchReceipt struct {
	Op, Quote          string
	BeforeLevel, Level uint32
	Version            string
	Fragments          []medalTouchSpend
}

// medalTouchReport 将实际触屏、构建身份和永久回执绑定。
type medalTouchReport struct {
	Passed, Enabled                   bool
	FixtureSha256, ClientBundleSha256 string
	Receipts                          []medalTouchReceipt
}

// TestMedalCreatorTouchMongo 对默认包和勋章候选包进行相同的手机触屏与数据库核验。
func TestMedalCreatorTouchMongo(t *testing.T) {
	webRoot := os.Getenv("MINIMIR_MEDAL_BROWSER_WEB_ROOT")
	if webRoot == "" || os.Getenv("MINIMIR_MONGO_E2E") != "1" || os.Getenv("MINIMIR_INTEGRATION_TEST_MONGO_URI") == "" {
		t.Skip("勋章触屏需要独立 Creator Web Mobile 和受控 MongoDB 副本集")
	}
	mode := os.Getenv("MINIMIR_MEDAL_BROWSER_MODE")
	if mode != "default" && mode != "medal" {
		t.Fatal("须指定默认包或勋章候选包")
	}
	repo, err := filepath.Abs("../../..")
	must(t, err)
	build, err := creatorBrowserBuildPath(repo, webRoot)
	must(t, err)
	bundle, err := os.ReadFile(filepath.Join(build, "assets", "main", "index.js"))
	must(t, err)
	bundleHash := persistence.Hash(bundle)
	enabled := mode == "medal"
	fixtureFile := filepath.Join(repo, "config", "development", "first-loop.json")
	var fixture devconfig.Fixture
	if enabled {
		fixtureFile, err = filepath.EvalSymlinks(os.Getenv("MINIMIR_MEDAL_BROWSER_FIXTURE"))
		must(t, err)
		relative, err := filepath.Rel(filepath.Join(repo, ".work", "m4-medal-projection"), fixtureFile)
		must(t, err)
		if !filepath.IsLocal(relative) || relative == "." {
			t.Fatal("候选只能读取本轮导出的隔离规则")
		}
		fixture = readMedalFixture(t, fixtureFile)
	} else {
		raw, err := os.ReadFile(fixtureFile)
		must(t, err)
		must(t, json.Unmarshal(raw, &fixture))
		fixture.Hash = persistence.Hash(raw)
		must(t, fixture.Validate())
	}
	if fixture.RuleVersion != 30 || (fixture.Medal != nil) != enabled {
		t.Fatal("勋章触屏缺少实际选用的规则 30 夹具")
	}
	ctx, store, account, route := startChallengePlayerDeathServer(t, func(cfg *devconfig.Config) {
		cfg.Fixture, cfg.FixtureFile = fixture, fixtureFile
	})
	trial := worldBossRewardTrial{ctx: ctx, store: store, endpoint: account, route: route}
	role := trial.create(t, "勋章触屏", time.Now().Add(-time.Hour), false)
	roleID := domain.Hex(role.summary.Key.RoleId)
	before, weapon, bound, free := seedMedalTouchRole(t, ctx, store, roleID, &fixture)
	definition, known := fixture.Item(weapon.ItemID)
	if !known {
		t.Fatal("默认装备列缺少原木剑")
	}

	// 原账号请求同源代理，游戏票据照常连接本轮隔离游戏服。
	upstream, err := url.Parse(account)
	must(t, err)
	upstream.Path = ""
	mux := http.NewServeMux()
	mux.Handle("/account/rpc", http.StripPrefix("/account", httputil.NewSingleHostReverseProxy(upstream)))
	mux.Handle("/", http.FileServer(http.Dir(build)))
	web := httptest.NewServer(mux)
	defer web.Close()
	evidence, err := os.MkdirTemp(filepath.Join(repo, ".work"), "medal-browser-")
	must(t, err)
	manifest := filepath.Join(evidence, "manifest.json")
	data := map[string]any{"mode": mode, "enabled": enabled, "role": role.summary.Name, "region": route.origin.RegionName,
		"server": route.origin.Name, "endpoint": route.assignment.GameWSURL, "baseUrl": web.URL,
		"normalMap": fixture.Map.ID, "ruleVersion": fixture.RuleVersion, "fixtureSha256": fixture.Hash,
		"clientBundleSha256": bundleHash, "weapon": weapon, "weaponName": definition.Name, "roleLevel": before.Level}
	if enabled {
		costs := make([]string, 0, len(fixture.Medal.Levels))
		for _, level := range fixture.Medal.Levels {
			costs = append(costs, strconv.FormatInt(level.FragmentCost, 10))
		}
		data["boundFragment"], data["freeFragment"], data["fragmentItemId"], data["levelCosts"] = bound, free, fixture.Medal.FragmentItemID, costs
	}
	raw, err := json.Marshal(data)
	must(t, err)
	must(t, os.WriteFile(manifest, raw, 0600))
	t.Log("勋章触屏验收目录 " + evidence)
	browserCtx, cancel := context.WithTimeout(ctx, 210*time.Second)
	defer cancel()
	command := exec.CommandContext(browserCtx, "node", filepath.Join(repo, "tools/client/verify_medal_browser.cjs"), manifest)
	command.Dir = repo
	command.Env = append(os.Environ(), "MINIMIR_MEDAL_BROWSER_ACCOUNT="+role.account, "MINIMIR_MEDAL_BROWSER_PASSWORD="+role.password)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("勋章触屏验收失败 %v\n%s", err, output)
	}
	t.Logf("%s", output)
	verifyMedalTouchLedger(t, ctx, store, evidence, before, bound, free, &fixture, bundleHash)
}

// seedMedalTouchRole 冻结二级角色、真实装备槽和两个拥有独立来源的隔离材料堆。
func seedMedalTouchRole(t *testing.T, ctx context.Context, store *persistence.Store, roleID string,
	fixture *devconfig.Fixture) (domain.Role, domain.Item, *domain.Item, *domain.Item) {
	t.Helper()
	initial, err := store.Role(ctx, roleID)
	must(t, err)
	index := slices.IndexFunc(initial.Items, func(item domain.Item) bool { return item.ItemID == 218 })
	if index < 0 || len(initial.Equipped) != 0 || initial.Medal != nil {
		t.Fatal("隔离角色前态已有勋章，或缺少初始木剑")
	}
	weapon := persistence.InitializeItemDurability(initial.Items[index], fixture)
	items := slices.Delete(initial.Items, index, index+1)
	var bound, free *domain.Item
	if rules := fixture.Medal; rules != nil {
		bound = &domain.Item{ID: domain.ID(), ItemID: rules.FragmentItemID, Quantity: 10, Version: 1, Bound: true,
			SourceKind: "creator_medal_fixture", SourceID: domain.ID()}
		free = &domain.Item{ID: domain.ID(), ItemID: rules.FragmentItemID, Quantity: 1, Version: 1,
			SourceKind: "creator_medal_fixture", SourceID: domain.ID()}
		items = append(items, *free, *bound)
	}
	wallet := domain.Wallet{Gold: 37, BoundGold: 59, Yuanbao: 17, BoundYuanbao: 23}
	equipped := []domain.Equipped{{Slot: devconfig.WeaponSlot, Item: weapon}}
	update, err := store.DB.Collection("roles").UpdateByID(ctx, roleID,
		bson.M{"$set": bson.M{"items": items, "equipped": equipped, "level": uint32(2), "wallet": wallet}})
	must(t, err)
	before, err := store.Role(ctx, roleID)
	must(t, err)
	if update.ModifiedCount != 1 || before.Level != 2 || before.Medal != nil || before.Wallet != wallet || !slices.Equal(before.Equipped, equipped) {
		t.Fatal("勋章隔离触屏角色前态未保存")
	}
	return before, weapon, bound, free
}

// verifyMedalTouchLedger 对照浏览器原回执、数据库操作、审计和装备钱包守恒。
func verifyMedalTouchLedger(t *testing.T, ctx context.Context, store *persistence.Store, evidence string,
	before domain.Role, bound, free *domain.Item, fixture *devconfig.Fixture, bundleHash string) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(evidence, "medal-browser-report.json"))
	must(t, err)
	var report medalTouchReport
	must(t, json.Unmarshal(raw, &report))
	enabled := fixture.Medal != nil
	if !report.Passed || report.Enabled != enabled || report.FixtureSha256 != fixture.Hash || report.ClientBundleSha256 != bundleHash {
		t.Fatal("浏览器没有通过当前实包与夹具", report)
	}
	after, err := store.Role(ctx, before.ID)
	must(t, err)
	if before.Wallet != after.Wallet || !slices.Equal(before.Equipped, after.Equipped) {
		t.Fatal("勋章触屏改变了原钱包或普通穿戴")
	}
	if !enabled {
		if len(report.Receipts) != 0 || after.Medal != nil || !slices.Equal(before.Items, after.Items) {
			t.Fatal("默认规则触屏留下勋章或修改了原背包")
		}
		t.Log("默认实包原装备可见，勋章入口未开放，原资产一致")
		return
	}
	if len(report.Receipts) != 2 || after.Medal == nil || after.Medal.Level != 2 || after.Medal.Version != 2 ||
		after.EconomicSeq != before.EconomicSeq+2 {
		t.Fatal("两次触屏没有且仅有两笔永久提升", report.Receipts)
	}
	// 首次混合投入两堆，全部用掉非绑堆；第二次只提交升级后的绑定堆。
	expect := [][]medalTouchSpend{{{free.ID, "1", "1", false}, {bound.ID, "1", "1", true}}, {{bound.ID, "2", "3", true}}}
	for index, row := range report.Receipts {
		operation := waitMedalOperation(t, ctx, store, before.ID, row.Op)
		receipt := operation.Medal.Proto()
		level := uint32(index + 1)
		if row.BeforeLevel != level-1 || row.Level != level || row.Version != strconv.Itoa(index+1) ||
			domain.Hex(receipt.QuoteId) != row.Quote || receipt.BeforeLevel != row.BeforeLevel ||
			receipt.Medal.Level != row.Level || strconv.FormatUint(receipt.Medal.Version, 10) != row.Version ||
			len(receipt.Fragments) != len(row.Fragments) || !slices.Equal(expect[index], row.Fragments) {
			t.Fatal("触屏的原勋章提升回执与数据库不一致", row, receipt)
		}
		for i, spent := range receipt.Fragments {
			if view := (medalTouchSpend{domain.Hex(spent.InstanceId), strconv.FormatUint(spent.Version, 10),
				strconv.FormatInt(spent.Quantity, 10), spent.Bound}); view != row.Fragments[i] {
				t.Fatal("浏览器收到的碎片 ID、版本、数量或绑定与原回执不同", view)
			}
		}
	}
	if slices.ContainsFunc(after.Items, func(item domain.Item) bool { return item.ID == free.ID }) {
		t.Fatal("已经用尽的非绑碎片堆仍存在")
	}
	i := slices.IndexFunc(after.Items, func(item domain.Item) bool { return item.ID == bound.ID })
	if i < 0 || after.Items[i].Quantity != 6 || after.Items[i].Version != 3 || !after.Items[i].Bound {
		t.Fatal("绑定碎片余量、版本或归属发生额外变化")
	}
	audit, err := store.DB.Collection("audit_outbox").CountDocuments(ctx, bson.M{"role_id": before.ID, "kind": persistence.AssetMedalCommit, "result_code": 0})
	must(t, err)
	if audit != 2 {
		t.Fatal("触屏勋章原结算缺少对应审计", audit)
	}
	t.Log("两次真实触屏提升中首次混用原绑定与非绑堆，角色二级封顶，原装备和四钱包守恒")
}
