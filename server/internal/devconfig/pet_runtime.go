// 本文件建立宠物模板、来源属性和升级参数，深度冻结可用配置并校验启用条件。

package devconfig

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"math"
	"os"
	"reflect"
	"slices"
	"strings"
)

// 独立候选文件与实际profile选择分开；数值均由唯一Excel编译产生。
type PetRuntimeCandidateCatalog struct {
	// SchemaVersion 记录“schema”版本，使用数值表示。
	SchemaVersion int `json:"schema_version"`
	// DevelopmentOnly 标记是否处于“development”“only”状态。
	DevelopmentOnly bool `json:"development_only"`
	// RuntimeEnabled 标记是否处于运行时已开启状态。
	RuntimeEnabled bool `json:"runtime_enabled"`
	// Budgets 记录“budgets”，使用宠物运行时“budgets”表示。
	Budgets PetRuntimeBudgets `json:"budgets"`
	// Definitions 记录定义集合，使用宠物运行时定义列表表示。
	Definitions []PetRuntimeDefinition `json:"definitions"`
	// SourceEvidence 记录来源“evidence”，使用按文本索引的原始值消息映射表示。
	SourceEvidence map[string]json.RawMessage `json:"source_evidence"`
}

// PetRuntimeRules 保存宠物运行时规则集合所需的状态。
type PetRuntimeRules struct {
	// Enabled 标记是否处于已开启状态。
	Enabled bool `json:"enabled"`
	// BootstrapMode 记录“bootstrap”模式，使用文本表示。
	BootstrapMode string `json:"bootstrap_mode"`
	// Budgets 记录“budgets”，使用宠物运行时“budgets”表示。
	Budgets PetRuntimeBudgets `json:"budgets"`
	// Definitions 记录定义集合，使用宠物运行时定义列表表示。
	Definitions []PetRuntimeDefinition `json:"definitions"`
}

// PetRuntimeBudgets 保存宠物运行时“budgets”所需的状态。
type PetRuntimeBudgets struct {
	// RetainedBodyCapacityPerOwner 记录“retained”消息正文容量“per”所有者，使用数值表示。
	RetainedBodyCapacityPerOwner int `json:"retained_body_capacity_per_owner"`
	// FullPolicy 记录完整或满额策略，使用文本表示。
	FullPolicy string `json:"full_policy"`
}

// PetRuntimeDefinition 保存宠物运行时定义所需的状态。
type PetRuntimeDefinition struct {
	// ID 区分当前步骤指向的目标对象。
	ID uint32 `json:"id"`
	// SourceSkillID 标识当前业务使用的来源技能。
	SourceSkillID uint32 `json:"source_skill_id"`
	// MonsterTemplateID 标识当前业务使用的怪物模板。
	MonsterTemplateID uint32 `json:"monster_template_id"`
	// OwnerJobID 标识当前业务使用的所有者任务或职业编号。
	OwnerJobID uint32 `json:"owner_job_id"`
	// ResourceKey 记录“resource”键，使用文本表示。
	ResourceKey string `json:"resource_key"`
	// RecallName 记录召回名称，使用文本表示。
	RecallName string `json:"recall_name,omitempty"`
	// Base 记录基础值，使用战斗单位表示。
	Base Fighter `json:"base"`
	// Undead 标记是否处于“undead”状态。
	Undead bool `json:"undead"`
	// Training 记录训练，使用宠物训练规则集合表示。
	Training PetTrainingRules `json:"training"`
	// Speed 记录速度，使用宠物速度规则集合表示。
	Speed PetSpeedRules `json:"speed"`
	// Lifecycle 记录“lifecycle”，使用宠物“lifetime”规则集合表示。
	Lifecycle PetLifetimeRules `json:"lifecycle"`
	// Growth 记录成长，使用宠物成长等级列表表示。
	Growth []PetGrowthLevel `json:"growth"`
}

