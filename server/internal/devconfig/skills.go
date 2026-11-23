// 本文件装配职业技能、攻击模式、伤害范围和来源战斗时间配置。

package devconfig

import (
	"encoding/json"
	"errors"
	"math"
)

// Skill 保存技能所需的状态。
type Skill struct {
	// Summon 记录召唤，使用技能召唤规则集合表示。
	Summon *SkillSummonRules `json:"summon,omitempty"`
	// Teleport 记录瞬移，使用技能瞬移规则集合表示。
	Teleport *SkillTeleportRules `json:"teleport,omitempty"`
	// Displacement 记录位移，使用技能位移表示。
	Displacement *SkillDisplacement `json:"displacement,omitempty"`
	// ID 区分当前步骤指向的目标对象。
	ID uint32 `json:"id"`
	// Name 记录名称，使用文本表示。
	Name string `json:"name"`
	// JobID 标识当前业务使用的任务或职业编号。
	JobID uint32 `json:"job_id"`
	// BookItemID 标识当前业务使用的技能书道具。
	BookItemID uint32 `json:"book_item_id"`
	// LearnLevel 记录学习等级，使用数值表示。
	LearnLevel uint32 `json:"learn_level"`
	// LearnedLevel 记录已学习等级，使用数值表示。
	LearnedLevel uint32 `json:"learned_level"`
	// MaxLevel 记录最大等级，使用数值表示。
	MaxLevel uint32 `json:"max_level"`
	// Behavior 记录行为，使用文本表示。
	Behavior string `json:"behavior"`
	// BuffID 标识当前业务使用的“buff”。
	BuffID uint32 `json:"buff_id"`
	// MaterialItemID 标识当前业务使用的材料道具。
	MaterialItemID uint32 `json:"material_item_id"`
	// MaterialQuantity 记录材料数量，使用数值表示。
	MaterialQuantity int64 `json:"material_quantity"`
	// Poison 仅为施毒技能指定可实际装入和消耗的两色药粉。
	Poison *PoisonMaterials `json:"poison,omitempty"`
	// ActivationKind 记录“activation”类型，使用文本表示。
	ActivationKind string `json:"activation_kind"`
	// MPBaseCost 记录魔法值基础值消耗，使用数值表示。
	MPBaseCost int64 `json:"mp_base_cost"`
	// MPLevelCost 记录魔法值等级消耗，使用数值表示。
	MPLevelCost int64 `json:"mp_level_cost"`
	// CooldownBaseMS 记录冷却基础值毫秒，使用数值表示。
	CooldownBaseMS int64 `json:"cooldown_base_ms"`
	// CooldownReductionMS 记录冷却“reduction”毫秒，使用数值表示。
	CooldownReductionMS int64 `json:"cooldown_reduction_ms"`
	// GlobalSpellMS 记录全局法术毫秒，使用数值表示。
	GlobalSpellMS int64 `json:"global_spell_ms"`
	// ActionMS 记录动作毫秒，使用数值表示。
	ActionMS int64 `json:"action_ms"`
	// RangeCells 记录范围地图格集合，使用数值表示。
	RangeCells int `json:"range_cells"`
	// DelayBaseMS 记录“delay”基础值毫秒，使用数值表示。
	DelayBaseMS int64 `json:"delay_base_ms"`
	// ProjectileMSPerCell 记录“projectile”毫秒“per”地图格，使用数值表示。
	ProjectileMSPerCell int64 `json:"projectile_ms_per_cell"`
	// TargetMoveToleranceCells 记录目标移动“tolerance”地图格集合，使用数值表示。
	TargetMoveToleranceCells int `json:"target_move_tolerance_cells"`
	// AccuracyPerLevel 记录“accuracy”“per”等级，使用数值表示。
	AccuracyPerLevel int64 `json:"accuracy_per_level"`
	// AccuracyByLevel 记录“accuracy”“by”等级，使用数值列表表示。
	AccuracyByLevel []int64 `json:"accuracy_by_level"`
	// TargetKind 记录目标类型，使用文本表示。
	TargetKind string `json:"target_kind"`
	// AreaRadius 记录区域“radius”，使用数值表示。
	AreaRadius int `json:"area_radius"`
	// RequireLineOfSight 标记是否处于“require”线路“of”“sight”状态。
	RequireLineOfSight bool `json:"require_line_of_sight"`
	// HealingStatMultiplier 记录治疗属性倍率，使用数值表示。
	HealingStatMultiplier int64 `json:"healing_stat_multiplier"`
	// AddCasterLevel 标记是否处于新增施法者等级状态。
	AddCasterLevel bool `json:"add_caster_level"`
	// UndeadMultiplierPPM 记录“undead”倍率“ppm”，使用数值表示。
	UndeadMultiplierPPM int64 `json:"undead_multiplier_ppm"`
	// AutoPriority 记录自动“priority”，使用数值表示。
	AutoPriority int `json:"auto_priority"`
	// AutoMinTargets 记录自动最小目标集合，使用数值表示。
	AutoMinTargets int `json:"auto_min_targets"`
	// AutoEnabled 标记是否处于自动已开启状态。
	AutoEnabled bool `json:"auto_enabled"`
	// PowerBase 记录能力值基础值，使用数值表示。
	PowerBase int64 `json:"power_base"`
	// PowerBonus 记录能力值加成，使用数值表示。
	PowerBonus int64 `json:"power_bonus"`
	// MPowerBase 记录m能力值基础值，使用数值表示。
	MPowerBase int64 `json:"mpower_base"`
	// MPowerBonus 记录m能力值加成，使用数值表示。
	MPowerBonus int64 `json:"mpower_bonus"`
	// MultiplierBasePPM 记录倍率基础值“ppm”，使用数值表示。
	MultiplierBasePPM int64 `json:"multiplier_base_ppm"`
	// MultiplierBonusPPM 记录倍率加成“ppm”，使用数值表示。
	MultiplierBonusPPM int64 `json:"multiplier_bonus_ppm"`
	// MaxAttackBonusByLevel 记录最大攻击加成“by”等级，使用数值列表表示。
	MaxAttackBonusByLevel []int64 `json:"max_attack_bonus_by_level"`
	// ReadyProcDenominator 记录就绪“proc”“denominator”，使用数值表示。
	ReadyProcDenominator uint32 `json:"ready_proc_denominator"`
	// ReadyProcNumerators 记录就绪“proc”“numerators”，使用数值列表表示。
	ReadyProcNumerators []uint32 `json:"ready_proc_numerators"`
	// SecondaryDirectionOffsets 记录次优先朝向偏移集合，使用数值列表表示。
	SecondaryDirectionOffsets []int32 `json:"secondary_direction_offsets"`
	// SecondaryDistanceCells 记录次优先距离地图格集合，使用数值表示。
	SecondaryDistanceCells int `json:"secondary_distance_cells"`
	// SecondaryImpactMS 记录次优先命中影响毫秒，使用数值表示。
	SecondaryImpactMS int64 `json:"secondary_impact_ms"`
	// ChargeWindowMS 记录“charge”窗口毫秒，使用数值表示。
	ChargeWindowMS int64 `json:"charge_window_ms"`
	// LineCells 记录线路地图格集合，使用数值表示。
	LineCells int `json:"line_cells"`
	// LineStepMS 记录线路步骤毫秒，使用数值表示。
	LineStepMS int64 `json:"line_step_ms"`
	// LineStopOnBlocked 标记是否处于线路停止启用状态阻挡状态。
	LineStopOnBlocked bool `json:"line_stop_on_blocked"`
	// NonUndeadDivisor 记录“non”“undead”“divisor”，使用数值表示。
	NonUndeadDivisor int64 `json:"non_undead_divisor"`
	// present 记录“present”，使用按文本索引的布尔状态映射表示。
	present map[string]bool
}

