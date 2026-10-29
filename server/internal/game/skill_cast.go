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

type skillHandler struct{ Passive, Healing, Area, UndeadBonus, Direction, SelfArea, Status, Displacement, Teleport, Summon bool }

type preparedCast struct {
	Scene                                 uint64 // 原附着身份独立于执行actorRef。
	Skill                                 devconfig.Skill
	Level                                 uint32
	Caster, Target                        actorRef
	Origin, Point                         devconfig.Point
	Direction                             uint32
	Cost, Amount, DelayMS, BuffDurationMS int64
	Summon                                *summonPetPlan
}

var skillHandlers = map[string]skillHandler{
	"fencing": {Passive: true}, "spirit_sword": {Passive: true},
	"fireball": {}, "great_fireball": {}, "thunderbolt": {UndeadBonus: true},
	"firebang": {Area: true}, "healing": {Healing: true}, "mass_healing": {Healing: true, Area: true},
	"magic_shield": {Status: true}, "soul_shield": {Status: true, Area: true}, "blessed_armour": {Status: true, Area: true},
	"slaying": {Passive: true}, "thrusting": {Passive: true}, "half_moon": {Passive: true}, "flaming_sword": {Passive: true},
	"hell_fire": {Direction: true}, "lightning": {Direction: true}, "ice_storm": {Area: true}, "thunder_storm": {Area: true, SelfArea: true},
	"repulsion": {SelfArea: true, Displacement: true}, "shoulder_dash": {Direction: true, Displacement: true},
	"teleport":   {Teleport: true},
	"summon_pet": {Summon: true},
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
	return w.areaTargetsInWorld(r, r.currentWorld(), s, grid)
}
func (w *worker) areaTargetsInWorld(r *room, world *worldState, s devconfig.Skill, grid devconfig.Point) []*entity {
	all := world.Monsters[:world.MonsterCount]
	if world == r.currentWorld() {
		all = append([]*entity{&r.Player}, all...)
	}
	all = append(all[:len(all):len(all)], r.worldPet(world))
	friendly := strings.HasPrefix(targetKind(s), "friendly")
	out := []*entity{}
	for _, e := range all {
		if e == nil || e.HP <= 0 || distance(e.Pos, grid) > s.AreaRadius || !w.worldWalkable(world, e.Pos) {
			continue
		}
		if (friendly && (e.Kind == 1 || e.Kind == 2)) || (!friendly && e.Kind == 3) {
			out = append(out, e)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Handle < out[j].Handle })
	return out
}

