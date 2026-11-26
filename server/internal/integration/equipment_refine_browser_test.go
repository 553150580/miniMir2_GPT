//go:build creator_equipment_refine

// 本文件用实际 Creator 触屏和独立 MongoDB 核对洗炼的生成、放弃、采纳与原资源守恒。
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
	"reflect"
	"slices"
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

const (
	refineTouchSwordID  = 220    // 规则 30 原精良乌木剑。
	refineTouchStoneID  = 900051 // 隔离配置的原洗炼石模板。
	refineTouchGold     = 1000   // 乌木剑需求等级所属首档，每次实际生成的金币。
	refineTouchGenerate = 1      // 支付生成新候选。
	refineTouchAdopt    = 2      // 采纳原已付候选。
	refineTouchDiscard  = 3      // 放弃原已付候选。
)

// refineTouchOperation 保存浏览器实际收到的原请求身份，账本真值还由 Go 查询。
type refineTouchOperation struct {
	Op            string // 原经济操作的 128 位标识。
	Action        uint32 // 实际接收的生成或决定命令。
	Quote         string // 已确认报价，决定时为空。
	Pending       string // 决定所用的原已付编号，生成时为空。
	ResultPending string // 回执采用的新生成或已决定的待选原号。
	Revision      string // 原请求采用的角色版本，使用十进制无损比较。
}

// refineTouchReport 记录触屏通过的界面分支和实际返回的原操作，不直接作为持久状态的权威。
type refineTouchReport struct {
	Passed, Enabled, OldEquipmentVisible, DefaultRefineHidden bool
	BindingCancelled, BindingGranted, LeftEmpty, RightVisible bool
	ImmediateBound, DiscardPreserved, SecondQuoteNonbound     bool
	Adopted, ReopenBoundOff                                   bool
	FixtureSha256, ClientBundleSha256                         string
	Operations                                                []refineTouchOperation
}

