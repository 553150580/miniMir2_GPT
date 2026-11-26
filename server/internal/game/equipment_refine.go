// 本文件受理普通场景中的洗炼与决定，保持原附着、耐久和经济队列顺序。
package game

import (
	"math"
	"time"

	"google.golang.org/protobuf/proto"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
	"minimir/internal/wire"
)

// equipmentRefineAvailable 只在启用当前候选的普通地图或城镇受理首次操作。
func (w *worker) equipmentRefineAvailable(r *room) bool {
	fixture := &w.roomStore(r).Cfg.Fixture
	if !r.Ready || r.Player.HP <= 0 || r.bossBattle != nil || fixture.EquipmentRefine == nil || miningMap(fixture, r.MapID) {
		return false
	}
	m, found := w.currentMap(r)
	return found && (m.SceneKind == devconfig.SceneKindPersonal || m.SceneKind == devconfig.SceneKindTown)
}

// equipmentRefinePreview 复用有界异步报价槽，旧场景不能接收新窗口可用的报价。
func (w *worker) equipmentRefinePreview(r *room, frame wire.Frame) {
	var request pb.EquipmentRefineQuoteRequest
	if proto.Unmarshal(frame.Payload, &request) != nil || request.SceneEpoch != r.Scene || !persistence.ValidEquipmentRefineQuoteRequest(&request) {
		w.failure(r, frame, p.CodeInvalidArgument)
		return
	}
	if !w.equipmentRefineAvailable(r) {
		w.failure(r, frame, p.CodeEquipmentRefineUnavailable)
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
		quote, err := store.PreviewEquipmentRefine(w.e.ctx, binding.auth, &request)
		return func(owner *worker) {
			if !owner.claimPreview(binding) {
				return
			}
			if !owner.equipmentRefineAvailable(r) {
				owner.failure(r, frame, p.CodeEquipmentRefineUnavailable)
				return
			}
			if err != nil {
				owner.failure(r, frame, persistence.Code(err))
				return
			}
			owner.send(r, p.MsgS2CEquipmentRefineQuote, frame.Request, quote)
		}
	}}
	if !w.submit(work) {
		r.PreviewInFlight = false
		w.failure(r, frame, p.CodeServerBusy)
	}
}

// readEquipmentRefineAction 解析原持久请求，复用存储定义的确切形状。
func readEquipmentRefineAction(frame wire.Frame) (*pb.EquipmentRefineActionRequest, bool) {
	var request pb.EquipmentRefineActionRequest
	if proto.Unmarshal(frame.Payload, &request) != nil || !persistence.ValidEquipmentRefineAction(&request) {
		return nil, false
	}
	return &request, true
}

// deferEquipmentRefine 先确认已经发生的同装备磨损。原确认保留，旧版本随后由存储判定。
func (w *worker) deferEquipmentRefine(r *room, c command, now time.Time) bool {
	if c.frame.ID != p.MsgC2SEquipmentRefineAction || r.Pending || len(r.Wear.open) == 0 || !w.equipmentRefineAvailable(r) {
		return false
	}
	request, valid := readEquipmentRefineAction(c.frame)
	if !valid || !request.Target.Equipped || r.Role.EconomicSeq == math.MaxInt64 ||
		request.Operation.CommandSeq != uint64(r.Role.EconomicSeq+1) || request.Operation.ExpectedRevision == 0 ||
		request.Operation.ExpectedRevision > uint64(r.Role.Revision) {
		return false
	}
	id := domain.Hex(request.Target.InstanceId)
	for _, cost := range r.Wear.open {
		if cost.InstanceID == id && cost.Version == int64(request.Target.Version) && w.flushWear(r, now, true) {
			if r.Pending && !r.Isolated {
				w.queueAssetCommand(r, c)
			}
			return true
		}
	}
	return false
}
