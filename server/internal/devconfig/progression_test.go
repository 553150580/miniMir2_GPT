// 本文件核对经验曲线完整性、阈值闭包、显式布尔值和未启用玩法边界。

package devconfig

import (
	"encoding/json"
	"testing"
)

// progressionDocument 返回成长文档供调用方使用。
func progressionDocument() map[string]any {
	// thresholds 创建用于维护“thresholds”的容器或通道。
	thresholds := make([]int64, 50)
	// i 标记遍历“thresholds”时访问的下标。
	for i := range thresholds {
		thresholds[i] = int64(i + 1)
	}
	return map[string]any{"exp_per_level": thresholds, "curve_version": "test_candidate_v1", "initial_level_cap": 50, "rebirth_level_cap_increment": 1, "rebirth_reset_progression": false, "operating_level_cap": 50, "catalog_max_level": 50, "pending_pool_policy": "one_next_level", "new_overflow_policy": "reject_new_with_receipt", "legacy_overflow_policy": "preserve_frozen", "level_up_resources": "source_full_if_alive_original_birth", "rebirth_enabled": false}
}

// decodeProgression 解码成长。
// t 提供断言失败报告、子用例和清理注册，raw 记录原始值，使用按文本索引的任意类型的数据映射表示。
func decodeProgression(t *testing.T, raw map[string]any) Fixture {
	t.Helper()
	// data 保存由序列化取得的数据，err 接收序列化过程中可能产生的错误，供后续失败分支处理。
	data, err := json.Marshal(raw)
	if err != nil {
		t.Fatal(err)
	}
	// value 记录值，使用成长规则集合表示。
	var value ProgressionRules
	if err = json.Unmarshal(data, &value); err != nil {
		t.Fatal(err)
	}
	return Fixture{RuleVersion: 20, Progression: value}
}

// TestProgressionRequiresEveryExplicitFieldIncludingFalse 核对「缺失/null合同被默认补齐」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestProgressionRequiresEveryExplicitFieldIncludingFalse(t *testing.T) {
	// f 接收解码成长供后续步骤使用。
	f := decodeProgression(t, progressionDocument())
	// err 接收校验成长过程中可能产生的错误，供后续失败分支处理。
	if err := f.ValidateProgression(); err != nil {
		t.Fatal(err)
	}
	// key 接收遍历成长键集合时当前的元素。
	for _, key := range progressionKeys {
		// t 提供断言失败报告、子用例和清理注册。
		t.Run(key, func(t *testing.T) {
			// missing 接收遍历布尔状态列表时当前的元素。
			for _, missing := range []bool{true, false} {
				// raw 保存由成长文档取得的原始值。
				raw := progressionDocument()
				if missing {
					delete(raw, key)
				} else {
					raw[key] = nil
				}
				// bad 保存由解码成长取得的“bad”。
				bad := decodeProgression(t, raw)
				// err 接收校验成长过程中可能产生的错误，供后续失败分支处理。
				if err := bad.ValidateProgression(); err == nil {
					t.Fatalf("缺失/null合同被默认补齐：%s missing=%t", key, missing)
				}
			}
		})
	}
}

// TestProgressionRejectsMissingTailIllegalThresholdAndRebirthBypass 核验类型/未知字段应被拒绝，以及拒绝无效成长合同。
// t 提供断言失败报告、子用例和清理注册。
func TestProgressionRejectsMissingTailIllegalThresholdAndRebirthBypass(t *testing.T) {
	// name 定位按文本索引的供当前流程调用的函数映射中正在处理的成员，change 接收遍历按文本索引的供当前流程调用的函数映射时当前的元素。
	for name, change := range map[string]func(map[string]any){
		// r 记录按文本索引的任意类型的数据映射。
		"少了50级池容量": func(r map[string]any) { r["exp_per_level"] = make([]int64, 49) },
		// r 记录按文本索引的任意类型的数据映射。
		"零经验": func(r map[string]any) { r["exp_per_level"].([]int64)[3] = 0 },
		// r 记录按文本索引的任意类型的数据映射。
		"负经验": func(r map[string]any) { r["exp_per_level"].([]int64)[3] = -1 },
		// r 记录按文本索引的任意类型的数据映射。
		"来源多一行不准暗升51": func(r map[string]any) { r["exp_per_level"] = append(r["exp_per_level"].([]int64), 51) },
		// r 记录按文本索引的任意类型的数据映射。
		"运营开放超过目录": func(r map[string]any) { r["operating_level_cap"] = 51 },
		// r 记录按文本索引的任意类型的数据映射。
		"改变已定初始上限": func(r map[string]any) { r["initial_level_cap"] = 21 },
		// r 记录按文本索引的任意类型的数据映射。
		"改变转生增量": func(r map[string]any) { r["rebirth_level_cap_increment"] = 2 },
		// r 记录按文本索引的任意类型的数据映射。
		"错误清零转生": func(r map[string]any) { r["rebirth_reset_progression"] = true },
		// r 记录按文本索引的任意类型的数据映射。
		"无挑战开放转生": func(r map[string]any) { r["rebirth_enabled"] = true },
		// r 记录按文本索引的任意类型的数据映射。
		"丢失旧余额": func(r map[string]any) { r["legacy_overflow_policy"] = "discard" },
		// r 记录按文本索引的任意类型的数据映射。
		"池无限累计": func(r map[string]any) { r["pending_pool_policy"] = "unlimited" },
		// r 记录按文本索引的任意类型的数据映射。
		"旧身份升级回血": func(r map[string]any) { r["level_up_resources"] = "full_on_reconnect" },
	} {
		// t 提供断言失败报告、子用例和清理注册。
		t.Run(name, func(t *testing.T) {
			// raw 保存由成长文档取得的原始值。
			raw := progressionDocument()
			change(raw)
			// f 接收解码成长供后续步骤使用。
			f := decodeProgression(t, raw)
			// err 接收校验成长过程中可能产生的错误，供后续失败分支处理。
			if err := f.ValidateProgression(); err == nil {
				t.Fatal("无效成长合同被接受")
			}
		})
	}
	// raw 接收遍历文本列表时当前的元素。
	for _, raw := range []string{`{"exp_per_level":[9223372036854775808]}`, `{"initial_level_cap":true}`, `{"rebirth_enabled":0}`, `{"unknown_growth_default":1}`} {
		// p 记录成长规则集合。
		var p ProgressionRules
		// err 接收反序列化过程中可能产生的错误，供后续失败分支处理。
		if err := json.Unmarshal([]byte(raw), &p); err == nil {
			t.Fatalf("类型/未知字段没有被拒绝：%s", raw)
		}
	}
	// f 接收解码成长供后续步骤使用。
	f := decodeProgression(t, progressionDocument())
	f.RuleVersion = 19
	// err 接收校验成长过程中可能产生的错误，供后续失败分支处理。
	if err := f.ValidateProgression(); err == nil {
		t.Fatal("新成长合同混入旧版本")
	}
}
