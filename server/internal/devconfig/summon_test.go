// 本文件验证候选召唤来源信息、宠物数量策略和关闭分支的显式合同。

package devconfig

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// summonCandidatePath 保存召唤候选路径所使用的文本。
const summonCandidatePath = "../../../config/development/summon-candidates.json"

// readSummonCandidate 读取“summon”“candidate”。
// t 提供断言失败报告、子用例和清理注册。
func readSummonCandidate(t *testing.T) *SummonCandidateCatalog {
	t.Helper()
	// c 接收加载召唤候选集合供后续步骤使用，err 接收加载召唤候选集合过程中可能产生的错误，供后续失败分支处理。
	c, err := LoadSummonCandidates(summonCandidatePath)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// TestSummonCandidatesPreserveSourceAndClosedProfile 核验候选必须沿已登记来源，不可把16困魔咒当作17召唤，以及召回名与共用DataRange必须由唯一表显式导出。
// t 提供断言失败报告、子用例和清理注册。
func TestSummonCandidatesPreserveSourceAndClosedProfile(t *testing.T) {
	// c 接收读取召唤候选供后续步骤使用。
	c := readSummonCandidate(t)
	// s 保存从技能集合取出的技能集合中的指定项。
	s := c.Skills[0]
	if s.ID != 17 || s.BookItemID != 25 || s.LearnLevel != 19 || s.DelayBaseMS != 500 || s.MaterialItemID != 187 || s.MaterialQuantity != 1 || s.MPBaseCost != 12 || s.MPLevelCost != 4 ||
		s.GlobalSpellMS != 1800 || s.CooldownBaseMS != 1800 || s.ActionMS != 600 || s.Summon.DefinitionID != 1 {
		t.Fatal("候选必须沿已登记来源，不可把16困魔咒当作17召唤")
	}
	if len(c.Upgrades) != 3 || !reflect.DeepEqual([]uint32{c.Upgrades[0].RequiredRoleLevel, c.Upgrades[1].RequiredRoleLevel, c.Upgrades[2].RequiredRoleLevel}, []uint32{19, 22, 26}) {
		t.Fatal("本作采用的Crystal逐阶候选不能被MYF差异或低资格替代")
	}
	// cfg 保存由加载取得的服务配置，err 接收加载过程中可能产生的错误，供后续失败分支处理。
	cfg, err := Load("../../../conf/dev/server.json")
	if err != nil {
		t.Fatal(err)
	}
	// f 保存当前处理所用的开发测试配置。
	f := cfg.Fixture
	expectedSkills := 22
	if f.RuleVersion >= 28 {
		expectedSkills = 24
	}
	if f.RuleVersion < 23 || len(f.Skills) != expectedSkills || f.CastingSupply.Enabled || f.PetRuntime.BootstrapMode != "legacy_untrained_pet" {
		t.Fatal("候选载入不能开启主profile或更换免费开发宠物模式")
	}
	// ok 表明查询、校验或类型转换是否成功。
	if _, ok := f.Skill(17); ok {
		t.Fatal("17尚未加入主profile")
	}
	// ok 表明查询、校验或类型转换是否成功。
	if _, ok := f.Item(25); ok {
		t.Fatal("候选书不能变成隐形运行物品")
	}
	// d 接收遍历定义集合时当前的元素。
	for _, d := range f.PetRuntime.Definitions {
		if d.RecallName != "BoneFamiliar" || d.SourceDataRangeCells() != 16 {
			t.Fatal("召回名与共用DataRange必须由唯一表显式导出")
		}
	}
	// 独立目录的技能、原书和升级可以显式合入隔离配置，仍通过完整校验。
	f.Skills = append(append([]Skill(nil), f.Skills...), c.Skills...)
	f.Items = append(append([]Item(nil), f.Items...), c.Books...)
	f.SkillUpgrades = append(append([]SkillUpgrade(nil), f.SkillUpgrades...), c.Upgrades...)
	// err 接收校验过程中可能产生的错误，供后续失败分支处理。
	if err := f.Validate(); err != nil {
		t.Fatal(err)
	}
}

// TestSummonCandidatesRejectIncompleteOrMisleadingContracts 核对「不完整/跨行为/未启用候选被接受」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestSummonCandidatesRejectIncompleteOrMisleadingContracts(t *testing.T) {
	// data 保存由读取文件取得的数据，err 接收读取文件过程中可能产生的错误，供后续失败分支处理。
	data, err := os.ReadFile(summonCandidatePath)
	if err != nil {
		t.Fatal(err)
	}
	// tests 初始化用作测试集合的按文本索引的供当前流程调用的函数映射。
	tests := map[string]func(map[string]any){
		// v 记录按文本索引的任意类型的数据映射。
		"运行开启": func(v map[string]any) { v["runtime_enabled"] = true },
		// v 记录按文本索引的任意类型的数据映射。
		"降版伪装": func(v map[string]any) { v["required_rule_version"] = 16 },
		// v 记录按文本索引的任意类型的数据映射。
		"缺来源": func(v map[string]any) { delete(v, "source_evidence") },
		// v 记录按文本索引的任意类型的数据映射。
		"空来源": func(v map[string]any) { v["source_evidence"] = map[string]any{} },
		// v 记录按文本索引的任意类型的数据映射。
		"未知字段": func(v map[string]any) { v["grant_free_pet"] = true },
		// v 记录按文本索引的任意类型的数据映射。
		"错误技能": func(v map[string]any) { firstSkill(v)["id"] = 16 },
		// v 记录按文本索引的任意类型的数据映射。
		"空召唤对象": func(v map[string]any) { firstSkill(v)["summon"] = nil },
		// v 记录按文本索引的任意类型的数据映射。
		"缺模板": func(v map[string]any) { delete(firstSkill(v)["summon"].(map[string]any), "definition_id") },
		// v 记录按文本索引的任意类型的数据映射。
		"模板字符串": func(v map[string]any) { firstSkill(v)["summon"].(map[string]any)["definition_id"] = "1" },
		// v 记录按文本索引的任意类型的数据映射。
		"模板小数": func(v map[string]any) { firstSkill(v)["summon"].(map[string]any)["definition_id"] = 1.5 },
		// v 记录按文本索引的任意类型的数据映射。
		"未知模板字段": func(v map[string]any) { firstSkill(v)["summon"].(map[string]any)["spawn_delay_ms"] = 1 },
		// v 记录按文本索引的任意类型的数据映射。
		"忽略技能字段": func(v map[string]any) { firstSkill(v)["unknown_skill_field"] = 1 },
		// v 记录按文本索引的任意类型的数据映射。
		"缺基础字段": func(v map[string]any) { delete(firstSkill(v), "mp_level_cost") },
		// v 记录按文本索引的任意类型的数据映射。
		"基础字段null": func(v map[string]any) { firstSkill(v)["mp_level_cost"] = nil },
		// v 记录按文本索引的任意类型的数据映射。
		"混入伤害": func(v map[string]any) { firstSkill(v)["power_base"] = 1 },
		// v 记录按文本索引的任意类型的数据映射。
		"未用倍率": func(v map[string]any) { firstSkill(v)["multiplier_base_ppm"] = 1000000 },
		// v 记录按文本索引的任意类型的数据映射。
		"混入自动": func(v map[string]any) { firstSkill(v)["auto_enabled"] = true },
		// v 记录按文本索引的任意类型的数据映射。
		"混入旁格": func(v map[string]any) { firstSkill(v)["secondary_direction_offsets"] = []any{1} },
		// v 记录按文本索引的任意类型的数据映射。
		"其它空合同": func(v map[string]any) { firstSkill(v)["teleport"] = nil },
		// v 记录按文本索引的任意类型的数据映射。
		"无材料": func(v map[string]any) { firstSkill(v)["material_quantity"] = 0 },
		// v 记录按文本索引的任意类型的数据映射。
		"模板不匹配": func(v map[string]any) { v["pet_definition_ids"] = []any{2} },
		// v 记录按文本索引的任意类型的数据映射。
		"书属别职业": func(v map[string]any) { firstBook(v)["allowed_jobs"] = []any{1} },
		// v 记录按文本索引的任意类型的数据映射。
		"书装备属性": func(v map[string]any) { firstBook(v)["attack_max_bonus"] = 1 },
		// v 记录按文本索引的任意类型的数据映射。
		"原书伪装堆叠": func(v map[string]any) { firstBook(v)["stackable"] = true },
		// v 记录按文本索引的任意类型的数据映射。
		"未知书字段": func(v map[string]any) { firstBook(v)["free_recall"] = true },
		// v 记录按文本索引的任意类型的数据映射。
		"缺升级阶": func(v map[string]any) { v["upgrades"] = v["upgrades"].([]any)[:2] },
		// v 记录按文本索引的任意类型的数据映射。
		"重复升级阶": func(v map[string]any) { v["upgrades"].([]any)[1].(map[string]any)["level"] = 1 },
		// v 记录按文本索引的任意类型的数据映射。
		"降低升级资格": func(v map[string]any) { v["upgrades"].([]any)[0].(map[string]any)["required_role_level"] = 1 },
	}
	// v 记录按文本索引的任意类型的数据映射。
	tests["少于目标阶加一原书"] = func(v map[string]any) {
		// u 接收遍历当前数据时当前的元素。
		for _, u := range v["upgrades"].([]any) {
			u.(map[string]any)["book_costs"].([]any)[0].(map[string]any)["quantity"] = 1
		}
	}
	// name 定位测试集合中正在处理的成员，mutate 接收遍历测试集合时当前的元素。
	for name, mutate := range tests {
		// t 提供断言失败报告、子用例和清理注册。
		t.Run(name, func(t *testing.T) {
			// v 记录按文本索引的任意类型的数据映射。
			var v map[string]any
			// err 接收反序列化过程中可能产生的错误，供后续失败分支处理。
			if err := json.Unmarshal(data, &v); err != nil {
				t.Fatal(err)
			}
			mutate(v)
			// body 保存由序列化取得的消息正文，err 接收序列化过程中可能产生的错误，供后续失败分支处理。
			body, err := json.Marshal(v)
			if err != nil {
				t.Fatal(err)
			}
			// path 保存由加入取得的路径。
			path := filepath.Join(t.TempDir(), "candidate.json")
			// err 接收写入文件过程中可能产生的错误，供后续失败分支处理。
			if err := os.WriteFile(path, body, 0600); err != nil {
				t.Fatal(err)
			}
			// err 接收加载召唤候选集合过程中可能产生的错误，供后续失败分支处理。
			if _, err := LoadSummonCandidates(path); err == nil {
				t.Fatal("不完整/跨行为/未启用候选被接受")
			}
		})
	}
}

