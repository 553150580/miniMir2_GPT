// 本文件验证位移基础区分缺失、空值和显式布尔值，并拒绝回退到普攻时间。

package devconfig

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// displacementFixtureJSON 返回位移开发测试配置JSON供调用方使用。
// t 提供断言失败报告、子用例和清理注册。
func displacementFixtureJSON(t *testing.T) map[string]any {
	t.Helper()
	// data 保存由读取文件取得的数据，err 接收读取文件过程中可能产生的错误，供后续失败分支处理。
	data, err := os.ReadFile("../../../config/development/first-loop.json")
	if err != nil {
		t.Fatal(err)
	}
	// raw 记录原始值，使用按文本索引的任意类型的数据映射表示。
	var raw map[string]any
	// err 接收反序列化过程中可能产生的错误，供后续失败分支处理。
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	// 沿当前版本核验rule10引入的基础约束，避免强退版本使后续合法技能变成非法前态。
	// actor 接收遍历当前数据时当前的元素。
	for _, actor := range raw["players"].([]any) {
		actor.(map[string]any)["can_push"] = false
	}
	raw["pet"].(map[string]any)["can_push"] = false
	// rule16的继承定义与旧bootstrap应一致；只改同一个隔离前态，不退回旧版本。
	// rules 按目标类型读取当前接口值，得到规则集合，ok 表明运行时值能否按要求的类型读取。
	if rules, ok := raw["pet_runtime"].(map[string]any); ok {
		// definition 接收遍历当前数据时当前的元素。
		for _, definition := range rules["definitions"].([]any) {
			definition.(map[string]any)["base"].(map[string]any)["can_push"] = false
		}
	}
	// actor 接收遍历当前数据时当前的元素。
	for _, actor := range raw["monsters"].([]any) {
		actor.(map[string]any)["can_push"] = false
	}
	// skill 接收遍历当前数据时当前的元素。
	for _, skill := range raw["skills"].([]any) {
		skill.(map[string]any)["auto_enabled"] = false
	}
	// timing 按目标类型读取当前接口值，得到时序。
	timing := raw["combat_timing"].(map[string]any)
	// field 定位按文本索引的数值映射中正在处理的成员，value 接收遍历按文本索引的数值映射时当前的元素。
	for field, value := range map[string]int64{"player_push_action_ms": 503, "monster_push_action_ms_per_step": 307, "monster_push_move_ms_per_step": 509, "pushed_step_display_ms": 311} {
		timing[field] = value
	}
	return raw
}

// TestDisplacementFoundationLoadRejectsMissingNullAndWrongBoolean 核对「rule10缺失/null/非布尔字段被静默接受」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestDisplacementFoundationLoadRejectsMissingNullAndWrongBoolean(t *testing.T) {
	// actor 接收遍历文本列表时当前的元素。
	for _, actor := range []string{"player", "pet", "monster", "skill"} {
		// invalid 接收遍历文本列表时当前的元素。
		for _, invalid := range []string{"missing", "null", "number", "string"} {
			// t 提供断言失败报告、子用例和清理注册。
			t.Run(actor+"_"+invalid, func(t *testing.T) {
				// raw 保存由位移开发测试配置JSON取得的原始值。
				raw := displacementFixtureJSON(t)
				// target 记录目标，使用按文本索引的任意类型的数据映射表示。
				var target map[string]any
				// key 保存键所使用的文本。
				key := "can_push"
				switch actor {
				case "player":
					target = raw["players"].([]any)[0].(map[string]any)
				case "pet":
					target = raw["pet"].(map[string]any)
				case "monster":
					target = raw["monsters"].([]any)[0].(map[string]any)
				case "skill":
					target = raw["skills"].([]any)[0].(map[string]any)
					key = "auto_enabled"
				}
				switch invalid {
				case "missing":
					delete(target, key)
				case "null":
					target[key] = nil
				case "number":
					target[key] = 0
				case "string":
					target[key] = "false"
				}
				// err 接收加载位移开发测试配置过程中可能产生的错误，供后续失败分支处理。
				if _, err := loadDisplacementFixture(t, raw); err == nil {
					t.Fatal("rule10缺失/null/非布尔字段被静默接受")
				}
			})
		}
	}
}

