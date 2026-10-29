package devconfig

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"math"
	"os"
	"reflect"
	"strings"
)

// 独立候选文件与实际profile选择分开；数值均由唯一Excel编译产生。
type PetRuntimeCandidateCatalog struct {
	SchemaVersion   int                        `json:"schema_version"`
	DevelopmentOnly bool                       `json:"development_only"`
	RuntimeEnabled  bool                       `json:"runtime_enabled"`
	Budgets         PetRuntimeBudgets          `json:"budgets"`
	Definitions     []PetRuntimeDefinition     `json:"definitions"`
	SourceEvidence  map[string]json.RawMessage `json:"source_evidence"`
}

type PetRuntimeRules struct {
	Enabled       bool                   `json:"enabled"`
	BootstrapMode string                 `json:"bootstrap_mode"`
	Budgets       PetRuntimeBudgets      `json:"budgets"`
	Definitions   []PetRuntimeDefinition `json:"definitions"`
}

type PetRuntimeBudgets struct {
	RetainedBodyCapacityPerOwner int    `json:"retained_body_capacity_per_owner"`
	FullPolicy                   string `json:"full_policy"`
}

type PetRuntimeDefinition struct {
	ID                uint32           `json:"id"`
	SourceSkillID     uint32           `json:"source_skill_id"`
	MonsterTemplateID uint32           `json:"monster_template_id"`
	OwnerJobID        uint32           `json:"owner_job_id"`
	ResourceKey       string           `json:"resource_key"`
	RecallName        string           `json:"recall_name,omitempty"`
	Base              Fighter          `json:"base"`
	Undead            bool             `json:"undead"`
	Training          PetTrainingRules `json:"training"`
	Speed             PetSpeedRules    `json:"speed"`
	Lifecycle         PetLifetimeRules `json:"lifecycle"`
	Growth            []PetGrowthLevel `json:"growth"`
}

type PetTrainingRules struct {
	InitialLevelByRank   []uint32 `json:"initial_level_by_rank"`
	MaximumLevelByRank   []uint32 `json:"maximum_level_by_rank"`
	ExperienceMultiplier uint64   `json:"experience_multiplier"`
	OwnerRangeCells      int      `json:"owner_range_cells"`
	ExperienceBasis      string   `json:"experience_basis"`
	GrantPhase           string   `json:"grant_phase"`
	Eligibility          string   `json:"eligibility"`
	MaxLevelUpsPerGrant  int      `json:"max_level_ups_per_grant"`
	ExcessExperience     string   `json:"excess_experience"`
	AtMaximum            string   `json:"at_maximum"`
	CurrentHPOnGrowth    string   `json:"current_hp_on_growth"`
}

type PetSpeedRules struct {
	Basis                     string `json:"basis"`
	MoveReductionMSPerLevel   int64  `json:"move_reduction_ms_per_level"`
	AttackReductionMSPerLevel int64  `json:"attack_reduction_ms_per_level"`
	MinimumMoveIntervalMS     int64  `json:"minimum_move_interval_ms"`
	MinimumAttackIntervalMS   int64  `json:"minimum_attack_interval_ms"`
}

type PetLifetimeRules struct {
	CorpseTTLMS  int64  `json:"corpse_ttl_ms"`
	SpawnReadyMS int64  `json:"spawn_ready_ms"`
	LiveExpiry   string `json:"live_expiry"`
	ColdEnter    string `json:"cold_enter"`
}

type PetGrowthLevel struct {
	Level                uint32  `json:"level"`
	HPBonus              int64   `json:"hp_bonus"`
	AttackMinBonus       int64   `json:"attack_min_bonus"`
	AttackMaxBonus       int64   `json:"attack_max_bonus"`
	DefenceMinBonus      int64   `json:"defence_min_bonus"`
	DefenceMaxBonus      int64   `json:"defence_max_bonus"`
	MagicDefenceMinBonus int64   `json:"magic_defence_min_bonus"`
	MagicDefenceMaxBonus int64   `json:"magic_defence_max_bonus"`
	RequiredExperience   *uint64 `json:"required_experience"`
}

