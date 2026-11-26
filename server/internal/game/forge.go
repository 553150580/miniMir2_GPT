// 本文件只受理原附着的打造报价，资产核验和结果由持久层原事务处理。
package game

import (
	"math"

	"google.golang.org/protobuf/proto"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
	"minimir/internal/wire"
)

// forgeAvailable 让新打造限于启用规则中的存活角色，读请求不改变战斗或托管状态。
func (w *worker) forgeAvailable(r *room) bool {
	return w.roomStore(r).Cfg.Fixture.Forge != nil && r.Ready && r.Player.HP > 0
}

// forgePreview 复用当前角色有界的预览槽，旧连接或旧场景的回调不能进入新附着。
func (w *worker) forgePreview(r *room, frame wire.Frame) {
	var request pb.ForgeQuoteRequest
	if proto.Unmarshal(frame.Payload, &request) != nil || request.SceneEpoch != r.Scene || len(request.ProtoReflect().GetUnknown()) != 0 {
		w.failure(r, frame, p.CodeInvalidArgument)
		return
	}
	if !w.forgeAvailable(r) {
		w.failure(r, frame, p.CodeForgeUnavailable)
		return
	}
	if r.Pending || r.PreviewInFlight || r.PreviewGeneration == math.MaxUint64 {
		w.failure(r, frame, p.CodeOperationPending)
		return
	}
	r.PreviewGeneration++
	r.PreviewInFlight = true
	binding := skillPreviewBinding{room: r, conn: r.Conn, auth: r.Auth, scene: r.Scene, generation: r.PreviewGeneration}
	store := w.roomStore(r)
	work := job{target: w, run: func() func(*worker) {
		quote, err := store.PreviewForge(w.e.ctx, binding.auth, &request)
		return func(owner *worker) {
			if !owner.claimPreview(binding) {
				return
			}
			if !owner.forgeAvailable(r) {
				owner.failure(r, frame, p.CodeForgeUnavailable)
				return
			}
			if err != nil {
				owner.failure(r, frame, persistence.Code(err))
				return
			}
			owner.send(r, p.MsgS2CForgeQuote, frame.Request, quote)
		}
	}}
	if !w.submit(work) {
		r.PreviewInFlight = false
		w.failure(r, frame, p.CodeServerBusy)
	}
}
