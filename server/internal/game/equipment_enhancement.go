// 本文件受理普通地图和城镇中的部位强化，并沿用原角色资产确认路径。
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

// equipmentEnhancementAvailable 限定新报价和新写入。挑战快照不会被改动。
func (w *worker) equipmentEnhancementAvailable(r *room) bool {
	fixture := &w.roomStore(r).Cfg.Fixture
	if !r.Ready || r.Player.HP <= 0 || r.bossBattle != nil || fixture.EquipmentEnhancement == nil || miningMap(fixture, r.MapID) {
		return false
	}
	m, found := w.currentMap(r)
	return found && (m.SceneKind == devconfig.SceneKindPersonal || m.SceneKind == devconfig.SceneKindTown)
}

// equipmentEnhancementPreview 延续房间有界预览槽和当前连接及场景的返回校验。
func (w *worker) equipmentEnhancementPreview(r *room, frame wire.Frame) {
	var request pb.EquipmentEnhancementQuoteRequest
	if proto.Unmarshal(frame.Payload, &request) != nil || request.SceneEpoch != r.Scene || !persistence.ValidEquipmentEnhancementQuoteRequest(&request) {
		w.failure(r, frame, p.CodeInvalidArgument)
		return
	}
	if !w.equipmentEnhancementAvailable(r) {
		w.failure(r, frame, p.CodeEquipmentEnhancementUnavailable)
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
		quote, err := store.PreviewEquipmentEnhancement(w.e.ctx, binding.auth, &request)
		return func(owner *worker) {
			if !owner.claimPreview(binding) {
				return
			}
			if !owner.equipmentEnhancementAvailable(r) {
				owner.failure(r, frame, p.CodeEquipmentEnhancementUnavailable)
				return
			}
			if err != nil {
				owner.failure(r, frame, persistence.Code(err))
				return
			}
			owner.send(r, p.MsgS2CEquipmentEnhancementQuote, frame.Request, quote)
		}
	}}
	if !w.submit(work) {
		r.PreviewInFlight = false
		w.failure(r, frame, p.CodeServerBusy)
	}
}

// readEquipmentEnhancementCommit 识别原经济命令和原界面报价，不信任客户端传入概率。
func readEquipmentEnhancementCommit(frame wire.Frame) (*pb.EquipmentEnhancementCommitRequest, bool) {
	var request pb.EquipmentEnhancementCommitRequest
	if proto.Unmarshal(frame.Payload, &request) != nil || !persistence.ValidEquipmentEnhancementCommitRequest(&request) {
		return nil, false
	}
	return &request, true
}
