// 验证技能资产计划、材料选择和拒绝路径，核对升级、分解等资产计算与回执快照。

package persistence

import (
	"math"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	p "minimir/internal/protocol"
	"reflect"
	"testing"
)

// skillEconomyFixture 处理技能经济开发测试配置。
// t 提供断言失败报告、子用例和清理注册。
func skillEconomyFixture(t *testing.T) (domain.Role, devconfig.Fixture) {
	t.Helper()
	// cfg 保存由加载取得的服务配置，err 接收加载过程中可能产生的错误，供后续失败分支处理。
	cfg, err := devconfig.Load("../../../conf/dev/server.json")
	if err != nil {
		t.Fatal(err)
	}
	// f 保存当前处理所用的开发测试配置。
	f := cfg.Fixture
	if len(f.SkillUpgrades) == 0 {
		t.Fatal("本组测试需要唯一Excel导出的技能经济候选")
	}
	// s 接收技能供后续步骤使用。
	s, _ := f.Skill(1)
	// r 组装角色所需的数据。
	r := domain.Role{ID: domain.ID(), Job: s.JobID, Level: 100, Revision: 1, Capacity: f.InventoryCapacity, Skills: []domain.LearnedSkill{{SkillID: s.ID, Level: s.LearnedLevel}}, Wallet: domain.Wallet{Gold: math.MaxInt32, BoundGold: math.MaxInt32, Yuanbao: math.MaxInt32, BoundYuanbao: math.MaxInt32}}
	// n 规定技能经济开发测试配置的当前数值的初始取值。
	for n := 0; n < 10; n++ {
		r.Items = append(r.Items, domain.Item{ID: domain.ID(), ItemID: s.BookItemID, Quantity: 1, Version: 1, Bound: n%2 == 0, SourceKind: "npc_shop", SourceID: domain.ID()})
	}
	// pages 保存由整数“64”取得的“pages”。
	pages := int64(0)
	// row 接收遍历技能“upgrades”时当前的元素。
	for _, row := range f.SkillUpgrades {
		if row.SkillID == s.ID {
			pages += row.PageCost
		}
	}
	r.Items = append(r.Items, domain.Item{ID: domain.ID(), ItemID: f.SkillEconomy.PageItemID, Quantity: pages, Version: 1, Bound: true, SourceKind: "npc_shop", SourceID: domain.ID()})
	return r, f
}

// TestSkillPreviewShowsMaximumAttackGrowthOnlyForSlaying 核对「攻杀预览未反映真实永久最大DC成长」与「技能混入仅攻杀使用的攻击上限预览」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestSkillPreviewShowsMaximumAttackGrowthOnlyForSlaying(t *testing.T) {
	// f 接收技能经济开发测试配置供后续步骤使用。
	_, f := skillEconomyFixture(t)
	// skill 接收遍历技能集合时当前的元素。
	for _, skill := range f.Skills {
		// quote 初始化用作报价的技能报价。
		quote := skillQuote{Plan: skillPlan{Kind: 2, SkillID: skill.ID, From: 0, To: 1}}
		// preview 保存由协议取得的预览。
		preview := quote.Proto(f)
		// want 规定期望值的初始取值。
		want := 3
		if skill.Behavior == "slaying" || skill.Behavior == "repulsion" {
			want = 4
		}
		if len(preview.Stats) != want {
			t.Fatalf("技能%d无意义中性数组污染预览:%d/%d", skill.ID, len(preview.Stats), want)
		}
		// stat 接收遍历统计或属性时当前的元素。
		for _, stat := range preview.Stats {
			if stat.Key == "max_attack_bonus" && skill.Behavior != "slaying" {
				t.Fatalf("技能%d混入仅攻杀使用的攻击上限预览", skill.ID)
			}
		}
		if skill.Behavior == "slaying" {
			// bonus 保存从统计或属性取出的加成。
			bonus := preview.Stats[3]
			if bonus.Key != "max_attack_bonus" || bonus.BeforeMin != 5 || bonus.AfterMin != 6 || bonus.BeforeMax != bonus.BeforeMin || bonus.AfterMax != bonus.AfterMin {
				t.Fatal("攻杀预览未反映真实永久最大DC成长")
			}
		}
	}
}

