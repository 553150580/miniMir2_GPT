// 本文件生成双月卡的服务端报价并确认游戏内购买与特权每日领取。
package persistence

import (
	"context"
	"math"
	"time"

	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	p "minimir/internal/protocol"
)

const (
	AssetBuyRoleCard        = "buy_role_card"         // 从本角色非绑定元宝钱包购买或续期一张月卡。
	AssetClaimRoleCardDaily = "claim_role_card_daily" // 领取本角色本上海日的特权绑元奖励。
)

// RoleCardQuote 保存原角色分片签发的购买条款和可重放的配置身份。
type RoleCardQuote struct {
	QuoteID           string    `bson:"quote_id"`                 // 报价的随机身份。
	SceneEpoch        int64     `bson:"scene_epoch"`              // 原场景 uint64 的位模式，以 BSON 有符号整数保存。
	RoleID            string    `bson:"role_id"`                  // 获得报价的永久角色身份。
	Origin            uint64    `bson:"origin_server_id"`         // 原角色永久入口服。
	RoleRevision      uint64    `bson:"role_revision"`            // 发出报价时的角色修订。
	RuleVersion       uint64    `bson:"rule_version"`             // 原经济规则版本。
	ConfigHash        string    `bson:"config_hash"`              // 原报价引用的完整快照。
	QuotedAt          time.Time `bson:"quoted_at"`                // 服务端生成条款的时刻。
	ExpiresAt         time.Time `bson:"expires_at"`               // 仅允许此时刻之前的新购买。
	ProductID         uint32    `bson:"product_id"`               // 唯一玩法配置的产品编号。
	Kind              uint32    `bson:"kind"`                     // 特权或托管卡种。
	Days              uint32    `bson:"days"`                     // 获得的月卡天数。
	PriceYuanbaoUnits int64     `bson:"price_yuanbao_units"`      // 以非绑定元宝最小单位计的全额价格。
	PreviousSince     time.Time `bson:"previous_since,omitempty"` // 确认页对应的原连续期起点。
	PreviousUntil     time.Time `bson:"previous_until,omitempty"` // 确认页对应的原卡到期时刻。
}

// RoleCardPurchaseReceipt 固定首次确认的扣款、生效和续期结果。
type RoleCardPurchaseReceipt struct {
	Quote               RoleCardQuote `bson:"quote"`                    // 本次按其成交的原报价。
	EffectiveAt         time.Time     `bson:"effective_at"`             // 权益变更起效的服务端时刻。
	PreviousSince       time.Time     `bson:"previous_since,omitempty"` // 原连续期起点。
	PreviousUntil       time.Time     `bson:"previous_until,omitempty"` // 原连续期终点。
	ValidSince          time.Time     `bson:"valid_since,omitempty"`    // 更新后的连续期起点。
	ValidUntil          time.Time     `bson:"valid_until"`              // 更新后的连续期终点。
	BalanceYuanbaoUnits int64         `bson:"balance_yuanbao_units"`    // 该次扣款后的非绑元宝余额。
}

// RoleCardDailyReceipt 固定原领取的上海日、特权期限、奖额与绑定钱包。
type RoleCardDailyReceipt struct {
	OperatingDay             string    `bson:"operating_day"`               // 原请求受理的上海运营日。
	AcceptedAt               time.Time `bson:"accepted_at"`                 // 原请求的服务端受理时钟。
	PrivilegeUntil           time.Time `bson:"privilege_until"`             // 原运营日领取时的特权期限。
	GrantedBoundYuanbaoUnits int64     `bson:"granted_bound_yuanbao_units"` // 首次到账的绑定元宝最小单位。
	BalanceBoundYuanbaoUnits int64     `bson:"balance_bound_yuanbao_units"` // 该次领取后的绑定元宝余额。
}

