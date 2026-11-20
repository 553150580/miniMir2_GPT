package game

import (
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	p "minimir/internal/protocol"
	"minimir/internal/wire"

	"google.golang.org/protobuf/proto"
)

func (r *room) activeMonster(handle uint32, life uint64) *entity {
	for _, monster := range r.Monsters[:r.MonsterCount] {
		if monster != nil && monster.Kind == 3 && monster.HP > 0 && monster.Handle == handle && monster.Life == life {
			return monster
		}
	}
	return nil
}

func (r *room) liveAutoTarget() *entity {
	ref := r.AutoTarget
	if ref.Handle == 0 {
		return nil
	}
	if ref.World == r.currentWorld().Ref {
		if target := r.activeMonster(ref.Handle, ref.Life); target != nil {
			return target
		}
	}
	r.AutoTarget = actorRef{}
	return nil
}

// 仅锁住仍属于当前现场的同一只活怪；攻击中的即时命中或世界移交可以让该引用先失效。
func (r *room) lockAutoTarget(ref actorRef) {
	if r.Player.HP > 0 && ref.World == r.currentWorld().Ref && r.activeMonster(ref.Handle, ref.Life) != nil {
		r.AutoTarget = ref
	}
}

func (w *worker) autoTargetIntent(r *room, frame wire.Frame) {
	var q pb.AutoTargetIntent
	if proto.Unmarshal(frame.Payload, &q) != nil || q.SceneEpoch != r.Scene || q.TargetHandle == 0 {
		w.failure(r, frame, p.CodeInvalidArgument)
		return
	}
	if !acceptInput(r, q.InputSequence) {
		w.failure(r, frame, p.CodeStaleInput)
		return
	}
	if m, known := w.currentMap(r); !known || m.SceneKindValue() != devconfig.SceneKindPersonal || !r.Auto || r.Player.HP <= 0 {
		w.failure(r, frame, p.CodeLineForbidden)
		return
	}
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
