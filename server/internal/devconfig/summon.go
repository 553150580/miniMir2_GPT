// 本文件解析召唤来源、素材规则、宠物资格和当前开放状态。

package devconfig

import (
	"encoding/json"
	"errors"
	"math"
	"strings"
)

const (
	// SummonSkeletonSkillID 是骷髅召唤的稳定技能标识。
	SummonSkeletonSkillID uint32 = 17
	// SummonShinsuSkillID 是神兽召唤的稳定技能标识。
	SummonShinsuSkillID uint32 = 30
)

// 召唤只保存模板引用；延迟、材料数量及新生动作时钟继续使用既有唯一字段。
type SkillSummonRules struct {
	// DefinitionID 标识当前业务使用的“definition”。
	DefinitionID uint32 `json:"definition_id"`
}

// UnmarshalJSON 执行反序列化JSON，并把失败交给调用方处理。
// r 是调用该方法的技能召唤规则集合实例，data 携带供当前流程读取或解析的原始字节。
func (r *SkillSummonRules) UnmarshalJSON(data []byte) error {
	// plain 描述“plain”。
	type plain SkillSummonRules
	// value 记录值，使用“plain”表示。
	var value plain
	// err 接收解码宠物规则合同过程中可能产生的错误，供后续失败分支处理。
	if err := decodePetContract(data, &value); err != nil {
		return err
	}
	*r = SkillSummonRules(value)
	return nil
}

// 旧版是否夹带新键必须在JSON入口检查；不把presence标志留在可复制地图值中。
// data 携带供当前流程读取或解析的原始字节。
func validateSummoningVersionPresence(data []byte) error {
	// value 记录值，使用临时结构表示。
	var value struct {
		// RuleVersion 记录规则版本，使用数值表示。
		RuleVersion uint64 `json:"rule_version"`
		// Map 记录地图，使用按文本索引的原始值消息映射表示。
		Map map[string]json.RawMessage `json:"map"`
		// Maps 记录地图集合，使用按文本索引的原始值消息映射列表表示。
		Maps []map[string]json.RawMessage `json:"maps"`
		// PetRuntime 记录宠物运行时，使用临时结构表示。
		PetRuntime *struct {
			// Definitions 记录定义集合，使用按文本索引的原始值消息映射列表表示。
			Definitions []map[string]json.RawMessage `json:"definitions"`
		} `json:"pet_runtime"`
	}
	// err 接收反序列化过程中可能产生的错误，供后续失败分支处理。
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	if value.RuleVersion < 17 {
		// m 接收遍历“append”时当前的元素。
		for _, m := range append([]map[string]json.RawMessage{value.Map}, value.Maps...) {
			// provided 保存从m取出的“provided”。
			if _, provided := m["no_pets"]; provided {
				return errors.New("历史地图JSON不能夹带NoPets，即使其值为null")
			}
		}
		if value.PetRuntime != nil {
			// d 接收遍历定义集合时当前的元素。
			for _, d := range value.PetRuntime.Definitions {
				// provided 保存从d取出的“provided”。
				if _, provided := d["recall_name"]; provided {
					return errors.New("历史宠物JSON不能夹带召回名，即使是空字符串或null")
				}
			}
		}
	}
	return nil
}

// validRecallName 判断有效召回名称是否满足要求。
// name 记录名称，使用文本表示。
func validRecallName(name string) bool {
	return name != "" && strings.TrimSpace(name) == name && !strings.ContainsRune(name, '\x00') && len(name) <= 256
}

// 禁宠政策属于地图本身；安全区、禁传及是否有怪物均不能代替显式NoPets。
// f 是调用该方法的开发测试配置实例。
func (f *Fixture) ValidateSummoning() error {
	// maps 保留加入新条目后的地图集合。
	maps := append([]MapTemplate{f.Map}, f.Maps...)
	if f.RuleVersion < 17 {
		// m 接收遍历地图集合时当前的元素。
		for _, m := range maps {
			if m.NoPets != nil {
				return errors.New("禁宠地图政策须使用rule17及后续版本")
			}
		}
		// s 接收遍历技能集合时当前的元素。
		for _, s := range f.Skills {
			// provided 保存从“present”取出的“provided”。
			_, provided := s.present["summon"]
			if provided || s.Summon != nil || s.Behavior == "summon_pet" {
				return errors.New("召唤技能合同须使用rule17及后续版本")
			}
		}
		if f.PetRuntime != nil {
			// d 接收遍历定义集合时当前的元素。
			for _, d := range f.PetRuntime.Definitions {
				if d.RecallName != "" {
					return errors.New("历史宠物定义不能夹带新召回名合同")
				}
			}
		}
		return nil
	}
	// m 接收遍历地图集合时当前的元素。
	for _, m := range maps {
		if m.NoPets == nil {
			return errors.New("所有运行地图须显式提供独立NoPets规则")
		}
	}
	if f.PetRuntime == nil || !f.PetRuntime.Enabled {
		return errors.New("召唤地图政策须先启用独立宠物生命周期")
	}
	// d 接收遍历定义集合时当前的元素。
	for _, d := range f.PetRuntime.Definitions {
		if !validRecallName(d.RecallName) || d.SourceDataRangeCells() <= 0 {
			return errors.New("宠物召回须明确冻结的来源名称与共用DataRange")
		}
	}
	// s 接收遍历技能集合时当前的元素。
	for _, s := range f.Skills {
		// provided 保存从“present”取出的“provided”。
		_, provided := s.present["summon"]
		if s.Behavior != "summon_pet" {
			if s.Summon != nil || provided {
				return errors.New("非召唤技能不能附带召唤合同")
			}
			continue
		}
		// err 接收校验召唤技能过程中可能产生的错误，供后续失败分支处理。
		if err := validateSummonSkill(s); err != nil {
			return err
		}
		// d 接收定义“by”标识供后续步骤使用，ok 表明查询、校验或类型转换是否成功。
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

// validateSummonSkill 校验召唤技能。
// s 记录技能。
func validateSummonSkill(s Skill) error {
	// 技能与原书ID为稳定身份；材料数量及动作时钟由当前规则配置决定。
	validBook := s.ID == SummonSkeletonSkillID && s.BookItemID == 25 || s.ID == SummonShinsuSkillID && s.BookItemID == 33
	if !validBook || s.JobID != 2 || s.Behavior != "summon_pet" || s.Summon == nil || s.Summon.DefinitionID == 0 ||
		s.LearnedLevel != 0 || s.MaxLevel != 3 || s.TargetKind != "friendly_self" || s.Activation() != "cast" || s.AutoEnabled ||
		s.MaterialItemID == 0 || s.MaterialQuantity <= 0 || s.DelayBaseMS <= 0 || s.BuffID != 0 || s.Teleport != nil || s.Displacement != nil {
		return errors.New("职业召唤须引用对应技能书与单一宠物模板，并使用自身目标、材料与延迟施法")
	}
	// field 接收遍历文本列表时当前的元素。
	for _, field := range []string{"teleport", "displacement"} {
		// provided 保存从“present”取出的“provided”。
		if _, provided := s.present[field]; provided {
			return errors.New("召唤不能携带其它行为的子合同，即使为null")
		}
	}
	// v 接收遍历数值列表时当前的元素。
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
