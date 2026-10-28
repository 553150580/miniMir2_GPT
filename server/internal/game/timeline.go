package game

import (
	"fmt"
	"math"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
	"sort"
	"time"
)

type actorRef struct {
	World  worldRef
	Handle uint32
	Life   uint64
}
type pendingAttack struct {
	Scene             uint64 // 仅观察身份，不参与World执行解析。
	Caster, Target    actorRef
	Due               time.Time
	Amount            int64
	Sequence          uint32
	Order             uint64
	HostSession       string
	PrivilegeUntil    time.Time
	PrivilegeCaptured bool
	SkillID           uint32
	SkillLevel        uint32
	Defence           physicalDefence
}
type killFact struct {
	World               worldRef
	Order               uint64
	DropPoint           devconfig.Point
	ID                  string
	Target              actorRef
	Caster              actorRef
	Monster             devconfig.Monster
	At                  time.Time
	Bound               bool
	HostSession         string
	StopReason          uint32
	Roll                int64
	EquipmentID, ItemID uint32
	Gold                int64
	InstanceID          string
	Prepared            bool
}

func (w *worker) sourceTimeline() bool {
	return w.e.Store.Cfg.Fixture.DamageFormula == "development_source_combat" && w.e.Store.Cfg.Fixture.CombatTiming.PlayerAttackActionMS > 0
}

type hitContext struct {
	At                time.Time
	HostSession       string
	PrivilegeUntil    time.Time
	PrivilegeCaptured bool
}

// 权限变更只更新尚未到期的事件；已经发生的事件不借迟处理、重连或续卡改绑。
func transitionEventPrivilege(r *room, at, until time.Time) {
	for _, world := range r.worlds() {
		for i := range world.Attacks {
			if !world.Attacks[i].Due.Before(at) {
				world.Attacks[i].PrivilegeUntil = until
				world.Attacks[i].PrivilegeCaptured = true
			}
		}
		for i := range world.Casts {
			if !world.Casts[i].Due.Before(at) {
				world.Casts[i].PrivilegeUntil = until
				world.Casts[i].PrivilegeCaptured = true
			}
		}
	}
	for _, event := range r.EventBatch {
		if !event.At.Before(at) {
			if event.Attack != nil {
				event.Attack.PrivilegeUntil, event.Attack.PrivilegeCaptured = until, true
			}
			if event.Cast != nil {
				event.Cast.PrivilegeUntil, event.Cast.PrivilegeCaptured = until, true
			}
		}
	}
}

// 只更新转换时尚未到期的事件；晚处理的事件保留其事件时刻所在的在线/托管区段。
func transitionEventHosting(r *room, at time.Time, session string) {
	for _, world := range r.worlds() {
		for i := range world.Attacks {
			if !world.Attacks[i].Due.Before(at) {
				world.Attacks[i].HostSession = session
			}
		}
		for i := range world.Casts {
			if !world.Casts[i].Due.Before(at) {
				world.Casts[i].HostSession = session
			}
		}
	}
	for _, event := range r.EventBatch {
		if !event.At.Before(at) {
			if event.Attack != nil {
				event.Attack.HostSession = session
			}
			if event.Cast != nil {
				event.Cast.HostSession = session
			}
		}
	}
}

// 围栏/身份硬失败不能给旧事实升级授权或重新抽奖；只隔离本房间，等待明确恢复。
func (w *worker) isolateRoom(r *room) {
	w.cancelTownNavigation(r)
	w.cancelMapTravelOnDetach(r)
	r.Isolated = true
	r.Pending = false
	r.Auto = false
	r.Manual = false
	r.PreviewGeneration++
	r.PreviewInFlight = false
	if r.Conn != nil {
		r.Conn.Close("room authority isolated")
	}
	for _, waiting := range r.Waiting {
		if waiting.conn != nil {
			waiting.conn.Close("room authority isolated")
		}
	}
	r.Waiting = nil
}

type dueEvent struct {
	World  *worldState
	At     time.Time
	Order  uint64
	Attack *pendingAttack
	Cast   *pendingCast
}

