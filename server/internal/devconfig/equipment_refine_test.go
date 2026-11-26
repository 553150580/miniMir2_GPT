// 本文件核对洗炼候选的实际装载、原工作簿费用和来源边界。
package devconfig

import (
	"encoding/json"
	"os"
	"testing"
)

// TestEquipmentRefineProjectedFixtureAndDefault 同时检查默认关闭及可选的真实生成配置。
func TestEquipmentRefineProjectedFixtureAndDefault(t *testing.T) {
	defaultConfig, err := Load("../../../conf/dev/server.json")
	if err != nil {
		t.Fatal(err)
	}
	if defaultConfig.Fixture.EquipmentRefine != nil {
		t.Fatal("默认试玩目录误开隔离洗炼")
	}
	path := os.Getenv("MINIMIR_EQUIPMENT_REFINE_TEST_FIXTURE")
	if path == "" {
		return
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	load := func() Fixture {
		t.Helper()
		var fixture Fixture
		if err := json.Unmarshal(data, &fixture); err != nil {
			t.Fatal(err)
		}
		return fixture
	}
	fixture := load()
	if err := fixture.Validate(); err != nil {
		t.Fatal(err)
	}
	rules := fixture.EquipmentRefine
	if rules == nil || rules.StoneItemID != 900051 || rules.Recycle != (EquipmentRefineRecycleRule{
		ItemID: 220, Quality: 1, SourceKind: "monster_drop", ProbabilityPPM: 50_000, Quantity: 1,
	}) || !rules.Target(220, 1) || !rules.Target(910022, 2) || rules.Target(220, 0) || rules.Target(220, 2) {
		t.Fatal("装备模板、原怪物掉落渠道或石头编号不符")
	}
	fine, exists := rules.Quality(1)
	if !exists || fine.StoneCost != 2 || fine.SlotCount != 1 || fine.AllowDuplicateAttr {
		t.Fatal("精良的实际品质、槽数或费用不符", fine)
	}
	rare, exists := rules.Quality(2)
	if !exists || rare.StoneCost != 4 || rare.SlotCount != 2 || rare.AllowDuplicateAttr {
		t.Fatal("稀有的实际品质、槽数或费用不符", rare)
	}
	for _, row := range []struct {
		level uint32
		gold  int64
	}{{1, 1000}, {19, 1000}, {20, 3000}, {29, 3000}, {30, 12000}, {39, 12000}, {40, 64000}, {49, 64000}, {50, 160000}, {99, 160000}} {
		if gold, valid := rules.GoldCost(row.level); !valid || gold != row.gold {
			t.Fatal("金币费用不是当前装备需求等级档", row, gold, valid)
		}
	}
	for slots, expected := range []int64{0, 2_000, 5_000, 9_000} {
		if cost, valid := rules.LockCost(uint32(slots)); !valid || cost != expected {
			t.Fatal("锁槽的原元宝小单位费用错误", slots, cost)
		}
	}
	if value, valid := rules.ValueUnits(RefineAttributeMaxHP, 0, 4); !valid || value != 76 {
		t.Fatal("精良生命的原 1.9 倍定点结果不符", value)
	}
	if _, valid := rules.ValueUnits(RefineAttributeAttack, 0, 1); valid {
		t.Fatal("精良阶段被无依据配置攻击词条")
	}
	if value, valid := rules.ValueUnits(RefineAttributeAttack, 1, 2); !valid || value != 24 {
		t.Fatal("稀有的实际攻击词条定点结果不符", value)
	}
	for _, row := range []struct {
		name   string
		mutate func(*Fixture)
	}{
		{"关闭洗炼但保留石标志", func(f *Fixture) { f.EquipmentRefine = nil }},
		{"借用NPC来源", func(f *Fixture) { f.EquipmentRefine.Recycle.SourceKind = "npc_shop" }},
		{"缺少普通怪原装备池", func(f *Fixture) { f.OrdinaryDropPools = nil }},
		{"覆盖强化石", func(f *Fixture) { f.EquipmentEnhancement.StoneItemID = f.EquipmentRefine.StoneItemID }},
		{"错用精良目标品质", func(f *Fixture) { f.EquipmentRefine.Targets[0].Quality = 2 }},
		{"误将攻击放入精良", func(f *Fixture) { f.EquipmentRefine.Affixes[0].MinQuality = 1 }},
		{"档位权重溢出", func(f *Fixture) { f.EquipmentRefine.Tiers[0].WeightPPM++ }},
		{"缺少阶段合法词条", func(f *Fixture) { delete(f.EquipmentRefine.Affixes[1].BaseValuesU, 0) }},
		{"金币档中断", func(f *Fixture) { f.EquipmentRefine.GoldBands[0].MaxLevel-- }},
	} {
		t.Run(row.name, func(t *testing.T) {
			damaged := load()
			row.mutate(&damaged)
			if err := damaged.validateEquipmentRefine(); err == nil {
				t.Fatal("非法隔离洗炼目录被接受")
			}
		})
	}
}
