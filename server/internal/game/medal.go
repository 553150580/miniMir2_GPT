// 本文件在就绪的普通地图和城镇受理勋章的报价及原确认。
package game

import (
	"math"

	"google.golang.org/protobuf/proto"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
	"minimir/internal/wire"
)

// medalAvailable 判断当前活角色是否可进行首次勋章培养，原回执重放不走这个限制。
func (w *worker) medalAvailable(r *room) bool {
	return w.roomStore(r).Cfg.Fixture.Medal != nil && w.growthSceneAvailable(r)
}

// growthSceneAvailable 允许活角色在普通挂机地图或城镇办理养成。
func (w *worker) growthSceneAvailable(r *room) bool {
	fixture := &w.roomStore(r).Cfg.Fixture
	if !r.Ready || r.Player.HP <= 0 || r.bossBattle != nil || r.mirrorMatch != nil ||
		miningMap(fixture, r.MapID) {
		return false
	}
	current, found := w.currentMap(r)
	return found && (current.SceneKind == devconfig.SceneKindPersonal || current.SceneKind == devconfig.SceneKindTown)
}

// medalPreview 复用角色原异步报价槽，旧连接换图或失去原附着后不能领取报价。
func (w *worker) medalPreview(r *room, frame wire.Frame) {
	var request pb.MedalQuoteRequest
	if proto.Unmarshal(frame.Payload, &request) != nil || request.SceneEpoch != r.Scene ||
		!persistence.ValidMedalQuoteRequest(&request) {
		w.failure(r, frame, p.CodeInvalidArgument)
		return
	}
	if !w.medalAvailable(r) {
		w.failure(r, frame, p.CodeMedalUnavailable)
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
		quote, err := store.PreviewMedal(w.e.ctx, binding.auth, &request)
		return func(owner *worker) {
			if !owner.claimPreview(binding) {
				return
			}
			if !owner.medalAvailable(r) {
				owner.failure(r, frame, p.CodeMedalUnavailable)
				return
			}
			if err != nil {
				owner.failure(r, frame, persistence.Code(err))
				return
			}
			owner.send(r, p.MsgS2CMedalQuote, frame.Request, quote)
		}
	}}
	if !w.submit(work) {
		r.PreviewInFlight = false
		w.failure(r, frame, p.CodeServerBusy)
	}
}

// readMedalCommit 解析首次确认和历史重放共用的原经济指令。
func readMedalCommit(frame wire.Frame) (*pb.MedalCommitRequest, bool) {
	var request pb.MedalCommitRequest
	if proto.Unmarshal(frame.Payload, &request) != nil {
		return nil, false
	}
	return &request, persistence.ValidMedalCommit(&request, request.Operation)
}
