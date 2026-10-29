package devconfig

import (
	"errors"
	"math"
)

var equipmentStatFields = [...]string{"magic_min_bonus", "magic_max_bonus", "tao_min_bonus", "tao_max_bonus", "equipment_stage"}

// 显式零仍是来源事实；旧规则只允许新字段缺失或中性零。
func (i Item) EquipmentStatsKnown() bool {
	for _, present := range i.equipmentStatsExplicit {
		if !present {
			return false
		}
	}
	return i.MagicMinBonus >= 0 && i.MagicMaxBonus >= i.MagicMinBonus && i.TaoMinBonus >= 0 && i.TaoMaxBonus >= i.TaoMinBonus
}

func equipmentAdd(a, b int64) (int64, bool) {
	if b > 0 && a > math.MaxInt64-b || b < 0 && a < math.MinInt64-b {
		return 0, false
	}
	return a + b, true
}

func equipmentMultiply(a, b int64) (int64, bool) {
	if a < 0 || b < 0 || b != 0 && a > math.MaxInt64/b {
		return 0, false
	}
	return a * b, true
}

// 只做基础装备加法，不重复放大被动、状态或护符。溢出不能静默变成负属性。
func (i Item) AddEquipmentBonuses(a Fighter) (Fighter, bool) {
	values := []struct {
		target *int64
		bonus  int64
	}{
		{&a.AttackMin, i.AttackBonus}, {&a.AttackMin, i.AttackMinBonus},
		{&a.AttackMax, i.AttackBonus}, {&a.AttackMax, i.AttackMaxBonus},
		{&a.MagicMin, i.MagicMinBonus}, {&a.MagicMax, i.MagicMaxBonus},
		{&a.TaoMin, i.TaoMinBonus}, {&a.TaoMax, i.TaoMaxBonus},
		{&a.AttackSpeed, i.AttackSpeedBonus}, {&a.Defence, i.DefenceBonus},
		{&a.DefenceMin, i.DefenceMinBonus}, {&a.DefenceMax, i.DefenceMaxBonus},
		{&a.MagicDefenceMin, i.MagicDefenceMinBonus}, {&a.MagicDefenceMax, i.MagicDefenceMaxBonus},
	}
	for _, v := range values {
		n, ok := equipmentAdd(*v.target, v.bonus)
		if !ok {
			return Fighter{}, false
		}
		*v.target = n
	}
	for _, pair := range [][2]int64{{a.AttackMin, a.AttackMax}, {a.MagicMin, a.MagicMax}, {a.TaoMin, a.TaoMax}, {a.DefenceMin, a.DefenceMax}, {a.MagicDefenceMin, a.MagicDefenceMax}} {
		if pair[0] < 0 || pair[1] < pair[0] || pair[1]-pair[0] == math.MaxInt64 {
			return Fighter{}, false // interval的闭区间长度还需+1。
		}
	}
	return a, true
}

func sourcePowerBound(rank uint32, stat, power int64) (int64, bool) {
	if rank == math.MaxUint32 || stat < 0 || power < 0 {
		return 0, false
	}
	quarter := float32(float32(stat) / float32(4))
	scaled := float32(quarter * float32(rank+1))
	value := math.RoundToEven(float64(float32(scaled + float32(power))))
	if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 || value >= math.Exp2(63) {
		return 0, false
	}
	return int64(value), true
}

// 这里限制的是整数中间值，不是装备强度或玩法平衡上限。
func (f *Fixture) EquipmentMagicArithmeticSafe(a Fighter) bool {
	for _, s := range f.Skills {
		if s.JobID != a.JobID || s.Behavior == "teleport" || s.Behavior == "summon_pet" {
			continue
		}
		power, ok := equipmentAdd(s.PowerBase, max(int64(0), s.PowerBonus-1))
		if !ok {
			return false
		}
		mpower, ok := equipmentAdd(s.MPowerBase, max(int64(0), s.MPowerBonus-1))
		if !ok {
			return false
		}
		derived, ok := sourcePowerBound(s.MaxLevel, mpower, power)
		if !ok {
			return false
		}
		base := a.MagicMax
		if s.Behavior == "healing" || s.Behavior == "mass_healing" {
			base, ok = equipmentMultiply(a.TaoMax, s.HealingStatMultiplier)
			if !ok {
				return false
			}
		}
		if _, ok = equipmentAdd(base, derived); !ok {
			return false
		}
		b, status := f.Buff(s.BuffID)
		if !status {
			continue
		}
		switch b.DurationKind {
		case "mc_source_power":
			stat, valid := equipmentAdd(a.MagicMax, b.StatAdd)
			if !valid {
				return false
			}
			if _, valid = sourcePowerBound(s.MaxLevel, stat, power); !valid {
				return false
			}
		case "sc_rank_linear":
			seconds, valid := equipmentMultiply(a.TaoMax, b.StatMultiplier)
			if !valid {
				return false
			}
			for _, extra := range b.DurationSecondsByLevel {
				if _, valid = equipmentAdd(seconds, extra); !valid {
					return false
				}
			}
		}
	}
	return true
}