// TestSkillPreviewTeleportFixedDurationAndManaPenaltyFromCurrentFixture 核对「Flux须为固定毫秒时长且无道士逐阶秒数数组」与「21预览混入伤害/道士/材料或缺少固定时长」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestSkillPreviewTeleportFixedDurationAndManaPenaltyFromCurrentFixture(t *testing.T) {
	// f 接收技能经济开发测试配置供后续步骤使用。
	_, f := skillEconomyFixture(t)
	// skill 接收技能供后续步骤使用，found 保存由技能取得的已找到标记。
	skill, found := f.Skill(21)
	if !found || skill.Behavior != "teleport" {
		t.Fatal("本组测试须使用已选21的当前唯一配置")
	}
	// index 保存当前处理所用的下标。
	index := -1
	// i 标记遍历“buffs”时访问的下标。
	for i := range f.Buffs {
		if f.Buffs[i].ID == skill.BuffID {
			index = i
		}
	}
	if index < 0 || f.Buffs[index].DurationKind != "fixed_ms" || len(f.Buffs[index].DurationSecondsByLevel) != 0 {
		t.Fatal("Flux须为固定毫秒时长且无道士逐阶秒数数组")
	}
	// 测试值区别当前候选，证明预览读取原Buff字段而不填造30秒/30%。
	f.Buffs[index].DurationMS = 41007
	f.Buffs[index].ValueByLevel = []int64{11, 23, 37, 53}
	// from 保存由无符号整数“32”取得的来源。
	for from := uint32(0); from < skill.MaxLevel; from++ {
		// preview 保存由协议取得的预览。
		preview := (skillQuote{Plan: skillPlan{Kind: 2, SkillID: 21, From: from, To: from + 1}}).Proto(f)
		// keys 初始化用作键集合的文本列表。
		keys := []string{}
		// stat 接收遍历统计或属性时当前的元素。
		for _, stat := range preview.Stats {
			keys = append(keys, stat.Key)
		}
		if !reflect.DeepEqual(keys, []string{"mp_cost", "duration_ms", "teleport_mana_penalty_percent"}) {
			t.Fatalf("21预览混入伤害/道士/材料或缺少固定时长:%v", keys)
		}
		// wantBefore 初始化用作期望值操作前的状态的数值列表。
		wantBefore := []int64{skill.MPBaseCost + int64(from)*skill.MPLevelCost, 41007, f.Buffs[index].ValueByLevel[from]}
		// wantAfter 初始化用作期望值操作后的状态的数值列表。
		wantAfter := []int64{skill.MPBaseCost + int64(from+1)*skill.MPLevelCost, 41007, f.Buffs[index].ValueByLevel[from+1]}
		// i 定位统计或属性中正在处理的成员，stat 接收遍历统计或属性时当前的元素。
		for i, stat := range preview.Stats {
			if stat.BeforeMin != wantBefore[i] || stat.BeforeMax != wantBefore[i] || stat.AfterMin != wantAfter[i] || stat.AfterMax != wantAfter[i] {
				t.Fatalf("21阶%d字段%s未反映原永久数值:%+v", from, stat.Key, stat)
			}
		}
	}
}

// explicitUpgrade 返回显式升级供调用方使用。
// r 记录角色，f 记录开发测试配置，id 区分当前步骤指向的目标对象。
func explicitUpgrade(r domain.Role, f devconfig.Fixture, id uint32) *pb.SkillAssetPreviewRequest {
	// level 保存由技能取得的等级。
	level, _ := r.Skill(id)
	// row 保存由升级取得的行记录。
	row, _ := f.Upgrade(id, level+1)
	// need 初始化用作所需量的按数值索引的数值映射。
	need := map[uint32]int64{}
	// c 接收遍历技能书“costs”时当前的元素。
	for _, c := range row.BookCosts {
		need[c.ItemID] += c.Quantity
	}
	need[row.PageItemID] += row.PageCost
	// q 保存当前处理所用的技能资产预览请求。
	q := &pb.SkillAssetPreviewRequest{Kind: 2, SkillId: id}
	// item 接收遍历道具集合时当前的元素。
	for _, item := range r.Items {
		// n 保存从所需量取出的所需量中的指定项。
		n := need[item.ItemID]
		if n <= 0 {
			continue
		}
		if n > item.Quantity {
			n = item.Quantity
		}
		q.Materials = append(q.Materials, &pb.ItemConsumption{InstanceId: domain.Bytes(item.ID), Version: uint64(item.Version), Quantity: n})
		need[item.ItemID] -= n
	}
	return q
}

