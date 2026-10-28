package game

import (
	"math"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	p "minimir/internal/protocol"
	"time"
)

type meleeState struct {
	Thrusting, HalfMoon, Slaying, Flaming bool
	FlamingUntil                          time.Time
	Revision                              uint64
}

type physicalDefence uint8

const (
	defenceACAgility physicalDefence = iota
	defenceAC
	defenceAgility
)

func remainingMS(until, now time.Time) uint32 {
	if !until.After(now) {
		return 0
	}
	ms := (until.Sub(now) + time.Millisecond - 1) / time.Millisecond
	if ms > math.MaxUint32 {
		return math.MaxUint32
	}
	return uint32(ms)
}

func (w *worker) learnedBehavior(r *room, behavior string) (devconfig.Skill, uint32, bool) {
	for _, s := range w.e.Store.Cfg.Fixture.Skills {
		rank, learned := r.Role.Skill(s.ID)
		if s.Behavior == behavior && s.JobID == r.Role.Job && learned && rank >= s.LearnedLevel && rank <= s.MaxLevel {
			return s, rank, true
		}
	}
	return devconfig.Skill{}, 0, false
}

func (w *worker) modeState(r *room, now time.Time) *pb.SkillModeState {
	out := &pb.SkillModeState{SceneEpoch: r.Scene, Revision: r.Melee.Revision, PlayerLife: r.Player.Life, ServerTimeUnixMs: uint64(now.UnixMilli())}
	for _, s := range w.e.Store.Cfg.Fixture.Skills {
		if s.Activation() != "toggle" && s.Activation() != "charge" && s.Activation() != "passive_proc" {
			continue
		}
		_, _, learned := w.learnedBehavior(r, s.Behavior)
		entry := &pb.SkillModeEntry{SkillId: s.ID}
		if learned {
			switch s.Behavior {
			case "thrusting":
				entry.Enabled = r.Melee.Thrusting
			case "half_moon":
				entry.Enabled = r.Melee.HalfMoon
			case "slaying":
				entry.Enabled, entry.Ready = true, r.Melee.Slaying
			case "flaming_sword":
				entry.Enabled = true
				entry.Ready = r.Melee.Flaming && now.Before(r.Melee.FlamingUntil)
				entry.CooldownRemainingMs = remainingMS(r.Melee.FlamingUntil, now)
				if entry.Ready {
					entry.RemainingMs = entry.CooldownRemainingMs
				}
			}
		}
		out.States = append(out.States, entry)
	}
	return out
}

func (w *worker) modes(r *room, now time.Time) {
	w.send(r, p.MsgS2CSkillModeState, 0, w.modeState(r, now))
}

func (w *worker) expireModes(r *room, now time.Time) {
	if r.Melee.Flaming && !now.Before(r.Melee.FlamingUntil) {
		r.Melee.Flaming = false
		r.Melee.Revision++
		w.modes(r, now)
	}
}

func (w *worker) setSkillMode(r *room, id uint32, enabled bool, now time.Time) uint32 {
	s, exists := w.e.Store.Cfg.Fixture.Skill(id)
	rank, learned := r.Role.Skill(id)
	if !exists || !learned || s.JobID != r.Role.Job || rank < s.LearnedLevel || rank > s.MaxLevel {
		return p.CodeSkillNotLearned
	}
	if r.Pending || r.Isolated || r.Player.HP <= 0 || w.mapCellBudget(r) == 0 {
		return p.CodeSkillStateBlocked
	}
	switch s.Behavior {
	case "thrusting":
		if r.Melee.Thrusting == enabled {
			w.modes(r, now)
			return 0
		}
		r.Melee.Thrusting = enabled
	case "half_moon":
		if r.Melee.HalfMoon == enabled {
			w.modes(r, now)
			return 0
		}
		r.Melee.HalfMoon = enabled
	case "flaming_sword":
		if !enabled {
			return p.CodeSkillStateBlocked
		}
		if now.Before(r.Melee.FlamingUntil) {
			return p.CodeSkillCooldown
		}
		cost := s.MPBaseCost + int64(rank)*s.MPLevelCost
		// Crystal SpellToggle：充能保留至少1点MP，与普通施法/半月的>=门槛不同。
		if r.Player.MP <= cost {
			return p.CodeSkillResource
		}
		r.Player.MP -= cost
		r.Melee.Flaming = true
		r.Melee.FlamingUntil = now.Add(time.Duration(s.ChargeWindowMS) * time.Millisecond)
	default:
		return p.CodeSkillStateBlocked
	}
	r.Melee.Revision++
	w.modes(r, now)
	w.snapshot(r)
	return 0
}

