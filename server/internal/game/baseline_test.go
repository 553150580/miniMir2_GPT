// 本文件回归场景基线的分帧上限、实体身份、元数据和单帧快速路径。
package game

import (
	"google.golang.org/protobuf/proto"
	"math"
	pb "minimir/api/generated/minimirv1"
	p "minimir/internal/protocol"
	"minimir/internal/wire"
	"testing"
)

// TestPetRuntimeBaselineSplitsByActualWireBytesAndKeepsCompleteIdentity 核验边界前态应实际超过单帧，以及完整基线应正确拆块。
// t 提供断言失败报告、子用例和清理注册。
func TestPetRuntimeBaselineSplitsByActualWireBytesAndKeepsCompleteIdentity(t *testing.T) {
	// out 保存当前处理所用的输出。
	out := &pb.SceneBaselineChunk{SceneEpoch: math.MaxUint64, BaselineId: math.MaxUint64, StateSequence: math.MaxUint64, SafeZoneId: 106,
		NavigationState: &pb.NavigationState{InputSequence: math.MaxUint32, Revision: math.MaxUint64, Status: navigationCancelled, Destination: &pb.GridPoint{X: 633, Y: 613}}}
	// i 标记循环或列表中的当前位置。
	for i := 0; i < 128; i++ {
		// e 保存当前处理所用的实体状态。
		e := &pb.EntityState{EntityHandle: uint32(1001 + i), EntityKind: 2, DefinitionId: math.MaxUint32, Position: &pb.GridPoint{X: math.MaxInt32, Y: math.MaxInt32}, Direction: 7, HpMax: math.MaxInt64, Mp: math.MaxInt64, MpMax: math.MaxInt64, Action: 4,
			Life: math.MaxUint64, AttackSequence: math.MaxUint32, CastSequence: math.MaxUint32, CastSkillId: math.MaxUint32, MoveIntervalMs: math.MaxUint32, AttackIntervalMs: math.MaxUint32, AttackSkillId: math.MaxUint32, AttackSkillLevel: math.MaxUint32, MovementRevision: math.MaxUint64, MovementKind: math.MaxUint32, IsPetCorpse: true,
			PetTraining: &pb.PetTrainingState{TrainingLevel: 7, MaxTrainingLevel: 7, Experience: math.MaxUint64}, CombatAttributes: &pb.CombatAttributes{PhysicalAttackMin: math.MaxInt64, PhysicalAttackMax: math.MaxInt64, PhysicalDefenceMin: math.MaxInt64, PhysicalDefenceMax: math.MaxInt64, MagicDefenceMin: math.MaxInt64, MagicDefenceMax: math.MaxInt64, MagicAttackMin: math.MaxInt64, MagicAttackMax: math.MaxInt64, SpiritAttackMin: math.MaxInt64, SpiritAttackMax: math.MaxInt64, Accuracy: math.MaxInt64, Agility: math.MaxInt64, DamageReductionPercent: math.MaxUint32}}
		// j 标记循环或列表中的当前位置。
		for j := 0; j < 4; j++ {
			e.Statuses = append(e.Statuses, &pb.EntityStatusState{StatusId: math.MaxUint32 - uint32(j), SourceSkillId: math.MaxUint32, SkillLevel: math.MaxUint32, RemainingMs: math.MaxUint32, MaxPhysicalDefenceBonus: math.MaxInt64, MaxMagicDefenceBonus: math.MaxInt64, DamageReductionPercent: math.MaxUint32, CasterHandle: math.MaxUint32, CasterLife: math.MaxUint64, TeleportManaPenaltyPercent: math.MaxUint32})
		}
		out.Entities = append(out.Entities, e)
	}
	if proto.Size(out) < wire.MaxFrame {
		t.Fatal("边界前态没有实际超过单帧")
	}
	// chunks 保存由拆分场景基线取得的“chunks”，err 接收拆分场景基线过程中可能产生的错误，供后续失败分支处理。
	chunks, err := splitBaseline(out, math.MaxUint32, math.MaxUint64)
	if err != nil || len(chunks) < 2 {
		t.Fatal("完整基线没有正确拆块", err)
	}
	// seen 规定“seen”的初始取值。
	seen := 0
	// i 定位“chunks”中正在处理的成员，chunk 接收遍历“chunks”时当前的元素。
	for i, chunk := range chunks {
		// encoded 保存由编码取得的“encoded”。
		encoded := wire.Encode(wire.Message(p.MsgS2CSceneBaseline, math.MaxUint32, math.MaxUint64, chunk))
		if len(encoded) > wire.MaxFrame || chunk.ChunkIndex != uint32(i) || chunk.ChunkCount != uint32(len(chunks)) || chunk.StateSequence != out.StateSequence || chunk.SceneEpoch != out.SceneEpoch || !proto.Equal(chunk.NavigationState, out.NavigationState) || chunk.SafeZoneId != 106 {
			t.Fatal("分块超过wire边界或公用身份/取消状态缺失")
		}
		// e 接收遍历实体时当前的元素。
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

// TestPetRuntimeBaselineRejectsSingleOversizedEntityBeforeSendingAnyChunk 核对「允许基线跨块重复handle」与「单实体越界仍发出部分基线」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestPetRuntimeBaselineRejectsSingleOversizedEntityBeforeSendingAnyChunk(t *testing.T) {
	// e 保存当前处理所用的实体状态。
	e := &pb.EntityState{EntityHandle: 3}
	// i 标记循环或列表中的当前位置。
	for i := 0; i < wire.MaxFrame; i++ {
		e.Statuses = append(e.Statuses, &pb.EntityStatusState{StatusId: uint32(i + 1)})
	}
	// chunks 保存由拆分场景基线取得的“chunks”，err 接收拆分场景基线过程中可能产生的错误，供后续失败分支处理。
	if chunks, err := splitBaseline(&pb.SceneBaselineChunk{ChunkCount: 1, Entities: []*pb.EntityState{e}}, 0, 0); err == nil || chunks != nil {
		t.Fatal("单实体越界仍发出部分基线")
	}
	// err 接收拆分场景基线过程中可能产生的错误，供后续失败分支处理。
	if _, err := splitBaseline(&pb.SceneBaselineChunk{Entities: []*pb.EntityState{{EntityHandle: 3}, {EntityHandle: 3}}}, 0, 0); err == nil {
		t.Fatal("允许基线跨块重复handle")
	}
}

// TestSmallBaselineKeepsMetadataAndValidatesIdentity 核验单帧基线必须保留正常的封包身份，用诊断「单帧基线接收了无效或重叠的实体身份」定位对应断言。
// t 提供断言失败报告、子用例和清理注册。
func TestSmallBaselineKeepsMetadataAndValidatesIdentity(t *testing.T) {
	// out 保存当前处理所用的输出。
	out := &pb.SceneBaselineChunk{
		SceneEpoch: 7, BaselineId: 8, ChunkCount: 1, StateSequence: 9,
		Entities:        []*pb.EntityState{{EntityHandle: 1}, {EntityHandle: 10}},
		NavigationState: &pb.NavigationState{Destination: &pb.GridPoint{X: 12, Y: 12}},
	}
	// want 保存由副本取得的期望值。
	want := proto.Clone(out)
	// chunks 保存由拆分场景基线取得的“chunks”，err 接收拆分场景基线过程中可能产生的错误，供后续失败分支处理。
	chunks, err := splitBaseline(out, math.MaxUint32, math.MaxUint64)
	if err != nil || len(chunks) != 1 || !proto.Equal(chunks[0], want) {
		t.Fatalf("个人地图单帧基线应保留原元数据：%v", err)
	}
	// encoded 保存由编码取得的“encoded”。
	encoded := wire.Encode(wire.Message(p.MsgS2CSceneBaseline, math.MaxUint32, math.MaxUint64, chunks[0]))
	// frame 保存由解码取得的协议帧。
	frame, err := wire.Decode(encoded)
	if err != nil || frame.Request != math.MaxUint32 || frame.Binding != math.MaxUint64 {
		t.Fatalf("单帧基线必须保留正常的封包身份：%v", err)
	}
	// got 保存当前处理所用的实际结果。
	got := &pb.SceneBaselineChunk{}
	// err 接收反序列化过程中可能产生的错误，供后续失败分支处理。
	if err := proto.Unmarshal(frame.Payload, got); err != nil || !proto.Equal(got, want) {
		t.Fatalf("单帧基线编码后字段发生变化：%v", err)
	}
	// invalid 接收遍历实体状态列表时当前的元素。
	for _, invalid := range []*pb.EntityState{nil, {}, {EntityHandle: 1}} {
		out.Entities[1] = invalid
		// chunks 保存由拆分场景基线取得的“chunks”，err 接收拆分场景基线过程中可能产生的错误，供后续失败分支处理。
		if chunks, err := splitBaseline(out, 0, 0); err == nil || chunks != nil {
			t.Fatal("单帧基线接收了无效或重叠的实体身份")
		}
	}
}
