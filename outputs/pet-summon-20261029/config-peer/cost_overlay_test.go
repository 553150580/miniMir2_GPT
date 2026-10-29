package devconfig

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestAuditSummonCatalogRejectsUnderchargedBookFormula(t *testing.T) {
	data, err := os.ReadFile("../../../config/development/summon-candidates.json")
	if err != nil { t.Fatal(err) }
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil { t.Fatal(err) }
	for _, upgrade := range raw["upgrades"].([]any) {
		upgrade.(map[string]any)["book_costs"].([]any)[0].(map[string]any)["quantity"] = 1
	}
	encoded, err := json.Marshal(raw)
	if err != nil { t.Fatal(err) }
	path := filepath.Join(t.TempDir(), "undercharged-candidate.json")
	if err := os.WriteFile(path, encoded, 0600); err != nil { t.Fatal(err) }
	c, err := LoadSummonCandidates(path)
	if err != nil { return }
	cfg, err := Load("../../../conf/dev/server.json")
	if err != nil { t.Fatal(err) }
	f := cfg.Fixture
	f.Skills = append(append([]Skill(nil), f.Skills...), c.Skills...)
	f.Items = append(append([]Item(nil), f.Items...), c.Books...)
	f.SkillUpgrades = append(append([]SkillUpgrade(nil), f.SkillUpgrades...), c.Upgrades...)
	if err := f.Validate(); err == nil {
		t.Fatal("三个升级阶全部仅需1本书，同时被独立loader和完整Fixture.Validate接受，违反目标阶+1的既定公式")
	}
}
