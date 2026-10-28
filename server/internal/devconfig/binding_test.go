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
