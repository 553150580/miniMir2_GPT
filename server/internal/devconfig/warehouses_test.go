package devconfig

import (
	"encoding/json"
	"testing"
)

func TestWarehouseRulesExplicitBoundedAndLegacyDisabled(t *testing.T) {
	valid := `{"personal_initial_slots":30,"personal_expansion_slots":30,"personal_expansion_yuanbao_units":30000,"shared_slots":50,"query_page_limit":30}`
	var rules WarehouseRules
	if err := json.Unmarshal([]byte(valid), &rules); err != nil {
		t.Fatal(err)
	}
	f := Fixture{RuleVersion: 23, Warehouses: &rules}
	if err := f.ValidateWarehouses(); err != nil {
		t.Fatal(err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(valid), &raw); err != nil {
		t.Fatal(err)
	}
	for key := range raw {
		for _, missing := range []bool{false, true} {
			copy := map[string]json.RawMessage{}
			for k, v := range raw {
				copy[k] = v
			}
			if missing {
				delete(copy, key)
			} else {
				copy[key] = json.RawMessage(`null`)
			}
			b, _ := json.Marshal(copy)
			if json.Unmarshal(b, &WarehouseRules{}) == nil {
				t.Fatalf("未知字段被接受:%s missing=%v", key, missing)
			}
		}
	}
	for _, bad := range []string{`{"personal_initial_slots":-1}`, `{"personal_initial_slots":1.5}`, `{"query_page_limit":4294967296}`} {
		if json.Unmarshal([]byte(bad), &WarehouseRules{}) == nil {
			t.Fatal("非法数字被接受", bad)
		}
	}
	f.RuleVersion = 22
	if f.ValidateWarehouses() == nil {
		t.Fatal("旧版启用了仓库")
	}
	f.Warehouses = nil
	if f.ValidateWarehouses() != nil {
		t.Fatal("旧版nil不兼容")
	}
	f.RuleVersion = 23
	if f.ValidateWarehouses() == nil {
		t.Fatal("新版缺配置被接受")
	}
	f.Warehouses = &rules
	rules.QueryPageLimit = 51
	if f.ValidateWarehouses() == nil {
		t.Fatal("查询越过显式窗口")
	}
	rules.QueryPageLimit = 30
	rules.PersonalInitialSlots = 31
	if f.ValidateWarehouses() == nil {
		t.Fatal("初始不是完整页")
	}
	rules.PersonalInitialSlots = 30
	rules.SharedSlots = 1 << 53
	if f.ValidateWarehouses() == nil {
		t.Fatal("超出Excel精确整数")
	}
}
