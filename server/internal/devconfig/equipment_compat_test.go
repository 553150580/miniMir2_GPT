package devconfig

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func loadEquipmentCandidateBytes(t *testing.T, data []byte) (*SummonCandidateCatalog, error) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "candidate.json")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	return LoadSummonCandidates(path)
}

func TestEquipmentSummonRaw17AndMarshalRoundtripRemainNeutral(t *testing.T) {
	data, err := os.ReadFile(summonCandidatePath)
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]any
	if err = json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	for _, name := range equipmentStatFields {
		if _, exists := firstBook(raw)[name]; exists {
			t.Fatalf("旧17原书JSON不应被本测试补造新字段%s", name)
		}
	}
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
	encoded, err := json.Marshal(candidate)
	if err != nil {
		t.Fatal(err)
	}
	roundtrip, err := loadEquipmentCandidateBytes(t, encoded)
	if err != nil {
		t.Fatal(err)
	}
	if roundtrip.RequiredRuleVersion != 17 || roundtrip.RuntimeEnabled || roundtrip.Books[0].IsEquipment || roundtrip.Books[0].BookSkillID != 17 {
		t.Fatal("Go往返将旧书改成运行装备")
	}
	for _, catalog := range []*SummonCandidateCatalog{candidate, roundtrip} {
		cfg, e := Load("../../../conf/dev/server.json")
		if e != nil {
			t.Fatal(e)
		}
		if cfg.Fixture.RuleVersion != 23 {
			t.Fatal("必须保持主rule23")
		}
		if _, found := cfg.Fixture.Skill(17); found {
			t.Fatal("主profile意外开放17")
		}
		if _, found := cfg.Fixture.Item(25); found {
			t.Fatal("候选书意外进入主目录")
		}
		cfg.Fixture.Skills = append(cfg.Fixture.Skills, catalog.Skills...)
		cfg.Fixture.Items = append(cfg.Fixture.Items, catalog.Books...)
		cfg.Fixture.SkillUpgrades = append(cfg.Fixture.SkillUpgrades, catalog.Upgrades...)
		if e = cfg.Fixture.Validate(); e != nil {
			t.Fatal(e)
		}
		bytes, e := json.Marshal(cfg.Fixture)
		if e != nil {
			t.Fatal(e)
		}
		var fixture Fixture
		if e = json.Unmarshal(bytes, &fixture); e != nil {
			t.Fatal(e)
		}
		if e = fixture.Validate(); e != nil {
			t.Fatal(e)
		}
		for _, item := range fixture.Items {
			if item.IsEquipment && !item.EquipmentStatsKnown() {
				t.Fatal("主22往返丢了装备五项显式来源")
			}
		}
	}
}

func TestEquipmentSummonCandidateRejectsNewCapabilitiesAndNull(t *testing.T) {
	data, err := os.ReadFile(summonCandidatePath)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range equipmentStatFields {
		for _, value := range []any{1, -1, nil, "0", 0.5} {
			t.Run(name+"/"+func() string { b, _ := json.Marshal(value); return string(b) }(), func(t *testing.T) {
				var raw map[string]any
				if err := json.Unmarshal(data, &raw); err != nil {
					t.Fatal(err)
				}
				firstBook(raw)[name] = value
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
	var raw map[string]any
	if err = json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	for _, name := range equipmentStatFields {
		firstBook(raw)[name] = 0
	}
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

func TestEquipmentMain22StillRequiresAllFiveExplicitFields(t *testing.T) {
	data, err := os.ReadFile("../../../config/development/first-loop.json")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range equipmentStatFields {
		for _, nullValue := range []bool{false, true} {
			t.Run(name+map[bool]string{false: "/missing", true: "/null"}[nullValue], func(t *testing.T) {
				var raw map[string]any
				if err := json.Unmarshal(data, &raw); err != nil {
					t.Fatal(err)
				}
				found := false
				for _, value := range raw["items"].([]any) {
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
				body, err := json.Marshal(raw)
				if err != nil {
					t.Fatal(err)
				}
				var fixture Fixture
				if err = json.Unmarshal(body, &fixture); err == nil && fixture.Validate() == nil {
					t.Fatal("主22实际装备必填项被旧候选兼容放宽")
				}
			})
		}
	}
}
