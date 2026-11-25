// 本文件按同一时间线仲裁镜像双方的行动、伤害和超时，以临时胜负交接场次账本。
package game

import (
	"slices"
	"time"

	"minimir/internal/devconfig"
)

func (b *mirrorCombat) acceptsActions(at time.Time) bool {
	return b.result.Cause == 0 && !at.Before(b.startedAt) && at.Before(b.deadline)
}

func (b *mirrorCombat) finish(winner, cause uint32, at time.Time) {
	if b.result.Cause != 0 {
		return
	}
	b.result = mirrorCombatResult{Winner: winner, Cause: cause, FinishedAt: at}
	for _, side := range b.sides {
		world := side.room.currentWorld()
		world.Attacks, world.Casts, world.FireWalls = nil, nil, nil
	}
}

// survivingLife 仅计算原本人及原入场宠物的剩余生命；死亡的宠物仍计入冻结的分母。
func (s *mirrorCombatSide) survivingLife() int64 {
	life := s.room.Player.HP
	if s.room.Pet != nil {
		life += s.room.Pet.HP
	}
	return life
}

func (b *mirrorCombat) checkDefeat(at time.Time) {
	attacker, defender := b.sides[0].survivingLife(), b.sides[1].survivingLife()
	if attacker == 0 {
		b.finish(2, mirrorDefeat, at)
	} else if defender == 0 {
		b.finish(1, mirrorDefeat, at)
	}
}

func (b *mirrorCombat) finishTimeout() {
	left, right := &b.sides[0], &b.sides[1]
	winner := uint32(2)
	// 比较精确的有理数；既不舍入生命百分比，也不按宠物是否尚存改变分母。
	if left.survivingLife()*right.initialLife > right.survivingLife()*left.initialLife {
		winner = 1
	}
	b.finish(winner, mirrorTimeout, b.deadline)
}

// advanceMirrorCombat 在原分片串行推进两套职业 AI。已受理的同刻命中全部处理后才判定团灭。
func (w *worker) advanceMirrorCombat(b *mirrorCombat, now time.Time) {
	if b == nil || b.result.Cause != 0 || now.Before(b.startedAt) || !b.lastTick.IsZero() && !now.After(b.lastTick) {
		return
	}
	until := minBossTime(now, b.deadline.Add(-time.Nanosecond))
	b.lastTick = now
	for _, side := range b.sides {
		child := side.room
		child.LastTick = until
		b.due = collectSharedCombatEvents(b.due, child, until)
	}
	sortSharedCombatEvents(b.due)
	for index, item := range b.due {
		item.room.InflightEvents--
		if b.acceptsActions(item.event.At) && !item.room.Isolated {
			if index == 0 || !item.event.At.Equal(b.due[index-1].event.At) {
				for _, side := range b.sides {
					tickEntityStatuses([]*entity{&side.room.Player, side.room.Pet}, item.event.At)
				}
			}
			// 伤害归原受理时刻；持续技的下一跳仍从实际处理帧推进，遵循原战斗的卡帧规则。
			w.resolveDueEvent(item.room, item.event, until)
		}
		if index+1 == len(b.due) || !item.event.At.Equal(b.due[index+1].event.At) {
			b.checkDefeat(item.event.At)
		}
	}
	clear(b.due)
	b.due = b.due[:0]
	if b.result.Cause != 0 {
		return
	}
	if b.sides[0].room.Isolated || b.sides[1].room.Isolated {
		b.finish(0, mirrorInterrupted, until)
		return
	}
	if !now.Before(b.deadline) {
		b.finishTimeout()
		return
	}
	for _, side := range b.sides {
		child := side.room
		clearFinishedAction(&child.Player, until)
		clearFinishedAction(child.Pet, until)
		w.expireModes(child, until)
		tickEntityStatuses([]*entity{&child.Player, child.Pet}, until)
		w.tickPotions(child, until)
		w.tickHealing(child, until)
		w.stepMirrorSide(child, until)
	}
	if b.sides[0].room.Isolated || b.sides[1].room.Isolated {
		b.finish(0, mirrorInterrupted, until)
		return
	}
	b.checkDefeat(until)
}

// stepMirrorSide 对攻守应用相同的技能队列、普通攻击和寻路。追击仅使用合法的另一阵营身体。
func (w *worker) stepMirrorSide(r *room, now time.Time) {
	if r.Player.HP > 0 {
		focus := r.liveAutoTarget()
		if focus == nil {
			focus = w.nearest(r, &r.Player)
		}
		if focus != nil {
			direction := facing(r.Player.Pos, focus.Pos, r.Player.Direction)
			accepted, need := w.tryAttackSkillSlots(r, focus, direction, 0, now, false)
			if !accepted && !w.tryManaPotion(r, need, now) {
				if distance(r.Player.Pos, focus.Pos) <= 1 && w.enemyAt(r, stepPoint(r.Player.Pos, direction, 1)) == focus {
					if w.attackSlotFallback(r, direction, 0, now, focus) == 0 {
						r.lockAutoTarget(reference(r, focus))
					}
				} else {
					w.move(r, &r.Player, focus.Pos, now, w.fighter(r, &r.Player).MoveMS)
				}
			}
		}
	}
	if r.Pet == nil || r.Pet.HP <= 0 {
		return
	}
	if focus := w.nearest(r, r.Pet); focus != nil && !w.tryPetAttack(r, r.Pet, focus, now) {
		w.move(r, r.Pet, focus.Pos, now, w.fighter(r, r.Pet).MoveMS)
	}
}

// applyMirrorDamage 只更新另一阵营的临时身体。死亡不进入普通场景的掉落、装备损耗或角色死亡流水。
func (w *worker) applyMirrorDamage(r *room, world *worldState, from, target *entity, damage int64, now time.Time, context []hitContext) {
	b := world.mirrorCombat
	if world != r.currentWorld() || from == nil || target == nil || damage <= 0 || target.HP <= 0 ||
		from != &r.Player && from != r.Pet || !slices.Contains(world.combatOpponents(), target) {
		return
	}
	at := now
	if len(context) != 0 && !context[0].At.IsZero() {
		at = context[0].At
	}
	if !b.acceptsActions(at) {
		return
	}
	exposeHiddenAttacker(world, from, target)
	if len(context) == 0 || !context[0].PeriodicPoison {
		target.ShockUntilMS = 0
	}
	wearStatuses(target, damage)
	target.HP = max(0, target.HP-damage)
	target.Action, target.ActionUntil = 3, now.Add(200*time.Millisecond)
	if target.HP != 0 {
		return
	}
	target.Action = 4
	removeDeathStatuses(target)
	opponent := b.sides[1-world.mirrorSide].room
	if target == &opponent.Player && opponent.Pet != nil && b.config.PetDeathPolicy == devconfig.MirrorArenaPetFollowsOwnerDeath {
		opponent.Pet.HP, opponent.Pet.Action = 0, 4
		removeDeathStatuses(opponent.Pet)
	}
}
