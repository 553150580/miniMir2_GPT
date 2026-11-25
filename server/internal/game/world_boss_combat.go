// 本文件在一个共享 actor 内推进全场命中顺序、占格、唯一首领 AI 和跨角色观察。
package game

import (
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	p "minimir/internal/protocol"
	"sort"
	"time"
)

// sortedSeats 以稳定句柄排序，不让 Go map 遍历次序决定同一逻辑帧先后。
func (a *worldBossArena) sortedSeats() []*worldBossSeat {
	seats := make([]*worldBossSeat, 0, len(a.seats))
	for _, seat := range a.seats {
		seats = append(seats, seat)
	}
	sort.Slice(seats, func(i, j int) bool { return seats[i].room.Player.Handle < seats[j].room.Player.Handle })
	return seats
}

// eachLiving 访问场次的全部活体占格，包含尚未 ACK 的已预约落点。
func (a *worldBossArena) eachLiving(visit func(*entity)) {
	if a.boss.HP > 0 {
		visit(a.boss)
	}
	for _, seat := range a.seats {
		if seat.observation.revoked.Load() {
			continue
		}
		if child := seat.room; child.Player.HP > 0 {
			visit(&child.Player)
			if child.Pet != nil && child.Pet.HP > 0 {
				visit(child.Pet)
			}
		}
	}
}

// occupied 判断真实场次的下一格，身体之外的观察副本不参与碰撞。
func (a *worldBossArena) occupied(point devconfig.Point, self *entity) bool {
	occupied := false
	a.eachLiving(func(body *entity) {
		if body != self && body.Pos == point {
			occupied = true
		}
	})
	return occupied
}

// appendWorldBossViews 把其他已确认角色和宠物的当前权威身体投影到客户端基线。
func (w *worker) appendWorldBossViews(observer *room, baseline *pb.SceneBaselineChunk) {
	arena := observer.worldBossCombat
	if arena == nil {
		return
	}
	for _, seat := range arena.sortedSeats() {
		peer := seat.room
		if peer == observer || !peer.Ready || seat.observation.revoked.Load() {
			continue
		}
		avatar := w.worldBossBodyView(peer, &peer.Player)
		avatar.DisplayName = peer.Role.Name
		baseline.Entities = append(baseline.Entities, avatar)
		if peer.Pet != nil {
			baseline.Entities = append(baseline.Entities, w.worldBossBodyView(peer, peer.Pet))
		}
		for _, corpse := range peer.currentWorld().PetCorpses {
			baseline.Entities = append(baseline.Entities, w.worldBossBodyView(peer, corpse.Body))
		}
	}
}

// worldBossBodyView 包含渲染远端实际动作与位置需要的只读状态。
func (w *worker) worldBossBodyView(child *room, body *entity) *pb.EntityState {
	stats := w.fighter(child, body)
	return &pb.EntityState{EntityHandle: body.Handle, EntityKind: body.Kind, DefinitionId: body.Def, Position: pointPB(body.Pos), Direction: body.Direction,
		Hp: body.HP, HpMax: body.MaxHP, Mp: body.MP, MpMax: body.MaxMP, Action: body.Action, MoveIntervalMs: uint32(stats.MoveMS),
		AttackSequence: body.AttackSequence, CastSequence: body.CastSequence, CastSkillId: body.CastSkillID, AttackSkillId: body.AttackSkillID,
		AttackSkillLevel: body.AttackSkillLevel, AttackIntervalMs: uint32(stats.AttackMS), Life: body.Life, MovementRevision: body.MovementRevision,
		MovementKind: body.MovementKind, IsPetCorpse: body.IsPetCorpse, Statuses: statusesPB(body, time.Now())}
}

// worldBossDue 把每个角色行动的原世界身份和到期时间合并进入同一场次顺序。
type worldBossDue struct {
	room  *room    // 原已受理动作的角色战斗上下文。
	event dueEvent // 通用攻击、施法解析器接受的原事件。
}

// advanceWorldBossEvents 先汇总再排序，时钟相同时使用稳定席位和原动作顺序仲裁。
func (w *worker) advanceWorldBossEvents(arena *worldBossArena, until time.Time) {
	var all []worldBossDue
	for _, seat := range arena.sortedSeats() {
		child := seat.room
		if !child.Ready || child.Isolated || seat.observation.revoked.Load() {
			continue
		}
		world := child.currentWorld()
		attacks := world.Attacks[:0]
		for _, attack := range world.Attacks {
			if attack.Due.After(until) {
				attacks = append(attacks, attack)
				continue
			}
			copy := attack
			all = append(all, worldBossDue{room: child, event: dueEvent{World: world, At: copy.Due, Order: copy.Order, Attack: &copy}})
			child.InflightEvents++
		}
		casts := world.Casts[:0]
		for _, cast := range world.Casts {
			if cast.Due.After(until) {
				casts = append(casts, cast)
				continue
			}
			copy := cast
			all = append(all, worldBossDue{room: child, event: dueEvent{World: world, At: copy.Due, Order: copy.Order, Cast: &copy}})
			child.InflightEvents++
		}
		world.Attacks, world.Casts = attacks, casts
	}
	sort.SliceStable(all, func(i, j int) bool {
		left, right := all[i], all[j]
		if !left.event.At.Equal(right.event.At) {
			return left.event.At.Before(right.event.At)
		}
		if left.room.Player.Handle != right.room.Player.Handle {
			return left.room.Player.Handle < right.room.Player.Handle
		}
		return left.event.Order < right.event.Order
	})
	for _, job := range all {
		job.room.InflightEvents--
		if arena.challenge.finish == nil && !job.room.Isolated && job.room.Player.HP > 0 {
			w.resolveDueEvent(job.room, job.event, until)
		}
	}
}

