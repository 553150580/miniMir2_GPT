package devconfig

import (
	"encoding/json"
	"os"
	"testing"
)

func TestStatusFixtureExplicitContractAndUnknownSupplyClosed(t *testing.T) {
	data, err := os.ReadFile("../../../config/development/first-loop.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture Fixture
	if err = json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	if len(fixture.Buffs) != 3 || fixture.CastingSupply.Enabled || len(fixture.CastingSupply.ItemIDs) != 1 || fixture.CastingSupply.ItemIDs[0] != 187 {
		t.Fatal("当前阶段护符未定单位必须关闭且只选187")
	}
	item, _ := fixture.Item(187)
	if item.MaxStack != 0 || item.IsEquipment || item.EquipmentSlot != 100 {
		t.Fatal("护符未定单位被伪装为正式堆叠/强化装备")
	}
	if err = fixture.ValidateBuffs(); err != nil {
		t.Fatal(err)
	}
	for _, missing := range []string{"enabled", "slot", "item_ids", "buff_id", "material_item_id", "material_quantity", "kind", "value_by_level", "target_level_divisor", "target_level_add", "duration_kind", "stat_add", "stat_multiplier", "duration_seconds_by_level", "tick_ms", "hit_duration_loss_ms_per_hp", "stack_policy", "remove_on_player_death", "allow_dead_targets"} {
		for _, null := range []bool{false, true} {
			t.Run(missing+map[bool]string{false: "_missing", true: "_null"}[null], func(t *testing.T) {
				var raw map[string]any
				if err := json.Unmarshal(data, &raw); err != nil {
					t.Fatal(err)
				}
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
				changed, _ := json.Marshal(raw)
				var candidate Fixture
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
	if fixture.ValidateBuffs() == nil {
		t.Fatal("单位未定却可激活护符功能")
	}
}
