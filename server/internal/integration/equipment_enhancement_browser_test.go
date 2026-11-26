//go:build creator_equipment_enhancement

// 本文件以实际 Creator 触屏验证部位强化的选择与绑定确认，并从隔离原数据库对账。
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

const enhancementTouchStoneCost = 10 // 工作簿 0→1 候选消耗，供当前触屏路径对拍。

// enhancementTouchOperation 是触屏实际收到的原回执摘要。
type enhancementTouchOperation struct {
	Op           string // 原永久操作号。
	Quote        string // 所确认的原报价。
	Slot         uint32 // 本次操作的装备部位。
	BeforeLevel  uint32 // 点击时该部位原等级。
	AfterLevel   uint32 // 确认后强化等级。
	RoleRevision string // 用于确认原报价的角色修订。
	StoneID      string // 实际选择的唯一原强化石堆。
	StoneBound   bool   // 本笔投入对应原绑定身份。
	Quantity     string // 回执携带的实际石数。
}

// enhancementTouchReport 将网页包、实际触控与原操作绑定，永久账本另行查询。
type enhancementTouchReport struct {
	Passed, Enabled, OldEquipmentVisible, DefaultEnhancementHidden bool
	EmptySlotVisible, UnboundQuoteVerified, BoundCancelled         bool
	BoundGranted, PermissionPreserved, ReopenedOff                 bool
	WalletEqual, WeaponEqual                                       bool
	FixtureSha256, ClientBundleSha256                              string
	Operation                                                      *enhancementTouchOperation
	Receipts                                                       []struct {
		Op, Quote, Revision string // 触屏实际收到的原操作、报价与最终修订。
	}
}

// TestEquipmentEnhancementCreatorTouchMongo 在独立运行服上分别复验默认实包和开放强化的实包。
func TestEquipmentEnhancementCreatorTouchMongo(t *testing.T) {
	webRoot := os.Getenv("MINIMIR_EQUIPMENT_ENHANCEMENT_BROWSER_WEB_ROOT")
	if webRoot == "" || os.Getenv("MINIMIR_MONGO_E2E") != "1" || os.Getenv("MINIMIR_INTEGRATION_TEST_MONGO_URI") == "" {
		t.Skip("部位强化触屏需要独立 Creator Web Mobile 和受控 MongoDB 副本集")
	}
	mode := os.Getenv("MINIMIR_EQUIPMENT_ENHANCEMENT_BROWSER_MODE")
	if mode != "default" && mode != "equipment-enhancement" {
		t.Fatal("须指定默认包或部位强化候选包")
	}
	enabled := mode == "equipment-enhancement"
	repo, err := filepath.Abs("../../..")
	must(t, err)
	build, err := creatorBrowserBuildPath(repo, webRoot)
	must(t, err)
	bundle, err := os.ReadFile(filepath.Join(build, "assets", "main", "index.js"))
	must(t, err)
	bundleHash := persistence.Hash(bundle)
	fixture, fixtureFile := readEnhancementTouchFixture(t, repo, enabled)
	ctx, store, account, route := startChallengePlayerDeathServer(t, func(cfg *devconfig.Config) {
		cfg.Fixture, cfg.FixtureFile = fixture, fixtureFile
	})
	login, role := &pb.LoginResult{}, &pb.RoleSummary{}
	name, password := "eq_touch_"+domain.ID()[:8], domain.ID()[:16]
	rpc(t, account, "", p.MsgC2SLocalRegister, &pb.LocalLoginRequest{Account: name, Password: password}, login, 0)
	rpc(t, account, login.AccountToken, p.MsgC2SCreateRole, &pb.CreateRoleRequest{
		OriginServerId: route.origin.OriginID, Slot: 1, Job: 0, Name: "部位触屏" + domain.ID()[:5], OpId: domain.Bytes(domain.ID()),
	}, role, 0)
	before, weapon, bound, free := seedEnhancementTouchRole(t, ctx, store, domain.Hex(role.Key.RoleId), &fixture)
	definition, known := fixture.Item(weapon.ItemID)
	if !known {
		t.Fatal("缺少原角色栏木剑定义")
	}

	// 代理仅供隔离浏览器原账号 RPC，游戏连接直接指向此次新分配的 WebSocket。
	upstream, err := url.Parse(account)
	must(t, err)
	upstream.Path = ""
	mux := http.NewServeMux()
	mux.Handle("/account/rpc", http.StripPrefix("/account", httputil.NewSingleHostReverseProxy(upstream)))
	mux.Handle("/", http.FileServer(http.Dir(build)))
	web := httptest.NewServer(mux)
	defer web.Close()
	evidence, err := os.MkdirTemp(filepath.Join(repo, ".work"), "equipment-enhancement-browser-")
	must(t, err)
	manifest := filepath.Join(evidence, "manifest.json")
	data := map[string]any{"enabled": enabled, "role": role.Name, "region": route.origin.RegionName, "server": route.origin.Name,
		"endpoint": route.assignment.GameWSURL, "baseUrl": web.URL, "normalMap": fixture.Map.ID, "ruleVersion": fixture.RuleVersion,
		"fixtureSha256": fixture.Hash, "clientBundleSha256": bundleHash, "weapon": weapon, "weaponName": definition.Name,
		"roleLevel": before.Level, "beforeLevel": before.EnhancementLevel(devconfig.WeaponSlot)}
	if enabled {
		data["boundStone"], data["freeStone"] = bound, free
		data["stoneItemId"] = fixture.EquipmentEnhancement.StoneItemID
		data["stoneCost"] = strconv.Itoa(enhancementTouchStoneCost)
		data["targetBonusBp"] = fixture.EquipmentEnhancement.BonusBP(1)
	}
	raw, err := json.Marshal(data)
	must(t, err)
	must(t, os.WriteFile(manifest, raw, 0600))
	t.Log("部位强化触屏验收目录 " + evidence)
	caseCtx, stop := context.WithTimeout(ctx, 210*time.Second)
	defer stop()
	command := exec.CommandContext(caseCtx, "node", filepath.Join(repo, "tools/client/verify_equipment_enhancement_browser.cjs"), manifest)
	command.Dir = repo
	command.Env = append(os.Environ(), "MINIMIR_EQUIPMENT_ENHANCEMENT_BROWSER_ACCOUNT="+name, "MINIMIR_EQUIPMENT_ENHANCEMENT_BROWSER_PASSWORD="+password)
	log, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("部位强化触屏验收失败：%v\n%s", err, log)
	}
	t.Logf("%s", log)
	verifyEnhancementTouchLedger(t, ctx, store, evidence, before, bound, &fixture, bundleHash)
}