// Proto 投影客户端确认页所需的原报价。
func (q RoleCardQuote) Proto() *pb.RoleCardQuoteView {
	return &pb.RoleCardQuoteView{QuoteId: domain.Bytes(q.QuoteID),
		Product:     &pb.RoleCardProductView{ProductId: q.ProductID, Kind: q.Kind, Days: q.Days, PriceYuanbaoUnits: q.PriceYuanbaoUnits},
		RuleVersion: q.RuleVersion, ConfigHash: q.ConfigHash,
		ExpiresAtUnixMs: cardUnixMillis(q.ExpiresAt), PreviousUntilUnixMs: cardUnixMillis(q.PreviousUntil)}
}

// Proto 还原首个已确认的原购买回执。
func (r *RoleCardPurchaseReceipt) Proto() *pb.RoleCardPurchaseReceipt {
	if r == nil {
		return nil
	}
	return &pb.RoleCardPurchaseReceipt{Quote: r.Quote.Proto(), EffectiveAtUnixMs: r.EffectiveAt.UnixMilli(),
		NewUntilUnixMs: r.ValidUntil.UnixMilli(), YuanbaoBalanceUnits: r.BalanceYuanbaoUnits}
}

// Proto 还原首个已确认的原每日领奖回执。
func (r *RoleCardDailyReceipt) Proto() *pb.RoleCardDailyReceipt {
	if r == nil {
		return nil
	}
	return &pb.RoleCardDailyReceipt{OperatingDay: r.OperatingDay, AcceptedAtUnixMs: r.AcceptedAt.UnixMilli(),
		GrantedUnits: r.GrantedBoundYuanbaoUnits, BoundYuanbaoBalanceUnits: r.BalanceBoundYuanbaoUnits}
}

// QuoteRoleCard 按可信产品目录、角色及其当前卡期生成元宝购买条款。
func QuoteRoleCard(r domain.Role, f *devconfig.Fixture, productID uint32, at time.Time) (RoleCardQuote, uint32) {
	if productID == 0 || at.IsZero() {
		return RoleCardQuote{}, p.CodeInvalidArgument
	}
	product, ok := f.RoleCardProduct(productID)
	if !ok || product.Kind != devconfig.RoleCardKindPrivilege && product.Kind != devconfig.RoleCardKindHosting ||
		product.Days != 30 || product.PriceYuanbaoUnits <= 0 {
		return RoleCardQuote{}, p.CodeLineForbidden
	}
	since, until := roleCardPeriod(r, product.Kind)
	quote := RoleCardQuote{RoleID: r.ID, Origin: r.Origin, RoleRevision: uint64(r.Revision),
		RuleVersion: f.RuleVersion, ConfigHash: f.Hash, QuotedAt: bossTime(at), ProductID: product.ID,
		Kind: product.Kind, Days: product.Days, PriceYuanbaoUnits: product.PriceYuanbaoUnits, PreviousSince: since, PreviousUntil: until}
	if r.Wallet.Yuanbao < product.PriceYuanbaoUnits {
		return quote, p.CodeVendorFunds
	}
	return quote, 0
}

