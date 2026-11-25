// 本文件沿用合法目标和耗材规则，受理队列选中的自动或手动魔法技能。
package game

import (
	"time"

	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
)

// autoSkill 先用就绪技能队列，没能释放且确实缺蓝时沿用原自动用药。
func (w *worker) autoSkill(r *room, now time.Time) bool {
	accepted, need := w.tryAttackSkillSlots(r, r.liveAutoTarget(), r.Player.Direction, 0, now, false)
	return accepted || r.Auto && w.tryManaPotion(r, need, now)
}

// castQueuedAttackSkill 复用自动技能的治疗、状态及攻击形状，原法术入口仍使用同一套结算。
func (w *worker) castQueuedAttackSkill(r *room, skill devconfig.Skill, focus *entity, attackDirection, input uint32, now time.Time, manual bool, need *automaticManaNeed) bool {
	handler, registered := skillHandlers[skill.Behavior]
	if !registered || handler.Passive || handler.Displacement || handler.Teleport || now.Before(r.NextSpell) {
		return false
	}
	if handler.Summon {
		// 队列只补空缺的活宠位置，召回和主动换宠仍由独立施法入口处理。
		m, known := w.currentMap(r)
		return skill.AttackSlotSummon() && r.Pet == nil && r.bossBattle == nil && r.worldBossObservation == nil && r.worldBossCombat == nil &&
			known && m.SceneKindValue() == devconfig.SceneKindPersonal && w.autoMaterialCast(r, skill, 0, attackDirection, nil, now, need, input)
	}
	direction := r.Player.Direction
	pet := r.worldPet(r.currentWorld())
	if handler.Status {
		missing := func(e *entity) bool { return e != nil && e.HP > 0 && e.Statuses[skill.BuffID] == nil }
		center := r.Player.Pos
		if skill.TargetKind == "friendly_self" {
			if !missing(&r.Player) {
				return false
			}
		} else {
			if !missing(&r.Player) {
				if !missing(pet) {
					return false
				}
				center = pet.Pos
			}
			count := 0
			for _, target := range []*entity{&r.Player, pet} {
				if missing(target) && distance(target.Pos, center) <= skill.AreaRadius && w.walkable(r, target.Pos) {
					count++
				}
			}
			if count < skill.AutoMinTargets {
				return false
			}
		}
		if skill.MaterialQuantity > 0 {
			return w.autoMaterialCast(r, skill, 0, direction, pointPB(center), now, need, input)
		}
		return w.castAutomatically(r, skill.ID, 0, direction, now, need, pointPB(center), input)
	}
	if handler.Healing {
		target := w.automaticHealingTarget(r, pet, skill.RangeCells)
		if target == nil {
			return false
		}
		var grid *pb.GridPoint
		handle := target.Handle
		if handler.Area {
			count := 0
			for _, ally := range w.areaTargets(r, skill, target.Pos) {
				if ally.HP < ally.MaxHP && ally.HealAmount == 0 {
					count++
				}
			}
			if count < skill.AutoMinTargets {
				return false
			}
			grid, handle = pointPB(target.Pos), 0
		}
		return w.castAutomatically(r, skill.ID, handle, direction, now, need, grid, input)
	}
	return (!manual || focus != nil) && w.autoOffensiveCast(r, skill, handler, focus, now, need, input, !manual)
}
