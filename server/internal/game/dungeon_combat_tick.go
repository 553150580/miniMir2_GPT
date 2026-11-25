// 本文件以统一时钟推进讨伐链的原动作、伤害和换段。
package game

import (
	"slices"
	"time"

	"minimir/internal/persistence"
)

// stepDungeonArena 先收束同场到期动作，再推进真人、宠物和每头敌人的一轮行动。
func (w *worker) stepDungeonArena(a *dungeonArena, now time.Time) {
	if a.outcome != persistence.DungeonActive || now.Before(a.lastTick) {
		return
	}
	until := minBossTime(now, a.grant.ExpiresAt.Add(-time.Nanosecond))
	a.lastTick = now
	a.eachSeat(func(seat *dungeonCombatSeat) {
		if seat.room.Ready {
			seat.started = true
		}
	})
	for _, monster := range a.monsters {
		clearFinishedAction(monster, until)
		tickEntityStatuses([]*entity{monster}, until)
	}
	w.advanceDungeonEvents(a, until)
	if a.outcome != persistence.DungeonActive {
		return
	}
	if !now.Before(a.grant.ExpiresAt) {
		a.end(persistence.DungeonFailed, a.grant.ExpiresAt)
		return
	}
	if !a.clearedAt.IsZero() {
		w.advanceDungeonStage(a, until)
	}
	a.eachSeat(func(seat *dungeonCombatSeat) {
		child := seat.room
		if !seat.started || child.Isolated || child.Player.HP <= 0 || !a.acceptsActions(until) {
			return
		}
		child.LastTick = until
		w.expireModes(child, until)
		changed := tickEntityStatuses([]*entity{&child.Player, child.Pet}, until)
		changed = w.tickHealing(child, until) || changed
		if w.expirePetCorpses(child, until) || changed {
			w.snapshot(child)
		}
		w.stepGridCombat(child, &a.geometry, until, child.Conn == nil || !child.Ready)
	})
	for _, monster := range a.monsters {
		w.stepDungeonMonster(a, monster, until)
	}
}

// advanceDungeonEvents 由唯一战斗分片决定同一时刻的命中先后，清空当前段后丢弃旧段后缀。
func (w *worker) advanceDungeonEvents(a *dungeonArena, until time.Time) {
	var events []sharedCombatDue
	a.eachSeat(func(seat *dungeonCombatSeat) {
		if child := seat.room; seat.started && !child.Isolated {
			events = collectSharedCombatEvents(events, child, until)
		}
	})
	sortSharedCombatEvents(events)
	for _, event := range events {
		child := event.room
		child.InflightEvents--
		if a.outcome == persistence.DungeonActive && a.clearedAt.IsZero() && !child.Isolated && child.Player.HP > 0 {
			w.resolveDueEvent(child, event.event, until)
		}
	}
}

// advanceDungeonStage 在最后一次命中处理完之后发布新段，保留巩固阶段的原角色和宠物状态。
func (w *worker) advanceDungeonStage(a *dungeonArena, now time.Time) {
	a.stage++
	a.openedAt = now
	a.clearedAt = time.Time{}
	a.openStage()
	a.eachSeat(func(seat *dungeonCombatSeat) {
		child := seat.room
		resetDungeonActions(child)
		world := child.currentWorld()
		world.Monsters, world.MonsterCount = a.monsters, len(a.monsters)
		world.Ref.ID++
		child.WorldSequence = world.Ref.ID
		if child.Pet != nil {
			child.PetWorld = world.Ref
		}
	})
	w.snapshotDungeonSeats(a, now)
}

// stepDungeonMonster 只为一头共享敌人选一次最近目标并执行业务，不按观察人数重复执行。
func (w *worker) stepDungeonMonster(a *dungeonArena, monster *entity, now time.Time) {
	if monster.HP <= 0 || monster.ShockUntilMS != 0 && now.UnixMilli() < monster.ShockUntilMS ||
		a.outcome != persistence.DungeonActive || !a.clearedAt.IsZero() {
		return
	}
	var victim *room
	var body *entity
	a.eachSeat(func(seat *dungeonCombatSeat) {
		child := seat.room
		if !seat.started || child.Isolated || child.Player.HP <= 0 {
			return
		}
		target := visibleMonsterTarget(child.currentWorld(), monster, &child.Player, child.Pet, child.Role.Level)
		if target != nil && (body == nil || distance(monster.Pos, target.Pos) < distance(monster.Pos, body.Pos)) {
			victim, body = child, target
		}
	})
	if victim == nil {
		return
	}
	if distance(monster.Pos, body.Pos) <= 1 {
		w.attack(victim, monster, body, now)
	} else {
		w.move(victim, monster, body.Pos, now, monster.Monster.MoveMS)
	}
}

// applyDungeonDamage 仅接收当前段、原身体之间的权威结果；永久资源由原角色所有者管理。
func (w *worker) applyDungeonDamage(r *room, from, target *entity, damage int64, now time.Time, context []hitContext) {
	a := r.dungeonCombat
	seat := a.seats[r.Role.ID]
	at := now
	if len(context) > 0 && !context[0].At.IsZero() {
		at = context[0].At
	}
	if seat == nil || seat.room != r || !seat.started || r.Isolated || from == nil || from.HP <= 0 || !a.acceptsActions(at) {
		return
	}
	enemy := slices.Contains(a.monsters, target)
	if enemy {
		if from != &r.Player && from != r.Pet {
			return
		}
	} else if target != &r.Player && target != r.Pet || !slices.Contains(a.monsters, from) {
		return
	}
	exposeHiddenAttacker(r.currentWorld(), from, target)
	wearStatuses(target, damage)
	target.HP = max(0, target.HP-damage)
	target.Action, target.ActionUntil = 3, now.Add(200*time.Millisecond)
	if len(context) == 0 || !context[0].PeriodicPoison {
		target.ShockUntilMS = 0
	}
	if target.HP > 0 {
		return
	}
	removeDeathStatuses(target)
	target.Action = 4
	if enemy && a.stage+1 == len(a.template.Stages) && target == a.monsters[0] {
		w.freezeDungeonRewards(a, at)
	}
	if enemy && !slices.ContainsFunc(a.monsters, func(body *entity) bool { return body.HP > 0 }) {
		a.clearedAt = at
		if a.stage+1 == len(a.template.Stages) {
			a.end(persistence.DungeonSucceeded, at)
		}
	} else if target == &r.Player {
		r.AutoTarget = actorRef{}
		if w.sourcePetLifecycle() {
			w.killOwnedPet(r, now)
		} else if r.Pet != nil {
			r.Pet.HP, r.Pet.Action = 0, 4
			removeDeathStatuses(r.Pet)
		}
		if a.everyPlayerDefeated() {
			a.end(persistence.DungeonFailed, at)
		}
	} else if target == r.Pet && w.petRuntimeEnabled() && !w.archivePetDeath(r, r.currentWorld(), reference(r, target), now) {
		w.isolateRoom(r)
	}
	w.snapshotDungeonSeats(a, now)
}

// everyPlayerDefeated 只在原批准的所有真人都确实失去生命后宣告战败。
func (a *dungeonArena) everyPlayerDefeated() bool {
	return len(a.seats) == len(a.grant.Members) && !slices.ContainsFunc(a.grant.Members, func(member persistence.DungeonMember) bool {
		return a.seats[member.RoleID].room.Player.HP > 0
	})
}
