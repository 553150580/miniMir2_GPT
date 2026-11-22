// 本文件验证位移技能参数完整性、来源限制和允许关闭的条款。

package devconfig

import (
	"encoding/json"
	"os"
	"testing"
)

// TestDisplacementSkillsRequireTypedCompleteApprovedContract 核对「有缺口或越过范围门禁的位移配置被接受」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestDisplacementSkillsRequireTypedCompleteApprovedContract(t *testing.T) {
	// content 保存由读取文件取得的“content”，err 接收读取文件过程中可能产生的错误，供后续失败分支处理。
	content, err := os.ReadFile("../../../config/development/first-loop.json")
	if err != nil {
		t.Fatal(err)
	}
	// scenario 接收遍历文本列表时当前的元素。
	for _, scenario := range []string{"missing", "null", "unknown", "short_ranks", "negative_distance", "auto_enabled", "delayed", "old_rule"} {
		// t 提供断言失败报告、子用例和清理注册。
		t.Run(scenario, func(t *testing.T) {
			// raw 记录原始值，使用按文本索引的任意类型的数据映射表示。
			var raw map[string]any
			// err 接收反序列化过程中可能产生的错误，供后续失败分支处理。
			if err := json.Unmarshal(content, &raw); err != nil {
				t.Fatal(err)
			}
			// skill 记录技能，使用按文本索引的任意类型的数据映射表示。
			var skill map[string]any
			// value 接收遍历当前数据时当前的元素。
			for _, value := range raw["skills"].([]any) {
				// row 按目标类型读取当前接口值，得到行记录。
				row := value.(map[string]any)
				if row["id"].(float64) == 8 {
					skill = row
				}
			}
			if skill == nil {
				t.Fatal("本组需要已选择的抗拒候选")
			}
			switch scenario {
			case "missing":
				delete(skill["displacement"].(map[string]any), "success_base")
			case "null":
				skill["displacement"] = nil
			case "unknown":
				skill["displacement"].(map[string]any)["free_teleport"] = true
			case "short_ranks":
				skill["displacement"].(map[string]any)["distance_base_by_level"] = []int{1}
			case "negative_distance":
				skill["displacement"].(map[string]any)["distance_random_bound"] = -1
			case "auto_enabled":
				skill["auto_enabled"] = true
			case "delayed":
				skill["delay_base_ms"] = 200
			case "old_rule":
				raw["rule_version"] = 10
			}
			// encoded 保存由序列化取得的“encoded”，err 接收序列化过程中可能产生的错误，供后续失败分支处理。
			encoded, err := json.Marshal(raw)
			if err != nil {
				t.Fatal(err)
			}
			// fixture 记录开发测试配置。
			var fixture Fixture
			if err = json.Unmarshal(encoded, &fixture); err == nil {
				err = fixture.Validate()
			}
			if err == nil {
				t.Fatal("有缺口或越过范围门禁的位移配置被接受")
			}
		})
	}
}
