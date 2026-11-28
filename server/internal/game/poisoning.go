// 本文件推进道士施毒的原目标、两色覆盖规则和离散伤害。
package game

import (
	"math"
	"slices"
	"time"

	"minimir/internal/devconfig"
)

// poisonInterval 是来源每次毒性推进之间的间隔。
const poisonInterval = 2 * time.Second

// monsterPoisons 分别保存同一怪物生命上的红毒和绿毒，两色同时作用。
type monsterPoisons struct {
	Green *poisonEffect // 通过真实伤害管线按期扣除生命的绿毒。
	Red   *poisonEffect // 生效期间使抽取后的实际防御减半的红毒。
}

// poisonEffect 由应用时的颜色、原身体和剩余的离散处理次数唯一确定。
type poisonEffect struct {
	Color      devconfig.PoisonColor // 即使之后更换专槽仍保存消耗时的颜色。
	TargetLife uint64                // 防止原毒污染采用同一怪物槽位的重生身体。
	Remaining  int64                 // 移除之前的剩余处理次数，最后一次只做移除。
	Damage     int64                 // 绿毒每次直接扣血值；红毒始终为零。
}

// poisonSlot 定位毒色对应的独立状态引用，仅在颜色已经过材料白名单验证时调用。
func (state *monsterPoisons) poisonSlot(color devconfig.PoisonColor) **poisonEffect {
	if color == devconfig.PoisonGreen {
		return &state.Green
	}
	return &state.Red
}

// hasPoison 判断同一怪物的当前生命上是否已有指定颜色的毒。
func hasPoison(target *entity, color devconfig.PoisonColor) bool {
	if target.Poisons == nil || color != devconfig.PoisonGreen && color != devconfig.PoisonRed {
		return false
	}
	effect := *target.Poisons.poisonSlot(color)
	return effect != nil && effect.TargetLife == target.Life
}

// poisonArmour 缩放实际抽出的原防御，同时排除旧生命上的毒。
func poisonArmour(target *entity, armour int64) int64 {
	if target.Poisons != nil && target.Poisons.Red != nil && target.Poisons.Red.TargetLife == target.Life {
		return armour / 2
	}
	return armour
}

// beginPoison 根据原道术结果和阶位生成一个状态。弱绿毒及更短红毒不能覆盖原状态。
func beginPoison(world *worldState, target *entity, cast pendingCast, damageAmount int64, resistanceBP uint32) *poisonEffect {
	if cast.PoisonColor != devconfig.PoisonGreen && cast.PoisonColor != devconfig.PoisonRed || cast.Amount < 0 {
		return nil
	}
	// rank 使用原生阶加一，防止初学0级失去原作用。
	rank := int64(cast.SkillLevel) + 1
	if cast.Amount > (math.MaxInt64-rank*7)/2 {
		return nil
	}
	// effect 的离散次数与当前伤害强度独立；红毒只有降防。
	effect := &poisonEffect{Color: cast.PoisonColor, TargetLife: target.Life,
		Remaining: devconfig.ReduceByBP(cast.Amount*2+rank*7, resistanceBP)}
	if effect.Color == devconfig.PoisonGreen {
		effect.Damage = damageAmount/15 + rank
	}
	if target.Poisons == nil {
		target.Poisons = &monsterPoisons{}
	}
	// slot 是新旧同色状态的唯一执行位置。
	slot := target.Poisons.poisonSlot(effect.Color)
	if current := *slot; current != nil && current.TargetLife == target.Life {
		if effect.Color == devconfig.PoisonGreen && current.Damage > effect.Damage || effect.Color == devconfig.PoisonRed && current.Remaining > effect.Remaining {
			return nil
		}
		// 被替换状态只可能还有一个在队后继，已进入当前批次的旧事件由指针身份拦截。
		world.Casts = slices.DeleteFunc(world.Casts, func(waiting pendingCast) bool { return waiting.Poison == current })
	}
	*slot = effect
	return effect
}

// clearPoison 只释放当前事件仍拥有的红毒或绿毒，防止迟到的旧事件清除新状态。
func clearPoison(target *entity, effect *poisonEffect) {
	if target.Poisons == nil || *target.Poisons.poisonSlot(effect.Color) != effect {
		return
	}
	*target.Poisons.poisonSlot(effect.Color) = nil
	if target.Poisons.Red == nil && target.Poisons.Green == nil {
		target.Poisons = nil
	}
}

// resolvePoisoning 处理原施法应用与已经应用的红绿毒后继，保留原世界和各次真实命中归属。
func (w *worker) resolvePoisoning(r *room, world *worldState, caster *entity, cast pendingCast, now time.Time) {
	// target 包含原世界和出生代次，即便相同句柄重生也不能接受此前的毒。
	target := w.resolveInWorld(r, world, actorRef{World: world.Ref, Handle: cast.Target, Life: cast.TargetLife})
	if target == nil || (target.Kind != 3 || target.Monster == nil) &&
		(world.mirrorCombat == nil || !slices.Contains(world.combatOpponents(), target)) {
		return
	}
	// effect 为 nil 表示本次只是尚未应用的施法，仍要求施法者留在同一世界。
	effect := cast.Poison
	if effect == nil {
		if caster != &r.Player || target.HP <= 0 {
			return
		}
		effect = beginPoison(world, target, cast, w.damageWithLuck(r, target, cast.Amount, cast.Luck), w.masteryBonuses(r, target).ResistanceBP)
		if effect == nil {
			return
		}
		exposeHiddenAttacker(world, caster, target)
	} else {
		if target.Poisons == nil || *target.Poisons.poisonSlot(effect.Color) != effect {
			return
		}
		// 已应用的毒跟随目标身体；原施法者离图但仍同一生命时继续提供信用。
		caster = w.resolveCredit(r, cast.Caster)
	}
	if caster == nil || target.HP <= 0 || effect.Remaining <= 1 {
		clearPoison(target, effect)
		return
	}
	effect.Remaining--
	if effect.Color == devconfig.PoisonGreen {
		// before 与 context 分别记录实际减少的血量和该次时间线事件的归属。
		before := target.HP
		context := hitContext{At: cast.Due, HostSession: cast.HostSession, PrivilegeUntil: cast.PrivilegeUntil, PrivilegeCaptured: cast.PrivilegeCaptured, PeriodicPoison: true}
		w.applyDamageInWorld(r, world, caster, target, effect.Damage, now, context)
		w.impactInWorld(r, world, cast.Scene, cast.Skill.ID, caster.Handle, target.Handle, target.Life, target.Pos, before-target.HP, 1, false, cast.ID, 0)
		if target.HP <= 0 || r.Isolated {
			clearPoison(target, effect)
			return
		}
	}
	if r.ScheduleID == math.MaxUint64 {
		w.isolateRoom(r)
		return
	}
	// next 记录同色状态的下一次处理，从实际运行时钟推进，不为晚到而多扣血。
	next := cast
	next.Poison, next.Due = effect, now.Add(poisonInterval)
	next.HostSession = ""
	if r.Conn == nil {
		next.HostSession = r.HostSession
	}
	next.PrivilegeUntil, next.PrivilegeCaptured = r.Role.PrivilegeUntil, true
	r.ScheduleID++
	next.Order = r.ScheduleID
	world.Casts = append(world.Casts, next)
}
