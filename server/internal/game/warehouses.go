package game

import (
	"bytes"
	"google.golang.org/protobuf/proto"
	"math"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
	"minimir/internal/wire"
	"time"
)

func (w *worker) warehouseQuery(r *room, f wire.Frame) {
	if r.PreviewInFlight {
		w.failure(r, f, p.CodeOperationPending)
		return
	}
	q := &pb.WarehouseQuery{}
	if proto.Unmarshal(f.Payload, q) != nil || len(q.ProtoReflect().GetUnknown()) != 0 {
		w.failure(r, f, p.CodeInvalidArgument)
		return
	}
	if r.PreviewGeneration == math.MaxUint64 {
		w.failure(r, f, p.CodeServerBusy)
		return
	}
	r.PreviewGeneration++
	r.PreviewInFlight = true
	b := skillPreviewBinding{room: r, conn: r.Conn, auth: r.Auth, scene: r.Scene, generation: r.PreviewGeneration}
	j := job{target: w, run: func() func(*worker) {
		page, err := w.e.Store.QueryWarehouse(w.e.ctx, b.auth, q)
		return func(w *worker) { w.finishWarehouseQuery(b, f, page, err) }
	}}
	if !w.submit(j) {
		r.PreviewInFlight = false
		w.failure(r, f, p.CodeServerBusy)
	}
}
func (w *worker) finishWarehouseQuery(b skillPreviewBinding, f wire.Frame, page *pb.WarehousePage, err error) {
	r := b.room
	if r.PreviewGeneration != b.generation || !r.PreviewInFlight {
		return
	}
	r.PreviewInFlight = false
	if w.rooms[b.auth.RoleID] != r || r.Auth != b.auth || r.Conn != b.conn || r.Scene != b.scene || r.Isolated || !r.Ready || b.conn == nil || !b.conn.Alive() || b.conn.Binding() != uint64(b.auth.SessionEpoch) {
		return
	}
	if err != nil {
		w.failure(r, f, persistence.Code(err))
		return
	}
	if page == nil || page.Role == nil || !bytes.Equal(page.Role.RoleId, r.Role.Summary().Key.RoleId) || page.Role.OriginServerId != r.Role.Origin || page.Scope == persistence.WarehouseShared && !r.Role.PrivilegeUntil.After(time.Now()) {
		w.failure(r, f, p.CodeWarehousePrivilegeRequired)
		return
	}
	// 以实际wire字节验最后边界，未来实例字段扩充也不能发送超长报文。
	if len(wire.Encode(wire.Message(p.MsgS2CWarehousePage, f.Request, b.conn.Binding(), page))) > wire.MaxFrame {
		w.failure(r, f, p.CodeServerBusy)
		return
	}
	w.send(r, p.MsgS2CWarehousePage, f.Request, page)
}
