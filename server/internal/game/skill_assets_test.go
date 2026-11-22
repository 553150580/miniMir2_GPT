// 本文件回归技能材料操作结果未知时保留原预览和原回执核对。
package game

import (
	"bytes"
	"google.golang.org/protobuf/proto"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/domain"
	p "minimir/internal/protocol"
	"minimir/internal/wire"
	"testing"
)

// TestSkillCommitPendingRetainsOriginalPreviewAndReceiptReconciliation 核对「队列已满把尚未核对的历史升级回执误判未受理」与「未知提交期间允许新预览覆盖原quote」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestSkillCommitPendingRetainsOriginalPreviewAndReceiptReconciliation(t *testing.T) {
	// w 接收药水房间执行器供后续步骤使用，r 接收药水房间执行器供后续步骤使用，c 接收药水房间执行器供后续步骤使用。
	w, r, c := potionWorker()
	r.Pending = true
	// op 保存当前处理所用的操作。
	op := &pb.OperationContext{OpId: domain.Bytes(domain.ID()), CommandSeq: uint64(r.Role.EconomicSeq + 1)}
	// q 保存当前处理所用的技能资产提交请求。
	q := &pb.SkillAssetCommitRequest{PreviewId: domain.Bytes(domain.ID()), Operation: op}
	// frame 保存由消息取得的协议帧。
	frame := wire.Message(p.MsgC2SSkillAssetCommit, 1, 3, q)
	// saved 保留加入新条目后的“saved”。
	saved := append([]byte(nil), frame.Payload...)
	w.command(command{role: "role", conn: c, frame: frame})
	if len(r.Waiting) != 1 || len(c.frames) != 0 || !bytes.Equal(r.Waiting[0].frame.Payload, saved) {
		t.Fatal("在途升级未保留原request/preview，或返回未受理的假Pending")
	}
	frame.Payload[0] ^= 1
	if !bytes.Equal(r.Waiting[0].frame.Payload, saved) {
		t.Fatal("排队升级共享可变消息缓冲")
	}
	r.Waiting = append(r.Waiting, r.Waiting[0])
	// historical 按目标类型读取当前接口值，得到“historical”。
	historical := proto.Clone(q).(*pb.SkillAssetCommitRequest)
	historical.Operation.OpId = domain.Bytes(domain.ID())
	historical.Operation.CommandSeq = uint64(r.Role.EconomicSeq)
	w.command(command{role: "role", conn: c, frame: wire.Message(p.MsgC2SSkillAssetCommit, 2, 3, historical)})
	// result 保存当前处理所用的结果。
	result := &pb.OperationResult{}
	// err 接收反序列化过程中可能产生的错误，供后续失败分支处理。
	if err := proto.Unmarshal(c.frames[len(c.frames)-1].Payload, result); err != nil {
		t.Fatal(err)
	}
	if !result.Pending || result.ErrorCode != p.CodeOperationPending {
		t.Fatal("队列已满把尚未核对的历史升级回执误判未受理")
	}
	// request 保存由消息取得的请求。
	request := wire.Message(p.MsgC2SSkillAssetPreview, 3, 3, &pb.SkillAssetPreviewRequest{Kind: 1, OfferId: 1, Quantity: 1})
	w.command(command{role: "role", conn: c, frame: request})
	if c.frames[len(c.frames)-1].Error != p.CodeOperationPending {
		t.Fatal("未知提交期间允许新预览覆盖原quote")
	}
}
