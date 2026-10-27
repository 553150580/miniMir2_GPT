package game

import (
	"google.golang.org/protobuf/proto"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
	"minimir/internal/wire"
)

type skillPreviewBinding struct {
	room       *room
	conn       Connection
	auth       persistence.Auth
	scene      uint64
	generation uint64
}

func (w *worker) skillAssetPreview(r *room, f wire.Frame) {
	if r.Pending || r.PreviewInFlight {
		w.failure(r, f, p.CodeOperationPending)
		return
	}
	q := &pb.SkillAssetPreviewRequest{}
	if proto.Unmarshal(f.Payload, q) != nil {
		w.failure(r, f, p.CodeInvalidArgument)
		return
	}
	// 报价只保留有界的异步槽，不占用冻结临时战斗的永久变更标记。
	r.PreviewGeneration++
	r.PreviewInFlight = true
	binding := skillPreviewBinding{room: r, conn: r.Conn, auth: r.Auth, scene: r.Scene, generation: r.PreviewGeneration}
	j := job{target: w, run: func() func(*worker) {
		result, err := w.e.Store.PreviewSkillAsset(w.e.ctx, binding.auth, q)
		return func(w *worker) {
			w.finishSkillAssetPreview(binding, f, result, err)
		}
	}}
	if !w.submit(j) {
		r.PreviewInFlight = false
		w.failure(r, f, p.CodeServerBusy)
	}
}

func (w *worker) finishSkillAssetPreview(binding skillPreviewBinding, f wire.Frame, result *pb.SkillAssetPreview, err error) {
	r := binding.room
	if r.PreviewGeneration != binding.generation || !r.PreviewInFlight {
		return
	}
	// 只释放本代槽位；新连接或新请求建立的槽位不受旧回调影响。
	r.PreviewInFlight = false
	if w.rooms[binding.auth.RoleID] != r || r.Conn != binding.conn || r.Auth != binding.auth || r.Scene != binding.scene || !r.Ready || binding.conn == nil || !binding.conn.Alive() || binding.conn.Binding() != uint64(binding.auth.SessionEpoch) {
		return
	}
	if err != nil {
		w.failure(r, f, persistence.Code(err))
	} else {
		w.send(r, p.MsgS2CSkillAssetPreview, f.Request, result)
	}
}
