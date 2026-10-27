package game

import (
	"context"
	"crypto/rand"
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
func (w *worker) walkable(q devconfig.Point) bool {
	f := &w.e.Store.Cfg.Fixture
	if !w.inBounds(q) {
		return false
	}
	for _, blocked := range f.Map.Blocked {
		if blocked == q {
			return false
		}
	}
	return true
}

func (w *worker) inBounds(q devconfig.Point) bool {
	f := w.e.Store.Cfg.Fixture
	return q.X >= 0 && q.Y >= 0 && q.X < f.Map.Width && q.Y < f.Map.Height
}
func (w *worker) free(r *room, q devconfig.Point, self *entity) bool {
	if !w.walkable(q) {
		return false
	}
	all := []*entity{&r.Player}
	if r.Pet != nil {
		all = append(all, r.Pet)
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
		if !w.walkable(devconfig.Point{X: from.X + dx, Y: from.Y}) || !w.walkable(devconfig.Point{X: from.X, Y: from.Y + dy}) {
			return false
		}
	}
	return true
}
func (w *worker) move(r *room, e *entity, target devconfig.Point, now time.Time, interval int64) {
	if now.Before(e.NextMove) || now.Before(e.ActionReadyAt) || e.HP <= 0 || e.Pos == target {
		return
	}
	start := e.Pos
	budget := w.e.Store.Cfg.Fixture.GridCellBudget()
	if budget == 0 || !w.inBounds(start) || !w.inBounds(target) {
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
	for i, d := range directions {
		if next.X-start.X == d.X && next.Y-start.Y == d.Y {
			e.Direction = uint32(i)
			break
		}
	}
	e.Pos = *next
	e.Action = 1
	e.ActionUntil = now.Add(200 * time.Millisecond)
	e.NextMove = now.Add(time.Duration(interval) * time.Millisecond)
	if e.Kind == 1 {
		e.ActionReadyAt = e.NextMove
	}
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
	var best *entity
	for _, m := range r.Monsters[:r.MonsterCount] {
		if m.HP > 0 && (best == nil || distance(from.Pos, m.Pos) < distance(from.Pos, best.Pos)) {
			best = m
		}
	}
	return best
}
func (w *worker) attack(r *room, from, target *entity, now time.Time, input ...uint32) {
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
	if r.Pending || from.HP <= 0 || target.HP <= 0 || now.Before(from.NextAttack) || now.Before(from.NextMove) || now.Before(from.ActionReadyAt) || distance(from.Pos, target.Pos) > 1 {
		return
	}
	dx, dy := target.Pos.X-from.Pos.X, target.Pos.Y-from.Pos.Y
	if dx != 0 && dy != 0 && (!w.walkable(devconfig.Point{X: from.Pos.X + dx, Y: from.Pos.Y}) || !w.walkable(devconfig.Point{X: from.Pos.X, Y: from.Pos.Y + dy})) {
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
				w.enqueueKillFact(r, target, context[0].At, from, context[0].HostSession, until)
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
		w.snapshot(r)
	}
}
func (w *worker) step(r *room, now time.Time) {
	if w.e.stopped.Load() || r.Isolated {
		return
	}
	if r.Scene == 0 {
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
	w.advanceEvents(r, now)
	w.flushKillFacts(r)
	if r.Pending {
		return
	}
	if w.tickHealing(r, now) {
		w.snapshot(r)
	}
	offline := r.Conn == nil
	if r.Player.HP <= 0 {
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
	if !r.LootUntil.IsZero() && !now.Before(r.LootUntil) {
		r.Loot = nil
	}
	for i, m := range r.Monsters {
		if m.HP == 0 && !m.RespawnAt.IsZero() && !now.Before(m.RespawnAt) {
			m.HP = m.MaxHP
			m.Life++
			m.Statuses = nil
			m.Pos = w.e.Store.Cfg.Fixture.Map.MonsterSpawns[i]
			m.RespawnAt = time.Time{}
			m.Action = 0
		}
	}
	a := w.fighter(r, &r.Player)
	if r.Manual {
		d := directions[r.ManualDirection]
		target := devconfig.Point{X: r.Player.Pos.X + d.X, Y: r.Player.Pos.Y + d.Y}
		if w.neighbor(r, r.Player.Pos, target, &r.Player) && !now.Before(r.Player.NextMove) && !now.Before(r.Player.ActionReadyAt) {
			r.Player.Direction = r.ManualDirection
			r.Player.Pos = target
			r.Player.Action = 1
			r.Player.ActionUntil = now.Add(200 * time.Millisecond)
			r.Player.NextMove = now.Add(time.Duration(a.MoveMS) * time.Millisecond)
			r.Player.ActionReadyAt = r.Player.NextMove
		}
	} else if r.Destination != nil {
		if r.Player.Pos == *r.Destination {
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
	if r.Pending {
		return
	}
	if r.Pet != nil && r.Pet.HP > 0 {
		if r.Auto {
			if target := w.nearest(r, r.Pet); target != nil {
				if distance(r.Pet.Pos, target.Pos) <= 1 {
					w.attack(r, r.Pet, target, now)
				} else {
					w.move(r, r.Pet, target.Pos, now, w.e.Store.Cfg.Fixture.Pet.MoveMS)
				}
			}
		} else if distance(r.Pet.Pos, r.Player.Pos) > 2 {
			w.move(r, r.Pet, r.Player.Pos, now, w.e.Store.Cfg.Fixture.Pet.MoveMS)
		}
	}
	if r.Pending {
		return
	}
	for _, m := range r.Monsters[:r.MonsterCount] {
		if m.HP <= 0 {
			continue
		}
		target := &r.Player
		if r.Pet != nil && r.Pet.HP > 0 && distance(m.Pos, r.Pet.Pos) < distance(m.Pos, r.Player.Pos) {
			target = r.Pet
		}
		if distance(m.Pos, target.Pos) <= 1 {
			w.attack(r, m, target, now)
		} else {
			w.move(r, m, target.Pos, now, m.Monster.MoveMS)
		}
		if r.Pending {
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
	r.Pending = true
	auth := r.Auth
	var j job
	j = job{target: w, run: func() func(*worker) {
		role, err := w.e.Store.Settle(context.Background(), auth, q)
		return func(w *worker) {
			if err != nil && (persistence.Code(err) == p.CodeOperationPending || persistence.Code(err) == p.CodeServerBusy) {
				r.PendingRetry = &j
				r.RetryAt = time.Now().Add(500 * time.Millisecond)
				return
			}
			r.Pending = false
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
				r.Auto = false
			}
			if q.Item != nil {
				r.Loot = []*entity{{Handle: 1000, Kind: 4, Def: q.Item.ItemID, Pos: r.Player.Pos, HP: 1, MaxHP: 1}}
				r.LootUntil = time.Now().Add(500 * time.Millisecond)
			}
			w.assets(r)
			w.snapshot(r)
			w.flushKillFacts(r)
			w.after(r)
		}
	}}
	if !w.submit(j) {
		r.PendingRetry = &j
		r.RetryAt = time.Now().Add(100 * time.Millisecond)
	}
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
