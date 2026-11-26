// 本文件向原角色附着返回道具市场和金币市场分页。
package game

import (
	"math"

	"google.golang.org/protobuf/proto"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
	"minimir/internal/wire"
)

// queryItemMarket 复用有界只读槽，结果仅返回同一连接和场景。
func (w *worker) queryItemMarket(r *room, frame wire.Frame) {
	var q pb.ItemMarketQuery
	if proto.Unmarshal(frame.Payload, &q) != nil || q.SceneEpoch != r.Scene || len(q.ProtoReflect().GetUnknown()) != 0 {
		w.failure(r, frame, p.CodeInvalidArgument)
		return
	}
	store, auth := w.roomStore(r), r.Auth
	w.submitMarketQuery(r, frame, p.MsgS2CItemMarketState, func() (proto.Message, error) {
		return store.QueryItemMarket(w.e.ctx, auth, &q)
	})
}

// queryGoldMarket 和道具市场共用同一原角色读槽。
func (w *worker) queryGoldMarket(r *room, frame wire.Frame) {
	var q pb.GoldMarketQuery
	if proto.Unmarshal(frame.Payload, &q) != nil || q.SceneEpoch != r.Scene || len(q.ProtoReflect().GetUnknown()) != 0 {
		w.failure(r, frame, p.CodeInvalidArgument)
		return
	}
	store, auth := w.roomStore(r), r.Auth
	w.submitMarketQuery(r, frame, p.MsgS2CGoldMarketState, func() (proto.Message, error) {
		return store.QueryGoldMarket(w.e.ctx, auth, &q)
	})
}

// submitMarketQuery 只向发起查询的原连接和场景交付结果。
func (w *worker) submitMarketQuery(r *room, frame wire.Frame, response uint16, read func() (proto.Message, error)) {
	if r.PreviewInFlight || r.PreviewGeneration == math.MaxUint64 {
		w.failure(r, frame, p.CodeOperationPending)
		return
	}
	r.PreviewGeneration++
	r.PreviewInFlight = true
	binding := skillPreviewBinding{room: r, conn: r.Conn, auth: r.Auth, scene: r.Scene, generation: r.PreviewGeneration}
	if !w.submit(job{target: w, run: func() func(*worker) {
		state, err := read()
		return func(owner *worker) {
			owner.finishMarketQuery(binding, frame, response, state, err)
		}
	}}) {
		r.PreviewInFlight = false
		w.failure(r, frame, p.CodeServerBusy)
	}
}

func (w *worker) finishMarketQuery(binding skillPreviewBinding, frame wire.Frame, response uint16, state proto.Message, err error) {
	r := binding.room
	if r.PreviewGeneration != binding.generation || !r.PreviewInFlight {
		return
	}
	r.PreviewInFlight = false
	if w.rooms[binding.auth.RoleID] != r || r.Auth != binding.auth || r.Conn != binding.conn || r.Scene != binding.scene ||
		r.Isolated || !r.Ready || binding.conn == nil || !binding.conn.Alive() || binding.conn.Binding() != uint64(binding.auth.SessionEpoch) {
		return
	}
	if err != nil {
		w.failure(r, frame, persistence.Code(err))
		return
	}
	if state == nil || !state.ProtoReflect().IsValid() || len(wire.Encode(wire.Message(response, frame.Request, binding.conn.Binding(), state))) > wire.MaxFrame {
		w.failure(r, frame, p.CodeServerBusy)
		return
	}
	w.send(r, response, frame.Request, state)
}
