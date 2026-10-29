package devconfig

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

const summonCandidatePath = "../../../config/development/summon-candidates.json"

func readSummonCandidate(t *testing.T) *SummonCandidateCatalog {
	t.Helper()
	c, err := LoadSummonCandidates(summonCandidatePath)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestSummonCandidatesPreserveSourceAndClosedProfile(t *testing.T) {
	c := readSummonCandidate(t)
	s := c.Skills[0]
	if s.ID != 17 || s.BookItemID != 25 || s.LearnLevel != 19 || s.DelayBaseMS != 500 || s.MaterialItemID != 187 || s.MaterialQuantity != 1 || s.MPBaseCost != 12 || s.MPLevelCost != 4 ||
		s.GlobalSpellMS != 1800 || s.CooldownBaseMS != 1800 || s.ActionMS != 600 || s.Summon.DefinitionID != 1 {
		t.Fatal("候选必须沿已登记来源，不可把16困魔咒当作17召唤")
	}
	if len(c.Upgrades) != 3 || !reflect.DeepEqual([]uint32{c.Upgrades[0].RequiredRoleLevel, c.Upgrades[1].RequiredRoleLevel, c.Upgrades[2].RequiredRoleLevel}, []uint32{19, 22, 26}) {
		t.Fatal("本作采用的Crystal逐阶候选不能被MYF差异或低资格替代")
	}
	cfg, err := Load("../../../conf/dev/server.json")
	if err != nil {
		t.Fatal(err)
	}
	f := cfg.Fixture
	if f.RuleVersion != 21 || len(f.Skills) != 22 || f.CastingSupply.Enabled || f.PetRuntime.BootstrapMode != "legacy_untrained_pet" {
		t.Fatal("候选载入不能开启主profile或更换免费开发宠物模式")
	}
	if _, ok := f.Skill(17); ok {
		t.Fatal("17尚未加入主profile")
	}
	if _, ok := f.Item(25); ok {
		t.Fatal("候选书不能变成隐形运行物品")
	}
	for _, d := range f.PetRuntime.Definitions {
		if d.RecallName != "BoneFamiliar" || d.SourceDataRangeCells() != 16 {
			t.Fatal("召回名与共用DataRange必须由唯一表显式导出")
		}
	}
	// 独立目录的技能、原书和升级可以显式合入隔离配置，仍通过完整校验。
	f.Skills = append(append([]Skill(nil), f.Skills...), c.Skills...)
	f.Items = append(append([]Item(nil), f.Items...), c.Books...)
	f.SkillUpgrades = append(append([]SkillUpgrade(nil), f.SkillUpgrades...), c.Upgrades...)
	if err := f.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestSummonCandidatesRejectIncompleteOrMisleadingContracts(t *testing.T) {
	data, err := os.ReadFile(summonCandidatePath)
	if err != nil {
		t.Fatal(err)
	}
	tests := map[string]func(map[string]any){
		"运行开启":     func(v map[string]any) { v["runtime_enabled"] = true },
		"降版伪装":     func(v map[string]any) { v["required_rule_version"] = 16 },
		"缺来源":      func(v map[string]any) { delete(v, "source_evidence") },
		"空来源":      func(v map[string]any) { v["source_evidence"] = map[string]any{} },
		"未知字段":     func(v map[string]any) { v["grant_free_pet"] = true },
		"错误技能":     func(v map[string]any) { firstSkill(v)["id"] = 16 },
		"空召唤对象":    func(v map[string]any) { firstSkill(v)["summon"] = nil },
		"缺模板":      func(v map[string]any) { delete(firstSkill(v)["summon"].(map[string]any), "definition_id") },
		"模板字符串":    func(v map[string]any) { firstSkill(v)["summon"].(map[string]any)["definition_id"] = "1" },
		"模板小数":     func(v map[string]any) { firstSkill(v)["summon"].(map[string]any)["definition_id"] = 1.5 },
		"未知模板字段":   func(v map[string]any) { firstSkill(v)["summon"].(map[string]any)["spawn_delay_ms"] = 1 },
		"忽略技能字段":   func(v map[string]any) { firstSkill(v)["unknown_skill_field"] = 1 },
		"缺基础字段":    func(v map[string]any) { delete(firstSkill(v), "mp_level_cost") },
		"基础字段null": func(v map[string]any) { firstSkill(v)["mp_level_cost"] = nil },
		"混入伤害":     func(v map[string]any) { firstSkill(v)["power_base"] = 1 },
		"未用倍率":     func(v map[string]any) { firstSkill(v)["multiplier_base_ppm"] = 1000000 },
		"混入自动":     func(v map[string]any) { firstSkill(v)["auto_enabled"] = true },
		"混入旁格":     func(v map[string]any) { firstSkill(v)["secondary_direction_offsets"] = []any{1} },
		"其它空合同":    func(v map[string]any) { firstSkill(v)["teleport"] = nil },
		"无材料":      func(v map[string]any) { firstSkill(v)["material_quantity"] = 0 },
		"模板不匹配":    func(v map[string]any) { v["pet_definition_ids"] = []any{2} },
		"书属别职业":    func(v map[string]any) { firstBook(v)["allowed_jobs"] = []any{1} },
		"书装备属性":    func(v map[string]any) { firstBook(v)["attack_max_bonus"] = 1 },
		"原书伪装堆叠":   func(v map[string]any) { firstBook(v)["stackable"] = true },
		"未知书字段":    func(v map[string]any) { firstBook(v)["free_recall"] = true },
		"缺升级阶":     func(v map[string]any) { v["upgrades"] = v["upgrades"].([]any)[:2] },
		"重复升级阶":    func(v map[string]any) { v["upgrades"].([]any)[1].(map[string]any)["level"] = 1 },
		"降低升级资格":   func(v map[string]any) { v["upgrades"].([]any)[0].(map[string]any)["required_role_level"] = 1 },
	}
	tests["少于目标阶加一原书"] = func(v map[string]any) {
		for _, u := range v["upgrades"].([]any) {
			u.(map[string]any)["book_costs"].([]any)[0].(map[string]any)["quantity"] = 1
		}
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			var v map[string]any
			if err := json.Unmarshal(data, &v); err != nil {
				t.Fatal(err)
			}
			mutate(v)
			body, err := json.Marshal(v)
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(t.TempDir(), "candidate.json")
			if err := os.WriteFile(path, body, 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := LoadSummonCandidates(path); err == nil {
				t.Fatal("不完整/跨行为/未启用候选被接受")
			}
		})
	}
}

func firstSkill(v map[string]any) map[string]any { return v["skills"].([]any)[0].(map[string]any) }
func firstBook(v map[string]any) map[string]any  { return v["books"].([]any)[0].(map[string]any) }

func TestNoPetsAndRecallNamesRequireExplicitVersionedPolicy(t *testing.T) {
	cfg, err := Load("../../../conf/dev/server.json")
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(cfg.Fixture)
	if err != nil {
		t.Fatal(err)
	}
	tests := map[string]func(map[string]any){
		"当前图缺禁宠": func(v map[string]any) { delete(v["map"].(map[string]any), "no_pets") },
		"城镇缺禁宠":  func(v map[string]any) { delete(v["maps"].([]any)[0].(map[string]any), "no_pets") },
		"禁宠null": func(v map[string]any) { v["map"].(map[string]any)["no_pets"] = nil },
		"旧版夹带":   func(v map[string]any) { v["rule_version"] = 16 },
		"缺召回名": func(v map[string]any) {
			delete(v["pet_runtime"].(map[string]any)["definitions"].([]any)[0].(map[string]any), "recall_name")
		},
		"空召回名": func(v map[string]any) {
			v["pet_runtime"].(map[string]any)["definitions"].([]any)[0].(map[string]any)["recall_name"] = ""
		},
		"隐形召回空格": func(v map[string]any) {
			v["pet_runtime"].(map[string]any)["definitions"].([]any)[0].(map[string]any)["recall_name"] = "BoneFamiliar "
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			var v map[string]any
			if err := json.Unmarshal(data, &v); err != nil {
				t.Fatal(err)
			}
			mutate(v)
			body, _ := json.Marshal(v)
			var f Fixture
			if err := json.Unmarshal(body, &f); err == nil {
				if err := f.ValidateSummoning(); err == nil {
					t.Fatal("缺少或降版的NoPets/召回名被当默认允许")
				}
			}
		})
	}
	// 历史16没有新字段；显式null也属于夹带，不能借nil绕过版本检查。
	for _, raw := range []string{`{"rule_version":16,"map":{"no_pets":null}}`, `{"rule_version":16,"skills":[{"summon":null}]}`} {
		var f Fixture
		if err := json.Unmarshal([]byte(raw), &f); err == nil && f.ValidateSummoning() == nil {
			t.Fatal("旧版夹带null召唤合同被接受")
		}
	}
	for _, raw := range []string{`{"rule_version":16,"pet_runtime":{"definitions":[{"recall_name":""}]}}`, `{"rule_version":16,"pet_runtime":{"definitions":[{"recall_name":null}]}}`} {
		var f Fixture
		if err := json.Unmarshal([]byte(raw), &f); err == nil {
			t.Fatal("历史JSON中的空召回名键没有在入口按presence拒绝")
		}
	}
}

func TestLegacyPetCandidateSchemaCannotHideEmptyRecallName(t *testing.T) {
	data, err := os.ReadFile(petCandidatePath())
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"absent", "empty", "null"} {
		t.Run(mode, func(t *testing.T) {
			var v map[string]any
			if err := json.Unmarshal(data, &v); err != nil {
				t.Fatal(err)
			}
			v["schema_version"] = 1
			for _, entry := range v["definitions"].([]any) {
				d := entry.(map[string]any)
				switch mode {
				case "absent":
					delete(d, "recall_name")
				case "empty":
					d["recall_name"] = ""
				case "null":
					d["recall_name"] = nil
				}
			}
			body, _ := json.Marshal(v)
			path := filepath.Join(t.TempDir(), "legacy.json")
			if err := os.WriteFile(path, body, 0600); err != nil {
				t.Fatal(err)
			}
			_, err := LoadPetRuntimeCandidates(path)
			if (mode == "absent") != (err == nil) {
				t.Fatalf("历史Name存在性校验错误: mode=%s err=%v", mode, err)
			}
		})
	}
}
