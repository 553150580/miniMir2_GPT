package devconfig

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func petCandidatePath() string {
	return filepath.Join("..", "..", "..", "config", "development", "pet-runtime-candidates.json")
}

func sourcePetCandidates(t *testing.T) *PetRuntimeCandidateCatalog {
	t.Helper()
	catalog, err := LoadPetRuntimeCandidates(petCandidatePath())
	if err != nil {
		t.Fatal("必须先从唯一Excel生成明确候选，不能跳过或用代码默认：", err)
	}
	return catalog
}

func TestPetRuntimeCandidateSourceVectorsAndIndependentTemplateLevel(t *testing.T) {
	catalog := sourcePetCandidates(t)
	if catalog.RuntimeEnabled || catalog.Budgets.RetainedBodyCapacityPerOwner != 128 || len(catalog.Definitions) != 1 {
		t.Fatal("候选范围或名额不符")
	}
	d := catalog.Definitions[0]
	if d.SourceSkillID != 17 || d.OwnerJobID != 2 || d.Training.OwnerRangeCells != 16 || d.Training.ExperienceMultiplier != 3 {
		t.Fatal("稳定来源映射或训练资格不符")
	}
	for _, vector := range []struct {
		level, maximum                                 uint32
		hp, attackMin, attackMax, ac, moveMS, attackMS int64
	}{
		{0, 4, 60, 10, 20, 10, 480, 720},
		{1, 4, 80, 11, 21, 12, 480, 720},
		{3, 7, 120, 13, 23, 16, 400, 510},
		{7, 7, 200, 17, 27, 24, 400, 510},
	} {
		got, ok := d.FighterAt(vector.level, vector.maximum)
		if !ok || got.Level != 15 || got.HP != vector.hp || got.AttackMin != vector.attackMin || got.AttackMax != vector.attackMax || got.DefenceMin != vector.ac || got.DefenceMax != vector.ac || got.MagicDefenceMin != vector.ac || got.MagicDefenceMax != vector.ac || got.MoveMS != vector.moveMS || got.AttackMS != vector.attackMS {
			t.Fatalf("来源向量不符 level=%d max=%d: %+v", vector.level, vector.maximum, got)
		}
	}
	if _, ok := d.FighterAt(0, math.MaxUint32); ok {
		t.Fatal("未知出生上限不得默认为最快速度")
	}
	if _, ok := d.FighterAt(5, 4); ok {
		t.Fatal("训练超过该birth上限")
	}
}

func TestPetRuntimeCandidateRejectsMissingNullUnknownAndCoercedFields(t *testing.T) {
	sourcePetCandidates(t)
	data, err := os.ReadFile(petCandidatePath())
	if err != nil {
		t.Fatal(err)
	}
	cases := map[string]func(map[string]any){
		"missing_runtime_gate": func(m map[string]any) { delete(m, "runtime_enabled") },
		"active_candidate":     func(m map[string]any) { m["runtime_enabled"] = true },
		"non_development":      func(m map[string]any) { m["development_only"] = false },
		"unknown_root":         func(m map[string]any) { m["automatic_defaults"] = true },
		"null_budgets":         func(m map[string]any) { m["budgets"] = nil },
		"zero_capacity":        func(m map[string]any) { m["budgets"].(map[string]any)["retained_body_capacity_per_owner"] = 0 },
		"unsupported_capacity": func(m map[string]any) { m["budgets"].(map[string]any)["retained_body_capacity_per_owner"] = 129 },
		"discard_early_policy": func(m map[string]any) { m["budgets"].(map[string]any)["full_policy"] = "discard_oldest" },
		"missing_range":        func(m map[string]any) { delete(petJSONDefinition(m)["training"].(map[string]any), "owner_range_cells") },
		"string_range":         func(m map[string]any) { petJSONDefinition(m)["training"].(map[string]any)["owner_range_cells"] = "16" },
		"fractional_rank": func(m map[string]any) {
			petJSONDefinition(m)["training"].(map[string]any)["initial_level_by_rank"].([]any)[0] = 0.5
		},
		"while_leveling": func(m map[string]any) {
			petJSONDefinition(m)["training"].(map[string]any)["max_level_ups_per_grant"] = 2
		},
		"missing_can_push": func(m map[string]any) { delete(petJSONDefinition(m)["base"].(map[string]any), "can_push") },
		"null_can_push":    func(m map[string]any) { petJSONDefinition(m)["base"].(map[string]any)["can_push"] = nil },
		"unknown_base_key": func(m map[string]any) { petJSONDefinition(m)["base"].(map[string]any)["undead"] = true },
		"negative_magic":   func(m map[string]any) { petJSONDefinition(m)["base"].(map[string]any)["magic_min"] = -1 },
		"missing_growth":   func(m map[string]any) { delete(petJSONDefinition(m), "growth") },
		"missing_growth_threshold": func(m map[string]any) {
			delete(petJSONDefinition(m)["growth"].([]any)[0].(map[string]any), "required_experience")
		},
		"null_middle_threshold": func(m map[string]any) {
			petJSONDefinition(m)["growth"].([]any)[0].(map[string]any)["required_experience"] = nil
		},
		"nonterminal_final": func(m map[string]any) {
			petJSONDefinition(m)["growth"].([]any)[7].(map[string]any)["required_experience"] = 1
		},
		"no_evidence": func(m map[string]any) { m["source_evidence"] = map[string]any{} },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			var object map[string]any
			if err := json.Unmarshal(data, &object); err != nil {
				t.Fatal(err)
			}
			mutate(object)
			encoded, err := json.Marshal(object)
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(t.TempDir(), "candidate.json")
			if err := os.WriteFile(path, encoded, 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := LoadPetRuntimeCandidates(path); err == nil {
				t.Fatal("缺项、未知或非来源合同被当成可用候选")
			}
		})
	}
}