// UnmarshalJSON 执行反序列化JSON，并把失败交给调用方处理。
// s 是调用该方法的技能实例，data 携带供当前流程读取或解析的原始字节。
func (s *Skill) UnmarshalJSON(data []byte) error {
	// rawSkill 描述原始值技能。
	type rawSkill Skill
	// raw 记录原始值，使用原始值技能表示。
	var raw rawSkill
	// err 接收反序列化过程中可能产生的错误，供后续失败分支处理。
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	// fields 记录字段集合，使用按文本索引的原始值消息映射表示。
	var fields map[string]json.RawMessage
	// err 接收反序列化过程中可能产生的错误，供后续失败分支处理。
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	*s = Skill(raw)
	s.present = map[string]bool{}
	// name 定位字段集合中正在处理的成员，value 接收遍历字段集合时当前的元素。
	for name, value := range fields {
		s.present[name] = string(value) != "null"
	}
	return nil
}

// HealingRules 保存治疗规则集合所需的状态。
type HealingRules struct {
	// TickMS 记录模拟帧毫秒，使用数值表示。
	TickMS int64 `json:"tick_ms"`
	// BasePerTick 记录基础值“per”模拟帧，使用数值表示。
	BasePerTick int64 `json:"base_per_tick"`
	// LevelsPerStep 记录等级集合“per”步骤，使用数值表示。
	LevelsPerStep int64 `json:"levels_per_step"`
	// QueueFractionDivisor 记录队列“fraction”“divisor”，使用数值表示。
	QueueFractionDivisor int64 `json:"queue_fraction_divisor"`
	// QueueCap 记录队列上限，使用数值表示。
	QueueCap int64 `json:"queue_cap"`
	// PetBasePerTick 记录宠物基础值“per”模拟帧，使用数值表示。
	PetBasePerTick int64 `json:"pet_base_per_tick"`
}