// TestEquipmentRefineCreatorTouchMongo 让两种真实网页包使用同一测试账号创建和原库存准备方式。
func TestEquipmentRefineCreatorTouchMongo(t *testing.T) {
	webRoot := os.Getenv("MINIMIR_EQUIPMENT_REFINE_BROWSER_WEB_ROOT")
	if webRoot == "" || os.Getenv("MINIMIR_MONGO_E2E") != "1" || os.Getenv("MINIMIR_INTEGRATION_TEST_MONGO_URI") == "" {
		t.Skip("洗炼触屏需要已核验的真实 Creator 构建和受控 MongoDB 副本集")
	}
	mode := os.Getenv("MINIMIR_EQUIPMENT_REFINE_BROWSER_MODE")
	if mode != "default" && mode != "equipment-refine" {
		t.Fatal("须指定默认包或开放洗炼的隔离包")
	}
	enabled := mode == "equipment-refine"
	repo, err := filepath.Abs("../../..")
	must(t, err)
	build, err := creatorBrowserBuildPath(repo, webRoot)
	must(t, err)
	bundle, err := os.ReadFile(filepath.Join(build, "assets", "main", "index.js"))
	must(t, err)
	bundleHash := persistence.Hash(bundle)
	fixture, fixtureFile := readRefineTouchFixture(t, repo, enabled)
	ctx, store, account, route := startChallengePlayerDeathServer(t, func(cfg *devconfig.Config) {
		cfg.Fixture, cfg.FixtureFile = fixture, fixtureFile
	})
	login, role := &pb.LoginResult{}, &pb.RoleSummary{}
	name, password := "rf_touch_"+domain.ID()[:8], domain.ID()[:16]
	rpc(t, account, "", p.MsgC2SLocalRegister, &pb.LocalLoginRequest{Account: name, Password: password}, login, 0)
	rpc(t, account, login.AccountToken, p.MsgC2SCreateRole, &pb.CreateRoleRequest{
		OriginServerId: route.origin.OriginID, Slot: 1, Job: 0, Name: "洗炼触屏" + domain.ID()[:5], OpId: domain.Bytes(domain.ID()),
	}, role, 0)
	before, target, bound, free := seedRefineTouchRole(t, ctx, store, domain.Hex(role.Key.RoleId), &fixture)
	definition, exists := fixture.Item(target.ItemID)
	if !exists {
		t.Fatal("当前实包缺少原乌木剑目录")
	}

	// 隔离代理只为浏览器提供原账号 RPC 和静态包，游戏直连这次的隔离 WebSocket 地址。
	upstream, err := url.Parse(account)
	must(t, err)
	upstream.Path = ""
	mux := http.NewServeMux()
	mux.Handle("/account/rpc", http.StripPrefix("/account", httputil.NewSingleHostReverseProxy(upstream)))
	mux.Handle("/", http.FileServer(http.Dir(build)))
	web := httptest.NewServer(mux)
	defer web.Close()
	evidence, err := os.MkdirTemp(filepath.Join(repo, ".work"), "equipment-refine-browser-")
	must(t, err)
	manifest := filepath.Join(evidence, "manifest.json")
	data := map[string]any{"enabled": enabled, "mode": mode, "role": role.Name, "region": route.origin.RegionName, "server": route.origin.Name,
		"endpoint": route.assignment.GameWSURL, "baseUrl": web.URL, "normalMap": fixture.Map.ID, "ruleVersion": fixture.RuleVersion,
		"fixtureSha256": fixture.Hash, "clientBundleSha256": bundleHash, "target": target, "weaponName": definition.Name, "roleLevel": before.Level,
		"wallet": map[string]string{"gold": strconv.FormatInt(before.Wallet.Gold, 10), "boundGold": strconv.FormatInt(before.Wallet.BoundGold, 10),
			"yuanbaoUnits": strconv.FormatInt(before.Wallet.Yuanbao, 10), "boundYuanbaoUnits": strconv.FormatInt(before.Wallet.BoundYuanbao, 10)}}
	if enabled {
		data["boundStones"], data["freeStones"] = bound, free
		data["stoneItemId"], data["stoneCost"], data["goldCost"] = refineTouchStoneID, "2", strconv.Itoa(refineTouchGold)
	}
	raw, err := json.Marshal(data)
	must(t, err)
	must(t, os.WriteFile(manifest, raw, 0600))
	t.Log("洗炼触屏验收目录 " + evidence)
	caseCtx, stop := context.WithTimeout(ctx, 250*time.Second)
	defer stop()
	command := exec.CommandContext(caseCtx, "node", filepath.Join(repo, "tools/client/verify_equipment_refine_browser.cjs"), "--manifest", manifest, "--output", evidence)
	command.Dir = repo
	command.Env = append(os.Environ(), "MINIMIR_EQUIPMENT_REFINE_BROWSER_ACCOUNT="+name, "MINIMIR_EQUIPMENT_REFINE_BROWSER_PASSWORD="+password)
	log, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("实际洗炼触屏执行失败：%v\n%s", err, log)
	}
	t.Logf("%s", log)
	verifyRefineTouchLedger(t, ctx, store, evidence, before, target, bound, free, &fixture, bundleHash)
}

