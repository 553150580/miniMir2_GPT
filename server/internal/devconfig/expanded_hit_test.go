// 本文件核对扩展命中属性的统一封顶、精度和来源额度校验。
package devconfig

import (
	"math"
	"testing"
)

func TestExpandedHitBudget(t *testing.T) {
	budget := &ExpandedHitBudget{DamageCapBP: 2000, AbsorptionCapBP: 2000}
	total := budget.Combine(HitBonuses{DamageBP: 1500, AbsorptionBP: 700}, HitBonuses{DamageBP: math.MaxUint32, AbsorptionBP: 500})
	if total != (HitBonuses{DamageBP: 2000, AbsorptionBP: 1200}) {
		t.Fatal("同类加算及两组独立封顶错误", total)
	}
	rounding := budget.Combine(HitBonuses{DamageBP: 375}, HitBonuses{DamageBP: 375}, HitBonuses{DamageBP: 375})
	if got := budget.Apply(23, rounding.DamageBP, 0, 0); got != 25 {
		t.Fatal("取整应发生在同类来源合并之后", got)
	}
	if got := budget.Apply(127, total.DamageBP, total.AbsorptionBP, 0); got != 134 {
		t.Fatal("增伤和吸收的前后顺序或取整错误", got)
	}
	if got := budget.Apply(100, 1000, 1200, 2500); got != 119 {
		t.Fatal("增伤和暴击应共用原净伤基数，最后统一计算吸收", got)
	}
	if got := budget.Apply(math.MaxInt64, 2000, 2000, 0); got != 8_854_437_155_380_584_775 {
		t.Fatal("大伤害在增伤后、吸收前被过早截断", got)
	}
	if budget.Apply(math.MaxInt64, 2000, 0, 0) != math.MaxInt64 || budget.Apply(0, 2000, 1200, 0) != 0 {
		t.Fatal("伤害边界、未命中保留错误")
	}
}

func TestBodyHitBudgetSourceBounds(t *testing.T) {
	f := Fixture{RuleVersion: 30, ExpandedHitBudget: &ExpandedHitBudget{DamageCapBP: 2000, AbsorptionCapBP: 2000},
		BodyTraining: &BodyTrainingConfig{Tracks: []BodyTrainingTrack{{ID: BodyAbsorption, Name: "伤害吸收", GoldBase: 1500, BonusPerRank: 100, ShareCapBP: 200}},
			Steps:       []BodyTrainingStep{{From: 0, CostMultiplier: 1, SuccessPPM: BodyChancePPM}, {From: 1, CostMultiplier: 1, SuccessPPM: 900_000}},
			HealthCapBP: 500, ManaCapBP: 500}}
	f.Progression.CatalogMaxLevel = 50
	if err := f.validateExpandedHit(); err != nil {
		t.Fatal(err)
	}
	if err := f.validateBodyTraining(); err != nil {
		t.Fatal(err)
	}
	if got := f.BodyHitBonuses(1, 10, 2); got != (HitBonuses{AbsorptionBP: 100}) {
		t.Fatal("旧档和未配置系别未按有效等级截断", got)
	}
	f.BodyTraining.Tracks[0].ShareCapBP = 199
	if f.validateBodyTraining() == nil {
		t.Fatal("收费档位超过来源可提升范围")
	}
	f.Players = []Fighter{{HitBonuses: HitBonuses{DamageBP: 1}}}
	if f.validateExpandedHit() == nil {
		t.Fatal("基础模板绕过了统一来源组")
	}
}
