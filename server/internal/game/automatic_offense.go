package game

import (
	"slices"
	"time"

	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
)

func automaticTargetIn(r *room, targets []*entity, focus *entity) *entity {
	if focus != nil {
		if slices.Contains(targets, focus) {
			return focus
		}
		return nil
	}
	var nearest *entity
	for _, target := range targets {
		if nearest == nil || distance(r.Player.Pos, target.Pos) < distance(r.Player.Pos, nearest.Pos) {
			nearest = target
		}
	}
	return nearest
}

func (w *worker) autoLineAim(r *room, skill devconfig.Skill, focus *entity) (*entity, uint32) {
	var aim *entity
	var direction uint32
	best := 0
	for index := range directions {
		ray := w.lineTargets(r, pendingCast{Skill: skill, TargetPoint: r.Player.Pos, Direction: uint32(index)})
		if len(ray) < skill.AutoMinTargets || len(ray) <= best {
			continue
		}
		if target := automaticTargetIn(r, ray, focus); target != nil {
			aim, direction, best = target, uint32(index), len(ray)
		}
	}
	return aim, direction
}

func (w *worker) autoAreaAim(r *room, skill devconfig.Skill, focus *entity) (*entity, *pb.GridPoint) {
	if focus != nil && w.canAimEnemy(r, skill, focus) {
		targets := w.areaTargets(r, skill, focus.Pos)
		if len(targets) >= skill.AutoMinTargets && slices.Contains(targets, focus) {
			return focus, pointPB(focus.Pos)
		}
	}
	var target, center *entity
	for _, candidate := range r.Monsters[:r.MonsterCount] {
		if !w.canAimEnemy(r, skill, candidate) {
			continue
		}
		targets := w.areaTargets(r, skill, candidate.Pos)
		if len(targets) < skill.AutoMinTargets {
			continue
		}
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

func (w *worker) canAimEnemy(r *room, skill devconfig.Skill, enemy *entity) bool {
	return enemy != nil && enemy.HP > 0 && distance(r.Player.Pos, enemy.Pos) <= skill.RangeCells &&
		(!skillRequiresFlight(skill) || w.lineOfSight(r, r.Player.Pos, enemy.Pos))
}

func (w *worker) autoDirectAim(r *room, skill devconfig.Skill, focus *entity) *entity {
	if focus != nil {
		if w.canAimEnemy(r, skill, focus) {
			return focus
		}
		return nil
	}
	var nearest *entity
	for _, candidate := range r.Monsters[:r.MonsterCount] {
		if w.canAimEnemy(r, skill, candidate) && (nearest == nil || distance(r.Player.Pos, candidate.Pos) < distance(r.Player.Pos, nearest.Pos)) {
			nearest = candidate
		}
	}
	return nearest
}

// 选心和朝向只控制本次施法。已释放的直线与范围依旧依其能力结算附带目标。
func (w *worker) autoOffensiveCast(r *room, skill devconfig.Skill, handler skillHandler, focus *entity, now time.Time) bool {
	var target *entity
	var grid *pb.GridPoint
	var handle uint32
	direction := r.Player.Direction
	switch {
	case handler.SelfArea:
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
	ref := reference(r, target)
	if w.castDirected(r, skill.ID, handle, 0, direction, now, grid) != 0 {
		return false
	}
	r.lockAutoTarget(ref)
	return true
}
