package game

import (
	"math"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	p "minimir/internal/protocol"
	"sort"
	"time"
)

// 记录实际入格顺序。最多十个活实体，按格读当前列表可保留源扫描中删除/追加的语义。
func (w *worker) relocate(r *room, e *entity, point devconfig.Point) bool {
	if !w.walkable(r, point) {
		return false
	}
	if r.CellEntrySequence == math.MaxUint64 {
		w.isolateRoom(r)
		return false
	}
	r.CellEntrySequence++
	e.Pos, e.CellEntry = point, r.CellEntrySequence
	return true
}

func cellObjects(r *room, point devconfig.Point) []*entity {
	all := []*entity{&r.Player, r.Pet}
	all = append(all, r.Monsters[:r.MonsterCount]...)
	result := []*entity{}
	for _, e := range all {
		if e != nil && e.Pos == point {
			result = append(result, e)
		}
	}
	sort.SliceStable(result, func(i, j int) bool { return result[i].CellEntry < result[j].CellEntry })
	return result
}

func (w *worker) displacementEnemy(r *room, e *entity) bool {
	return e != nil && e.Kind == 3 && e.HP > 0 && w.fighter(r, e).Level < r.Role.Level
}

func (w *worker) resolveDisplacementSkill(r *room, cast pendingCast, now time.Time) {
	if r.Isolated || r.Pending || w.resolve(r, cast.Caster) != &r.Player || r.Player.HP <= 0 || cast.Skill.Displacement == nil || w.mapCellBudget(r) == 0 {
		return
	}
	// Pushed内部会尝试同步基线；整次同步施放完成前延后，防止基线先于冲撞事件确认水位。
	before := r.DeferMovementSnapshot
	r.DeferMovementSnapshot = true
	defer func() {
		r.DeferMovementSnapshot = before
		if !before {
			w.snapshot(r)
		}
	}()
	if cast.Skill.Behavior == "repulsion" {
		w.repulsion(r, cast, now)
	} else {
		w.shoulderDash(r, cast, now)
	}
}

func (w *worker) repulsion(r *room, cast pendingCast, now time.Time) {
	center := r.Player.Pos
	rules := cast.Skill.Displacement
	for d := 0; d <= cast.Skill.AreaRadius; d++ {
		for y := center.Y - d; y <= center.Y+d; y++ {
			for x := center.X - d; x <= center.X+d; x++ {
				if d > 0 && abs(y-center.Y) != d && abs(x-center.X) != d {
					continue
				}
				point := devconfig.Point{X: x, Y: y}
				if !w.walkable(r, point) {
					continue
				}
				for i := 0; !r.Isolated; i++ {
					// 每次重新读取实际列表；移动删除当前元素后，源i++会跳过随后同格对象。
					objects := cellObjects(r, point)
					if i >= len(objects) {
						break
					}
					target := objects[i]
					if !w.displacementEnemy(r, target) {
						continue
					}
					threshold := rules.SuccessBase + rules.SuccessPerLevel*int64(cast.SkillLevel) + int64(r.Role.Level) - int64(w.fighter(r, target).Level)
					if w.draw(rules.SuccessDenominator) >= threshold {
						continue
					}
					length := rules.DistanceBaseByLevel[cast.SkillLevel] + int(w.draw(rules.DistanceRandomBound))
					// 源DirectionFromPoint同格固定Up，不使用角色当前朝向。
					direction := facing(center, target.Pos, 0)
					w.sourcePushed(r, reference(r, target), direction, length, now, displacementSource{SkillID: cast.Skill.ID, CastID: cast.ID})
				}
			}
		}
	}
}

func (w *worker) shoulderDash(r *room, cast pendingCast, now time.Time) {
	mapTemplate, known := w.currentMap(r)
	if !known {
		return
	}
	r.Player.ActionReadyAt = now.Add(time.Duration(cast.Skill.ActionMS) * time.Millisecond)
	origin := r.Player.Pos
	rules := cast.Skill.Displacement
	length := rules.DistanceBaseByLevel[cast.SkillLevel] + int(w.draw(rules.DistanceRandomBound))
	var target *entity
	path := []devconfig.Point{}
	for i := 0; i < length && !r.Isolated; i++ {
		next := stepPoint(r.Player.Pos, cast.Direction, 1)
		if !w.walkable(r, next) {
			break
		}
		// Crystal HumanObject.ShoulderDash:5002只检查下一格安全区；不禁止普通walk/pushed。
		if _, inside := mapTemplate.SafeZoneAt(next); inside {
			break
		}
		if i == 0 {
			blocked := false
			for _, ob := range cellObjects(r, next) {
				if w.displacementEnemy(r, ob) {
					target = ob
					break
				}
				if ob.HP > 0 {
					blocked = true
					break
				}
			}
			if blocked {
				break
			}
		}
		if target == nil {
			if !w.free(r, next, &r.Player) {
				break
			}
		} else if w.sourcePushed(r, reference(r, target), cast.Direction, 1, now, displacementSource{SkillID: cast.Skill.ID, CastID: cast.ID}).Steps == 0 {
			break
		}
		if len(path) == 0 && !w.beginMovement(r, &r.Player, movementDash) {
			return
		}
		// 源已选中首目标后不再复查同格其它Blocking，保留叠放前态下的这一差异。
		if !w.relocate(r, &r.Player, next) {
			return
		}
		path = append(path, next)
	}
	if r.Isolated {
		return
	}
	kind := movementDash
	if len(path) == 0 {
		kind = movementDashFailed
		if !w.beginMovement(r, &r.Player, kind) {
			return
		}
	}
	r.Player.Action, r.Player.ActionUntil = 0, now
	event := &pb.EntityDisplacement{SceneEpoch: r.Scene, EntityHandle: r.Player.Handle, EntityLife: r.Player.Life,
		MovementRevision: r.Player.MovementRevision, Kind: kind, Origin: pointPB(origin), FacingDirection: cast.Direction,
		MovementDirection: cast.Direction, StepDurationMs: uint32(rules.StepDisplayMS), SourceSkillId: cast.Skill.ID, CastId: cast.ID,
		AttackSequence: r.Player.AttackSequence, CastSequence: r.Player.CastSequence}
	for _, point := range path {
		event.Path = append(event.Path, pointPB(point))
	}
	w.send(r, p.MsgS2CEntityDisplacement, 0, event)
	if len(path) > 0 && target != nil {
		before := target.HP
		w.applyDamage(r, &r.Player, target, cast.Amount, now, hitContext{At: now, HostSession: cast.HostSession, PrivilegeUntil: cast.PrivilegeUntil, PrivilegeCaptured: cast.PrivilegeCaptured})
		outcome := uint32(2)
		if target.HP < before {
			outcome = 1
		}
		w.impact(r, cast.Skill.ID, r.Player.Handle, target.Handle, target.Life, target.Pos, before-target.HP, outcome, false, cast.ID, 0)
	}
}
