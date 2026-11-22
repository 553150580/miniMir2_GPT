// 本文件比对三职业来源成长与技能输入上下界。

package devconfig

import "testing"

// TestSourcePlayerGrowthThreeJobs 核对「职业来源7级成长不符」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestSourcePlayerGrowthThreeJobs(t *testing.T) {
	// cfg 保存由加载取得的服务配置，err 接收加载过程中可能产生的错误，供后续失败分支处理。
	cfg, err := Load("../../../conf/dev/server.json")
	if err != nil {
		t.Fatal(err)
	}
	// expected 接收遍历临时结构列表时当前的元素。
	for _, expected := range []struct {
		// job 记录任务或职业编号，使用数值表示。
		job uint32
		// hp 记录生命值，使用数值表示。
		// mp 记录魔法值，使用数值表示。
		// dc 记录“dc”，使用数值表示。
		// ac 记录“ac”，使用数值表示。
		// mc 记录“mc”，使用数值表示。
		// sc 记录“sc”，使用数值表示。
		// macMin 记录“mac”最小，使用数值表示。
		// macMax 记录“mac”最大，使用数值表示。
		hp, mp, dc, ac, mc, sc, macMin, macMax int64
	}{
		{0, 60, 35, 1, 1, 0, 0, 0, 0}, {1, 29, 65, 1, 0, 1, 0, 0, 0}, {2, 39, 26, 1, 0, 0, 1, 0, 1},
	} {
		// a 保存由玩家发生时间等级取得的a，ok 表明查询、校验或类型转换是否成功。
		a, ok := cfg.Fixture.PlayerAtLevel(expected.job, 7)
		if !ok || a.HP != expected.hp || a.MP != expected.mp || a.AttackMin != expected.dc || a.AttackMax != expected.dc || a.DefenceMax != expected.ac || a.MagicMin != expected.mc || a.TaoMin != expected.sc || a.MagicDefenceMin != expected.macMin || a.MagicDefenceMax != expected.macMax {
			t.Fatalf("职业%d来源7级成长不符: %+v", expected.job, a)
		}
	}
}

// TestSkillFixtureRejectsInvalidSourceBounds 核验拒绝失效技能或成长配置。
// t 提供断言失败报告、子用例和清理注册。
func TestSkillFixtureRejectsInvalidSourceBounds(t *testing.T) {
	// change 接收遍历供当前流程调用的函数列表时当前的元素。
	for _, change := range []func(*Fixture){
		// f 记录开发测试配置。
		func(f *Fixture) { f.Healing.PetBasePerTick = 0 },
		// f 记录开发测试配置。
		func(f *Fixture) {
			f.Skills[0].CooldownReductionMS = f.Skills[0].CooldownBaseMS + 1
			f.Skills[0].LearnedLevel = 1
		},
		// f 记录开发测试配置。
		func(f *Fixture) { f.PlayerGrowth[0].HPGain = -1 },
		// f 记录开发测试配置。
		func(f *Fixture) { f.Players[0].MagicResistWeight = 0 },
		// f 记录开发测试配置。
		func(f *Fixture) { f.Skills[0].BookItemID = 0 },
	} {
		// cfg 保存由加载取得的服务配置，err 接收加载过程中可能产生的错误，供后续失败分支处理。
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
