package game

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"math/big"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
	"time"
)

var directions = []devconfig.Point{{X: 0, Y: -1}, {X: 1, Y: -1}, {X: 1, Y: 0}, {X: 1, Y: 1}, {X: 0, Y: 1}, {X: -1, Y: 1}, {X: -1, Y: 0}, {X: -1, Y: -1}}

func random(n int64) int64 {
	if n <= 1 {
		return 0
	}
	x, e := rand.Int(rand.Reader, big.NewInt(n))
	if e != nil {
		panic(e)
	}
	return x.Int64()
}
func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
func distance(a, b devconfig.Point) int {
	x, y := abs(a.X-b.X), abs(a.Y-b.Y)
	if x > y {
		return x
	}
	return y
}
func (w *worker) free(r *room, q devconfig.Point, self *entity) bool {
	if !w.walkable(r, q) {
		return false
	}
	all := []*entity{&r.Player}
	if pet := r.worldPet(r.currentWorld()); pet != nil {
		all = append(all, pet)
	}
	all = append(all, r.Monsters[:r.MonsterCount]...)
	for _, e := range all {
		if e != self && e.HP > 0 && e.Pos == q {
			return false
		}
	}
	return true
}
func (w *worker) neighbor(r *room, from, to devconfig.Point, self *entity) bool {
	if !w.free(r, to, self) {
		return false
	}
	dx, dy := to.X-from.X, to.Y-from.Y
	if dx != 0 && dy != 0 {
		if !w.walkable(r, devconfig.Point{X: from.X + dx, Y: from.Y}) || !w.walkable(r, devconfig.Point{X: from.X, Y: from.Y + dy}) {
			return false
		}
	}
	return true
}
func (w *worker) move(r *room, e *entity, target devconfig.Point, now time.Time, interval int64) {
	if r.petAway(e) {
		return
	}
	if w.moveTown(r, e, target, now, interval) {
		if e == &r.Player {
			w.finishFailedTownNavigation(r)
		}
		return
	}
	if now.Before(e.NextMove) || now.Before(e.ActionReadyAt) || e.HP <= 0 || e.Pos == target {
		return
	}
	start := e.Pos
	budget := w.mapCellBudget(r)
	if budget == 0 || !w.inBounds(r, start) || !w.inBounds(r, target) {
		return
	}
	queue := []devconfig.Point{start}
	seen := map[devconfig.Point]bool{start: true}
	first := map[devconfig.Point]devconfig.Point{}
	var next *devconfig.Point
	// 每个合法格至多发现一次；访问预算随已校验地图格数增长，避免17格绕路被旧256截断。
	for head := 0; head < len(queue) && head < budget; head++ {
		cur := queue[head]
		for _, d := range directions {
			q := devconfig.Point{X: cur.X + d.X, Y: cur.Y + d.Y}
			if seen[q] || !w.neighbor(r, cur, q, e) {
				continue
			}
			seen[q] = true
			if cur == start {
				first[q] = q
			} else {
				first[q] = first[cur]
			}
			if q == target || distance(q, target) < distance(start, target) && distance(q, target) == 1 {
				candidate := first[q]
				next = &candidate
				break
			}
			queue = append(queue, q)
		}
		if next != nil {
			break
		}
	}
	if next == nil {
		return
	}
	direction := e.Direction
	for i, d := range directions {
		if next.X-start.X == d.X && next.Y-start.Y == d.Y {
			direction = uint32(i)
			break
		}
	}
	w.commitWalk(r, e, *next, direction, now, interval)
}
func (w *worker) fighter(r *room, e *entity) devconfig.Fighter {
	f := &w.e.Store.Cfg.Fixture
	if e.Kind == 1 {
		a, _ := f.PlayerAtLevel(r.Role.Job, r.Role.Level)
		for _, item := range r.Role.Equipped {
			if item.Slot == devconfig.CastingSupplySlot {
				continue
			}
			def, _ := f.Item(item.Item.ItemID)
			a.AttackMin += def.AttackBonus + def.AttackMinBonus
			a.AttackMax += def.AttackBonus + def.AttackMaxBonus
			a.AttackSpeed += def.AttackSpeedBonus
			a.Defence += def.DefenceBonus
			a.DefenceMin += def.DefenceMinBonus
			a.DefenceMax += def.DefenceMaxBonus
			a.MagicDefenceMin += def.MagicDefenceMinBonus
			a.MagicDefenceMax += def.MagicDefenceMaxBonus
		}
		for _, skill := range f.Skills {
			if level, learned := r.Role.Skill(skill.ID); learned && (skill.Behavior == "fencing" || skill.Behavior == "spirit_sword" || skill.Behavior == "slaying") && skill.JobID == r.Role.Job && level >= skill.LearnedLevel && level <= skill.MaxLevel {
				a.Accuracy += skill.AccuracyBonus(level)
				a.AttackMax += skill.MaximumAttackBonus(level)
			}
		}
		if w.sourceTimeline() {
			a.AttackMS = f.CombatTiming.PlayerAttackInterval(r.Role.Level, a.AttackSpeed)
		}
		return statusFighter(e, a)
	}
	if e.Kind == 2 {
		return statusFighter(e, f.Pet)
	}
	return statusFighter(e, e.Monster.Fighter)
}
func (w *worker) nearest(r *room, from *entity) *entity {
	if r.petAway(from) {
		return nil
	}
	var best *entity
	for _, m := range r.Monsters[:r.MonsterCount] {
		if m.HP > 0 && (best == nil || distance(from.Pos, m.Pos) < distance(from.Pos, best.Pos)) {
			best = m
		}
	}
	return best
}
func (w *worker) attack(r *room, from, target *entity, now time.Time, input ...uint32) {
	if w.mapCellBudget(r) == 0 || r.petAway(from) || r.petAway(target) {
		return
	}
	if from.Kind == 1 && w.sourceTimeline() {
		if target == nil || target.HP <= 0 || distance(from.Pos, target.Pos) > 1 {
			return
		}
		sequence := uint32(0)
		if len(input) > 0 {
			sequence = input[0]
		}
		w.attackFacing(r, facing(from.Pos, target.Pos, from.Direction), 0, sequence, now)
		return
	}
	// 攻击不依赖移动节流；保留本作>=截止边界，来源怪物严格>的调度差异不在本次改变。
	if r.Pending || from.HP <= 0 || target.HP <= 0 || now.Before(from.NextAttack) || now.Before(from.ActionReadyAt) || distance(from.Pos, target.Pos) > 1 {
		return
	}
	dx, dy := target.Pos.X-from.Pos.X, target.Pos.Y-from.Pos.Y
	if dx != 0 && dy != 0 && (!w.walkable(r, devconfig.Point{X: from.Pos.X + dx, Y: from.Pos.Y}) || !w.walkable(r, devconfig.Point{X: from.Pos.X, Y: from.Pos.Y + dy})) {
		return
	}
	a, b := w.fighter(r, from), w.fighter(r, target)
	damage := w.physicalDamage(r, from, target)
	if w.e.Store.Cfg.Fixture.DamageFormula != "development_source_combat" {
		damage = w.interval(a.AttackMin, a.AttackMax) - b.Defence
		if damage < 1 {
			damage = 1
		}
	}
	from.AttackSequence++
	from.NextAttack = now.Add(time.Duration(a.AttackMS) * time.Millisecond)
	from.Action = 2
	from.ActionUntil = now.Add(200 * time.Millisecond)
	for i, d := range directions {
		if (dx == 0 && d.X == 0 || dx > 0 && d.X > 0 || dx < 0 && d.X < 0) && (dy == 0 && d.Y == 0 || dy > 0 && d.Y > 0 || dy < 0 && d.Y < 0) {
			from.Direction = uint32(i)
			break
		}
	}
	w.applyDamage(r, from, target, damage, now)
}
func (w *worker) applyDamage(r *room, from, target *entity, damage int64, now time.Time, context ...hitContext) {
	w.applyDamageInWorld(r, r.currentWorld(), from, target, damage, now, context...)
}
func (w *worker) applyDamageInWorld(r *room, world *worldState, from, target *entity, damage int64, now time.Time, context ...hitContext) {
	if damage <= 0 || target.HP <= 0 {
		return
	}
	wearStatuses(target, damage)
	target.HP -= damage
	if target.HP < 0 {
		target.HP = 0
	}
	target.Action = 3
	target.ActionUntil = now.Add(200 * time.Millisecond)
	if target.HP == 0 {
		removeDeathStatuses(target)
		target.Action = 4
		if target.Kind == 3 {
			target.RespawnAt = now.Add(10 * time.Second)
			if len(context) > 0 && w.sourceTimeline() {
				until := r.Role.PrivilegeUntil
				if context[0].PrivilegeCaptured {
					until = context[0].PrivilegeUntil
				}
				w.enqueueKillFactInWorld(r, world, target, context[0].At, from, context[0].HostSession, until)
			} else {
				w.kill(r, target, now, from)
			}
		} else if target.Kind == 1 {
			r.DeathID = domain.ID()
			w.deathNotice(r)
			if r.Conn == nil {
				if w.sourceTimeline() {
					r.KillFacts = append(r.KillFacts, killFact{At: now, HostSession: r.HostSession, StopReason: 2})
				} else {
					w.settle(r, persistence.Settlement{SourceSeq: r.Role.SourceSeq + 1, HostingSession: r.HostSession, StopReason: 2})
				}
			}
		}
		// 致死HP/动作属于临时现场，立即同步；奖励仍只在资产事务提交后推送。
		r.LastBroadcast = now
		if world == r.currentWorld() {
			w.snapshot(r)
		}
	}
}
func (w *worker) step(r *room, now time.Time) {
	if w.e.stopped.Load() || r.Isolated {
		return
	}
	if r.Scene == 0 {
		return
	}
	mapTemplate, ok := w.currentMap(r)
	if !ok {
		w.isolateRoom(r)
		return
	}
	if r.Conn != nil && !r.Ready {
		if now.After(r.AwaitUntil) {
			r.Conn.Close("baseline acknowledgement timeout")
		}
	}
	if r.LastTick.IsZero() {
		r.LastTick = now
	}
	delta := now.Sub(r.LastTick)
	r.LastTick = now
	if delta < 0 {
		delta = 0
	}
	if delta > 200*time.Millisecond {
		delta = 200 * time.Millisecond
	}
	if r.Pending {
		w.progressMapTravel(r, now)
		if r.PendingRetry != nil && !now.Before(r.RetryAt) {
			j := *r.PendingRetry
			if w.submit(j) {
				r.PendingRetry = nil
			}
		}
		return
	}
	w.expireModes(r, now)
	// A28：本作统一在本帧事件前处理全部实体状态；按实际处理时钟离散扣时，不回造Due历史。
	if w.tickStatuses(r, now) {
		w.snapshot(r)
	}
	w.flushKillFacts(r)
	if r.Pending || len(r.KillFacts) > 0 {
		return
	}
	// 已经释放的法术与恢复队列不属于后续离线战斗授权；未知事务仍在上方冻结。
	activeBeforeEvents, sceneBeforeEvents := r.currentWorld(), r.Scene
	reactive := w.reactiveWorlds(r)
	w.advanceEvents(r, now)
	for _, world := range w.reactiveWorlds(r) {
		found := false
		for _, earlier := range reactive {
			if earlier == world {
				found = true
			}
		}
		if !found {
			reactive = append(reactive, world)
		}
	}
	for _, world := range reactive {
		if !r.Pending && !r.Isolated {
			w.worldDeadlines(r, world, now)
		}
	}
	w.flushKillFacts(r)
	if r.Pending || r.Isolated {
		return
	}
	if r.currentWorld() != activeBeforeEvents || r.Scene != sceneBeforeEvents {
		return
	}
	if w.tickHealing(r, now) {
		w.snapshot(r)
	}
	offline := r.Conn == nil
	if r.Player.HP <= 0 {
		w.progressMapTravel(r, now)
		return
	}
	if w.tickPotions(r, now) {
		// 恢复量已经由权威场景应用，立即同步；不等待下一次战斗或奖励事务。
		w.snapshot(r)
	}
	if r.Player.HP >= r.Player.MaxHP {
		// Crystal ProcessRegen统一结算满值后清除所有HP恢复余额，不能供同帧受击后复用。
		r.Player.HealAmount = 0
		r.Potions.HP = 0
	}
	if w.progressMapTravel(r, now) {
		return
	}
	if mapTemplate.SceneKindValue() == devconfig.SceneKindTown {
		// 来源城镇不运行离线战斗或扣托管；既有状态/恢复已在上方按真实时钟推进。
		r.HostMS = 0
		if offline {
			return
		}
	}
	if offline && (r.Role.HostingSeconds <= 0 || r.Role.PickupStopped || !r.Auto) {
		return
	}
	if offline {
		r.HostMS += delta.Milliseconds()
		if r.HostMS >= 1000 {
			w.settle(r, persistence.Settlement{SourceSeq: r.Role.SourceSeq + 1, HostSeconds: 1, HostingSession: r.HostSession})
			r.HostMS -= 1000
			return
		}
	}
	if r.Player.HP <= 0 {
		return
	}
	for _, e := range append(append([]*entity{&r.Player}, r.Monsters...), r.Pet) {
		if e != nil && e.HP > 0 && !now.Before(e.ActionUntil) {
			e.Action = 0
		}
	}
	w.worldDeadlines(r, r.currentWorld(), now)
	if r.Isolated {
		return
	}
	// 原截止和恢复照常推进；新地图必须先收到匹配ACK才允许新增移动/AI/攻击。
	if r.Conn != nil && !r.Ready || r.Travel != nil && r.Travel.Phase == travelCommitted && !r.Travel.Acked {
		return
	}
	a := w.fighter(r, &r.Player)
	if r.Manual {
		d := directions[r.ManualDirection]
		target := devconfig.Point{X: r.Player.Pos.X + d.X, Y: r.Player.Pos.Y + d.Y}
		if w.neighbor(r, r.Player.Pos, target, &r.Player) && !now.Before(r.Player.NextMove) && !now.Before(r.Player.ActionReadyAt) {
			w.commitWalk(r, &r.Player, target, r.ManualDirection, now, a.MoveMS)
		}
	} else if r.Destination != nil {
		if r.Player.Pos == *r.Destination {
			if r.Navigation.InputSequence != 0 && r.Navigation.Status == navigationActive {
				r.Navigation.Status = navigationArrived
				w.snapshot(r)
			}
			r.Destination = nil
		} else {
			w.move(r, &r.Player, *r.Destination, now, a.MoveMS)
		}
	} else if r.Auto {
		if target, handled := w.autoSkill(r, now); handled {
			if target != nil {
				w.move(r, &r.Player, target.Pos, now, a.MoveMS)
			}
		} else if target := w.nearest(r, &r.Player); target != nil {
			if distance(r.Player.Pos, target.Pos) <= 1 {
				w.attack(r, &r.Player, target, now)
			} else {
				w.move(r, &r.Player, target.Pos, now, a.MoveMS)
			}
		}
	}
	if r.Pending || r.Isolated {
		return
	}
	pet := r.worldPet(r.currentWorld())
	if pet != nil && pet.HP > 0 {
		if r.Auto && mapTemplate.SceneKindValue() != devconfig.SceneKindTown {
			if target := w.nearest(r, pet); target != nil {
				if distance(pet.Pos, target.Pos) <= 1 {
					w.attack(r, pet, target, now)
				} else {
					w.move(r, pet, target.Pos, now, w.e.Store.Cfg.Fixture.Pet.MoveMS)
				}
			}
		} else if distance(pet.Pos, r.Player.Pos) > 2 {
			w.move(r, pet, r.Player.Pos, now, w.e.Store.Cfg.Fixture.Pet.MoveMS)
		}
	}
	if r.Pending || r.Isolated {
		return
	}
	for _, m := range r.Monsters[:r.MonsterCount] {
		if m.HP <= 0 {
			continue
		}
		target := &r.Player
		if pet != nil && pet.HP > 0 && distance(m.Pos, pet.Pos) < distance(m.Pos, r.Player.Pos) {
			target = pet
		}
		if distance(m.Pos, target.Pos) <= 1 {
			w.attack(r, m, target, now)
		} else {
			w.move(r, m, target.Pos, now, m.Monster.MoveMS)
		}
		if r.Pending || r.Isolated {
			break
		}
	}
	if now.Sub(r.LastBroadcast) >= 200*time.Millisecond {
		r.LastBroadcast = now
		w.snapshot(r)
	}
	w.flushKillFacts(r)
}
func (w *worker) legacyKill(r *room, m *entity, now time.Time) {
	f := &w.e.Store.Cfg.Fixture
	bound := !r.Role.PrivilegeUntil.After(now)
	q := persistence.Settlement{SourceSeq: r.Role.SourceSeq + 1, Kills: 1, Experience: m.Monster.Exp, Bound: bound, NoEquipmentKills: r.Role.NoEquipmentKills + 1}
	roll := random(1000000)
	equipment := q.NoEquipmentKills >= 20 || roll >= 950000
	if equipment {
		ids := f.Drops.EquipmentItemIDs
		item := domain.Item{ID: domain.ID(), ItemID: ids[random(int64(len(ids)))], Quantity: 1, Bound: bound, Version: 1}
		item.SourceKind, item.SourceID = "monster_drop", fmt.Sprint(q.SourceSeq)
		definition, _ := f.Item(item.ItemID)
		item.Quality = definition.DropQuality
		q.Item = &item
		q.NoEquipmentKills = 0
	} else if roll >= 650000 && roll < 850000 {
		q.Gold = m.Monster.GoldMin + random(m.Monster.GoldMax-m.Monster.GoldMin+1)
	} else if roll >= 850000 && roll < 950000 && len(f.Drops.ItemIDs) > 0 {
		ids := f.Drops.ItemIDs
		item := domain.Item{ID: domain.ID(), ItemID: ids[random(int64(len(ids)))], Quantity: 1, Bound: bound, Version: 1}
		item.SourceKind, item.SourceID = "monster_drop", fmt.Sprint(q.SourceSeq)
		definition, _ := f.Item(item.ItemID)
		item.Quality = definition.DropQuality
		q.Item = &item
	}
	if r.Conn == nil {
		q.HostingSession = r.HostSession
	}
	w.settle(r, q)
}
func (w *worker) settle(r *room, q persistence.Settlement, finished ...func()) {
	w.settleInWorld(r, r.currentWorld(), r.Player.Pos, q, finished...)
}

