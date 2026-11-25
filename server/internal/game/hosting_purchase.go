// 本文件为当前角色签发短时补时报价，并将原确认事实交给资产事务。
package game

import (
	"time"

	"google.golang.org/protobuf/proto"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/domain"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
	"minimir/internal/wire"
)

// hostingQuoteTTL 限制确认页报价的有效时间；价格版本提前到期时使用更早的截止时刻。
const hostingQuoteTTL = 30 * time.Second

// quoteHostingPurchase 只读房间内已确认的角色资产；购买时由持久层重新核对。
func (w *worker) quoteHostingPurchase(r *room, frame wire.Frame, now time.Time) {
	var request pb.HostingQuoteRequest // 当前场景与玩家选择的补时秒数。
	if proto.Unmarshal(frame.Payload, &request) != nil || request.SceneEpoch != r.Scene || len(request.ProtoReflect().GetUnknown()) != 0 {
		w.failure(r, frame, p.CodeInvalidArgument)
		return
	}
	if r.Pending && request.RequestedSeconds != 0 {
		w.failure(r, frame, p.CodeOperationPending)
		return
	}
	fixture := &w.roomStore(r).Cfg.Fixture
	quote, code := persistence.QuoteHostingTime(r.Role, fixture, request.RequestedSeconds, now)
	quote.SceneEpoch, quote.RoleRevision = int64(r.Scene), uint64(r.Role.Revision)
	quote.RuleVersion, quote.ConfigHash = fixture.RuleVersion, fixture.Hash
	if request.RequestedSeconds == 0 {
		quote.RequestedSeconds = 0
	} else if code == 0 {
		quote.QuoteID = domain.ID()
		quote.ExpiresAt = now.Add(hostingQuoteTTL).UTC().Truncate(time.Millisecond)
		if !quote.EffectiveUntil.IsZero() && quote.EffectiveUntil.Before(quote.ExpiresAt) {
			quote.ExpiresAt = quote.EffectiveUntil
		}
		r.hostingQuote = &quote
	}
	result := quote.Proto()
	result.ErrorCode = code
	w.send(r, p.MsgS2CHostingQuote, frame.Request, result)
}

// hostingPurchaseTerms 仅交出本场景的原报价；缺失时仍允许事务查找已提交的同一操作。
func (r *room) hostingPurchaseTerms(request *pb.BuyHostingTimeRequest) *persistence.HostingPurchaseTerms {
	quote := r.hostingQuote
	if quote == nil || uint64(quote.SceneEpoch) != r.Scene || quote.QuoteID != domain.Hex(request.QuoteId) || quote.RequestedSeconds != request.RequestedSeconds {
		return nil
	}
	return &persistence.HostingPurchaseTerms{Quote: *quote}
}