// readRefineTouchFixture 核验只有本轮的候选目录可以打开洗炼，默认规则 30 仍使用原配置。
func readRefineTouchFixture(t *testing.T, repo string, enabled bool) (devconfig.Fixture, string) {
	t.Helper()
	file := filepath.Join(repo, "config", "development", "first-loop.json")
	if enabled {
		input := os.Getenv("MINIMIR_EQUIPMENT_REFINE_BROWSER_FIXTURE")
		resolved, err := filepath.EvalSymlinks(input)
		must(t, err)
		file, err = filepath.Abs(resolved)
		must(t, err)
		if file != filepath.Join(repo, ".work", "m4-equipment-refine-projection", "fixture.json") {
			t.Fatal("只接收本轮已登记的洗炼隔离投影")
		}
	}
	raw, err := os.ReadFile(file)
	must(t, err)
	var fixture devconfig.Fixture
	must(t, json.Unmarshal(raw, &fixture))
	fixture.Hash = persistence.Hash(raw)
	must(t, fixture.Validate())
	rules := fixture.EquipmentRefine
	if fixture.RuleVersion != 30 || (rules != nil) != enabled {
		t.Fatal("默认和隔离模式需分别配置规则 30 的洗炼状态")
	}
	if enabled {
		sword, exists := fixture.Item(refineTouchSwordID)
		if !exists || sword.RequiredLevel == nil || rules.StoneItemID != refineTouchStoneID || !rules.Target(refineTouchSwordID, sword.DropQuality) {
			t.Fatal("隔离包没有原精良乌木剑和原洗炼石")
		}
		quality, qualityKnown := rules.Quality(sword.DropQuality)
		gold, priceKnown := rules.GoldCost(*sword.RequiredLevel)
		if !qualityKnown || quality.SlotCount != 1 || quality.StoneCost != 2 || !priceKnown || gold != refineTouchGold {
			t.Fatal("隔离包精良档位、石数和金币与原配置不同")
		}
	}
	return fixture, file
}

// seedRefineTouchRole 只准备隔离资格与原材料。之后所有洗炼资产写入都来自真实触控。
func seedRefineTouchRole(t *testing.T, ctx context.Context, store *persistence.Store, roleID string, fixture *devconfig.Fixture) (
	domain.Role, domain.Item, []domain.Item, []domain.Item,
) {
	t.Helper()
	initial, err := store.Role(ctx, roleID)
	must(t, err)
	if len(initial.Equipped) != 0 || initial.EquipmentEnhancement != nil {
		t.Fatal("隔离初始装备和强化档不是空态")
	}
	definition, exists := fixture.Item(refineTouchSwordID)
	if !exists || !definition.IsEquipment || definition.DropQuality != 1 {
		t.Fatal("隔离前态缺少可复核原精良乌木剑")
	}
	target := persistence.InitializeItemDurability(domain.Item{ID: domain.ID(), ItemID: definition.ID, Version: 2, Quantity: 1,
		Quality: definition.DropQuality, SourceKind: "monster_drop", SourceID: "1"}, fixture)
	if !target.HasValidDurability() || target.DurabilityU <= 0 {
		t.Fatal("隔离原乌木剑必须有原耐久")
	}
	items := append([]domain.Item{target}, initial.Items...)
	var bound, free []domain.Item
	if fixture.EquipmentRefine != nil {
		newStone := func(isBound bool) domain.Item {
			return domain.Item{ID: domain.ID(), ItemID: refineTouchStoneID, Version: 1, Quantity: 1, Bound: isBound,
				SourceKind: "equipment_recycle_refine", SourceID: domain.ID()}
		}
		for range 2 {
			bound, free = append(bound, newStone(true)), append(free, newStone(false))
		}
		items = append(items, bound...)
		items = append(items, free...)
	}
	wallet := domain.Wallet{Gold: 1037, BoundGold: 1059, Yuanbao: 17, BoundYuanbao: 23}
	update, err := store.DB.Collection("roles").UpdateByID(ctx, roleID,
		bson.M{"$set": bson.M{"items": items, "level": uint32(5), "wallet": wallet}})
	must(t, err)
	before, err := store.Role(ctx, roleID)
	must(t, err)
	if update.ModifiedCount != 1 || before.Level != 5 || before.Wallet != wallet || !slices.Equal(before.Items, items) || len(items) > before.Capacity {
		t.Fatal("隔离库存没有完整保存原装备、石堆及四类钱包")
	}
	return before, target, bound, free
}

