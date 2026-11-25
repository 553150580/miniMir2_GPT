// 本文件校验方向、地形、行动身份和时钟，提交行走与受推位置并产生位移观察。
package game

import (
	"math"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	p "minimir/internal/protocol"
	"time"
)

// 协议类型是固定枚举，具体距离、时钟与显示节拍来自唯一配置。
const (
	// movementInitial 规定移动初始值的初始取值。
	movementInitial uint32 = 0
	// movementWalk 规定移动行走的初始取值。
	movementWalk uint32 = 1
	// movementPush 规定移动推入的初始取值。
	movementPush uint32 = 2
	// movementDash 规定移动冲锋的初始取值。
	movementDash uint32 = 3
	// movementTeleport 规定移动瞬移的初始取值。
	movementTeleport uint32 = 4
	// movementDashFailed 规定移动冲锋已失败的初始取值。
	movementDashFailed uint32 = 5
)

const (
	// navigationNone 规定导航“none”的初始取值。
	navigationNone uint32 = 0
	// navigationActive 规定导航“active”的初始取值。
	navigationActive uint32 = 1
	// navigationArrived 规定导航“arrived”的初始取值。
	navigationArrived uint32 = 2
	// navigationManual 规定导航手动输入的初始取值。
	navigationManual uint32 = 3
	// navigationCancelled 规定导航已经取消的初始取值。
	navigationCancelled uint32 = 4
)

// navigationState 汇总当前附着导航原目标、输入与可观察修订。
type navigationState struct {
	Revision      uint64           // 导航状态变化的修订号，客户端只接受较新的状态。
	InputSequence uint32           // 引起当前导航状态的原输入序号。
	Destination   *devconfig.Point // 有地面目标时保留原逻辑坐标。
	Status        uint32           // 空闲、导航中、到达、手动或取消的一种协议状态。
}

// Proto 将已确认的导航状态转换给客户端。
// n 是调用该方法的导航状态实例。
func (n navigationState) Proto() *pb.NavigationState {
	// out 保存当前处理所用的输出。
	out := &pb.NavigationState{InputSequence: n.InputSequence, Status: n.Status, Revision: n.Revision}
	if n.Destination != nil {
		out.Destination = pointPB(*n.Destination)
	}
	return out
}

// beginMovement 在身份仍可用时先分配位移修订号，再允许位置写入。
// w 是调用该方法的房间执行器实例，r 指向当前执行器拥有并更新的目标房间，e 记录实体，kind 记录类型，使用数值表示。
func (w *worker) beginMovement(r *room, e *entity, kind uint32) bool {
	if r.Isolated {
		return false
	}
	if e.MovementRevision == math.MaxUint64 {
		w.isolateRoom(r)
		return false
	}
	e.MovementRevision++
	e.MovementKind = kind
	return true
}

// commitWalk 按原格和目标格校验行走，更新时钟并在必要时提交安全区绑定。
// w 是调用该方法的房间执行器实例，r 指向当前执行器拥有并更新的目标房间，e 记录实体，point 记录点位，direction 记录朝向，使用数值表示。
// now 提供当前时点，作为状态有效期和计时判断的基准，interval 记录周期，使用数值表示。
func (w *worker) commitWalk(r *room, e *entity, point devconfig.Point, direction uint32, now time.Time, interval int64) bool {
	if r.Pending || r.petAway(e) || !w.walkable(r, point) {
		return false
	}
	// binding 保存由准备安全区域行走取得的会话绑定，allowed 保存由准备安全区域行走取得的允许。
	binding, allowed := w.prepareSafeZoneWalk(r, e, point)
	if !allowed {
		return false
	}
	if r.Isolated || e.MovementRevision == math.MaxUint64 {
		if !r.Isolated {
			w.isolateRoom(r)
		}
		return false
	}
	if !w.relocate(r, e, point) {
		return false
	}
	// 共享城镇的占格可能被另一分片先提交，只有真正落格的行走才推进修订。
	w.beginMovement(r, e, movementWalk)
	breakHidingOnWalk(e)
	e.Direction = direction
	e.Action, e.ActionUntil = 1, now.Add(200*time.Millisecond)
	e.NextMove = now.Add(time.Duration(interval) * time.Millisecond)
	if e.Kind == 1 {
		e.ActionReadyAt = e.NextMove
	} else if e.NextAttack.Before(e.NextMove) {
		// Crystal Monster.Walk:2128–2131：普通行走推进AttackTime；推挤的移动节流不承担攻击门禁。
		e.NextAttack = e.NextMove
	}
	if binding != nil {
		w.submitSafeZoneBinding(r, binding, now)
	}
	// 原怪物行走在真正入格后立即触发该格的火墙。
	w.enterFireWall(r, e, now)
	return true
}

// displacementSource 关联本次位移与造成位移的技能和施法。
type displacementSource struct {
	SkillID uint32 // 引起强制位移的技能编号。
	CastID  uint64 // 本次强制位移应关联的原施法序号。
}

// pushStop 表示受推时停止在某个格之前或操作失败的原因。
type pushStop uint8

const (
	// pushCompleted 保存当前处理所用的推入“completed”。
	pushCompleted pushStop = iota
	// pushTerrain 记录推入地形。
	pushTerrain
	// pushOccupied 记录推入已占用。
	pushOccupied
	// pushImmune 记录推入“immune”。
	pushImmune
	// pushStale 记录推入已失效。
	pushStale
	// pushUnavailable 记录推入不可用。
	pushUnavailable
	// pushInvalid 记录推入无效。
	pushInvalid
)

