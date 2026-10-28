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
	movementInitial    uint32 = 0
	movementWalk       uint32 = 1
	movementPush       uint32 = 2
	movementDash       uint32 = 3
	movementTeleport   uint32 = 4
	movementDashFailed uint32 = 5
)

const (
	navigationNone      uint32 = 0
	navigationActive    uint32 = 1
	navigationArrived   uint32 = 2
	navigationManual    uint32 = 3
	navigationCancelled uint32 = 4
)

type navigationState struct {
	Revision      uint64
	InputSequence uint32
	Destination   *devconfig.Point
	Status        uint32
}

func (n navigationState) Proto() *pb.NavigationState {
	out := &pb.NavigationState{InputSequence: n.InputSequence, Status: n.Status, Revision: n.Revision}
	if n.Destination != nil {
		out.Destination = pointPB(*n.Destination)
	}
	return out
}

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

func (w *worker) commitWalk(r *room, e *entity, point devconfig.Point, direction uint32, now time.Time, interval int64) bool {
	if r.Pending || !w.walkable(r, point) {
		return false
	}
	binding, allowed := w.prepareSafeZoneWalk(r, e, point)
	if !allowed {
		return false
	}
	if !w.beginMovement(r, e, movementWalk) {
		return false
	}
	if !w.relocate(r, e, point) {
		return false
	}
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
	return true
}

type displacementSource struct {
	SkillID uint32
	CastID  uint64
}
type pushStop uint8

const (
	pushCompleted pushStop = iota
	pushTerrain
	pushOccupied
	pushImmune
	pushStale
	pushUnavailable
	pushInvalid
)

type pushOutcome struct {
	Steps     int
	Stop      pushStop
	Postamble bool
}

// sourcePushed只处理当前ActorRef的源推挤原语，不开放任何新技能入口或链式推挡路对象。
func (w *worker) sourcePushed(r *room, target actorRef, direction uint32, distance int, now time.Time, source displacementSource) (out pushOutcome) {
	if r.Scene == 0 || r.Pending || r.Isolated {
		out.Stop = pushUnavailable
		return
	}
	e := w.resolve(r, target)
	if e == nil {
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
	timing := w.e.Store.Cfg.Fixture.CombatTiming
	if timing.PlayerPushActionMS <= 0 || timing.MonsterPushActionMSPerStep <= 0 || timing.MonsterPushMoveMSPerStep <= 0 || timing.PushedStepDisplayMS <= 0 {
		out.Stop = pushInvalid
		return
	}
	origin := e.Pos
	path := []devconfig.Point{}
	defer func() {
		if len(path) == 0 {
			return
		}
		// 成功前缀已经改变现场，即使随后遇无效地格提前返回，也必须观察到真实路径。
		if e.HP > 0 {
			e.Action, e.ActionUntil = 0, now
		} // 尸体保持既有死亡姿态，不因位移伪装成站立。
		if r.Conn != nil && r.Ready && !r.Isolated {
			event := &pb.EntityDisplacement{SceneEpoch: r.Scene, EntityHandle: e.Handle, EntityLife: e.Life, MovementRevision: e.MovementRevision, Kind: movementPush, Origin: pointPB(origin), FacingDirection: e.Direction, MovementDirection: direction, StepDurationMs: uint32(timing.PushedStepDisplayMS), SourceSkillId: source.SkillID, CastId: source.CastID, AttackSequence: e.AttackSequence, CastSequence: e.CastSequence}
			for _, point := range path {
				event.Path = append(event.Path, pointPB(point))
			}
			w.send(r, p.MsgS2CEntityDisplacement, 0, event)
		}
		w.snapshot(r)
	}()
	for step := 0; step < distance; step++ {
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
