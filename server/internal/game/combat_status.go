package game

import (
	"math"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	"sort"
	"time"
)

// 状态只属于当前实体现场；施法者引用保留来源，施法者后续死亡不撤销已经生效的状态。
type combatStatus struct {
	ID                      uint32
	Kind                    string
	SourceSkillID           uint32
	SkillLevel              uint32
	Caster                  actorRef
	TargetLife              uint64
	RemainingMS             int64
	LastProcess             time.Time
	NextProcess             time.Time
	TickMS                  int64
	MaxPhysicalDefenceBonus int64
	MaxMagicDefenceBonus    int64
	DamageReductionPercent  int64
	DamageWearMS            int64
	RemoveOnDeath           bool
}

func buffDurationSeconds(skill devconfig.Skill, definition devconfig.BuffDefinition, rank uint32, stat, power int64) int64 {
	if definition.DurationKind == "mc_source_power" {
		return skill.SourcePower(rank, stat+definition.StatAdd, power)
	}
	return stat*definition.StatMultiplier + definition.DurationSecondsByLevel[rank]
}

func statusTargetBonus(level uint32, definition devconfig.BuffDefinition) int64 {
	return int64(level)/definition.TargetLevelDivisor + definition.TargetLevelAdd
}

func statusFighter(e *entity, base devconfig.Fighter) devconfig.Fighter {
	for _, status := range e.Statuses {
		if status.TargetLife != e.Life {
			continue
		}
		base.DefenceMax += status.MaxPhysicalDefenceBonus
		base.MagicDefenceMax += status.MaxMagicDefenceBonus
	}
	return base
}

func statusReduction(e *entity) int64 {
	var total int64
	for _, status := range e.Statuses {
		if status.TargetLife == e.Life {
			total += status.DamageReductionPercent
		}
	}
	return total
}

// 来源先对原始伤害做整数减伤，再扣AC/MAC；不能把已扣防御的净伤害再次乘比例。
func statusDamage(e *entity, raw, armour int64) int64 {
	if reduction := statusReduction(e); reduction > 0 {
		raw -= raw * reduction / 100
	}
	raw -= armour
	if raw < 0 {
		return 0
	}
	return raw
}

// 削时依据扣HP前的正净伤害，不能改用实际扣除的有限剩余HP；负remaining暂保留至状态tick。
func wearStatuses(e *entity, damage int64) {
	if damage <= 0 {
		return
	}
	for _, status := range e.Statuses {
		if status.TargetLife != e.Life || status.DamageWearMS <= 0 {
			continue
		}
		if damage > math.MaxInt64/status.DamageWearMS {
			status.RemainingMS = math.MinInt64
			continue
		}
		cost := damage * status.DamageWearMS
		if status.RemainingMS < math.MinInt64+cost {
			status.RemainingMS = math.MinInt64
		} else {
			status.RemainingMS -= cost
		}
	}
}

func removeDeathStatuses(e *entity) {
	for id, status := range e.Statuses {
		if status.RemoveOnDeath {
			delete(e.Statuses, id)
		}
	}
}

func (w *worker) tickStatuses(r *room, now time.Time) bool {
	changed := false
	all := append([]*entity{&r.Player, r.Pet}, r.Monsters...)
	for _, e := range all {
		if e == nil {
			continue
		}
		for id, status := range e.Statuses {
			if status.TargetLife != e.Life {
				delete(e.Statuses, id)
				changed = true
				continue
			}
			if status.NextProcess.After(now) {
				continue
			}
			elapsed := now.Sub(status.LastProcess).Milliseconds()
			if elapsed < 0 {
				elapsed = 0
			}
			if status.RemainingMS < math.MinInt64+elapsed {
				status.RemainingMS = math.MinInt64
			} else {
				status.RemainingMS -= elapsed
			}
			status.LastProcess, status.NextProcess = now, now.Add(time.Duration(status.TickMS)*time.Millisecond)
			if status.RemainingMS <= 0 {
				delete(e.Statuses, id)
			}
			changed = true
		}
	}
	return changed
}

