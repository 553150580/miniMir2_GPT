package devconfig

import (
	"encoding/json"
	"os"
	"testing"
)

func TestDisplacementSkillsRequireTypedCompleteApprovedContract(t *testing.T) {
	content, err := os.ReadFile("../../../config/development/first-loop.json")
	if err != nil {
		t.Fatal(err)
	}
	for _, scenario := range []string{"missing", "null", "unknown", "short_ranks", "negative_distance", "auto_enabled", "delayed", "old_rule"} {
		t.Run(scenario, func(t *testing.T) {
			var raw map[string]any
			if err := json.Unmarshal(content, &raw); err != nil {
				t.Fatal(err)
			}
			var skill map[string]any
			for _, value := range raw["skills"].([]any) {
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
			encoded, err := json.Marshal(raw)
			if err != nil {
				t.Fatal(err)
			}
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
