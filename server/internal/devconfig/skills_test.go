package devconfig

import "testing"

func TestSourcePlayerGrowthThreeJobs(t *testing.T) {
	cfg, err := Load("../../../conf/dev/server.json")
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []struct {
		job                                    uint32
		hp, mp, dc, ac, mc, sc, macMin, macMax int64
	}{
		{0, 60, 35, 1, 1, 0, 0, 0, 0}, {1, 29, 65, 1, 0, 1, 0, 0, 0}, {2, 39, 26, 1, 0, 0, 1, 0, 1},
	} {
		a, ok := cfg.Fixture.PlayerAtLevel(expected.job, 7)
		if !ok || a.HP != expected.hp || a.MP != expected.mp || a.AttackMin != expected.dc || a.AttackMax != expected.dc || a.DefenceMax != expected.ac || a.MagicMin != expected.mc || a.TaoMin != expected.sc || a.MagicDefenceMin != expected.macMin || a.MagicDefenceMax != expected.macMax {
			t.Fatalf("职业%d来源7级成长不符: %+v", expected.job, a)
		}
	}
}

func TestSkillFixtureRejectsInvalidSourceBounds(t *testing.T) {
	for _, change := range []func(*Fixture){
		func(f *Fixture) { f.Healing.PetBasePerTick = 0 },
		func(f *Fixture) {
			f.Skills[0].CooldownReductionMS = f.Skills[0].CooldownBaseMS + 1
			f.Skills[0].LearnedLevel = 1
		},
		func(f *Fixture) { f.PlayerGrowth[0].HPGain = -1 },
		func(f *Fixture) { f.Players[0].MagicResistWeight = 0 },
		func(f *Fixture) { f.Skills[0].BookItemID = 0 },
	} {
		cfg, err := Load("../../../conf/dev/server.json")
		if err != nil {
			t.Fatal(err)
		}
		change(&cfg.Fixture)
		if cfg.Fixture.Validate() == nil {
			t.Fatal("失效技能或成长配置被接受")
		}
	}
}