func (w *worker) lineTargets(r *room, c pendingCast) []*entity {
	return w.lineTargetsInWorld(r, r.currentWorld(), c)
}
func (w *worker) lineTargetsInWorld(r *room, world *worldState, c pendingCast) []*entity {
	if c.Skill.Behavior == "hell_fire" && c.LineRemaining > 0 {
		point := stepPoint(c.TargetPoint, c.Direction, 1)
		if !w.worldWalkable(world, point) {
			return nil
		}
		var out []*entity
		for _, e := range world.Monsters[:world.MonsterCount] {
			if e.HP > 0 && e.Pos == point {
				out = append(out, e)
			}
		}
		sort.Slice(out, func(i, j int) bool { return out[i].Handle < out[j].Handle })
		return out
	}
	var out []*entity
	for i := 1; i <= c.Skill.LineCells; i++ {
		point := stepPoint(c.TargetPoint, c.Direction, i)
		if !w.worldWalkable(world, point) {
			if c.Skill.LineStopOnBlocked {
				break
			}
			continue
		}
		if target := w.enemyAtInWorld(world, point); target != nil {
			out = append(out, target)
		}
	}
	return out
}
func (w *worker) cast(r *room, id, targetHandle, input uint32, now time.Time, ground ...*pb.GridPoint) uint32 {
	return w.castDirected(r, id, targetHandle, input, r.Player.Direction, now, ground...)
}
func (w *worker) castDirected(r *room, id, targetHandle, input, direction uint32, now time.Time, ground ...*pb.GridPoint) uint32 {
	plan, code := w.prepareCast(r, id, targetHandle, direction, now, false, ground...)
	if code != 0 {
		return code
	}
	return w.acceptPreparedCast(r, plan, input, now)
}
func (w *worker) prepareCast(r *room, id, targetHandle, direction uint32, now time.Time, materialAllowed bool, ground ...*pb.GridPoint) (*preparedCast, uint32) {
	if w.mapCellBudget(r) == 0 {
		return nil, p.CodeSkillStateBlocked
	}
	if direction > 7 {
		return nil, p.CodeSkillTarget
	}
	s, exists := w.e.Store.Cfg.Fixture.Skill(id)
	level, learned := r.Role.Skill(id)
	handler, registered := skillHandlers[s.Behavior]
	if !exists || !learned || !registered || s.JobID != r.Role.Job || level < s.LearnedLevel || level > s.MaxLevel {
		return nil, p.CodeSkillNotLearned
	}
	if handler.Passive || r.Pending || r.ScheduleID == math.MaxUint64 || r.CastID == math.MaxUint64 || r.Player.HP <= 0 || now.Before(r.Player.NextMove) || now.Before(r.Player.ActionReadyAt) || r.eventCount() >= w.eventCapacity() || !w.killBudgetAvailable(r) {
		return nil, p.CodeSkillStateBlocked
	}
	if now.Before(r.NextSpell) || now.Before(r.SkillCooldowns[id]) {
		return nil, p.CodeSkillCooldown
	}
	if handler.Teleport {
		if s.Teleport == nil || !w.trustedTeleportBinding(r) {
			return nil, p.CodeSkillStateBlocked
		}
		if targetHandle != 0 || len(ground) > 0 && ground[0] != nil {
			return nil, p.CodeSkillTarget
		}
	}
	var target *entity
	var point devconfig.Point
	var summon *summonPetPlan
	if handler.Summon {
		if targetHandle != 0 || len(ground) > 0 && ground[0] != nil {
			return nil, p.CodeSkillTarget
		}
		var code uint32
		summon, code = w.prepareSummonPet(r, s, direction, now)
		if code != 0 {
			return nil, code
		}
		point = r.Player.Pos
	} else if handler.Direction || handler.SelfArea || s.TargetKind == "friendly_self" {
		if targetHandle != 0 || direction > 7 {
			return nil, p.CodeSkillTarget
		}
		if len(ground) > 0 && ground[0] != nil && !w.inBounds(r, devconfig.Point{X: int(ground[0].X), Y: int(ground[0].Y)}) {
			return nil, p.CodeSkillTarget
		}
		point = r.Player.Pos
	} else if handler.Area {
		if targetHandle != 0 || len(ground) == 0 || ground[0] == nil {
			return nil, p.CodeSkillTarget
		}
		point = devconfig.Point{X: int(ground[0].X), Y: int(ground[0].Y)}
		if !w.inBounds(r, point) {
			return nil, p.CodeSkillTarget
		}
	} else {
		target = w.skillTarget(r, s, targetHandle)
		if target == nil {
			return nil, p.CodeSkillTarget
		}
		point = target.Pos
	}
	if distance(r.Player.Pos, point) > s.RangeCells {
		return nil, p.CodeSkillTarget
	}
	requireFlight := s.RequireLineOfSight || (s.TargetKind == "" && s.Behavior == "fireball")
	if requireFlight && !w.lineOfSight(r, r.Player.Pos, point) {
		return nil, p.CodeSkillTarget
	}
	cost, validCost := teleportManaCost(&r.Player, s, level)
	if !validCost {
		return nil, p.CodeSkillStateBlocked
	}
	if r.Player.MP < cost {
		return nil, p.CodeSkillResource
	}
	if r.eventCount()+1 > w.eventCapacity() {
		return nil, p.CodeSkillStateBlocked
	}
	if s.MaterialQuantity > 0 && !materialAllowed && (summon == nil || !summon.Recall) {
		return nil, p.CodeSkillMaterialRequired
	}
	stats := w.fighter(r, &r.Player)
	amount := int64(0)
	duration := int64(0)
	if handler.Teleport || handler.Summon {
		// 传送没有伤害或治疗随机量，成功后的状态在实际移交完成时才添加。
	} else if handler.Status {
		definition, ok := w.e.Store.Cfg.Fixture.Buff(s.BuffID)
		if !ok {
			return nil, p.CodeSkillNotLearned
		}
		switch definition.DurationKind {
		case "mc_source_power":
			stat := w.interval(stats.MagicMin, stats.MagicMax)
			power := s.PowerBase + w.draw(s.PowerBonus)
			duration = buffDurationSeconds(s, definition, level, stat, power)
		case "sc_rank_linear":
			duration = buffDurationSeconds(s, definition, level, w.interval(stats.TaoMin, stats.TaoMax), 0)
		}
		if duration <= 0 || duration > math.MaxInt32/int64(time.Second/time.Millisecond) {
			return nil, p.CodeSkillStateBlocked
		}
		duration *= int64(time.Second / time.Millisecond)
	} else if handler.Displacement {
		if s.Displacement == nil {
			return nil, p.CodeSkillStateBlocked
		}
		// 野蛮只以0进入源技能公式，不抽职业DC/MC；抗拒怪物分支没有伤害计算。
		if s.Behavior == "shoulder_dash" {
			amount = w.magicAmount(s, level, 0)
		}
	} else if handler.Healing {
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
		return nil, p.CodeSkillStateBlocked
	}
	plan := &preparedCast{Scene: r.Scene, Skill: s, Level: level, Caster: reference(r, &r.Player), Origin: r.Player.Pos, Point: point, Cost: cost, Amount: amount, DelayMS: delay, BuffDurationMS: duration, Direction: r.Player.Direction}
	plan.Summon = summon
	if target != nil {
		plan.Target = reference(r, target)
	}
	if handler.Direction || handler.Summon {
		plan.Direction = direction
	} else if !handler.SelfArea && !handler.Summon && s.TargetKind != "friendly_self" {
		plan.Direction = facing(r.Player.Pos, point, r.Player.Direction)
	}
	return plan, 0
}

