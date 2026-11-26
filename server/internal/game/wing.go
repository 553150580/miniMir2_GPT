// 本文件验证角色翅膀报价及提交的原附着，资产归属和扣羽毛由持久事务确认。
package game

import (
	"math"

	"google.golang.org/protobuf/proto"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
	"minimir/internal/wire"
)

// wingAvailable 对首次请求验证当前角色身体和运行服配置。
func (w *worker) wingAvailable(r *room) bool {
	return r.Ready && r.Player.HP > 0 && r.bossBattle == nil && w.roomStore(r).Cfg.Fixture.Wings != nil
}

// wingPreview 沿用当前角色有界的异步预览槽，只有原附着仍有效时才能返回报价。
func (w *worker) wingPreview(r *room, frame wire.Frame) {
	var request pb.WingQuoteRequest
	if proto.Unmarshal(frame.Payload, &request) != nil || request.SceneEpoch == 0 || request.SceneEpoch != r.Scene ||
		len(request.UiSessionId) != 16 || !validWingReference(request.WingId, request.WingVersion, true) ||
		len(request.ProtoReflect().GetUnknown()) != 0 {
		w.failure(r, frame, p.CodeInvalidArgument)
		return
	}
	if !w.wingAvailable(r) {
		w.failure(r, frame, p.CodeWingUnavailable)
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
		quote, err := store.PreviewWing(w.e.ctx, binding.auth, &request)
		return func(owner *worker) {
			if !owner.claimPreview(binding) {
				return
			}
			if !owner.wingAvailable(r) {
				owner.failure(r, frame, p.CodeWingUnavailable)
				return
			}
			if err != nil {
				owner.failure(r, frame, persistence.Code(err))
				return
			}
			owner.send(r, p.MsgS2CWingQuote, frame.Request, quote)
		}
	}}
	if !w.submit(work) {
		r.PreviewInFlight = false
		w.failure(r, frame, p.CodeServerBusy)
	}
}

// validWingReference 激活时允许空引用，其余修改须指向已存在的原实例及版本。
func validWingReference(id []byte, version uint64, mayActivate bool) bool {
	return len(id) == 16 && version > 0 && version < math.MaxInt64 || mayActivate && len(id) == 0 && version == 0
}

// readWingAction 只接收当前定义的成长或外观请求形状，原归属和实际扣费继续由存储检查。
func readWingAction(frame wire.Frame) (*pb.WingActionRequest, bool) {
	var request pb.WingActionRequest
	if proto.Unmarshal(frame.Payload, &request) != nil || len(request.ProtoReflect().GetUnknown()) != 0 {
		return nil, false
	}
	switch request.Action {
	case 1:
		return &request, len(request.QuoteId) == 16 && len(request.UiSessionId) == 16 && request.AppearanceTier == 0 && !request.Hidden &&
			validWingReference(request.WingId, request.WingVersion, true)
	case 2:
		return &request, len(request.QuoteId) == 0 && (len(request.UiSessionId) == 0 || len(request.UiSessionId) == 16) && !request.AllowBound &&
			request.AppearanceTier > 0 && validWingReference(request.WingId, request.WingVersion, false)
	default:
		return nil, false
	}
}

// roleWingLuck 只用角色自己已确认的永久翅膀。当前外观和隐藏开关不参与战斗数值。
func roleWingLuck(role domain.Role, fixture *devconfig.Fixture) uint32 {
	wing := role.Wing
	if fixture.Wings == nil || wing == nil || wing.ID == "" || wing.OwnerRoleID != role.ID || wing.Rank == 0 || wing.Version <= 0 {
		return 0
	}
	return wing.Luck
}
