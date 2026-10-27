package game

import (
	"math"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	p "minimir/internal/protocol"
	"sort"
	"strings"
	"time"
)

type skillHandler struct{ Passive, Healing, Area, UndeadBonus bool }

var skillHandlers = map[string]skillHandler{
	"fencing": {Passive: true}, "spirit_sword": {Passive: true},
	"fireball": {}, "great_fireball": {}, "thunderbolt": {UndeadBonus: true},
	"firebang": {Area: true}, "healing": {Healing: true}, "mass_healing": {Healing: true, Area: true},
}

func targetKind(s devconfig.Skill) string {
	if s.TargetKind != "" {
		return s.TargetKind
	}
	if s.Behavior == "fencing" || s.Behavior == "spirit_sword" {
		return "passive"
	}
	if s.Behavior == "healing" {
		return "friendly_entity"
	}
	return "enemy_entity"
}
func (w *worker) eventCapacity() int {
	n := w.e.Store.Cfg.Fixture.CombatTiming.EventCapacity
	if n <= 0 {
		return 64
	}
	return n
}
func (w *worker) areaTargets(r *room, s devconfig.Skill, grid devconfig.Point) []*entity {
	all := []*entity{&r.Player, r.Pet}
	all = append(all, r.Monsters[:r.MonsterCount]...)
	friendly := strings.HasPrefix(targetKind(s), "friendly")
	out := []*entity{}
	for _, e := range all {
		if e == nil || e.HP <= 0 || distance(e.Pos, grid) > s.AreaRadius || !w.walkable(e.Pos) {
			continue
		}
		if (friendly && (e.Kind == 1 || e.Kind == 2)) || (!friendly && e.Kind == 3) {
			out = append(out, e)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Handle < out[j].Handle })
	return out
}
func (w *worker) cast(r *room, id, targetHandle, input uint32, now time.Time, ground ...*pb.GridPoint) uint32 {
	s, exists := w.e.Store.Cfg.Fixture.Skill(id)
	level, learned := r.Role.Skill(id)
	handler, registered := skillHandlers[s.Behavior]
	if !exists || !learned || !registered || s.JobID != r.Role.Job || level < s.LearnedLevel || level > s.MaxLevel {
		return p.CodeSkillNotLearned
	}
	if handler.Passive || r.Pending || r.Player.HP <= 0 || now.Before(r.Player.NextMove) || now.Before(r.Player.ActionReadyAt) || len(r.Casts)+len(r.Attacks) >= w.eventCapacity() || !w.killBudgetAvailable(r) {
		return p.CodeSkillStateBlocked
	}
	if now.Before(r.NextSpell) || now.Before(r.SkillCooldowns[id]) {
		return p.CodeSkillCooldown
	}
	var target *entity
	var point devconfig.Point
	if handler.Area {
		if targetHandle != 0 || len(ground) == 0 || ground[0] == nil {
			return p.CodeSkillTarget
		}
		point = devconfig.Point{X: int(ground[0].X), Y: int(ground[0].Y)}
		if !w.inBounds(point) {
			return p.CodeSkillTarget
		}
	} else {
		target = w.skillTarget(r, s, targetHandle)
		if target == nil {
			return p.CodeSkillTarget
		}
		point = target.Pos
	}
	if distance(r.Player.Pos, point) > s.RangeCells {
		return p.CodeSkillTarget
	}
	requireFlight := s.RequireLineOfSight || (s.TargetKind == "" && s.Behavior == "fireball")
	if requireFlight && !w.lineOfSight(r.Player.Pos, point) {
		return p.CodeSkillTarget
	}
	cost := s.MPBaseCost + int64(level)*s.MPLevelCost
	if r.Player.MP < cost {
		return p.CodeSkillResource
	}
	stats := w.fighter(r, &r.Player)
	amount := int64(0)
	if handler.Healing {
		multiplier := s.HealingStatMultiplier
		addLevel := s.AddCasterLevel
		if s.TargetKind == "" && s.Behavior == "healing" {
			multiplier = 2
			addLevel = true
		}
		amount = w.magicAmount(s, level, w.interval(stats.TaoMin, stats.TaoMax)*multiplier)
		if addLevel {
			amount += int64(r.Role.Level)
		}
	} else {
		amount = w.magicAmount(s, level, w.interval(stats.MagicMin, stats.MagicMax))
	}
	if handler.UndeadBonus && target != nil && target.Monster != nil && target.Monster.Undead {
		amount = amount * s.UndeadMultiplierPPM / 1000000
	}
	delay := s.DelayBaseMS + int64(distance(r.Player.Pos, point))*s.ProjectileMSPerCell
	if delay > math.MaxUint32 || r.CastID == math.MaxUint64 {
		return p.CodeSkillStateBlocked
	}
	r.Player.MP -= cost
	r.Player.Action = 5
	r.Player.ActionUntil = now.Add(time.Duration(s.ActionMS) * time.Millisecond)
	r.Player.ActionReadyAt = r.Player.ActionUntil
	r.Player.NextAttack = r.Player.ActionReadyAt
	r.Player.CastSequence++
	r.Player.CastSkillID = id
	r.CastID++
	r.ScheduleID++
	r.Player.Direction = facing(r.Player.Pos, point, r.Player.Direction)
	r.NextSpell = now.Add(time.Duration(s.GlobalSpellMS) * time.Millisecond)
	if r.SkillCooldowns == nil {
		r.SkillCooldowns = map[uint32]time.Time{}
	}
	r.SkillCooldowns[id] = now.Add(time.Duration(s.CooldownBaseMS-int64(level)*s.CooldownReductionMS) * time.Millisecond)
	handle := uint32(0)
	life := uint64(0)
	if target != nil {
		handle = target.Handle
		life = target.Life
	}
	r.Casts = append(r.Casts, pendingCast{Scene: r.Scene, Skill: s, Target: handle, TargetLife: life, TargetPoint: point, Due: now.Add(time.Duration(delay) * time.Millisecond), Amount: amount, Caster: reference(r, &r.Player), ID: r.CastID, Order: r.ScheduleID, PrivilegeUntil: r.Role.PrivilegeUntil, PrivilegeCaptured: true})
	if r.Conn == nil {
		r.Casts[len(r.Casts)-1].HostSession = r.HostSession
	}
	w.send(r, p.MsgS2CCastAccepted, 0, &pb.CastAccepted{SceneEpoch: r.Scene, CastId: r.CastID, SkillId: id, CasterHandle: r.Player.Handle, TargetHandle: handle, Origin: pointPB(r.Player.Pos), TargetGrid: pointPB(point), Direction: r.Player.Direction, DelayMs: uint32(delay), InputSequence: input, CasterLife: r.Player.Life, TargetLife: life})
	w.snapshot(r)
	return 0
}