// 这些合同不接受缺字段、null默认、未知键或以字符串冒充整数。
// 只有成长终态的required_experience允许显式null。
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
		var fields map[string]json.RawMessage
		if json.Unmarshal(raw, &fields) != nil || fields == nil {
			return errors.New("宠物合同须为对象")
		}
		known := map[string]bool{}
		for i := 0; i < kind.NumField(); i++ {
			field := kind.Field(i)
			if field.PkgPath != "" {
				continue
			}
			name := strings.Split(field.Tag.Get("json"), ",")[0]
			if name == "-" {
				continue
			}
			if name == "" {
				name = field.Name
			}
			known[name] = true
			value, exists := fields[name]
			if !exists && kind == reflect.TypeOf(Skill{}) && strings.Contains(field.Tag.Get("json"), ",omitempty") {
				continue // 技能的独占子合同按behavior选择，其余基础字段仍须完整。
			}
			if !exists && kind == reflect.TypeOf(Item{}) && field.Name == "RequiredLevel" {
				continue // 装备需求等级由rule19装备投影验证；召唤技能书不借此补造需求。
			}
			// rule16历史定义尚无召回名；rule17由Fixture版本校验要求该字段非空。
			if !exists && kind == reflect.TypeOf(PetRuntimeDefinition{}) && field.Name == "RecallName" {
				continue
			}
			if !exists {
				return errors.New("宠物合同缺少字段：" + name)
			}
			if err := requirePetFields(value, field.Type); err != nil {
				return err
			}
		}
		for name := range fields {
			if !known[name] {
				return errors.New("宠物合同出现未知字段：" + name)
			}
		}
	case reflect.Slice:
		var values []json.RawMessage
		if err := json.Unmarshal(raw, &values); err != nil {
			return err
		}
		for _, value := range values {
			if err := requirePetFields(value, kind.Elem()); err != nil {
				return err
			}
		}
	}
	return nil
}

func decodePetContract(data []byte, target any) error {
	if err := requirePetFields(data, reflect.TypeOf(target).Elem()); err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return errors.New("宠物合同后有多余内容")
	}
	return nil
}

func (r *PetRuntimeRules) UnmarshalJSON(data []byte) error {
	type plain PetRuntimeRules
	var value plain
	if err := decodePetContract(data, &value); err != nil {
		return err
	}
	*r = PetRuntimeRules(value)
	return nil
}

func LoadPetRuntimeCandidates(path string) (*PetRuntimeCandidateCatalog, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var catalog PetRuntimeCandidateCatalog
	if err := decodePetContract(data, &catalog); err != nil {
		return nil, err
	}
	if (catalog.SchemaVersion != 1 && catalog.SchemaVersion != 2) || !catalog.DevelopmentOnly || catalog.RuntimeEnabled || len(catalog.SourceEvidence) == 0 {
		return nil, errors.New("独立宠物候选不能自动启用运行")
	}
	if catalog.SchemaVersion == 1 {
		var old struct {
			Definitions []map[string]json.RawMessage `json:"definitions"`
		}
		if err := json.Unmarshal(data, &old); err != nil {
			return nil, err
		}
		for _, d := range old.Definitions {
			if _, provided := d["recall_name"]; provided {
				return nil, errors.New("旧候选目录不能夹带召回名，即使其值为空或null")
			}
		}
	}
	for _, d := range catalog.Definitions {
		if catalog.SchemaVersion == 1 && d.RecallName != "" || catalog.SchemaVersion == 2 && !validRecallName(d.RecallName) {
			return nil, errors.New("独立宠物候选的召回名称与目录版本不匹配")
		}
	}
	if err := validatePetDefinitions(catalog.Budgets, catalog.Definitions); err != nil {
		return nil, err
	}
	return &catalog, nil
}

func validatePetDefinitions(b PetRuntimeBudgets, definitions []PetRuntimeDefinition) error {
	if b.RetainedBodyCapacityPerOwner < 1 || b.RetainedBodyCapacityPerOwner > 128 || b.FullPolicy != "reject_birth_without_discarding_corpses" || len(definitions) == 0 || len(definitions) > 256 {
		return errors.New("宠物身体预算或满额策略不完整")
	}
	ids, bodies := map[uint32]bool{}, map[uint32]bool{}
	for _, definition := range definitions {
		if ids[definition.ID] || bodies[definition.Base.DefinitionID] {
			return errors.New("宠物模板身份重复")
		}
		ids[definition.ID], bodies[definition.Base.DefinitionID] = true, true
		if err := definition.Validate(); err != nil {
			return err
		}
	}
	return nil
}

