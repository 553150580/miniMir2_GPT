// 本文件核对宠物模板的来源向量、版本门禁和不可变副本，验证未知或溢出输入。

package devconfig

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// petCandidatePath 返回宠物候选路径供调用方使用。
func petCandidatePath() string {
	return filepath.Join("..", "..", "..", "config", "development", "pet-runtime-candidates.json")
}

// sourcePetCandidates 返回来源宠物候选集合供调用方使用。
// t 提供断言失败报告、子用例和清理注册。
func sourcePetCandidates(t *testing.T) *PetRuntimeCandidateCatalog {
	t.Helper()
	// catalog 保存由加载宠物运行时候选集合取得的目录，err 接收加载宠物运行时候选集合过程中可能产生的错误，供后续失败分支处理。
	catalog, err := LoadPetRuntimeCandidates(petCandidatePath())
	if err != nil {
		t.Fatal("必须先从唯一Excel生成明确候选，不能跳过或用代码默认：", err)
	}
	return catalog
}

// TestPetRuntimeCandidateSourceVectorsAndIndependentTemplateLevel 核验未知出生上限不得默认为最快速度，用诊断「来源向量不符 level= max=」定位对应断言。
// t 提供断言失败报告、子用例和清理注册。
func TestPetRuntimeCandidateSourceVectorsAndIndependentTemplateLevel(t *testing.T) {
	// catalog 保存由来源宠物候选集合取得的目录。
	catalog := sourcePetCandidates(t)
	if catalog.RuntimeEnabled || catalog.Budgets.RetainedBodyCapacityPerOwner != 128 || len(catalog.Definitions) != 1 {
		t.Fatal("候选范围或名额不符")
	}
	// d 保存从定义集合取出的定义集合中的指定项。
	d := catalog.Definitions[0]
	if d.SourceSkillID != 17 || d.OwnerJobID != 2 || d.Training.OwnerRangeCells != 16 || d.Training.ExperienceMultiplier != 3 {
		t.Fatal("稳定来源映射或训练资格不符")
	}
	// vector 接收遍历临时结构列表时当前的元素。
	for _, vector := range []struct {
		// level 记录等级，使用数值表示。
		// maximum 记录最大值，使用数值表示。
		level, maximum uint32
		// hp 记录生命值，使用数值表示。
		// attackMin 记录攻击最小，使用数值表示。
		// attackMax 记录攻击最大，使用数值表示。
		// ac 记录“ac”，使用数值表示。
		// moveMS 记录移动毫秒，使用数值表示。
		// attackMS 记录攻击毫秒，使用数值表示。
		hp, attackMin, attackMax, ac, moveMS, attackMS int64
	}{
		{0, 4, 60, 10, 20, 10, 480, 720},
		{1, 4, 80, 11, 21, 12, 480, 720},
		{3, 7, 120, 13, 23, 16, 400, 510},
		{7, 7, 200, 17, 27, 24, 400, 510},
	} {
		// got 保存由战斗单位发生时间取得的实际结果，ok 表明查询、校验或类型转换是否成功。
		got, ok := d.FighterAt(vector.level, vector.maximum)
		if !ok || got.Level != 15 || got.HP != vector.hp || got.AttackMin != vector.attackMin || got.AttackMax != vector.attackMax || got.DefenceMin != vector.ac || got.DefenceMax != vector.ac || got.MagicDefenceMin != vector.ac || got.MagicDefenceMax != vector.ac || got.MoveMS != vector.moveMS || got.AttackMS != vector.attackMS {
			t.Fatalf("来源向量不符 level=%d max=%d: %+v", vector.level, vector.maximum, got)
		}
	}
	// ok 表明查询、校验或类型转换是否成功。
	if _, ok := d.FighterAt(0, math.MaxUint32); ok {
		t.Fatal("未知出生上限不得默认为最快速度")
	}
	// ok 表明查询、校验或类型转换是否成功。
	if _, ok := d.FighterAt(5, 4); ok {
		t.Fatal("训练超过该birth上限")
	}
}

