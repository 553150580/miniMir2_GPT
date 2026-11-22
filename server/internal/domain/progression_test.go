// 本文件通过确定的经验输入验证等级计算、迁移、经验池和数值上界。

package domain

import (
	"math"
	"reflect"
	"testing"
)

// growthContract 返回成长规则合同供调用方使用。
func growthContract() GrowthContract {
	return GrowthContract{FormatVersion: 1, CurveVersion: "explicit-test-curve", InitialLevelCap: 3, RebirthLevelIncrement: 1, OperatingLevelCap: 3, ExpPerLevel: []int64{10, 20, 30}}
}

// growthRole 返回成长角色供调用方使用。
func growthRole() Role {
	return Role{Level: 1, ProgressionFormatVersion: 1, ProgressionCurveVersion: growthContract().CurveVersion, Revision: 9, EconomicSeq: 4, SourceSeq: 3, Wallet: Wallet{BoundGold: 7}}
}

// TestExperienceGrantConsumesMultipleLevelsThenBoundedPool 核对「纯成长修改了其它永久事实」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestExperienceGrantConsumesMultipleLevelsThenBoundedPool(t *testing.T) {
	// r 接收成长角色供后续步骤使用。
	r := growthRole()
	// out 保存由应用经验取得的输出，g 保存应用经验返回的结果，err 接收应用经验过程中可能产生的错误，供后续失败分支处理。
	out, g, err := ApplyExperience(r, 75, growthContract())
	if err != nil || out.Level != 3 || out.Experience != 0 || out.PendingExperience != 30 || g.RawExperience != 75 || g.ActualGranted != 60 || g.ToProgress != 30 || g.ToPool != 30 || g.Rejected != 15 || g.LevelBefore != 1 || g.LevelAfter != 3 {
		t.Fatal(out, g, err)
	}
	// want 保存当前处理所用的期望值。
	want := r
	want.Level = 3
	want.PendingExperience = 30
	if !reflect.DeepEqual(out, want) {
		t.Fatal("纯成长修改了其它永久事实", out, want)
	}
}

// TestExperienceFullPoolRejectsRewardWithoutOverflowOrLegacyGrowth 验证经验池已满时拒绝全部新经验，维持等级、角色状态和已封存的历史余额。
// t 提供断言失败报告、子用例和清理注册。
func TestExperienceFullPoolRejectsRewardWithoutOverflowOrLegacyGrowth(t *testing.T) {
	// r 接收成长角色供后续步骤使用。
	r := growthRole()
	r.Level = 3
	r.PendingExperience = 30
	r.LegacyExperienceOverflow = math.MaxInt64
	// out 保存由应用经验取得的输出，g 保存应用经验返回的结果，err 接收应用经验过程中可能产生的错误，供后续失败分支处理。
	out, g, err := ApplyExperience(r, math.MaxInt64, growthContract())
	if err != nil || !reflect.DeepEqual(out, r) || g.ActualGranted != 0 || g.Rejected != math.MaxInt64 || g.ToPool != 0 || g.ToProgress != 0 {
		t.Fatal(out, g, err)
	}
}

// TestExperienceNearMaxThresholdUsesHeadroomWithoutOverflow 验证接近整数上限时仅用所需经验升级，剩余经验安全转入待用池。
// t 提供断言失败报告、子用例和清理注册。
func TestExperienceNearMaxThresholdUsesHeadroomWithoutOverflow(t *testing.T) {
	// r 接收成长角色供后续步骤使用。
	r := growthRole()
	r.Experience = math.MaxInt64 - 1
	// c 接收成长规则合同供后续步骤使用。
	c := growthContract()
	c.InitialLevelCap = 2
	c.OperatingLevelCap = 2
	c.ExpPerLevel = []int64{math.MaxInt64, math.MaxInt64}
	// out 保存由应用经验取得的输出，g 保存应用经验返回的结果，err 接收应用经验过程中可能产生的错误，供后续失败分支处理。
	out, g, err := ApplyExperience(r, math.MaxInt64, c)
	if err != nil || out.Level != 2 || out.PendingExperience != math.MaxInt64-1 || g.ActualGranted != math.MaxInt64 || g.ToProgress != 1 || g.ToPool != math.MaxInt64-1 || g.Rejected != 0 {
		t.Fatal(out, g, err)
	}
}