// applyRoleCardPurchase 用事务时刻复核原报价，购买、续期均只扣本角色非绑定元宝。
func applyRoleCardPurchase(r domain.Role, f *devconfig.Fixture, trusted *RoleCardQuote, scene uint64, now time.Time) (domain.Role, *RoleCardPurchaseReceipt, uint32) {
	now = bossTime(now)
	if trusted == nil || !lowerHex(trusted.QuoteID, 16) || scene == 0 ||
		uint64(trusted.SceneEpoch) != scene || trusted.RoleID != r.ID || trusted.Origin != r.Origin ||
		trusted.RoleRevision == 0 || trusted.RoleRevision > uint64(r.Revision) || trusted.QuotedAt.IsZero() ||
		now.Before(trusted.QuotedAt) || !now.Before(bossTime(trusted.ExpiresAt)) {
		return r, nil, p.CodeOperationConflict
	}
	quote, code := QuoteRoleCard(r, f, trusted.ProductID, now)
	if code != 0 && code != p.CodeVendorFunds {
		return r, nil, code
	}
	if trusted.RuleVersion != quote.RuleVersion || trusted.ConfigHash != quote.ConfigHash || trusted.Kind != quote.Kind || trusted.Days != quote.Days ||
		trusted.PriceYuanbaoUnits != quote.PriceYuanbaoUnits || !trusted.PreviousSince.Equal(quote.PreviousSince) || !trusted.PreviousUntil.Equal(quote.PreviousUntil) ||
		quote.PreviousSince.After(now) || trusted.PreviousUntil.After(trusted.QuotedAt) != quote.PreviousUntil.After(now) {
		return r, nil, p.CodeOperationConflict
	}
	if code != 0 {
		return r, nil, code
	}
	since, until := now, now
	if quote.PreviousUntil.After(now) {
		since, until = quote.PreviousSince, quote.PreviousUntil
	}
	length := time.Duration(quote.Days) * 24 * time.Hour
	if until.UnixMilli() > math.MaxInt64-length.Milliseconds() {
		return r, nil, p.CodeInvalidArgument
	}
	until = until.Add(length)
	if quote.Kind == devconfig.RoleCardKindPrivilege {
		r.PrivilegeSince, r.PrivilegeUntil = since, until
	} else {
		r.HostingCardSince, r.HostingCardUntil = since, until
	}
	r.Wallet.Yuanbao -= quote.PriceYuanbaoUnits
	original := *trusted
	original.QuotedAt, original.ExpiresAt = bossTime(trusted.QuotedAt), bossTime(trusted.ExpiresAt)
	original.PreviousSince, original.PreviousUntil = bossTime(trusted.PreviousSince), bossTime(trusted.PreviousUntil)
	return r, &RoleCardPurchaseReceipt{Quote: original, EffectiveAt: now,
		PreviousSince: quote.PreviousSince, PreviousUntil: quote.PreviousUntil,
		ValidSince: since, ValidUntil: until, BalanceYuanbaoUnits: r.Wallet.Yuanbao}, 0
}

// RoleCardOperatingDay 返回现有全项目上海日口径。
func RoleCardOperatingDay(at time.Time) string {
	return shanghaiOperatingDay(at)
}

// CanClaimRoleCardDaily 为当前角色投影当日领取资格。
func CanClaimRoleCardDaily(r domain.Role, f *devconfig.Fixture, at time.Time) bool {
	return f != nil && f.RoleCards != nil && f.RoleCards.DailyPrivilegeUnits > 0 && r.PrivilegeActiveAt(at) && r.PrivilegeClaimDay < RoleCardOperatingDay(at)
}

// applyRoleCardDaily 同时核对原上海日水位和该时刻有效的特权，只有首次领取才增加绑元。
func (s *Store) applyRoleCardDaily(ctx context.Context, r domain.Role, day string, at time.Time) (domain.Role, *RoleCardDailyReceipt, uint32, error) {
	rules := s.Cfg.Fixture.RoleCards
	if rules == nil || rules.DailyPrivilegeUnits <= 0 {
		return r, nil, p.CodeLineForbidden, nil
	}
	if at.IsZero() || at.After(time.Now()) || day != RoleCardOperatingDay(at) || r.PrivilegeClaimDay >= day {
		return r, nil, p.CodeOperationConflict, nil
	}
	until, err := s.RolePrivilegeUntilAt(ctx, r, at)
	if err != nil {
		return r, nil, 0, err
	}
	if !until.After(at) {
		return r, nil, p.CodeLineForbidden, nil
	}
	balance, err := add(r.Wallet.BoundYuanbao, rules.DailyPrivilegeUnits)
	if err != nil {
		return r, nil, 0, err
	}
	r.Wallet.BoundYuanbao, r.PrivilegeClaimDay = balance, day
	return r, &RoleCardDailyReceipt{OperatingDay: day, AcceptedAt: at, PrivilegeUntil: until,
		GrantedBoundYuanbaoUnits: rules.DailyPrivilegeUnits, BalanceBoundYuanbaoUnits: balance}, 0, nil
}

// cardUnixMillis 使用协议零值表达未曾激活。
func cardUnixMillis(at time.Time) int64 {
	if at.IsZero() {
		return 0
	}
	return at.UnixMilli()
}
