// 本文件经数据库执行仓库分页查询，仅向原场景与原附着返还有效结果。
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

// warehouseQuery 处理仓库查询。
// w 是调用该方法的房间执行器实例，r 指向当前执行器拥有并更新的目标房间，f 记录协议帧。
func (w *worker) warehouseQuery(r *room, f wire.Frame) {
	if r.PreviewInFlight {
		w.failure(r, f, p.CodeOperationPending)
		return
	}
	// q 保存当前处理所用的仓库查询。
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
	// b 建立带初始值的技能预览会话绑定，供本段继续处理。
	b := skillPreviewBinding{room: r, conn: r.Conn, auth: r.Auth, scene: r.Scene, generation: r.PreviewGeneration}
	// store 按当前房间的运行服授权读取独立或共享仓库。
	store := w.roomStore(r)
	// j 初始化用作当前位置的下标的任务或职业编号。
	j := job{target: w, run: func() func(*worker) {
		// page 保存由查询仓库取得的页面或分页，err 接收查询仓库过程中可能产生的错误，供后续失败分支处理。
		page, err := store.QueryWarehouse(w.e.ctx, b.auth, q)
		// w 记录房间执行器。
		return func(w *worker) { w.finishWarehouseQuery(b, f, page, err) }
	}}
	if !w.submit(j) {
		r.PreviewInFlight = false
		w.failure(r, f, p.CodeServerBusy)
	}
}

// finishWarehouseQuery 完成仓库查询。
// w 是调用该方法的房间执行器实例，b 记录b，使用技能预览会话绑定表示，f 记录协议帧，page 记录页面或分页，使用仓库页面或分页表示，err 保存当前操作的失败原因，供调用方检查或返回。
func (w *worker) finishWarehouseQuery(b skillPreviewBinding, f wire.Frame, page *pb.WarehousePage, err error) {
	// r 保存当前处理所用的房间。
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
