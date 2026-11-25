// 本文件为定点普通攻击和自动治疗寻找合法目标，维持射程与移动限制。
package game

import (
	"time"

	"minimir/internal/devconfig"
)

// skillRequiresFlight 判断技能要求“flight”是否满足要求。
// skill 记录技能。
func skillRequiresFlight(skill devconfig.Skill) bool {
	return skill.RequireLineOfSight || skill.TargetKind == "" && skill.Behavior == "fireball"
}

// 自动普攻只使用当前脚点能打到的目标，刺杀沿用真实第二格判定。
// w 是调用该方法的房间执行器实例，r 指向当前执行器拥有并更新的目标房间，now 提供当前时点，作为状态有效期和计时判断的基准。
func (w *worker) autoAttack(r *room, now time.Time) {
	// locked 保存由存活自动目标取得的已锁定。
	if locked := r.liveAutoTarget(); locked != nil {
		w.autoAttackMonster(r, locked, now)
		return
	}
	// cells 接收遍历数值列表时当前的元素。
	for _, cells := range []int{1, 2} {
		// target 接收遍历怪物集合中的范围时当前的元素。
		for _, target := range r.Monsters[:r.MonsterCount] {
			if target != nil && distance(r.Player.Pos, target.Pos) == cells && w.autoAttackMonster(r, target, now) {
				return
			}
		}
	}
}

// autoAttackMonster 判断自动攻击怪物是否满足要求。
// w 是调用该方法的房间执行器实例，r 指向当前执行器拥有并更新的目标房间，target 记录目标，使用实体表示，now 提供当前时点，作为状态有效期和计时判断的基准。
func (w *worker) autoAttackMonster(r *room, target *entity, now time.Time) bool {
	// cells 保存由距离取得的地图格集合。
	cells := distance(r.Player.Pos, target.Pos)
	if target.HP <= 0 || cells < 1 || cells > 2 || cells == 2 && (!w.sourceTimeline() || !r.Melee.Thrusting || len(r.Role.SkillSlotIDs) == devconfig.AttackSkillSlotCount) {
		return false
	}
	// ref 接收引用供后续步骤使用。
	ref := reference(r, target)
	// sequence 保存攻击序号，用于当前步骤中的序号。
	sequence := r.Player.AttackSequence
	if w.sourceTimeline() {
		// direction 保存由“facing”取得的朝向。
		direction := facing(r.Player.Pos, target.Pos, r.Player.Direction)
		w.attackSlotFallback(r, direction, 0, now, target)
	} else {
		w.attack(r, &r.Player, target, now)
	}
	if sequence == r.Player.AttackSequence {
		return false
	}
	r.lockAutoTarget(ref)
	return true
}

// automaticHealingTarget 返回自动治疗目标供调用方使用。
// w 是调用该方法的房间执行器实例，r 指向当前执行器拥有并更新的目标房间，pet 记录宠物，使用实体表示，rangeCells 记录范围地图格集合，使用数值表示。
func (w *worker) automaticHealingTarget(r *room, pet *entity, rangeCells int) *entity {
	if r.dungeonCombat != nil {
		for _, ally := range r.dungeonAllies() {
			if needsHealing(ally.body) && distance(r.Player.Pos, ally.body.Pos) <= rangeCells {
				return ally.body
			}
		}
		return nil
	}
	// candidate 接收遍历实体列表时当前的元素。
	for _, candidate := range []*entity{&r.Player, pet} {
		if needsHealing(candidate) && distance(r.Player.Pos, candidate.Pos) <= rangeCells {
			return candidate
		}
	}
	return nil
}

// needsHealing 排除死亡、满血及已有治疗队列的身体。
func needsHealing(body *entity) bool {
	return body != nil && body.HP > 0 && body.HP < body.MaxHP && body.HealAmount == 0
}