// readEnhancementTouchFixture 拒绝其它环境输入，并校验实包预期的固定鉴权和候选步长。
func readEnhancementTouchFixture(t *testing.T, repo string, enabled bool) (devconfig.Fixture, string) {
	t.Helper()
	file := filepath.Join(repo, "config", "development", "first-loop.json")
	if enabled {
		input := os.Getenv("MINIMIR_EQUIPMENT_ENHANCEMENT_BROWSER_FIXTURE")
		if input == "" {
			t.Fatal("缺少当前隔离部位强化配置")
		}
		resolved, err := filepath.EvalSymlinks(input)
		must(t, err)
		file, err = filepath.Abs(resolved)
		must(t, err)
		relative, err := filepath.Rel(filepath.Join(repo, ".work", "m4-equipment-enhancement-projection"), file)
		must(t, err)
		if !filepath.IsLocal(relative) || relative == "." {
			t.Fatal("只接受本轮生成的隔离部位强化配置")
		}
	}
	raw, err := os.ReadFile(file)
	must(t, err)
	var fixture devconfig.Fixture
	must(t, json.Unmarshal(raw, &fixture))
	fixture.Hash = persistence.Hash(raw)
	must(t, fixture.Validate())
	rules := fixture.EquipmentEnhancement
	if fixture.RuleVersion != 30 || (rules != nil) != enabled {
		t.Fatal("实包需按原规则 30 分开承载默认与候选条件")
	}
	if enabled {
		step, ok := rules.Level(0)
		if !ok || rules.StoneItemID != 900041 || step.StoneCost != enhancementTouchStoneCost || step.SuccessPPM != 1_000_000 || step.BonusBP != 10 {
			t.Fatal("候选实包须使用工作簿的原 0→1 概率和强化石", step)
		}
	}
	return fixture, file
}

