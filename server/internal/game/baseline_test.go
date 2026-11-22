package game

import (
	"google.golang.org/protobuf/proto"
	"math"
	pb "minimir/api/generated/minimirv1"
	p "minimir/internal/protocol"
	"minimir/internal/wire"
	"testing"
)

func TestPetRuntimeBaselineSplitsByActualWireBytesAndKeepsCompleteIdentity(t *testing.T) {
	out := &pb.SceneBaselineChunk{SceneEpoch: math.MaxUint64, BaselineId: math.MaxUint64, StateSequence: math.MaxUint64, SafeZoneId: 106,
		NavigationState: &pb.NavigationState{InputSequence: math.MaxUint32, Revision: math.MaxUint64, Status: navigationCancelled, Destination: &pb.GridPoint{X: 633, Y: 613}}}
	for i := 0; i < 128; i++ {
		e := &pb.EntityState{EntityHandle: uint32(1001 + i), EntityKind: 2, DefinitionId: math.MaxUint32, Position: &pb.GridPoint{X: math.MaxInt32, Y: math.MaxInt32}, Direction: 7, HpMax: math.MaxInt64, Mp: math.MaxInt64, MpMax: math.MaxInt64, Action: 4,
			Life: math.MaxUint64, AttackSequence: math.MaxUint32, CastSequence: math.MaxUint32, CastSkillId: math.MaxUint32, MoveIntervalMs: math.MaxUint32, AttackIntervalMs: math.MaxUint32, AttackSkillId: math.MaxUint32, AttackSkillLevel: math.MaxUint32, MovementRevision: math.MaxUint64, MovementKind: math.MaxUint32, IsPetCorpse: true,
			PetTraining: &pb.PetTrainingState{TrainingLevel: 7, MaxTrainingLevel: 7, Experience: math.MaxUint64}, CombatAttributes: &pb.CombatAttributes{PhysicalAttackMin: math.MaxInt64, PhysicalAttackMax: math.MaxInt64, PhysicalDefenceMin: math.MaxInt64, PhysicalDefenceMax: math.MaxInt64, MagicDefenceMin: math.MaxInt64, MagicDefenceMax: math.MaxInt64, MagicAttackMin: math.MaxInt64, MagicAttackMax: math.MaxInt64, SpiritAttackMin: math.MaxInt64, SpiritAttackMax: math.MaxInt64, Accuracy: math.MaxInt64, Agility: math.MaxInt64, DamageReductionPercent: math.MaxUint32}}
		for j := 0; j < 4; j++ {
			e.Statuses = append(e.Statuses, &pb.EntityStatusState{StatusId: math.MaxUint32 - uint32(j), SourceSkillId: math.MaxUint32, SkillLevel: math.MaxUint32, RemainingMs: math.MaxUint32, MaxPhysicalDefenceBonus: math.MaxInt64, MaxMagicDefenceBonus: math.MaxInt64, DamageReductionPercent: math.MaxUint32, CasterHandle: math.MaxUint32, CasterLife: math.MaxUint64, TeleportManaPenaltyPercent: math.MaxUint32})
		}
		out.Entities = append(out.Entities, e)
	}
	if proto.Size(out) < wire.MaxFrame {
		t.Fatal("边界前态没有实际超过单帧")
	}
	chunks, err := splitBaseline(out, math.MaxUint32, math.MaxUint64)
	if err != nil || len(chunks) < 2 {
		t.Fatal("完整基线没有正确拆块", err)
	}
	seen := 0
	for i, chunk := range chunks {
		encoded := wire.Encode(wire.Message(p.MsgS2CSceneBaseline, math.MaxUint32, math.MaxUint64, chunk))
		if len(encoded) > wire.MaxFrame || chunk.ChunkIndex != uint32(i) || chunk.ChunkCount != uint32(len(chunks)) || chunk.StateSequence != out.StateSequence || chunk.SceneEpoch != out.SceneEpoch || !proto.Equal(chunk.NavigationState, out.NavigationState) || chunk.SafeZoneId != 106 {
			t.Fatal("分块超过wire边界或公用身份/取消状态缺失")
		}
		for _, e := range chunk.Entities {
			if !proto.Equal(e, out.Entities[seen]) {
				t.Fatal("拆块丢失/改写尸体身份或字段")
			}
			seen++
		}
	}
	if seen != len(out.Entities) {
		t.Fatal("只发送部分尸体")
	}
}

func TestPetRuntimeBaselineRejectsSingleOversizedEntityBeforeSendingAnyChunk(t *testing.T) {
	e := &pb.EntityState{EntityHandle: 3}
	for i := 0; i < wire.MaxFrame; i++ {
		e.Statuses = append(e.Statuses, &pb.EntityStatusState{StatusId: uint32(i + 1)})
	}
	if chunks, err := splitBaseline(&pb.SceneBaselineChunk{ChunkCount: 1, Entities: []*pb.EntityState{e}}, 0, 0); err == nil || chunks != nil {
		t.Fatal("单实体越界仍发出部分基线")
	}
	if _, err := splitBaseline(&pb.SceneBaselineChunk{Entities: []*pb.EntityState{{EntityHandle: 3}, {EntityHandle: 3}}}, 0, 0); err == nil {
		t.Fatal("允许基线跨块重复handle")
	}
}

func TestSmallBaselineKeepsMetadataAndValidatesIdentity(t *testing.T) {
	out := &pb.SceneBaselineChunk{
		SceneEpoch: 7, BaselineId: 8, ChunkCount: 1, StateSequence: 9,
		Entities:        []*pb.EntityState{{EntityHandle: 1}, {EntityHandle: 10}},
		NavigationState: &pb.NavigationState{Destination: &pb.GridPoint{X: 12, Y: 12}},
	}
	want := proto.Clone(out)
	chunks, err := splitBaseline(out, math.MaxUint32, math.MaxUint64)
	if err != nil || len(chunks) != 1 || !proto.Equal(chunks[0], want) {
		t.Fatalf("个人地图单帧基线应保留原元数据：%v", err)
	}
	encoded := wire.Encode(wire.Message(p.MsgS2CSceneBaseline, math.MaxUint32, math.MaxUint64, chunks[0]))
	frame, err := wire.Decode(encoded)
	if err != nil || frame.Request != math.MaxUint32 || frame.Binding != math.MaxUint64 {
		t.Fatalf("单帧基线必须保留正常的封包身份：%v", err)
	}
	got := &pb.SceneBaselineChunk{}
	if err := proto.Unmarshal(frame.Payload, got); err != nil || !proto.Equal(got, want) {
		t.Fatalf("单帧基线编码后字段发生变化：%v", err)
	}
	for _, invalid := range []*pb.EntityState{nil, {}, {EntityHandle: 1}} {
		out.Entities[1] = invalid
		if chunks, err := splitBaseline(out, 0, 0); err == nil || chunks != nil {
			t.Fatal("单帧基线接收了无效或重叠的实体身份")
		}
	}
}
