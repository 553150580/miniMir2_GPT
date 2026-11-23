// 本文件验证任务图鉴命令经过房间会话和经济命令分派。
package game

import (
	"bytes"
	"testing"

	"google.golang.org/protobuf/proto"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	p "minimir/internal/protocol"
	"minimir/internal/wire"
)

// TestTaskCodexCommandUsesConfirmedRoleAndOperationIdentity 核对回包绑定、配置缺失和待确认请求身份。
func TestTaskCodexCommandUsesConfirmedRoleAndOperationIdentity(t *testing.T) {
	w, r, conn := skillWorker(t, 0)
	r.Role.Revision = 23
	f := &w.e.Store.Cfg.Fixture
	// 未发布定义时仍返回可识别的查询结果。
	f.TasksCodex = nil
	query := func(request uint32, epoch uint64) wire.Frame {
		t.Helper()
		conn.frames = nil
		w.command(command{role: "role", conn: conn, frame: wire.Message(p.MsgC2STaskCodexState, request, epoch, &pb.TaskCodexStateRequest{})})
		if len(conn.frames) != 1 || conn.frames[0].Request != request {
			t.Fatalf("任务查询响应没有关联原请求：%v", conn.frames)
		}
		return conn.frames[0]
	}
	frame := query(31, uint64(r.Auth.SessionEpoch))
	var state pb.TaskCodexStateResult
	if err := proto.Unmarshal(frame.Payload, &state); err != nil || frame.ID != p.MsgS2CTaskCodexState || state.ErrorCode != p.CodeTaskCodexUnavailable || len(state.Rows) != 0 {
		t.Fatalf("缺少任务定义时未关闭入口：%v %v", &state, err)
	}
	f.TasksCodex = &devconfig.TaskCodexRules{Definitions: []devconfig.TaskCodexDefinition{
		{ID: 1, Kind: devconfig.TaskCodexAchievement, Name: "隔离成就", Event: devconfig.TaskCodexLevelReached, Target: 5},
	}}
	frame = query(32, uint64(r.Auth.SessionEpoch))
	if err := proto.Unmarshal(frame.Payload, &state); err != nil || frame.ID != p.MsgS2CTaskCodexState || state.ErrorCode != 0 || state.RoleRevision != 23 || state.RuleVersion != f.RuleVersion || len(state.Rows) != 1 || !state.Rows[0].Claimable {
		t.Fatalf("客户端未收到当前已确认角色进度：%v %v", &state, err)
	}
	if stale := query(33, uint64(r.Auth.SessionEpoch+1)); stale.Error != p.CodeStaleSession || len(stale.Payload) != 0 {
		t.Fatalf("旧会话可查询当前角色任务：%v", stale)
	}
	// 在途请求不得始发第二个永久领取，按原编号给出待确认结果。
	r.Pending = true
	conn.frames = nil
	op := &pb.OperationContext{OpId: domain.Bytes(domain.ID()), CommandSeq: 1, ExpectedRevision: 23, ExpectedRuleVersion: f.RuleVersion}
	claim := &pb.TaskCodexClaimRequest{Operation: op, Kind: devconfig.TaskCodexAchievement, Id: 1}
	w.command(command{role: "role", conn: conn, frame: wire.Message(p.MsgC2STaskCodexClaim, 34, uint64(r.Auth.SessionEpoch), claim)})
	if len(conn.frames) != 1 || conn.frames[0].ID != p.MsgS2COperationResult || conn.frames[0].Request != 34 {
		t.Fatalf("待确认领取未返回经济操作响应：%v", conn.frames)
	}
	var result pb.OperationResult
	if err := proto.Unmarshal(conn.frames[0].Payload, &result); err != nil || !result.Pending || result.ErrorCode != p.CodeOperationPending || !bytes.Equal(result.OpId, op.OpId) {
		t.Fatalf("待确认领取丢失原操作身份：%v %v", &result, err)
	}
}