func (w *worker) acceptPreparedCast(r *room, plan *preparedCast, input uint32, now time.Time) uint32 {
	if plan == nil || r.ScheduleID == math.MaxUint64 || r.CastID == math.MaxUint64 || plan.Scene != r.Scene || plan.Caster.World != r.currentWorld().Ref || plan.Caster.Life != r.Player.Life || r.Player.HP <= 0 || !w.summonPlanCurrent(r, plan) {
		return p.CodeSkillStateBlocked
	}
	s, level := plan.Skill, plan.Level
	id := s.ID
	point, amount, delay, cost := plan.Point, plan.Amount, plan.DelayMS, plan.Cost

	r.Player.MP -= cost
	r.Player.Action = 5
	r.Player.ActionUntil = now.Add(time.Duration(s.ActionMS) * time.Millisecond)
	r.Player.ActionReadyAt = r.Player.ActionUntil
	r.Player.NextAttack = r.Player.ActionReadyAt
	r.Player.CastSequence++
	r.Player.CastSkillID = id
	r.CastID++
	r.ScheduleID++
	r.Player.Direction = plan.Direction
	r.NextSpell = now.Add(time.Duration(s.GlobalSpellMS) * time.Millisecond)
	if r.SkillCooldowns == nil {
		r.SkillCooldowns = map[uint32]time.Time{}
	}
	r.SkillCooldowns[id] = now.Add(time.Duration(s.CooldownBaseMS-int64(level)*s.CooldownReductionMS) * time.Millisecond)
	handle, life := plan.Target.Handle, plan.Target.Life
	event := pendingCast{Scene: r.Scene, Skill: s, Target: handle, TargetLife: life, TargetPoint: point, Due: now.Add(time.Duration(delay) * time.Millisecond), Amount: amount, Caster: plan.Caster, ID: r.CastID, Order: r.ScheduleID, Direction: r.Player.Direction, PrivilegeUntil: r.Role.PrivilegeUntil, PrivilegeCaptured: true, SkillLevel: level, BuffDurationMS: plan.BuffDurationMS}
	event.Summon = plan.Summon
	if s.Behavior == "hell_fire" {
		event.LineRemaining = s.LineCells
	}
	if r.Conn == nil {
		event.HostSession = r.HostSession
	}
	if !skillHandlers[s.Behavior].Displacement {
		r.Casts = append(r.Casts, event)
	}
	w.send(r, p.MsgS2CCastAccepted, 0, &pb.CastAccepted{SceneEpoch: r.Scene, CastId: r.CastID, SkillId: id, CasterHandle: r.Player.Handle, TargetHandle: handle, Origin: pointPB(plan.Origin), TargetGrid: pointPB(point), Direction: r.Player.Direction, DelayMs: uint32(delay), InputSequence: input, CasterLife: r.Player.Life, TargetLife: life, SkillLevel: level})
	if skillHandlers[s.Behavior].Displacement {
		w.resolveDisplacementSkill(r, event, now)
		return 0
	}
	w.snapshot(r)
	return 0
}

