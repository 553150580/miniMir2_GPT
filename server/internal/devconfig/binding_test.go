package devconfig

import (
	"encoding/json"
	"testing"
)

func TestBindingRulesRequireExplicitCoordinatesAndConfiguredTerrain(t *testing.T) {
	for _, scenario := range []struct {
		name string
		json string
	}{
		{"missing_enabled", `{"start_points":[]}`},
		{"null_enabled", `{"enabled":null,"start_points":[]}`},
		{"missing_points", `{"enabled":true}`},
		{"null_points", `{"enabled":true,"start_points":null}`},
		{"unknown_rule", `{"enabled":true,"start_points":[],"free_reset":true}`},
		{"empty_enabled", `{"enabled":true,"start_points":[]}`},
		{"disabled_with_points", `{"enabled":false,"start_points":[{"map_id":1,"x":0,"y":0}]}`},
		{"missing_map", `{"enabled":true,"start_points":[{"x":0,"y":0}]}`},
		{"missing_x", `{"enabled":true,"start_points":[{"map_id":1,"y":0}]}`},
		{"null_y", `{"enabled":true,"start_points":[{"map_id":1,"x":0,"y":null}]}`},
		{"unknown_point", `{"enabled":true,"start_points":[{"map_id":1,"x":0,"y":0,"hp":10}]}`},
		{"foreign_map", `{"enabled":true,"start_points":[{"map_id":2,"x":0,"y":0}]}`},
		{"zero_map", `{"enabled":true,"start_points":[{"map_id":0,"x":0,"y":0}]}`},
		{"negative", `{"enabled":true,"start_points":[{"map_id":1,"x":-1,"y":0}]}`},
		{"outside", `{"enabled":true,"start_points":[{"map_id":1,"x":17,"y":0}]}`},
		{"blocked", `{"enabled":true,"start_points":[{"map_id":1,"x":1,"y":1}]}`},
		{"overflow", `{"enabled":true,"start_points":[{"map_id":1,"x":2147483648,"y":0}]}`},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			f := Fixture{RuleVersion: 12}
			f.Map.ID, f.Map.Width, f.Map.Height, f.Map.Blocked = 1, 17, 17, []Point{{X: 1, Y: 1}}
			err := json.Unmarshal([]byte(scenario.json), &f.Binding)
			if err == nil {
				err = f.ValidateBinding()
			}
			if err == nil {
				t.Fatal("缺失、关闭、未知地图或无效地格被补成了有效绑定")
			}
		})
	}
}

func TestBindingZeroCoordinatesAreExplicitAndLegacyDoesNotInventOrigin(t *testing.T) {
	f := Fixture{RuleVersion: 11}
	f.Map.ID, f.Map.Width, f.Map.Height = 1, 17, 17
	if err := f.ValidateBinding(); err != nil {
		t.Fatal(err)
	}
	f.RuleVersion = 12
	if f.ValidateBinding() == nil {
		t.Fatal("新规则缺失绑定合同未被拒绝")
	}
	for _, raw := range []string{`{"enabled":false,"start_points":[]}`, `{"enabled":true,"start_points":[{"map_id":1,"x":0,"y":0}]}`} {
		if err := json.Unmarshal([]byte(raw), &f.Binding); err != nil {
			t.Fatal(err)
		}
		if err := f.ValidateBinding(); err != nil {
			t.Fatal(err)
		}
	}
	f.RuleVersion = 11
	if f.ValidateBinding() == nil {
		t.Fatal("旧规则被悄悄启用绑定配置")
	}
}

func TestSafeZoneBindingGateDoesNotEnableRandomInitialization(t *testing.T) {
	for _, scenario := range []struct {
		name   string
		rule   uint64
		json   string
		valid  bool
		active bool
	}{
		{"missing_gate", 14, `{"enabled":false,"start_points":[]}`, false, false},
		{"null_gate", 14, `{"enabled":false,"start_points":[],"safe_zone_enabled":null}`, false, false},
		{"integer_gate", 14, `{"enabled":false,"start_points":[],"safe_zone_enabled":1}`, false, false},
		{"text_gate", 14, `{"enabled":false,"start_points":[],"safe_zone_enabled":"true"}`, false, false},
		{"old_rule_new_gate", 13, `{"enabled":false,"start_points":[],"safe_zone_enabled":false}`, false, false},
		{"old_rule_unchanged", 13, `{"enabled":false,"start_points":[]}`, true, false},
		{"both_closed", 14, `{"enabled":false,"start_points":[],"safe_zone_enabled":false}`, true, false},
		{"visit_only", 14, `{"enabled":false,"start_points":[],"safe_zone_enabled":true}`, true, true},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			f := Fixture{RuleVersion: scenario.rule}
			f.Maps = []MapTemplate{{ID: 2, Width: 4, Height: 4, SceneKind: SceneKindTown, CollisionBits: make([]byte, 2), Source: &SourceMapIdentity{}, SafeZones: []SafeZone{{ID: 106, Center: Point{X: 2, Y: 2}, Radius: 1}}}}
			err := json.Unmarshal([]byte(scenario.json), &f.Binding)
			if err == nil {
				err = f.ValidateBinding()
			}
			if (err == nil) != scenario.valid {
				t.Fatalf("valid=%v err=%v", scenario.valid, err)
			}
			if scenario.valid && (f.SafeZoneBindingEnabled() != scenario.active || f.Binding.Enabled || len(f.Binding.StartPoints) != 0) {
				t.Fatal("安全区更新门禁改变随机初始化或缺省启用")
			}
		})
	}
}

func TestSafeZoneBindingRequiresSourceTownAndWalkableCenter(t *testing.T) {
	for _, invalid := range []string{"no_town", "no_source", "no_zones", "blocked_center", "personal_zone"} {
		t.Run(invalid, func(t *testing.T) {
			f := Fixture{RuleVersion: 14}
			if err := json.Unmarshal([]byte(`{"enabled":false,"start_points":[],"safe_zone_enabled":true}`), &f.Binding); err != nil {
				t.Fatal(err)
			}
			f.Maps = []MapTemplate{{ID: 2, Width: 4, Height: 4, SceneKind: SceneKindTown, CollisionBits: make([]byte, 2), Source: &SourceMapIdentity{}, SafeZones: []SafeZone{{ID: 106, Center: Point{X: 2, Y: 2}, Radius: 1}}}}
			switch invalid {
			case "no_town":
				f.Maps = nil
			case "no_source":
				f.Maps[0].Source = nil
			case "no_zones":
				f.Maps[0].SafeZones = nil
			case "blocked_center":
				f.Maps[0].CollisionBits[1] = 1 << 2
			case "personal_zone":
				f.Map.SafeZones = []SafeZone{{ID: 1, Center: Point{X: 2, Y: 2}, Radius: 1}}
			}
			if f.ValidateBinding() == nil {
				t.Fatal("缺少来源安全区或不可走中心仍启用绑定")
			}
		})
	}
}
