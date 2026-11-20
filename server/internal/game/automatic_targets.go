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
	if locked := r.liveAutoTarget(); locked != nil {
		w.autoAttackMonster(r, locked, now)
		return
	}
	for _, cells := range []int{1, 2} {
		for _, target := range r.Monsters[:r.MonsterCount] {
			if target != nil && distance(r.Player.Pos, target.Pos) == cells && w.autoAttackMonster(r, target, now) {
				return
			}
		}
	}
}

func (w *worker) autoAttackMonster(r *room, target *entity, now time.Time) bool {
	cells := distance(r.Player.Pos, target.Pos)
	if target.HP <= 0 || cells < 1 || cells > 2 || cells == 2 && (!w.sourceTimeline() || !r.Melee.Thrusting) {
		return false
	}
	ref := reference(r, target)
	sequence := r.Player.AttackSequence
	if w.sourceTimeline() {
		direction := facing(r.Player.Pos, target.Pos, r.Player.Direction)
		w.attackFacingTarget(r, direction, 0, 0, now, target)
	} else {
		w.attack(r, &r.Player, target, now)
	}
	if sequence == r.Player.AttackSequence {
		return false
	}
	r.lockAutoTarget(ref)
	return true
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