func (d PetRuntimeDefinition) Validate() error {
	if d.ID == 0 || d.SourceSkillID == 0 || d.MonsterTemplateID == 0 || d.OwnerJobID > 2 || d.Base.JobID != d.OwnerJobID || d.Base.DefinitionID == 0 || d.ResourceKey == "" || strings.Contains(d.ResourceKey, "..") || d.Base.HP <= 0 || d.Base.MP < 0 || d.Base.AttackMin < 0 || d.Base.AttackMax < d.Base.AttackMin || d.Base.DefenceMin < 0 || d.Base.DefenceMax < d.Base.DefenceMin || d.Base.MagicDefenceMin < 0 || d.Base.MagicDefenceMax < d.Base.MagicDefenceMin {
		return errors.New("宠物基础模板身份或属性无效")
	}
	if d.Base.Level == 0 || d.Base.MagicMin < 0 || d.Base.MagicMax < d.Base.MagicMin || d.Base.TaoMin < 0 || d.Base.TaoMax < d.Base.TaoMin || d.Base.Accuracy < 0 || d.Base.Agility < 0 || d.Base.MagicResist < 0 || d.Base.MagicResistWeight <= 0 || d.Base.MagicResist > d.Base.MagicResistWeight {
		return errors.New("宠物基础等级、法道属性或抗性无效")
	}
	t := d.Training
	if len(t.InitialLevelByRank) != 4 || len(t.MaximumLevelByRank) != 4 || t.ExperienceMultiplier == 0 || t.OwnerRangeCells < 0 || t.OwnerRangeCells > 65535 || t.ExperienceBasis != "owner_actual_granted_experience" || t.GrantPhase != "confirmed_owner_experience" || t.Eligibility != "alive_same_world_within_owner_range" || t.MaxLevelUpsPerGrant != 1 || t.ExcessExperience != "keep" || t.AtMaximum != "do_not_accumulate" || t.CurrentHPOnGrowth != "keep" {
		return errors.New("宠物训练资格/刷新规则不完整")
	}
	if len(d.Growth) == 0 || len(d.Growth) > 256 {
		return errors.New("宠物逐级成长表缺失或过大")
	}
	maximum := uint32(0)
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
	for i, growth := range d.Growth {
		if growth.Level != uint32(i) || (i+1 == len(d.Growth)) != (growth.RequiredExperience == nil) || growth.RequiredExperience != nil && *growth.RequiredExperience == 0 {
			return errors.New("宠物逐级阈值必须连续、明确且只有终态可空")
		}
		if _, ok := d.FighterAt(growth.Level, maximum); !ok {
			return errors.New("宠物成长属性或速度溢出")
		}
	}
	s := d.Speed
	if s.Basis != "maximum_training_level" || s.MoveReductionMSPerLevel < 0 || s.AttackReductionMSPerLevel < 0 || s.MinimumMoveIntervalMS <= 0 || s.MinimumAttackIntervalMS <= 0 || d.Base.MoveMS <= 0 || d.Base.AttackMS <= 0 || s.MinimumMoveIntervalMS > math.MaxUint32 || s.MinimumAttackIntervalMS > math.MaxUint32 || d.Base.MoveMS > math.MaxUint32 || d.Base.AttackMS > math.MaxUint32 {
		return errors.New("宠物动作间隔合同无效")
	}
	if d.Lifecycle.CorpseTTLMS <= 0 || d.Lifecycle.CorpseTTLMS > 86400000 || d.Lifecycle.SpawnReadyMS < 0 || d.Lifecycle.SpawnReadyMS > 86400000 || d.Lifecycle.LiveExpiry != "no_tame_expiry" || d.Lifecycle.ColdEnter != "do_not_restore_temporary_pet" {
		return errors.New("宠物生命/尸体期限不完整")
	}
	return nil
}

