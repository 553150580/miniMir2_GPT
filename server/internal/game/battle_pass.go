// 本文件通过当前角色附着查看战令，领取沿原角色资产流水提交。
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

// queryBattlePass 通过现有有界只读槽读取原角色的一个历史页和指定周期。
func (w *worker) queryBattlePass(r *room, f wire.Frame, now time.Time) {
	var req pb.BattlePassQuery // 页游标和周期只在当前角色下解释。
	if proto.Unmarshal(f.Payload, &req) != nil || len(req.ProtoReflect().GetUnknown()) != 0 || req.SceneEpoch != r.Scene ||
		len(req.SeasonStart) > 10 || len(req.HistoryBefore) > 10 {
		w.failure(r, f, p.CodeInvalidArgument)
		return
	}
	store := w.roomStore(r)
	if !w.growthSceneAvailable(r) || store.Cfg.Fixture.BattlePass == nil {
		w.failure(r, f, p.CodeBattlePassUnavailable)
		return
	}
	if r.PreviewInFlight || r.PreviewGeneration == math.MaxUint64 {
		w.failure(r, f, p.CodeServerBusy)
		return
	}
	r.PreviewInFlight = true
	r.PreviewGeneration++
	bound := skillPreviewBinding{room: r, conn: r.Conn, auth: r.Auth, scene: r.Scene, generation: r.PreviewGeneration}
	role := r.Role // 查询期间不读取在房间线程更新的状态。
	if !w.submit(job{target: w, run: func() func(*worker) {
		state, err := store.ReadBattlePassState(w.e.ctx, role, bound.scene, req.SeasonStart, req.HistoryBefore, now)
		return func(w *worker) {
			if !w.claimPreview(bound) {
				return
			}
			if err != nil {
				w.failure(r, f, persistence.Code(err))
				return
			}
			w.send(r, p.MsgS2CBattlePassState, f.Request, state)
		}
	}}) {
		r.PreviewInFlight = false
		w.failure(r, f, p.CodeServerBusy)
	}
}

// readBattlePassClaim 同时为新领取和断线回执重放解析同一原身份。
func readBattlePassClaim(frame wire.Frame) (*pb.BattlePassClaimRequest, bool) {
	var req pb.BattlePassClaimRequest
	if proto.Unmarshal(frame.Payload, &req) != nil {
		return nil, false
	}
	return &req, persistence.ValidBattlePassClaim(&req, req.Operation)
}
