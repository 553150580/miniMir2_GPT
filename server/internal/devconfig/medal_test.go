// 本文件核对勋章候选的独立材料、等级与有效属性。
package devconfig

import (
	"encoding/json"
	"math"
	"os"
	"testing"
)

// TestMedalConfigValidation 校验缺省关闭及材料、连续等级和算术上界。
func TestMedalConfigValidation(t *testing.T) {
	defaults, err := Load("../../../conf/dev/server.json")
	if err != nil {
		t.Fatal(err)
	}
	if defaults.Fixture.Medal != nil {
		t.Fatal("默认规则误开放隔离勋章")
	}
	for _, test := range []struct {
		name   string
		change func(*Fixture)
	}{
		{"有效候选", nil},
		{"缺少一级", func(f *Fixture) { f.Medal.Levels[0].Level = 2 }},
		{"费用为零", func(f *Fixture) { f.Medal.Levels[0].FragmentCost = 0 }},
		{"防御等级回退", func(f *Fixture) { f.Medal.Levels[1].Defense = 0 }},
		{"重复模板", func(f *Fixture) { f.Items = append(f.Items, f.Items[0]) }},
		{"复用羽毛", func(f *Fixture) { f.Wings = &WingConfig{FeatherItemID: 900061} }},
		{"复用药品", func(f *Fixture) { f.Potions.Effects = append(f.Potions.Effects, PotionEffect{ItemID: 900061}) }},
		{"独立材料误作装备", func(f *Fixture) { f.Items[0].IsEquipment = true }},
		{"旧规则启用", func(f *Fixture) { f.RuleVersion = 29 }},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := Fixture{
				RuleVersion: 30, DevelopmentOnly: true,
				Items: []Item{{ID: 900061, Name: "勋章碎片", Stackable: true, MaxStack: 99}},
				Medal: &MedalConfig{FragmentItemID: 900061, Levels: []MedalLevelConfig{
					{Level: 1, FragmentCost: 2, MainAttack: 1, Defense: 1},
					{Level: 2, FragmentCost: 3, MainAttack: 2, Defense: 1},
				}},
			}
			if test.change != nil {
				test.change(&fixture)
			}
			if err := fixture.validateMedal(); (err == nil) != (test.change == nil) {
				t.Fatalf("勋章候选校验结果与预期不符：%v", err)
			}
		})
	}
	var rules MedalConfig
	if err := json.Unmarshal([]byte(`{"fragment_item_id": 900061, "levels":[{"level":1,"gold":5}]}`), &rules); err == nil {
		t.Fatal("未识别的勋章费用字段应被拒绝")
	}
}

// TestMedalProjectedFixture 校验配置工具真实生成的双端规则所对应的服务端候选。
func TestMedalProjectedFixture(t *testing.T) {
	path := os.Getenv("MINIMIR_MEDAL_TEST_FIXTURE")
	if path == "" {
		return
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var fixture Fixture
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}
	if err := fixture.Validate(); err != nil {
		t.Fatal(err)
	}
	if fixture.Medal == nil || fixture.Medal.FragmentItemID != 900061 || len(fixture.Medal.Levels) != 3 {
		t.Fatal("勋章已装配档位与独立碎片不符")
	}
	if fixture.WorldBoss == nil || fixture.WorldBoss.Rewards == nil {
		t.Fatal("勋章候选缺失原世界首领奖励")
	}
	participation := fixture.WorldBoss.Rewards.Participation
	want := []ChallengeRewardItem{{ItemID: 900031, Quantity: 6}, {ItemID: 900061, Quantity: 6}}
	if len(participation) != len(want) || participation[0] != want[0] || participation[1] != want[1] {
		t.Fatalf("勋章替换了世界首领原参与奖或隔离奖数量不符：%+v", participation)
	}
}

// TestMedalCombatBonuses 检查三职业映射、缺省停用和叠加溢出拒绝。
func TestMedalCombatBonuses(t *testing.T) {
	fixture := Fixture{RuleVersion: 30, Medal: &MedalConfig{Levels: []MedalLevelConfig{{Level: 1, MainAttack: 2, Defense: 1}}}}
	for job := uint32(0); job < 3; job++ {
		base := Fighter{JobID: job, AttackMin: 1, AttackMax: 2, MagicMin: 2, MagicMax: 3, TaoMin: 3, TaoMax: 4,
			Defence: 1, DefenceMin: 1, DefenceMax: 2, MagicDefenceMin: 3, MagicDefenceMax: 4}
		expected := base
		targets := [][2]*int64{{&expected.AttackMin, &expected.AttackMax}, {&expected.MagicMin, &expected.MagicMax}, {&expected.TaoMin, &expected.TaoMax}}
		for _, target := range targets[job] {
			*target += 2
		}
		expected.Defence, expected.DefenceMin, expected.DefenceMax = 2, 2, 3
		if got, valid := fixture.ApplyMedalBonuses(base, 1); !valid || got != expected {
			t.Fatalf("职业 %d 的权威勋章属性不符：%+v", job, got)
		}
		if got, valid := fixture.ApplyMedalBonuses(base, 0); !valid || got != base {
			t.Fatalf("职业 %d 未激活勋章时改变了战斗属性", job)
		}
		if _, valid := fixture.ApplyMedalBonuses(base, 2); valid {
			t.Fatalf("职业 %d 获得了未配置档位的勋章属性", job)
		}
	}
	full := Fighter{AttackMin: 1, AttackMax: math.MaxInt64}
	fixture.Players = []Fighter{full}
	original := fixture
	original.Medal = nil
	if err := original.ValidateEquipmentStats(); err != nil {
		t.Fatalf("接入勋章前的职业区间应有效：%v", err)
	}
	if fixture.ValidateEquipmentStats() == nil {
		t.Fatal("最高勋章应随装备及职业最坏属性复核算术边界")
	}
	if got, valid := (&Fixture{}).ApplyMedalBonuses(full, 1); !valid || got != full {
		t.Fatal("关闭勋章时应忽略已保存的候选勋章等级")
	}
}