// PetTrainingRules 保存宠物训练规则集合所需的状态。
type PetTrainingRules struct {
	// InitialLevelByRank 记录初始值等级“by”排序位次，使用数值列表表示。
	InitialLevelByRank []uint32 `json:"initial_level_by_rank"`
	// MaximumLevelByRank 记录最大值等级“by”排序位次，使用数值列表表示。
	MaximumLevelByRank []uint32 `json:"maximum_level_by_rank"`
	// ExperienceMultiplier 记录经验倍率，使用数值表示。
	ExperienceMultiplier uint64 `json:"experience_multiplier"`
	// OwnerRangeCells 记录所有者范围地图格集合，使用数值表示。
	OwnerRangeCells int `json:"owner_range_cells"`
	// ExperienceBasis 记录经验“basis”，使用文本表示。
	ExperienceBasis string `json:"experience_basis"`
	// GrantPhase 记录赋予道具或权限阶段，使用文本表示。
	GrantPhase string `json:"grant_phase"`
	// Eligibility 记录“eligibility”，使用文本表示。
	Eligibility string `json:"eligibility"`
	// MaxLevelUpsPerGrant 记录最大等级“ups”“per”赋予道具或权限，使用数值表示。
	MaxLevelUpsPerGrant int `json:"max_level_ups_per_grant"`
	// ExcessExperience 记录“excess”经验，使用文本表示。
	ExcessExperience string `json:"excess_experience"`
	// AtMaximum 记录发生时间最大值，使用文本表示。
	AtMaximum string `json:"at_maximum"`
	// CurrentHPOnGrowth 记录当前状态生命值启用状态成长，使用文本表示。
	CurrentHPOnGrowth string `json:"current_hp_on_growth"`
}

// PetSpeedRules 保存宠物速度规则集合所需的状态。
type PetSpeedRules struct {
	// Basis 记录“basis”，使用文本表示。
	Basis string `json:"basis"`
	// MoveReductionMSPerLevel 记录移动“reduction”毫秒“per”等级，使用数值表示。
	MoveReductionMSPerLevel int64 `json:"move_reduction_ms_per_level"`
	// AttackReductionMSPerLevel 记录攻击“reduction”毫秒“per”等级，使用数值表示。
	AttackReductionMSPerLevel int64 `json:"attack_reduction_ms_per_level"`
	// MinimumMoveIntervalMS 记录“minimum”移动周期毫秒，使用数值表示。
	MinimumMoveIntervalMS int64 `json:"minimum_move_interval_ms"`
	// MinimumAttackIntervalMS 记录“minimum”攻击周期毫秒，使用数值表示。
	MinimumAttackIntervalMS int64 `json:"minimum_attack_interval_ms"`
}

// PetLifetimeRules 保存宠物“lifetime”规则集合所需的状态。
type PetLifetimeRules struct {
	// CorpseTTLMS 记录尸体“ttlms”，使用数值表示。
	CorpseTTLMS int64 `json:"corpse_ttl_ms"`
	// SpawnReadyMS 记录生成或出生点就绪毫秒，使用数值表示。
	SpawnReadyMS int64 `json:"spawn_ready_ms"`
	// LiveExpiry 记录存活过期时间，使用文本表示。
	LiveExpiry string `json:"live_expiry"`
	// ColdEnter 记录“cold”入场，使用文本表示。
	ColdEnter string `json:"cold_enter"`
}

// PetGrowthLevel 保存宠物成长等级所需的状态。
type PetGrowthLevel struct {
	// Level 记录等级，使用数值表示。
	Level uint32 `json:"level"`
	// HPBonus 记录生命值加成，使用数值表示。
	HPBonus int64 `json:"hp_bonus"`
	// AttackMinBonus 记录攻击最小加成，使用数值表示。
	AttackMinBonus int64 `json:"attack_min_bonus"`
	// AttackMaxBonus 记录攻击最大加成，使用数值表示。
	AttackMaxBonus int64 `json:"attack_max_bonus"`
	// DefenceMinBonus 记录防御最小加成，使用数值表示。
	DefenceMinBonus int64 `json:"defence_min_bonus"`
	// DefenceMaxBonus 记录防御最大加成，使用数值表示。
	DefenceMaxBonus int64 `json:"defence_max_bonus"`
	// MagicDefenceMinBonus 记录魔法防御最小加成，使用数值表示。
	MagicDefenceMinBonus int64 `json:"magic_defence_min_bonus"`
	// MagicDefenceMaxBonus 记录魔法防御最大加成，使用数值表示。
	MagicDefenceMaxBonus int64 `json:"magic_defence_max_bonus"`
	// RequiredExperience 记录“required”经验，使用数值表示。
	RequiredExperience *uint64 `json:"required_experience"`
}