// firstSkill 返回第一项技能供调用方使用。
// v 记录按文本索引的任意类型的数据映射。
func firstSkill(v map[string]any) map[string]any { return v["skills"].([]any)[0].(map[string]any) }

// firstBook 返回第一项技能书供调用方使用。
// v 记录按文本索引的任意类型的数据映射。
func firstBook(v map[string]any) map[string]any { return v["books"].([]any)[0].(map[string]any) }

// TestNoPetsAndRecallNamesRequireExplicitVersionedPolicy 核验历史JSON中的空召回名键应在入口按presence拒绝，用诊断「缺少或降版的NoPets/召回名被当默认允许」定位对应断言。
// t 提供断言失败报告、子用例和清理注册。
func TestNoPetsAndRecallNamesRequireExplicitVersionedPolicy(t *testing.T) {
	// cfg 保存由加载取得的服务配置，err 接收加载过程中可能产生的错误，供后续失败分支处理。
	cfg, err := Load("../../../conf/dev/server.json")
	if err != nil {
		t.Fatal(err)
	}
	// data 保存由序列化取得的数据。
	data, err := json.Marshal(cfg.Fixture)
	if err != nil {
		t.Fatal(err)
	}
	// tests 初始化用作测试集合的按文本索引的供当前流程调用的函数映射。
	tests := map[string]func(map[string]any){
		// v 记录按文本索引的任意类型的数据映射。
		"当前图缺禁宠": func(v map[string]any) { delete(v["map"].(map[string]any), "no_pets") },
		// v 记录按文本索引的任意类型的数据映射。
		"城镇缺禁宠": func(v map[string]any) { delete(v["maps"].([]any)[0].(map[string]any), "no_pets") },
		// v 记录按文本索引的任意类型的数据映射。
		"禁宠null": func(v map[string]any) { v["map"].(map[string]any)["no_pets"] = nil },
		// v 记录按文本索引的任意类型的数据映射。
		"旧版夹带": func(v map[string]any) { v["rule_version"] = 16 },
		// v 记录按文本索引的任意类型的数据映射。
		"缺召回名": func(v map[string]any) {
			delete(v["pet_runtime"].(map[string]any)["definitions"].([]any)[0].(map[string]any), "recall_name")
		},
		// v 记录按文本索引的任意类型的数据映射。
		"空召回名": func(v map[string]any) {
			v["pet_runtime"].(map[string]any)["definitions"].([]any)[0].(map[string]any)["recall_name"] = ""
		},
		// v 记录按文本索引的任意类型的数据映射。
		"隐形召回空格": func(v map[string]any) {
			v["pet_runtime"].(map[string]any)["definitions"].([]any)[0].(map[string]any)["recall_name"] = "BoneFamiliar "
		},
	}
	// name 定位测试集合中正在处理的成员，mutate 接收遍历测试集合时当前的元素。
	for name, mutate := range tests {
		// t 提供断言失败报告、子用例和清理注册。
		t.Run(name, func(t *testing.T) {
			// v 记录按文本索引的任意类型的数据映射。
			var v map[string]any
			// err 接收反序列化过程中可能产生的错误，供后续失败分支处理。
			if err := json.Unmarshal(data, &v); err != nil {
				t.Fatal(err)
			}
			mutate(v)
			// body 保存由序列化取得的消息正文。
			body, _ := json.Marshal(v)
			// f 记录开发测试配置。
			var f Fixture
			// err 接收反序列化过程中可能产生的错误，供后续失败分支处理。
			if err := json.Unmarshal(body, &f); err == nil {
				// err 接收校验“summoning”过程中可能产生的错误，供后续失败分支处理。
				if err := f.ValidateSummoning(); err == nil {
					t.Fatal("缺少或降版的NoPets/召回名被当默认允许")
				}
			}
		})
	}
	// 历史16没有新字段；显式null也属于夹带，不能借nil绕过版本检查。
	// raw 接收遍历文本列表时当前的元素。
	for _, raw := range []string{`{"rule_version":16,"map":{"no_pets":null}}`, `{"rule_version":16,"skills":[{"summon":null}]}`} {
		// f 记录开发测试配置。
		var f Fixture
		// err 接收反序列化过程中可能产生的错误，供后续失败分支处理。
		if err := json.Unmarshal([]byte(raw), &f); err == nil && f.ValidateSummoning() == nil {
			t.Fatal("旧版夹带null召唤合同被接受")
		}
	}
	// raw 接收遍历文本列表时当前的元素。
	for _, raw := range []string{`{"rule_version":16,"pet_runtime":{"definitions":[{"recall_name":""}]}}`, `{"rule_version":16,"pet_runtime":{"definitions":[{"recall_name":null}]}}`} {
		// f 记录开发测试配置。
		var f Fixture
		// err 接收反序列化过程中可能产生的错误，供后续失败分支处理。
		if err := json.Unmarshal([]byte(raw), &f); err == nil {
			t.Fatal("历史JSON中的空召回名键没有在入口按presence拒绝")
		}
	}
}