// TestSkillUpgradePreviewPinsBoundedConcreteMaterialsWithoutMutation 核对「没有使用唯一逐级表的书本/书页数量」与「升级预览改变资产或不支持零级已学」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestSkillUpgradePreviewPinsBoundedConcreteMaterialsWithoutMutation(t *testing.T) {
	// r 接收技能经济开发测试配置供后续步骤使用，f 接收技能经济开发测试配置供后续步骤使用。
	r, f := skillEconomyFixture(t)
	// code 保存由构建技能计划取得的结果码。
	if _, code := buildSkillPlan(r, &pb.SkillAssetPreviewRequest{Kind: 2, SkillId: 1}, f); code != p.CodeSkillMaterials {
		t.Fatal("没有明确材料实例也自动选材")
	}
	// before 保留加入新条目后的操作前的状态。
	before := append([]domain.Item(nil), r.Items...)
	// plan 保存由构建技能计划取得的计划，code 保存由构建技能计划取得的结果码。
	plan, code := buildSkillPlan(r, explicitUpgrade(r, f, 1), f)
	// row 保存由升级取得的行记录。
	row, _ := f.Upgrade(1, 1)
	if code != 0 || plan.From != 0 || plan.To != 1 || !reflect.DeepEqual(before, r.Items) {
		t.Fatal("升级预览改变资产或不支持零级已学")
	}
	// quantities 建立带初始值的按数值索引的数值映射，供本段继续处理。
	quantities := map[uint32]int64{}
	// m 接收遍历材料集合时当前的元素。
	for _, m := range plan.Materials {
		quantities[m.Item.ItemID] += m.Quantity
		if m.Item.Version != 1 || m.Quantity <= 0 {
			t.Fatal("预览未锁具体版本/数量")
		}
	}
	if quantities[row.PageItemID] != row.PageCost || quantities[row.BookCosts[0].ItemID] != row.BookCosts[0].Quantity {
		t.Fatal("没有使用唯一逐级表的书本/书页数量")
	}
	// q 保存当前处理所用的技能资产预览请求。
	q := &pb.SkillAssetPreviewRequest{Kind: 2, SkillId: 1}
	// m 接收遍历材料集合时当前的元素。
	for _, m := range plan.Materials {
		q.Materials = append(q.Materials, &pb.ItemConsumption{InstanceId: domain.Bytes(m.Item.ID), Version: uint64(m.Item.Version), Quantity: m.Quantity})
	}
	q.Materials[0].Version++
	// code 保存由构建技能计划取得的结果码。
	if _, code := buildSkillPlan(r, q, f); code != p.CodeSkillMaterials {
		t.Fatal("旧材料版本被预览接受")
	}
	q.Materials[0].Version--
	q.Materials = append(q.Materials, q.Materials[0])
	// code 保存由构建技能计划取得的结果码。
	if _, code := buildSkillPlan(r, q, f); code != p.CodeSkillMaterials {
		t.Fatal("重复实例绕过数量验证")
	}
	r.Skills = nil
	// code 保存由构建技能计划取得的结果码。
	if _, code := buildSkillPlan(r, &pb.SkillAssetPreviewRequest{Kind: 2, SkillId: 1}, f); code != p.CodeSkillUpgradeBlocked {
		t.Fatal("未学习被当成rank0")
	}
}

