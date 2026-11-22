// 本文件按照版本化经验曲线管理等级、溢出池、旧档迁移以及溢出校验。

package domain

import (
	"errors"
	"math"
)

// 成长数值由唯一配置传入；此类型不提供转生资格或修改次数的入口。
type GrowthContract struct {
	// FormatVersion 记录格式版本，使用数值表示。
	FormatVersion uint32
	// CurveVersion 记录“curve”版本，使用文本表示。
	CurveVersion string
	// InitialLevelCap 记录初始值等级上限，使用数值表示。
	InitialLevelCap uint32
	// RebirthLevelIncrement 记录“rebirth”等级增量，使用数值表示。
	RebirthLevelIncrement uint32
	// OperatingLevelCap 记录“operating”等级上限，使用数值表示。
	OperatingLevelCap uint32
	// ExpPerLevel 记录经验“per”等级，使用数值列表表示。
	ExpPerLevel []int64
}

// ProgressionSnapshot 保存成长快照所需的状态。
type ProgressionSnapshot struct {
	// FormatVersion 记录格式版本，使用数值表示。
	FormatVersion uint32 `bson:"format_version"`
	// CurveVersion 记录“curve”版本，使用文本表示。
	CurveVersion string `bson:"curve_version"`
	// Level 记录等级，使用数值表示。
	Level uint32 `bson:"level"`
	// Experience 记录经验，使用数值表示。
	Experience int64 `bson:"experience"`
	// RebirthCount 记录“rebirth”数量，使用数值表示。
	RebirthCount uint32 `bson:"rebirth_count"`
	// PendingExperience 记录待确认经验，使用数值表示。
	PendingExperience int64 `bson:"pending_experience"`
	// LegacyExperienceOverflow 记录兼容旧数据经验溢出，使用数值表示。
	LegacyExperienceOverflow int64 `bson:"legacy_experience_overflow"`
	// MigrationRequired 标记是否处于迁移“required”状态。
	MigrationRequired bool `bson:"migration_required"`
}

// ProgressionSnapshot 返回成长快照供调用方使用。
// r 是调用该方法的角色实例。
func (r Role) ProgressionSnapshot() ProgressionSnapshot {
	return ProgressionSnapshot{r.ProgressionFormatVersion, r.ProgressionCurveVersion, r.Level, r.Experience, r.RebirthCount, r.PendingExperience, r.LegacyExperienceOverflow, r.ProgressionMigrationRequired}
}

// 原奖励、实际接收和未接收分列；旧余额迁移绝不算成本次新grant。
type ExperienceGrantReceipt struct {
	// RawExperience 记录原始值经验，使用数值表示。
	RawExperience int64 `bson:"raw_experience"`
	// ActualGranted 记录实测已授予，使用数值表示。
	ActualGranted int64 `bson:"actual_granted"`
	// ToProgress 记录目标进度，使用数值表示。
	ToProgress int64 `bson:"to_progress"`
	// ToPool 记录目标连接或数值池。
	ToPool int64 `bson:"to_pool"`
	// Rejected 记录拒绝，使用数值表示。
	Rejected int64 `bson:"rejected"`
	// LevelBefore 记录等级操作前的状态，使用数值表示。
	LevelBefore uint32 `bson:"level_before"`
	// LevelAfter 记录等级操作后的状态，使用数值表示。
	LevelAfter uint32 `bson:"level_after"`
	// CurveVersion 记录“curve”版本，使用文本表示。
	CurveVersion string `bson:"curve_version"`
	// EffectiveCap 记录有效上限，使用数值表示。
	EffectiveCap uint32 `bson:"effective_cap"`
	// PoolCap 记录连接或数值池上限。
	PoolCap int64 `bson:"pool_cap"`
	// Before 记录操作前的状态，使用成长快照表示。
	Before ProgressionSnapshot `bson:"before"`
	// After 记录操作后的状态，使用成长快照表示。
	After ProgressionSnapshot `bson:"after"`
	// MigrationBefore 记录迁移操作前的状态，使用成长快照表示。
	MigrationBefore *ProgressionSnapshot `bson:"migration_before,omitempty"`
	// MigrationAfter 记录迁移操作后的状态，使用成长快照表示。
	MigrationAfter *ProgressionSnapshot `bson:"migration_after,omitempty"`
}

// Validate 校验当前请求或状态，并把结果交给调用方。
// c 是调用该方法的成长规则合同实例。
func (c GrowthContract) Validate() error {
	if c.FormatVersion == 0 || c.CurveVersion == "" || c.InitialLevelCap == 0 || c.RebirthLevelIncrement == 0 || c.OperatingLevelCap < c.InitialLevelCap || uint64(len(c.ExpPerLevel)) < uint64(c.OperatingLevelCap) {
		return errors.New("成长版本、上限或连续经验配置缺失")
	}
	// n 接收遍历经验“per”等级时当前的元素。
	for _, n := range c.ExpPerLevel {
		if n <= 0 {
			return errors.New("逐级经验必须为正")
		}
	}
	return nil
}

// EffectiveCap 处理有效上限。
// c 是调用该方法的成长规则合同实例，rebirths 记录“rebirths”，使用数值表示。
func (c GrowthContract) EffectiveCap(rebirths uint32) (uint32, error) {
	// err 接收校验过程中可能产生的错误，供后续失败分支处理。
	if err := c.Validate(); err != nil {
		return 0, err
	}
	// cap 保存根据无符号整数“64”与无符号整数“64”与无符号整数“64”的计算结果计算的上限。
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
// r 记录角色，raw 记录原始值，使用数值表示，c 记录成长规则合同。
func ApplyExperience(r Role, raw int64, c GrowthContract) (Role, ExperienceGrantReceipt, error) {
	// before 保存由成长快照取得的操作前的状态。
	before := r.ProgressionSnapshot()
	// grant 初始化用作授予的经验赋予道具或权限回执。
	grant := ExperienceGrantReceipt{RawExperience: raw, Before: before, CurveVersion: c.CurveVersion}
	// cap 保存由有效上限取得的上限，err 接收有效上限过程中可能产生的错误，供后续失败分支处理。
	cap, err := c.EffectiveCap(r.RebirthCount)
	if err != nil {
		return r, grant, err
	}
	grant.EffectiveCap = cap
	grant.PoolCap = c.ExpPerLevel[cap-1]
	if raw < 0 || r.Level == 0 || r.Experience < 0 || r.PendingExperience < 0 || r.LegacyExperienceOverflow < 0 || r.ProgressionFormatVersion > c.FormatVersion {
		return r, grant, errors.New("角色成长事实无效")
	}
	// out 保存当前处理所用的输出。
	out := r
	if out.ProgressionFormatVersion == 0 {
		if out.RebirthCount != 0 || out.PendingExperience != 0 || out.LegacyExperienceOverflow != 0 || out.ProgressionMigrationRequired || out.ProgressionCurveVersion != "" {
			return r, grant, errors.New("历史格式不能预置新成长资格")
		}
		// migrationBefore 保存当前处理所用的迁移操作前的状态。
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
		// migrationAfter 保存由成长快照取得的迁移操作后的状态。
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
	// remaining 保存当前处理所用的剩余额度。
	remaining := raw
	for remaining > 0 && out.Level < cap {
		// required 保存从经验“per”等级取出的“required”。
		required := c.ExpPerLevel[out.Level-1]
		// take 保存由最小取得的取出。
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
