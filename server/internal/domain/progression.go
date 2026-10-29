package domain

import (
	"errors"
	"math"
)

// 成长数值由唯一配置传入；此类型不提供转生资格或修改次数的入口。
type GrowthContract struct {
	FormatVersion         uint32
	CurveVersion          string
	InitialLevelCap       uint32
	RebirthLevelIncrement uint32
	OperatingLevelCap     uint32
	ExpPerLevel           []int64
}

type ProgressionSnapshot struct {
	FormatVersion            uint32 `bson:"format_version"`
	CurveVersion             string `bson:"curve_version"`
	Level                    uint32 `bson:"level"`
	Experience               int64  `bson:"experience"`
	RebirthCount             uint32 `bson:"rebirth_count"`
	PendingExperience        int64  `bson:"pending_experience"`
	LegacyExperienceOverflow int64  `bson:"legacy_experience_overflow"`
	MigrationRequired        bool   `bson:"migration_required"`
}

func (r Role) ProgressionSnapshot() ProgressionSnapshot {
	return ProgressionSnapshot{r.ProgressionFormatVersion, r.ProgressionCurveVersion, r.Level, r.Experience, r.RebirthCount, r.PendingExperience, r.LegacyExperienceOverflow, r.ProgressionMigrationRequired}
}

// 原奖励、实际接收和未接收分列；旧余额迁移绝不算成本次新grant。
type ExperienceGrantReceipt struct {
	RawExperience   int64                `bson:"raw_experience"`
	ActualGranted   int64                `bson:"actual_granted"`
	ToProgress      int64                `bson:"to_progress"`
	ToPool          int64                `bson:"to_pool"`
	Rejected        int64                `bson:"rejected"`
	LevelBefore     uint32               `bson:"level_before"`
	LevelAfter      uint32               `bson:"level_after"`
	CurveVersion    string               `bson:"curve_version"`
	EffectiveCap    uint32               `bson:"effective_cap"`
	PoolCap         int64                `bson:"pool_cap"`
	Before          ProgressionSnapshot  `bson:"before"`
	After           ProgressionSnapshot  `bson:"after"`
	MigrationBefore *ProgressionSnapshot `bson:"migration_before,omitempty"`
	MigrationAfter  *ProgressionSnapshot `bson:"migration_after,omitempty"`
}

func (c GrowthContract) Validate() error {
	if c.FormatVersion == 0 || c.CurveVersion == "" || c.InitialLevelCap == 0 || c.RebirthLevelIncrement == 0 || c.OperatingLevelCap < c.InitialLevelCap || uint64(len(c.ExpPerLevel)) < uint64(c.OperatingLevelCap) {
		return errors.New("成长版本、上限或连续经验配置缺失")
	}
	for _, n := range c.ExpPerLevel {
		if n <= 0 {
			return errors.New("逐级经验必须为正")
		}
	}
	return nil
}

func (c GrowthContract) EffectiveCap(rebirths uint32) (uint32, error) {
	if err := c.Validate(); err != nil {
		return 0, err
	}
	cap := uint64(c.InitialLevelCap) + uint64(rebirths)*uint64(c.RebirthLevelIncrement)
	if cap > math.MaxUint32 {
		return 0, errors.New("转生等级上限溢出")
	}
	if cap > uint64(c.OperatingLevelCap) {
		cap = uint64(c.OperatingLevelCap)
	}
	return uint32(cap), nil
}

// 纯函数不推进Role/Source/Economic修订，不修改角色其它资产或临时现场。
// 分段使用剩余容量，避免在池已满时先把原余额与新奖励相加而溢出。
func ApplyExperience(r Role, raw int64, c GrowthContract) (Role, ExperienceGrantReceipt, error) {
	before := r.ProgressionSnapshot()
	grant := ExperienceGrantReceipt{RawExperience: raw, Before: before, CurveVersion: c.CurveVersion}
	cap, err := c.EffectiveCap(r.RebirthCount)
	if err != nil {
		return r, grant, err
	}
	grant.EffectiveCap = cap
	grant.PoolCap = c.ExpPerLevel[cap-1]
	if raw < 0 || r.Level == 0 || r.Experience < 0 || r.PendingExperience < 0 || r.LegacyExperienceOverflow < 0 || r.ProgressionFormatVersion > c.FormatVersion {
		return r, grant, errors.New("角色成长事实无效")
	}
	out := r
	if out.ProgressionFormatVersion == 0 {
		if out.RebirthCount != 0 || out.PendingExperience != 0 || out.LegacyExperienceOverflow != 0 || out.ProgressionMigrationRequired || out.ProgressionCurveVersion != "" {
			return r, grant, errors.New("历史格式不能预置新成长资格")
		}
		migrationBefore := before
		grant.MigrationBefore = &migrationBefore
		out.ProgressionFormatVersion = c.FormatVersion
		out.ProgressionCurveVersion = c.CurveVersion
		if out.Level > cap || uint64(out.Level) > uint64(len(c.ExpPerLevel)) {
			out.LegacyExperienceOverflow = out.Experience
			out.Experience = 0
			out.ProgressionMigrationRequired = true
		} else {
			for out.Level < cap && out.Experience >= c.ExpPerLevel[out.Level-1] {
				out.Experience -= c.ExpPerLevel[out.Level-1]
				out.Level++
			}
			if out.Level == cap {
				out.PendingExperience = min(out.Experience, grant.PoolCap)
				out.LegacyExperienceOverflow = out.Experience - out.PendingExperience
				out.Experience = 0
			}
		}
		migrationAfter := out.ProgressionSnapshot()
		grant.MigrationAfter = &migrationAfter
	} else if out.ProgressionFormatVersion != c.FormatVersion {
		return r, grant, errors.New("不支持未登记的成长格式迁移")
	}
	grant.LevelBefore = out.Level
	if out.ProgressionCurveVersion != c.CurveVersion {
		out.ProgressionMigrationRequired = true
	}
	if out.ProgressionMigrationRequired {
		grant.Rejected = raw
		grant.LevelAfter = out.Level
		grant.After = out.ProgressionSnapshot()
		return out, grant, nil
	}
	if out.Level > cap || uint64(out.Level) > uint64(len(c.ExpPerLevel)) || out.Level < cap && (out.PendingExperience != 0 || out.Experience >= c.ExpPerLevel[out.Level-1]) || out.Level == cap && (out.Experience != 0 || out.PendingExperience > grant.PoolCap) {
		return r, grant, errors.New("角色经验余额超出已确认成长格式")
	}
	remaining := raw
	for remaining > 0 && out.Level < cap {
		required := c.ExpPerLevel[out.Level-1]
		take := min(remaining, required-out.Experience)
		out.Experience += take
		remaining -= take
		grant.ToProgress += take
		if out.Experience == required {
			out.Experience = 0
			out.Level++
		}
	}
	if remaining > 0 {
		grant.ToPool = min(remaining, grant.PoolCap-out.PendingExperience)
		out.PendingExperience += grant.ToPool
		remaining -= grant.ToPool
	}
	grant.Rejected = remaining
	grant.ActualGranted = raw - remaining
	grant.LevelAfter = out.Level
	grant.After = out.ProgressionSnapshot()
	return out, grant, nil
}
