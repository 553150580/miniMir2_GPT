// 本文件处理角色暴击、攻速和全抗的实时或冻结投影，以及独立控制状态。
package game

import (
	"time"

	"minimir/internal/devconfig"
)

// roleMasteryBonuses 统一合并角色炼体及附体的暴击、攻速和抵抗贡献。
func (w *worker) roleMasteryBonuses(r *room) devconfig.MasteryBonuses {
	f := &w.e.Store.Cfg.Fixture
	state := r.Role.BodyTrainingLevels()
	return f.MasteryCombat.Combine(f.BodyMasteryBonuses(r.Role.Level, state.Critical, state.Speed, state.Resistance), w.soulBonuses(r).Mastery)
}

// masteryBonuses 保留挑战入场时的值。普通交战只读取当前角色的确认聚合。
func (w *worker) masteryBonuses(r *room, e *entity) devconfig.MasteryBonuses {
	rules := w.e.Store.Cfg.Fixture.MasteryCombat
	if rules == nil || e == nil || e.Kind != 1 {
		return devconfig.MasteryBonuses{}
	}
	if r.bossBattle != nil {
		return rules.Combine(w.fighter(r, e).MasteryBonuses)
	}
	return w.roleMasteryBonuses(r)
}

// criticalExtraBP 在原直接攻击已命中时掷一次点，返回能产生实际增量的暴击额度。
func (w *worker) criticalExtraBP(r *room, from *entity, damage int64, context []hitContext) uint32 {
	rules := w.e.Store.Cfg.Fixture.MasteryCombat
	if rules == nil || len(context) == 0 || !context[0].DirectCritical || damage <= 0 ||
		devconfig.ReduceByBP(damage, rules.CriticalExtraBP) == damage {
		return 0
	}
	chance := w.masteryBonuses(r, from).CriticalBP
	if chance != 0 && w.draw(10_000) < int64(chance) {
		return rules.CriticalExtraBP
	}
	return 0
}

// masteryControlActive 检查此生命当前是否还被指定状态控制。
func masteryControlActive(e *entity, id uint32, now time.Time) bool {
	status := e.Statuses[id]
	return status != nil && status.TargetLife == e.Life && status.NextProcess.After(now)
}

// masteryControlHeld 只限制新的自主移动、攻击和施法，已受理动作仍走原时间线。
func masteryControlHeld(e *entity, now time.Time) bool {
	return len(e.Statuses) != 0 && (masteryControlActive(e, devconfig.MasteryFrozen, now) ||
		masteryControlActive(e, devconfig.MasteryParalysis, now) || masteryControlActive(e, devconfig.MasteryPetrified, now))
}

// applyMonsterControl 只处理实际怪物的已确认命中。每种状态独立归属且同种未到期时不刷新。
func (w *worker) applyMonsterControl(r *room, world *worldState, from, target *entity, now time.Time) {
	rules := w.e.Store.Cfg.Fixture.MasteryCombat
	if rules == nil || len(rules.MonsterControls) == 0 || target != &r.Player || target.HP <= 0 ||
		world != r.currentWorld() || from.Kind != 3 || from.Monster == nil {
		return
	}
	control, exists := rules.MonsterControls[from.Monster.ID]
	if !exists || masteryControlActive(target, control.StatusID, now) {
		return
	}
	chance, duration := int64(control.ChancePPM), int64(control.DurationMS)
	resistance := w.masteryBonuses(r, target).ResistanceBP
	if control.StatusID == devconfig.MasteryParalysis {
		// 麻痹抵抗按原触发率比例削减，已经触发时采用原控制时间。
		chance = devconfig.ReduceByBP(chance, resistance)
	} else {
		// 冰冻、石化保留各自原触发率，以毫秒为单位缩短动作锁。
		duration = devconfig.ReduceByBP(duration, resistance)
	}
	if chance < int64(devconfig.BodyChancePPM) && w.draw(int64(devconfig.BodyChancePPM)) >= chance {
		return
	}
	if target.Statuses == nil {
		target.Statuses = make(map[uint32]*combatStatus)
	}
	target.Statuses[control.StatusID] = &combatStatus{
		ID: control.StatusID, Caster: referenceIn(world, from), TargetLife: target.Life, RemoveOnDeath: true,
		RemainingMS: duration, LastProcess: now, NextProcess: now.Add(time.Duration(duration) * time.Millisecond), TickMS: duration,
	}
}
