// 本文件在原角色房间查询双卡状态、签发确认报价，并从原事务回执安装权益。
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

// queryRoleCards 仅从当前运行服规则和已确认角色资产读取双卡状态。
func (w *worker) queryRoleCards(r *room, frame wire.Frame, now time.Time) {
	var request pb.RoleCardQuery // 商品为零时只读目录，非零时才生成报价。
	if proto.Unmarshal(frame.Payload, &request) != nil || request.SceneEpoch != r.Scene ||
		len(request.ProtoReflect().GetUnknown()) != 0 {
		w.failure(r, frame, p.CodeInvalidArgument)
		return
	}
	fixture := &w.roomStore(r).Cfg.Fixture
	state := &pb.RoleCardState{
		SceneEpoch: r.Scene, RoleRevision: uint64(r.Role.Revision),
		PrivilegeUntilUnixMs: roleCardTime(r.Role.PrivilegeUntil),
		HostingUntilUnixMs:   roleCardTime(r.Role.HostingCardUntil),
		OperatingDay:         persistence.RoleCardOperatingDay(now),
		PrivilegeClaimable:   persistence.CanClaimRoleCardDaily(r.Role, fixture, now),
	}
	if cards := fixture.RoleCards; cards != nil {
		state.DailyPrivilegeUnits = cards.DailyPrivilegeUnits
		for _, product := range cards.Products {
			state.Products = append(state.Products, &pb.RoleCardProductView{
				ProductId: product.ID, Kind: product.Kind, Days: product.Days, PriceYuanbaoUnits: product.PriceYuanbaoUnits,
			})
		}
	}
	if request.ProductId != 0 {
		state.Quote, state.ErrorCode = r.issueRoleCardQuote(fixture, request.ProductId, now)
	}
	w.send(r, p.MsgS2CRoleCardState, frame.Request, state)
}

// issueRoleCardQuote 在当前附着只保留最后一份可成交条款。
func (r *room) issueRoleCardQuote(fixture *devconfig.Fixture, productID uint32, now time.Time) (*pb.RoleCardQuoteView, uint32) {
	if r.Pending {
		return nil, p.CodeOperationPending
	}
	quote, code := persistence.QuoteRoleCard(r.Role, fixture, productID, now)
	if code != 0 {
		return nil, code
	}
	quote.QuoteID = domain.ID()
	quote.SceneEpoch = int64(r.Scene)
	quote.ExpiresAt = now.Add(30 * time.Second).UTC().Truncate(time.Millisecond)
	if quote.PreviousUntil.After(now) && quote.PreviousUntil.Before(quote.ExpiresAt) {
		quote.ExpiresAt = quote.PreviousUntil // 原期跨过到期点后，购买结果须重新确认。
	}
	r.roleCardQuote = &quote
	return quote.Proto(), 0
}

// readRoleCardAsset 验证客户端的原报价编号或上海日，永久操作仍交给角色事务核实。
func (r *room) readRoleCardAsset(frame wire.Frame, asset *persistence.AssetRequest, acceptedAt time.Time) bool {
	if frame.ID == p.MsgC2SBuyRoleCard {
		var request pb.BuyRoleCardRequest
		if proto.Unmarshal(frame.Payload, &request) != nil || request.ProductId == 0 || len(request.QuoteId) != 16 ||
			len(request.ProtoReflect().GetUnknown()) != 0 {
			return false
		}
		asset.Kind, asset.Context, asset.SceneEpoch = persistence.AssetBuyRoleCard, request.Operation, r.Scene
		if quote := r.roleCardQuote; quote != nil && uint64(quote.SceneEpoch) == r.Scene &&
			quote.ProductID == request.ProductId && quote.QuoteID == domain.Hex(request.QuoteId) {
			asset.CardPurchase = quote
		}
		// 缺失报价的恢复请求仍须进事务核对原操作是否已成交。
		return true
	}
	var request pb.ClaimRoleCardDailyRequest
	if proto.Unmarshal(frame.Payload, &request) != nil || len(request.OperatingDay) != len("2006-01-02") ||
		len(request.ProtoReflect().GetUnknown()) != 0 {
		return false
	}
	asset.Kind, asset.Context = persistence.AssetClaimRoleCardDaily, request.Operation
	asset.CardDay, asset.CardAcceptedAt = request.OperatingDay, acceptedAt
	return true
}

// acceptRoleCardReceipt 仅安装本次首次观察到的新卡期，并让原命中沿确认时刻判定。
func (r *room) acceptRoleCardReceipt(next domain.Role, op persistence.Operation, now time.Time) time.Time {
	receipt := op.CardPurchase
	if receipt == nil || next.Revision <= r.Role.Revision || next.Revision != op.Revision {
		return now
	}
	return r.observeRoleCardChange(receipt.Quote.Kind, receipt.PreviousSince, receipt.PreviousUntil, receipt.EffectiveAt, now)
}

// observeRoleCardChange 保留卡期变更前尚未处理的命中资格，游戏内购买和核验后的现金卡共用。
func (r *room) observeRoleCardChange(kind uint32, previousSince, previousUntil, effectiveAt, now time.Time) time.Time {
	if kind == devconfig.RoleCardKindHosting {
		if earliest := r.earliestHostingCardEvent(effectiveAt); earliest.Before(effectiveAt) {
			r.hostingCardHistory = append(r.hostingCardHistory, hostingCardChange{
				at: effectiveAt, since: previousSince, until: previousUntil,
			})
		}
		return now
	}
	return effectiveAt
}

// roleCardTime 以协议零值标记从未激活。
func roleCardTime(at time.Time) int64 {
	if at.IsZero() {
		return 0
	}
	return at.UnixMilli()
}
