// 本文件装配来源攻击、防御取值与技能伤害公式，并校验各职业战斗参数。

package devconfig

import (
	"errors"
	"math"
)

// supportedSkill 判断“supported”技能是否满足要求。
// behavior 记录行为，使用文本表示。
func supportedSkill(behavior string) bool {
	switch behavior {
	case "fencing", "spirit_sword", "fireball", "great_fireball", "thunderbolt", "firebang", "fire_wall", "healing", "mass_healing",
		"slaying", "thrusting", "half_moon", "flaming_sword", "hell_fire", "lightning", "ice_storm", "thunder_storm", "magic_shield", "soul_shield", "blessed_armour", "repulsion", "shoulder_dash", "teleport", "summon_pet", "turn_undead":
		return true
	}
	return false
}

// Activation 返回“activation”供调用方使用。
// s 是调用该方法的技能实例。
func (s Skill) Activation() string {
	if s.ActivationKind != "" {
		return s.ActivationKind
	}
	// 旧开发夹具兼容；新快照明确输出激活方式。
	switch s.Behavior {
	case "fencing", "spirit_sword":
		return "passive"
	case "slaying":
		return "passive_proc"
	case "thrusting", "half_moon":
		return "toggle"
	case "flaming_sword":
		return "charge"
	}
	return "cast"
}

// MaximumAttackBonus 返回最大值攻击加成供调用方使用。
// s 是调用该方法的技能实例，level 记录等级，使用数值表示。
func (s Skill) MaximumAttackBonus(level uint32) int64 {
	if int(level) < len(s.MaxAttackBonusByLevel) {
		return s.MaxAttackBonusByLevel[level]
	}
	return 0
}

// CombatTiming 保存战斗时序所需的状态。
type CombatTiming struct {
	// PlayerAttackBaseMS 记录玩家攻击基础值毫秒，使用数值表示。
	PlayerAttackBaseMS int64 `json:"player_attack_base_ms"`
	// PlayerAttackSpeedStepMS 记录玩家攻击速度步骤毫秒，使用数值表示。
	PlayerAttackSpeedStepMS int64 `json:"player_attack_speed_step_ms"`
	// PlayerAttackLevelStepMS 记录玩家攻击等级步骤毫秒，使用数值表示。
	PlayerAttackLevelStepMS int64 `json:"player_attack_level_step_ms"`
	// PlayerAttackLevelCapMS 记录玩家攻击等级上限毫秒，使用数值表示。
	PlayerAttackLevelCapMS int64 `json:"player_attack_level_cap_ms"`
	// PlayerAttackMinMS 记录玩家攻击最小毫秒，使用数值表示。
	PlayerAttackMinMS int64 `json:"player_attack_min_ms"`
	// PlayerAttackActionMS 记录玩家攻击动作毫秒，使用数值表示。
	PlayerAttackActionMS int64 `json:"player_attack_action_ms"`
	// PlayerAttackImpactMS 记录玩家攻击命中影响毫秒，使用数值表示。
	PlayerAttackImpactMS int64 `json:"player_attack_impact_ms"`
	// PlayerPushActionMS 记录玩家推入动作毫秒，使用数值表示。
	PlayerPushActionMS int64 `json:"player_push_action_ms"`
	// MonsterPushActionMSPerStep 记录怪物推入动作毫秒“per”步骤，使用数值表示。
	MonsterPushActionMSPerStep int64 `json:"monster_push_action_ms_per_step"`
	// MonsterPushMoveMSPerStep 记录怪物推入移动毫秒“per”步骤，使用数值表示。
	MonsterPushMoveMSPerStep int64 `json:"monster_push_move_ms_per_step"`
	// PushedStepDisplayMS 记录被推动位移步骤显示毫秒，使用数值表示。
	PushedStepDisplayMS int64 `json:"pushed_step_display_ms"`
	// EventCapacity 记录事件容量，使用数值表示。
	EventCapacity int `json:"event_capacity"`
	// KillQueueCapacity 记录击杀队列容量，使用数值表示。
	KillQueueCapacity int `json:"kill_queue_capacity"`
}

// AccuracyBonus 返回“accuracy”加成供调用方使用。
// s 是调用该方法的技能实例，level 记录等级，使用数值表示。
func (s Skill) AccuracyBonus(level uint32) int64 {
	if int(level) < len(s.AccuracyByLevel) {
		return s.AccuracyByLevel[level]
	}
	return int64(level) * s.AccuracyPerLevel
}

