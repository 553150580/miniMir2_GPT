// 本文件统一角色养成报价的异步槽、附着检查和首领场景门禁。
package game

import (
	"context"
	"math"

	"google.golang.org/protobuf/proto"
	"minimir/internal/devconfig"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
	"minimir/internal/wire"
)

// growthSceneAvailable 允许活角色在普通挂机地图或城镇办理养成。
func (w *worker) growthSceneAvailable(r *room) bool {
	fixture := &w.roomStore(r).Cfg.Fixture
	if !r.Ready || r.Player.HP <= 0 || r.bossBattle != nil || r.mirrorMatch != nil || miningMap(fixture, r.MapID) {
		return false
	}
	current, found := w.currentMap(r)
	return found && (current.SceneKind == devconfig.SceneKindPersonal || current.SceneKind == devconfig.SceneKindTown)
}

// growthPreviewRequest 在数据库任务内生成已经校验的原报价。
type growthPreviewRequest func(context.Context, *persistence.Store, persistence.Auth) (proto.Message, error)

// enqueueGrowthPreview 在原角色附着上异步请求报价，换图和断线时丢弃旧报价响应。
func (w *worker) enqueueGrowthPreview(r *room, frame wire.Frame, responseID uint16, unavailable uint32,
	available func(*room) bool, preview growthPreviewRequest) {
	if !available(r) {
		w.failure(r, frame, unavailable)
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
	if !w.submit(job{target: w, run: func() func(*worker) {
		quote, err := preview(w.e.ctx, store, binding.auth)
		return func(owner *worker) {
			if !owner.claimPreview(binding) {
				return
			}
			if !available(r) {
				owner.failure(r, frame, unavailable)
				return
			}
			if err != nil {
				owner.failure(r, frame, persistence.Code(err))
				return
			}
			owner.send(r, responseID, frame.Request, quote)
		}
	}}) {
		r.PreviewInFlight = false
		w.failure(r, frame, p.CodeServerBusy)
	}
}
