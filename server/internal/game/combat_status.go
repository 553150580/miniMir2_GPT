// 本文件计算、施加和推进战斗状态效果，生成同步数据并处理伤害减免、磨损和死亡清理。
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
	// TeleportManaPenaltyPercent 记录瞬移魔法值“penalty”“percent”，使用数值表示。
	TeleportManaPenaltyPercent int64
	// ID 区分当前步骤指向的目标对象。
	ID uint32
	// Kind 记录类型，使用文本表示。
	Kind string
	// SourceSkillID 标识当前业务使用的来源技能。
	SourceSkillID uint32
	// SkillLevel 记录技能等级，使用数值表示。
	SkillLevel uint32
	// Caster 记录施法者，使用行动实体引用表示。
	Caster actorRef
	// TargetLife 记录目标生命代次，使用数值表示。
	TargetLife uint64
	// RemainingMS 记录剩余额度毫秒，使用数值表示。
	RemainingMS int64
	// LastProcess 记录上一次状态“process”，使用时间点表示。
	LastProcess time.Time
	// NextProcess 记录下一状态“process”，使用时间点表示。
	NextProcess time.Time
	// TickMS 记录模拟帧毫秒，使用数值表示。
	TickMS int64
	// MaxPhysicalDefenceBonus 记录最大物理防御加成，使用数值表示。
	MaxPhysicalDefenceBonus int64
	// MaxMagicDefenceBonus 记录最大魔法防御加成，使用数值表示。
	MaxMagicDefenceBonus int64
	// DamageReductionPercent 记录伤害“reduction”“percent”，使用数值表示。
	DamageReductionPercent int64
	// DamageWearMS 记录伤害磨损毫秒，使用数值表示。
	DamageWearMS int64
	// RemoveOnDeath 标记是否处于移除启用状态死亡状态。
	RemoveOnDeath bool
}

// buffDurationSeconds 返回状态效果持续时间秒集合供调用方使用。
// skill 记录技能，definition 记录定义，使用状态效果定义表示，rank 记录排序位次，使用数值表示。
// stat 记录属性，使用数值表示，power 记录能力值，使用数值表示。
func buffDurationSeconds(skill devconfig.Skill, definition devconfig.BuffDefinition, rank uint32, stat, power int64) int64 {
	if definition.DurationKind == "mc_source_power" {
		return skill.SourcePower(rank, stat+definition.StatAdd, power)
	}
	return stat*definition.StatMultiplier + definition.DurationSecondsByLevel[rank]
}

// statusTargetBonus 返回状态目标加成供调用方使用。
// level 记录等级，使用数值表示，definition 记录定义，使用状态效果定义表示。
func statusTargetBonus(level uint32, definition devconfig.BuffDefinition) int64 {
	return int64(level)/definition.TargetLevelDivisor + definition.TargetLevelAdd
}

// statusFighter 返回状态战斗单位供调用方使用。
// e 记录实体，base 记录基础值，使用战斗单位表示。
func statusFighter(e *entity, base devconfig.Fighter) devconfig.Fighter {
	// status 接收遍历状态集合时当前的元素。
	for _, status := range e.Statuses {
		if status.TargetLife != e.Life {
			continue
		}
		base.DefenceMax += status.MaxPhysicalDefenceBonus
		base.MagicDefenceMax += status.MaxMagicDefenceBonus
	}
	return base
}

// statusReduction 返回状态“reduction”供调用方使用。
// e 记录实体。
func statusReduction(e *entity) int64 {
	// total 记录合计，使用数值表示。
	var total int64
	// status 接收遍历状态集合时当前的元素。
	for _, status := range e.Statuses {
		if status.TargetLife == e.Life {
			total += status.DamageReductionPercent
		}
	}
	return total
}

