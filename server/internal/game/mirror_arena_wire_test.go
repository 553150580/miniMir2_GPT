// 本文件核对镜像补召观察只能绑定包含本次新生活体的完整帧。
package game

import (
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	pb "minimir/api/generated/minimirv1"
	p "minimir/internal/protocol"
)

// TestMirrorPetBornUsesOnlyCurrentFullFrame 核对旧代次、旧尸体、旧方向及过期移动水位不能播放出生。
func TestMirrorPetBornUsesOnlyCurrentFullFrame(t *testing.T) {
	bornAt := time.Now()
	fact := &pb.PetBorn{SceneEpoch: 42, EntityHandle: 103, EntityLife: 2, DefinitionId: 17, MapId: 90001,
		SourceCastId: 5, SourceSkillId: 17, BornAtUnixMs: uint64(bornAt.UnixMilli()), Direction: 3,
		Role: &pb.RoleKey{OriginServerId: 7}}
	observation := mirrorCombatObservation{ID: p.MsgS2CPetBorn, Message: fact, bornAt: bornAt}
	frame := &pb.MirrorArenaCombatFrame{SceneEpoch: 6, BattleSceneEpoch: 42, Sequence: 9,
		Entities: []*pb.EntityState{{EntityKind: 2, EntityHandle: 103, Life: 2, Hp: 12, DefinitionId: 17, Direction: 3}}}
	result := mirrorPetBornInFrame(observation, frame, 90001, bornAt.Add(84*time.Millisecond))
	if result == nil || result.SceneEpoch != 42 || result.BaselineId != 42 || result.StateSequence != 9 || result.ElapsedMs != 84 ||
		result.Role != nil || result.SourceCastId != 5 || fact.BaselineId != 0 || fact.Role == nil {
		t.Fatal("镜像完整帧没有准确绑定出生，或改变了原事实", result)
	}
	for name, change := range map[string]func(*pb.MirrorArenaCombatFrame){
		"场次": func(value *pb.MirrorArenaCombatFrame) { value.BattleSceneEpoch++ },
		"代次": func(value *pb.MirrorArenaCombatFrame) { value.Entities[0].Life++ },
		"尸体": func(value *pb.MirrorArenaCombatFrame) { value.Entities[0].IsPetCorpse = true },
		"死亡": func(value *pb.MirrorArenaCombatFrame) { value.Entities[0].Hp = 0 },
		"转向": func(value *pb.MirrorArenaCombatFrame) { value.Entities[0].Direction++ },
		"移动": func(value *pb.MirrorArenaCombatFrame) { value.Entities[0].MovementRevision++ },
	} {
		t.Run(name, func(t *testing.T) {
			stale := proto.CloneOf(frame)
			change(stale)
			if mirrorPetBornInFrame(observation, stale, 90001, bornAt) != nil {
				t.Fatal("补召观察被串到不符的完整帧")
			}
		})
	}
}
