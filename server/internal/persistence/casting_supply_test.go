package persistence

import (
	"math"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	p "minimir/internal/protocol"
	"reflect"
	"testing"
)

func castingSupplyFixture() (domain.Role, AssetRequest, devconfig.Fixture) {
	item := domain.Item{ID: domain.ID(), ItemID: 187, Quantity: 3, Version: 4, Bound: true, SourceKind: "npc_shop", SourceID: "supply-test", Quality: 1}
	role := domain.Role{Job: 2, Capacity: 2, Skills: []domain.LearnedSkill{{SkillID: 14}}, Items: []domain.Item{item}}
	request := AssetRequest{ItemID: item.ID, ItemVersion: 4, SkillID: 14, SceneEpoch: 20}
	fixture := devconfig.Fixture{Items: []devconfig.Item{{ID: 187, EquipmentSlot: 100, Stackable: true, MaxStack: 10}}, Skills: []devconfig.Skill{{ID: 14, JobID: 2, MaxLevel: 3, MaterialItemID: 187, MaterialQuantity: 1}}, CastingSupply: devconfig.CastingSupplyRules{Enabled: true, Slot: 100, ItemIDs: []uint32{187}}}
	return role, request, fixture
}

func TestCastingSupplySlotMoveSwapAndConsumptionCopyOwnership(t *testing.T) {
	r, q, f := castingSupplyFixture()
	original := append([]domain.Item(nil), r.Items...)
	equipped, code := setCastingSupply(r, q, f)
	if code != 0 || len(equipped.Items) != 0 || len(equipped.Equipped) != 1 || equipped.Equipped[0].Slot != 100 || equipped.Equipped[0].Item.Version != 5 || !reflect.DeepEqual(r.Items, original) {
		t.Fatal("装入专槽未版本化/污染原包")
	}
	q.ItemVersion = 5
	consumed, receipt, code := consumeCastingMaterial(equipped, q, f)
	if code != 0 || receipt.Quantity != 1 || receipt.Scope != "manual" || consumed.Equipped[0].Item.Quantity != 2 || consumed.Equipped[0].Item.Version != 6 || equipped.Equipped[0].Item.Quantity != 3 || !consumed.Equipped[0].Item.Bound || consumed.Equipped[0].Item.SourceID != "supply-test" {
		t.Fatal("护符未精确减Count1或改写原来源/绑定/切片")
	}
	q.ItemVersion = 6
	q.Unequip = true
	bag, code := setCastingSupply(consumed, q, f)
	if code != 0 || len(bag.Equipped) != 0 || len(bag.Items) != 1 || bag.Items[0].Version != 7 || bag.Items[0].Quantity != 2 {
		t.Fatal("卸下未保留原单位/版本")
	}
	other := domain.Item{ID: domain.ID(), ItemID: 187, Quantity: 1, Version: 1, SourceKind: "monster_drop", SourceID: "9"}
	equipped.Items = []domain.Item{other}
	q.ItemID = other.ID
	q.ItemVersion = 1
	q.Unequip = false
	swapped, code := setCastingSupply(equipped, q, f)
	if code != 0 || swapped.Equipped[0].Item.ID != other.ID || len(swapped.Items) != 1 || swapped.Items[0].ID != original[0].ID || swapped.Items[0].Version != 6 || equipped.Equipped[0].Item.Version != 5 {
		t.Fatal("换装洗来源/少物品/污染原状态")
	}
	q.ItemID = other.ID
	q.ItemVersion = 2
	q.SourceSeq = 5
	empty, receipt, code := consumeCastingMaterial(swapped, q, f)
	if code != 0 || len(empty.Equipped) != 0 || receipt.Scope != "auto" || swapped.Equipped[0].Item.Quantity != 1 {
		t.Fatal("最后单位未移除或自动作用域错误")
	}
}

