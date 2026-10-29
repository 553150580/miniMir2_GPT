package devconfig

import (
	"encoding/json"
	"math"
	"strings"
	"testing"
)

func equipmentStatsFixture(t *testing.T) Fixture {
	t.Helper()
	var item Item
	if err := json.Unmarshal([]byte(`{"id":218,"is_equipment":true,"required_level":1,"equipment_stage":0,"magic_min_bonus":0,"magic_max_bonus":1,"tao_min_bonus":0,"tao_max_bonus":2}`), &item); err != nil {
		t.Fatal(err)
	}
	return Fixture{RuleVersion: 22, Items: []Item{item}, Players: []Fighter{{JobID: 1, MagicMin: 1, MagicMax: 1, TaoMin: 1, TaoMax: 1}}, Progression: ProgressionRules{CatalogMaxLevel: 50}}
}

func TestEquipmentStatsExplicitEndpointsAndStage(t *testing.T) {
	valid := `{"id":218,"is_equipment":true,"required_level":1,"equipment_stage":0,"magic_min_bonus":0,"magic_max_bonus":1,"tao_min_bonus":0,"tao_max_bonus":2}`
	for _, key := range equipmentStatFields {
		for _, replacement := range []string{"missing", "null", "\"1\"", "1.5", "9223372036854775808"} {
			t.Run(key+"/"+replacement, func(t *testing.T) {
				var raw map[string]json.RawMessage
				if err := json.Unmarshal([]byte(valid), &raw); err != nil {
					t.Fatal(err)
				}
				if replacement == "missing" {
					delete(raw, key)
				} else {
					raw[key] = json.RawMessage(replacement)
				}
				data, _ := json.Marshal(raw)
				f := equipmentStatsFixture(t)
				if err := json.Unmarshal(data, &f.Items[0]); err == nil && f.ValidateEquipmentStats() == nil {
					t.Fatal("不完整或错误类型的装备属性被当成显式零")
				}
			})
		}
	}
	f := equipmentStatsFixture(t)
	if err := f.ValidateEquipmentStats(); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*Item){func(i *Item) { i.MagicMinBonus = -1 }, func(i *Item) { i.MagicMinBonus = 2 }, func(i *Item) { i.TaoMinBonus = 3 }, func(i *Item) { i.EquipmentStage = 1 }, func(i *Item) { i.RequiredLevel = nil }} {
		bad := equipmentStatsFixture(t)
		mutate(&bad.Items[0])
		if bad.ValidateEquipmentStats() == nil {
			t.Fatal("非法装备区间/阶段未拒绝")
		}
	}
}

func TestEquipmentStatsLegacyNeutralRoundTripAndNonEquipment(t *testing.T) {
	f := equipmentStatsFixture(t)
	f.RuleVersion = 21
	f.Items[0].MagicMaxBonus, f.Items[0].TaoMaxBonus = 0, 0
	encoded, err := json.Marshal(f.Items[0])
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(encoded), `"equipment_stage":0`) {
		t.Fatal("零阶段被序列化丢失")
	}
	if err = json.Unmarshal(encoded, &f.Items[0]); err != nil {
		t.Fatal(err)
	}
	if err = f.ValidateEquipmentStats(); err != nil {
		t.Fatal(err)
	}
	f.Items[0].MagicMaxBonus = 1
	if f.ValidateEquipmentStats() == nil {
		t.Fatal("旧规则接受非中性MC")
	}
	f = equipmentStatsFixture(t)
	f.Items[0].IsEquipment = false
	if f.ValidateEquipmentStats() == nil {
		t.Fatal("非装备带属性未拒绝")
	}
	f.Items[0].MagicMaxBonus, f.Items[0].TaoMaxBonus = 0, 0
	if err = f.ValidateEquipmentStats(); err != nil {
		t.Fatal(err)
	}
	f = equipmentStatsFixture(t)
	f.Items[0].RequiredLevel = new(uint32(11))
	f.Items[0].EquipmentStage = 1
	if err = f.ValidateEquipmentStats(); err != nil {
		t.Fatal(err)
	}
}

func TestEquipmentStatsWorstCombinationAndSkillArithmetic(t *testing.T) {
	f := equipmentStatsFixture(t)
	f.Items[0].TaoMaxBonus = math.MaxInt64/2 + 1
	second := f.Items[0]
	second.ID = 219
	second.EquipmentSlot = 1
	f.Items = append(f.Items, second)
	if f.ValidateEquipmentStats() == nil {
		t.Fatal("两个槽位累加溢出未拒绝")
	}
	f = equipmentStatsFixture(t)
	f.Items[0].MagicMaxBonus = math.MaxInt64
	if f.ValidateEquipmentStats() == nil {
		t.Fatal("闭区间长度或基础累加溢出未拒绝")
	}
	f = equipmentStatsFixture(t)
	f.Items[0].TaoMaxBonus = math.MaxInt64 / 2
	f.Skills = []Skill{{JobID: 1, Behavior: "healing", HealingStatMultiplier: 3, MaxLevel: 3}}
	if f.ValidateEquipmentStats() == nil {
		t.Fatal("SC治疗乘法溢出未拒绝")
	}
	f.Skills = []Skill{{JobID: 1, Behavior: "blessed_armour", BuffID: 15, MaxLevel: 3}}
	f.Buffs = []BuffDefinition{{ID: 15, DurationKind: "sc_rank_linear", StatMultiplier: 3, DurationSecondsByLevel: []int64{1, 2, 3, 4}}}
	if f.ValidateEquipmentStats() == nil {
		t.Fatal("SC时长乘法溢出未拒绝")
	}
	f = equipmentStatsFixture(t)
	f.Items[0].MagicMaxBonus = math.MaxInt64 / 2
	f.Skills = []Skill{{JobID: 1, Behavior: "magic_shield", BuffID: 31, MaxLevel: 3}}
	f.Buffs = []BuffDefinition{{ID: 31, DurationKind: "mc_source_power", StatAdd: math.MaxInt64/2 + 1}}
	if f.ValidateEquipmentStats() == nil {
		t.Fatal("MC时长加法或Single转整数溢出未拒绝")
	}
}
