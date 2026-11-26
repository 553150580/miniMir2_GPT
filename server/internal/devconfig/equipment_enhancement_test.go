// 本文件覆盖部位强化的目录、算术与默认关闭契约。
package devconfig

import (
	"encoding/json"
	"math"
	"os"
	"testing"
)

// TestEquipmentContributionsRoundAfterSum 验证装备基础贡献和强化余数汇总后再取整。
func TestEquipmentContributionsRoundAfterSum(t *testing.T) {
	base := Fighter{AttackMin: 20, AttackMax: 30, MagicMin: 3, MagicMax: 6, AttackSpeed: 20, HP: 100, LuckAttackPPM: 82_000}
	item := Item{AttackBonus: 2, AttackMinBonus: 3, AttackMaxBonus: 6, MagicMinBonus: 5, MagicMaxBonus: 8, AttackSpeedBonus: -5}
	var contributions EquipmentContributions
	if !contributions.Add(item, 1_000) || !contributions.Add(item, 1_000) {
		t.Fatal("有效两槽贡献未被接受")
	}
	got, valid := contributions.Apply(base)
	if !valid || got.AttackMin != 31 || got.AttackMax != 47 || got.MagicMin != 14 || got.MagicMax != 23 || got.AttackSpeed != 9 ||
		got.HP != base.HP || got.LuckAttackPPM != base.LuckAttackPPM {
		t.Fatalf("汇总后取整、基础端点或独立属性错误：%+v", got)
	}
	var neutral EquipmentContributions
	if !neutral.Add(item, 0) {
		t.Fatal("未强化装备未被接受")
	}
	got, valid = neutral.Apply(base)
	legacy, legacyValid := item.AddEquipmentBonuses(base)
	if !valid || !legacyValid || got != legacy {
		t.Fatalf("零级强化改变原装备贡献：%+v != %+v", got, legacy)
	}
}

// TestEquipmentContributionsOverflow 验证带符号的定点乘法、跨部位叠加和原闭区间防护。
func TestEquipmentContributionsOverflow(t *testing.T) {
	for _, value := range []int64{math.MaxInt64, math.MinInt64} {
		var contributions EquipmentContributions
		if contributions.Add(Item{AttackSpeedBonus: value}, 10) {
			t.Fatal("越界的装备定点乘法未拒绝", value)
		}
	}
	var contributions EquipmentContributions
	if !contributions.Add(Item{TaoMaxBonus: math.MaxInt64 / 100}, 100) ||
		contributions.Add(Item{TaoMaxBonus: math.MaxInt64 / 100}, 100) {
		t.Fatal("同一属性的两槽定点加法未阻止溢出")
	}
	got, valid := contributions.Apply(Fighter{})
	if !valid || got.TaoMax != math.MaxInt64/100+(math.MaxInt64/100)*100/10_000 {
		t.Fatalf("被拒的装备污染了有效贡献：%+v", got)
	}
	var rangeContributions EquipmentContributions
	if !rangeContributions.Add(Item{DefenceMaxBonus: math.MaxInt64}, 0) {
		t.Fatal("闭区间端点的原整数数据未被接受")
	}
	if _, valid := rangeContributions.Apply(Fighter{}); valid {
		t.Fatal("闭区间长度的加一溢出未拒绝")
	}
}