// verifyRefineTouchLedger 核对当前浏览器确实完成两次付费与两次免费决定，原左右状态和账本对应。
func verifyRefineTouchLedger(t *testing.T, ctx context.Context, store *persistence.Store, evidence string, before domain.Role, target domain.Item,
	bound, free []domain.Item, fixture *devconfig.Fixture, bundleHash string,
) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(evidence, "equipment-refine-browser-report.json"))
	must(t, err)
	var report refineTouchReport
	must(t, json.Unmarshal(raw, &report))
	enabled := fixture.EquipmentRefine != nil
	if !report.Passed || report.Enabled != enabled || !report.OldEquipmentVisible || report.FixtureSha256 != fixture.Hash || report.ClientBundleSha256 != bundleHash {
		t.Fatal("报告不是本次实包和原乌木剑的实际触屏")
	}
	after, err := store.Role(ctx, before.ID)
	must(t, err)
	operations, audits, quotes := store.DB.Collection("operations"), store.DB.Collection("audit_outbox"), store.DB.Collection("equipment_refine_quotes")
	selector := bson.M{"role_id": before.ID, "kind": persistence.AssetEquipmentRefine}
	opCount, err := operations.CountDocuments(ctx, selector)
	must(t, err)
	auditCount, err := audits.CountDocuments(ctx, selector)
	must(t, err)
	quoteCount, err := quotes.CountDocuments(ctx, bson.M{"_id": before.ID})
	must(t, err)
	if after.Level != before.Level || after.Capacity != before.Capacity || after.SourceSeq != before.SourceSeq ||
		!slices.Equal(after.Equipped, before.Equipped) || after.EconomicSeq != before.EconomicSeq+opCount || opCount != auditCount || quoteCount != 0 {
		t.Fatal("真实触控产生额外账本、角色变更或遗留报价", opCount, auditCount, quoteCount)
	}
	if !enabled {
		if !report.DefaultRefineHidden || len(report.Operations) != 0 || opCount != 0 || after.Wallet != before.Wallet || !slices.Equal(after.Items, before.Items) {
			t.Fatal("默认包显示洗炼或留下永久事实")
		}
		t.Log("默认包原装备详情可见；洗炼入口、原回执、原审计和已付词条均无")
		return
	}
	if opCount != 4 || len(report.Operations) != 4 || !report.BindingCancelled || !report.BindingGranted || !report.LeftEmpty || !report.RightVisible ||
		!report.ImmediateBound || !report.DiscardPreserved || !report.SecondQuoteNonbound || !report.Adopted || !report.ReopenBoundOff {
		t.Fatal("缺少原确定绑定、左右观察、免费放弃或采纳的实际触控分支", report)
	}
	var receipts [4]*persistence.EquipmentRefineReceipt
	last := target
	for index, action := range []uint32{refineTouchGenerate, refineTouchDiscard, refineTouchGenerate, refineTouchAdopt} {
		observed := report.Operations[index]
		key := bson.M{"_id": "asset:" + before.ID + ":" + observed.Op, "role_id": before.ID}
		var saved persistence.Operation
		must(t, operations.FindOne(ctx, key).Decode(&saved))
		receipt := saved.EquipmentRefine
		inputPending := ""
		if action != refineTouchGenerate {
			inputPending = last.PendingResultID
		}
		if receipt == nil || saved.Kind != persistence.AssetEquipmentRefine || saved.Code != 0 || saved.CommandSeq != before.EconomicSeq+int64(index)+1 ||
			receipt.Action != action || observed.Action != action || observed.Revision != strconv.FormatInt(saved.Revision-1, 10) ||
			observed.Pending != inputPending || receipt.PendingID != observed.ResultPending || receipt.QuoteID != observed.Quote || receipt.Before != last || receipt.Equipped ||
			receipt.Rule != fixture.RuleVersion || receipt.Config != fixture.Hash {
			t.Fatal("原操作、会话看到的状态与永久回执不符", index, receipt)
		}
		var audit struct {
			Receipt *persistence.EquipmentRefineReceipt `bson:"equipment_refine"` // 原同事务产物及实际材料。
		}
		must(t, audits.FindOne(ctx, key).Decode(&audit))
		if !reflect.DeepEqual(audit.Receipt, receipt) {
			t.Fatal("洗炼审计没有确认同一份永久事实", index)
		}
		receipts[index], last = receipt, receipt.Target
	}
	firstPending := verifyRefineTouchGeneration(t, receipts[0], bound, domain.Wallet{BoundGold: refineTouchGold})
	verifyRefineTouchDecision(t, receipts[1], firstPending, false)
	secondPending := verifyRefineTouchGeneration(t, receipts[2], free, domain.Wallet{Gold: refineTouchGold})
	verifyRefineTouchDecision(t, receipts[3], secondPending, true)
	if firstPending.PendingResultID == secondPending.PendingResultID {
		t.Fatal("独立付费生成复用了原待选身份")
	}
	spentIDs := make(map[string]bool, 4)
	for _, item := range append(slices.Clone(bound), free...) {
		spentIDs[item.ID] = true
	}
	items := slices.DeleteFunc(slices.Clone(before.Items), func(item domain.Item) bool { return spentIDs[item.ID] })
	items[0] = last
	wallet := before.Wallet
	wallet.Gold -= refineTouchGold
	wallet.BoundGold -= refineTouchGold
	if after.Wallet != wallet || !slices.Equal(after.Items, items) {
		t.Fatal("触屏最终原库存、武器或四钱包不守恒", after.Wallet)
	}
	t.Log("两次生成各扣两颗原石，绑定金币与非绑金币各1000；放弃及采纳均零消费，原武器与四笔永久回执、审计一致")
}