// 这些合同不接受缺字段、null默认、未知键或以字符串冒充整数。
// 只有成长终态的required_experience允许显式null。
// raw 记录原始值，使用原始值消息表示，kind 记录类型，使用类型表示。
func requirePetFields(raw json.RawMessage, kind reflect.Type) error {
	if kind.Kind() == reflect.Pointer {
		if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			return nil
		}
		return requirePetFields(raw, kind.Elem())
	}
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return errors.New("宠物合同不得缺省为null")
	}
	switch kind.Kind() {
	case reflect.Struct:
		// fields 记录字段集合，使用按文本索引的原始值消息映射表示。
		var fields map[string]json.RawMessage
		if json.Unmarshal(raw, &fields) != nil || fields == nil {
			return errors.New("宠物合同须为对象")
		}
		// known 初始化用作已知状态的按文本索引的布尔状态映射。
		known := map[string]bool{}
		// i 标记循环或列表中的当前位置。
		for i := 0; i < kind.NumField(); i++ {
			// field 接收字段供后续步骤使用。
			field := kind.Field(i)
			if field.PkgPath != "" {
				continue
			}
			// name 保存从拆分取出的名称。
			name := strings.Split(field.Tag.Get("json"), ",")[0]
			if name == "-" {
				continue
			}
			if name == "" {
				name = field.Name
			}
			if kind == reflect.TypeOf(Fighter{}) && (field.Name == "LuckAttackPPM" || field.Name == "LuckResistancePPM") {
				continue // 这两个值来自运行态投影，不能成为宠物基础模板字段。
			}
			known[name] = true
			// value 保存从字段集合取出的值，exists 保存从字段集合取出的存在。
			value, exists := fields[name]
			if !exists && kind == reflect.TypeOf(Skill{}) && strings.Contains(field.Tag.Get("json"), ",omitempty") {
				continue // 技能的独占子合同按behavior选择，其余基础字段仍须完整。
			}
			if !exists && kind == reflect.TypeOf(Item{}) && field.Name == "RequiredLevel" {
				continue // 装备需求等级由rule19装备投影验证；召唤技能书不借此补造需求。
			}
			if !exists && kind == reflect.TypeOf(Item{}) && slices.Contains(equipmentStatFields[:], name) {
				continue // 旧17非装备原书可缺新装备字段；rule22实际装备仍由presence验证，存在null仍拒。
			}
			if !exists && kind == reflect.TypeOf(Item{}) && (field.Name == "OriginalDurabilityU" || field.Name == "RepairBasePrice") {
				continue // 历史召唤书可以省略后来新增的装备维修字段。
			}
			// rule16历史定义尚无召回名；rule17由Fixture版本校验要求该字段非空。
			if !exists && kind == reflect.TypeOf(PetRuntimeDefinition{}) && field.Name == "RecallName" {
				continue
			}
			if !exists {
				return errors.New("宠物合同缺少字段：" + name)
			}
			// err 接收“require”宠物字段集合过程中可能产生的错误，供后续失败分支处理。
			if err := requirePetFields(value, field.Type); err != nil {
				return err
			}
		}
		// name 接收遍历字段集合时当前的元素。
		for name := range fields {
			if !known[name] {
				return errors.New("宠物合同出现未知字段：" + name)
			}
		}
	case reflect.Slice:
		// values 记录值集合，使用原始值消息列表表示。
		var values []json.RawMessage
		// err 接收反序列化过程中可能产生的错误，供后续失败分支处理。
		if err := json.Unmarshal(raw, &values); err != nil {
			return err
		}
		// value 接收遍历值集合时当前的元素。
		for _, value := range values {
			// err 接收“require”宠物字段集合过程中可能产生的错误，供后续失败分支处理。
			if err := requirePetFields(value, kind.Elem()); err != nil {
				return err
			}
		}
	}
	return nil
}

