// 本文件向当前已入场连接返回任务与图鉴的已确认状态。
package game

import (
	"math"
	"time"

	"google.golang.org/protobuf/proto"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
	"minimir/internal/wire"
)

// taskCodexQuery 验证只读请求，并使用当前角色已确认状态返回活动运营日和目标行。
func (w *worker) taskCodexQuery(r *room, f wire.Frame) {
	var q pb.TaskCodexStateRequest // 查询无需传入目标进度或角色编号。
	if proto.Unmarshal(f.Payload, &q) != nil || len(q.ProtoReflect().GetUnknown()) != 0 {
		w.failure(r, f, p.CodeInvalidArgument)
		return
	}
	store := w.roomStore(r)
	if !store.Cfg.Fixture.HasWorldBossTaskProgress() {
		w.send(r, p.MsgS2CTaskCodexState, f.Request, persistence.TaskCodexState(r.Role, &store.Cfg.Fixture, time.Now()))
		return
	}
	// 世界首领事实由共享场次事务写入。通过有界只读队列读取，保留原房间提交权限。
	if !r.Ready || r.PreviewInFlight || r.PreviewGeneration == math.MaxUint64 {
		w.send(r, p.MsgS2CTaskCodexState, f.Request, &pb.TaskCodexStateResult{ErrorCode: p.CodeServerBusy})
		return
	}
	r.PreviewInFlight = true
	r.PreviewGeneration++
	bound := skillPreviewBinding{room: r, conn: r.Conn, auth: r.Auth, scene: r.Scene, generation: r.PreviewGeneration}
	role := r.Role
	if !w.submit(job{target: w, run: func() func(*worker) {
		progress, err := store.ReadWorldBossTaskProgress(w.e.ctx, role)
		return func(w *worker) {
			if r.PreviewGeneration != bound.generation || !r.PreviewInFlight {
				return
			}
			r.PreviewInFlight = false
			if w.rooms[bound.auth.RoleID] != r || r.Auth != bound.auth || r.Conn != bound.conn || r.Scene != bound.scene ||
				r.Isolated || !r.Ready || bound.conn == nil || !bound.conn.Alive() || bound.conn.Binding() != uint64(bound.auth.SessionEpoch) {
				return
			}
			state := &pb.TaskCodexStateResult{ErrorCode: persistence.Code(err)}
			if err == nil {
				state = persistence.TaskCodexStateWithWorldBossProgress(r.Role, &store.Cfg.Fixture, progress, time.Now())
			}
			w.send(r, p.MsgS2CTaskCodexState, f.Request, state)
		}
	}}) {
		r.PreviewInFlight = false
		w.send(r, p.MsgS2CTaskCodexState, f.Request, &pb.TaskCodexStateResult{ErrorCode: p.CodeServerBusy})
	}
}
