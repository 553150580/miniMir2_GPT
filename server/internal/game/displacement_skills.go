// 本文件处理抗拒火环、野蛮冲撞等位移技能的推进顺序、方向、碰撞和安全区规则。
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
// w 是调用该方法的房间执行器实例，r 指向当前执行器拥有并更新的目标房间，e 记录实体，point 记录点位。
func (w *worker) relocate(r *room, e *entity, point devconfig.Point) bool {
	if r.petAway(e) || !w.walkable(r, point) {
		return false
	}
	return w.commitRelocation(r, e, point)
}

// 仅供已完成各自来源地形校验的受信动作使用，不把Teleport规则混进普通walk/pushed。
// w 是调用该方法的房间执行器实例，r 指向当前执行器拥有并更新的目标房间，e 记录实体，point 记录点位。
func (w *worker) commitRelocation(r *room, e *entity, point devconfig.Point) bool {
	if r.CellEntrySequence == math.MaxUint64 {
		w.isolateRoom(r)
		return false
	}
	if e == &r.Player && !w.moveSharedTownPlayer(r, point) {
		return false
	}
	r.CellEntrySequence++
	e.Pos, e.CellEntry = point, r.CellEntrySequence
	if e == r.Pet {
		r.recordPetPosition()
	}
	return true
}

// cellObjects 返回地图格对象集合供调用方使用。
// r 指向当前执行器拥有并更新的目标房间，point 记录点位。
func cellObjects(r *room, point devconfig.Point) []*entity {
	// all 初始化用作全部的实体列表。
	all := []*entity{&r.Player, r.worldPet(r.currentWorld())}
	// corpse 接收遍历宠物尸体集合时当前的元素。
	for _, corpse := range r.currentWorld().PetCorpses {
		all = append(all, corpse.Body)
	}
	all = append(all, r.Monsters[:r.MonsterCount]...)
	// result 初始化用作结果的实体列表。
	result := []*entity{}
	// e 接收遍历全部时当前的元素。
	for _, e := range all {
		if e != nil && e.Pos == point {
			result = append(result, e)
		}
	}
	// i 记录当前位置的下标，使用数值表示，j 记录当前位置的下标，使用数值表示。
	sort.SliceStable(result, func(i, j int) bool { return result[i].CellEntry < result[j].CellEntry })
	return result
}

// displacementEnemy 判断位移敌人是否满足要求。
// w 是调用该方法的房间执行器实例，r 指向当前执行器拥有并更新的目标房间，e 记录实体。
func (w *worker) displacementEnemy(r *room, e *entity) bool {
	return e != nil && e.Kind == 3 && e.HP > 0 && w.fighter(r, e).Level < r.Role.Level
}