// TestSkillUpgradeLevelPrerequisiteCapAndExactCost 核对「书本数不符合用户确定的目标阶+1」与「逐阶预览拒绝」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestSkillUpgradeLevelPrerequisiteCapAndExactCost(t *testing.T) {
	// r 接收技能经济开发测试配置供后续步骤使用，f 接收技能经济开发测试配置供后续步骤使用。
	r, f := skillEconomyFixture(t)
	// s 接收技能供后续步骤使用。
	s, _ := f.Skill(1)
	// rank 保存已学习等级，用于当前步骤中的排序位次。
	for rank := s.LearnedLevel; rank < s.MaxLevel; rank++ {
		r.Skills[0].Level = rank
		// row 保存由升级取得的行记录。
		row, _ := f.Upgrade(1, rank+1)
		r.Level = row.RequiredRoleLevel - 1
		// code 保存由构建技能计划取得的结果码。
		if _, code := buildSkillPlan(r, &pb.SkillAssetPreviewRequest{Kind: 2, SkillId: 1}, f); code != p.CodeSkillUpgradeBlocked {
			t.Fatal("等级前置未校验")
		}
		r.Level = row.RequiredRoleLevel
		// plan 保存由构建技能计划取得的计划，code 保存由构建技能计划取得的结果码。
		plan, code := buildSkillPlan(r, explicitUpgrade(r, f, 1), f)
		if code != 0 || plan.To != rank+1 {
			t.Fatalf("逐阶预览拒绝:%d", code)
		}
		// books 保存由整数“64”取得的技能书集合。
		books := int64(0)
		// m 接收遍历材料集合时当前的元素。
		for _, m := range plan.Materials {
			if m.Item.ItemID == s.BookItemID {
				books += m.Quantity
			}
		}
		if books != int64(plan.To)+1 {
			t.Fatal("书本数不符合用户确定的目标阶+1")
		}
	}
	r.Skills[0].Level = s.MaxLevel
	// code 保存由构建技能计划取得的结果码。
	if _, code := buildSkillPlan(r, &pb.SkillAssetPreviewRequest{Kind: 2, SkillId: 1}, f); code != p.CodeSkillUpgradeBlocked {
		t.Fatal("满级仍可升级")
	}
}

// TestSkillShopExplicitCurrenciesSplitGoldAndOverflowAndCapacity 核验金币应按绑定优先准确拆分，用诊断「商店预览币种、来源或绑定不符」定位对应断言。
// t 提供断言失败报告、子用例和清理注册。
func TestSkillShopExplicitCurrenciesSplitGoldAndOverflowAndCapacity(t *testing.T) {
	// r 接收技能经济开发测试配置供后续步骤使用，f 接收技能经济开发测试配置供后续步骤使用。
	r, f := skillEconomyFixture(t)
	// offer 接收遍历技能商店时当前的元素。
	for _, offer := range f.SkillShop {
		// copy 保存当前处理所用的“copy”。
		copy := r
		copy.Items = nil
		copy.Wallet = domain.Wallet{}
		switch offer.Currency {
		case "gold":
			copy.Wallet.BoundGold = offer.PriceUnits - 1
			copy.Wallet.Gold = 1
		case "ingot":
			copy.Wallet.Yuanbao = offer.PriceUnits
			copy.Wallet.BoundYuanbao = math.MaxInt32
		case "bound_ingot":
			copy.Wallet.BoundYuanbao = offer.PriceUnits
			copy.Wallet.Yuanbao = math.MaxInt32
		}
		// plan 保存由构建技能计划取得的计划，code 保存由构建技能计划取得的结果码。
		plan, code := buildSkillPlan(copy, &pb.SkillAssetPreviewRequest{Kind: 1, OfferId: offer.ID, Quantity: 1}, f)
		if code != 0 || len(plan.Outputs) == 0 || !plan.Outputs[0].Bound || plan.Outputs[0].SourceKind != "npc_shop" || plan.Outputs[0].Quality != 0 {
			t.Fatal("商店预览币种、来源或绑定不符")
		}
		if offer.Currency == "gold" && (plan.Spend.BoundGold != offer.PriceUnits-1 || plan.Spend.Gold != 1) {
			t.Fatal("金币没有按绑定优先准确拆分")
		}
		if offer.Currency == "ingot" {
			copy.Wallet.Yuanbao = 0
			// code 保存由构建技能计划取得的结果码。
			if _, code := buildSkillPlan(copy, &pb.SkillAssetPreviewRequest{Kind: 1, OfferId: offer.ID, Quantity: 1}, f); code != p.CodeSkillPurchaseBlocked {
				t.Fatal("元宝商品自动改扣绑元")
			}
		}
		if offer.Currency == "bound_ingot" {
			copy.Wallet.BoundYuanbao = 0
			// code 保存由构建技能计划取得的结果码。
			if _, code := buildSkillPlan(copy, &pb.SkillAssetPreviewRequest{Kind: 1, OfferId: offer.ID, Quantity: 1}, f); code != p.CodeSkillPurchaseBlocked {
				t.Fatal("绑元商品自动改扣元宝")
			}
		}
	}
	// offer 保存从技能商店取出的尝试提交。
	offer := f.SkillShop[0]
	f.SkillShop[0].MaxPurchaseQuantity = math.MaxInt64
	// code 保存由构建技能计划取得的结果码。
	if _, code := buildSkillPlan(r, &pb.SkillAssetPreviewRequest{Kind: 1, OfferId: offer.ID, Quantity: math.MaxInt64}, f); code != p.CodeSkillPurchaseBlocked {
		t.Fatal("数量乘法溢出未拒绝")
	}
	r.Capacity = len(r.Items)
	// code 保存由构建技能计划取得的结果码。
	if _, code := buildSkillPlan(r, &pb.SkillAssetPreviewRequest{Kind: 1, OfferId: offer.ID, Quantity: 1}, f); code != p.CodeSkillMaterials {
		t.Fatal("满包购买未拒绝")
	}
}

