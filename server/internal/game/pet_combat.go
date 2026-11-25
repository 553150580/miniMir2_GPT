// 本文件处理职业宠物普攻，神兽的两格直线攻击走公共命中时间线。
package game

import (
	"math"
	"minimir/internal/devconfig"
	"time"
)

const (
	// shinsuLineCells 是神兽按八方向扫描的来源攻击格数。
	shinsuLineCells = 2
	// shinsuAttackAction 是神兽一次喷焰后占用的来源动作时长。
	shinsuAttackAction = 300 * time.Millisecond
	// shinsuHitDelay 是喷焰公共起效延迟。
	shinsuHitDelay = 500 * time.Millisecond
	// shinsuHitPerCell 是沿直线每多一格的来源命中延迟。
	shinsuHitPerCell = 50 * time.Millisecond
)

// tryPetAttack 尝试对当前目标攻击。目标在攻击范围内时返回真，即使宠物正处于动作或攻击冷却。
func (w *worker) tryPetAttack(r *room, pet, target *entity, now time.Time) bool {
	if pet.PetRuntime != nil && pet.PetRuntime.Definition.SourceSkillID == devconfig.SummonShinsuSkillID {
		direction, reachable := w.shinsuAttackDirection(r, pet, target)
		if reachable {
			w.attackShinsu(r, pet, direction, now)
		}
		return reachable
	}
	if distance(pet.Pos, target.Pos) > 1 {
		return false
	}
	w.attack(r, pet, target, now)
	return true
}

// shinsuAttackDirection 核对目标在八方向之一的两个格子内，且攻击线没有被地图阻挡。
func (w *worker) shinsuAttackDirection(r *room, pet, target *entity) (uint32, bool) {
	cells := distance(pet.Pos, target.Pos)
	if cells == 0 || cells > shinsuLineCells {
		return 0, false
	}
	direction := facing(pet.Pos, target.Pos, pet.Direction)
	return direction, stepPoint(pet.Pos, direction, cells) == target.Pos && w.lineOfSight(r, pet.Pos, target.Pos)
}

// attackShinsu 一次喷焰只抽一次基础攻击力。每格按稳定实体顺序冻结一个敌人并校验各自防御。
func (w *worker) attackShinsu(r *room, pet *entity, direction uint32, now time.Time) {
	if r.Pending || r.Isolated || r.worldPet(r.currentWorld()) != pet || pet.HP <= 0 || pet.AttackSequence == math.MaxUint32 ||
		now.Before(pet.NextAttack) || now.Before(pet.ActionReadyAt) || !w.killBudgetAvailable(r) {
		return
	}
	world := r.currentWorld()
	var targets [shinsuLineCells]*entity
	needed := 0
	for i := range targets {
		point := stepPoint(pet.Pos, direction, i+1)
		if !w.worldWalkable(world, point) {
			break
		}
		targets[i] = w.enemyAtInWorld(world, point)
		if targets[i] != nil {
			needed++
		}
	}
	if needed == 0 || r.eventCount()+needed > w.eventCapacity() || uint64(needed) > math.MaxUint64-r.ScheduleID {
		return
	}
	stats := w.fighter(r, pet)
	amount := w.interval(stats.AttackMin, stats.AttackMax)
	pet.AttackSequence++
	pet.NextAttack = now.Add(time.Duration(stats.AttackMS) * time.Millisecond)
	pet.ActionReadyAt = now.Add(shinsuAttackAction)
	pet.Action, pet.ActionUntil, pet.Direction = 2, pet.ActionReadyAt, direction
	for i, target := range targets {
		if target == nil {
			continue
		}
		r.ScheduleID++
		event := pendingAttack{
			Scene: r.Scene, Caster: referenceIn(world, pet), Target: referenceIn(world, target),
			Due: now.Add(shinsuHitDelay + time.Duration(i+1)*shinsuHitPerCell), Amount: amount,
			Defence: defenceACAgility, Sequence: pet.AttackSequence, Order: r.ScheduleID,
			PrivilegeUntil: r.Role.PrivilegeUntil, PrivilegeCaptured: true,
		}
		if r.Conn == nil {
			event.HostSession = r.HostSession
		}
		world.Attacks = append(world.Attacks, event)
	}
}
