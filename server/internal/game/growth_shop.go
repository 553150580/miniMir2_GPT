// 本文件从原角色房间提供商城目录、短期报价和可重放的购买请求。
package game

import (
	"time"

	"google.golang.org/protobuf/proto"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
	"minimir/internal/wire"
)

// queryGrowthShop 读目录不占资产槽，选具体商品时才签发本附着的唯一报价。
func (w *worker) queryGrowthShop(r *room, frame wire.Frame, at time.Time) {
	var request pb.GrowthShopQuery
	if proto.Unmarshal(frame.Payload, &request) != nil || len(request.ProtoReflect().GetUnknown()) != 0 || request.SceneEpoch != r.Scene ||
		(request.ProductId == 0) != (request.Count == 0) || request.Count > devconfig.GrowthShopMaxCount {
		w.failure(r, frame, p.CodeInvalidArgument)
		return
	}
	if !w.growthSceneAvailable(r) {
		w.failure(r, frame, p.CodeGrowthShopUnavailable)
		return
	}
	f := &w.roomStore(r).Cfg.Fixture
	state, code := persistence.GrowthShopCatalog(r.Role, f, r.Scene, at)
	if code != 0 {
		w.failure(r, frame, code)
		return
	}
	if request.ProductId != 0 {
		if r.Pending {
			w.failure(r, frame, p.CodeOperationPending)
			return
		}
		r.growthShopQuote = nil
		quote, failure := persistence.QuoteGrowthShop(r.Role, f, r.Scene, at, request.ProductId, request.Count)
		if failure != 0 {
			w.failure(r, frame, failure)
			return
		}
		r.growthShopQuote = &quote
		state.Quote = quote.Proto()
	}
	w.send(r, p.MsgS2CGrowthShopState, frame.Request, state)
}

// readGrowthShopCommit 共用首次提交和查回原账的消息形状。
func readGrowthShopCommit(frame wire.Frame) (*pb.GrowthShopCommitRequest, bool) {
	var req pb.GrowthShopCommitRequest
	if proto.Unmarshal(frame.Payload, &req) != nil {
		return nil, false
	}
	return &req, persistence.ValidGrowthShopCommit(&req, req.Operation)
}

// readGrowthShopAsset 只取原场景最后签发的报价。旧操作仍由事务先查原回执。
func (w *worker) readGrowthShopAsset(r *room, frame wire.Frame, asset *persistence.AssetRequest) bool {
	request, valid := readGrowthShopCommit(frame)
	if !valid {
		return false
	}
	asset.Kind, asset.Context, asset.GrowthShopCommit = persistence.AssetGrowthShopCommit, request.Operation, request
	asset.SceneEpoch = r.Scene
	asset.GrowthShopGate = historicalGrowthOperation(r, request.Operation) || !w.growthSceneAvailable(r)
	if quote := r.growthShopQuote; quote != nil && quote.ID == domain.Hex(request.QuoteId) && uint64(quote.Scene) == r.Scene {
		asset.GrowthShopQuote = quote
	}
	return true
}