func (w *worker) resolveCast(r *room, c pendingCast, now time.Time) {
	w.resolveCastInWorld(r, r.world(c.Caster.World), c, now)
}
func (w *worker) resolveCastInWorld(r *room, world *worldState, c pendingCast, now time.Time) {
	if world == nil || r.world(c.Caster.World) != world {
		return
	}
	if _, known := w.worldMap(world); !known {
		return
	}
	handler, registered := skillHandlers[c.Skill.Behavior]
	caster := w.resolveInWorld(r, world, c.Caster)
	if c.Skill.Behavior == "hell_fire" {
		caster = w.resolveCredit(r, c.Caster)
	}
	if !registered || caster == nil {
		w.impactInWorld(r, world, c.Scene, c.Skill.ID, c.Caster.Handle, c.Target, c.TargetLife, c.TargetPoint, 0, 3, handler.Healing, c.ID, 0)
		return
	}
	targets := []*entity{}
	if handler.Status {
		if world == r.currentWorld() {
			targets = w.buffTargets(r, c)
		}
	} else if handler.Direction {
		targets = w.lineTargetsInWorld(r, world, c)
		if c.Skill.Behavior == "hell_fire" {
			point := stepPoint(c.TargetPoint, c.Direction, 1)
			if !w.worldWalkable(world, point) {
				return
			}
			if c.LineRemaining > 1 && r.ScheduleID == math.MaxUint64 {
				// 已付费地图动作不能回绕为0次序；隔离保留其它原事件，不制造无序后继。
				w.isolateRoom(r)
				return
			}
			if r.observesWorld(world, c.Scene) {
				w.send(r, p.MsgS2CCastStep, 0, &pb.CastStep{SceneEpoch: r.Scene, CastId: c.ID, SkillId: c.Skill.ID, CasterHandle: c.Caster.Handle, CasterLife: c.Caster.Life, Grid: pointPB(point), Direction: c.Direction, StepIndex: uint32(c.Skill.LineCells - c.LineRemaining + 1), SkillLevel: c.SkillLevel})
			}
			if c.LineRemaining > 1 {
				// Crystal Map.CompleteMagic按本次实际处理时间创建下一格，不追赶卡帧。
				child := c
				child.TargetPoint, child.LineRemaining = point, c.LineRemaining-1
				child.Due = now.Add(time.Duration(c.Skill.LineStepMS) * time.Millisecond)
				r.ScheduleID++
				child.Order = r.ScheduleID
				child.PrivilegeUntil, child.PrivilegeCaptured = r.Role.PrivilegeUntil, true
				child.HostSession = ""
				if r.Conn == nil {
					child.HostSession = r.HostSession
				}
				world.Casts = append(world.Casts, child)
			}
		}
	} else if handler.Area {
		targets = w.areaTargetsInWorld(r, world, c.Skill, c.TargetPoint)
	} else {
		target := w.skillTargetInWorld(r, world, c.Skill, c.Target)
		if target != nil && target.Life == c.TargetLife && (!handler.Healing && distance(target.Pos, c.TargetPoint) <= c.Skill.TargetMoveToleranceCells || handler.Healing) {
			targets = append(targets, target)
		}
	}
	if len(targets) == 0 {
		outcome := uint32(3)
		if handler.Area || handler.Direction {
			outcome = 2
		}
		point := c.TargetPoint
		if c.Skill.Behavior == "hell_fire" {
			point = stepPoint(point, c.Direction, 1)
		}
		w.impactInWorld(r, world, c.Scene, c.Skill.ID, c.Caster.Handle, c.Target, c.TargetLife, point, 0, outcome, handler.Healing, c.ID, 0)
		return
	}
	for _, target := range targets {
		if handler.Status {
			w.applyBuff(r, c, target, now)
			continue
		}
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
			value := c.Amount
			if c.Skill.Behavior == "thunder_storm" && target.Monster != nil && !target.Monster.Undead {
				value /= c.Skill.NonUndeadDivisor
			}
			damage := w.magicDamage(r, target, value)
			before := target.HP
			w.applyDamageInWorld(r, world, caster, target, damage, now, hitContext{At: c.Due, HostSession: c.HostSession, PrivilegeUntil: c.PrivilegeUntil, PrivilegeCaptured: c.PrivilegeCaptured})
			amount = before - target.HP
			if amount > 0 {
				outcome = 1
			}
		}
		w.impactInWorld(r, world, c.Scene, c.Skill.ID, c.Caster.Handle, target.Handle, target.Life, target.Pos, amount, outcome, handler.Healing, c.ID, 0)
	}
}
func (w *worker) autoSkill(r *room, now time.Time) (*entity, bool) {
	pet := r.worldPet(r.currentWorld())
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
		if !ok || !s.AutoEnabled || handler.Passive || s.Activation() != "cast" || s.JobID != r.Role.Job {
			continue
		}
		rank, learned := r.Role.Skill(s.ID)
		if !learned || rank < s.LearnedLevel || rank > s.MaxLevel || now.Before(r.NextSpell) || now.Before(r.SkillCooldowns[s.ID]) || r.Player.MP < s.MPBaseCost+int64(rank)*s.MPLevelCost {
			continue
		}
		var target *entity
		var grid *pb.GridPoint
		direction := r.Player.Direction
		if handler.Status {
			missing := func(e *entity) bool { return e != nil && e.HP > 0 && e.Statuses[s.BuffID] == nil }
			center := r.Player.Pos
			if s.TargetKind == "friendly_self" {
				if !missing(&r.Player) {
					continue
				}
			} else {
				if !missing(&r.Player) {
					if !missing(pet) {
						continue
					}
					center = pet.Pos
				}
				count := 0
				for _, e := range []*entity{&r.Player, pet} {
					if missing(e) && distance(e.Pos, center) <= s.AreaRadius && w.walkable(r, e.Pos) {
						count++
					}
				}
				if count < s.AutoMinTargets {
					continue
				}
			}
			if s.MaterialQuantity > 0 {
				if w.autoMaterialCast(r, s, center, now) {
					return nil, true
				}
			} else if w.castDirected(r, s.ID, 0, 0, direction, now, pointPB(center)) == 0 {
				return nil, true
			}
			continue
		}
		if handler.SelfArea {
			if len(w.areaTargets(r, s, r.Player.Pos)) < s.AutoMinTargets {
				continue
			}
			if w.castDirected(r, s.ID, 0, 0, direction, now) == 0 {
				return nil, true
			}
			continue
		}
		if handler.Direction {
			count := 0
			for d := range directions {
				n := len(w.lineTargets(r, pendingCast{Skill: s, TargetPoint: r.Player.Pos, Direction: uint32(d)}))
				if n > count {
					count, direction = n, uint32(d)
				}
			}
			if count < s.AutoMinTargets {
				if approach == nil {
					nearest := w.nearest(r, &r.Player)
					if nearest != nil && distance(r.Player.Pos, nearest.Pos) > 1 {
						approach = nearest
					}
				}
				continue
			}
			if w.castDirected(r, s.ID, 0, 0, direction, now) == 0 {
				return nil, true
			}
			continue
		}
		if handler.Healing {
			target = &r.Player
			if target.HP >= target.MaxHP && pet != nil && pet.HP > 0 && pet.HP < pet.MaxHP {
				target = pet
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
			if distance(r.Player.Pos, target.Pos) > s.RangeCells || s.RequireLineOfSight && !w.lineOfSight(r, r.Player.Pos, target.Pos) {
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
		if w.castDirected(r, s.ID, handle, 0, direction, now, grid) == 0 {
			return nil, true
		}
	}
	return approach, approach != nil
}