// 全部临时到期事件按统一时钟与受理次序结算，不以消息种类改变同帧命中顺序。
func (w *worker) advanceEvents(r *room, now time.Time) {
	w.advanceEventsUsing(r, now, w.resolveDueEvent)
}

func (w *worker) resolveDueEvent(r *room, e dueEvent, now time.Time) *worldTransfer {
	if e.Cast != nil && e.Cast.Skill.Behavior == "teleport" {
		return w.resolveTeleport21(r, e.World, *e.Cast, now)
	}
	if e.Attack != nil {
		w.resolveAttackInWorld(r, e.World, *e.Attack, now)
	} else {
		w.resolveCastInWorld(r, e.World, *e.Cast, now)
	}
	return nil
}

// 整个owner只有一个到期批；从队列摘取到局部批不归还共享容量。
func (w *worker) advanceEventsUsing(r *room, now time.Time, resolve func(*room, dueEvent, time.Time) *worldTransfer) {
	if r.AdvancingEvents || r.Pending || r.Isolated {
		return
	}
	r.AdvancingEvents = true
	defer func() { r.AdvancingEvents = false; r.EventBatch = nil }()
	events := []dueEvent{}
	for _, world := range r.worlds() {
		attacks := world.Attacks[:0]
		casts := world.Casts[:0]
		for _, a := range world.Attacks {
			if now.Before(a.Due) {
				attacks = append(attacks, a)
			} else {
				copy := a
				events = append(events, dueEvent{World: world, At: a.Due, Order: a.Order, Attack: &copy})
			}
		}
		for _, c := range world.Casts {
			if now.Before(c.Due) {
				casts = append(casts, c)
			} else {
				copy := c
				events = append(events, dueEvent{World: world, At: c.Due, Order: c.Order, Cast: &copy})
			}
		}
		world.Attacks, world.Casts = attacks, casts
	}
	sort.SliceStable(events, func(i, j int) bool {
		if events[i].At.Equal(events[j].At) {
			return events[i].Order < events[j].Order
		}
		return events[i].At.Before(events[j].At)
	})
	r.InflightEvents += len(events)
	for i, e := range events {
		r.EventBatch = events[i:]
		if r.Pending || r.Isolated {
			for _, waiting := range events[i:] {
				if waiting.Attack != nil {
					waiting.World.Attacks = append(waiting.World.Attacks, *waiting.Attack)
				} else {
					waiting.World.Casts = append(waiting.World.Casts, *waiting.Cast)
				}
				r.InflightEvents--
			}
			break
		}
		r.InflightEvents-- // 当前slot可转交给同一地图动作下一子格。
		r.EventBatch = events[i+1:]
		if transfer := resolve(r, e, now); transfer != nil {
			w.commitWorldTransfer(r, transfer, now)
		}
	}
	if len(events) > 0 {
		w.snapshot(r)
	}
}