func (w *worker) resolveCast(r *room, c pendingCast, now time.Time) {
	if c.Scene != r.Scene {
		return
	}
	handler, registered := skillHandlers[c.Skill.Behavior]
	caster := w.resolve(r, c.Caster)
	if c.Scene != r.Scene || !registered || caster == nil {
		w.impact(r, c.Skill.ID, c.Caster.Handle, c.Target, c.TargetLife, c.TargetPoint, 0, 3, handler.Healing, c.ID, 0)
		return
	}
	targets := []*entity{}
	if handler.Area {
		targets = w.areaTargets(r, c.Skill, c.TargetPoint)
	} else {
		target := w.skillTarget(r, c.Skill, c.Target)
		if target != nil && target.Life == c.TargetLife && (!handler.Healing && distance(target.Pos, c.TargetPoint) <= c.Skill.TargetMoveToleranceCells || handler.Healing) {
			targets = append(targets, target)
		}
	}
	if len(targets) == 0 {
		outcome := uint32(3)
		if handler.Area {
			outcome = 2
		}
		w.impact(r, c.Skill.ID, c.Caster.Handle, c.Target, c.TargetLife, c.TargetPoint, 0, outcome, handler.Healing, c.ID, 0)
		return
	}
	for _, target := range targets {
		amount := int64(0)
		outcome := uint32(2)
		if handler.Healing {
			if target.HP < target.MaxHP {
				before := target.HealAmount
				target.HealAmount = queueAdd(target.HealAmount, c.Amount, w.e.Store.Cfg.Fixture.Healing.QueueCap)
				amount = target.HealAmount - before
				if amount > 0 {
					outcome = 4
				}
			}
		} else {
			damage := w.magicDamage(r, target, c.Amount)
			before := target.HP
			w.applyDamage(r, caster, target, damage, now, hitContext{At: c.Due, HostSession: c.HostSession, PrivilegeUntil: c.PrivilegeUntil, PrivilegeCaptured: c.PrivilegeCaptured})
			amount = before - target.HP
			if amount > 0 {
				outcome = 1
			}
		}
		w.impact(r, c.Skill.ID, c.Caster.Handle, target.Handle, target.Life, target.Pos, amount, outcome, handler.Healing, c.ID, 0)
	}
}
func (w *worker) advanceCasts(r *room, now time.Time) {
	remaining := r.Casts[:0]
	resolved := false
	for i, c := range r.Casts {
		if r.Pending {
			remaining = append(remaining, r.Casts[i:]...)
			break
		}
		if now.Before(c.Due) {
			remaining = append(remaining, c)
			continue
		}
		w.resolveCast(r, c, now)
		resolved = true
	}
	r.Casts = remaining
	if resolved {
		w.snapshot(r)
	}
}

