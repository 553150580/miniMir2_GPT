package domain

import (
	"math"
	"reflect"
	"testing"
)

func growthContract() GrowthContract {
	return GrowthContract{FormatVersion: 1, CurveVersion: "explicit-test-curve", InitialLevelCap: 3, RebirthLevelIncrement: 1, OperatingLevelCap: 3, ExpPerLevel: []int64{10, 20, 30}}
}
func growthRole() Role {
	return Role{Level: 1, ProgressionFormatVersion: 1, ProgressionCurveVersion: growthContract().CurveVersion, Revision: 9, EconomicSeq: 4, SourceSeq: 3, Wallet: Wallet{BoundGold: 7}}
}

func TestExperienceGrantConsumesMultipleLevelsThenBoundedPool(t *testing.T) {
	r := growthRole()
	out, g, err := ApplyExperience(r, 75, growthContract())
	if err != nil || out.Level != 3 || out.Experience != 0 || out.PendingExperience != 30 || g.RawExperience != 75 || g.ActualGranted != 60 || g.ToProgress != 30 || g.ToPool != 30 || g.Rejected != 15 || g.LevelBefore != 1 || g.LevelAfter != 3 {
		t.Fatal(out, g, err)
	}
	want := r
	want.Level = 3
	want.PendingExperience = 30
	if !reflect.DeepEqual(out, want) {
		t.Fatal("纯成长修改了其它永久事实", out, want)
	}
}

func TestExperienceFullPoolRejectsRewardWithoutOverflowOrLegacyGrowth(t *testing.T) {
	r := growthRole()
	r.Level = 3
	r.PendingExperience = 30
	r.LegacyExperienceOverflow = math.MaxInt64
	out, g, err := ApplyExperience(r, math.MaxInt64, growthContract())
	if err != nil || !reflect.DeepEqual(out, r) || g.ActualGranted != 0 || g.Rejected != math.MaxInt64 || g.ToPool != 0 || g.ToProgress != 0 {
		t.Fatal(out, g, err)
	}
}

func TestExperienceNearMaxThresholdUsesHeadroomWithoutOverflow(t *testing.T) {
	r := growthRole()
	r.Experience = math.MaxInt64 - 1
	c := growthContract()
	c.InitialLevelCap = 2
	c.OperatingLevelCap = 2
	c.ExpPerLevel = []int64{math.MaxInt64, math.MaxInt64}
	out, g, err := ApplyExperience(r, math.MaxInt64, c)
	if err != nil || out.Level != 2 || out.PendingExperience != math.MaxInt64-1 || g.ActualGranted != math.MaxInt64 || g.ToProgress != 1 || g.ToPool != math.MaxInt64-1 || g.Rejected != 0 {
		t.Fatal(out, g, err)
	}
}

func TestLegacyExperienceMigratesWithoutCountingAsNewReward(t *testing.T) {
	r := growthRole()
	r.ProgressionFormatVersion = 0
	r.ProgressionCurveVersion = ""
	r.Experience = 100
	out, g, err := ApplyExperience(r, 5, growthContract())
	if err != nil || out.Level != 3 || out.PendingExperience != 30 || out.LegacyExperienceOverflow != 40 || out.Experience != 0 || out.ProgressionFormatVersion != 1 || g.ActualGranted != 0 || g.Rejected != 5 || g.LevelBefore != 3 || g.LevelAfter != 3 || g.MigrationBefore == nil || g.MigrationBefore.Experience != 100 || g.MigrationAfter.LegacyExperienceOverflow != 40 {
		t.Fatal(out, g, err)
	}
	again, g2, err := ApplyExperience(out, 0, growthContract())
	if err != nil || !reflect.DeepEqual(again, out) || g2.MigrationBefore != nil || g2.MigrationAfter != nil {
		t.Fatal("历史迁移重复消费", again, g2, err)
	}
}

