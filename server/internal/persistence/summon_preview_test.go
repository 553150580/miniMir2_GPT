package persistence

import (
	"minimir/internal/devconfig"
	"reflect"
	"testing"
)

func TestSummonUpgradePreviewDescribesNewBirthInsteadOfDamage(t *testing.T) {
	cfg, err := devconfig.Load("../../../conf/dev/server.json")
	if err != nil {
		t.Fatal(err)
	}
	c, err := devconfig.LoadSummonCandidates("../../../config/development/summon-candidates.json")
	if err != nil {
		t.Fatal(err)
	}
	f := cfg.Fixture
	f.Skills, f.Items, f.SkillUpgrades = append(f.Skills, c.Skills...), append(f.Items, c.Books...), append(f.SkillUpgrades, c.Upgrades...)
	if err := f.Validate(); err != nil {
		t.Fatal(err)
	}
	d, found := f.PetRuntime.DefinitionByID(c.PetDefinitionIDs[0])
	if !found {
		t.Fatal("没有候选宠物")
	}
	for rank := uint32(0); rank < 3; rank++ {
		preview := (skillQuote{Plan: skillPlan{Kind: 2, SkillID: 17, From: rank, To: rank + 1}}).Proto(f)
		keys := []string{}
		for _, s := range preview.Stats {
			keys = append(keys, s.Key)
		}
		if !reflect.DeepEqual(keys, []string{"mp_cost", "new_pet_training_level", "new_pet_max_training_level", "new_pet_material_quantity", "new_pet_hp_max"}) {
			t.Fatal("召唤预览含无效通用伤害或遗漏新生限定", keys)
		}
		wantBefore := []int64{12 + int64(rank)*4, int64(rank), 4 + int64(rank), 1, d.Base.HP + int64(rank)*20}
		wantAfter := []int64{16 + int64(rank)*4, int64(rank) + 1, 5 + int64(rank), 1, d.Base.HP + int64(rank+1)*20}
		for i, s := range preview.Stats {
			if s.BeforeMin != wantBefore[i] || s.BeforeMax != wantBefore[i] || s.AfterMin != wantAfter[i] || s.AfterMax != wantAfter[i] {
				t.Fatalf("阶%d新生字段%s未反映来源投影:%v", rank, s.Key, s)
			}
		}
	}
}