type settlementBinding struct {
	room            *room
	auth            persistence.Auth
	world           worldRef
	point           devconfig.Point
	generation      uint64
	id, fingerprint string
}

func (w *worker) settleInWorld(r *room, world *worldState, point devconfig.Point, q persistence.Settlement, finished ...func()) {
	r.SettlementGeneration++
	raw, err := json.Marshal(q)
	if err != nil {
		w.isolateRoom(r)
		return
	}
	binding := settlementBinding{room: r, auth: r.Auth, world: world.Ref, point: point, generation: r.SettlementGeneration, id: fmt.Sprintf("settle:%d", q.SourceSeq), fingerprint: persistence.Hash(raw)}
	r.Pending = true
	r.PendingOperationID, r.PendingFingerprint = binding.id, binding.fingerprint
	auth := r.Auth
	var j job
	j = job{target: w, run: func() func(*worker) {
		role, err := w.e.Store.Settle(context.Background(), auth, q)
		return func(w *worker) {
			w.finishSettlement(binding, q, role, err, &j, finished...)
		}
	}}
	if !w.submit(j) {
		r.PendingRetry = &j
		r.RetryAt = time.Now().Add(100 * time.Millisecond)
	}
}

func (w *worker) finishSettlement(binding settlementBinding, q persistence.Settlement, role domain.Role, err error, retry *job, finished ...func()) {
	r := binding.room
	if w.rooms[binding.auth.RoleID] != r || r.Auth != binding.auth || r.Isolated || !r.Pending || r.SettlementGeneration != binding.generation || r.PendingOperationID != binding.id || r.PendingFingerprint != binding.fingerprint {
		return
	}
	if err != nil && (persistence.Code(err) == p.CodeOperationPending || persistence.Code(err) == p.CodeServerBusy) {
		r.PendingRetry = retry
		r.RetryAt = time.Now().Add(500 * time.Millisecond)
		return
	}
	r.Pending = false
	r.PendingRetry = nil
	r.PendingOperationID, r.PendingFingerprint = "", ""
	if err != nil {
		w.isolateRoom(r)
		return
	}
	if role.Revision >= r.Role.Revision {
		r.Role = role
		transitionEventPrivilege(r, time.Now(), role.PrivilegeUntil)
		w.refreshPlayerStats(r)
	}
	if len(finished) > 0 {
		finished[0]()
	}
	if r.Role.PickupStopped {
		for _, world := range r.worlds() {
			world.Auto = false
		}
	}
	if q.Item != nil {
		world := r.world(binding.world)
		if world == nil {
			w.isolateRoom(r)
			return
		}
		world.Loot = []*entity{{Handle: 1000, Kind: 4, Def: q.Item.ItemID, Pos: binding.point, HP: 1, MaxHP: 1}}
		world.LootUntil = time.Now().Add(500 * time.Millisecond)
	}
	w.assets(r)
	w.snapshot(r)
	w.flushKillFacts(r)
	w.after(r)
}

func (w *worker) deathNotice(r *room) {
	if r.Conn == nil {
		return
	}
	allowed := uint32(0)
	if w.e.Store.Cfg.Fixture.AllowFreeDevRevive {
		allowed = 1
	}
	w.send(r, p.MsgS2CDeathNotice, 0, &pb.DeathNotice{DeathId: domain.Bytes(r.DeathID), CauseCode: 1, AllowedActions: allowed, RuleVersion: w.e.Store.Cfg.Fixture.RuleVersion})
}
