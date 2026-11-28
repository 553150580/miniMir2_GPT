// 本文件从原角色场景签发转生报价，并核对原升级产生的一次资源恢复。
package game

import (
	"math"

	"google.golang.org/protobuf/proto"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
	"minimir/internal/wire"
)

// rebirthAvailable 对首次转生校验当前玩法开关和所在地图。
func (w *worker) rebirthAvailable(r *room) bool {
	return w.roomStore(r).Cfg.Fixture.Progression.RebirthEnabled && w.growthSceneAvailable(r)
}

// rebirthPreview 复用原报价任务槽，换图或换连接时不发送旧面板的报价。
func (w *worker) rebirthPreview(r *room, frame wire.Frame) {
	var request pb.RebirthQuoteRequest
	if proto.Unmarshal(frame.Payload, &request) != nil || !persistence.ValidRebirthQuoteRequest(&request) || request.SceneEpoch != r.Scene {
		w.failure(r, frame, p.CodeInvalidArgument)
		return
	}
	if !w.rebirthAvailable(r) {
		w.failure(r, frame, p.CodeRebirthUnavailable)
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
	if !w.submit(job{target: w, run: func() func(*worker) {
		quote, err := store.PreviewRebirth(w.e.ctx, binding.auth, &request)
		return func(owner *worker) {
			if !owner.claimPreview(binding) {
				return
			}
			if !owner.rebirthAvailable(r) {
				owner.failure(r, frame, p.CodeRebirthUnavailable)
				return
			}
			if err != nil {
				owner.failure(r, frame, persistence.Code(err))
				return
			}
			owner.send(r, p.MsgS2CRebirthQuote, frame.Request, quote)
		}
	}}) {
		r.PreviewInFlight = false
		w.failure(r, frame, p.CodeServerBusy)
	}
}

// readRebirthCommit 解析首次确认和历史回执重放共同使用的原指令。
func readRebirthCommit(frame wire.Frame) (*pb.RebirthCommitRequest, bool) {
	var request pb.RebirthCommitRequest
	if proto.Unmarshal(frame.Payload, &request) != nil {
		return nil, false
	}
	return &request, persistence.ValidRebirthCommit(&request, request.Operation)
}

// applyRebirthRecovery 遵循普通升级的原出生恢复规则，换场、死亡和历史重放不会恢复生命。
func (w *worker) applyRebirthRecovery(r *room, b assetBinding, receipt persistence.Operation) {
	if receipt.Rebirth == nil || receipt.Revision != r.Role.Revision || receipt.CommandSeq <= b.commandBefore ||
		receipt.Rebirth.Plan.Transition.After.Level <= receipt.Rebirth.Plan.Transition.Before.Level {
		return
	}
	body := w.resolve(r, b.progressionActor)
	if body == &r.Player && body.HP > 0 && r.world(b.progressionActor.World) == r.currentWorld() {
		body.HP, body.MP = body.MaxHP, body.MaxMP
	}
}
