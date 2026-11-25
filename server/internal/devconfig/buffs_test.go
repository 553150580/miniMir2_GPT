// 本文件验证状态合同的显式字段以及未知耗材保持关闭的门禁。

package devconfig

import (
	"encoding/json"
	"os"
	"slices"
	"testing"
)

// TestStatusFixtureExplicitContractAndUnknownSupplyClosed 核对当前材料专槽、逐字段显式配置与未知数量拒绝。
// t 提供断言失败报告、子用例和清理注册。
func TestStatusFixtureExplicitContractAndUnknownSupplyClosed(t *testing.T) {
	// data 保存由读取文件取得的数据，err 接收读取文件过程中可能产生的错误，供后续失败分支处理。
	data, err := os.ReadFile("../../../config/development/first-loop.json")
	if err != nil {
		t.Fatal(err)
	}
	// fixture 记录开发测试配置。
	var fixture Fixture
	if err = json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	if fixture.RuleVersion <= 28 {
		item, _ := fixture.Item(187)
		if len(fixture.Buffs) != 4 || fixture.CastingSupply.Enabled || !slices.Equal(fixture.CastingSupply.ItemIDs, []uint32{187}) ||
			item.MaxStack != 0 || item.IsEquipment || item.EquipmentSlot != CastingSupplySlot {
			t.Fatal("旧规则在材料单位未定时只保留关闭的护符入口")
		}
	} else {
		if !fixture.CastingSupply.Enabled || len(fixture.CastingSupply.ItemIDs) != 3 {
			t.Fatal("已启用的专槽必须登记护符和两色药粉")
		}
		for _, id := range []uint32{187, 181, 182} {
			item, ok := fixture.Item(id)
			if !ok || !fixture.IsCastingSupply(id) || item.MaxStack <= 0 || item.IsEquipment || item.EquipmentSlot != CastingSupplySlot {
				t.Fatalf("材料 %d 缺少明确堆叠数量、专槽或白名单", id)
			}
		}
	}
	if err = fixture.ValidateBuffs(); err != nil {
		t.Fatal(err)
	}
	// missing 接收遍历文本列表时当前的元素。
	for _, missing := range []string{"enabled", "slot", "item_ids", "buff_id", "material_item_id", "material_quantity", "kind", "value_by_level", "target_level_divisor", "target_level_add", "duration_kind", "stat_add", "stat_multiplier", "duration_seconds_by_level", "tick_ms", "hit_duration_loss_ms_per_hp", "stack_policy", "remove_on_player_death", "allow_dead_targets"} {
		// null 接收遍历布尔状态列表时当前的元素。
		for _, null := range []bool{false, true} {
			// t 提供断言失败报告、子用例和清理注册。
			t.Run(missing+map[bool]string{false: "_missing", true: "_null"}[null], func(t *testing.T) {
				// raw 记录原始值，使用按文本索引的任意类型的数据映射表示。
				var raw map[string]any
				// err 接收反序列化过程中可能产生的错误，供后续失败分支处理。
				if err := json.Unmarshal(data, &raw); err != nil {
					t.Fatal(err)
				}
				// target 记录目标，使用按文本索引的任意类型的数据映射表示。
				var target map[string]any
				switch missing {
				case "enabled", "slot", "item_ids":
					target = raw["casting_supply"].(map[string]any)
				case "buff_id", "material_item_id", "material_quantity":
					target = raw["skills"].([]any)[0].(map[string]any)
				default:
					target = raw["buffs"].([]any)[0].(map[string]any)
				}
				if null {
					target[missing] = nil
				} else {
					delete(target, missing)
				}
				// changed 保存由序列化取得的已变化。
				changed, _ := json.Marshal(raw)
				// candidate 记录候选，使用开发测试配置表示。
				var candidate Fixture
				// err 接收反序列化过程中可能产生的错误，供后续失败分支处理。
				if err := json.Unmarshal(changed, &candidate); err != nil {
					t.Fatal(err)
				}
				if candidate.ValidateBuffs() == nil {
					t.Fatal("状态专有参数缺失/null被静默补默认")
				}
			})
		}
	}
	fixture.CastingSupply.Enabled = true
	for i := range fixture.Items {
		if fixture.Items[i].ID == 187 {
			fixture.Items[i].MaxStack = 0
		}
	}
	if fixture.ValidateBuffs() == nil {
		t.Fatal("单位未定却可激活护符功能")
	}
}