func stepPoint(origin devconfig.Point, direction uint32, distance int) devconfig.Point {
	d := directions[direction%uint32(len(directions))]
	return devconfig.Point{X: origin.X + d.X*distance, Y: origin.Y + d.Y*distance}
}

// 本阶段只允许活跃怪物为敌；每格以稳定句柄选第一个合法敌人，不把宠物算作敌人。
func (w *worker) enemyAt(r *room, point devconfig.Point) *entity {
	return w.enemyAtInWorld(r.currentWorld(), point)
}
func (w *worker) enemyAtInWorld(world *worldState, point devconfig.Point) *entity {
	if !w.worldWalkable(world, point) {
		return nil
	}
	var found *entity
	for _, e := range world.Monsters[:world.MonsterCount] {
		if e.HP > 0 && e.Pos == point && (found == nil || e.Handle < found.Handle) {
			found = e
		}
	}
	return found
}

func (w *worker) secondaryTargets(r *room, s devconfig.Skill, direction uint32) []*entity {
	var targets []*entity
	for _, offset := range s.SecondaryDirectionOffsets {
		d := uint32((int32(direction) + offset + int32(len(directions))) % int32(len(directions)))
		if e := w.enemyAt(r, stepPoint(r.Player.Pos, d, s.SecondaryDistanceCells)); e != nil {
			targets = append(targets, e)
		}
	}
	return targets
}

// Crystal客户端的本子集覆盖顺序：烈火、半月、刺杀、攻杀，模式开关互不覆盖。
func (w *worker) selectedAttack(r *room, direction uint32, now time.Time) (devconfig.Skill, uint32) {
	front := w.enemyAt(r, stepPoint(r.Player.Pos, direction, 1))
	if s, rank, learned := w.learnedBehavior(r, "flaming_sword"); learned && r.Melee.Flaming && now.Before(r.Melee.FlamingUntil) && front != nil {
		return s, rank
	}
	if s, rank, learned := w.learnedBehavior(r, "half_moon"); learned && r.Melee.HalfMoon && r.Player.MP >= s.MPBaseCost+int64(rank)*s.MPLevelCost && (front != nil || len(w.secondaryTargets(r, s, direction)) > 0) {
		return s, rank
	}
	if s, rank, learned := w.learnedBehavior(r, "thrusting"); learned && r.Melee.Thrusting && len(w.secondaryTargets(r, s, direction)) > 0 {
		return s, rank
	}
	if s, rank, learned := w.learnedBehavior(r, "slaying"); learned && r.Melee.Slaying && front != nil {
		return s, rank
	}
	return devconfig.Skill{}, 0
}