func TestCastingSupplyRejectedPathsNeverMutateOriginal(t *testing.T) {
	changes := []struct {
		name   string
		change func(*domain.Role, *AssetRequest, *devconfig.Fixture)
		code   uint32
	}{
		{"未开放", func(_ *domain.Role, _ *AssetRequest, f *devconfig.Fixture) { f.CastingSupply.Enabled = false }, p.CodeSkillMaterialRequired},
		{"未学", func(r *domain.Role, _ *AssetRequest, _ *devconfig.Fixture) { r.Skills = nil }, p.CodeSkillNotLearned},
		{"错职业", func(r *domain.Role, _ *AssetRequest, _ *devconfig.Fixture) { r.Job = 1 }, p.CodeSkillNotLearned},
		{"错实例", func(_ *domain.Role, q *AssetRequest, _ *devconfig.Fixture) { q.ItemID = domain.ID() }, p.CodeSkillMaterialChanged},
		{"旧版本", func(_ *domain.Role, q *AssetRequest, _ *devconfig.Fixture) { q.ItemVersion-- }, p.CodeSkillMaterialChanged},
		{"溢出版本", func(r *domain.Role, q *AssetRequest, _ *devconfig.Fixture) {
			r.Equipped[0].Item.Version = math.MaxInt64
			q.ItemVersion = math.MaxInt64
		}, p.CodeSkillMaterialChanged},
		{"数量不足", func(r *domain.Role, _ *AssetRequest, _ *devconfig.Fixture) { r.Equipped[0].Item.Quantity = 0 }, p.CodeSkillMaterialRequired},
		{"不等价大符", func(r *domain.Role, _ *AssetRequest, _ *devconfig.Fixture) { r.Equipped[0].Item.ItemID = 188 }, p.CodeSkillMaterialRequired},
		{"背包不能直用", func(r *domain.Role, _ *AssetRequest, _ *devconfig.Fixture) {
			r.Items = append(r.Items, r.Equipped[0].Item)
			r.Equipped = nil
		}, p.CodeSkillMaterialRequired},
		{"旧场景门禁", func(_ *domain.Role, q *AssetRequest, _ *devconfig.Fixture) { q.CastGateCode = p.CodeStaleSession }, p.CodeStaleSession},
	}
	for _, test := range changes {
		t.Run(test.name, func(t *testing.T) {
			r, q, f := castingSupplyFixture()
			r.Equipped = []domain.Equipped{{Slot: 100, Item: r.Items[0]}}
			r.Items = nil
			test.change(&r, &q, &f)
			before := append([]domain.Equipped(nil), r.Equipped...)
			out, receipt, code := consumeCastingMaterial(r, q, f)
			if code != test.code || receipt != nil || !reflect.DeepEqual(before, r.Equipped) || !reflect.DeepEqual(out.Equipped, r.Equipped) {
				t.Fatalf("拒绝码%d/%d或修改原物品", code, test.code)
			}
		})
	}
}

func TestCastingSupplyUnequipFullBagAndDisabledUnknownUnits(t *testing.T) {
	r, q, f := castingSupplyFixture()
	r.Equipped = []domain.Equipped{{Slot: 100, Item: r.Items[0]}}
	r.Items = []domain.Item{{ID: domain.ID()}, {ID: domain.ID()}}
	q.Unequip = true
	if _, code := setCastingSupply(r, q, f); code != p.CodeOperationConflict || len(r.Equipped) != 1 {
		t.Fatal("满包卸符丢失物品")
	}
	f.CastingSupply.Enabled = false
	f.Items[0].MaxStack = 0
	if _, code := setCastingSupply(r, q, f); code != p.CodeSkillMaterialRequired {
		t.Fatal("单位未定停用却可装卸")
	}
}

func TestStatusUpgradeQuoteShowsRealRankParametersWithoutFakeDamage(t *testing.T) {
	config, err := devconfig.Load("../../../conf/dev/server.json")
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []uint32{31, 14, 15} {
		quote := skillQuote{Plan: skillPlan{Kind: 2, SkillID: id, From: 0, To: 1}}
		stats := quote.Proto(config.Fixture).Stats
		seen := map[string]bool{}
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
