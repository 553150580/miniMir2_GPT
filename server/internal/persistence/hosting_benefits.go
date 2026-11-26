// 本文件计算角色当日托管时长领取差额，并固定原领取的对账明细。
package persistence

import (
	"time"

	"minimir/internal/devconfig"
	"minimir/internal/domain"
	p "minimir/internal/protocol"
)

// AssetClaimHostingTime 标识日领托管时间的经济操作。
const AssetClaimHostingTime = "claim_hosting"

// HostingClaimReceipt 保存原领取所在的运营日、当时权益和实际到账。
type HostingClaimReceipt struct {
	OperatingDay      string    `bson:"operating_day"`       // 受理时刻所属的上海运营日。
	AcceptedAt        time.Time `bson:"accepted_at"`         // 服务端受理原领取的时刻。
	DailyLimitSeconds int64     `bson:"daily_limit_seconds"` // 按受理时有效权益得到的当日额度。
	GrantedSeconds    int64     `bson:"granted_seconds"`     // 本次实际增加的托管秒数。
	ClaimedSeconds    int64     `bson:"claimed_seconds"`     // 本次完成后的当日累计已领秒数。
	BalanceSeconds    int64     `bson:"balance_seconds"`     // 本次完成后的累计托管余额。
	CardActive        bool      `bson:"card_active"`         // 原领取受理时托管卡是否有效。
}

// applyHostingClaim 用固定的受理时刻为原角色补足当日额度，拒绝迟来的旧日请求。
// r 是未变更的原角色，kind 是现有协议领取类型，at 是服务端时钟，rules 是当前投影的权益配置。
func applyHostingClaim(r domain.Role, kind uint32, at time.Time, rules *devconfig.HostingBenefits) (domain.Role, *HostingClaimReceipt, uint32, error) {
	if kind != 0 && kind != 1 {
		return r, nil, p.CodeInvalidArgument, nil
	}
	// day 是原请求所属运营日；不能用迟来的旧日额度覆盖较新的水位。
	day := shanghaiOperatingDay(at)
	if r.ClaimDay > day || r.ClaimedSeconds < 0 || r.HostingSeconds < 0 {
		return r, nil, p.CodeOperationConflict, nil
	}
	// free、card 是已定日领总额度，旧配置没有权益表时沿用历史运行口径。
	free, card := int64(6*3600), int64(12*3600)
	if rules != nil {
		free, card = rules.FreeDailySeconds, rules.CardDailySeconds
	}
	// active 和 limit 固定原受理时的权益，到期时刻本身已无卡权益。
	active, limit := r.HostingCardActiveAt(at), free
	if active {
		limit = card
	}
	// claimed 是原运营日已经领取的额度，新的一天从零计算。
	claimed := int64(0)
	if r.ClaimDay == day {
		claimed = r.ClaimedSeconds
	}
	// grant 是实际尚可补足的秒数，已领余额不会随到期降低。
	grant := limit - claimed
	if grant <= 0 {
		return r, nil, p.CodeOperationConflict, nil
	}
	// balance 是已领余额和本次增量的安全求和。
	balance, err := add(r.HostingSeconds, grant)
	if err != nil {
		return r, nil, 0, err
	}
	r.ClaimDay, r.ClaimedSeconds, r.HostingSeconds = day, limit, balance
	return r, &HostingClaimReceipt{
		OperatingDay: day, AcceptedAt: at, DailyLimitSeconds: limit, GrantedSeconds: grant,
		ClaimedSeconds: r.ClaimedSeconds, BalanceSeconds: balance, CardActive: active,
	}, 0, nil
}
