// 本文件按锁定目标和技能形状选择自动施法方向、地格或直接目标，合法时受理释放。
package game

import (
	"slices"
	"time"

	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
)

// automaticTargetIn 返回自动目标输入供调用方使用。
// r 指向当前执行器拥有并更新的目标房间，targets 记录目标集合，使用实体列表表示，focus 记录“focus”，使用实体表示。
func automaticTargetIn(r *room, targets []*entity, focus *entity) *entity {
	if focus != nil {
		if slices.Contains(targets, focus) {
			return focus
		}
		return nil
	}
	// nearest 记录最近，使用实体表示。
	var nearest *entity
	// target 接收遍历目标集合时当前的元素。
	for _, target := range targets {
		if nearest == nil || distance(r.Player.Pos, target.Pos) < distance(r.Player.Pos, nearest.Pos) {
			nearest = target
		}
	}
	return nearest
}

// autoLineAim 处理自动线路“aim”。
// w 是调用该方法的房间执行器实例，r 指向当前执行器拥有并更新的目标房间，skill 记录技能，focus 记录“focus”，使用实体表示。
func (w *worker) autoLineAim(r *room, skill devconfig.Skill, focus *entity) (*entity, uint32) {
	// aim 记录“aim”，使用实体表示。
	var aim *entity
	// direction 记录朝向，使用数值表示。
	var direction uint32
	// best 规定“best”的初始取值。
	best := 0
	// index 标记遍历朝向集合时访问的下标。
	for index := range directions {
		// ray 保存由线路目标集合取得的“ray”。
		ray := w.lineTargets(r, pendingCast{Skill: skill, TargetPoint: r.Player.Pos, Direction: uint32(index)})
		if len(ray) < skill.AutoMinTargets || len(ray) <= best {
			continue
		}
		// target 保存由自动目标输入取得的目标。
		if target := automaticTargetIn(r, ray, focus); target != nil {
			aim, direction, best = target, uint32(index), len(ray)
		}
	}
	return aim, direction
}

// autoAreaAim 处理自动区域“aim”。
// w 是调用该方法的房间执行器实例，r 指向当前执行器拥有并更新的目标房间，skill 记录技能，focus 记录“focus”，使用实体表示。
func (w *worker) autoAreaAim(r *room, skill devconfig.Skill, focus *entity) (*entity, *pb.GridPoint) {
	if focus != nil && w.canAimEnemy(r, skill, focus) {
		// targets 保存由区域目标集合取得的目标集合。
		targets := w.areaTargets(r, skill, focus.Pos)
		if len(targets) >= skill.AutoMinTargets && slices.Contains(targets, focus) {
			return focus, pointPB(focus.Pos)
		}
	}
	// target 记录目标，使用实体表示，center 记录中心格，使用实体表示。
	var target, center *entity
	// candidate 接收遍历怪物集合中的范围时当前的元素。
	for _, candidate := range r.Monsters[:r.MonsterCount] {
		if !w.canAimEnemy(r, skill, candidate) {
			continue
		}
		// targets 保存由区域目标集合取得的目标集合。
		targets := w.areaTargets(r, skill, candidate.Pos)
		if len(targets) < skill.AutoMinTargets {
			continue
		}
		// required 保存当前处理所用的“required”。
		required := focus
		if required == nil {
			required = candidate
		}
		if slices.Contains(targets, required) && (center == nil || distance(r.Player.Pos, candidate.Pos) < distance(r.Player.Pos, center.Pos)) {
			target, center = required, candidate
		}
	}
	if center == nil {
		return nil, nil
	}
	return target, pointPB(center.Pos)
}

// canAimEnemy 判断能否执行“aim”敌人。
// w 是调用该方法的房间执行器实例，r 指向当前执行器拥有并更新的目标房间，skill 记录技能，enemy 记录敌人，使用实体表示。
func (w *worker) canAimEnemy(r *room, skill devconfig.Skill, enemy *entity) bool {
	return enemy != nil && enemy.HP > 0 && distance(r.Player.Pos, enemy.Pos) <= skill.RangeCells &&
		(!skillRequiresFlight(skill) || w.lineOfSight(r, r.Player.Pos, enemy.Pos))
}

// autoDirectAim 返回自动“direct”“aim”供调用方使用。
// w 是调用该方法的房间执行器实例，r 指向当前执行器拥有并更新的目标房间，skill 记录技能，focus 记录“focus”，使用实体表示。
func (w *worker) autoDirectAim(r *room, skill devconfig.Skill, focus *entity) *entity {
	if focus != nil {
		if w.canAimEnemy(r, skill, focus) {
			return focus
		}
		return nil
	}
	// nearest 记录最近，使用实体表示。
	var nearest *entity
	// candidate 接收遍历怪物集合中的范围时当前的元素。
	for _, candidate := range r.Monsters[:r.MonsterCount] {
		if w.canAimEnemy(r, skill, candidate) && (nearest == nil || distance(r.Player.Pos, candidate.Pos) < distance(r.Player.Pos, nearest.Pos)) {
			nearest = candidate
		}
	}
	return nearest
}

// 选心和朝向只控制本次施法。已释放的直线与范围依旧依其能力结算附带目标。
// w 是调用该方法的房间执行器实例，r 指向当前执行器拥有并更新的目标房间，skill 记录技能，handler 记录处理器，使用技能处理器表示，focus 记录“focus”，使用实体表示。
// now 提供当前时点，作为状态有效期和计时判断的基准，need 记录所需量，使用自动魔法值所需量表示。
func (w *worker) autoOffensiveCast(r *room, skill devconfig.Skill, handler skillHandler, focus *entity, now time.Time, need *automaticManaNeed, input uint32, lock bool) bool {
	// target 记录目标，使用实体表示。
	var target *entity
	// grid 记录网格，使用网格点位表示。
	var grid *pb.GridPoint
	// handle 记录实体句柄，使用数值表示。
	var handle uint32
	// direction 保存当前处理所用的朝向。
	direction := r.Player.Direction
	switch {
	case handler.SelfArea:
		// targets 保存由区域目标集合取得的目标集合。
		targets := w.areaTargets(r, skill, r.Player.Pos)
		if len(targets) >= skill.AutoMinTargets {
			target = automaticTargetIn(r, targets, focus)
		}
	case handler.Direction:
		target, direction = w.autoLineAim(r, skill, focus)
	case handler.Area:
		target, grid = w.autoAreaAim(r, skill, focus)
	default:
		target = w.autoDirectAim(r, skill, focus)
		if target != nil {
			handle = target.Handle
		}
	}
	if target == nil {
		return false
	}
	// ref 接收引用供后续步骤使用。
	ref := reference(r, target)
	// accepted 表示本次动作已受理，耗材技能先提交永久扣料任务。
	var accepted bool
	if skill.MaterialQuantity > 0 {
		accepted = w.autoMaterialCast(r, skill, handle, direction, grid, now, need, input)
	} else {
		accepted = w.castAutomatically(r, skill.ID, handle, direction, now, need, grid, input)
	}
	if !accepted {
		return false
	}
	if lock {
		r.lockAutoTarget(ref)
	}
	return true
}