// decodePetContract 解码宠物“contract”。
// data 携带供当前流程读取或解析的原始字节，target 记录目标，使用任意类型的数据表示。
func decodePetContract(data []byte, target any) error {
	// err 接收“require”宠物字段集合过程中可能产生的错误，供后续失败分支处理。
	if err := requirePetFields(data, reflect.TypeOf(target).Elem()); err != nil {
		return err
	}
	// decoder 保存由创建“decoder”取得的“decoder”。
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	// err 接收解码过程中可能产生的错误，供后续失败分支处理。
	if err := decoder.Decode(target); err != nil {
		return err
	}
	// err 接收解码过程中可能产生的错误，供后续失败分支处理。
	if err := decoder.Decode(new(any)); err != io.EOF {
		return errors.New("宠物合同后有多余内容")
	}
	return nil
}

// UnmarshalJSON 执行反序列化JSON，并把失败交给调用方处理。
// r 是调用该方法的宠物运行时规则集合实例，data 携带供当前流程读取或解析的原始字节。
func (r *PetRuntimeRules) UnmarshalJSON(data []byte) error {
	// plain 描述“plain”。
	type plain PetRuntimeRules
	// value 记录值，使用“plain”表示。
	var value plain
	// err 接收解码宠物规则合同过程中可能产生的错误，供后续失败分支处理。
	if err := decodePetContract(data, &value); err != nil {
		return err
	}
	*r = PetRuntimeRules(value)
	return nil
}

// LoadPetRuntimeCandidates 加载宠物运行时“candidates”。
// path 记录路径，使用文本表示。
func LoadPetRuntimeCandidates(path string) (*PetRuntimeCandidateCatalog, error) {
	// data 保存由读取文件取得的数据，err 接收读取文件过程中可能产生的错误，供后续失败分支处理。
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	// catalog 记录目录，使用宠物运行时候选目录表示。
	var catalog PetRuntimeCandidateCatalog
	// err 接收解码宠物规则合同过程中可能产生的错误，供后续失败分支处理。
	if err := decodePetContract(data, &catalog); err != nil {
		return nil, err
	}
	if (catalog.SchemaVersion != 1 && catalog.SchemaVersion != 2) || !catalog.DevelopmentOnly || catalog.RuntimeEnabled || len(catalog.SourceEvidence) == 0 {
		return nil, errors.New("独立宠物候选不能自动启用运行")
	}
	if catalog.SchemaVersion == 1 {
		// old 记录原状态，使用临时结构表示。
		var old struct {
			// Definitions 记录定义集合，使用按文本索引的原始值消息映射列表表示。
			Definitions []map[string]json.RawMessage `json:"definitions"`
		}
		// err 接收反序列化过程中可能产生的错误，供后续失败分支处理。
		if err := json.Unmarshal(data, &old); err != nil {
			return nil, err
		}
		// d 接收遍历定义集合时当前的元素。
		for _, d := range old.Definitions {
			// provided 保存从d取出的“provided”。
			if _, provided := d["recall_name"]; provided {
				return nil, errors.New("旧候选目录不能夹带召回名，即使其值为空或null")
			}
		}
	}
	// d 接收遍历定义集合时当前的元素。
	for _, d := range catalog.Definitions {
		if catalog.SchemaVersion == 1 && d.RecallName != "" || catalog.SchemaVersion == 2 && !validRecallName(d.RecallName) {
			return nil, errors.New("独立宠物候选的召回名称与目录版本不匹配")
		}
	}
	// err 接收校验宠物定义集合过程中可能产生的错误，供后续失败分支处理。
	if err := validatePetDefinitions(catalog.Budgets, catalog.Definitions); err != nil {
		return nil, err
	}
	return &catalog, nil
}

// validatePetDefinitions 校验宠物定义集合。
// b 记录b，使用宠物运行时“budgets”表示，definitions 记录定义集合，使用宠物运行时定义列表表示。
func validatePetDefinitions(b PetRuntimeBudgets, definitions []PetRuntimeDefinition) error {
	if b.RetainedBodyCapacityPerOwner < 1 || b.RetainedBodyCapacityPerOwner > 128 || b.FullPolicy != "reject_birth_without_discarding_corpses" || len(definitions) == 0 || len(definitions) > 256 {
		return errors.New("宠物身体预算或满额策略不完整")
	}
	// ids 初始化用作标识集合的按数值索引的布尔状态映射，bodies 建立带初始值的按数值索引的布尔状态映射，供本段继续处理。
	ids, bodies := map[uint32]bool{}, map[uint32]bool{}
	// definition 接收遍历定义集合时当前的元素。
	for _, definition := range definitions {
		if ids[definition.ID] || bodies[definition.Base.DefinitionID] {
			return errors.New("宠物模板身份重复")
		}
		ids[definition.ID], bodies[definition.Base.DefinitionID] = true, true
		// err 接收校验过程中可能产生的错误，供后续失败分支处理。
		if err := definition.Validate(); err != nil {
			return err
		}
	}
	return nil
}

