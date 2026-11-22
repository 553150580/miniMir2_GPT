// 本文件验证近战模式、烈火和地狱火时间的显式配置完整性。

package devconfig

import (
	"encoding/json"
	"testing"
)

// TestCombatModesRequireEveryExplicitV6Field 分别删除近战模式与烈火、地狱火的显式配置字段，或把它写为 null，确认校验拒绝缺值。
// t 提供断言失败报告、子用例和清理注册。
func TestCombatModesRequireEveryExplicitV6Field(t *testing.T) {
	// cfg 保存由加载取得的服务配置，err 接收加载过程中可能产生的错误，供后续失败分支处理。
	cfg, err := Load("../../../conf/dev/server.json")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Fixture.RuleVersion < 6 {
		t.Fatal("本组需要唯一Excel导出的v6夹具")
	}
	// data 保存由序列化取得的数据。
	data, err := json.Marshal(cfg.Fixture.Skills[0])
	if err != nil {
		t.Fatal(err)
	}
	// raw 记录原始值，使用按文本索引的原始值消息映射表示。
	var raw map[string]json.RawMessage
	// err 接收反序列化过程中可能产生的错误，供后续失败分支处理。
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	// field 接收遍历文本列表时当前的元素。
	for _, field := range []string{"activation_kind", "max_attack_bonus_by_level", "ready_proc_denominator", "ready_proc_numerators", "secondary_direction_offsets", "secondary_distance_cells", "secondary_impact_ms", "charge_window_ms", "line_cells", "line_step_ms", "line_stop_on_blocked", "non_undead_divisor"} {
		// missing 接收遍历布尔状态列表时当前的元素。
		for _, missing := range []bool{false, true} {
			// t 提供断言失败报告、子用例和清理注册。
			t.Run(field+map[bool]string{false: "_null", true: "_missing"}[missing], func(t *testing.T) {
				// copy 建立带初始值的按文本索引的原始值消息映射，供本段继续处理。
				copy := map[string]json.RawMessage{}
				// key 定位原始值中正在处理的成员，value 接收遍历原始值时当前的元素。
				for key, value := range raw {
					copy[key] = value
				}
				if missing {
					delete(copy, field)
				} else {
					copy[field] = json.RawMessage("null")
				}
				// encoded 保存由序列化取得的“encoded”，err 接收序列化过程中可能产生的错误，供后续失败分支处理。
				encoded, err := json.Marshal(copy)
				if err != nil {
					t.Fatal(err)
				}
				// skill 记录技能。
				var skill Skill
				// err 接收反序列化过程中可能产生的错误，供后续失败分支处理。
				if err := json.Unmarshal(encoded, &skill); err != nil {
					t.Fatal(err)
				}
				// f 保存当前处理所用的开发测试配置。
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

// TestHellFireRequiresExplicitSingleRayTimingParameters 核对地狱火沿单线传播四格、逐格间隔 100 毫秒、遇阻停止，并拒绝非法时序或输入配置。
// t 提供断言失败报告、子用例和清理注册。
func TestHellFireRequiresExplicitSingleRayTimingParameters(t *testing.T) {
	// cfg 保存由加载取得的服务配置，err 接收加载过程中可能产生的错误，供后续失败分支处理。
	cfg, err := Load("../../../conf/dev/server.json")
	if err != nil {
		t.Fatal(err)
	}
	// skill 接收技能供后续步骤使用，found 保存由技能取得的已找到标记。
	skill, found := cfg.Fixture.Skill(9)
	if !found || skill.LineCells != 4 || !skill.LineStopOnBlocked || skill.LineStepMS != 100 {
		t.Fatal("用户确认的固定单线四格/链式100ms合同缺失")
	}
	// invalid 接收遍历供当前流程调用的函数列表时当前的元素，s 记录技能，s 记录技能，s 记录技能，s 记录技能，s 记录技能，s 记录技能。
	for _, invalid := range []func(*Skill){func(s *Skill) { s.LineCells = 0 }, func(s *Skill) { s.LineCells = 5 }, func(s *Skill) { s.LineStopOnBlocked = false }, func(s *Skill) { s.LineStepMS = 0 }, func(s *Skill) { s.ActivationKind = "toggle" }, func(s *Skill) { s.TargetKind = "enemy_grid" }} {
		// f 保存当前处理所用的开发测试配置。
		f := cfg.Fixture
		f.Skills = append([]Skill(nil), f.Skills...)
		// i 定位技能集合中正在处理的成员，s 接收遍历技能集合时当前的元素。
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
