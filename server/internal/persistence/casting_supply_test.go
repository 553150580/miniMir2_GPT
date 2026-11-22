// 验证施法耗材的准入、选择来源、扣除结果和业务拒绝，确保错误输入不污染原库存。

package persistence

import (
	"math"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	p "minimir/internal/protocol"
	"reflect"
	"testing"
)

// castingSupplyFixture 处理施法耗材开发测试配置。
func castingSupplyFixture() (domain.Role, AssetRequest, devconfig.Fixture) {
	// item 组装道具所需的数据。
	item := domain.Item{ID: domain.ID(), ItemID: 187, Quantity: 3, Version: 4, Bound: true, SourceKind: "npc_shop", SourceID: "supply-test", Quality: 1}
	// role 组装角色所需的数据。
	role := domain.Role{Job: 2, Capacity: 2, Skills: []domain.LearnedSkill{{SkillID: 14}}, Items: []domain.Item{item}}
	// request 初始化用作请求的资产请求。
	request := AssetRequest{ItemID: item.ID, ItemVersion: 4, SkillID: 14, SceneEpoch: 20}
	// fixture 组装开发测试配置所需的数据。
	fixture := devconfig.Fixture{Items: []devconfig.Item{{ID: 187, EquipmentSlot: 100, Stackable: true, MaxStack: 10}}, Skills: []devconfig.Skill{{ID: 14, JobID: 2, MaxLevel: 3, MaterialItemID: 187, MaterialQuantity: 1}}, CastingSupply: devconfig.CastingSupplyRules{Enabled: true, Slot: 100, ItemIDs: []uint32{187}}}
	return role, request, fixture
}

// TestCastingSupplySlotMoveSwapAndConsumptionCopyOwnership 核对「护符未精确减Count1或改写原来源/绑定/切片」与「换装洗来源/少物品/污染原状态」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestCastingSupplySlotMoveSwapAndConsumptionCopyOwnership(t *testing.T) {
	// r 接收施法耗材开发测试配置供后续步骤使用，q 接收施法耗材开发测试配置供后续步骤使用，f 接收施法耗材开发测试配置供后续步骤使用。
	r, q, f := castingSupplyFixture()
	// original 保留加入新条目后的“original”。
	original := append([]domain.Item(nil), r.Items...)
	// equipped 保存由设置施法耗材取得的已穿戴装备，code 保存由设置施法耗材取得的结果码。
	equipped, code := setCastingSupply(r, q, f)
	if code != 0 || len(equipped.Items) != 0 || len(equipped.Equipped) != 1 || equipped.Equipped[0].Slot != 100 || equipped.Equipped[0].Item.Version != 5 || !reflect.DeepEqual(r.Items, original) {
		t.Fatal("装入专槽未版本化/污染原包")
	}
	q.ItemVersion = 5
	// consumed 保存由消费施法材料取得的“consumed”，receipt 保存由消费施法材料取得的回执。
	consumed, receipt, code := consumeCastingMaterial(equipped, q, f)
	if code != 0 || receipt.Quantity != 1 || receipt.Scope != "manual" || consumed.Equipped[0].Item.Quantity != 2 || consumed.Equipped[0].Item.Version != 6 || equipped.Equipped[0].Item.Quantity != 3 || !consumed.Equipped[0].Item.Bound || consumed.Equipped[0].Item.SourceID != "supply-test" {
		t.Fatal("护符未精确减Count1或改写原来源/绑定/切片")
	}
	q.ItemVersion = 6
	q.Unequip = true
	// bag 保存由设置施法耗材取得的背包。
	bag, code := setCastingSupply(consumed, q, f)
	if code != 0 || len(bag.Equipped) != 0 || len(bag.Items) != 1 || bag.Items[0].Version != 7 || bag.Items[0].Quantity != 2 {
		t.Fatal("卸下未保留原单位/版本")
	}
	// other 建立带初始值的道具，供本段继续处理。
	other := domain.Item{ID: domain.ID(), ItemID: 187, Quantity: 1, Version: 1, SourceKind: "monster_drop", SourceID: "9"}
	equipped.Items = []domain.Item{other}
	q.ItemID = other.ID
	q.ItemVersion = 1
	q.Unequip = false
	// swapped 保存由设置施法耗材取得的“swapped”。
	swapped, code := setCastingSupply(equipped, q, f)
	if code != 0 || swapped.Equipped[0].Item.ID != other.ID || len(swapped.Items) != 1 || swapped.Items[0].ID != original[0].ID || swapped.Items[0].Version != 6 || equipped.Equipped[0].Item.Version != 5 {
		t.Fatal("换装洗来源/少物品/污染原状态")
	}
	q.ItemID = other.ID
	q.ItemVersion = 2
	q.SourceSeq = 5
	// empty 保存由消费施法材料取得的空。
	empty, receipt, code := consumeCastingMaterial(swapped, q, f)
	if code != 0 || len(empty.Equipped) != 0 || receipt.Scope != "auto" || swapped.Equipped[0].Item.Quantity != 1 {
		t.Fatal("最后单位未移除或自动作用域错误")
	}
}

