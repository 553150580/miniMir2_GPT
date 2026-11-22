// 本文件以怪物句柄和生命代次保持自动战斗锁定，校验客户端选怪请求和失效边界。
package game

import (
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	p "minimir/internal/protocol"
	"minimir/internal/wire"

	"google.golang.org/protobuf/proto"
)

// activeMonster 返回“active”怪物供调用方使用。
// r 是调用该方法的房间实例，handle 记录实体句柄，使用数值表示，life 记录生命代次，使用数值表示。
func (r *room) activeMonster(handle uint32, life uint64) *entity {
	// monster 接收遍历怪物集合中的范围时当前的元素。
	for _, monster := range r.Monsters[:r.MonsterCount] {
		if monster != nil && monster.Kind == 3 && monster.HP > 0 && monster.Handle == handle && monster.Life == life {
			return monster
		}
	}
	return nil
}

// liveAutoTarget 返回存活自动目标供调用方使用。
// r 是调用该方法的房间实例。
func (r *room) liveAutoTarget() *entity {
	// ref 保存自动目标，用于当前步骤中的引用。
	ref := r.AutoTarget
	if ref.Handle == 0 {
		return nil
	}
	if ref.World == r.currentWorld().Ref {
		// target 保存由“active”怪物取得的目标。
		if target := r.activeMonster(ref.Handle, ref.Life); target != nil {
			return target
		}
	}
	r.AutoTarget = actorRef{}
	return nil
}

// 仅锁住仍属于当前现场的同一只活怪；攻击中的即时命中或世界移交可以让该引用先失效。
// r 是调用该方法的房间实例，ref 记录引用，使用行动实体引用表示。
func (r *room) lockAutoTarget(ref actorRef) {
	if r.Player.HP > 0 && ref.World == r.currentWorld().Ref && r.activeMonster(ref.Handle, ref.Life) != nil {
		r.AutoTarget = ref
	}
}

// autoTargetIntent 处理自动目标意图。
// w 是调用该方法的房间执行器实例，r 指向当前执行器拥有并更新的目标房间，frame 记录协议帧。
func (w *worker) autoTargetIntent(r *room, frame wire.Frame) {
	// q 记录自动目标意图。
	var q pb.AutoTargetIntent
	if proto.Unmarshal(frame.Payload, &q) != nil || q.SceneEpoch != r.Scene || q.TargetHandle == 0 {
		w.failure(r, frame, p.CodeInvalidArgument)
		return
	}
	if !acceptInput(r, q.InputSequence) {
		w.failure(r, frame, p.CodeStaleInput)
		return
	}
	// m 接收当前状态地图供后续步骤使用，known 保存由当前状态地图取得的已知状态。
	if m, known := w.currentMap(r); !known || m.SceneKindValue() != devconfig.SceneKindPersonal || !r.Auto || r.Player.HP <= 0 {
		w.failure(r, frame, p.CodeLineForbidden)
		return
	}
	// monster 保存由“active”怪物取得的怪物。
	monster := r.activeMonster(q.TargetHandle, q.TargetLife)
	if monster == nil {
		w.failure(r, frame, p.CodeSkillTarget)
		return
	}
	// 点选只指定一轮的首个目标；另一个有效的手动选择不改变既有自动锁。
	if r.liveAutoTarget() == nil {
		r.lockAutoTarget(reference(r, monster))
	}
}
