package devconfig

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func displacementFixtureJSON(t *testing.T) map[string]any {
	t.Helper()
	data, err := os.ReadFile("../../../config/development/first-loop.json")
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	// 沿当前版本核验rule10引入的基础约束，避免强退版本使后续合法技能变成非法前态。
	for _, actor := range raw["players"].([]any) {
		actor.(map[string]any)["can_push"] = false
	}
	raw["pet"].(map[string]any)["can_push"] = false
	for _, actor := range raw["monsters"].([]any) {
		actor.(map[string]any)["can_push"] = false
	}
	for _, skill := range raw["skills"].([]any) {
		skill.(map[string]any)["auto_enabled"] = false
	}
	timing := raw["combat_timing"].(map[string]any)
	for field, value := range map[string]int64{"player_push_action_ms": 503, "monster_push_action_ms_per_step": 307, "monster_push_move_ms_per_step": 509, "pushed_step_display_ms": 311} {
		timing[field] = value
	}
	return raw
}

func TestDisplacementFoundationLoadRejectsMissingNullAndWrongBoolean(t *testing.T) {
	for _, actor := range []string{"player", "pet", "monster", "skill"} {
		for _, invalid := range []string{"missing", "null", "number", "string"} {
			t.Run(actor+"_"+invalid, func(t *testing.T) {
				raw := displacementFixtureJSON(t)
				var target map[string]any
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
				if _, err := loadDisplacementFixture(t, raw); err == nil {
					t.Fatal("rule10缺失/null/非布尔字段被静默接受")
				}
			})
		}
	}
}

func loadDisplacementFixture(t *testing.T, raw map[string]any) (Config, error) {
	t.Helper()
	dir := t.TempDir()
	data, err := json.Marshal(raw)
	if err != nil {
		t.Fatal(err)
	}
	fixturePath := filepath.Join(dir, "fixture.json")
	if err = os.WriteFile(fixturePath, data, 0600); err != nil {
		t.Fatal(err)
	}
	configBytes, err := os.ReadFile("../../../conf/dev/server.json")
	if err != nil {
		t.Fatal(err)
	}
	var config map[string]any
	if err = json.Unmarshal(configBytes, &config); err != nil {
		t.Fatal(err)
	}
	config["fixture_file"] = fixturePath
	configBytes, err = json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(dir, "server.json")
	if err = os.WriteFile(configPath, configBytes, 0600); err != nil {
		t.Fatal(err)
	}
	return Load(configPath)
}

func TestDisplacementFoundationFalseAndDistinctTimingArePreserved(t *testing.T) {
	raw := displacementFixtureJSON(t)
	cfg, err := loadDisplacementFixture(t, raw)
	if err != nil {
		t.Fatal(err)
	}
	f := cfg.Fixture
	if f.Players[0].CanPush || f.Pet.CanPush || f.Monsters[0].CanPush || f.Skills[0].AutoEnabled {
		t.Fatal("显式false未保留")
	}
	if f.Monsters[0].ID != 1 || f.Monsters[0].Exp <= 0 || f.Monsters[0].RespawnMS <= 0 {
		t.Fatal("Fighter字段解码吞掉Monster外层字段")
	}
	timing := f.CombatTiming
	if timing.PlayerPushActionMS != 503 || timing.MonsterPushActionMSPerStep != 307 || timing.MonsterPushMoveMSPerStep != 509 || timing.PushedStepDisplayMS != 311 {
		t.Fatal("四种推挤时钟被合并或回退到攻击时钟")
	}
	for _, skill := range f.Skills {
		if skill.ID == 21 {
			t.Fatal("尚未建立绑定点前不能开启瞬息移动")
		}
	}
	first := raw["skills"].([]any)[0].(map[string]any)
	first["auto_enabled"] = true
	first["auto_priority"] = 0
	cfg, err = loadDisplacementFixture(t, raw)
	if err != nil || !cfg.Fixture.Skills[0].AutoEnabled || cfg.Fixture.Skills[0].AutoPriority != 0 {
		t.Fatalf("优先级0被误当作禁用开关：%v", err)
	}
}

func TestDisplacementFoundationTimingNeverFallsBackToAttackClock(t *testing.T) {
	for _, field := range []string{"player_push_action_ms", "monster_push_action_ms_per_step", "monster_push_move_ms_per_step", "pushed_step_display_ms"} {
		for _, invalid := range []string{"missing", "null", "zero"} {
			t.Run(field+"_"+invalid, func(t *testing.T) {
				raw := displacementFixtureJSON(t)
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
				if _, err := loadDisplacementFixture(t, raw); err == nil {
					t.Fatal("缺失推挤时钟被攻击时钟掩盖")
				}
			})
		}
	}
}