// seedEnhancementTouchRole 只准备独立角色和材料，后续强化通过真实触屏发起。
func seedEnhancementTouchRole(t *testing.T, ctx context.Context, store *persistence.Store, roleID string, fixture *devconfig.Fixture) (
	domain.Role, domain.Item, *domain.Item, *domain.Item,
) {
	t.Helper()
	initial, err := store.Role(ctx, roleID)
	must(t, err)
	index := slices.IndexFunc(initial.Items, func(item domain.Item) bool { return item.ItemID == 218 })
	if index < 0 || len(initial.Equipped) != 0 || initial.EquipmentEnhancement != nil {
		t.Fatal("隔离角色须持有原始未穿的木剑及空白强化档")
	}
	weapon := persistence.InitializeItemDurability(initial.Items[index], fixture)
	weapon.Bound = false // 确认绑定石不会改变当前非绑装备。
	if weapon.DurabilityU <= 0 || !weapon.HasValidDurability() {
		t.Fatal("原木剑需要有效耐久")
	}
	items := slices.Delete(initial.Items, index, index+1)
	var bound, free *domain.Item
	if rules := fixture.EquipmentEnhancement; rules != nil {
		// 隔离样本与首领实物都保留材料来源，报价不能把来源缺失的库存当成合法投入。
		bound = &domain.Item{ID: domain.ID(), ItemID: rules.StoneItemID, Quantity: 18, Version: 1, Bound: true,
			SourceKind: "creator_enhancement_fixture", SourceID: domain.ID()}
		free = &domain.Item{ID: domain.ID(), ItemID: rules.StoneItemID, Quantity: 19, Version: 1,
			SourceKind: "creator_enhancement_fixture", SourceID: domain.ID()}
		items = append(items, *bound, *free)
	}
	// 四钱包均有非零余额，能识别强化错误扣费；原木剑和石堆保持独立实例。
	wallet := domain.Wallet{Gold: 37, BoundGold: 59, Yuanbao: 17, BoundYuanbao: 23}
	equipped := []domain.Equipped{{Slot: devconfig.WeaponSlot, Item: weapon}}
	change, err := store.DB.Collection("roles").UpdateByID(ctx, roleID,
		bson.M{"$set": bson.M{"items": items, "equipped": equipped, "level": uint32(2), "wallet": wallet}})
	must(t, err)
	if change.ModifiedCount != 1 {
		t.Fatal("未能建立隔离部位强化前态")
	}
	before, err := store.Role(ctx, roleID)
	must(t, err)
	if before.Level != 2 || before.Wallet != wallet || before.EquipmentEnhancement != nil || len(before.Items) > before.Capacity ||
		!slices.Equal(before.Equipped, equipped) || len(before.Equipped) != 1 {
		t.Fatal("隔离前态未保留原空槽、钱包或木剑")
	}
	return before, weapon, bound, free
}

