// 本文件定义并校验装备加成、基础装备数据和全套属性叠加的算术上界。

package devconfig

import (
	"errors"
	"math"
)

// equipmentStatFields 初始化用作装备属性字段集合的文本列表。
var equipmentStatFields = [...]string{"magic_min_bonus", "magic_max_bonus", "tao_min_bonus", "tao_max_bonus", "equipment_stage"}

// 显式零仍是来源事实；旧规则只允许新字段缺失或中性零。
// i 是调用该方法的道具实例。
func (i Item) EquipmentStatsKnown() bool {
	// present 接收遍历装备统计或属性显式时当前的元素。
	for _, present := range i.equipmentStatsExplicit {
		if !present {
			return false
		}
	}
	return i.MagicMinBonus >= 0 && i.MagicMaxBonus >= i.MagicMinBonus && i.TaoMinBonus >= 0 && i.TaoMaxBonus >= i.TaoMinBonus
}

// equipmentAdd 处理装备新增。
// a 记录a，使用数值表示，b 记录b，使用数值表示。
func equipmentAdd(a, b int64) (int64, bool) {
	if b > 0 && a > math.MaxInt64-b || b < 0 && a < math.MinInt64-b {
		return 0, false
	}
	return a + b, true
}

// equipmentMultiply 处理装备相乘。
// a 记录a，使用数值表示，b 记录b，使用数值表示。
func equipmentMultiply(a, b int64) (int64, bool) {
	if a < 0 || b < 0 || b != 0 && a > math.MaxInt64/b {
		return 0, false
	}
	return a * b, true
}

// 只做基础装备加法，不重复放大被动、状态或护符。溢出不能静默变成负属性。
// i 是调用该方法的道具实例，a 记录a，使用战斗单位表示。
func (i Item) AddEquipmentBonuses(a Fighter) (Fighter, bool) {
	// 原固定攻击对区间两端生效，仍按历史顺序先加固定攻击再加基础区间。
	minimum, minimumOK := equipmentAdd(a.AttackMin, i.AttackBonus)
	maximum, maximumOK := equipmentAdd(a.AttackMax, i.AttackBonus)
	if !minimumOK || !maximumOK {
		return Fighter{}, false
	}
	a.AttackMin, a.AttackMax = minimum, maximum
	return applyEquipmentStats(a, i.equipmentStats())
}

// sourcePowerBound 处理来源能力值绑定。
// rank 记录排序位次，使用数值表示，stat 记录属性，使用数值表示，power 记录能力值，使用数值表示。
func sourcePowerBound(rank uint32, stat, power int64) (int64, bool) {
	if rank == math.MaxUint32 || stat < 0 || power < 0 {
		return 0, false
	}
	// quarter 保存由“float”“32”取得的“quarter”。
	quarter := float32(float32(stat) / float32(4))
	// scaled 保存由“float”“32”取得的“scaled”。
	scaled := float32(quarter * float32(rank+1))
	// value 保存由“round”目标“even”取得的值。
	value := math.RoundToEven(float64(float32(scaled + float32(power))))
	if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 || value >= math.Exp2(63) {
		return 0, false
	}
	return int64(value), true
}

// 这里限制的是整数中间值，不是装备强度或玩法平衡上限。
// f 是调用该方法的开发测试配置实例，a 记录a，使用战斗单位表示。
func (f *Fixture) EquipmentMagicArithmeticSafe(a Fighter) bool {
	// s 接收遍历技能集合时当前的元素。
	for _, s := range f.Skills {
		if s.JobID != a.JobID || s.Behavior == "teleport" || s.Behavior == "summon_pet" {
			continue
		}
		// power 保存由装备新增取得的能力值，ok 表明查询、校验或类型转换是否成功。
		power, ok := equipmentAdd(s.PowerBase, max(int64(0), s.PowerBonus-1))
		if !ok {
			return false
		}
		// mpower 保存由装备新增取得的“mpower”。
		mpower, ok := equipmentAdd(s.MPowerBase, max(int64(0), s.MPowerBonus-1))
		if !ok {
			return false
		}
		// derived 保存由来源能力值绑定取得的“derived”。
		derived, ok := sourcePowerBound(s.MaxLevel, mpower, power)
		if !ok {
			return false
		}
		// base 保存魔法最大，用于当前步骤中的基础值。
		base := a.MagicMax
		if s.Behavior == "soul_fire_ball" || s.Behavior == "poisoning" || s.Behavior == "revelation" {
			base = a.TaoMax
		}
		if s.Behavior == "healing" || s.Behavior == "mass_healing" {
			base, ok = equipmentMultiply(a.TaoMax, s.HealingStatMultiplier)
			if !ok {
				return false
			}
		}
		if _, ok = equipmentAdd(base, derived); !ok {
			return false
		}
		// b 保存状态效果返回的结果，status 保存由状态效果取得的状态。
		b, status := f.Buff(s.BuffID)
		if !status {
			continue
		}
		switch b.DurationKind {
		case "mc_source_power":
			// stat 保存由装备新增取得的属性，valid 保存由装备新增取得的有效。
			stat, valid := equipmentAdd(a.MagicMax, b.StatAdd)
			if !valid {
				return false
			}
			if _, valid = sourcePowerBound(s.MaxLevel, stat, power); !valid {
				return false
			}
		case "sc_rank_linear", "sc_half_rank_linear":
			// 集体隐身先取半道术，再拼接每阶的固定时长。
			tao := a.TaoMax
			if b.DurationKind == "sc_half_rank_linear" {
				tao /= 2
			}
			// seconds 保存由装备相乘取得的秒集合，valid 保存由装备相乘取得的有效。
			seconds, valid := equipmentMultiply(tao, b.StatMultiplier)
			if !valid {
				return false
			}
			// extra 接收遍历持续时间秒集合“by”等级时当前的元素。
			for _, extra := range b.DurationSecondsByLevel {
				if _, valid = equipmentAdd(seconds, extra); !valid {
					return false
				}
			}
		}
	}
	return true
}