// loadDisplacementFixture 加载位移开发测试配置。
// t 提供断言失败报告、子用例和清理注册，raw 记录原始值，使用按文本索引的任意类型的数据映射表示。
func loadDisplacementFixture(t *testing.T, raw map[string]any) (Config, error) {
	t.Helper()
	// dir 保存由“temp”方向或目录取得的方向或目录。
	dir := t.TempDir()
	// data 保存由序列化取得的数据，err 接收序列化过程中可能产生的错误，供后续失败分支处理。
	data, err := json.Marshal(raw)
	if err != nil {
		t.Fatal(err)
	}
	// fixturePath 保存由加入取得的开发测试配置路径。
	fixturePath := filepath.Join(dir, "fixture.json")
	if err = os.WriteFile(fixturePath, data, 0600); err != nil {
		t.Fatal(err)
	}
	// configBytes 保存由读取文件取得的配置字节。
	configBytes, err := os.ReadFile("../../../conf/dev/server.json")
	if err != nil {
		t.Fatal(err)
	}
	// config 记录配置，使用按文本索引的任意类型的数据映射表示。
	var config map[string]any
	if err = json.Unmarshal(configBytes, &config); err != nil {
		t.Fatal(err)
	}
	config["fixture_file"] = fixturePath
	configBytes, err = json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	// configPath 保存由加入取得的配置路径。
	configPath := filepath.Join(dir, "server.json")
	if err = os.WriteFile(configPath, configBytes, 0600); err != nil {
		t.Fatal(err)
	}
	return Load(configPath)
}

// TestDisplacementFoundationFalseAndDistinctTimingArePreserved 核对「Fighter字段解码吞掉Monster外层字段」与「四种推挤时钟被合并或回退到攻击时钟」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestDisplacementFoundationFalseAndDistinctTimingArePreserved(t *testing.T) {
	// raw 保存由位移开发测试配置JSON取得的原始值。
	raw := displacementFixtureJSON(t)
	// cfg 保存由加载位移开发测试配置取得的服务配置，err 接收加载位移开发测试配置过程中可能产生的错误，供后续失败分支处理。
	cfg, err := loadDisplacementFixture(t, raw)
	if err != nil {
		t.Fatal(err)
	}
	// f 保存当前处理所用的开发测试配置。
	f := cfg.Fixture
	if f.Players[0].CanPush || f.Pet.CanPush || f.Monsters[0].CanPush || f.Skills[0].AutoEnabled {
		t.Fatal("显式false未保留")
	}
	if f.Monsters[0].ID != 1 || f.Monsters[0].Exp <= 0 || f.Monsters[0].RespawnMS <= 0 {
		t.Fatal("Fighter字段解码吞掉Monster外层字段")
	}
	// timing 保存战斗时序，用于当前步骤中的时序。
	timing := f.CombatTiming
	if timing.PlayerPushActionMS != 503 || timing.MonsterPushActionMSPerStep != 307 || timing.MonsterPushMoveMSPerStep != 509 || timing.PushedStepDisplayMS != 311 {
		t.Fatal("四种推挤时钟被合并或回退到攻击时钟")
	}
	// skill 接收遍历技能集合时当前的元素。
	for _, skill := range f.Skills {
		if skill.ID == 21 && (f.RuleVersion < 15 || !f.SafeZoneBindingEnabled() || skill.Teleport == nil) {
			t.Fatal("瞬移须在来源城镇及可信绑定之后以完整rule15合同选择")
		}
	}
	// first 按目标类型读取当前接口值，得到第一项。
	first := raw["skills"].([]any)[0].(map[string]any)
	first["auto_enabled"] = true
	first["auto_priority"] = 0
	cfg, err = loadDisplacementFixture(t, raw)
	if err != nil || !cfg.Fixture.Skills[0].AutoEnabled || cfg.Fixture.Skills[0].AutoPriority != 0 {
		t.Fatalf("优先级0被误当作禁用开关：%v", err)
	}
}

// TestDisplacementFoundationTimingNeverFallsBackToAttackClock 核对「缺失推挤时钟被攻击时钟掩盖」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestDisplacementFoundationTimingNeverFallsBackToAttackClock(t *testing.T) {
	// field 接收遍历文本列表时当前的元素。
	for _, field := range []string{"player_push_action_ms", "monster_push_action_ms_per_step", "monster_push_move_ms_per_step", "pushed_step_display_ms"} {
		// invalid 接收遍历文本列表时当前的元素。
		for _, invalid := range []string{"missing", "null", "zero"} {
			// t 提供断言失败报告、子用例和清理注册。
			t.Run(field+"_"+invalid, func(t *testing.T) {
				// raw 保存由位移开发测试配置JSON取得的原始值。
				raw := displacementFixtureJSON(t)
				// timing 按目标类型读取当前接口值，得到时序。
				timing := raw["combat_timing"].(map[string]any)
				timing["player_attack_impact_ms"] = 911
				switch invalid {
				case "missing":
					delete(timing, field)
				case "null":
					timing[field] = nil
				case "zero":
					timing[field] = 0
				}
				// err 接收加载位移开发测试配置过程中可能产生的错误，供后续失败分支处理。
				if _, err := loadDisplacementFixture(t, raw); err == nil {
					t.Fatal("缺失推挤时钟被攻击时钟掩盖")
				}
			})
		}
	}
}
