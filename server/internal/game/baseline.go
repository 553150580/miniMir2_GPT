package game

import (
	"errors"
	"slices"

	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"
	pb "minimir/api/generated/minimirv1"
	p "minimir/internal/protocol"
	"minimir/internal/wire"
)

// 与双端组装合同一致的协议硬边界，不是宠物玩法数量或性能容量默认。
const baselineChunkLimit = 64
const baselineEntityLimit = 2048

// 先构建完整合法分块再发送；单个实体超帧时不能发半套基线或丢弃尸体。
func splitBaseline(out *pb.SceneBaselineChunk, request uint32, binding uint64) ([]*pb.SceneBaselineChunk, error) {
	if len(out.Entities) > baselineEntityLimit {
		return nil, errors.New("场景基线实体超过双端合同")
	}
	header := len(wire.Encode(wire.Frame{ID: p.MsgS2CSceneBaseline, Request: request, Binding: binding}))
	// 普通挂机图的完整基线已经带单帧身份。校验后直接交给发送端编码，
	// 省去整份实体克隆和仅用于长度核验的临时编码。
	var smallHandles [16]uint32
	if len(out.Entities) <= len(smallHandles) && out.ChunkCount == 1 && out.ChunkIndex == 0 && header+proto.Size(out) <= wire.MaxFrame {
		for i, entity := range out.Entities {
			if entity == nil || entity.EntityHandle == 0 || slices.Contains(smallHandles[:i], entity.EntityHandle) {
				return nil, errors.New("场景基线实体身份重复或无效")
			}
			smallHandles[i] = entity.EntityHandle
		}
		return []*pb.SceneBaselineChunk{out}, nil
	}

	base := proto.Clone(out).(*pb.SceneBaselineChunk)
	base.Entities = nil
	base.ChunkCount, base.ChunkIndex = baselineChunkLimit, baselineChunkLimit-1
	metadata := proto.Size(base)
	if header+metadata > wire.MaxFrame {
		return nil, errors.New("场景基线元数据超过单帧")
	}
	chunks := []*pb.SceneBaselineChunk{}
	newChunk := func() *pb.SceneBaselineChunk {
		chunk := proto.Clone(base).(*pb.SceneBaselineChunk)
		chunks = append(chunks, chunk)
		return chunk
	}
	chunk, size := newChunk(), header+metadata
	handles := map[uint32]bool{}
	for _, entity := range out.Entities {
		if entity == nil || entity.EntityHandle == 0 || handles[entity.EntityHandle] {
			return nil, errors.New("场景基线实体身份重复或无效")
		}
		handles[entity.EntityHandle] = true
		itemSize := protowire.SizeTag(6) + protowire.SizeBytes(proto.Size(entity))
		if header+metadata+itemSize > wire.MaxFrame {
			return nil, errors.New("单个场景实体超过单帧")
		}
		if size+itemSize > wire.MaxFrame {
			chunk, size = newChunk(), header+metadata
		}
		chunk.Entities = append(chunk.Entities, entity)
		size += itemSize
	}
	if len(chunks) > baselineChunkLimit {
		return nil, errors.New("场景基线分块超过双端合同")
	}
	for i, chunk := range chunks {
		chunk.ChunkIndex, chunk.ChunkCount = uint32(i), uint32(len(chunks))
		if len(wire.Encode(wire.Message(p.MsgS2CSceneBaseline, request, binding, chunk))) > wire.MaxFrame {
			return nil, errors.New("场景基线实际编码超过单帧")
		}
	}
	return chunks, nil
}
