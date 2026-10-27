package game

import (
	"google.golang.org/protobuf/proto"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
	"minimir/internal/wire"
)

func (w *worker) skillAssetPreview(r *room, f wire.Frame) {
	if r.Pending {
		w.failure(r, f, p.CodeOperationPending)
		return
	}
	q := &pb.SkillAssetPreviewRequest{}
	if proto.Unmarshal(f.Payload, q) != nil {
		w.failure(r, f, p.CodeInvalidArgument)
		return
	}
	r.Pending = true
	auth := r.Auth
	j := job{target: w, run: func() func(*worker) {
		result, err := w.e.Store.PreviewSkillAsset(w.e.ctx, auth, q)
		return func(w *worker) {
			r.Pending = false
			if err != nil {
				w.failure(r, f, persistence.Code(err))
			} else {
				w.send(r, p.MsgS2CSkillAssetPreview, f.Request, result)
			}
			w.after(r)
		}
	}}
	if !w.submit(j) {
		r.Pending = false
		w.failure(r, f, p.CodeServerBusy)
	}
}