// PlayerAttackInterval 返回玩家攻击周期供调用方使用。
// t 是调用该方法的战斗时序实例，level 记录等级，使用数值表示，speed 记录速度，使用数值表示。
func (t CombatTiming) PlayerAttackInterval(level uint32, speed int64) int64 {
	// reduction 保存根据整数“64”与玩家攻击等级步骤毫秒计算的“reduction”。
	reduction := int64(level) * t.PlayerAttackLevelStepMS
	if reduction > t.PlayerAttackLevelCapMS {
		reduction = t.PlayerAttackLevelCapMS
	}
	// value 保存根据玩家攻击基础值毫秒与速度与玩家攻击速度步骤毫秒的计算结果的计算结果与“reduction”计算的值。
	value := t.PlayerAttackBaseMS - speed*t.PlayerAttackSpeedStepMS - reduction
	if value < t.PlayerAttackMinMS {
		value = t.PlayerAttackMinMS
	}
	if value > math.MaxInt32 {
		value = math.MaxInt32
	}
	return value
}

// ValidateCombat 校验战斗。
// f 是调用该方法的开发测试配置实例。
func (f *Fixture) ValidateCombat() error {
	// t 保存战斗时序，用于当前步骤中的t。
	t := f.CombatTiming
	// v 接收遍历数值列表时当前的元素。
	for _, v := range []int64{t.PlayerAttackBaseMS, t.PlayerAttackSpeedStepMS, t.PlayerAttackLevelStepMS, t.PlayerAttackLevelCapMS, t.PlayerAttackMinMS, t.PlayerAttackActionMS, t.PlayerAttackImpactMS} {
		if v <= 0 || v > math.MaxInt32 {
			return errors.New("战斗时序参数必须完整且可计时")
		}
	}
	if t.PlayerAttackMinMS > t.PlayerAttackBaseMS || t.EventCapacity < 1 || t.EventCapacity > 4096 || t.KillQueueCapacity < len(f.Map.MonsterSpawns)+1 || t.KillQueueCapacity > 4096 {
		return errors.New("战斗事件或逐杀预算无效")
	}
	// s 接收遍历技能集合时当前的元素。
	for _, s := range f.Skills {
		if s.Behavior == "summon_pet" {
			// err 接收校验召唤技能过程中可能产生的错误，供后续失败分支处理。
			if err := validateSummonSkill(s); err != nil {
				return err
			}
			continue // 召唤不判准确或伤害，不能为满足其它技能路径而伪造参数。
		}
		if len(s.AccuracyByLevel) != int(s.MaxLevel)+1 {
			return errors.New("逐阶准确数组不完整")
		}
		// v 接收遍历“accuracy”“by”等级时当前的元素。
		for _, v := range s.AccuracyByLevel {
			if v < 0 || v > math.MaxInt32 {
				return errors.New("逐阶准确越界")
			}
		}
		if s.AreaRadius < 0 || s.AreaRadius >= f.Map.Width || s.HealingStatMultiplier < 0 || s.HealingStatMultiplier > math.MaxInt32 || s.UndeadMultiplierPPM <= 0 || s.UndeadMultiplierPPM > math.MaxInt32 {
			return errors.New("技能范围或数值路径无效")
		}
		switch s.TargetKind {
		case "passive", "enemy_entity", "enemy_grid", "friendly_entity", "friendly_grid", "friendly_self", "attack_mode", "attack_charge", "enemy_direction", "enemy_self":
		default:
			return errors.New("技能目标类型无效")
		}
		// wanted 保存期望状态所使用的文本。
		wanted := "enemy_entity"
		switch s.Behavior {
		case "fencing", "spirit_sword", "slaying":
			wanted = "passive"
		case "thrusting", "half_moon":
			wanted = "attack_mode"
		case "flaming_sword":
			wanted = "attack_charge"
		case "hell_fire", "lightning", "shoulder_dash":
			wanted = "enemy_direction"
		case "thunder_storm", "repulsion":
			wanted = "enemy_self"
		case "healing":
			wanted = "friendly_entity"
		case "mass_healing":
			wanted = "friendly_grid"
		case "soul_shield", "blessed_armour":
			wanted = "friendly_grid"
		case "magic_shield", "teleport":
			wanted = "friendly_self"
		case "firebang", "ice_storm", "fire_wall":
			wanted = "enemy_grid"
		}
		if s.Behavior == "fire_wall" && (s.AreaRadius != 1 || s.AutoEnabled || s.DelayBaseMS != 500 || s.ProjectileMSPerCell != 0) {
			return errors.New("开发火墙必须手动指定中心，500毫秒后建立正交邻格")
		}
		if s.TargetKind != wanted || s.AutoPriority < 0 || s.AutoMinTargets < 0 || s.AutoMinTargets > len(f.Map.MonsterSpawns)+2 {
			return errors.New("技能行为、目标或自动策略不一致")
		}
		if s.Activation() != "cast" {
			if s.AutoMinTargets != 0 {
				return errors.New("被动或攻击模式不能配置施法自动目标数")
			}
		} else if s.AutoMinTargets < 1 {
			return errors.New("主动自动目标数必须明确")
		}
		// err 接收校验攻击技能过程中可能产生的错误，供后续失败分支处理。
		if err := f.validateAttackSkill(s); err != nil {
			return err
		}
		// err 接收校验位移技能过程中可能产生的错误，供后续失败分支处理。
		if err := f.validateDisplacementSkill(s); err != nil {
			return err
		}
	}
	return nil
}

