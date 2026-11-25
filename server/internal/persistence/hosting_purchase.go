// 本文件依据当前角色、永久余额和生效价格生成补时报价，并在资产事务中复核和成交。
package persistence

import (
	"time"

	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	p "minimir/internal/protocol"
)

// AssetBuyHostingTime 标识使用金币购买托管时长的经济操作。
const AssetBuyHostingTime = "buy_hosting"

// HostingPurchaseTerms 是玩家确认报价时带回的原交易事实，由服务端全部重新计算。
type HostingPurchaseTerms struct {
	Quote HostingPurchaseQuote // 原房间曾签发并保存的完整报价，客户端只带回编号。
}

// HostingPurchaseQuote 保存当前有效的报价快照，钱包拆分按原角色当前余额计算。
type HostingPurchaseQuote struct {
	QuoteID              string    `bson:"quote_id"`                  // 当前房间签发的随机报价编号。
	SceneEpoch           int64     `bson:"scene_epoch"`               // 用 BSON 有符号整型保存原场景代次的全部 64 个位。
	RoleRevision         uint64    `bson:"role_revision"`             // 预览时的角色持久版本。
	RuleVersion          uint64    `bson:"rule_version"`              // 签发时适用的玩法规则版本。
	ConfigHash           string    `bson:"config_hash"`               // 签发时适用的配置摘要。
	ExpiresAt            time.Time `bson:"expires_at"`                // 本次报价的首个无效时刻。
	QuoteVersion         uint64    `bson:"quote_version"`             // 正式价格版本。
	BandMinLevel         uint32    `bson:"band_min_level"`            // 档位起始等级。
	BandMaxLevel         uint32    `bson:"band_max_level"`            // 档位结束等级，零表示没有上限。
	RequestedSeconds     uint32    `bson:"requested_seconds"`         // 购买的整数秒数。
	GranularitySeconds   uint32    `bson:"granularity_seconds"`       // 定价配置允许的购买粒度。
	ReferenceGoldPerHour int64     `bson:"reference_gold_per_hour"`   // 该等级档的标准每小时毛金币。
	GoldPerHour          int64     `bson:"gold_per_hour"`             // 标准毛金币两倍的每小时售价。
	TotalGold            int64     `bson:"total_gold"`                // 整笔耗费，只在最后一步向上取整。
	BeforeSeconds        int64     `bson:"before_seconds"`            // 购买前托管秒数。
	BoundGoldCost        int64     `bson:"bound_gold_cost"`           // 优先扣取的绑定金币。
	GoldCost             int64     `bson:"gold_cost"`                 // 绑金不足时应扣的普通金币。
	EffectiveFrom        time.Time `bson:"effective_from"`            // 价格版本的生效时刻，含起点。
	EffectiveUntil       time.Time `bson:"effective_until,omitempty"` // 价格版本的截止时刻，零值表示没有截止。
}

// HostingPurchaseReceipt 固定首次成交的价格、资产拆分和时长，不随之后的角色或配置变化。
type HostingPurchaseReceipt struct {
	Quote          HostingPurchaseQuote `bson:"quote"`           // 事务内使用的原报价。
	BalanceSeconds int64                `bson:"balance_seconds"` // 成交后的累计托管秒数。
	AcceptedAt     time.Time            `bson:"accepted_at"`     // 交易确认使用的服务端时刻。
}

// Proto 将报价的原整数、角色版本和确认时限投影到网络，预览的业务结果码由调用方设置。
func (q HostingPurchaseQuote) Proto() *pb.HostingQuoteResult {
	result := &pb.HostingQuoteResult{QuoteId: domain.Bytes(q.QuoteID), SceneEpoch: uint64(q.SceneEpoch), RoleRevision: q.RoleRevision,
		RuleVersion: q.RuleVersion, ConfigHash: q.ConfigHash, QuoteVersion: q.QuoteVersion, BandMinLevel: q.BandMinLevel,
		BandMaxLevel: q.BandMaxLevel, GranularitySeconds: q.GranularitySeconds, RequestedSeconds: q.RequestedSeconds,
		ReferenceGoldPerHour: q.ReferenceGoldPerHour, GoldPerHour: q.GoldPerHour, TotalGold: q.TotalGold,
		BeforeSeconds: q.BeforeSeconds, BoundGoldCost: q.BoundGoldCost, GoldCost: q.GoldCost}
	if !q.ExpiresAt.IsZero() {
		result.ExpiresAtUnixMs = q.ExpiresAt.UnixMilli()
	}
	return result
}

// Proto 返回新提交与幂等重试共用的已确认回执。
func (r *HostingPurchaseReceipt) Proto() *pb.HostingPurchaseReceipt {
	if r == nil {
		return nil
	}
	return &pb.HostingPurchaseReceipt{Quote: r.Quote.Proto(), BalanceSeconds: r.BalanceSeconds}
}

