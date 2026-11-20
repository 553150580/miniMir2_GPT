package game

import (
	"time"

	"minimir/internal/devconfig"
)

func skillRequiresFlight(skill devconfig.Skill) bool {
	return skill.RequireLineOfSight || skill.TargetKind == "" && skill.Behavior == "fireball"
}

// 自动普攻只使用当前脚点能打到的目标，刺杀沿用真实第二格判定。
func (w *worker) autoAttack(r *room, now time.Time) {
	if target := w.nearest(r, &r.Player); target != nil && distance(r.Player.Pos, target.Pos) <= 1 {
		w.attack(r, &r.Player, target, now)
		return
	}
	if !w.sourceTimeline() || !r.Melee.Thrusting {
		return
	}
	for _, target := range r.Monsters[:r.MonsterCount] {
		direction := facing(r.Player.Pos, target.Pos, r.Player.Direction)
		if target.HP <= 0 || target.Pos != stepPoint(r.Player.Pos, direction, 2) {
			continue
		}
		skill, _ := w.selectedAttack(r, direction, now)
		if skill.Behavior == "thrusting" {
			w.attackFacing(r, direction, skill.ID, 0, now)
			return
		}
	}
}

func (w *worker) automaticHealingTarget(r *room, pet *entity, rangeCells int) *entity {
	for _, candidate := range []*entity{&r.Player, pet} {
		if candidate != nil && candidate.HP > 0 && candidate.HP < candidate.MaxHP && candidate.HealAmount == 0 &&
			distance(r.Player.Pos, candidate.Pos) <= rangeCells {
			return candidate
		}
	}
	return nil
}

// 以当前脚点寻找最近的合法施放中心；同距离保留原怪物槽顺序。
func (w *worker) automaticEnemyTarget(r *room, skill devconfig.Skill, area bool) *entity {
	var ready *entity
	for _, candidate := range r.Monsters[:r.MonsterCount] {
		if candidate.HP <= 0 {
			continue
		}
		dist := distance(r.Player.Pos, candidate.Pos)
		if dist > skill.RangeCells || skillRequiresFlight(skill) && !w.lineOfSight(r, r.Player.Pos, candidate.Pos) {
			continue
		}
		if area && len(w.areaTargets(r, skill, candidate.Pos)) < skill.AutoMinTargets {
			continue
		}
		if ready == nil || dist < distance(r.Player.Pos, ready.Pos) {
			ready = candidate
		}
	}
	return ready
}