// TestCastingSupplyRejectedPathsNeverMutateOriginal 核对「拒绝码/或修改原物品」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestCastingSupplyRejectedPathsNeverMutateOriginal(t *testing.T) {
	// changes 建立带初始值的临时结构列表，供本段继续处理。
	changes := []struct {
		// name 记录名称，使用文本表示。
		name string
		// change 记录变更，使用供当前流程调用的函数表示。
		change func(*domain.Role, *AssetRequest, *devconfig.Fixture)
		// code 记录结果码，使用数值表示。
		code uint32
	}{
		// f 记录开发测试配置。
		{"未开放", func(_ *domain.Role, _ *AssetRequest, f *devconfig.Fixture) { f.CastingSupply.Enabled = false }, p.CodeSkillMaterialRequired},
		// r 记录角色。
		{"未学", func(r *domain.Role, _ *AssetRequest, _ *devconfig.Fixture) { r.Skills = nil }, p.CodeSkillNotLearned},
		// r 记录角色。
		{"错职业", func(r *domain.Role, _ *AssetRequest, _ *devconfig.Fixture) { r.Job = 1 }, p.CodeSkillNotLearned},
		// q 记录资产请求。
		{"错实例", func(_ *domain.Role, q *AssetRequest, _ *devconfig.Fixture) { q.ItemID = domain.ID() }, p.CodeSkillMaterialChanged},
		// q 记录资产请求。
		{"旧版本", func(_ *domain.Role, q *AssetRequest, _ *devconfig.Fixture) { q.ItemVersion-- }, p.CodeSkillMaterialChanged},
		// r 记录角色，q 记录资产请求。
		{"溢出版本", func(r *domain.Role, q *AssetRequest, _ *devconfig.Fixture) {
			r.Equipped[0].Item.Version = math.MaxInt64
			q.ItemVersion = math.MaxInt64
		}, p.CodeSkillMaterialChanged},
		// r 记录角色。
		{"数量不足", func(r *domain.Role, _ *AssetRequest, _ *devconfig.Fixture) { r.Equipped[0].Item.Quantity = 0 }, p.CodeSkillMaterialRequired},
		// r 记录角色。
		{"不等价大符", func(r *domain.Role, _ *AssetRequest, _ *devconfig.Fixture) { r.Equipped[0].Item.ItemID = 188 }, p.CodeSkillMaterialRequired},
		// r 记录角色。
		{"背包不能直用", func(r *domain.Role, _ *AssetRequest, _ *devconfig.Fixture) {
			r.Items = append(r.Items, r.Equipped[0].Item)
			r.Equipped = nil
		}, p.CodeSkillMaterialRequired},
		// q 记录资产请求。
		{"旧场景门禁", func(_ *domain.Role, q *AssetRequest, _ *devconfig.Fixture) { q.CastGateCode = p.CodeStaleSession }, p.CodeStaleSession},
	}
	// test 接收遍历“changes”时当前的元素。
	for _, test := range changes {
		// t 提供断言失败报告、子用例和清理注册。
		t.Run(test.name, func(t *testing.T) {
			// r 接收施法耗材开发测试配置供后续步骤使用，q 接收施法耗材开发测试配置供后续步骤使用，f 接收施法耗材开发测试配置供后续步骤使用。
			r, q, f := castingSupplyFixture()
			r.Equipped = []domain.Equipped{{Slot: 100, Item: r.Items[0]}}
			r.Items = nil
			test.change(&r, &q, &f)
			// before 保留加入新条目后的操作前的状态。
			before := append([]domain.Equipped(nil), r.Equipped...)
			// out 保存由消费施法材料取得的输出，receipt 保存由消费施法材料取得的回执，code 保存由消费施法材料取得的结果码。
			out, receipt, code := consumeCastingMaterial(r, q, f)
			if code != test.code || receipt != nil || !reflect.DeepEqual(before, r.Equipped) || !reflect.DeepEqual(out.Equipped, r.Equipped) {
				t.Fatalf("拒绝码%d/%d或修改原物品", code, test.code)
			}
		})
	}
}