// verifyEnhancementTouchLedger 从真实回执和原数据库核对独立槽等级、原绑定石余量及四钱包守恒。
func verifyEnhancementTouchLedger(t *testing.T, ctx context.Context, store *persistence.Store, evidence string, before domain.Role,
	bound *domain.Item, fixture *devconfig.Fixture, bundleHash string,
) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(evidence, "equipment-enhancement-browser-report.json"))
	must(t, err)
	var report enhancementTouchReport
	must(t, json.Unmarshal(raw, &report))
	enabled := fixture.EquipmentEnhancement != nil
	if !report.Passed || report.Enabled != enabled || !report.OldEquipmentVisible || !report.WalletEqual || !report.WeaponEqual ||
		report.FixtureSha256 != fixture.Hash || report.ClientBundleSha256 != bundleHash {
		t.Fatal("实际包、触屏旧装备观察与隔离角色资产不符", report)
	}
	after, err := store.Role(ctx, before.ID)
	must(t, err)
	operations := store.DB.Collection("operations")
	audits := store.DB.Collection("audit_outbox")
	states := store.DB.Collection("role_equipment_enhancements")
	selector := bson.M{"role_id": before.ID, "kind": persistence.AssetEquipmentEnhancement}
	opCount, err := operations.CountDocuments(ctx, selector)
	must(t, err)
	auditCount, err := audits.CountDocuments(ctx, selector)
	must(t, err)
	stateCount, err := states.CountDocuments(ctx, bson.M{"_id": before.ID})
	must(t, err)
	if after.Level != before.Level || after.Capacity != before.Capacity || after.SourceSeq != before.SourceSeq || after.Wallet != before.Wallet ||
		!slices.Equal(after.Equipped, before.Equipped) || after.EconomicSeq != before.EconomicSeq+opCount {
		t.Fatal("强化触屏改变了原装备、角色资产或普通收益序列")
	}
	if !enabled {
		if !report.DefaultEnhancementHidden || report.Operation != nil || opCount != 0 || auditCount != 0 || stateCount != 0 ||
			after.EquipmentEnhancement != nil || !slices.Equal(after.Items, before.Items) {
			t.Fatal("默认包显示强化入口或留下了永久影响")
		}
		t.Log("默认实包原装备详情可见且无强化入口，原库存、装备和四钱包一致")
		return
	}
	actual := report.Operation
	if !report.EmptySlotVisible || !report.UnboundQuoteVerified || !report.BoundCancelled || !report.BoundGranted ||
		!report.PermissionPreserved || !report.ReopenedOff || opCount != 1 || auditCount != 1 || stateCount != 1 || actual == nil ||
		len(domain.Bytes(actual.Op)) != 16 || domain.Hex(domain.Bytes(actual.Op)) != actual.Op || len(domain.Bytes(actual.Quote)) != 16 ||
		actual.Slot != devconfig.WeaponSlot || actual.BeforeLevel != 0 || actual.AfterLevel != 1 ||
		actual.StoneID != bound.ID || !actual.StoneBound || actual.Quantity != strconv.Itoa(enhancementTouchStoneCost) {
		t.Fatal("原空槽、非绑报价、两次绑定授权、刷新或唯一触屏回执不同", report)
	}
	key := bson.M{"_id": "asset:" + before.ID + ":" + actual.Op, "role_id": before.ID}
	var saved persistence.Operation
	must(t, operations.FindOne(ctx, key).Decode(&saved))
	receipt := saved.EquipmentEnhancement
	if saved.Kind != persistence.AssetEquipmentEnhancement || saved.Code != 0 || saved.CommandSeq != before.EconomicSeq+1 ||
		actual.RoleRevision != strconv.FormatInt(saved.Revision-1, 10) || len(report.Receipts) != 1 ||
		report.Receipts[0].Op != actual.Op || report.Receipts[0].Quote != actual.Quote ||
		report.Receipts[0].Revision != strconv.FormatInt(saved.Revision, 10) || receipt == nil || !receipt.Success || receipt.AfterLevel != 1 ||
		receipt.Rule != fixture.RuleVersion || receipt.Config != fixture.Hash || receipt.QuoteID != actual.Quote ||
		receipt.Plan.Slot != devconfig.WeaponSlot || receipt.Plan.CurrentLevel != 0 || receipt.Plan.CurrentBonusBP != 0 ||
		receipt.Plan.TargetBonusBP != 10 || receipt.Plan.SuccessPPM != 1_000_000 || !receipt.Plan.AllowBound ||
		len(receipt.Plan.Stones) != 1 || receipt.Plan.Stones[0].Item != *bound || receipt.Plan.Stones[0].Quantity != enhancementTouchStoneCost {
		t.Fatal("唯一永久回执未记录原报价、精确基础加成、绑定授权或实际原石堆", receipt)
	}
	var audit struct {
		Receipt *persistence.EquipmentEnhancementReceipt `bson:"equipment_enhancement"`
	}
	must(t, audits.FindOne(ctx, key).Decode(&audit))
	var independent domain.EquipmentEnhancement
	must(t, states.FindOne(ctx, bson.M{"_id": before.ID}).Decode(&independent))
	expected := domain.EquipmentEnhancement{RoleID: before.ID, Version: saved.Revision}
	expected.Levels[devconfig.WeaponSlot] = 1
	wantItems := slices.Clone(before.Items)
	index := slices.Index(wantItems, *bound)
	if index < 0 {
		t.Fatal("原库存缺少触屏选中的绑定强化石")
	}
	wantItems[index].Quantity -= enhancementTouchStoneCost
	wantItems[index].Version++
	if !reflect.DeepEqual(audit.Receipt, receipt) || !slices.Equal(after.Items, wantItems) ||
		independent != expected || after.EquipmentEnhancement == nil || *after.EquipmentEnhancement != expected {
		t.Fatal("原库存、独立槽强化档或同事务审计与唯一回执不符", independent)
	}
	t.Log("实际触屏只扣十枚原绑定石，独立武器部位升一级且与唯一回执和审计一致")
}
