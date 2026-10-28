package devconfig

import (
	"encoding/json"
	"testing"
)

func teleportMapFixture() Fixture {
	allowed := false
	return Fixture{RuleVersion: 15, Map: MapTemplate{ID: 1, Width: 17, Height: 17, NoTeleport: &allowed}, Maps: []MapTemplate{{ID: 2, Width: 3, Height: 2, SceneKind: SceneKindTown, NoTeleport: &allowed, Source: &SourceMapIdentity{Code: "0", Width: 3, Height: 2}, CollisionBits: []byte{10}, SourceStaticBits: []byte{2}}}}
}

func TestTeleportUsesSourceStaticWalkWithoutClosedDoorOrDynamicOccupancyInference(t *testing.T) {
	f := teleportMapFixture()
	m := &f.Maps[0]
	if err := f.ValidateTeleport(); err != nil {
		t.Fatal(err)
	}
	if m.Walkable(Point{X: 0, Y: 1}) || !m.TeleportWalkable(Point{X: 0, Y: 1}) {
		t.Fatal("闭门专用阻挡混入源静态Teleport判定")
	}
	if m.TeleportWalkable(Point{X: 1, Y: 0}) {
		t.Fatal("源静态墙变为可传送")
	}
	for _, p := range []Point{{X: -1}, {X: 3}, {Y: 2}} {
		if m.TeleportWalkable(p) {
			t.Fatal("越界点被当作有效样本", p)
		}
	}
	m.SourceStaticBits = nil
	if m.TeleportWalkable(Point{}) {
		t.Fatal("缺少源静态位图时偷用闭门位图")
	}
}

func TestTeleportMapRulesRejectMissingPolicyTamperedBitsAndLegacyInjection(t *testing.T) {
	for _, name := range []string{"missing_personal_policy", "missing_town_policy", "missing_static", "static_not_subset", "tail_bits", "personal_parallel_bits", "old_rule"} {
		t.Run(name, func(t *testing.T) {
			f := teleportMapFixture()
			switch name {
			case "missing_personal_policy":
				f.Map.NoTeleport = nil
			case "missing_town_policy":
				f.Maps[0].NoTeleport = nil
			case "missing_static":
				f.Maps[0].SourceStaticBits = nil
			case "static_not_subset":
				f.Maps[0].SourceStaticBits[0] |= 1
			case "tail_bits":
				f.Maps[0].SourceStaticBits[0] |= 128
				f.Maps[0].CollisionBits[0] |= 128
			case "personal_parallel_bits":
				f.Map.SourceStaticBits = []byte{0}
			case "old_rule":
				f.RuleVersion = 14
			}
			if f.ValidateTeleport() == nil {
				t.Fatal("不完整或错版本瞬移地图合同未拒绝")
			}
		})
	}
}

func TestTeleportRulesJSONRequiresAllSourceChoicesWithoutCoordinateEscapeHatch(t *testing.T) {
	good := map[string]any{"max_attempts": 200, "axis_rule": "map_dimension_divided_by_rank_plus_one", "landing_rule": "source_static_walk", "allow_occupied": true, "allow_accepted_dead": true, "check_source_map_at_due": true}
	for key := range good {
		t.Run("missing_"+key, func(t *testing.T) {
			copy := map[string]any{}
			for k, v := range good {
				if k != key {
					copy[k] = v
				}
			}
			raw, _ := json.Marshal(copy)
			var rules SkillTeleportRules
			if json.Unmarshal(raw, &rules) == nil {
				t.Fatal("缺字段被补成默认规则")
			}
		})
	}
	for _, change := range []map[string]any{{"max_attempts": 1.5}, {"max_attempts": true}, {"allow_accepted_dead": nil}, {"destination": map[string]int{"x": 8, "y": 8}}} {
		copy := map[string]any{}
		for k, v := range good {
			copy[k] = v
		}
		for k, v := range change {
			copy[k] = v
		}
		raw, _ := json.Marshal(copy)
		var rules SkillTeleportRules
		if json.Unmarshal(raw, &rules) == nil {
			t.Fatal("错误类型、null或坐标逃逸未拒绝", change)
		}
	}
	raw, _ := json.Marshal(good)
	var rules SkillTeleportRules
	if err := json.Unmarshal(raw, &rules); err != nil {
		t.Fatal(err)
	}
}