// InitialSkillBook 保存初始值技能技能书所需的状态。
type InitialSkillBook struct {
	// JobID 标识当前业务使用的任务或职业编号。
	JobID uint32 `json:"job_id"`
	// ItemID 标识当前业务使用的道具。
	ItemID uint32 `json:"item_id"`
}

// PlayerGrowth 保存玩家成长所需的状态。
type PlayerGrowth struct {
	// JobID 标识当前业务使用的任务或职业编号。
	JobID uint32 `json:"job_id"`
	// HPBase 记录生命值基础值，使用数值表示。
	HPBase int64 `json:"hp_base"`
	// MPBase 记录魔法值基础值，使用数值表示。
	MPBase int64 `json:"mp_base"`
	// HPGain 记录生命值“gain”，使用数值表示。
	HPGain float32 `json:"hp_gain"`
	// HPGainRate 记录生命值“gain”比例或变化速率，使用数值表示。
	HPGainRate float32 `json:"hp_gain_rate"`
	// MPGain 记录魔法值“gain”，使用数值表示。
	MPGain float32 `json:"mp_gain"`
	// MPGainRate 记录魔法值“gain”比例或变化速率，使用数值表示。
	MPGainRate float32 `json:"mp_gain_rate"`
	// AttackDivisor 记录攻击“divisor”，使用数值表示。
	AttackDivisor float32 `json:"attack_divisor"`
	// DefenceMaxDivisor 记录防御最大“divisor”，使用数值表示。
	DefenceMaxDivisor float32 `json:"defence_max_divisor"`
	// MagicDivisor 记录魔法“divisor”，使用数值表示。
	MagicDivisor float32 `json:"magic_divisor"`
	// TaoDivisor 记录“tao”“divisor”，使用数值表示。
	TaoDivisor float32 `json:"tao_divisor"`
	// MagicDefenceMinDivisor 记录魔法防御最小“divisor”，使用数值表示。
	MagicDefenceMinDivisor float32 `json:"magic_defence_min_divisor"`
	// MagicDefenceMaxDivisor 记录魔法防御最大“divisor”，使用数值表示。
	MagicDefenceMaxDivisor float32 `json:"magic_defence_max_divisor"`
}

// Crystal MagicInfo.cs:180–211：显式逐步转回float32，保留C# Single中间舍入。
// s 是调用该方法的技能实例，level 记录等级，使用数值表示，mpower 记录“mpower”，使用数值表示，power 记录能力值，使用数值表示。
func (s Skill) SourcePower(level uint32, mpower, power int64) int64 {
	// quarter 保存由“float”“32”取得的“quarter”。
	quarter := float32(float32(mpower) / float32(4))
	// scaled 保存由“float”“32”取得的“scaled”。
	scaled := float32(quarter * float32(level+1))
	return int64(math.RoundToEven(float64(float32(scaled + float32(power)))))
}

// SourceDamage 返回来源伤害供调用方使用。
// s 是调用该方法的技能实例，level 记录等级，使用数值表示，base 记录基础值，使用数值表示，power 记录能力值，使用数值表示。
func (s Skill) SourceDamage(level uint32, base, power int64) int64 {
	// baseMultiplier 保存由“float”“32”取得的基础值倍率。
	baseMultiplier := float32(float64(s.MultiplierBasePPM) / 1000000)
	// bonusMultiplier 保存由“float”“32”取得的加成倍率。
	bonusMultiplier := float32(float64(s.MultiplierBonusPPM) / 1000000)
	// bonus 保存由“float”“32”取得的加成。
	bonus := float32(float32(level) * bonusMultiplier)
	// multiplier 保存由“float”“32”取得的倍率。
	multiplier := float32(baseMultiplier + bonus)
	// amount 保存由“float”“32”取得的数量。
	amount := float32(float32(base+power) * multiplier)
	if float64(amount) >= math.MaxInt32 {
		return math.MaxInt32
	}
	return int64(amount)
}