// TestLegacyPetCandidateSchemaCannotHideEmptyRecallName 核对「历史Name存在性校验错误」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestLegacyPetCandidateSchemaCannotHideEmptyRecallName(t *testing.T) {
	// data 保存由读取文件取得的数据，err 接收读取文件过程中可能产生的错误，供后续失败分支处理。
	data, err := os.ReadFile(petCandidatePath())
	if err != nil {
		t.Fatal(err)
	}
	// mode 接收遍历文本列表时当前的元素。
	for _, mode := range []string{"absent", "empty", "null"} {
		// t 提供断言失败报告、子用例和清理注册。
		t.Run(mode, func(t *testing.T) {
			// v 记录按文本索引的任意类型的数据映射。
			var v map[string]any
			// err 接收反序列化过程中可能产生的错误，供后续失败分支处理。
			if err := json.Unmarshal(data, &v); err != nil {
				t.Fatal(err)
			}
			v["schema_version"] = 1
			// entry 接收遍历当前数据时当前的元素。
			for _, entry := range v["definitions"].([]any) {
				// d 按目标类型读取当前接口值，得到测试兼容旧数据宠物“candidate”“schema”“cannot”“hide”空“recall”名称所用数据。
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
			// body 保存由序列化取得的消息正文。
			body, _ := json.Marshal(v)
			// path 保存由加入取得的路径。
			path := filepath.Join(t.TempDir(), "legacy.json")
			// err 接收写入文件过程中可能产生的错误，供后续失败分支处理。
			if err := os.WriteFile(path, body, 0600); err != nil {
				t.Fatal(err)
			}
			// err 接收加载宠物运行时候选集合过程中可能产生的错误，供后续失败分支处理。
			_, err := LoadPetRuntimeCandidates(path)
			if (mode == "absent") != (err == nil) {
				t.Fatalf("历史Name存在性校验错误: mode=%s err=%v", mode, err)
			}
		})
	}
}
