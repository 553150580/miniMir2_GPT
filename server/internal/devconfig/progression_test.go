package devconfig

import (
	"encoding/json"
	"testing"
)

func progressionDocument() map[string]any {
	thresholds := make([]int64, 50)
	for i := range thresholds {
		thresholds[i] = int64(i + 1)
	}
	return map[string]any{"exp_per_level": thresholds, "curve_version": "test_candidate_v1", "initial_level_cap": 50, "rebirth_level_cap_increment": 1, "rebirth_reset_progression": false, "operating_level_cap": 50, "catalog_max_level": 50, "pending_pool_policy": "one_next_level", "new_overflow_policy": "reject_new_with_receipt", "legacy_overflow_policy": "preserve_frozen", "level_up_resources": "source_full_if_alive_original_birth", "rebirth_enabled": false}
}

func decodeProgression(t *testing.T, raw map[string]any) Fixture {
	t.Helper()
	data, err := json.Marshal(raw)
	if err != nil {
		t.Fatal(err)
	}
	var value ProgressionRules
	if err = json.Unmarshal(data, &value); err != nil {
		t.Fatal(err)
	}
	return Fixture{RuleVersion: 20, Progression: value}
}

func TestProgressionRequiresEveryExplicitFieldIncludingFalse(t *testing.T) {
	f := decodeProgression(t, progressionDocument())
	if err := f.ValidateProgression(); err != nil {
		t.Fatal(err)
	}
	for _, key := range progressionKeys {
		t.Run(key, func(t *testing.T) {
			for _, missing := range []bool{true, false} {
				raw := progressionDocument()
				if missing {
					delete(raw, key)
				} else {
					raw[key] = nil
				}
				bad := decodeProgression(t, raw)
				if err := bad.ValidateProgression(); err == nil {
					t.Fatalf("缺失/null合同被默认补齐：%s missing=%t", key, missing)
				}
			}
		})
	}
}

func TestProgressionRejectsMissingTailIllegalThresholdAndRebirthBypass(t *testing.T) {
	for name, change := range map[string]func(map[string]any){
		"少了50级池容量":    func(r map[string]any) { r["exp_per_level"] = make([]int64, 49) },
		"零经验":         func(r map[string]any) { r["exp_per_level"].([]int64)[3] = 0 },
		"负经验":         func(r map[string]any) { r["exp_per_level"].([]int64)[3] = -1 },
		"来源多一行不准暗升51": func(r map[string]any) { r["exp_per_level"] = append(r["exp_per_level"].([]int64), 51) },
		"运营开放超过目录":    func(r map[string]any) { r["operating_level_cap"] = 51 },
		"改变已定初始上限":    func(r map[string]any) { r["initial_level_cap"] = 21 },
		"改变转生增量":      func(r map[string]any) { r["rebirth_level_cap_increment"] = 2 },
		"错误清零转生":      func(r map[string]any) { r["rebirth_reset_progression"] = true },
		"无挑战开放转生":     func(r map[string]any) { r["rebirth_enabled"] = true },
		"丢失旧余额":       func(r map[string]any) { r["legacy_overflow_policy"] = "discard" },
		"池无限累计":       func(r map[string]any) { r["pending_pool_policy"] = "unlimited" },
		"旧身份升级回血":     func(r map[string]any) { r["level_up_resources"] = "full_on_reconnect" },
	} {
		t.Run(name, func(t *testing.T) {
			raw := progressionDocument()
			change(raw)
			f := decodeProgression(t, raw)
			if err := f.ValidateProgression(); err == nil {
				t.Fatal("无效成长合同被接受")
			}
		})
	}
	for _, raw := range []string{`{"exp_per_level":[9223372036854775808]}`, `{"initial_level_cap":true}`, `{"rebirth_enabled":0}`, `{"unknown_growth_default":1}`} {
		var p ProgressionRules
		if err := json.Unmarshal([]byte(raw), &p); err == nil {
			t.Fatalf("类型/未知字段没有被拒绝：%s", raw)
		}
	}
	f := decodeProgression(t, progressionDocument())
	f.RuleVersion = 19
	if err := f.ValidateProgression(); err == nil {
		t.Fatal("新成长合同混入旧版本")
	}
}
