// 本文件将角色扩展命中属性接入各场景共用的生命结算。
package game

import "minimir/internal/devconfig"

// bodyHitBonuses 汇总当前角色已确认的来源，统一用于原场景和挑战冻结。
func (w *worker) bodyHitBonuses(r *room) devconfig.HitBonuses {
	fixture := &w.e.Store.Cfg.Fixture
	levels := r.Role.BodyTrainingLevels()
	return fixture.ExpandedHitBudget.Combine(fixture.BodyHitBonuses(r.Role.Level, levels.Damage, levels.Absorption))
}

// hitBonuses 优先使用已经冻结的挑战属性，普通场景直接计算角色的已确认贡献。
func (w *worker) hitBonuses(r *room, e *entity) devconfig.HitBonuses {
	if e == nil || e.Kind != 1 {
		return devconfig.HitBonuses{}
	}
	if r.bossBattle != nil {
		return w.fighter(r, e).HitBonuses
	}
	return w.bodyHitBonuses(r)
}

// expandedHitDamage 调整已经经过原命中、防御和魔法盾的净伤害。
func (w *worker) expandedHitDamage(r *room, from, target *entity, damage int64, context []hitContext) (int64, bool) {
	budget := w.e.Store.Cfg.Fixture.ExpandedHitBudget
	if budget == nil || damage <= 0 || len(context) > 0 && (context[0].PeriodicPoison || context[0].InstantKill) {
		return damage, false
	}
	var criticalExtra uint32
	if target != nil && target.HP > 0 {
		criticalExtra = w.criticalExtraBP(r, from, damage, context)
	}
	return budget.Apply(damage, w.hitBonuses(r, from).DamageBP, w.hitBonuses(r, target).AbsorptionBP, criticalExtra), criticalExtra != 0
}