// verifyRefineTouchGeneration 只接受两颗原石与一次正确的钱包消费，右侧变化不能改左侧。
func verifyRefineTouchGeneration(t *testing.T, result *persistence.EquipmentRefineReceipt, stones []domain.Item, spend domain.Wallet) domain.Item {
	t.Helper()
	after := result.Target
	if result.Spend != spend || len(result.Stones) != len(stones) || len(result.QuoteID) != 32 || len(after.PendingResultID) != 32 ||
		result.PendingID != after.PendingResultID || after.Refine.Candidate[0].AttributeID == 0 || after.Refine.Candidate[0].ValueUnits <= 0 ||
		after.Refine.Candidate != (domain.EquipmentAffixes{after.Refine.Candidate[0]}) || len(result.Seed) != 64 || !after.Bound {
		t.Fatal("生成没有精确消费原材料、改绑或建立单槽右侧待选", result)
	}
	seen := map[string]bool{}
	for _, material := range result.Stones {
		if material.Quantity != 1 || !slices.Contains(stones, material.Item) || seen[material.Item.ID] {
			t.Fatal("生成扣除了原确认之外的石堆", material)
		}
		seen[material.Item.ID] = true
	}
	want := result.Before
	want.Bound, want.PendingResultID, want.Refine.Candidate = true, after.PendingResultID, after.Refine.Candidate
	want.Version++
	if after != want {
		t.Fatal("生成改变原装备的左侧词条或非洗炼资产", after)
	}
	return after
}

// verifyRefineTouchDecision 限定免费决定只处置原右侧，保留绑定、基础属性及原版本顺序。
func verifyRefineTouchDecision(t *testing.T, result *persistence.EquipmentRefineReceipt, pending domain.Item, adopt bool) {
	t.Helper()
	if len(result.Stones) != 0 || result.Spend != (domain.Wallet{}) || result.PendingID != pending.PendingResultID || result.QuoteID != "" || result.Seed != "" {
		t.Fatal("免费决定扣费、重新抽样或没有处理同一候选", result)
	}
	want := pending
	if adopt {
		want.Refine.Current = pending.Refine.Candidate
	}
	want.PendingResultID, want.Refine.Candidate = "", domain.EquipmentAffixes{}
	want.Version++
	if result.Target != want {
		t.Fatal("决定改错左侧、右侧、原版本或绑定", result)
	}
}