// TestCastingSupplyUnequipFullBagAndDisabledUnknownUnits 核对「单位未定停用却可装卸」与「满包卸符丢失物品」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestCastingSupplyUnequipFullBagAndDisabledUnknownUnits(t *testing.T) {
	// r 接收施法耗材开发测试配置供后续步骤使用，q 接收施法耗材开发测试配置供后续步骤使用，f 接收施法耗材开发测试配置供后续步骤使用。
	r, q, f := castingSupplyFixture()
	r.Equipped = []domain.Equipped{{Slot: 100, Item: r.Items[0]}}
	r.Items = []domain.Item{{ID: domain.ID()}, {ID: domain.ID()}}
	q.Unequip = true
	// code 保存由设置施法耗材取得的结果码。
	if _, code := setCastingSupply(r, q, f); code != p.CodeOperationConflict || len(r.Equipped) != 1 {
		t.Fatal("满包卸符丢失物品")
	}
	f.CastingSupply.Enabled = false
	f.Items[0].MaxStack = 0
	// code 保存由设置施法耗材取得的结果码。
	if _, code := setCastingSupply(r, q, f); code != p.CodeSkillMaterialRequired {
		t.Fatal("单位未定停用却可装卸")
	}
}

// TestStatusUpgradeQuoteShowsRealRankParametersWithoutFakeDamage 核对「防御状态报价伪造了伤害或准确成长」与「状态报价缺少实际等级参数」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestStatusUpgradeQuoteShowsRealRankParametersWithoutFakeDamage(t *testing.T) {
	// config 保存由加载取得的配置，err 接收加载过程中可能产生的错误，供后续失败分支处理。
	config, err := devconfig.Load("../../../conf/dev/server.json")
	if err != nil {
		t.Fatal(err)
	}
	// id 接收遍历数值列表时当前的元素。
	for _, id := range []uint32{31, 14, 15} {
		// quote 初始化用作报价的技能报价。
		quote := skillQuote{Plan: skillPlan{Kind: 2, SkillID: id, From: 0, To: 1}}
		// stats 保存当前处理所用的统计或属性。
		stats := quote.Proto(config.Fixture).Stats
		// seen 建立带初始值的按文本索引的布尔状态映射，供本段继续处理。
		seen := map[string]bool{}
		// entry 接收遍历统计或属性时当前的元素。
		for _, entry := range stats {
			seen[entry.Key] = true
			switch entry.Key {
			case "base_power", "accuracy_bonus", "max_attack_bonus":
				t.Fatal("防御状态报价伪造了伤害或准确成长")
			}
		}
		if !seen["mp_cost"] || id == 31 && (!seen["damage_reduction_percent"] || !seen["duration_rank_factor"]) || id != 31 && (!seen["base_duration_seconds"] || !seen["material_quantity"]) {
			t.Fatal("状态报价缺少实际等级参数")
		}
	}
}
