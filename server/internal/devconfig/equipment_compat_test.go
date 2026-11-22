// 本文件验证旧候选装备快照向后兼容及当前版本显式装备字段约束。

package devconfig

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// loadEquipmentCandidateBytes 加载装备“candidate”字节。
// t 提供断言失败报告、子用例和清理注册，data 携带供当前流程读取或解析的原始字节。
func loadEquipmentCandidateBytes(t *testing.T, data []byte) (*SummonCandidateCatalog, error) {
	t.Helper()
	// path 保存由加入取得的路径。
	path := filepath.Join(t.TempDir(), "candidate.json")
	// err 接收写入文件过程中可能产生的错误，供后续失败分支处理。
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	return LoadSummonCandidates(path)
}

// TestEquipmentSummonRaw17AndMarshalRoundtripRemainNeutral 核验旧17原书JSON不应被本测试补造新字段，以及必须保持主rule23。
// t 提供断言失败报告、子用例和清理注册。
func TestEquipmentSummonRaw17AndMarshalRoundtripRemainNeutral(t *testing.T) {
	// data 保存由读取文件取得的数据，err 接收读取文件过程中可能产生的错误，供后续失败分支处理。
	data, err := os.ReadFile(summonCandidatePath)
	if err != nil {
		t.Fatal(err)
	}
	// raw 记录原始值，使用按文本索引的任意类型的数据映射表示。
	var raw map[string]any
	if err = json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	// name 接收遍历装备属性字段集合时当前的元素。
	for _, name := range equipmentStatFields {
		// exists 保存从第一项技能书取出的存在。
		if _, exists := firstBook(raw)[name]; exists {
			t.Fatalf("旧17原书JSON不应被本测试补造新字段%s", name)
		}
	}
	// candidate 保存由加载装备候选字节取得的候选。
	candidate, err := loadEquipmentCandidateBytes(t, data)
	if err != nil {
		t.Fatal(err)
	}
	if candidate.RequiredRuleVersion != 17 || candidate.RuntimeEnabled || candidate.Books[0].IsEquipment {
		t.Fatal("载入原候选改变最低版本/开关/物品类别")
	}
	if candidate.Books[0].EquipmentStatsKnown() {
		t.Fatal("原书缺失字段被标成显式装备来源")
	}
	// encoded 保存由序列化取得的“encoded”。
	encoded, err := json.Marshal(candidate)
	if err != nil {
		t.Fatal(err)
	}
	// roundtrip 保存由加载装备候选字节取得的往返编码。
	roundtrip, err := loadEquipmentCandidateBytes(t, encoded)
	if err != nil {
		t.Fatal(err)
	}
	if roundtrip.RequiredRuleVersion != 17 || roundtrip.RuntimeEnabled || roundtrip.Books[0].IsEquipment || roundtrip.Books[0].BookSkillID != 17 {
		t.Fatal("Go往返将旧书改成运行装备")
	}
	// catalog 接收遍历召唤候选目录列表时当前的元素。
	for _, catalog := range []*SummonCandidateCatalog{candidate, roundtrip} {
		// cfg 保存由加载取得的服务配置，e 接收加载供后续步骤使用。
		cfg, e := Load("../../../conf/dev/server.json")
		if e != nil {
			t.Fatal(e)
		}
		if cfg.Fixture.RuleVersion < 23 {
			t.Fatal("必须保持主rule23")
		}
		// found 保存由技能取得的已找到标记。
		if _, found := cfg.Fixture.Skill(17); found {
			t.Fatal("主profile意外开放17")
		}
		// found 保存由道具取得的已找到标记。
		if _, found := cfg.Fixture.Item(25); found {
			t.Fatal("候选书意外进入主目录")
		}
		cfg.Fixture.Skills = append(cfg.Fixture.Skills, catalog.Skills...)
		cfg.Fixture.Items = append(cfg.Fixture.Items, catalog.Books...)
		cfg.Fixture.SkillUpgrades = append(cfg.Fixture.SkillUpgrades, catalog.Upgrades...)
		if e = cfg.Fixture.Validate(); e != nil {
			t.Fatal(e)
		}
		// bytes 保存由序列化取得的字节。
		bytes, e := json.Marshal(cfg.Fixture)
		if e != nil {
			t.Fatal(e)
		}
		// fixture 记录开发测试配置。
		var fixture Fixture
		if e = json.Unmarshal(bytes, &fixture); e != nil {
			t.Fatal(e)
		}
		if e = fixture.Validate(); e != nil {
			t.Fatal(e)
		}
		// item 接收遍历道具集合时当前的元素。
		for _, item := range fixture.Items {
			if item.IsEquipment && !item.EquipmentStatsKnown() {
				t.Fatal("主22往返丢了装备五项显式来源")
			}
		}
	}
}