// TestSkillBookRecyclingRequiresKnownDropQualityAndPreservesEachBinding 核验分解应保留各原书绑定，或没有读取唯一品质分解表，用诊断「伪造未配置品质可拆页」定位对应断言。
// t 提供断言失败报告、子用例和清理注册。
func TestSkillBookRecyclingRequiresKnownDropQualityAndPreservesEachBinding(t *testing.T) {
	// r 接收技能经济开发测试配置供后续步骤使用，f 接收技能经济开发测试配置供后续步骤使用。
	r, f := skillEconomyFixture(t)
	// rule 保存从技能技能书回收取出的规则。
	rule := f.SkillBookRecycle[0]
	r.Items = []domain.Item{{ID: domain.ID(), ItemID: rule.ItemID, Quantity: 1, Version: 1, SourceKind: "monster_drop", SourceID: "1", Quality: rule.BookQuality, Bound: true}, {ID: domain.ID(), ItemID: rule.ItemID, Quantity: 1, Version: 1, SourceKind: "monster_drop", SourceID: "2", Quality: rule.BookQuality}}
	// q 保存当前处理所用的技能资产预览请求。
	q := &pb.SkillAssetPreviewRequest{Kind: 3}
	// item 接收遍历道具集合时当前的元素。
	for _, item := range r.Items {
		q.Materials = append(q.Materials, &pb.ItemConsumption{InstanceId: domain.Bytes(item.ID), Version: 1, Quantity: 1})
	}
	// plan 保存由构建技能计划取得的计划，code 保存由构建技能计划取得的结果码。
	plan, code := buildSkillPlan(r, q, f)
	if code != 0 || len(plan.Outputs) != 2 || plan.Outputs[0].Quantity != rule.PagesPerBook || plan.Outputs[1].Quantity != rule.PagesPerBook || plan.Outputs[0].Bound == plan.Outputs[1].Bound {
		t.Fatal("分解没有保留各原书绑定，或没有读取唯一品质分解表")
	}
	// kind 接收遍历文本列表时当前的元素。
	for _, kind := range []string{"", "npc_shop", "initial_grant"} {
		r.Items[0].SourceKind = kind
		// code 保存由构建技能计划取得的结果码。
		if _, code := buildSkillPlan(r, q, f); code != p.CodeSkillBookSource {
			t.Fatalf("来源%s可以拆页", kind)
		}
	}
	r.Items[0].SourceKind = "monster_drop"
	r.Items[0].Quality = 0
	// code 保存由构建技能计划取得的结果码。
	if _, code := buildSkillPlan(r, q, f); code != p.CodeSkillBookSource {
		t.Fatal("伪造未配置品质可拆页")
	}
}

// TestSkillPowerRanksUseSourceToEvenAndFencingGrowth 核对「技能阶威力..预期」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestSkillPowerRanksUseSourceToEvenAndFencingGrowth(t *testing.T) {
	// f 接收技能经济开发测试配置供后续步骤使用。
	_, f := skillEconomyFixture(t)
	// test 接收遍历临时结构列表时当前的元素。
	for _, test := range []struct {
		// id 区分当前步骤指向的目标对象。
		id uint32
		// want 记录期望值，使用数值列表表示。
		want []int64
	}{{1, []int64{4, 6, 8, 10}}, {2, []int64{4, 7, 10, 14}}} {
		// s 接收技能供后续步骤使用。
		s, _ := f.Skill(test.id)
		// rank 定位期望值中正在处理的成员，want 接收遍历期望值时当前的元素。
		for rank, want := range test.want {
			// min 保存由能力值“bounds”取得的最小，max 保存由能力值“bounds”取得的最大。
			min, max := powerBounds(s, uint32(rank))
			if min != want || max != want {
				t.Fatalf("技能%d阶%d威力%d..%d预期%d", test.id, rank, min, max, want)
			}
		}
	}
}