func (w *worker) autoSkill(r *room, now time.Time) (*entity, bool) {
	skills := append([]devconfig.Skill(nil), w.e.Store.Cfg.Fixture.Skills...)
	sort.SliceStable(skills, func(i, j int) bool {
		if skills[i].AutoPriority == skills[j].AutoPriority {
			return skills[i].ID < skills[j].ID
		}
		return skills[i].AutoPriority > skills[j].AutoPriority
	})
	var approach *entity
	for _, s := range skills {
		handler, ok := skillHandlers[s.Behavior]
		if !ok || handler.Passive || s.JobID != r.Role.Job {
			continue
		}
		if _, learned := r.Role.Skill(s.ID); !learned {
			continue
		}
		var target *entity
		var grid *pb.GridPoint
		if handler.Healing {
			target = &r.Player
			if target.HP >= target.MaxHP && r.Pet != nil && r.Pet.HP > 0 && r.Pet.HP < r.Pet.MaxHP {
				target = r.Pet
			}
			if target.HP >= target.MaxHP || target.HealAmount > 0 {
				continue
			}
			if handler.Area {
				count := 0
				for _, e := range w.areaTargets(r, s, target.Pos) {
					if e.HP < e.MaxHP && e.HealAmount == 0 {
						count++
					}
				}
				if count < s.AutoMinTargets {
					continue
				}
				grid = pointPB(target.Pos)
				target = nil
			}
		} else {
			target = w.nearest(r, &r.Player)
			if target == nil {
				continue
			}
			if handler.Area {
				if len(w.areaTargets(r, s, target.Pos)) < s.AutoMinTargets {
					continue
				}
				grid = pointPB(target.Pos)
			}
			if distance(r.Player.Pos, target.Pos) > s.RangeCells || s.RequireLineOfSight && !w.lineOfSight(r.Player.Pos, target.Pos) {
				if approach == nil {
					approach = target
				}
				continue
			}
			if handler.Area {
				target = nil
			}
		}
		handle := uint32(0)
		if target != nil {
			handle = target.Handle
		}
		if w.cast(r, s.ID, handle, 0, now, grid) == 0 {
			return nil, true
		}
	}
	return approach, approach != nil
}