// 来源先对原始伤害做整数减伤，再扣AC/MAC；不能把已扣防御的净伤害再次乘比例。
// e 记录实体，raw 记录原始值，使用数值表示，armour 记录“armour”，使用数值表示。
func statusDamage(e *entity, raw, armour int64) int64 {
	// reduction 保存由状态“reduction”取得的“reduction”。
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
// e 记录实体，damage 记录伤害，使用数值表示。
func wearStatuses(e *entity, damage int64) {
	if damage <= 0 {
		return
	}
	// status 接收遍历状态集合时当前的元素。
	for _, status := range e.Statuses {
		if status.TargetLife != e.Life || status.DamageWearMS <= 0 {
			continue
		}
		if damage > math.MaxInt64/status.DamageWearMS {
			status.RemainingMS = math.MinInt64
			continue
		}
		// cost 保存根据伤害与伤害磨损毫秒计算的消耗。
		cost := damage * status.DamageWearMS
		if status.RemainingMS < math.MinInt64+cost {
			status.RemainingMS = math.MinInt64
		} else {
			status.RemainingMS -= cost
		}
	}
}

// removeDeathStatuses 移除死亡状态集合。
// e 记录实体。
func removeDeathStatuses(e *entity) {
	// id 定位状态集合中正在处理的成员，status 接收遍历状态集合时当前的元素。
	for id, status := range e.Statuses {
		if status.RemoveOnDeath {
			delete(e.Statuses, id)
		}
	}
}

// tickStatuses 判断模拟帧状态集合是否满足要求。
// w 是调用该方法的房间执行器实例，r 指向当前执行器拥有并更新的目标房间，now 提供当前时点，作为状态有效期和计时判断的基准。
func (w *worker) tickStatuses(r *room, now time.Time) bool {
	// all 保留加入新条目后的全部。
	all := append([]*entity{&r.Player, r.Pet}, r.Monsters...)
	// world 接收遍历“reactive”场景世界集合时当前的元素。
	for _, world := range w.reactiveWorlds(r) {
		all = append(all, world.Monsters...)
	}
	// world 接收遍历场景世界集合时当前的元素。
	for _, world := range r.worlds() {
		// corpse 接收遍历宠物尸体集合时当前的元素。
		for _, corpse := range world.PetCorpses {
			all = append(all, corpse.Body)
		}
	}
	return tickEntityStatuses(all, now)
}

// tickEntityStatuses 判断模拟帧实体状态集合是否满足要求。
// all 记录全部，使用实体列表表示，now 提供当前时点，作为状态有效期和计时判断的基准。
func tickEntityStatuses(all []*entity, now time.Time) bool {
	// changed 保存当前处理所用的已变化。
	changed := false
	// e 接收遍历全部时当前的元素。
	for _, e := range all {
		if e == nil {
			continue
		}
		// id 定位状态集合中正在处理的成员，status 接收遍历状态集合时当前的元素。
		for id, status := range e.Statuses {
			if status.TargetLife != e.Life {
				delete(e.Statuses, id)
				changed = true
				continue
			}
			// 固定传送状态在实际到期帧移除，不用数值tick节拍延长耗蓝惩罚。
			if status.Kind == "teleport_mana_penalty" && now.Sub(status.LastProcess).Milliseconds() >= status.RemainingMS {
				delete(e.Statuses, id)
				changed = true
				continue
			}
			if status.NextProcess.After(now) {
				continue
			}
			// elapsed 将子项换算为毫秒，供后续计量。
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

// applyBuff 应用“buff”。
// w 是调用该方法的房间执行器实例，r 指向当前执行器拥有并更新的目标房间，c 记录待确认施法，target 记录目标，使用实体表示，now 提供当前时点，作为状态有效期和计时判断的基准。
func (w *worker) applyBuff(r *room, c pendingCast, target *entity, now time.Time) bool {
	// definition 保存由状态效果取得的定义，exists 保存由状态效果取得的存在。
	definition, exists := w.e.Store.Cfg.Fixture.Buff(c.Skill.BuffID)
	if !exists || target.IsPetCorpse || !definition.AllowDeadTargets && target.HP <= 0 {
		return false
	}
	// old 保存从状态集合取出的原状态。
	if old := target.Statuses[definition.ID]; old != nil && old.TargetLife == target.Life {
		if definition.StackPolicy == "ignore_present_on_impact" {
			return false
		}
		// 来源刷新仅替换剩余时长；首次数值、来源施法者以及Last/Next时钟全部保留。
		old.RemainingMS = c.BuffDurationMS
		if definition.DurationKind == "fixed_ms" {
			old.LastProcess = now
			old.NextProcess = now.Add(time.Duration(definition.TickMS) * time.Millisecond)
		}
		return true
	}
	if len(target.Statuses) >= len(w.e.Store.Cfg.Fixture.Buffs) {
		return false
	}
	// status 保存当前处理所用的状态。
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
	case "teleport_mana_penalty":
		status.TeleportManaPenaltyPercent = definition.ValueByLevel[c.SkillLevel]
		status.NextProcess = now.Add(time.Duration(definition.TickMS) * time.Millisecond)
	}
	if target.Statuses == nil {
		target.Statuses = map[uint32]*combatStatus{}
	}
	target.Statuses[definition.ID] = status
	return true
}

// buffTargets 返回状态效果目标集合供调用方使用。
// w 是调用该方法的房间执行器实例，r 指向当前执行器拥有并更新的目标房间，c 记录待确认施法。
func (w *worker) buffTargets(r *room, c pendingCast) []*entity {
	// b 保存状态效果返回的结果，ok 表明查询、校验或类型转换是否成功。
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
	// out 记录输出，使用实体列表表示。
	var out []*entity
	// target 接收遍历实体列表时当前的元素。
	for _, target := range []*entity{&r.Player, r.worldPet(r.currentWorld())} {
		if target != nil && (b.AllowDeadTargets || target.HP > 0) && distance(target.Pos, c.TargetPoint) <= c.Skill.AreaRadius && w.walkable(r, target.Pos) {
			out = append(out, target)
		}
	}
	return out
}

// statusesPB 返回状态集合“pb”供调用方使用。
// e 记录实体，now 提供当前时点，作为状态有效期和计时判断的基准。
func statusesPB(e *entity, now time.Time) []*pb.EntityStatusState {
	// ids 创建用于维护标识集合的容器或通道。
	ids := make([]uint32, 0, len(e.Statuses))
	// id 定位状态集合中正在处理的成员，status 接收遍历状态集合时当前的元素。
	for id, status := range e.Statuses {
		if status.TargetLife == e.Life {
			ids = append(ids, id)
		}
	}
	// i 记录当前位置的下标，使用数值表示，j 记录当前位置的下标，使用数值表示。
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	// out 创建用于维护输出的容器或通道。
	out := make([]*pb.EntityStatusState, 0, len(ids))
	// id 接收遍历标识集合时当前的元素。
	for _, id := range ids {
		// status 保存从状态集合取出的状态。
		status := e.Statuses[id]
		// elapsed 将子项换算为毫秒，供后续计量。
		elapsed := now.Sub(status.LastProcess).Milliseconds()
		if elapsed < 0 {
			elapsed = 0
		}
		// remaining 保存剩余额度毫秒，用于当前步骤中的剩余额度。
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
			CasterHandle: status.Caster.Handle, CasterLife: status.Caster.Life, TeleportManaPenaltyPercent: uint32(status.TeleportManaPenaltyPercent)})
	}
	return out
}

// combatAttributesPB 返回战斗“attributes”“pb”供调用方使用。
// w 是调用该方法的房间执行器实例，r 指向当前执行器拥有并更新的目标房间，e 记录实体。
func (w *worker) combatAttributesPB(r *room, e *entity) *pb.CombatAttributes {
	// a 保存战斗单位返回的结果。
	a := w.fighter(r, e)
	return &pb.CombatAttributes{PhysicalAttackMin: a.AttackMin, PhysicalAttackMax: a.AttackMax, PhysicalDefenceMin: a.DefenceMin, PhysicalDefenceMax: a.DefenceMax,
		MagicDefenceMin: a.MagicDefenceMin, MagicDefenceMax: a.MagicDefenceMax, MagicAttackMin: a.MagicMin, MagicAttackMax: a.MagicMax,
		SpiritAttackMin: a.TaoMin, SpiritAttackMax: a.TaoMax, Accuracy: a.Accuracy, Agility: a.Agility, DamageReductionPercent: uint32(statusReduction(e))}
}