// pushOutcome 记录真实推进距离与是否执行来源规则的收尾。
type pushOutcome struct {
	Steps     int      // 目标最终被推进的实际合法格数。
	Stop      pushStop // 本次推进结束或被拒绝的终止原因。
	Postamble bool     // 是否执行了来源规则对应的受推收尾动作。
}

// sourcePushed只处理当前ActorRef的源推挤原语，不开放任何新技能入口或链式推挡路对象。
// w 是调用该方法的房间执行器实例，r 指向当前执行器拥有并更新的目标房间，target 记录目标，使用行动实体引用表示，direction 记录朝向，使用数值表示。
// distance 记录距离，使用数值表示，now 提供当前时点，作为状态有效期和计时判断的基准，source 记录来源，使用位移来源表示。
// out 记录输出，使用推入“outcome”表示。
func (w *worker) sourcePushed(r *room, target actorRef, direction uint32, distance int, now time.Time, source displacementSource) (out pushOutcome) {
	if r.Scene == 0 || r.Pending || r.Isolated {
		out.Stop = pushUnavailable
		return
	}
	// e 接收“resolve”供后续步骤使用。
	e := w.resolve(r, target)
	if e == nil || target.World != r.currentWorld().Ref {
		out.Stop = pushStale
		return
	}
	if direction >= uint32(len(directions)) || distance < 0 || e.Kind < 1 || e.Kind > 3 || !w.inBounds(r, e.Pos) || w.mapCellBudget(r) == 0 || e.Kind == 3 && e.Monster == nil {
		out.Stop = pushInvalid
		return
	}
	if e.Kind != 1 && !w.fighter(r, e).CanPush {
		out.Stop = pushImmune
		return
	}
	// timing 保存战斗时序，用于当前步骤中的时序。
	timing := w.e.Store.Cfg.Fixture.CombatTiming
	if timing.PlayerPushActionMS <= 0 || timing.MonsterPushActionMSPerStep <= 0 || timing.MonsterPushMoveMSPerStep <= 0 || timing.PushedStepDisplayMS <= 0 {
		out.Stop = pushInvalid
		return
	}
	// origin 保存位置，用于当前步骤中的大区或起点。
	origin := e.Pos
	// path 初始化用作路径的点位列表。
	path := []devconfig.Point{}
	defer func() {
		if len(path) == 0 {
			return
		}
		// 成功前缀已经改变现场，即使随后遇无效地格提前返回，也必须观察到真实路径。
		if e.HP > 0 {
			e.Action, e.ActionUntil = 0, now
		} // 尸体保持既有死亡姿态，不因位移伪装成站立。
		arena := r.currentWorld().mirrorCombat
		if arena != nil || r.Conn != nil && r.Ready && !r.Isolated {
			// event 保存当前处理所用的事件。
			event := &pb.EntityDisplacement{SceneEpoch: r.Scene, EntityHandle: e.Handle, EntityLife: e.Life, MovementRevision: e.MovementRevision, Kind: movementPush, Origin: pointPB(origin), FacingDirection: e.Direction, MovementDirection: direction, StepDurationMs: uint32(timing.PushedStepDisplayMS), SourceSkillId: source.SkillID, CastId: source.CastID, AttackSequence: e.AttackSequence, CastSequence: e.CastSequence}
			// point 接收遍历路径时当前的元素。
			for _, point := range path {
				event.Path = append(event.Path, pointPB(point))
			}
			if arena != nil {
				arena.addObservation(p.MsgS2CEntityDisplacement, event)
			} else if r.dungeonCombat != nil {
				w.sendDungeonObservation(r, p.MsgS2CEntityDisplacement, event)
			} else {
				w.send(r, p.MsgS2CEntityDisplacement, 0, event)
			}
		}
		if out.Postamble {
			// 原受推完成并广播后才检查最终格，途中经过的火墙不触发。
			w.enterFireWall(r, e, now)
		}
		w.snapshot(r)
	}()
	// step 规定步骤的初始取值。
	for step := 0; step < distance; step++ {
		// point 保存由步骤点位取得的点位。
		point := stepPoint(e.Pos, direction, 1)
		if !w.walkable(r, point) {
			out.Stop = pushTerrain
			return
		} // 源early return跳过所有后置时钟赋值。
		if !w.free(r, point, e) {
			out.Stop = pushOccupied
			break
		}
		if len(path) == 0 && !w.beginMovement(r, e, movementPush) {
			out.Stop = pushUnavailable
			return
		}
		if !w.relocate(r, e, point) {
			out.Stop = pushUnavailable
			return
		}
		e.Direction = (direction + uint32(len(directions))/2) % uint32(len(directions))
		path = append(path, point)
		out.Steps++
	}
	// 源赋值而非max：动态占格break即使0步也收尾；无效格/免疫/旧引用则完全不触碰时钟。
	if e.Kind == 1 {
		e.ActionReadyAt = now.Add(time.Duration(timing.PlayerPushActionMS) * time.Millisecond)
		// Human没有独立MoveTime；本项目NextMove曾映射同一walk ActionTime，须同步新截止。
		e.NextMove = e.ActionReadyAt
	} else {
		e.ActionReadyAt = now.Add(time.Duration(timing.MonsterPushActionMSPerStep) * time.Millisecond * time.Duration(out.Steps))
		e.NextMove = now.Add(time.Duration(timing.MonsterPushMoveMSPerStep) * time.Millisecond * time.Duration(out.Steps))
	}
	out.Postamble = true
	return
}
