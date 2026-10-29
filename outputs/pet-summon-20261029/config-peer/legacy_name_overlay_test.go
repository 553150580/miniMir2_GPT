package devconfig

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// 仅overlay反例：旧合同必须拒绝字段presence，不能因空字符串降成零值而绕过。
func TestAuditLegacySchemaOneRejectsExplicitEmptyRecallName(t *testing.T) {
	data, err := os.ReadFile("../../../config/development/pet-runtime-candidates.json")
	if err != nil { t.Fatal(err) }
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil { t.Fatal(err) }
	raw["schema_version"] = 1
	raw["definitions"].([]any)[0].(map[string]any)["recall_name"] = ""
	encoded, err := json.Marshal(raw)
	if err != nil { t.Fatal(err) }
	path := filepath.Join(t.TempDir(), "legacy-candidate.json")
	if err := os.WriteFile(path, encoded, 0600); err != nil { t.Fatal(err) }
	if _, err := LoadPetRuntimeCandidates(path); err == nil { t.Fatal("schema1接受了显式recall_name空字符串，旧合同受到新字段污染") }
}

func TestAuditLegacyRuleSixteenRejectsExplicitEmptyRecallName(t *testing.T) {
	data, err := os.ReadFile("../../../config/development/first-loop.json")
	if err != nil { t.Fatal(err) }
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil { t.Fatal(err) }
	raw["rule_version"] = 16
	delete(raw["map"].(map[string]any), "no_pets")
	for _, m := range raw["maps"].([]any) { delete(m.(map[string]any), "no_pets") }
	raw["pet_runtime"].(map[string]any)["definitions"].([]any)[0].(map[string]any)["recall_name"] = ""
	encoded, err := json.Marshal(raw)
	if err != nil { t.Fatal(err) }
	var f Fixture
	if err := json.Unmarshal(encoded, &f); err != nil { return }
	if err := f.ValidateSummoning(); err == nil { t.Fatal("rule16接受了显式recall_name空字符串，旧合同受到新字段污染") }
}