// TestEquipmentEnhancementDirectory 拒绝缺失阶段、越界概率、材料混用和未掌握的新配置。
func TestEquipmentEnhancementDirectory(t *testing.T) {
	makeFixture := func() Fixture {
		levels := make([]EquipmentEnhancementLevel, maxEnhancementLevel)
		for index := range levels {
			stage := index/10 + 1
			levels[index] = EquipmentEnhancementLevel{uint32(index), int64(10 * stage), uint32((10 - index%10) * 100_000), uint32(10 * stage)}
		}
		return Fixture{RuleVersion: 30, Items: []Item{{ID: 900041, Name: "强化石", Stackable: true, MaxStack: 99}},
			EquipmentEnhancement: &EquipmentEnhancementConfig{StoneItemID: 900041, Levels: levels}}
	}
	fixture := makeFixture()
	if err := fixture.validateEquipmentEnhancement(); err != nil {
		t.Fatal(err)
	}
	rules := fixture.EquipmentEnhancement
	step, exists := rules.Level(10)
	if !exists || step.StoneCost != 20 || step.SuccessPPM != 1_000_000 || step.BonusBP != 20 ||
		rules.BonusBP(11) != 120 || rules.BonusBP(50) != 1_500 || rules.BonusBP(51) != 0 {
		t.Fatal("目录顺序、阶段边界或累计基点不符", step)
	}
	if _, exists := rules.Level(50); exists {
		t.Fatal("未经配置的 51 级被开放")
	}
	for _, test := range []struct {
		name   string
		mutate func(*Fixture)
	}{
		{"缺少一档", func(f *Fixture) { f.EquipmentEnhancement.Levels = f.EquipmentEnhancement.Levels[1:] }},
		{"重复档位", func(f *Fixture) { f.EquipmentEnhancement.Levels[1].CurrentLevel = 0 }},
		{"免费尝试", func(f *Fixture) { f.EquipmentEnhancement.Levels[0].StoneCost = 0 }},
		{"概率溢出", func(f *Fixture) { f.EquipmentEnhancement.Levels[0].SuccessPPM = 1_000_001 }},
		{"超出强化预算", func(f *Fixture) { f.EquipmentEnhancement.Levels[49].BonusBP++ }},
		{"用装备充当材料", func(f *Fixture) { f.Items[0].IsEquipment = true }},
		{"材料键重复", func(f *Fixture) { f.Items = append(f.Items, f.Items[0]) }},
		{"药水混用", func(f *Fixture) { f.Potions.Effects = append(f.Potions.Effects, PotionEffect{ItemID: f.Items[0].ID}) }},
		{"沿用羽毛", func(f *Fixture) { f.Wings = &WingConfig{FeatherItemID: f.Items[0].ID} }},
	} {
		t.Run(test.name, func(t *testing.T) {
			invalid := makeFixture()
			test.mutate(&invalid)
			if invalid.validateEquipmentEnhancement() == nil {
				t.Fatal("错误的部位强化配置被接受")
			}
		})
	}
	if err := json.Unmarshal([]byte(`{"stone_item_id":900041,"levels":[{"current_level":0,"gold_cost":100}]}`), rules); err == nil {
		t.Fatal("未知金币费用字段被部位强化配置接受")
	}
}

// TestEquipmentEnhancementProjectedFixtureAndDefault 核查默认目录及可选的生成候选真库配置。
func TestEquipmentEnhancementProjectedFixtureAndDefault(t *testing.T) {
	defaultConfig, err := Load("../../../conf/dev/server.json")
	if err != nil {
		t.Fatal(err)
	}
	if defaultConfig.Fixture.EquipmentEnhancement != nil {
		t.Fatal("默认规则开启了隔离部位强化")
	}
	path := os.Getenv("MINIMIR_EQUIPMENT_ENHANCEMENT_TEST_FIXTURE")
	if path == "" {
		return
	}
	payload, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var candidate Fixture
	if err := json.Unmarshal(payload, &candidate); err != nil {
		t.Fatal(err)
	}
	if err := candidate.Validate(); err != nil {
		t.Fatal(err)
	}
	rules := candidate.EquipmentEnhancement
	if rules == nil || rules.BonusBP(50) != 1_500 {
		t.Fatal("未装载完整隔离强化候选")
	}
	first, _ := rules.Level(0)
	second, _ := rules.Level(1)
	boss := candidate.PersonalBossByTemplate(1)
	if first.StoneCost != 10 || first.SuccessPPM != 1_000_000 || second.SuccessPPM != 900_000 || boss == nil ||
		boss.ItemReward == nil || len(boss.ItemReward.Items) != 1 ||
		boss.ItemReward.Items[0] != (ChallengeRewardItem{ItemID: rules.StoneItemID, Quantity: 10}) {
		t.Fatal("隔离强化石未接入原个人首领奖励", first, second, boss)
	}
}