// Validate 校验当前请求或状态，并把结果交给调用方。
// d 是调用该方法的宠物运行时定义实例。
func (d PetRuntimeDefinition) Validate() error {
	if d.ID == 0 || d.SourceSkillID == 0 || d.MonsterTemplateID == 0 || d.OwnerJobID > 2 || d.Base.JobID != d.OwnerJobID || d.Base.DefinitionID == 0 || d.ResourceKey == "" || strings.Contains(d.ResourceKey, "..") || d.Base.HP <= 0 || d.Base.MP < 0 || d.Base.AttackMin < 0 || d.Base.AttackMax < d.Base.AttackMin || d.Base.DefenceMin < 0 || d.Base.DefenceMax < d.Base.DefenceMin || d.Base.MagicDefenceMin < 0 || d.Base.MagicDefenceMax < d.Base.MagicDefenceMin {
		return errors.New("宠物基础模板身份或属性无效")
	}
	if d.Base.Level == 0 || d.Base.MagicMin < 0 || d.Base.MagicMax < d.Base.MagicMin || d.Base.TaoMin < 0 || d.Base.TaoMax < d.Base.TaoMin || d.Base.Accuracy < 0 || d.Base.Agility < 0 || d.Base.MagicResist < 0 || d.Base.MagicResistWeight <= 0 || d.Base.MagicResist > d.Base.MagicResistWeight {
		return errors.New("宠物基础等级、法道属性或抗性无效")
	}
	// t 保存训练，用于当前步骤中的t。
	t := d.Training
	if len(t.InitialLevelByRank) != 4 || len(t.MaximumLevelByRank) != 4 || t.ExperienceMultiplier == 0 || t.OwnerRangeCells < 0 || t.OwnerRangeCells > 65535 || t.ExperienceBasis != "owner_actual_granted_experience" || t.GrantPhase != "confirmed_owner_experience" || t.Eligibility != "alive_same_world_within_owner_range" || t.MaxLevelUpsPerGrant != 1 || t.ExcessExperience != "keep" || t.AtMaximum != "do_not_accumulate" || t.CurrentHPOnGrowth != "keep" {
		return errors.New("宠物训练资格/刷新规则不完整")
	}
	if len(d.Growth) == 0 || len(d.Growth) > 256 {
		return errors.New("宠物逐级成长表缺失或过大")
	}
	// maximum 保存由无符号整数“32”取得的最大值。
	maximum := uint32(0)
	// i 定位最大值等级“by”排序位次中正在处理的成员，level 接收遍历最大值等级“by”排序位次时当前的元素。
	for i, level := range t.MaximumLevelByRank {
		if t.InitialLevelByRank[i] > level {
			return errors.New("宠物初级超过出生上限")
		}
		if level > maximum {
			maximum = level
		}
	}
	if int(maximum)+1 != len(d.Growth) {
		return errors.New("宠物成长必须覆盖全部出生上限")
	}
	// i 定位成长中正在处理的成员，growth 接收遍历成长时当前的元素。
	for i, growth := range d.Growth {
		if growth.Level != uint32(i) || (i+1 == len(d.Growth)) != (growth.RequiredExperience == nil) || growth.RequiredExperience != nil && *growth.RequiredExperience == 0 {
			return errors.New("宠物逐级阈值必须连续、明确且只有终态可空")
		}
		// ok 表明查询、校验或类型转换是否成功。
		if _, ok := d.FighterAt(growth.Level, maximum); !ok {
			return errors.New("宠物成长属性或速度溢出")
		}
	}
	// s 保存当前处理所用的速度。
	s := d.Speed
	if s.Basis != "maximum_training_level" || s.MoveReductionMSPerLevel < 0 || s.AttackReductionMSPerLevel < 0 || s.MinimumMoveIntervalMS <= 0 || s.MinimumAttackIntervalMS <= 0 || d.Base.MoveMS <= 0 || d.Base.AttackMS <= 0 || s.MinimumMoveIntervalMS > math.MaxUint32 || s.MinimumAttackIntervalMS > math.MaxUint32 || d.Base.MoveMS > math.MaxUint32 || d.Base.AttackMS > math.MaxUint32 {
		return errors.New("宠物动作间隔合同无效")
	}
	if d.Lifecycle.CorpseTTLMS <= 0 || d.Lifecycle.CorpseTTLMS > 86400000 || d.Lifecycle.SpawnReadyMS < 0 || d.Lifecycle.SpawnReadyMS > 86400000 || d.Lifecycle.LiveExpiry != "no_tame_expiry" || d.Lifecycle.ColdEnter != "do_not_restore_temporary_pet" {
		return errors.New("宠物生命/尸体期限不完整")
	}
	return nil
}

