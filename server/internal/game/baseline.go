// 本文件校验场景基线的实体身份与编码大小，在单帧与多分块路径之间选择。
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

// baselineEntityLimit 规定场景基线实体限额的初始取值。
const baselineEntityLimit = 2048

// 先构建完整合法分块再发送；单个实体超帧时不能发半套基线或丢弃尸体。
// out 记录输出，使用场景场景基线分片表示，request 关联客户端请求与服务端的对应响应，binding 携带连接上的会话代次，用于防止旧连接命令被接受。
func splitBaseline(out *pb.SceneBaselineChunk, request uint32, binding uint64) ([]*pb.SceneBaselineChunk, error) {
	if len(out.Entities) > baselineEntityLimit {
		return nil, errors.New("场景基线实体超过双端合同")
	}
	// header 记录编码中的条目数。
	header := len(wire.Encode(wire.Frame{ID: p.MsgS2CSceneBaseline, Request: request, Binding: binding}))
	// 普通挂机图的完整基线已经带单帧身份。校验后直接交给发送端编码，
	// 省去整份实体克隆和仅用于长度核验的临时编码。
	// smallHandles 记录“small”实体句柄集合，使用数值列表表示。
	var smallHandles [16]uint32
	if len(out.Entities) <= len(smallHandles) && out.ChunkCount == 1 && out.ChunkIndex == 0 && header+proto.Size(out) <= wire.MaxFrame {
		// i 定位实体中正在处理的成员，entity 接收遍历实体时当前的元素。
		for i, entity := range out.Entities {
			if entity == nil || entity.EntityHandle == 0 || slices.Contains(smallHandles[:i], entity.EntityHandle) {
				return nil, errors.New("场景基线实体身份重复或无效")
			}
			smallHandles[i] = entity.EntityHandle
		}
		return []*pb.SceneBaselineChunk{out}, nil
	}

	// base 按目标类型读取当前接口值，得到基础值。
	base := proto.Clone(out).(*pb.SceneBaselineChunk)
	base.Entities = nil
	base.ChunkCount, base.ChunkIndex = baselineChunkLimit, baselineChunkLimit-1
	// metadata 保存由大小取得的元数据。
	metadata := proto.Size(base)
	if header+metadata > wire.MaxFrame {
		return nil, errors.New("场景基线元数据超过单帧")
	}
	// chunks 建立带初始值的场景场景基线“chunk”列表，供本段继续处理。
	chunks := []*pb.SceneBaselineChunk{}
	// newChunk 封装创建分片对应的后续处理。
	newChunk := func() *pb.SceneBaselineChunk {
		// chunk 按目标类型读取当前接口值，得到“chunk”。
		chunk := proto.Clone(base).(*pb.SceneBaselineChunk)
		chunks = append(chunks, chunk)
		return chunk
	}
	// chunk 保存由创建分片取得的分片，size 保存根据头部与元数据计算的大小。
	chunk, size := newChunk(), header+metadata
	// handles 初始化用作实体句柄集合的按数值索引的布尔状态映射。
	handles := map[uint32]bool{}
	// entity 接收遍历实体时当前的元素。
	for _, entity := range out.Entities {
		if entity == nil || entity.EntityHandle == 0 || handles[entity.EntityHandle] {
			return nil, errors.New("场景基线实体身份重复或无效")
		}
		handles[entity.EntityHandle] = true
		// itemSize 保存根据大小标记与大小字节计算的道具大小。
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
	// i 定位“chunks”中正在处理的成员，chunk 接收遍历“chunks”时当前的元素。
	for i, chunk := range chunks {
		chunk.ChunkIndex, chunk.ChunkCount = uint32(i), uint32(len(chunks))
		if len(wire.Encode(wire.Message(p.MsgS2CSceneBaseline, request, binding, chunk))) > wire.MaxFrame {
			return nil, errors.New("场景基线实际编码超过单帧")
		}
	}
	return chunks, nil
}
