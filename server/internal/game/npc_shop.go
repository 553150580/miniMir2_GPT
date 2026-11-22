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

func (w *worker) vendorShop(r *room, scene, npcID uint64) *devconfig.NPCShop {
	f := &w.e.Store.Cfg.Fixture
	if !r.Ready || r.Player.HP <= 0 || scene != r.Scene || f.EquipmentDurability == nil {
		return nil
	}
	m, ok := w.currentMap(r)
	if !ok || m.SceneKindValue() != devconfig.SceneKindTown {
		return nil
	}
	for _, npc := range m.NPCs {
		if npc.ID == npcID && distance(npc.Position, r.Player.Pos) <= f.EquipmentDurability.RepairRangeCells {
			return f.NPCShopByRole(npc.Role)
		}
	}
	return nil
}

func (w *worker) vendorPreview(r *room, f wire.Frame) {
	if r.Pending || r.PreviewInFlight || r.PreviewGeneration == math.MaxUint64 {
		w.failure(r, f, p.CodeOperationPending)
		return
	}
	q := &pb.VendorQuoteRequest{}
	if proto.Unmarshal(f.Payload, q) != nil {
		w.failure(r, f, p.CodeInvalidArgument)
		return
	}
	shop := w.vendorShop(r, q.SceneEpoch, q.NpcId)
	if shop == nil {
		w.failure(r, f, p.CodeVendorUnavailable)
		return
	}
	r.PreviewGeneration++
	r.PreviewInFlight = true
	binding := skillPreviewBinding{room: r, conn: r.Conn, auth: r.Auth, scene: r.Scene, generation: r.PreviewGeneration}
	j := job{target: w, run: func() func(*worker) {
		result, err := w.e.Store.PreviewVendor(w.e.ctx, binding.auth, shop.Key, q)
		return func(w *worker) {
			if !w.claimPreview(binding) {
				return
			}
			if w.vendorShop(r, q.SceneEpoch, q.NpcId) != shop {
				w.failure(r, f, p.CodeVendorUnavailable)
			} else if err != nil {
				w.failure(r, f, persistence.Code(err))
			} else {
				w.send(r, p.MsgS2CVendorQuote, f.Request, result)
			}
		}
	}}
	if !w.submit(j) {
		r.PreviewInFlight = false
		w.failure(r, f, p.CodeServerBusy)
	}
}