func (f *Fixture) EquipmentArithmeticSafe(a Fighter) bool {
	if !f.EquipmentMagicArithmeticSafe(a) {
		return false
	}
	speed, ok := equipmentMultiply(a.AttackSpeed, f.CombatTiming.PlayerAttackSpeedStepMS)
	if !ok {
		return false
	}
	reduction, ok := equipmentMultiply(int64(a.Level), f.CombatTiming.PlayerAttackLevelStepMS)
	if !ok {
		return false
	}
	reduction = min(reduction, f.CombatTiming.PlayerAttackLevelCapMS)
	clock, ok := equipmentAdd(f.CombatTiming.PlayerAttackBaseMS, -speed)
	if !ok {
		return false
	}
	_, ok = equipmentAdd(clock, -reduction)
	return ok
}

func (f *Fixture) ValidateEquipmentStats() error {
	for _, i := range f.Items {
		if f.RuleVersion < 22 {
			if i.MagicMinBonus != 0 || i.MagicMaxBonus != 0 || i.TaoMinBonus != 0 || i.TaoMaxBonus != 0 || i.EquipmentStage != 0 {
				return errors.New("旧规则不能携带非中性装备MC/SC或阶段")
			}
			continue
		}
		if !i.IsEquipment {
			if i.MagicMinBonus != 0 || i.MagicMaxBonus != 0 || i.TaoMinBonus != 0 || i.TaoMaxBonus != 0 || i.EquipmentStage != 0 {
				return errors.New("非装备不能携带装备MC/SC或阶段")
			}
			continue
		}
		if !i.EquipmentStatsKnown() || i.RequiredLevel == nil || *i.RequiredLevel < 1 || i.EquipmentStage != (*i.RequiredLevel-1)/10 || i.EquipmentSlot > 9 {
			return errors.New("装备须显式给出合法MC/SC区间及与需求等级一致的阶段")
		}
	}
	if f.RuleVersion < 22 {
		return nil
	}
	// 每个槽分别取各端点最坏值，覆盖所有本次可穿组合；不按当前两件目录写死总和。
	for _, player := range f.Players {
		level := max(uint32(1), f.Progression.CatalogMaxLevel)
		a, _ := f.PlayerAtLevel(player.JobID, level)
		for slot := uint32(0); slot < 10; slot++ {
			var worst Item
			for _, i := range f.Items {
				if !i.IsEquipment || i.EquipmentSlot != slot {
					continue
				}
				allowed := len(i.AllowedJobs) == 0
				for _, job := range i.AllowedJobs {
					allowed = allowed || job == player.JobID
				}
				if !allowed {
					continue
				}
				worst.AttackBonus = max(worst.AttackBonus, i.AttackBonus)
				worst.AttackMinBonus = max(worst.AttackMinBonus, i.AttackMinBonus)
				worst.AttackMaxBonus = max(worst.AttackMaxBonus, i.AttackMaxBonus)
				worst.MagicMinBonus = max(worst.MagicMinBonus, i.MagicMinBonus)
				worst.MagicMaxBonus = max(worst.MagicMaxBonus, i.MagicMaxBonus)
				worst.TaoMinBonus = max(worst.TaoMinBonus, i.TaoMinBonus)
				worst.TaoMaxBonus = max(worst.TaoMaxBonus, i.TaoMaxBonus)
				worst.AttackSpeedBonus = max(worst.AttackSpeedBonus, i.AttackSpeedBonus)
				worst.DefenceBonus = max(worst.DefenceBonus, i.DefenceBonus)
				worst.DefenceMinBonus = max(worst.DefenceMinBonus, i.DefenceMinBonus)
				worst.DefenceMaxBonus = max(worst.DefenceMaxBonus, i.DefenceMaxBonus)
				worst.MagicDefenceMinBonus = max(worst.MagicDefenceMinBonus, i.MagicDefenceMinBonus)
				worst.MagicDefenceMaxBonus = max(worst.MagicDefenceMaxBonus, i.MagicDefenceMaxBonus)
			}
			var ok bool
			a, ok = worst.AddEquipmentBonuses(a)
			if !ok {
				return errors.New("装备组合基础属性或随机区间溢出")
			}
		}
		if !f.EquipmentArithmeticSafe(a) {
			return errors.New("装备组合的MC/SC技能中间值溢出")
		}
	}
	return nil
}
