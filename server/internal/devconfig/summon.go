package devconfig

import (
	"encoding/json"
	"errors"
	"math"
	"strings"
)

// 召唤只保存模板引用；延迟、材料数量及新生动作时钟继续使用既有唯一字段。
type SkillSummonRules struct {
	DefinitionID uint32 `json:"definition_id"`
}

func (r *SkillSummonRules) UnmarshalJSON(data []byte) error {
	type plain SkillSummonRules
	var value plain
	if err := decodePetContract(data, &value); err != nil {
		return err
	}
	*r = SkillSummonRules(value)
	return nil
}

// 旧版是否夹带新键必须在JSON入口检查；不把presence标志留在可复制地图值中。
func validateSummoningVersionPresence(data []byte) error {
	var value struct {
		RuleVersion uint64                       `json:"rule_version"`
		Map         map[string]json.RawMessage   `json:"map"`
		Maps        []map[string]json.RawMessage `json:"maps"`
		PetRuntime  *struct {
			Definitions []map[string]json.RawMessage `json:"definitions"`
		} `json:"pet_runtime"`
	}
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	if value.RuleVersion < 17 {
		for _, m := range append([]map[string]json.RawMessage{value.Map}, value.Maps...) {
			if _, provided := m["no_pets"]; provided {
				return errors.New("历史地图JSON不能夹带NoPets，即使其值为null")
			}
		}
		if value.PetRuntime != nil {
			for _, d := range value.PetRuntime.Definitions {
				if _, provided := d["recall_name"]; provided {
					return errors.New("历史宠物JSON不能夹带召回名，即使是空字符串或null")
				}
			}
		}
	}
	return nil
}

func validRecallName(name string) bool {
	return name != "" && strings.TrimSpace(name) == name && !strings.ContainsRune(name, '\x00') && len(name) <= 256
}

// 禁宠政策属于地图本身；安全区、禁传及是否有怪物均不能代替显式NoPets。
func (f *Fixture) ValidateSummoning() error {
	maps := append([]MapTemplate{f.Map}, f.Maps...)
	if f.RuleVersion < 17 {
		for _, m := range maps {
			if m.NoPets != nil {
				return errors.New("禁宠地图政策须使用rule17及后续版本")
			}
		}
		for _, s := range f.Skills {
			_, provided := s.present["summon"]
			if provided || s.Summon != nil || s.Behavior == "summon_pet" {
				return errors.New("召唤技能合同须使用rule17及后续版本")
			}
		}
		if f.PetRuntime != nil {
			for _, d := range f.PetRuntime.Definitions {
				if d.RecallName != "" {
					return errors.New("历史宠物定义不能夹带新召回名合同")
				}
			}
		}
		return nil
	}
	for _, m := range maps {
		if m.NoPets == nil {
			return errors.New("所有运行地图须显式提供独立NoPets规则")
		}
	}
	if f.PetRuntime == nil || !f.PetRuntime.Enabled {
		return errors.New("召唤地图政策须先启用独立宠物生命周期")
	}
	for _, d := range f.PetRuntime.Definitions {
		if !validRecallName(d.RecallName) || d.SourceDataRangeCells() <= 0 {
			return errors.New("宠物召回须明确冻结的来源名称与共用DataRange")
		}
	}
	for _, s := range f.Skills {
		_, provided := s.present["summon"]
		if s.Behavior != "summon_pet" {
			if s.Summon != nil || provided {
				return errors.New("非召唤技能不能附带召唤合同")
			}
			continue
		}
		if err := validateSummonSkill(s); err != nil {
			return err
		}
		d, ok := f.PetRuntime.DefinitionByID(s.Summon.DefinitionID)
		if !ok || d.OwnerJobID != s.JobID || d.SourceSkillID != s.ID || int(s.MaxLevel)+1 != len(d.Training.InitialLevelByRank) {
			return errors.New("召唤技能的唯一模板、职业及逐阶训练引用不一致")
		}
		if !f.IsCastingSupply(s.MaterialItemID) {
			return errors.New("召唤材料须引用明确的独立护符白名单")
		}
	}
	return nil
}

func validateSummonSkill(s Skill) error {
	if s.ID != 17 || s.JobID != 2 || s.BookItemID != 25 || s.Behavior != "summon_pet" || s.Summon == nil || s.Summon.DefinitionID == 0 ||
		s.LearnedLevel != 0 || s.MaxLevel != 3 || s.TargetKind != "friendly_self" || s.Activation() != "cast" || s.AutoEnabled ||
		s.MaterialItemID == 0 || s.MaterialQuantity <= 0 || s.DelayBaseMS <= 0 || s.BuffID != 0 || s.Teleport != nil || s.Displacement != nil {
		return errors.New("召唤骷髅须为独立0至3阶、单宠模板、自身材料延迟技能")
	}
	for _, field := range []string{"teleport", "displacement"} {
		if _, provided := s.present[field]; provided {
			return errors.New("召唤不能携带其它行为的子合同，即使为null")
		}
	}
	for _, v := range []int64{s.MPBaseCost, s.MPLevelCost, s.CooldownBaseMS, s.CooldownReductionMS, s.GlobalSpellMS, s.ActionMS, s.DelayBaseMS} {
		if v < 0 || v > math.MaxInt32 {
			return errors.New("召唤动作与耗蓝参数越界")
		}
	}
	if s.LearnLevel == 0 || s.CooldownReductionMS*int64(s.MaxLevel) > s.CooldownBaseMS {
		return errors.New("召唤学习等级及逐阶冷却不完整")
	}
	if s.RangeCells != 0 || s.AreaRadius != 0 || s.ProjectileMSPerCell != 0 || s.TargetMoveToleranceCells != 0 || s.RequireLineOfSight || s.AutoPriority != 0 || s.AutoMinTargets != 0 ||
		s.PowerBase != 0 || s.PowerBonus != 0 || s.MPowerBase != 0 || s.MPowerBonus != 0 || s.MultiplierBasePPM != 0 || s.MultiplierBonusPPM != 0 ||
		s.AccuracyPerLevel != 0 || len(s.AccuracyByLevel) != 0 || s.HealingStatMultiplier != 0 || s.AddCasterLevel || s.UndeadMultiplierPPM != 0 ||
		len(s.MaxAttackBonusByLevel) != 0 || s.ReadyProcDenominator != 0 || len(s.ReadyProcNumerators) != 0 || len(s.SecondaryDirectionOffsets) != 0 ||
		s.SecondaryDistanceCells != 0 || s.SecondaryImpactMS != 0 || s.ChargeWindowMS != 0 || s.LineCells != 0 || s.LineStepMS != 0 || s.LineStopOnBlocked || s.NonUndeadDivisor != 0 {
		return errors.New("召唤不能夹带未使用的伤害、治疗、随机或攻击模式参数")
	}
	return nil
}