// validateAttackSkill 校验攻击技能。
// f 是调用该方法的开发测试配置实例，s 记录技能。
func (f *Fixture) validateAttackSkill(s Skill) error {
	if f.RuleVersion >= 6 {
		// field 接收遍历文本列表时当前的元素。
		for _, field := range []string{"activation_kind", "max_attack_bonus_by_level", "ready_proc_denominator", "ready_proc_numerators", "secondary_direction_offsets", "secondary_distance_cells", "secondary_impact_ms", "charge_window_ms", "line_cells", "line_step_ms", "line_stop_on_blocked", "non_undead_divisor"} {
			if !s.present[field] {
				return errors.New("v6技能专有参数必须显式输出，不能缺失或为null：" + field)
			}
		}
		if s.ActivationKind == "" || s.MaxAttackBonusByLevel == nil || s.ReadyProcNumerators == nil || s.SecondaryDirectionOffsets == nil || s.NonUndeadDivisor < 1 {
			return errors.New("v6技能激活/数组/中性折减须明确")
		}
	}
	// expected 保存预期结果所使用的文本。
	expected := "cast"
	switch s.Behavior {
	case "fencing", "spirit_sword":
		expected = "passive"
	case "slaying":
		expected = "passive_proc"
	case "thrusting", "half_moon":
		expected = "toggle"
	case "flaming_sword":
		expected = "charge"
	}
	if s.Activation() != expected || s.SecondaryImpactMS < 0 || s.SecondaryImpactMS > math.MaxInt32 || s.ChargeWindowMS < 0 || s.ChargeWindowMS > math.MaxInt32 || s.LineStepMS < 0 || s.LineStepMS > math.MaxInt32 || s.NonUndeadDivisor < 0 || s.NonUndeadDivisor > math.MaxInt32 {
		return errors.New("技能激活类型或专有时序参数无效")
	}
	if s.Behavior == "slaying" {
		if s.ReadyProcDenominator == 0 || len(s.ReadyProcNumerators) != int(s.MaxLevel)+1 || len(s.MaxAttackBonusByLevel) != int(s.MaxLevel)+1 {
			return errors.New("攻杀准备概率和永久攻击成长不完整")
		}
		// i 定位就绪“proc”“numerators”中正在处理的成员，n 接收遍历就绪“proc”“numerators”时当前的元素。
		for i, n := range s.ReadyProcNumerators {
			if n > s.ReadyProcDenominator || s.MaxAttackBonusByLevel[i] < 0 || s.MaxAttackBonusByLevel[i] > math.MaxInt32 {
				return errors.New("攻杀准备概率或攻击成长越界")
			}
		}
	}
	if s.Behavior == "thrusting" || s.Behavior == "half_moon" {
		if len(s.SecondaryDirectionOffsets) == 0 || len(s.SecondaryDirectionOffsets) > 8 || s.SecondaryDistanceCells < 1 || s.SecondaryDistanceCells >= f.Map.Width {
			return errors.New("攻击模式旁格配置无效")
		}
		// seen 建立带初始值的按数值索引的布尔状态映射，供本段继续处理。
		seen := map[int32]bool{}
		// offset 接收遍历次优先朝向偏移集合时当前的元素。
		for _, offset := range s.SecondaryDirectionOffsets {
			if offset < -7 || offset > 7 || seen[(offset+8)%8] || offset == 0 && s.SecondaryDistanceCells == 1 {
				return errors.New("攻击模式格子重复或覆盖普通前格")
			}
			seen[(offset+8)%8] = true
		}
	}
	if s.Behavior == "flaming_sword" && s.ChargeWindowMS <= 0 {
		return errors.New("烈火充能窗口必须明确")
	}
	if s.Behavior == "hell_fire" || s.Behavior == "lightning" {
		if s.LineCells < 1 || s.LineCells >= f.Map.Width || s.Behavior == "hell_fire" && s.LineStepMS <= 0 {
			return errors.New("直线技能格数和推进时序缺失")
		}
		if s.Behavior == "hell_fire" && (s.LineCells != 4 || !s.LineStopOnBlocked) {
			return errors.New("地狱火须遵守用户确认的单线四格与无效格终止合同")
		}
	}
	if s.Behavior == "thunder_storm" && s.NonUndeadDivisor <= 0 {
		return errors.New("地狱雷光非不死系折减缺失")
	}
	return nil
}
