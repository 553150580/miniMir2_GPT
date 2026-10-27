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

func TestSkillCommitPendingRetainsOriginalPreviewAndReceiptReconciliation(t *testing.T) {
	w, r, c := potionWorker()
	r.Pending = true
	op := &pb.OperationContext{OpId: domain.Bytes(domain.ID()), CommandSeq: uint64(r.Role.EconomicSeq + 1)}
	q := &pb.SkillAssetCommitRequest{PreviewId: domain.Bytes(domain.ID()), Operation: op}
	frame := wire.Message(p.MsgC2SSkillAssetCommit, 1, 3, q)
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
	historical := proto.Clone(q).(*pb.SkillAssetCommitRequest)
	historical.Operation.OpId = domain.Bytes(domain.ID())
	historical.Operation.CommandSeq = uint64(r.Role.EconomicSeq)
	w.command(command{role: "role", conn: c, frame: wire.Message(p.MsgC2SSkillAssetCommit, 2, 3, historical)})
	result := &pb.OperationResult{}
	if err := proto.Unmarshal(c.frames[len(c.frames)-1].Payload, result); err != nil {
		t.Fatal(err)
	}
	if !result.Pending || result.ErrorCode != p.CodeOperationPending {
		t.Fatal("队列已满把尚未核对的历史升级回执误判未受理")
	}
	request := wire.Message(p.MsgC2SSkillAssetPreview, 3, 3, &pb.SkillAssetPreviewRequest{Kind: 1, OfferId: 1, Quantity: 1})
	w.command(command{role: "role", conn: c, frame: request})
	if c.frames[len(c.frames)-1].Error != p.CodeOperationPending {
		t.Fatal("未知提交期间允许新预览覆盖原quote")
	}
}