func (w *worker) attackFacing(r *room, direction, expected, input uint32, now time.Time) uint32 {
	from := &r.Player
	if direction >= uint32(len(directions)) {
		return p.CodeInvalidArgument
	}
	// Human.CanAttack只依赖动作与攻击时钟；保留本作截止时间相等即可受理的边界。
	if r.Pending || r.Isolated || from.HP <= 0 || now.Before(from.NextAttack) || now.Before(from.ActionReadyAt) || !w.killBudgetAvailable(r) || w.mapCellBudget(r) == 0 {
		return p.CodeSkillStateBlocked
	}
	s, rank := w.selectedAttack(r, direction, now)
	if expected != 0 && expected != s.ID {
		return p.CodeSkillStateBlocked
	}
	front := w.enemyAt(r, stepPoint(from.Pos, direction, 1))
	secondary := w.secondaryTargets(r, s, direction)
	needed := len(secondary)
	if front != nil {
		needed++
	}
	if r.eventCount()+needed > w.eventCapacity() || from.AttackSequence == math.MaxUint32 {
		return p.CodeSkillStateBlocked
	}
	stats := w.fighter(r, from)
	changed := false
	if s.Behavior == "slaying" {
		r.Melee.Slaying, changed = false, true
	}
	// 每次已受理挥刀都可准备下一次攻杀；准备状态保留到真正选中该次攻击。
	if proc, level, learned := w.learnedBehavior(r, "slaying"); learned && !r.Melee.Slaying && w.draw(int64(proc.ReadyProcDenominator)) < int64(proc.ReadyProcNumerators[level]) {
		r.Melee.Slaying, changed = true, true
	}
	if s.Behavior == "half_moon" {
		from.MP -= s.MPBaseCost + int64(rank)*s.MPLevelCost
	}
	if s.Behavior == "flaming_sword" && front != nil {
		r.Melee.Flaming, changed = false, true
	}
	base := w.interval(stats.AttackMin, stats.AttackMax)
	frontAmount, secondaryAmount := base, int64(0)
	frontDefence := defenceACAgility
	if s.Behavior == "slaying" || s.Behavior == "flaming_sword" {
		frontAmount = w.magicAmount(s, rank, base)
	}
	if s.Behavior == "flaming_sword" {
		frontDefence = defenceAC
	}
	if len(secondary) > 0 {
		secondaryAmount = w.magicAmount(s, rank, base)
	}
	from.AttackSequence++
	from.AttackSkillID, from.AttackSkillLevel = s.ID, rank
	from.NextAttack = now.Add(time.Duration(stats.AttackMS) * time.Millisecond)
	from.ActionReadyAt = now.Add(time.Duration(w.e.Store.Cfg.Fixture.CombatTiming.PlayerAttackActionMS) * time.Millisecond)
	from.Action, from.ActionUntil, from.Direction = 2, now.Add(200*time.Millisecond), direction
	queue := func(target *entity, amount int64, defence physicalDefence, delay int64) {
		r.ScheduleID++
		a := pendingAttack{Scene: r.Scene, Caster: reference(r, from), Target: reference(r, target), Due: now.Add(time.Duration(delay) * time.Millisecond), Amount: amount, Defence: defence, SkillID: s.ID, SkillLevel: rank, Sequence: from.AttackSequence, Order: r.ScheduleID, PrivilegeUntil: r.Role.PrivilegeUntil, PrivilegeCaptured: true}
		if r.Conn == nil {
			a.HostSession = r.HostSession
		}
		r.Attacks = append(r.Attacks, a)
	}
	if front != nil {
		queue(front, frontAmount, frontDefence, w.e.Store.Cfg.Fixture.CombatTiming.PlayerAttackImpactMS)
	}
	for _, target := range secondary {
		queue(target, secondaryAmount, defenceAgility, s.SecondaryImpactMS)
	}
	handle := uint32(0)
	if front != nil {
		handle = front.Handle
	}
	w.send(r, p.MsgS2CAttackAccepted, 0, &pb.AttackAccepted{SceneEpoch: r.Scene, EventId: w.nextEvent(r), CasterHandle: from.Handle, TargetHandle: handle, Origin: pointPB(from.Pos), TargetGrid: pointPB(stepPoint(from.Pos, direction, 1)), Direction: direction, DelayMs: uint32(w.e.Store.Cfg.Fixture.CombatTiming.PlayerAttackImpactMS), AttackSequence: from.AttackSequence, InputSequence: input, SkillId: s.ID, SkillLevel: rank, CasterLife: from.Life})
	if changed {
		r.Melee.Revision++
		w.modes(r, now)
	}
	// 旁格立即命中仍进入统一事件顺序，不能越过同一worker中已经到期的旧事件。
	w.advanceEvents(r, now)
	w.snapshot(r)
	return 0
}
