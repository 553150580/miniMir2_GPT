// 本文件验证同一属性管线使用职业和装备后的基础，在最后一次取整封顶。
package devconfig

import "testing"

// TestBodyTrainingVitalityBudget 检查跨系隔离、同一上限截断及角色实际等级约束。
func TestBodyTrainingVitalityBudget(t *testing.T) {
	f := Fixture{BodyTraining: &BodyTrainingConfig{HealthCapBP: 500, ManaCapBP: 500,
		Tracks: []BodyTrainingTrack{{ID: BodyHealth, BonusPerRank: 2}, {ID: BodyMana, BonusPerRank: 1}},
		Steps:  make([]BodyTrainingStep, 12)}}
	low := Fighter{HP: 99, MP: 39, Level: 12, AttackMin: 7, AttackMS: 800}
	out, ok := f.ApplyBodyTraining(low, 4, 3)
	if !ok || out.HP != 103 || out.MP != 40 || out.AttackMin != low.AttackMin || out.AttackMS != low.AttackMS {
		t.Fatalf("生命与魔法应以各自装备基础向下封顶：%+v", out)
	}
	geared := low
	geared.HP, geared.MP = 139, 100
	out, ok = f.ApplyBodyTraining(geared, 4, 3)
	if !ok || out.HP != 145 || out.MP != 103 {
		t.Fatalf("各系相同级别应使用换装后的合法基础：%+v", out)
	}
	geared.Level = 1
	out, ok = f.ApplyBodyTraining(geared, 50, 50)
	if !ok || out.HP != 141 || out.MP != 101 {
		t.Fatalf("实际角色等级应约束可生效的原等级：%+v", out)
	}
}