// tickWorldBosses 仅处理本分片承载的场次，并保留提前结束窗口防止重开第二只首领。
func (w *worker) tickWorldBosses(now time.Time) {
	if w.e.stopped.Load() {
		return
	}
	for _, ledger := range w.worldBossLedgers {
		w.advanceWorldBossOpening(ledger, now)
	}
	// 每个已承载的运行服只保留最后一场终态，直到下一场替换。
	for _, arena := range w.worldBossArenas {
		if (arena.result != nil || arena.finishError != 0) && len(arena.seats) == 0 {
			continue
		}
		w.stepWorldBoss(arena, now)
	}
}

// stepWorldBoss 将普通角色动作和首领单次 AI 排在同一个场次时钟内。
func (w *worker) stepWorldBoss(arena *worldBossArena, now time.Time) {
	if now.Before(arena.lastTick) {
		return
	}
	arena.lastTick = now
	for _, seat := range arena.sortedSeats() {
		child, obs := seat.room, seat.observation
		if obs.revoked.Load() || !obs.conn.Alive() || obs.conn.Binding() != uint64(obs.auth.SessionEpoch) || child.Isolated ||
			!child.Ready && !now.Before(child.AwaitUntil) && now.Before(arena.occurrence.closesAt) {
			if arena.challenge.finish == nil && now.Before(arena.occurrence.closesAt) && shouldSuspendWorldBoss(seat) {
				w.suspendWorldBossSeat(arena, seat)
			} else {
				w.removeWorldBossSeat(arena, seat, worldBossLeft)
			}
		}
	}
	if arena.challenge.finish == nil {
		until := minBossTime(now, arena.occurrence.closesAt.Add(-time.Nanosecond))
		tickEntityStatuses([]*entity{arena.boss}, until)
		w.advanceWorldBossEvents(arena, until)
		arena.challenge.expire(now)
	}
	if arena.challenge.finish != nil {
		finish := arena.challenge.finish
		clear(arena.resumable)
		w.persistWorldBossFinish(arena, now)
		if arena.result == nil {
			if arena.finishError != 0 {
				for _, seat := range arena.sortedSeats() {
					w.removeWorldBossSeat(arena, seat, worldBossLeft)
				}
			} else if now.Sub(arena.lastState) >= 200*time.Millisecond {
				arena.lastState = now
				for _, seat := range arena.sortedSeats() {
					if seat.shown {
						w.send(seat.room, p.MsgS2CWorldBossState, 0, arena.state(seat.room.Scene, worldBossFighting))
					}
				}
			}
			return
		}
		for _, seat := range arena.sortedSeats() {
			if !seat.done && seat.shown {
				w.snapshot(seat.room)
				w.send(seat.room, p.MsgS2CWorldBossState, 0, arena.state(seat.room.Scene, worldBossFighting))
				seat.done = true
			}
			if !now.Before(finish.finishedAt.Add(arena.config.ReturnDelay)) {
				w.removeWorldBossSeat(arena, seat, worldBossFighting)
			}
		}
		return
	}
	for _, seat := range arena.sortedSeats() {
		child := seat.room
		if !child.Ready || now.Before(child.LastTick) {
			continue
		}
		if child.Player.HP <= 0 {
			w.removeWorldBossSeat(arena, seat, worldBossDefeated)
			continue
		}
		child.LastTick = now
		w.expireModes(child, now)
		changed := tickEntityStatuses([]*entity{&child.Player, child.Pet}, now)
		changed = w.tickHealing(child, now) || changed
		if w.expirePetCorpses(child, now) || changed {
			w.snapshot(child)
		}
		w.stepGridCombat(child, &arena.config.Map, now, false)
	}
	w.stepWorldBossMonster(arena, now)
	if now.Sub(arena.lastState) >= 200*time.Millisecond && len(arena.seats) > 0 {
		arena.lastState = now
		status := arena.state(0, worldBossFighting)
		for _, seat := range arena.sortedSeats() {
			if child := seat.room; child.Ready {
				status.SceneEpoch = child.Scene
				w.send(child, p.MsgS2CWorldBossState, 0, status)
			}
		}
	}
}

// stepWorldBossMonster 由全场最近的存活身体决定目标，整场只运行一次首领移动或攻击。
func (w *worker) stepWorldBossMonster(arena *worldBossArena, now time.Time) {
	if arena.boss.HP <= 0 || arena.challenge.finish != nil {
		return
	}
	clearFinishedAction(arena.boss, now)
	var victim *room
	var body *entity
	for _, seat := range arena.sortedSeats() {
		child := seat.room
		if !child.Ready || child.Player.HP <= 0 || child.Isolated || seat.observation.revoked.Load() {
			continue
		}
		for _, target := range []*entity{&child.Player, child.Pet} {
			if target != nil && target.HP > 0 && (body == nil || distance(arena.boss.Pos, target.Pos) < distance(arena.boss.Pos, body.Pos)) {
				victim, body = child, target
			}
		}
	}
	if victim == nil {
		return
	}
	if distance(arena.boss.Pos, body.Pos) <= 1 {
		w.attack(victim, arena.boss, body, now)
	} else {
		w.move(victim, arena.boss, body.Pos, now, arena.config.Monster.MoveMS)
	}
}