// QuoteHostingTime 根据服务端时钟返回当前报价。余额或钱包不合格时可显示真实价格，结果码仍会阻止成交。
func QuoteHostingTime(r domain.Role, f *devconfig.Fixture, seconds uint32, at time.Time) (HostingPurchaseQuote, uint32) {
	if seconds > devconfig.MaxHostingTimePurchaseSeconds {
		return HostingPurchaseQuote{}, p.CodeInvalidArgument
	}
	price, found := f.HostingTimePrice(r.Level, at)
	if !found || price.Version == 0 {
		return HostingPurchaseQuote{}, p.CodeHostingPurchaseUnavailable
	}
	// 只读发现可先查出配置粒度和档位；实际资产提交禁止零秒。
	if seconds == 0 {
		seconds = price.PurchaseStepSeconds
	}
	return quoteHostingTime(r, price, seconds)
}

// quoteHostingTime 共用购买粒度、计价与支付规则，确保展示与成交计算一致。
func quoteHostingTime(r domain.Role, price devconfig.HostingTimePrice, seconds uint32) (HostingPurchaseQuote, uint32) {
	total, valid := price.Cost(seconds)
	if !valid {
		return HostingPurchaseQuote{}, p.CodeInvalidArgument
	}
	bound := min(max(r.Wallet.BoundGold, 0), total)
	quote := HostingPurchaseQuote{QuoteVersion: price.Version, BandMinLevel: price.Band.LevelMin, BandMaxLevel: price.Band.LevelMax,
		RequestedSeconds: seconds, GranularitySeconds: price.PurchaseStepSeconds, ReferenceGoldPerHour: price.Band.GRefGoldPerHour,
		GoldPerHour: price.GoldPerHour(), TotalGold: total, BeforeSeconds: r.HostingSeconds, BoundGoldCost: bound, GoldCost: total - bound,
		EffectiveFrom: price.EffectiveFrom.UTC().Truncate(time.Millisecond), EffectiveUntil: price.EffectiveUntil.UTC().Truncate(time.Millisecond)}
	switch {
	case r.HostingSeconds < 0 || r.Wallet.BoundGold < 0 || r.Wallet.Gold < 0:
		return quote, p.CodeOperationConflict
	case r.HostingSeconds >= int64(devconfig.MaxHostingTimePurchaseSeconds):
		return quote, p.CodeHostingPurchaseBalance
	case r.Wallet.Gold < quote.GoldCost:
		return quote, p.CodeHostingPurchaseFunds
	}
	return quote, 0
}

// applyHostingPurchase 从当前事务内的角色等级、余额及价格重新报价，再核对确认页并执行绑定金币优先的原资产变更。
func applyHostingPurchase(r domain.Role, f *devconfig.Fixture, terms *HostingPurchaseTerms, scene uint64, at time.Time) (domain.Role, *HostingPurchaseReceipt, uint32) {
	price, found := f.HostingTimePrice(r.Level, at)
	if terms != nil && !at.Before(terms.Quote.ExpiresAt.UTC().Truncate(time.Millisecond)) {
		return r, nil, p.CodeHostingPurchaseQuoteStale
	}
	if !found || price.Version == 0 {
		return r, nil, p.CodeHostingPurchaseUnavailable
	}
	if terms == nil {
		return r, nil, p.CodeHostingPurchaseQuoteStale
	}
	trusted := terms.Quote
	trusted.ExpiresAt = trusted.ExpiresAt.UTC().Truncate(time.Millisecond)
	if trusted.RequestedSeconds == 0 || trusted.RequestedSeconds > devconfig.MaxHostingTimePurchaseSeconds {
		return r, nil, p.CodeInvalidArgument
	}
	// 场景代次是完整的 uint64，比较和上报时恢复原位模式。
	id := domain.Bytes(trusted.QuoteID)
	if len(id) != 16 || domain.Hex(id) != trusted.QuoteID || scene == 0 || uint64(trusted.SceneEpoch) != scene ||
		trusted.RoleRevision != uint64(r.Revision) || trusted.RuleVersion != f.RuleVersion || trusted.ConfigHash != f.Hash ||
		trusted.QuoteVersion != price.Version || trusted.BandMinLevel != price.Band.LevelMin {
		return r, nil, p.CodeHostingPurchaseQuoteStale
	}
	quote, code := quoteHostingTime(r, price, trusted.RequestedSeconds)
	quote.QuoteID, quote.SceneEpoch, quote.RoleRevision = trusted.QuoteID, int64(scene), uint64(r.Revision)
	quote.RuleVersion, quote.ConfigHash, quote.ExpiresAt = f.RuleVersion, f.Hash, trusted.ExpiresAt
	if quote.TotalGold > 0 && quote != trusted {
		return r, nil, p.CodeHostingPurchaseQuoteStale
	}
	if code != 0 {
		return r, nil, code
	}
	r.Wallet.BoundGold -= quote.BoundGoldCost
	r.Wallet.Gold -= quote.GoldCost
	r.HostingSeconds += int64(quote.RequestedSeconds)
	return r, &HostingPurchaseReceipt{Quote: quote, BalanceSeconds: r.HostingSeconds, AcceptedAt: at.UTC().Truncate(time.Millisecond)}, 0
}
