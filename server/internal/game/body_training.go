// 本文件受理普通场景中的炼体报价和单次经济确认。
package game

import (
	"context"

	"google.golang.org/protobuf/proto"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
	"minimir/internal/wire"
)

// bodyTrainingAvailable 校验场景与当前规则开放状态。
func (w *worker) bodyTrainingAvailable(r *room) bool {
	return w.roomStore(r).Cfg.Fixture.BodyTraining != nil && w.growthSceneAvailable(r)
}

// bodyTrainingPreview 核对原系别和场景后进入角色报价槽。
func (w *worker) bodyTrainingPreview(r *room, frame wire.Frame) {
	var request pb.BodyTrainingQuoteRequest
	if proto.Unmarshal(frame.Payload, &request) != nil || !persistence.ValidBodyTrainingQuote(&request) || request.SceneEpoch != r.Scene {
		w.failure(r, frame, p.CodeInvalidArgument)
		return
	}
	w.enqueueGrowthPreview(r, frame, p.MsgS2CBodyTrainingQuote, p.CodeBodyTrainingUnavailable, w.bodyTrainingAvailable,
		func(ctx context.Context, store *persistence.Store, auth persistence.Auth) (proto.Message, error) {
			return store.PreviewBodyTraining(ctx, auth, &request)
		})
}

// readBodyTrainingCommit 读取首次尝试和重连重放共用的原指令。
func readBodyTrainingCommit(frame wire.Frame) (*pb.BodyTrainingCommitRequest, bool) {
	var request pb.BodyTrainingCommitRequest
	if proto.Unmarshal(frame.Payload, &request) != nil {
		return nil, false
	}
	return &request, persistence.ValidBodyTrainingCommit(&request, request.Operation)
}
