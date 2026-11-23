// 本文件向当前已入场连接返回任务与图鉴的已确认状态。
package game

import (
	"google.golang.org/protobuf/proto"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
	"minimir/internal/wire"
	"time"
)

// taskCodexQuery 验证只读请求，并使用当前角色已确认状态返回活动运营日和目标行。
func (w *worker) taskCodexQuery(r *room, f wire.Frame) {
	var q pb.TaskCodexStateRequest // 查询无需传入目标进度或角色编号。
	if proto.Unmarshal(f.Payload, &q) != nil || len(q.ProtoReflect().GetUnknown()) != 0 {
		w.failure(r, f, p.CodeInvalidArgument)
		return
	}
	state := persistence.TaskCodexState(r.Role, &w.roomStore(r).Cfg.Fixture, time.Now())
	w.send(r, p.MsgS2CTaskCodexState, f.Request, state)
}