// Clone 复制当前请求或状态，并把结果交给调用方。
// d 是调用该方法的宠物运行时定义实例。
func (d PetRuntimeDefinition) Clone() PetRuntimeDefinition {
	d.Training.InitialLevelByRank = append([]uint32(nil), d.Training.InitialLevelByRank...)
	d.Training.MaximumLevelByRank = append([]uint32(nil), d.Training.MaximumLevelByRank...)
	d.Growth = append([]PetGrowthLevel(nil), d.Growth...)
	// i 标记遍历成长时访问的下标。
	for i := range d.Growth {
		if d.Growth[i].RequiredExperience != nil {
			// value 保存当前处理所用的值。
			value := *d.Growth[i].RequiredExperience
			d.Growth[i].RequiredExperience = &value
		}
	}
	return d
}

// 训练资格与自动Recall均引用同一个Globals.DataRange，唯一Excel只保存一次数值。
// d 是调用该方法的宠物运行时定义实例。
func (d PetRuntimeDefinition) SourceDataRangeCells() int {
	return d.Training.OwnerRangeCells
}

// 训练级与模板Level始终分开；速度减量取出生时冻结的最大训练级。
// d 是调用该方法的宠物运行时定义实例，level 记录等级，使用数值表示，maximum 记录最大值，使用数值表示。
func (d PetRuntimeDefinition) FighterAt(level, maximum uint32) (Fighter, bool) {
	if level > maximum || uint64(maximum) >= uint64(len(d.Growth)) || uint64(level) >= uint64(len(d.Growth)) {
		return Fighter{}, false
	}
	// f 保存当前处理所用的基础值。
	f := d.Base
	// g 保存从成长取出的g。
	g := d.Growth[level]
	// pair 接收遍历临时结构列表时当前的元素。
	for _, pair := range []struct {
		// target 记录目标，使用数值表示。
		target *int64
		// bonus 记录加成，使用数值表示。
		bonus int64
	}{{&f.HP, g.HPBonus}, {&f.AttackMin, g.AttackMinBonus}, {&f.AttackMax, g.AttackMaxBonus}, {&f.DefenceMin, g.DefenceMinBonus}, {&f.DefenceMax, g.DefenceMaxBonus}, {&f.MagicDefenceMin, g.MagicDefenceMinBonus}, {&f.MagicDefenceMax, g.MagicDefenceMaxBonus}} {
		if pair.bonus < 0 || *pair.target < 0 || *pair.target > math.MaxInt64-pair.bonus {
			return Fighter{}, false
		}
		*pair.target += pair.bonus
	}
	// interval 封装周期对应的后续处理，base 记录基础值，使用数值表示，reduction 记录“reduction”，使用数值表示。
	// minimum 记录“minimum”，使用数值表示。
	interval := func(base, reduction, minimum int64) (int64, bool) {
		if base <= 0 || reduction < 0 || minimum <= 0 {
			return 0, false
		}
		if reduction > 0 && uint64(maximum) > uint64(base/reduction) {
			return minimum, true
		}
		// value 保存根据基础值与整数“64”与“reduction”的计算结果计算的值。
		value := base - int64(maximum)*reduction
		if value < minimum {
			value = minimum
		}
		return value, true
	}
	// ok 表明查询、校验或类型转换是否成功。
	var ok bool
	if f.MoveMS, ok = interval(f.MoveMS, d.Speed.MoveReductionMSPerLevel, d.Speed.MinimumMoveIntervalMS); !ok {
		return Fighter{}, false
	}
	if f.AttackMS, ok = interval(f.AttackMS, d.Speed.AttackReductionMSPerLevel, d.Speed.MinimumAttackIntervalMS); !ok {
		return Fighter{}, false
	}
	f.Defence = f.DefenceMax
	return f, f.AttackMax >= f.AttackMin && f.DefenceMax >= f.DefenceMin && f.MagicDefenceMax >= f.MagicDefenceMin
}

