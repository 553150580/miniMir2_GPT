package devconfig

import (
	"errors"
	"math"
)

type CombatTiming struct {
	PlayerAttackBaseMS      int64 `json:"player_attack_base_ms"`
	PlayerAttackSpeedStepMS int64 `json:"player_attack_speed_step_ms"`
	PlayerAttackLevelStepMS int64 `json:"player_attack_level_step_ms"`
	PlayerAttackLevelCapMS  int64 `json:"player_attack_level_cap_ms"`
	PlayerAttackMinMS       int64 `json:"player_attack_min_ms"`
	PlayerAttackActionMS    int64 `json:"player_attack_action_ms"`
	PlayerAttackImpactMS    int64 `json:"player_attack_impact_ms"`
	EventCapacity           int   `json:"event_capacity"`
	KillQueueCapacity       int   `json:"kill_queue_capacity"`
}

func (s Skill) AccuracyBonus(level uint32) int64 {
	if int(level) < len(s.AccuracyByLevel) {
		return s.AccuracyByLevel[level]
	}
	return int64(level) * s.AccuracyPerLevel
}
func (t CombatTiming) PlayerAttackInterval(level uint32, speed int64) int64 {
	reduction := int64(level) * t.PlayerAttackLevelStepMS
	if reduction > t.PlayerAttackLevelCapMS {
		reduction = t.PlayerAttackLevelCapMS
	}
	value := t.PlayerAttackBaseMS - speed*t.PlayerAttackSpeedStepMS - reduction
	if value < t.PlayerAttackMinMS {
		value = t.PlayerAttackMinMS
	}
	if value > math.MaxInt32 {
		value = math.MaxInt32
	}
	return value
}
func (f *Fixture) ValidateCombat() error {
	t := f.CombatTiming
	for _, v := range []int64{t.PlayerAttackBaseMS, t.PlayerAttackSpeedStepMS, t.PlayerAttackLevelStepMS, t.PlayerAttackLevelCapMS, t.PlayerAttackMinMS, t.PlayerAttackActionMS, t.PlayerAttackImpactMS} {
		if v <= 0 || v > math.MaxInt32 {
			return errors.New("战斗时序参数必须完整且可计时")
		}
	}
	if t.PlayerAttackMinMS > t.PlayerAttackBaseMS || t.EventCapacity < 1 || t.EventCapacity > 4096 || t.KillQueueCapacity < len(f.Map.MonsterSpawns)+1 || t.KillQueueCapacity > 4096 {
		return errors.New("战斗事件或逐杀预算无效")
	}
	for _, s := range f.Skills {
		if len(s.AccuracyByLevel) != int(s.MaxLevel)+1 {
			return errors.New("逐阶准确数组不完整")
		}
		for _, v := range s.AccuracyByLevel {
			if v < 0 || v > math.MaxInt32 {
				return errors.New("逐阶准确越界")
			}
		}
		if s.AreaRadius < 0 || s.AreaRadius >= f.Map.Width || s.HealingStatMultiplier < 0 || s.HealingStatMultiplier > math.MaxInt32 || s.UndeadMultiplierPPM <= 0 || s.UndeadMultiplierPPM > math.MaxInt32 {
			return errors.New("技能范围或数值路径无效")
		}
		switch s.TargetKind {
		case "passive", "enemy_entity", "enemy_grid", "friendly_entity", "friendly_grid":
		default:
			return errors.New("技能目标类型无效")
		}
		wanted := "enemy_entity"
		switch s.Behavior {
		case "fencing", "spirit_sword":
			wanted = "passive"
		case "healing":
			wanted = "friendly_entity"
		case "mass_healing":
			wanted = "friendly_grid"
		case "firebang":
			wanted = "enemy_grid"
		}
		if s.TargetKind != wanted || s.AutoPriority < 0 || s.AutoMinTargets < 0 || s.AutoMinTargets > len(f.Map.MonsterSpawns)+2 {
			return errors.New("技能行为、目标或自动策略不一致")
		}
		if wanted == "passive" {
			if s.AutoMinTargets != 0 {
				return errors.New("被动不能配置自动目标数")
			}
		} else if s.AutoMinTargets < 1 {
			return errors.New("主动自动目标数必须明确")
		}
	}
	return nil
}