func (w *worker) killBudgetAvailable(r *room) bool {
	cap := w.e.Store.Cfg.Fixture.CombatTiming.KillQueueCapacity
	if cap <= 0 {
		return true
	}
	possible := len(r.KillFacts) + 1
	if r.BindingCandidate != nil {
		possible++
	}
	for _, world := range r.worlds() {
		for _, m := range world.Monsters[:world.MonsterCount] {
			if m.HP > 0 {
				possible++
			}
		}
	}
	return possible <= cap
}
func reference(r *room, e *entity) actorRef { return referenceIn(r.currentWorld(), e) }
func (w *worker) resolve(r *room, ref actorRef) *entity {
	return w.resolveInWorld(r, r.currentWorld(), ref)
}
func facing(from, to devconfig.Point, fallback uint32) uint32 {
	dx, dy := to.X-from.X, to.Y-from.Y
	for i, d := range directions {
		if (dx == 0 && d.X == 0 || dx > 0 && d.X > 0 || dx < 0 && d.X < 0) && (dy == 0 && d.Y == 0 || dy > 0 && d.Y > 0 || dy < 0 && d.Y < 0) {
			return uint32(i)
		}
	}
	return fallback
}
func (w *worker) nextEvent(r *room) uint64 {
	if r.EventID < math.MaxUint64 {
		r.EventID++
	}
	return r.EventID
}
func (w *worker) impact(r *room, skill, caster, target uint32, life uint64, grid devconfig.Point, amount int64, outcome uint32, healing bool, cast uint64, attack uint32) {
	w.send(r, p.MsgS2CCombatImpact, 0, &pb.CombatImpact{SceneEpoch: r.Scene, EventId: w.nextEvent(r), SkillId: skill, CasterHandle: caster, TargetHandle: target, TargetLife: life, TargetGrid: pointPB(grid), Amount: amount, Outcome: outcome, Healing: healing, CastId: cast, AttackSequence: attack})
}
func (w *worker) impactInWorld(r *room, world *worldState, scene uint64, skill, caster, target uint32, life uint64, grid devconfig.Point, amount int64, outcome uint32, healing bool, cast uint64, attack uint32) {
	if r.observesWorld(world, scene) {
		w.impact(r, skill, caster, target, life, grid, amount, outcome, healing, cast, attack)
	}
}
func (w *worker) resolveAttack(r *room, a pendingAttack, now time.Time) {
	w.resolveAttackInWorld(r, r.world(a.Caster.World), a, now)
}
func (w *worker) resolveAttackInWorld(r *room, world *worldState, a pendingAttack, now time.Time) {
	if world == nil || a.Target.World != world.Ref || a.Caster.World != world.Ref {
		return
	}
	if _, known := w.worldMap(world); !known {
		return
	}
	target := w.resolveInWorld(r, world, a.Target)
	if target == nil || target.HP <= 0 {
		w.impactInWorld(r, world, a.Scene, a.SkillID, a.Caster.Handle, a.Target.Handle, a.Target.Life, devconfig.Point{}, 0, 3, false, 0, a.Sequence)
		return
	}
	// 来源CompleteAttack不要求施法者仍存活；伤害/防御在到期时结算，坐标不重新限制近战。
	caster := w.resolveInWorld(r, world, a.Caster)
	if caster == nil {
		w.impactInWorld(r, world, a.Scene, a.SkillID, a.Caster.Handle, target.Handle, target.Life, target.Pos, 0, 3, false, 0, a.Sequence)
		return
	}
	attacker, defender := w.fighter(r, caster), w.fighter(r, target)
	damage := int64(0)
	if a.Defence == defenceAC || w.draw(defender.Agility+1) <= attacker.Accuracy {
		armour := int64(0)
		if a.Defence != defenceAgility {
			armour = w.interval(defender.DefenceMin, defender.DefenceMax)
		}
		damage = statusDamage(target, a.Amount, armour)
	}
	if damage < 0 {
		damage = 0
	}
	before := target.HP
	w.applyDamageInWorld(r, world, caster, target, damage, now, hitContext{At: a.Due, HostSession: a.HostSession, PrivilegeUntil: a.PrivilegeUntil, PrivilegeCaptured: a.PrivilegeCaptured})
	actual := before - target.HP
	outcome := uint32(1)
	if actual == 0 {
		outcome = 2
	}
	w.impactInWorld(r, world, a.Scene, a.SkillID, caster.Handle, target.Handle, target.Life, target.Pos, actual, outcome, false, 0, a.Sequence)
}
func (w *worker) kill(r *room, m *entity, now time.Time, from ...*entity) {
	if !w.sourceTimeline() {
		w.legacyKill(r, m, now)
		return
	}
	caster := &r.Player
	if len(from) > 0 && from[0] != nil {
		caster = from[0]
	}
	session := ""
	if r.Conn == nil {
		session = r.HostSession
	}
	w.enqueueKillFact(r, m, now, caster, session, r.Role.PrivilegeUntil)
}