// EquipmentArithmeticSafe 判断装备“arithmetic”安全是否满足要求。
// f 是调用该方法的开发测试配置实例，a 记录a，使用战斗单位表示。
func (f *Fixture) EquipmentArithmeticSafe(a Fighter) bool {
	if !f.EquipmentMagicArithmeticSafe(a) {
		return false
	}
	// speed 保存由装备相乘取得的速度，ok 表明查询、校验或类型转换是否成功。
	speed, ok := equipmentMultiply(a.AttackSpeed, f.CombatTiming.PlayerAttackSpeedStepMS)
	if !ok {
		return false
	}
	// reduction 保存由装备相乘取得的“reduction”。
	reduction, ok := equipmentMultiply(int64(a.Level), f.CombatTiming.PlayerAttackLevelStepMS)
	if !ok {
		return false
	}
	reduction = min(reduction, f.CombatTiming.PlayerAttackLevelCapMS)
	// clock 保存由装备新增取得的时钟。
	clock, ok := equipmentAdd(f.CombatTiming.PlayerAttackBaseMS, -speed)
	if !ok {
		return false
	}
	_, ok = equipmentAdd(clock, -reduction)
	return ok
}

// ValidateEquipmentStats 校验装备统计或属性。
// f 是调用该方法的开发测试配置实例。
func (f *Fixture) ValidateEquipmentStats() error {
	// i 接收遍历道具集合时当前的元素。
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
	var maxEnhancementBP uint32
	if f.EquipmentEnhancement != nil {
		maxEnhancementBP = f.EquipmentEnhancement.BonusBP(maxEnhancementLevel)
	}
	// player 接收遍历玩家集合时当前的元素。
	for _, player := range f.Players {
		// level 保存由最大取得的等级。
		level := max(uint32(1), f.Progression.CatalogMaxLevel)
		// a 保存由玩家发生时间等级取得的a。
		a, _ := f.PlayerAtLevel(player.JobID, level)
		var strengthened EquipmentContributions // 汇集当前职业所有强化部位的最坏基础贡献。
		// slot 保存由无符号整数“32”取得的槽位。
		for slot := uint32(0); slot < equipmentSlotCount; slot++ {
			// worst 记录“worst”，使用道具表示。
			var worst Item
			// i 接收遍历道具集合时当前的元素。
			for _, i := range f.Items {
				if !i.IsEquipment || i.EquipmentSlot != slot {
					continue
				}
				// allowed 保存根据“len”与当前数据计算的允许。
				allowed := len(i.AllowedJobs) == 0
				// job 接收遍历允许任务或职业编号集合时当前的元素。
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
			if maxEnhancementBP != 0 {
				if !strengthened.Add(worst, maxEnhancementBP) {
					return errors.New("装备组合的部位强化定点中间值溢出")
				}
				continue
			}
			// ok 表明查询、校验或类型转换是否成功。
			var ok bool
			a, ok = worst.AddEquipmentBonuses(a)
			if !ok {
				return errors.New("装备组合基础属性或随机区间溢出")
			}
		}
		if maxEnhancementBP != 0 {
			var valid bool
			a, valid = strengthened.Apply(a)
			if !valid {
				return errors.New("装备组合的部位强化属性或随机区间溢出")
			}
		}
		if f.Medal != nil {
			var valid bool
			a, valid = f.ApplyMedalBonuses(a, uint32(len(f.Medal.Levels)))
			if !valid {
				return errors.New("装备组合与最高勋章叠加后的属性或随机区间溢出")
			}
		}
		if !f.EquipmentArithmeticSafe(a) {
			return errors.New("装备组合的MC/SC技能中间值溢出")
		}
	}
	return nil
}
