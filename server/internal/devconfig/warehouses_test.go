// 本文件核对仓库行为、容量门槛和旧规则关闭状态。

package devconfig

import (
	"encoding/json"
	"testing"
)

// TestWarehouseRulesExplicitBoundedAndLegacyDisabled 核验拒绝非法数字，用诊断「超出Excel精确整数」定位对应断言。
// t 提供断言失败报告、子用例和清理注册。
func TestWarehouseRulesExplicitBoundedAndLegacyDisabled(t *testing.T) {
	// valid 保存有效所使用的文本。
	valid := `{"personal_initial_slots":30,"personal_expansion_slots":30,"personal_expansion_yuanbao_units":30000,"shared_slots":50,"query_page_limit":30}`
	// rules 记录规则集合，使用仓库规则集合表示。
	var rules WarehouseRules
	// err 接收反序列化过程中可能产生的错误，供后续失败分支处理。
	if err := json.Unmarshal([]byte(valid), &rules); err != nil {
		t.Fatal(err)
	}
	// f 组装开发测试配置所需的数据。
	f := Fixture{RuleVersion: 23, Warehouses: &rules}
	// err 接收校验仓库集合过程中可能产生的错误，供后续失败分支处理。
	if err := f.ValidateWarehouses(); err != nil {
		t.Fatal(err)
	}
	// raw 记录原始值，使用按文本索引的原始值消息映射表示。
	var raw map[string]json.RawMessage
	// err 接收反序列化过程中可能产生的错误，供后续失败分支处理。
	if err := json.Unmarshal([]byte(valid), &raw); err != nil {
		t.Fatal(err)
	}
	// key 接收遍历原始值时当前的元素。
	for key := range raw {
		// missing 接收遍历布尔状态列表时当前的元素。
		for _, missing := range []bool{false, true} {
			// copy 建立带初始值的按文本索引的原始值消息映射，供本段继续处理。
			copy := map[string]json.RawMessage{}
			// k 定位原始值中正在处理的成员，v 接收遍历原始值时当前的元素。
			for k, v := range raw {
				copy[k] = v
			}
			if missing {
				delete(copy, key)
			} else {
				copy[key] = json.RawMessage(`null`)
			}
			// b 保存由序列化取得的b。
			b, _ := json.Marshal(copy)
			if json.Unmarshal(b, &WarehouseRules{}) == nil {
				t.Fatalf("未知字段被接受:%s missing=%v", key, missing)
			}
		}
	}
	// bad 接收遍历文本列表时当前的元素。
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