func (w *worker) enqueueKillFact(r *room, m *entity, at time.Time, caster *entity, session string, privilegeUntil time.Time) {
	w.enqueueKillFactInWorld(r, r.currentWorld(), m, at, caster, session, privilegeUntil)
}
func (w *worker) enqueueKillFactInWorld(r *room, world *worldState, m *entity, at time.Time, caster *entity, session string, privilegeUntil time.Time) {
	f := w.e.Store.Cfg.Fixture
	r.FactOrder++
	point := world.PlayerPosition
	if world == r.currentWorld() {
		point = r.Player.Pos
	}
	fact := killFact{ID: domain.ID(), World: world.Ref, Order: r.FactOrder, DropPoint: point, Target: referenceIn(world, m), Caster: referenceIn(world, caster), Monster: *m.Monster, At: at, Bound: !privilegeUntil.After(at), Roll: w.draw(1000000), InstanceID: domain.ID(), HostSession: session}
	if len(f.Drops.EquipmentItemIDs) > 0 {
		fact.EquipmentID = f.Drops.EquipmentItemIDs[w.draw(int64(len(f.Drops.EquipmentItemIDs)))]
	}
	if len(f.Drops.ItemIDs) > 0 {
		fact.ItemID = f.Drops.ItemIDs[w.draw(int64(len(f.Drops.ItemIDs)))]
	}
	fact.Gold = m.Monster.GoldMin + w.draw(m.Monster.GoldMax-m.Monster.GoldMin+1)
	r.KillFacts = append(r.KillFacts, fact)
}

func (w *worker) flushKillFacts(r *room) {
	if r.Pending || r.Isolated {
		return
	}
	if candidate := r.BindingCandidate; candidate != nil && (len(r.KillFacts) == 0 || candidate.order < r.KillFacts[0].Order) {
		w.admitBindingCandidate(r, time.Now())
		return
	}
	if len(r.KillFacts) == 0 || r.KillFacts[0].Prepared {
		return
	}
	head := &r.KillFacts[0]
	head.Prepared = true
	fact := *head
	q := w.killSettlement(r, fact)
	world := r.world(fact.World)
	if world == nil && fact.World == (worldRef{}) {
		world = r.currentWorld()
	} // 仅兼容旧手造killFact，正式捕获必有原World。
	if world == nil {
		w.isolateRoom(r)
		return
	}
	w.settleInWorld(r, world, fact.DropPoint, q, func() {
		if len(r.KillFacts) > 0 && r.KillFacts[0].ID == fact.ID {
			r.KillFacts = r.KillFacts[1:]
		}
	})
}

func (w *worker) killSettlement(r *room, fact killFact) persistence.Settlement {
	q := persistence.Settlement{SourceSeq: r.Role.SourceSeq + 1, HostingSession: fact.HostSession, StopReason: fact.StopReason, EventAt: fact.At}
	q.TargetHandle, q.TargetLife, q.CasterHandle = fact.Target.Handle, fact.Target.Life, fact.Caster.Handle
	if fact.StopReason == 0 {
		f := w.e.Store.Cfg.Fixture
		q.Kills = 1
		q.Experience = fact.Monster.Exp
		q.Bound = fact.Bound
		q.NoEquipmentKills = r.Role.NoEquipmentKills + 1
		itemID := uint32(0)
		goldEnd := int64(f.Drops.EmptyPPM + f.Drops.GoldPPM)
		itemEnd := goldEnd + int64(f.Drops.ItemPPM)
		if q.NoEquipmentKills >= f.Drops.EquipmentGuarantee || fact.Roll >= itemEnd {
			itemID = fact.EquipmentID
			q.NoEquipmentKills = 0
		} else if fact.Roll >= int64(f.Drops.EmptyPPM) && fact.Roll < goldEnd {
			q.Gold = fact.Gold
		} else if fact.Roll >= goldEnd {
			itemID = fact.ItemID
		}
		if itemID != 0 {
			definition, _ := f.Item(itemID)
			q.Item = &domain.Item{ID: fact.InstanceID, ItemID: itemID, Quantity: 1, Version: 1, Bound: fact.Bound, SourceKind: "monster_drop", SourceID: fmt.Sprint(q.SourceSeq), Quality: definition.DropQuality}
		}
	}
	return q
}