// DefinitionByID 处理定义“by”标识。
// r 是调用该方法的宠物运行时规则集合实例，id 区分当前步骤指向的目标对象。
func (r *PetRuntimeRules) DefinitionByID(id uint32) (PetRuntimeDefinition, bool) {
	if r == nil {
		return PetRuntimeDefinition{}, false
	}
	// d 接收遍历定义集合时当前的元素。
	for _, d := range r.Definitions {
		if d.ID == id {
			return d.Clone(), true
		}
	}
	return PetRuntimeDefinition{}, false
}

// DefinitionByBodyID 处理定义“by”消息正文标识。
// r 是调用该方法的宠物运行时规则集合实例，id 区分当前步骤指向的目标对象。
func (r *PetRuntimeRules) DefinitionByBodyID(id uint32) (PetRuntimeDefinition, bool) {
	if r == nil {
		return PetRuntimeDefinition{}, false
	}
	// d 接收遍历定义集合时当前的元素。
	for _, d := range r.Definitions {
		if d.Base.DefinitionID == id {
			return d.Clone(), true
		}
	}
	return PetRuntimeDefinition{}, false
}

// ValidatePetRuntime 校验宠物运行时。
// f 是调用该方法的开发测试配置实例。
func (f *Fixture) ValidatePetRuntime() error {
	if f.RuleVersion < 16 {
		if f.PetRuntime != nil {
			return errors.New("宠物生命周期合同须使用rule16及后续版本")
		}
		return nil
	}
	// r 保存当前处理所用的宠物运行时。
	r := f.PetRuntime
	if r == nil || !r.Enabled || (r.BootstrapMode != "legacy_untrained_pet" && r.BootstrapMode != "none") {
		return errors.New("rule16须显式提供宠物身体/训练合同及bootstrap模式")
	}
	// err 接收校验宠物定义集合过程中可能产生的错误，供后续失败分支处理。
	if err := validatePetDefinitions(r.Budgets, r.Definitions); err != nil {
		return err
	}
	if r.BootstrapMode == "legacy_untrained_pet" {
		// definition 保存由定义“by”消息正文标识取得的定义，exists 保存由定义“by”消息正文标识取得的存在。
		definition, exists := r.DefinitionByBodyID(f.Pet.DefinitionID)
		if !exists || definition.OwnerJobID != f.Pet.JobID {
			return errors.New("旧开发宠物必须匹配唯一来源模板")
		}
		// base 保存由序列化取得的基础值，baseErr 保存由序列化取得的基础值错误。
		base, baseErr := json.Marshal(definition.Base)
		// legacy 保存由序列化取得的兼容旧数据，legacyErr 保存由序列化取得的兼容旧数据错误。
		legacy, legacyErr := json.Marshal(f.Pet)
		if baseErr != nil || legacyErr != nil || !bytes.Equal(base, legacy) {
			return errors.New("旧开发宠物与来源继承属性不一致")
		}
	}
	return nil
}

// validatePetRuntimePresence 校验宠物运行时“presence”。
// data 携带供当前流程读取或解析的原始字节。
func validatePetRuntimePresence(data []byte) error {
	// raw 记录原始值，使用按文本索引的原始值消息映射表示。
	var raw map[string]json.RawMessage
	// err 接收反序列化过程中可能产生的错误，供后续失败分支处理。
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	// value 保存从原始值取出的值，exists 保存从原始值取出的存在。
	if value, exists := raw["pet_runtime"]; exists && bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
		return errors.New("宠物生命周期字段不得以null冒充未提供")
	}
	return nil
}
