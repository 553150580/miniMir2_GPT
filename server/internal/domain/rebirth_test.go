// 本文件核对待用经验、逐档解锁和旧规则回滚时的角色成长。
package domain

import (
	"reflect"
	"testing"
)

// TestRebirthReleasesPoolOnce 验证空池、半池和满池均只进入一级曲线。
func TestRebirthReleasesPoolOnce(t *testing.T) {
	contract := growthContract()
	contract.OperatingLevelCap, contract.ExpPerLevel = 5, []int64{10, 20, 30, 40, 50}
	for _, pending := range []int64{0, 27, 30} {
		role := growthRole()
		role.Level, role.PendingExperience, role.LegacyExperienceOverflow = 3, pending, 13
		out, move, err := ApplyRebirth(role, contract)
		expected := role
		expected.RebirthCount, expected.PendingExperience, expected.Experience = 1, 0, pending
		if pending == 30 {
			expected.Level, expected.Experience = 4, 0
		}
		if err != nil || !reflect.DeepEqual(expected, out) || move.PreviousCap != 3 || move.NewCap != 4 || move.ReleasedXP != pending {
			t.Fatalf("原经验池 %d 释放失败或改变其它角色事实：%+v %v", pending, move, err)
		}
		if pending != 30 {
			if same, _, err := ApplyRebirth(out, contract); err == nil || !reflect.DeepEqual(out, same) {
				t.Fatal("角色未升到新上限，不能继续转生")
			}
		}
		rolledBack, grant, err := ApplyExperience(out, 8, growthContract())
		if err != nil || !reflect.DeepEqual(out, rolledBack) || grant.Rejected != 8 || grant.ActualGranted != 0 {
			t.Fatal("旧规则截断了已经转生的原等级、次数或经验", grant, err)
		}
	}
}