func (d PetRuntimeDefinition) Clone() PetRuntimeDefinition {
	d.Training.InitialLevelByRank = append([]uint32(nil), d.Training.InitialLevelByRank...)
	d.Training.MaximumLevelByRank = append([]uint32(nil), d.Training.MaximumLevelByRank...)
	d.Growth = append([]PetGrowthLevel(nil), d.Growth...)
	for i := range d.Growth {
		if d.Growth[i].RequiredExperience != nil {
			value := *d.Growth[i].RequiredExperience
			d.Growth[i].RequiredExperience = &value
		}
	}
	return d
}

// 训练资格与自动Recall均引用同一个Globals.DataRange，唯一Excel只保存一次数值。
func (d PetRuntimeDefinition) SourceDataRangeCells() int {
	return d.Training.OwnerRangeCells
}

// 训练级与模板Level始终分开；速度减量取出生时冻结的最大训练级。
func (d PetRuntimeDefinition) FighterAt(level, maximum uint32) (Fighter, bool) {
	if level > maximum || uint64(maximum) >= uint64(len(d.Growth)) || uint64(level) >= uint64(len(d.Growth)) {
		return Fighter{}, false
	}
	f := d.Base
	g := d.Growth[level]
	for _, pair := range []struct {
		target *int64
		bonus  int64
	}{{&f.HP, g.HPBonus}, {&f.AttackMin, g.AttackMinBonus}, {&f.AttackMax, g.AttackMaxBonus}, {&f.DefenceMin, g.DefenceMinBonus}, {&f.DefenceMax, g.DefenceMaxBonus}, {&f.MagicDefenceMin, g.MagicDefenceMinBonus}, {&f.MagicDefenceMax, g.MagicDefenceMaxBonus}} {
		if pair.bonus < 0 || *pair.target < 0 || *pair.target > math.MaxInt64-pair.bonus {
			return Fighter{}, false
		}
		*pair.target += pair.bonus
	}
	interval := func(base, reduction, minimum int64) (int64, bool) {
		if base <= 0 || reduction < 0 || minimum <= 0 {
			return 0, false
		}
		if reduction > 0 && uint64(maximum) > uint64(base/reduction) {
			return minimum, true
		}
		value := base - int64(maximum)*reduction
		if value < minimum {
			value = minimum
		}
		return value, true
	}
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

func (r *PetRuntimeRules) DefinitionByID(id uint32) (PetRuntimeDefinition, bool) {
	if r == nil {
		return PetRuntimeDefinition{}, false
	}
	for _, d := range r.Definitions {
		if d.ID == id {
			return d.Clone(), true
		}
	}
	return PetRuntimeDefinition{}, false
}

func (r *PetRuntimeRules) DefinitionByBodyID(id uint32) (PetRuntimeDefinition, bool) {
	if r == nil {
		return PetRuntimeDefinition{}, false
	}
	for _, d := range r.Definitions {
		if d.Base.DefinitionID == id {
			return d.Clone(), true
		}
	}
	return PetRuntimeDefinition{}, false
}

func (f *Fixture) ValidatePetRuntime() error {
	if f.RuleVersion < 16 {
		if f.PetRuntime != nil {
			return errors.New("宠物生命周期合同须使用rule16及后续版本")
		}
		return nil
	}
	r := f.PetRuntime
	if r == nil || !r.Enabled || (r.BootstrapMode != "legacy_untrained_pet" && r.BootstrapMode != "none") {
		return errors.New("rule16须显式提供宠物身体/训练合同及bootstrap模式")
	}
	if err := validatePetDefinitions(r.Budgets, r.Definitions); err != nil {
		return err
	}
	if r.BootstrapMode == "legacy_untrained_pet" {
		definition, exists := r.DefinitionByBodyID(f.Pet.DefinitionID)
		if !exists || definition.OwnerJobID != f.Pet.JobID {
			return errors.New("旧开发宠物必须匹配唯一来源模板")
		}
		base, baseErr := json.Marshal(definition.Base)
		legacy, legacyErr := json.Marshal(f.Pet)
		if baseErr != nil || legacyErr != nil || !bytes.Equal(base, legacy) {
			return errors.New("旧开发宠物与来源继承属性不一致")
		}
	}
	return nil
}

func validatePetRuntimePresence(data []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	if value, exists := raw["pet_runtime"]; exists && bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
		return errors.New("宠物生命周期字段不得以null冒充未提供")
	}
	return nil
}