// Skill 处理技能。
// f 是调用该方法的开发测试配置实例，id 区分当前步骤指向的目标对象。
func (f *Fixture) Skill(id uint32) (Skill, bool) {
	// s 接收遍历技能集合时当前的元素。
	for _, s := range f.Skills {
		if s.ID == id {
			return s, true
		}
	}
	return Skill{}, false
}

// PlayerAtLevel 处理玩家发生时间等级。
// f 是调用该方法的开发测试配置实例，job 记录任务或职业编号，使用数值表示，level 记录等级，使用数值表示。
func (f *Fixture) PlayerAtLevel(job, level uint32) (Fighter, bool) {
	// p 接收玩家供后续步骤使用，ok 表明查询、校验或类型转换是否成功。
	p, ok := f.Player(job)
	if !ok {
		return p, false
	}
	p.Level = level
	// g 接收遍历玩家成长时当前的元素。
	for _, g := range f.PlayerGrowth {
		if g.JobID != job {
			continue
		}
		// l 保存“float”“32”返回的结果。
		l := float32(level)
		// Crystal Shared/BaseStats.cs:155–182；中间运算保留C# float32，最后截断。
		// hp 保存根据“float”“32”与“float”“32”与l的计算结果计算的生命值。
		hp := float32(g.HPBase) + float32(float32(l/g.HPGain)+g.HPGainRate)*l
		if job == 0 {
			hp = float32(g.HPBase) + float32(float32(float32(l/g.HPGain)+g.HPGainRate)+float32(l/20))*l
		}
		// mp 保存根据“float”“32”与“float”“32”的计算结果与“float”“32”计算的魔法值。
		mp := float32(g.MPBase) + float32(l*g.MPGain) + float32(l*g.MPGainRate)
		if job == 1 {
			mp = float32(float32(g.MPBase)+float32(float32(float32(l/g.MPGain)+2)*2.2)*l) + float32(l*g.MPGainRate)
		}
		if job == 2 {
			mp = float32(float32(g.MPBase)+float32(float32(l/g.MPGain)*2.2)*l) + float32(l*g.MPGainRate)
		}
		p.HP = int64(math.Min(float64(hp), math.MaxInt32))
		p.MP = int64(math.Min(float64(mp), math.MaxInt32))
		// stat 封装属性对应的后续处理，div 记录“div”，使用数值表示。
		stat := func(div float32) int64 {
			if div <= 0 {
				return 0
			}
			return int64(l / div)
		}
		p.AttackMin = stat(g.AttackDivisor)
		p.AttackMax = p.AttackMin
		p.DefenceMax = stat(g.DefenceMaxDivisor)
		p.MagicMin = stat(g.MagicDivisor)
		p.MagicMax = p.MagicMin
		p.TaoMin = stat(g.TaoDivisor)
		p.TaoMax = p.TaoMin
		p.MagicDefenceMin = stat(g.MagicDefenceMinDivisor)
		p.MagicDefenceMax = stat(g.MagicDefenceMaxDivisor)
		break
	}
	return p, true
}