func petJSONDefinition(m map[string]any) map[string]any {
	return m["definitions"].([]any)[0].(map[string]any)
}

func TestPetRuntimeDefinitionsAreDeepFrozenAndOverflowChecked(t *testing.T) {
	d := sourcePetCandidates(t).Definitions[0]
	before, _ := json.Marshal(d)
	copy := d.Clone()
	copy.Training.InitialLevelByRank[0] = 99
	*copy.Growth[0].RequiredExperience = 999
	copy.Growth[1].HPBonus = 999
	after, _ := json.Marshal(d)
	if string(before) != string(after) {
		t.Fatal("birth快照仍共享候选数组/阈值指针")
	}
	bad := d.Clone()
	bad.Base.HP = math.MaxInt64 - 1
	if err := bad.Validate(); err == nil {
		t.Fatal("成长属性加法溢出未拒绝")
	}
	bad = d.Clone()
	bad.Growth[1].HPBonus = -1
	if err := bad.Validate(); err == nil {
		t.Fatal("负成长量未拒绝")
	}
}

func TestPetRuntimeVersionGateAndNullAreExplicit(t *testing.T) {
	d := sourcePetCandidates(t)
	rules := &PetRuntimeRules{Enabled: true, BootstrapMode: "none", Budgets: d.Budgets, Definitions: d.Definitions}
	old := Fixture{RuleVersion: 15, PetRuntime: rules}
	if err := old.ValidatePetRuntime(); err == nil {
		t.Fatal("旧规则混入新生命周期")
	}
	current := Fixture{RuleVersion: 16}
	if err := current.ValidatePetRuntime(); err == nil {
		t.Fatal("新版缺失规则仍可用")
	}
	current.PetRuntime = rules
	if err := current.ValidatePetRuntime(); err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{`{"rule_version":15,"pet_runtime":null}`, `{"rule_version":16,"pet_runtime":null}`} {
		var fixture Fixture
		if err := json.Unmarshal([]byte(raw), &fixture); err == nil {
			t.Fatal("null绕过了显式合同")
		}
	}
}

func TestPetRuntimeSelectedProfileUsesSameCandidateDefinitionsWithoutSummonActivation(t *testing.T) {
	catalog := sourcePetCandidates(t)
	cfg, err := Load(filepath.Join("..", "..", "..", "conf", "dev", "server.json"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Fixture.RuleVersion != 23 || cfg.Fixture.PetRuntime == nil || !cfg.Fixture.PetRuntime.Enabled || cfg.Fixture.PetRuntime.BootstrapMode != "legacy_untrained_pet" {
		t.Fatal("生命周期未选择，或免费bootstrap模式被暗改")
	}
	if _, found := cfg.Fixture.Skill(17); found || cfg.Fixture.CastingSupply.Enabled {
		t.Fatal("未获护符单位决定即启用召唤/耗材")
	}
	if !reflect.DeepEqual(cfg.Fixture.PetRuntime.Budgets, catalog.Budgets) || !reflect.DeepEqual(cfg.Fixture.PetRuntime.Definitions, catalog.Definitions) {
		t.Fatal("候选与主profile不是同一编译定义")
	}
}