// TestPetRuntimeCandidateRejectsMissingNullUnknownAndCoercedFields 核对「缺项、未知或非来源合同被当成可用候选」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestPetRuntimeCandidateRejectsMissingNullUnknownAndCoercedFields(t *testing.T) {
	sourcePetCandidates(t)
	// data 保存由读取文件取得的数据，err 接收读取文件过程中可能产生的错误，供后续失败分支处理。
	data, err := os.ReadFile(petCandidatePath())
	if err != nil {
		t.Fatal(err)
	}
	// cases 建立带初始值的按文本索引的供当前流程调用的函数映射，供本段继续处理。
	cases := map[string]func(map[string]any){
		// m 记录按文本索引的任意类型的数据映射。
		"missing_runtime_gate": func(m map[string]any) { delete(m, "runtime_enabled") },
		// m 记录按文本索引的任意类型的数据映射。
		"active_candidate": func(m map[string]any) { m["runtime_enabled"] = true },
		// m 记录按文本索引的任意类型的数据映射。
		"non_development": func(m map[string]any) { m["development_only"] = false },
		// m 记录按文本索引的任意类型的数据映射。
		"unknown_root": func(m map[string]any) { m["automatic_defaults"] = true },
		// m 记录按文本索引的任意类型的数据映射。
		"null_budgets": func(m map[string]any) { m["budgets"] = nil },
		// m 记录按文本索引的任意类型的数据映射。
		"zero_capacity": func(m map[string]any) { m["budgets"].(map[string]any)["retained_body_capacity_per_owner"] = 0 },
		// m 记录按文本索引的任意类型的数据映射。
		"unsupported_capacity": func(m map[string]any) { m["budgets"].(map[string]any)["retained_body_capacity_per_owner"] = 129 },
		// m 记录按文本索引的任意类型的数据映射。
		"discard_early_policy": func(m map[string]any) { m["budgets"].(map[string]any)["full_policy"] = "discard_oldest" },
		// m 记录按文本索引的任意类型的数据映射。
		"missing_range": func(m map[string]any) { delete(petJSONDefinition(m)["training"].(map[string]any), "owner_range_cells") },
		// m 记录按文本索引的任意类型的数据映射。
		"string_range": func(m map[string]any) { petJSONDefinition(m)["training"].(map[string]any)["owner_range_cells"] = "16" },
		// m 记录按文本索引的任意类型的数据映射。
		"fractional_rank": func(m map[string]any) {
			petJSONDefinition(m)["training"].(map[string]any)["initial_level_by_rank"].([]any)[0] = 0.5
		},
		// m 记录按文本索引的任意类型的数据映射。
		"while_leveling": func(m map[string]any) {
			petJSONDefinition(m)["training"].(map[string]any)["max_level_ups_per_grant"] = 2
		},
		// m 记录按文本索引的任意类型的数据映射。
		"missing_can_push": func(m map[string]any) { delete(petJSONDefinition(m)["base"].(map[string]any), "can_push") },
		// m 记录按文本索引的任意类型的数据映射。
		"null_can_push": func(m map[string]any) { petJSONDefinition(m)["base"].(map[string]any)["can_push"] = nil },
		// m 记录按文本索引的任意类型的数据映射。
		"unknown_base_key": func(m map[string]any) { petJSONDefinition(m)["base"].(map[string]any)["undead"] = true },
		// m 记录按文本索引的任意类型的数据映射。
		"negative_magic": func(m map[string]any) { petJSONDefinition(m)["base"].(map[string]any)["magic_min"] = -1 },
		// m 记录按文本索引的任意类型的数据映射。
		"missing_growth": func(m map[string]any) { delete(petJSONDefinition(m), "growth") },
		// m 记录按文本索引的任意类型的数据映射。
		"missing_growth_threshold": func(m map[string]any) {
			delete(petJSONDefinition(m)["growth"].([]any)[0].(map[string]any), "required_experience")
		},
		// m 记录按文本索引的任意类型的数据映射。
		"null_middle_threshold": func(m map[string]any) {
			petJSONDefinition(m)["growth"].([]any)[0].(map[string]any)["required_experience"] = nil
		},
		// m 记录按文本索引的任意类型的数据映射。
		"nonterminal_final": func(m map[string]any) {
			petJSONDefinition(m)["growth"].([]any)[7].(map[string]any)["required_experience"] = 1
		},
		// m 记录按文本索引的任意类型的数据映射。
		"no_evidence": func(m map[string]any) { m["source_evidence"] = map[string]any{} },
	}
	// name 定位“cases”中正在处理的成员，mutate 接收遍历“cases”时当前的元素。
	for name, mutate := range cases {
		// t 提供断言失败报告、子用例和清理注册。
		t.Run(name, func(t *testing.T) {
			// object 记录对象，使用按文本索引的任意类型的数据映射表示。
			var object map[string]any
			// err 接收反序列化过程中可能产生的错误，供后续失败分支处理。
			if err := json.Unmarshal(data, &object); err != nil {
				t.Fatal(err)
			}
			mutate(object)
			// encoded 保存由序列化取得的“encoded”，err 接收序列化过程中可能产生的错误，供后续失败分支处理。
			encoded, err := json.Marshal(object)
			if err != nil {
				t.Fatal(err)
			}
			// path 保存由加入取得的路径。
			path := filepath.Join(t.TempDir(), "candidate.json")
			// err 接收写入文件过程中可能产生的错误，供后续失败分支处理。
			if err := os.WriteFile(path, encoded, 0600); err != nil {
				t.Fatal(err)
			}
			// err 接收加载宠物运行时候选集合过程中可能产生的错误，供后续失败分支处理。
			if _, err := LoadPetRuntimeCandidates(path); err == nil {
				t.Fatal("缺项、未知或非来源合同被当成可用候选")
			}
		})
	}
}

// petJSONDefinition 返回宠物JSON定义供调用方使用。
// m 记录按文本索引的任意类型的数据映射。
func petJSONDefinition(m map[string]any) map[string]any {
	return m["definitions"].([]any)[0].(map[string]any)
}

