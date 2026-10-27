package devconfig

import (
	"encoding/json"
	"testing"
)

func TestCombatModesRequireEveryExplicitV6Field(t *testing.T) {
	cfg, err := Load("../../../conf/dev/server.json")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Fixture.RuleVersion < 6 {
		t.Fatal("本组需要唯一Excel导出的v6夹具")
	}
	data, err := json.Marshal(cfg.Fixture.Skills[0])
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"activation_kind", "max_attack_bonus_by_level", "ready_proc_denominator", "ready_proc_numerators", "secondary_direction_offsets", "secondary_distance_cells", "secondary_impact_ms", "charge_window_ms", "line_cells", "line_step_ms", "line_stop_on_blocked", "non_undead_divisor"} {
		for _, missing := range []bool{false, true} {
			t.Run(field+map[bool]string{false: "_null", true: "_missing"}[missing], func(t *testing.T) {
				copy := map[string]json.RawMessage{}
				for key, value := range raw {
					copy[key] = value
				}
				if missing {
					delete(copy, field)
				} else {
					copy[field] = json.RawMessage("null")
				}
				encoded, err := json.Marshal(copy)
				if err != nil {
					t.Fatal(err)
				}
				var skill Skill
				if err := json.Unmarshal(encoded, &skill); err != nil {
					t.Fatal(err)
				}
				f := cfg.Fixture
				f.Skills = append([]Skill(nil), f.Skills...)
				f.Skills[0] = skill
				if f.Validate() == nil {
					t.Fatal("v6缺失/null字段被默认值掩盖")
				}
			})
		}
	}
}

func TestHellFireRequiresExplicitSingleRayTimingParameters(t *testing.T) {
	cfg, err := Load("../../../conf/dev/server.json")
	if err != nil {
		t.Fatal(err)
	}
	skill, found := cfg.Fixture.Skill(9)
	if !found || skill.LineCells != 4 || !skill.LineStopOnBlocked || skill.LineStepMS != 100 {
		t.Fatal("用户确认的固定单线四格/链式100ms合同缺失")
	}
	for _, invalid := range []func(*Skill){func(s *Skill) { s.LineCells = 0 }, func(s *Skill) { s.LineCells = 5 }, func(s *Skill) { s.LineStopOnBlocked = false }, func(s *Skill) { s.LineStepMS = 0 }, func(s *Skill) { s.ActivationKind = "toggle" }, func(s *Skill) { s.TargetKind = "enemy_grid" }} {
		f := cfg.Fixture
		f.Skills = append([]Skill(nil), f.Skills...)
		for i, s := range f.Skills {
			if s.ID == 9 {
				invalid(&f.Skills[i])
			}
		}
		if f.Validate() == nil {
			t.Fatal("地狱火无效传播时序/输入契约被接受")
		}
	}
}