// ValidateSkills 校验技能集合。
// f 是调用该方法的开发测试配置实例。
func (f *Fixture) ValidateSkills() error {
	// seen 建立带初始值的按数值索引的布尔状态映射，供本段继续处理。
	seen := map[uint32]bool{}
	// s 接收遍历技能集合时当前的元素。
	for _, s := range f.Skills {
		// book 保存由道具取得的技能书，ok 表明查询、校验或类型转换是否成功。
		book, ok := f.Item(s.BookItemID)
		if s.ID == 0 || seen[s.ID] || s.Name == "" || s.JobID > 2 || !ok || book.BookSkillID != s.ID || s.LearnedLevel > 255 || s.RangeCells < 0 || s.RangeCells > f.Map.Width || s.TargetMoveToleranceCells < 0 || s.TargetMoveToleranceCells > f.Map.Width {
			return errors.New("技能身份、职业、书或范围缺失")
		}
		if !supportedSkill(s.Behavior) {
			return errors.New("当前候选不支持此技能行为")
		}
		// v 接收遍历数值列表时当前的元素。
		for _, v := range []int64{s.MPBaseCost, s.MPLevelCost, s.CooldownBaseMS, s.CooldownReductionMS, s.GlobalSpellMS, s.ActionMS, s.DelayBaseMS, s.ProjectileMSPerCell, s.AccuracyPerLevel, s.PowerBase, s.PowerBonus, s.MPowerBase, s.MPowerBonus, s.MultiplierBasePPM, s.MultiplierBonusPPM} {
			if v < 0 || v > math.MaxInt32 {
				return errors.New("技能参数越界")
			}
		}
		if s.MaxLevel < s.LearnedLevel || s.MaxLevel > 255 || s.CooldownReductionMS*int64(s.MaxLevel) > s.CooldownBaseMS {
			return errors.New("技能冷却不能为负")
		}
		seen[s.ID] = true
	}
	if len(seen) == 0 || len(f.PlayerGrowth) != len(f.Players) {
		return errors.New("职业成长与技能定义不完整")
	}
	// growth 初始化用作成长的按数值索引的布尔状态映射。
	growth := map[uint32]bool{}
	// g 接收遍历玩家成长时当前的元素。
	for _, g := range f.PlayerGrowth {
		if g.JobID > 2 || growth[g.JobID] || g.HPBase <= 0 || g.MPBase < 0 || g.HPGain <= 0 || g.MPGain <= 0 {
			return errors.New("基础成长参数无效")
		}
		// value 接收遍历数值列表时当前的元素。
		for _, value := range []float32{g.HPGain, g.HPGainRate, g.MPGain, g.MPGainRate, g.AttackDivisor, g.DefenceMaxDivisor, g.MagicDivisor, g.TaoDivisor, g.MagicDefenceMinDivisor, g.MagicDefenceMaxDivisor} {
			if value < 0 || math.IsNaN(float64(value)) || math.IsInf(float64(value), 0) {
				return errors.New("成长参数须为有限非负数")
			}
		}
		growth[g.JobID] = true
	}
	// h 保存治疗，用于当前步骤中的h。
	h := f.Healing
	if h.TickMS <= 0 || h.TickMS > math.MaxInt32 || h.BasePerTick <= 0 || h.LevelsPerStep <= 0 || h.QueueFractionDivisor <= 0 || h.QueueCap <= 0 || h.QueueCap > math.MaxInt32 || h.PetBasePerTick <= 0 {
		return errors.New("治疗队列参数无效")
	}
	// b 接收遍历初始值技能技能书集合时当前的元素。
	for _, b := range f.InitialSkillBooks {
		// item 接收道具供后续步骤使用，ok 表明查询、校验或类型转换是否成功。
		item, ok := f.Item(b.ItemID)
		// s 接收技能供后续步骤使用，exists 保存由技能取得的存在。
		s, exists := f.Skill(item.BookSkillID)
		if !ok || !exists || s.JobID != b.JobID {
			return errors.New("初始职业书引用不符")
		}
	}
	// a 接收遍历“append”时当前的元素。
	for _, a := range append(append(append([]Fighter{}, f.Players...), f.Pet), monsterFighters(f.Monsters)...) {
		if a.MagicMin < 0 || a.MagicMax < a.MagicMin || a.TaoMin < 0 || a.TaoMax < a.TaoMin || a.DefenceMin < 0 || a.DefenceMax < a.DefenceMin || a.MagicDefenceMin < 0 || a.MagicDefenceMax < a.MagicDefenceMin || a.Accuracy < 0 || a.Agility < 0 || a.MagicResist < 0 || a.MagicResistWeight <= 0 || a.MagicResist > a.MagicResistWeight {
			return errors.New("AC/MAC/命中/抗魔区间无效")
		}
	}
	// upgradesEnabled 保存根据“len”与当前数据的计算结果与“len”与当前数据的计算结果的计算结果与页面或分页道具标识与当前数据的计算结果计算的“upgrades”已开启。
	upgradesEnabled := len(f.SkillUpgrades) > 0 || len(f.SkillShop) > 0 || f.SkillEconomy.PageItemID != 0
	if f.CombatTiming.PlayerAttackBaseMS > 0 {
		// err 接收校验战斗过程中可能产生的错误，供后续失败分支处理。
		if err := f.ValidateCombat(); err != nil {
			return err
		}
	}
	// s 接收遍历技能集合时当前的元素。
	for _, s := range f.Skills {
		upgradesEnabled = upgradesEnabled || s.MaxLevel > s.LearnedLevel
	}
	// err 接收校验“buffs”过程中可能产生的错误，供后续失败分支处理。
	if err := f.ValidateBuffs(); err != nil {
		return err
	}
	if upgradesEnabled {
		return f.ValidateSkillEconomy()
	}
	return nil
}

// monsterFighters 返回怪物“fighters”供调用方使用。
// monsters 记录怪物集合，使用怪物列表表示。
func monsterFighters(monsters []Monster) []Fighter {
	// out 创建用于维护输出的容器或通道。
	out := make([]Fighter, 0, len(monsters))
	// monster 接收遍历怪物集合时当前的元素。
	for _, monster := range monsters {
		out = append(out, monster.Fighter)
	}
	return out
}
