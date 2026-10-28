package persistence

import (
	"minimir/internal/devconfig"
	"testing"
)

func TestDisplacementUpgradePreviewShowsApprovedDistanceAndActualDamage(t *testing.T) {
	cfg, err := devconfig.Load("../../../conf/dev/server.json")
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []uint32{8, 27} {
		for from := uint32(0); from < 3; from++ {
			quote := skillQuote{Plan: skillPlan{Kind: 2, SkillID: id, From: from, To: from + 1}}
			preview := quote.Proto(cfg.Fixture)
			distance := preview.Stats[0]
			s, _ := cfg.Fixture.Skill(id)
			d := s.Displacement
			if distance.BeforeMin != int64(d.DistanceBaseByLevel[from]) || distance.AfterMax != int64(d.DistanceBaseByLevel[from+1])+d.DistanceRandomBound-1 {
				t.Fatal("升级预览未显示唯一Excel的用户已审定距离")
			}
			keys := map[string]bool{}
			for _, value := range preview.Stats {
				keys[value.Key] = true
				if id == 8 && value.Key == "monster_damage" && (value.BeforeMin != 0 || value.AfterMin != 0) {
					t.Fatal("抗拒预览伪造对怪物伤害")
				}
			}
			if id == 8 && (keys["base_power"] || !keys["monster_damage"] || !keys["push_base_success_percent"]) {
				t.Fatal("抗拒不能把玩家来源分支的基础威力冒充推怪伤害")
			}
			if id == 27 && (!keys["base_power"] || !keys["dash_distance"]) {
				t.Fatal("野蛮缺少单次威力或冲撞距离预览")
			}
		}
	}
}