// TestLegacyExperienceMigratesWithoutCountingAsNewReward 核对「历史迁移重复消费」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestLegacyExperienceMigratesWithoutCountingAsNewReward(t *testing.T) {
	// r 接收成长角色供后续步骤使用。
	r := growthRole()
	r.ProgressionFormatVersion = 0
	r.ProgressionCurveVersion = ""
	r.Experience = 100
	// out 保存由应用经验取得的输出，g 保存应用经验返回的结果，err 接收应用经验过程中可能产生的错误，供后续失败分支处理。
	out, g, err := ApplyExperience(r, 5, growthContract())
	if err != nil || out.Level != 3 || out.PendingExperience != 30 || out.LegacyExperienceOverflow != 40 || out.Experience != 0 || out.ProgressionFormatVersion != 1 || g.ActualGranted != 0 || g.Rejected != 5 || g.LevelBefore != 3 || g.LevelAfter != 3 || g.MigrationBefore == nil || g.MigrationBefore.Experience != 100 || g.MigrationAfter.LegacyExperienceOverflow != 40 {
		t.Fatal(out, g, err)
	}
	// again 保存由应用经验取得的“again”，g2 保存应用经验返回的结果。
	again, g2, err := ApplyExperience(out, 0, growthContract())
	if err != nil || !reflect.DeepEqual(again, out) || g2.MigrationBefore != nil || g2.MigrationAfter != nil {
		t.Fatal("历史迁移重复消费", again, g2, err)
	}
}

// TestLegacyLevelAboveOperatingCapPreservesLevelAndSealsBalance 核对「未确认迁移角色被降级或推定转生」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestLegacyLevelAboveOperatingCapPreservesLevelAndSealsBalance(t *testing.T) {
	// r 接收成长角色供后续步骤使用。
	r := growthRole()
	r.Level = 99
	r.ProgressionFormatVersion = 0
	r.ProgressionCurveVersion = ""
	r.Experience = math.MaxInt64
	// out 保存由应用经验取得的输出，g 保存应用经验返回的结果，err 接收应用经验过程中可能产生的错误，供后续失败分支处理。
	out, g, err := ApplyExperience(r, 15, growthContract())
	if err != nil || out.Level != 99 || out.RebirthCount != 0 || !out.ProgressionMigrationRequired || out.LegacyExperienceOverflow != math.MaxInt64 || out.PendingExperience != 0 || out.Experience != 0 || g.ActualGranted != 0 || g.Rejected != 15 {
		t.Fatal(out, g, err)
	}
	// again 保存由应用经验取得的“again”，g2 保存应用经验返回的结果。
	again, g2, err := ApplyExperience(out, 15, growthContract())
	if err != nil || !reflect.DeepEqual(again, out) || g2.Rejected != 15 {
		t.Fatal("未确认迁移角色被降级或推定转生", again, g2, err)
	}
}

// TestLegacyProgressionKeepsCurrentLevelRatherThanReplayingLowerLevels 验证迁移保留角色的实际等级和旧经验余额，新入账经验从该等级继续。
// t 提供断言失败报告、子用例和清理注册。
func TestLegacyProgressionKeepsCurrentLevelRatherThanReplayingLowerLevels(t *testing.T) {
	// r 接收成长角色供后续步骤使用。
	r := growthRole()
	r.ProgressionFormatVersion = 0
	r.ProgressionCurveVersion = ""
	r.Level = 2
	r.Experience = 15
	// out 保存由应用经验取得的输出，g 保存应用经验返回的结果，err 接收应用经验过程中可能产生的错误，供后续失败分支处理。
	out, g, err := ApplyExperience(r, 10, growthContract())
	if err != nil || out.Level != 3 || out.PendingExperience != 5 || g.LevelBefore != 2 || g.LevelAfter != 3 || g.ActualGranted != 10 || g.ToProgress != 5 || g.ToPool != 5 || g.MigrationAfter.Level != 2 || g.MigrationAfter.Experience != 15 {
		t.Fatal(out, g, err)
	}
}

