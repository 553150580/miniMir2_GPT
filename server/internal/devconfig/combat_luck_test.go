// 本文件验证幸运战斗装载、隔离启用和基础模板越权配置的边界。
package devconfig

import (
	"encoding/json"
	"math"
	"os"
	"slices"
	"testing"
)

func TestCombatLuckConfigValidatesSources(t *testing.T) {
	newFixture := func() Fixture {
		return Fixture{
			Players: []Fighter{{JobID: 0}, {JobID: 1}, {JobID: 2}}, Monsters: []Monster{{ID: 1}}, ChallengeMonsters: []Monster{{ID: 80001}},
			PetRuntime: &PetRuntimeRules{Definitions: []PetRuntimeDefinition{{ID: 1}}},
			CombatLuck: &CombatLuckConfig{
				WeaponWingPPM: []uint32{0, 20_000, 30_000, 44_000, 60_000, 80_000, 110_000, 150_000, 210_000, 300_000},
				OtherPPM:      []uint32{0, 20_000, 30_000, 44_500},
				DropPPM:       10_000, InitialWeights: []uint32{1, 1, 1},
				MonsterResistancePPM: map[uint32]uint32{1: 200_000, 80_001: 200_000}, RoleResistancePPM: map[uint32]uint32{1: 200_000},
			},
		}
	}
	fixture := newFixture()
	if err := fixture.validateCombatLuck(); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name   string
		mutate func(*Fixture)
	}{
		{"武器档位缺失", func(f *Fixture) { f.CombatLuck.WeaponWingPPM = f.CombatLuck.WeaponWingPPM[:4] }},
		{"其他装备错档", func(f *Fixture) { f.CombatLuck.OtherPPM = append(f.CombatLuck.OtherPPM, 60000) }},
		{"零级带幸运", func(f *Fixture) { f.CombatLuck.OtherPPM[0] = 1 }},
		{"档位反向下降", func(f *Fixture) { f.CombatLuck.WeaponWingPPM[9] = 1 }},
		{"贡献超过百分百", func(f *Fixture) { f.CombatLuck.OtherPPM[3] = 1_000_001 }},
		{"掉落概率超过百分百", func(f *Fixture) { f.CombatLuck.DropPPM = 1_000_001 }},
		{"条件权重缺档", func(f *Fixture) { f.CombatLuck.InitialWeights = f.CombatLuck.InitialWeights[:1] }},
		{"条件权重为零", func(f *Fixture) { f.CombatLuck.InitialWeights[1] = 0 }},
		{"条件权重溢出", func(f *Fixture) { f.CombatLuck.InitialWeights[0] = math.MaxUint32 }},
		{"不存在的怪物", func(f *Fixture) { f.CombatLuck.MonsterResistancePPM[10] = 1 }},
		{"怪物抵抗超过百分百", func(f *Fixture) { f.CombatLuck.MonsterResistancePPM[1] = 1_000_001 }},
		{"不存在的职业", func(f *Fixture) { f.CombatLuck.RoleResistancePPM[3] = 1 }},
		{"角色抵抗超过百分百", func(f *Fixture) { f.CombatLuck.RoleResistancePPM[1] = 1_000_001 }},
		{"角色直接配置贡献", func(f *Fixture) { f.Players[1].LuckAttackPPM = 1 }},
		{"普通怪直接配置抵抗", func(f *Fixture) { f.Monsters[0].LuckResistancePPM = 1 }},
		{"首领直接配置抵抗", func(f *Fixture) { f.ChallengeMonsters[0].LuckResistancePPM = 1 }},
		{"宠物直接配置贡献", func(f *Fixture) { f.Pet.LuckAttackPPM = 1 }},
		{"宠物来源直接配置贡献", func(f *Fixture) { f.PetRuntime.Definitions[0].Base.LuckAttackPPM = 1 }},
		{"关闭也不允许直接配置", func(f *Fixture) { f.CombatLuck = nil; f.Players[1].LuckAttackPPM = 1 }},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newFixture()
			test.mutate(&fixture)
			if fixture.validateCombatLuck() == nil {
				t.Fatal("无效幸运配置通过装载")
			}
		})
	}
	if err := json.Unmarshal([]byte(`{"combat_luck":{"monster_resist_ppm":{"1":100}}}`), &fixture); err == nil {
		t.Fatal("拼写错误的抵抗目录未被拒绝")
	}
}

func TestCombatLuckProjectedFixtureAndDefault(t *testing.T) {
	normal, err := Load("../../../conf/dev/server.json")
	if err != nil {
		t.Fatal(err)
	}
	if normal.Fixture.CombatLuck != nil {
		t.Fatal("默认试玩已开放隔离幸运")
	}
	path := os.Getenv("MINIMIR_LUCK_TEST_FIXTURE")
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
	luck := candidate.CombatLuck
	if luck == nil || luck.DropPPM != 10_000 || !slices.Equal(luck.InitialWeights, []uint32{1, 1, 1}) ||
		!slices.Equal(luck.WeaponWingPPM, []uint32{0, 20_000, 30_000, 44_000, 60_000, 80_000, 110_000, 150_000, 210_000, 300_000}) ||
		!slices.Equal(luck.OtherPPM, []uint32{0, 20_000, 30_000, 44_500}) ||
		luck.MonsterResistancePPM[1] != 200_000 || luck.RoleResistancePPM[1] != 200_000 {
		t.Fatal("完整幸运候选与当前工作簿、隔离抵抗不一致", luck)
	}
}