// TestEquipmentSummonCandidateRejectsNewCapabilitiesAndNull 核对「原书新能力/null/错误类型被兼容入口接受」与「旧Go Marshal明确中性零应兼容」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestEquipmentSummonCandidateRejectsNewCapabilitiesAndNull(t *testing.T) {
	// data 保存由读取文件取得的数据，err 接收读取文件过程中可能产生的错误，供后续失败分支处理。
	data, err := os.ReadFile(summonCandidatePath)
	if err != nil {
		t.Fatal(err)
	}
	// name 接收遍历装备属性字段集合时当前的元素。
	for _, name := range equipmentStatFields {
		// value 接收遍历任意类型的数据列表时当前的元素。
		for _, value := range []any{1, -1, nil, "0", 0.5} {
			// b 保存由序列化取得的b，t 提供断言失败报告、子用例和清理注册。
			t.Run(name+"/"+func() string { b, _ := json.Marshal(value); return string(b) }(), func(t *testing.T) {
				// raw 记录原始值，使用按文本索引的任意类型的数据映射表示。
				var raw map[string]any
				// err 接收反序列化过程中可能产生的错误，供后续失败分支处理。
				if err := json.Unmarshal(data, &raw); err != nil {
					t.Fatal(err)
				}
				firstBook(raw)[name] = value
				// body 保存由序列化取得的消息正文，err 接收序列化过程中可能产生的错误，供后续失败分支处理。
				body, err := json.Marshal(raw)
				if err != nil {
					t.Fatal(err)
				}
				if _, err = loadEquipmentCandidateBytes(t, body); err == nil {
					t.Fatal("原书新能力/null/错误类型被兼容入口接受")
				}
			})
		}
	}
	// raw 记录原始值，使用按文本索引的任意类型的数据映射表示。
	var raw map[string]any
	if err = json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	// name 接收遍历装备属性字段集合时当前的元素。
	for _, name := range equipmentStatFields {
		firstBook(raw)[name] = 0
	}
	// body 保存由序列化取得的消息正文。
	body, err := json.Marshal(raw)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = loadEquipmentCandidateBytes(t, body); err != nil {
		t.Fatal("旧Go Marshal明确中性零应兼容", err)
	}
	firstBook(raw)["unknown_equipment_bonus"] = 0
	body, _ = json.Marshal(raw)
	if _, err = loadEquipmentCandidateBytes(t, body); err == nil {
		t.Fatal("兼容新字段误放宽未知键")
	}
}

// TestEquipmentMain22StillRequiresAllFiveExplicitFields 核对「主22实际装备必填项被旧候选兼容放宽」与「缺主装备前态」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestEquipmentMain22StillRequiresAllFiveExplicitFields(t *testing.T) {
	// data 保存由读取文件取得的数据，err 接收读取文件过程中可能产生的错误，供后续失败分支处理。
	data, err := os.ReadFile("../../../config/development/first-loop.json")
	if err != nil {
		t.Fatal(err)
	}
	// name 接收遍历装备属性字段集合时当前的元素。
	for _, name := range equipmentStatFields {
		// nullValue 接收遍历布尔状态列表时当前的元素。
		for _, nullValue := range []bool{false, true} {
			// t 提供断言失败报告、子用例和清理注册。
			t.Run(name+map[bool]string{false: "/missing", true: "/null"}[nullValue], func(t *testing.T) {
				// raw 记录原始值，使用按文本索引的任意类型的数据映射表示。
				var raw map[string]any
				// err 接收反序列化过程中可能产生的错误，供后续失败分支处理。
				if err := json.Unmarshal(data, &raw); err != nil {
					t.Fatal(err)
				}
				// found 保存当前处理所用的已找到标记。
				found := false
				// value 接收遍历当前数据时当前的元素。
				for _, value := range raw["items"].([]any) {
					// item 按目标类型读取当前接口值，得到道具。
					item := value.(map[string]any)
					if item["is_equipment"] == true {
						if nullValue {
							item[name] = nil
						} else {
							delete(item, name)
						}
						found = true
						break
					}
				}
				if !found {
					t.Fatal("缺主装备前态")
				}
				// body 保存由序列化取得的消息正文，err 接收序列化过程中可能产生的错误，供后续失败分支处理。
				body, err := json.Marshal(raw)
				if err != nil {
					t.Fatal(err)
				}
				// fixture 记录开发测试配置。
				var fixture Fixture
				if err = json.Unmarshal(body, &fixture); err == nil && fixture.Validate() == nil {
					t.Fatal("主22实际装备必填项被旧候选兼容放宽")
				}
			})
		}
	}
}