// TestExperienceRejectsInvalidStateWithoutPartialChange 核对「无效状态仍改变成长」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestExperienceRejectsInvalidStateWithoutPartialChange(t *testing.T) {
	// tc 接收遍历临时结构列表时当前的元素。
	for _, tc := range []struct {
		// name 记录名称，使用文本表示。
		name string
		// change 记录变更，使用供当前流程调用的函数表示。
		change func(*Role, *GrowthContract, *int64)
	}{
		// raw 记录原始值，使用数值表示。
		{"负奖励", func(_ *Role, _ *GrowthContract, raw *int64) { *raw = -1 }},
		// r 记录角色。
		{"负余额", func(r *Role, _ *GrowthContract, _ *int64) { r.Experience = -1 }},
		// r 记录角色。
		{"负池", func(r *Role, _ *GrowthContract, _ *int64) { r.PendingExperience = -1 }},
		// r 记录角色。
		{"负封存", func(r *Role, _ *GrowthContract, _ *int64) { r.LegacyExperienceOverflow = -1 }},
		// r 记录角色。
		{"未知格式", func(r *Role, _ *GrowthContract, _ *int64) { r.ProgressionFormatVersion = 9 }},
		// r 记录角色。
		{"旧格式伪转生", func(r *Role, _ *GrowthContract, _ *int64) {
			r.ProgressionFormatVersion = 0
			r.ProgressionCurveVersion = ""
			r.RebirthCount = 1
		}},
		// r 记录角色。
		{"旧格式伪池", func(r *Role, _ *GrowthContract, _ *int64) {
			r.ProgressionFormatVersion = 0
			r.ProgressionCurveVersion = ""
			r.PendingExperience = 1
		}},
		// r 记录角色。
		{"新格式越级余额", func(r *Role, _ *GrowthContract, _ *int64) { r.Experience = 10 }},
		// r 记录角色。
		{"非cap私带池", func(r *Role, _ *GrowthContract, _ *int64) { r.PendingExperience = 1 }},
		// r 记录角色。
		{"满级活动经验", func(r *Role, _ *GrowthContract, _ *int64) { r.Level = 3; r.Experience = 1 }},
		// r 记录角色。
		{"池超限", func(r *Role, _ *GrowthContract, _ *int64) { r.Level = 3; r.PendingExperience = 31 }},
		// c 记录成长规则合同。
		{"缺阈值", func(_ *Role, c *GrowthContract, _ *int64) { c.ExpPerLevel = []int64{10, 20} }},
		// c 记录成长规则合同。
		{"零阈值", func(_ *Role, c *GrowthContract, _ *int64) { c.ExpPerLevel[1] = 0 }},
		// c 记录成长规则合同。
		{"无版本", func(_ *Role, c *GrowthContract, _ *int64) { c.CurveVersion = "" }},
		// r 记录角色，c 记录成长规则合同。
		{"cap乘法越界", func(r *Role, c *GrowthContract, _ *int64) {
			r.RebirthCount = math.MaxUint32
			c.RebirthLevelIncrement = math.MaxUint32
		}},
	} {
		// t 提供断言失败报告、子用例和清理注册。
		t.Run(tc.name, func(t *testing.T) {
			// r 接收成长角色供后续步骤使用，c 接收成长规则合同供后续步骤使用，raw 保存由整数“64”取得的原始值。
			r, c, raw := growthRole(), growthContract(), int64(5)
			tc.change(&r, &c, &raw)
			// before 保存当前处理所用的操作前的状态。
			before := r
			// out 保存由应用经验取得的输出，err 接收应用经验过程中可能产生的错误，供后续失败分支处理。
			out, _, err := ApplyExperience(r, raw, c)
			if err == nil || !reflect.DeepEqual(out, before) {
				t.Fatal("无效状态仍改变成长", out, err)
			}
		})
	}
}
