// 本文件定义炼体的逐级费用、概率、独立生命与魔法上限，以及战斗属性的来源额度。
package devconfig

import (
	"errors"
	"math"
)

const (
	BodyDamage     uint32 = 1         // 工作簿定义的伤害系。
	BodyAbsorption uint32 = 2         // 工作簿定义的伤害吸收系。
	BodyHealth     uint32 = 3         // 工作簿定义的生命上限系。
	BodyMana       uint32 = 4         // 工作簿定义的魔法上限系。
	BodyChancePPM  uint32 = 1_000_000 // 逐级概率的统一百万分率。
	vitalityBP     int64  = 10_000    // 上限相对装备基础使用万分比。
)

// BodyTrainingConfig 只有完整档位、增益和上限同时存在时才开放。
type BodyTrainingConfig struct {
	Tracks      []BodyTrainingTrack `json:"tracks"`        // 独立系别及阶段基数。
	Steps       []BodyTrainingStep  `json:"steps"`         // 按原炼体等级排列的连续档位。
	HealthCapBP uint32              `json:"health_cap_bp"` // 独立生命加成相对于装备基础的合计上限。
	ManaCapBP   uint32              `json:"mana_cap_bp"`   // 独立魔法加成相对于装备基础的合计上限。
}

// BodyTrainingTrack 保存一系重复尝试的基数及每级固定增量。
type BodyTrainingTrack struct {
	ID           uint32 `json:"id"`                     // 保留原工作簿编号。
	Name         string `json:"name"`                   // 客户端显示的系别名称。
	GoldBase     int64  `json:"gold_base"`              // 金币基础费用，逐阶段相乘。
	BonusPerRank int64  `json:"bonus_per_rank"`         // 生命、魔法使用点数，伤害、吸收使用万分比。
	ShareCapBP   uint32 `json:"share_cap_bp,omitempty"` // 伤害或吸收该系在统一属性组内的来源上限。
}

// BodyTrainingStep 保存原等级到下一级的一次尝试参数。
type BodyTrainingStep struct {
	From           uint32 `json:"from"`            // 零是尚未炼体。
	CostMultiplier uint32 `json:"cost_multiplier"` // 每十级上调一次。
	SuccessPPM     uint32 `json:"success_ppm"`     // 失败扣费并保持等级。
}

// Track 返回已配置的原系别。
func (c *BodyTrainingConfig) Track(id uint32) (BodyTrainingTrack, bool) {
	if c != nil {
		for _, track := range c.Tracks {
			if track.ID == id {
				return track, true
			}
		}
	}
	return BodyTrainingTrack{}, false
}

// IsBodyTrainingTrack 校验当前支持提交的原工作簿系别编号。
func IsBodyTrainingTrack(id uint32) bool {
	return id >= BodyDamage && id <= BodyMana
}

// validateBodyTraining 逐档核对原公式，拒绝收费与属性预算分离的配置。
func (f *Fixture) validateBodyTraining() error {
	c := f.BodyTraining
	if c == nil {
		return nil
	}
	if f.RuleVersion < 30 || len(c.Tracks) == 0 || len(c.Steps) == 0 ||
		uint64(len(c.Steps)) > uint64(f.Progression.CatalogMaxLevel) ||
		c.HealthCapBP == 0 || c.HealthCapBP > 4000 || c.ManaCapBP == 0 || c.ManaCapBP > uint32(vitalityBP) {
		return errors.New("炼体缺少原属性上限、阶段或适用的成长目录")
	}
	seen := make(map[uint32]bool, len(c.Tracks))
	for _, track := range c.Tracks {
		if !IsBodyTrainingTrack(track.ID) || seen[track.ID] || track.Name == "" ||
			track.GoldBase <= 0 || track.GoldBase > math.MaxInt64/int64(len(c.Steps)/10+1) ||
			track.BonusPerRank <= 0 || track.BonusPerRank > math.MaxInt64/int64(len(c.Steps)) {
			return errors.New("炼体系别、金币费用或属性额度无效")
		}
		if track.ID == BodyDamage || track.ID == BodyAbsorption {
			cap := f.ExpandedHitBudget.trackLimit(track.ID)
			if track.ShareCapBP == 0 || track.ShareCapBP > cap || track.BonusPerRank*int64(len(c.Steps)) > int64(track.ShareCapBP) {
				return errors.New("伤害或吸收炼体必须具备覆盖开放等级的独立来源额度")
			}
		} else if track.ShareCapBP != 0 {
			return errors.New("生命和魔法炼体使用各自的点数上限")
		}
		seen[track.ID] = true
	}
	for from, step := range c.Steps {
		if step.From != uint32(from) || step.CostMultiplier != uint32(from/10+1) ||
			step.SuccessPPM != BodyChancePPM-uint32(from%10)*100_000 {
			return errors.New("炼体档位必须连续并沿用已确认费用和成功率公式")
		}
	}
	return nil
}

// BodyHitBonuses 返回有效等级贡献。目录缩减时保留原档，只计算当前可用等级。
func (f *Fixture) BodyHitBonuses(level, damageRank, absorptionRank uint32) HitBonuses {
	c := f.BodyTraining
	if c == nil || f.ExpandedHitBudget == nil || damageRank == 0 && absorptionRank == 0 {
		return HitBonuses{}
	}
	limit := min(uint32(len(c.Steps)), level)
	return HitBonuses{
		DamageBP:     c.hitTrackBonus(BodyDamage, min(damageRank, limit)),
		AbsorptionBP: c.hitTrackBonus(BodyAbsorption, min(absorptionRank, limit)),
	}
}

// hitTrackBonus 计算指定伤害系或吸收系的来源万分比，缺少该系时为零。
func (c *BodyTrainingConfig) hitTrackBonus(id, rank uint32) uint32 {
	track, _ := c.Track(id)
	return uint32(min(int64(track.ShareCapBP), int64(rank)*track.BonusPerRank))
}

// ApplyBodyTraining 把已经确认的独立属性额度交给生命和魔法统一封顶。
func (f *Fixture) ApplyBodyTraining(base Fighter, healthRank, manaRank uint32) (Fighter, bool) {
	c := f.BodyTraining
	if c == nil || healthRank == 0 && manaRank == 0 {
		return base, true
	}
	health, _ := c.Track(BodyHealth)
	mana, _ := c.Track(BodyMana)
	limit := min(uint32(len(c.Steps)), base.Level) // 暂时关闭高档时保留原次数，效果只取现行目录和当前等级。
	return applyIndependentVitality(base, int64(min(healthRank, limit))*health.BonusPerRank,
		int64(min(manaRank, limit))*mana.BonusPerRank, c.HealthCapBP, c.ManaCapBP)
}

// applyIndependentVitality 按职业加穿戴后的基础计算一次上限。固定点先汇总、再统一取整。
func applyIndependentVitality(base Fighter, hp, mp int64, hpCapBP, mpCapBP uint32) (Fighter, bool) {
	if base.HP <= 0 || base.MP < 0 {
		return base, false
	}
	out := base
	var ok bool
	out.HP, ok = equipmentAdd(base.HP, min(hp, vitalityCeiling(base.HP, hpCapBP)))
	if !ok {
		return base, false
	}
	out.MP, ok = equipmentAdd(base.MP, min(mp, vitalityCeiling(base.MP, mpCapBP)))
	return out, ok
}

// vitalityCeiling 将非负基础的万分比向下取整，避免相乘中间值溢出。
func vitalityCeiling(value int64, basisPoints uint32) int64 {
	rate := int64(basisPoints)
	return (value/vitalityBP)*rate + (value%vitalityBP)*rate/vitalityBP
}