func TestLegacyLevelAboveOperatingCapPreservesLevelAndSealsBalance(t *testing.T) {
	r := growthRole()
	r.Level = 99
	r.ProgressionFormatVersion = 0
	r.ProgressionCurveVersion = ""
	r.Experience = math.MaxInt64
	out, g, err := ApplyExperience(r, 15, growthContract())
	if err != nil || out.Level != 99 || out.RebirthCount != 0 || !out.ProgressionMigrationRequired || out.LegacyExperienceOverflow != math.MaxInt64 || out.PendingExperience != 0 || out.Experience != 0 || g.ActualGranted != 0 || g.Rejected != 15 {
		t.Fatal(out, g, err)
	}
	again, g2, err := ApplyExperience(out, 15, growthContract())
	if err != nil || !reflect.DeepEqual(again, out) || g2.Rejected != 15 {
		t.Fatal("未确认迁移角色被降级或推定转生", again, g2, err)
	}
}

func TestLegacyProgressionKeepsCurrentLevelRatherThanReplayingLowerLevels(t *testing.T) {
	r := growthRole()
	r.ProgressionFormatVersion = 0
	r.ProgressionCurveVersion = ""
	r.Level = 2
	r.Experience = 15
	out, g, err := ApplyExperience(r, 10, growthContract())
	if err != nil || out.Level != 3 || out.PendingExperience != 5 || g.LevelBefore != 2 || g.LevelAfter != 3 || g.ActualGranted != 10 || g.ToProgress != 5 || g.ToPool != 5 || g.MigrationAfter.Level != 2 || g.MigrationAfter.Experience != 15 {
		t.Fatal(out, g, err)
	}
}

func TestExperienceRejectsInvalidStateWithoutPartialChange(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*Role, *GrowthContract, *int64)
	}{
		{"负奖励", func(_ *Role, _ *GrowthContract, raw *int64) { *raw = -1 }},
		{"负余额", func(r *Role, _ *GrowthContract, _ *int64) { r.Experience = -1 }},
		{"负池", func(r *Role, _ *GrowthContract, _ *int64) { r.PendingExperience = -1 }},
		{"负封存", func(r *Role, _ *GrowthContract, _ *int64) { r.LegacyExperienceOverflow = -1 }},
		{"未知格式", func(r *Role, _ *GrowthContract, _ *int64) { r.ProgressionFormatVersion = 9 }},
		{"旧格式伪转生", func(r *Role, _ *GrowthContract, _ *int64) {
			r.ProgressionFormatVersion = 0
			r.ProgressionCurveVersion = ""
			r.RebirthCount = 1
		}},
		{"旧格式伪池", func(r *Role, _ *GrowthContract, _ *int64) {
			r.ProgressionFormatVersion = 0
			r.ProgressionCurveVersion = ""
			r.PendingExperience = 1
		}},
		{"新格式越级余额", func(r *Role, _ *GrowthContract, _ *int64) { r.Experience = 10 }},
		{"非cap私带池", func(r *Role, _ *GrowthContract, _ *int64) { r.PendingExperience = 1 }},
		{"满级活动经验", func(r *Role, _ *GrowthContract, _ *int64) { r.Level = 3; r.Experience = 1 }},
		{"池超限", func(r *Role, _ *GrowthContract, _ *int64) { r.Level = 3; r.PendingExperience = 31 }},
		{"缺阈值", func(_ *Role, c *GrowthContract, _ *int64) { c.ExpPerLevel = []int64{10, 20} }},
		{"零阈值", func(_ *Role, c *GrowthContract, _ *int64) { c.ExpPerLevel[1] = 0 }},
		{"无版本", func(_ *Role, c *GrowthContract, _ *int64) { c.CurveVersion = "" }},
		{"cap乘法越界", func(r *Role, c *GrowthContract, _ *int64) {
			r.RebirthCount = math.MaxUint32
			c.RebirthLevelIncrement = math.MaxUint32
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r, c, raw := growthRole(), growthContract(), int64(5)
			tc.change(&r, &c, &raw)
			before := r
			out, _, err := ApplyExperience(r, raw, c)
			if err == nil || !reflect.DeepEqual(out, before) {
				t.Fatal("无效状态仍改变成长", out, err)
			}
		})
	}
}
