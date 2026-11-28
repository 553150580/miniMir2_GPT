// 本文件对拍原防御、魔法盾和扩展命中结算的前后顺序。
package game

import (
	"testing"

	"minimir/internal/devconfig"
	"minimir/internal/domain"
)

func TestExpandedHitUsesOriginalNetAndFrozenChallenge(t *testing.T) {
	w, r, _ := skillWorker(t, 1)
	f := &w.e.Store.Cfg.Fixture
	f.ExpandedHitBudget = &devconfig.ExpandedHitBudget{DamageCapBP: 2000, AbsorptionCapBP: 2000}
	f.BodyTraining = &devconfig.BodyTrainingConfig{
		Tracks: []devconfig.BodyTrainingTrack{{ID: devconfig.BodyAbsorption, BonusPerRank: 100, ShareCapBP: 1200}},
		Steps:  make([]devconfig.BodyTrainingStep, 12),
	}
	r.Role.Level = 20
	r.Role.BodyTraining = &domain.BodyTrainingState{Absorption: 1}
	wolf := &entity{Kind: 3}
	// 盾先扣原伤害的 20%，再减 8 点原防御，得到 800 的净伤。
	r.Player.Statuses = map[uint32]*combatStatus{31: {ID: 31, TargetLife: r.Player.Life, DamageReductionPercent: 20}}
	net := statusDamage(&r.Player, 1010, 8)
	amount := func(from, target *entity, damage int64, context []hitContext) int64 {
		value, critical := w.expandedHitDamage(r, from, target, damage, context)
		if critical {
			t.Fatal("只启用金币炼体时出现暴击")
		}
		return value
	}
	if net != 800 || amount(wolf, &r.Player, net, nil) != 792 {
		t.Fatal("原防御、魔法盾与普通场景吸收的结算顺序不符", net)
	}
	r.bossBattle = &personalBossBattle{fighter: devconfig.Fighter{HitBonuses: devconfig.HitBonuses{DamageBP: 1000, AbsorptionBP: 1200}}}
	if amount(wolf, &r.Player, net, nil) != 704 || amount(&r.Player, wolf, net, nil) != 880 {
		t.Fatal("挑战结算应该使用入场时固定的属性")
	}
	if amount(&r.Player, wolf, net, []hitContext{{PeriodicPoison: true}}) != net ||
		amount(&r.Player, wolf, net, []hitContext{{InstantKill: true}}) != net ||
		amount(&r.Player, wolf, 0, nil) != 0 {
		t.Fatal("绿毒、圣言或未命中发生变化")
	}
}
