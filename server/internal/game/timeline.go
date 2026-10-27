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
	Scene  uint64
	Handle uint32
	Life   uint64
}
type pendingAttack struct {
	Caster, Target    actorRef
	Due               time.Time
	Amount            int64
	Sequence          uint32
	Order             uint64
	HostSession       string
	PrivilegeUntil    time.Time
	PrivilegeCaptured bool
}
type killFact struct {
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
	for i := range r.Attacks {
		if !r.Attacks[i].Due.Before(at) {
			r.Attacks[i].PrivilegeUntil = until
			r.Attacks[i].PrivilegeCaptured = true
		}
	}
	for i := range r.Casts {
		if !r.Casts[i].Due.Before(at) {
			r.Casts[i].PrivilegeUntil = until
			r.Casts[i].PrivilegeCaptured = true
		}
	}
}

// 只更新转换时尚未到期的事件；晚处理的事件保留其事件时刻所在的在线/托管区段。
func transitionEventHosting(r *room, at time.Time, session string) {
	for i := range r.Attacks {
		if !r.Attacks[i].Due.Before(at) {
			r.Attacks[i].HostSession = session
		}
	}
	for i := range r.Casts {
		if !r.Casts[i].Due.Before(at) {
			r.Casts[i].HostSession = session
		}
	}
}

// 围栏/身份硬失败不能给旧事实升级授权或重新抽奖；只隔离本房间，等待明确恢复。
func (w *worker) isolateRoom(r *room) {
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
	At     time.Time
	Order  uint64
	Attack *pendingAttack
	Cast   *pendingCast
}

// 全部临时到期事件按统一时钟与受理次序结算，不以消息种类改变同帧命中顺序。
func (w *worker) advanceEvents(r *room, now time.Time) {
	events := []dueEvent{}
	attacks := r.Attacks[:0]
	casts := r.Casts[:0]
	for _, a := range r.Attacks {
		if now.Before(a.Due) {
			attacks = append(attacks, a)
		} else {
			copy := a
			events = append(events, dueEvent{At: a.Due, Order: a.Order, Attack: &copy})
		}
	}
	for _, c := range r.Casts {
		if now.Before(c.Due) {
			casts = append(casts, c)
		} else {
			copy := c
			events = append(events, dueEvent{At: c.Due, Order: c.Order, Cast: &copy})
		}
	}
	r.Attacks, r.Casts = attacks, casts
	sort.SliceStable(events, func(i, j int) bool {
		if events[i].At.Equal(events[j].At) {
			return events[i].Order < events[j].Order
		}
		return events[i].At.Before(events[j].At)
	})
	for i, e := range events {
		if r.Pending {
			for _, waiting := range events[i:] {
				if waiting.Attack != nil {
					r.Attacks = append(r.Attacks, *waiting.Attack)
				} else {
					r.Casts = append(r.Casts, *waiting.Cast)
				}
			}
			break
		}
		if e.Attack != nil {
			w.resolveAttack(r, *e.Attack, now)
		} else {
			w.resolveCast(r, *e.Cast, now)
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
	for _, m := range r.Monsters[:r.MonsterCount] {
		if m.HP > 0 {
			possible++
		}
	}
	return possible <= cap
}
func reference(r *room, e *entity) actorRef { return actorRef{r.Scene, e.Handle, e.Life} }
func (w *worker) resolve(r *room, ref actorRef) *entity {
	if ref.Scene != r.Scene {
		return nil
	}
	all := []*entity{&r.Player, r.Pet}
	all = append(all, r.Monsters[:r.MonsterCount]...)
	for _, e := range all {
		if e != nil && e.Handle == ref.Handle && e.Life == ref.Life {
			return e
		}
	}
	return nil
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
func (w *worker) resolveAttack(r *room, a pendingAttack, now time.Time) {
	if a.Target.Scene != r.Scene || a.Caster.Scene != r.Scene {
		return
	}
	target := w.resolve(r, a.Target)
	if target == nil || target.HP <= 0 {
		w.impact(r, 0, a.Caster.Handle, a.Target.Handle, a.Target.Life, devconfig.Point{}, 0, 3, false, 0, a.Sequence)
		return
	}
	// 来源CompleteAttack不要求施法者仍存活；伤害/防御在到期时结算，坐标不重新限制近战。
	caster := w.resolve(r, a.Caster)
	if caster == nil {
		w.impact(r, 0, a.Caster.Handle, target.Handle, target.Life, target.Pos, 0, 3, false, 0, a.Sequence)
		return
	}
	attacker, defender := w.fighter(r, caster), w.fighter(r, target)
	damage := int64(0)
	if w.draw(defender.Agility+1) <= attacker.Accuracy {
		damage = a.Amount - w.interval(defender.DefenceMin, defender.DefenceMax)
	}
	if damage < 0 {
		damage = 0
	}
	before := target.HP
	w.applyDamage(r, caster, target, damage, now, hitContext{At: a.Due, HostSession: a.HostSession, PrivilegeUntil: a.PrivilegeUntil, PrivilegeCaptured: a.PrivilegeCaptured})
	actual := before - target.HP
	outcome := uint32(1)
	if actual == 0 {
		outcome = 2
	}
	w.impact(r, 0, caster.Handle, target.Handle, target.Life, target.Pos, actual, outcome, false, 0, a.Sequence)
}
func (w *worker) advanceAttacks(r *room, now time.Time) {
	remaining := r.Attacks[:0]
	resolved := false
	for _, a := range r.Attacks {
		if now.Before(a.Due) {
			remaining = append(remaining, a)
			continue
		}
		w.resolveAttack(r, a, now)
		resolved = true
	}
	r.Attacks = remaining
	if resolved {
		w.snapshot(r)
	}
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
	f := w.e.Store.Cfg.Fixture
	fact := killFact{ID: domain.ID(), Target: reference(r, m), Caster: reference(r, caster), Monster: *m.Monster, At: at, Bound: !privilegeUntil.After(at), Roll: w.draw(1000000), InstanceID: domain.ID(), HostSession: session}
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
	if r.Pending || len(r.KillFacts) == 0 || r.KillFacts[0].Prepared {
		return
	}
	head := &r.KillFacts[0]
	head.Prepared = true
	fact := *head
	q := w.killSettlement(r, fact)
	w.settle(r, q, func() {
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