// resolveDisplacementSkill 确定位移技能。
// w 是调用该方法的房间执行器实例，r 指向当前执行器拥有并更新的目标房间，cast 记录施法，使用待确认施法表示，now 提供当前时点，作为状态有效期和计时判断的基准。
func (w *worker) resolveDisplacementSkill(r *room, cast pendingCast, now time.Time) {
	if r.Isolated || r.Pending || w.resolve(r, cast.Caster) != &r.Player || r.Player.HP <= 0 || cast.Skill.Displacement == nil || w.mapCellBudget(r) == 0 {
		return
	}
	// Pushed内部会尝试同步基线；整次同步施放完成前延后，防止基线先于冲撞事件确认水位。
	// before 保存延迟移动快照，用于当前步骤中的操作前的状态。
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

// repulsion 处理“repulsion”。
// w 是调用该方法的房间执行器实例，r 指向当前执行器拥有并更新的目标房间，cast 记录施法，使用待确认施法表示，now 提供当前时点，作为状态有效期和计时判断的基准。
func (w *worker) repulsion(r *room, cast pendingCast, now time.Time) {
	// center 保存位置，用于当前步骤中的中心格。
	center := r.Player.Pos
	// rules 保存位移，用于当前步骤中的规则集合。
	rules := cast.Skill.Displacement
	// d 规定“repulsion”所用数据的初始取值。
	for d := 0; d <= cast.Skill.AreaRadius; d++ {
		// y 保存根据纵坐标与d计算的纵坐标与d的计算结果。
		for y := center.Y - d; y <= center.Y+d; y++ {
			// x 保存根据横坐标与d计算的横坐标与d的计算结果。
			for x := center.X - d; x <= center.X+d; x++ {
				if d > 0 && abs(y-center.Y) != d && abs(x-center.X) != d {
					continue
				}
				// point 组装点位所需的数据。
				point := devconfig.Point{X: x, Y: y}
				if !w.walkable(r, point) {
					continue
				}
				// i 标记循环或列表中的当前位置。
				for i := 0; !r.Isolated; i++ {
					// 每次重新读取实际列表；移动删除当前元素后，源i++会跳过随后同格对象。
					// objects 保存由地图格对象集合取得的对象集合。
					objects := cellObjects(r, point)
					if i >= len(objects) {
						break
					}
					// target 保存从对象集合取出的目标。
					target := objects[i]
					if !w.displacementEnemy(r, target) {
						continue
					}
					// threshold 保存根据成功基础值与成功“per”等级与整数“64”的计算结果的计算结果与整数“64”的计算结果与整数“64”计算的“threshold”。
					threshold := rules.SuccessBase + rules.SuccessPerLevel*int64(cast.SkillLevel) + int64(r.Role.Level) - int64(w.fighter(r, target).Level)
					if w.draw(rules.SuccessDenominator) >= threshold {
						continue
					}
					// length 保存根据距离基础值“by”等级中的指定项与整数计算的长度。
					length := rules.DistanceBaseByLevel[cast.SkillLevel] + int(w.draw(rules.DistanceRandomBound))
					// 源DirectionFromPoint同格固定Up，不使用角色当前朝向。
					// direction 保存由“facing”取得的朝向。
					direction := facing(center, target.Pos, 0)
					w.sourcePushed(r, reference(r, target), direction, length, now, displacementSource{SkillID: cast.Skill.ID, CastID: cast.ID})
				}
			}
		}
	}
}

// shoulderDash 处理“shoulder”冲锋。
// w 是调用该方法的房间执行器实例，r 指向当前执行器拥有并更新的目标房间，cast 记录施法，使用待确认施法表示，now 提供当前时点，作为状态有效期和计时判断的基准。
func (w *worker) shoulderDash(r *room, cast pendingCast, now time.Time) {
	// mapTemplate 保存由当前状态地图取得的地图模板，known 保存由当前状态地图取得的已知状态。
	mapTemplate, known := w.currentMap(r)
	if !known {
		return
	}
	r.Player.ActionReadyAt = now.Add(time.Duration(cast.Skill.ActionMS) * time.Millisecond)
	// origin 保存位置，用于当前步骤中的大区或起点。
	origin := r.Player.Pos
	// rules 保存位移，用于当前步骤中的规则集合。
	rules := cast.Skill.Displacement
	// length 保存根据距离基础值“by”等级中的指定项与整数计算的长度。
	length := rules.DistanceBaseByLevel[cast.SkillLevel] + int(w.draw(rules.DistanceRandomBound))
	// target 记录目标，使用实体表示。
	var target *entity
	// path 初始化用作路径的点位列表。
	path := []devconfig.Point{}
	// i 标记循环或列表中的当前位置。
	for i := 0; i < length && !r.Isolated; i++ {
		// next 保存由步骤点位取得的下一状态。
		next := stepPoint(r.Player.Pos, cast.Direction, 1)
		if !w.walkable(r, next) {
			break
		}
		// Crystal HumanObject.ShoulderDash:5002只检查下一格安全区；不禁止普通walk/pushed。
		// inside 保存由安全区域发生时间取得的“inside”。
		if _, inside := mapTemplate.SafeZoneAt(next); inside {
			break
		}
		if i == 0 {
			// blocked 保存当前处理所用的阻挡。
			blocked := false
			// ob 接收遍历地图格对象集合时当前的元素。
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
	// kind 保存当前处理所用的类型。
	kind := movementDash
	if len(path) == 0 {
		kind = movementDashFailed
		if !w.beginMovement(r, &r.Player, kind) {
			return
		}
	}
	r.Player.Action, r.Player.ActionUntil = 0, now
	// event 保存当前处理所用的事件。
	event := &pb.EntityDisplacement{SceneEpoch: r.Scene, EntityHandle: r.Player.Handle, EntityLife: r.Player.Life,
		MovementRevision: r.Player.MovementRevision, Kind: kind, Origin: pointPB(origin), FacingDirection: cast.Direction,
		MovementDirection: cast.Direction, StepDurationMs: uint32(rules.StepDisplayMS), SourceSkillId: cast.Skill.ID, CastId: cast.ID,
		AttackSequence: r.Player.AttackSequence, CastSequence: r.Player.CastSequence}
	// point 接收遍历路径时当前的元素。
	for _, point := range path {
		event.Path = append(event.Path, pointPB(point))
	}
	w.send(r, p.MsgS2CEntityDisplacement, 0, event)
	if len(path) > 0 && target != nil {
		// before 保存生命值，用于当前步骤中的操作前的状态。
		before := target.HP
		w.applyDamage(r, &r.Player, target, cast.Amount, now, hitContext{At: now, HostSession: cast.HostSession, PrivilegeUntil: cast.PrivilegeUntil, PrivilegeCaptured: cast.PrivilegeCaptured})
		// outcome 保存由无符号整数“32”取得的“outcome”。
		outcome := uint32(2)
		if target.HP < before {
			outcome = 1
		}
		w.impact(r, cast.Skill.ID, r.Player.Handle, target.Handle, target.Life, target.Pos, before-target.HP, outcome, false, cast.ID, 0)
	}
}