func (w *worker) applyBuff(r *room, c pendingCast, target *entity, now time.Time) bool {
	definition, exists := w.e.Store.Cfg.Fixture.Buff(c.Skill.BuffID)
	if !exists || !definition.AllowDeadTargets && target.HP <= 0 {
		return false
	}
	if old := target.Statuses[definition.ID]; old != nil && old.TargetLife == target.Life {
		if definition.StackPolicy == "ignore_present_on_impact" {
			return false
		}
		// 来源刷新仅替换剩余时长；首次数值、来源施法者以及Last/Next时钟全部保留。
		old.RemainingMS = c.BuffDurationMS
		return true
	}
	if len(target.Statuses) >= len(w.e.Store.Cfg.Fixture.Buffs) {
		return false
	}
	status := &combatStatus{ID: definition.ID, Kind: definition.Kind, SourceSkillID: c.Skill.ID, SkillLevel: c.SkillLevel,
		Caster: c.Caster, TargetLife: target.Life, RemainingMS: c.BuffDurationMS, LastProcess: now, TickMS: definition.TickMS,
		DamageWearMS: definition.HitDurationLossMSPerHP, RemoveOnDeath: definition.RemoveOnPlayerDeath}
	switch definition.Kind {
	case "magic_shield":
		status.DamageReductionPercent = definition.ValueByLevel[c.SkillLevel]
	case "max_physical_defence":
		status.MaxPhysicalDefenceBonus = statusTargetBonus(w.fighter(r, target).Level, definition)
	case "max_magic_defence":
		status.MaxMagicDefenceBonus = statusTargetBonus(w.fighter(r, target).Level, definition)
	}
	if target.Statuses == nil {
		target.Statuses = map[uint32]*combatStatus{}
	}
	target.Statuses[definition.ID] = status
	return true
}

func (w *worker) buffTargets(r *room, c pendingCast) []*entity {
	b, ok := w.e.Store.Cfg.Fixture.Buff(c.Skill.BuffID)
	if !ok {
		return nil
	}
	if c.Skill.TargetKind == "friendly_self" {
		if r.Player.Handle != c.Caster.Handle || r.Player.Life != c.Caster.Life || !b.AllowDeadTargets && r.Player.HP <= 0 {
			return nil
		}
		return []*entity{&r.Player}
	}
	var out []*entity
	for _, target := range []*entity{&r.Player, r.Pet} {
		if target != nil && (b.AllowDeadTargets || target.HP > 0) && distance(target.Pos, c.TargetPoint) <= c.Skill.AreaRadius && w.walkable(r, target.Pos) {
			out = append(out, target)
		}
	}
	return out
}

func statusesPB(e *entity, now time.Time) []*pb.EntityStatusState {
	ids := make([]uint32, 0, len(e.Statuses))
	for id, status := range e.Statuses {
		if status.TargetLife == e.Life {
			ids = append(ids, id)
		}
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	out := make([]*pb.EntityStatusState, 0, len(ids))
	for _, id := range ids {
		status := e.Statuses[id]
		elapsed := now.Sub(status.LastProcess).Milliseconds()
		if elapsed < 0 {
			elapsed = 0
		}
		remaining := status.RemainingMS
		if remaining <= elapsed {
			remaining = 0
		} else {
			remaining -= elapsed
		}
		if remaining > math.MaxUint32 {
			remaining = math.MaxUint32
		}
		out = append(out, &pb.EntityStatusState{StatusId: id, SourceSkillId: status.SourceSkillID, SkillLevel: status.SkillLevel, RemainingMs: uint32(remaining),
			MaxPhysicalDefenceBonus: status.MaxPhysicalDefenceBonus, MaxMagicDefenceBonus: status.MaxMagicDefenceBonus, DamageReductionPercent: uint32(status.DamageReductionPercent),
			CasterHandle: status.Caster.Handle, CasterLife: status.Caster.Life})
	}
	return out
}

func (w *worker) combatAttributesPB(r *room, e *entity) *pb.CombatAttributes {
	a := w.fighter(r, e)
	return &pb.CombatAttributes{PhysicalAttackMin: a.AttackMin, PhysicalAttackMax: a.AttackMax, PhysicalDefenceMin: a.DefenceMin, PhysicalDefenceMax: a.DefenceMax,
		MagicDefenceMin: a.MagicDefenceMin, MagicDefenceMax: a.MagicDefenceMax, MagicAttackMin: a.MagicMin, MagicAttackMax: a.MagicMax,
		SpiritAttackMin: a.TaoMin, SpiritAttackMax: a.TaoMax, Accuracy: a.Accuracy, Agility: a.Agility, DamageReductionPercent: uint32(statusReduction(e))}
}