// TestPetRuntimeDefinitionsAreDeepFrozenAndOverflowChecked 核对「birth快照仍共享候选数组/阈值指针」与「成长属性加法溢出未拒绝」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestPetRuntimeDefinitionsAreDeepFrozenAndOverflowChecked(t *testing.T) {
	// d 保存从定义集合取出的定义集合中的指定项。
	d := sourcePetCandidates(t).Definitions[0]
	// before 保存由序列化取得的操作前的状态。
	before, _ := json.Marshal(d)
	// copy 保存由副本取得的“copy”。
	copy := d.Clone()
	copy.Training.InitialLevelByRank[0] = 99
	*copy.Growth[0].RequiredExperience = 999
	copy.Growth[1].HPBonus = 999
	// after 保存由序列化取得的操作后的状态。
	after, _ := json.Marshal(d)
	if string(before) != string(after) {
		t.Fatal("birth快照仍共享候选数组/阈值指针")
	}
	// bad 保存由副本取得的“bad”。
	bad := d.Clone()
	bad.Base.HP = math.MaxInt64 - 1
	// err 接收校验过程中可能产生的错误，供后续失败分支处理。
	if err := bad.Validate(); err == nil {
		t.Fatal("成长属性加法溢出未拒绝")
	}
	bad = d.Clone()
	bad.Growth[1].HPBonus = -1
	// err 接收校验过程中可能产生的错误，供后续失败分支处理。
	if err := bad.Validate(); err == nil {
		t.Fatal("负成长量未拒绝")
	}
}

// TestPetRuntimeVersionGateAndNullAreExplicit 核对「null绕过了显式合同」与「旧规则混入新生命周期」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestPetRuntimeVersionGateAndNullAreExplicit(t *testing.T) {
	// d 接收来源宠物候选集合供后续步骤使用。
	d := sourcePetCandidates(t)
	// rules 保存当前处理所用的规则集合。
	rules := &PetRuntimeRules{Enabled: true, BootstrapMode: "none", Budgets: d.Budgets, Definitions: d.Definitions}
	// old 初始化用作原状态的开发测试配置。
	old := Fixture{RuleVersion: 15, PetRuntime: rules}
	// err 接收校验宠物运行时过程中可能产生的错误，供后续失败分支处理。
	if err := old.ValidatePetRuntime(); err == nil {
		t.Fatal("旧规则混入新生命周期")
	}
	// current 初始化用作当前状态的开发测试配置。
	current := Fixture{RuleVersion: 16}
	// err 接收校验宠物运行时过程中可能产生的错误，供后续失败分支处理。
	if err := current.ValidatePetRuntime(); err == nil {
		t.Fatal("新版缺失规则仍可用")
	}
	current.PetRuntime = rules
	// err 接收校验宠物运行时过程中可能产生的错误，供后续失败分支处理。
	if err := current.ValidatePetRuntime(); err != nil {
		t.Fatal(err)
	}
	// raw 接收遍历文本列表时当前的元素。
	for _, raw := range []string{`{"rule_version":15,"pet_runtime":null}`, `{"rule_version":16,"pet_runtime":null}`} {
		// fixture 记录开发测试配置。
		var fixture Fixture
		// err 接收反序列化过程中可能产生的错误，供后续失败分支处理。
		if err := json.Unmarshal([]byte(raw), &fixture); err == nil {
			t.Fatal("null绕过了显式合同")
		}
	}
}

// TestPetRuntimeSelectedProfileUsesSameCandidateDefinitionsWithoutSummonActivation 核对「生命周期未选择，或免费bootstrap模式被暗改」与「候选与主profile不是同一编译定义」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestPetRuntimeSelectedProfileUsesSameCandidateDefinitionsWithoutSummonActivation(t *testing.T) {
	// catalog 保存由来源宠物候选集合取得的目录。
	catalog := sourcePetCandidates(t)
	// cfg 保存由加载取得的服务配置，err 接收加载过程中可能产生的错误，供后续失败分支处理。
	cfg, err := Load(filepath.Join("..", "..", "..", "conf", "dev", "server.json"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Fixture.RuleVersion < 23 || cfg.Fixture.PetRuntime == nil || !cfg.Fixture.PetRuntime.Enabled || cfg.Fixture.PetRuntime.BootstrapMode != "legacy_untrained_pet" {
		t.Fatal("生命周期未选择，或免费bootstrap模式被暗改")
	}
	// found 保存由技能取得的已找到标记。
	if _, found := cfg.Fixture.Skill(17); found || cfg.Fixture.CastingSupply.Enabled {
		t.Fatal("未获护符单位决定即启用召唤/耗材")
	}
	if !reflect.DeepEqual(cfg.Fixture.PetRuntime.Budgets, catalog.Budgets) || !reflect.DeepEqual(cfg.Fixture.PetRuntime.Definitions, catalog.Definitions) {
		t.Fatal("候选与主profile不是同一编译定义")
	}
}
