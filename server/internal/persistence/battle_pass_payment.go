// 本文件冻结战令的现金升档差额，并把已核验支付兑现到原角色与周期。
package persistence

import (
	"context"
	"errors"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	"minimir/internal/payment"
	p "minimir/internal/protocol"
)

// BattlePassOrderTerms 的值参与现金原单摘要，确保日历和奖表与确认时相同。
type BattlePassOrderTerms struct {
	Start    string `bson:"season_start"` // 原周期首个运营日，防止订单转派到下个周期。
	Contract string `bson:"contract"`     // 原冻结现金价格、奖励、活跃条件的摘要。
	FromTier uint32 `bson:"from_tier"`    // 创建原单时该角色已经持有的最高档。
	FromFen  int64  `bson:"from_fen"`     // 原档完整人民币价格。
	TotalFen int64  `bson:"total_fen"`    // 新档完整价格，实付仅为两档差额。
}

// IsZero 保留升级前不带战令字段的原订单和已验证摘要。
func (terms BattlePassOrderTerms) IsZero() bool { return terms == (BattlePassOrderTerms{}) }

// valid 检查内部冻结条款的形状和价格差。
func (terms BattlePassOrderTerms) valid(to uint32, amount int64) bool {
	_, dayValid := battlePassDate(terms.Start)
	return dayValid && lowerHex(terms.Contract, 32) && terms.FromTier >= devconfig.BattlePassFree &&
		to > terms.FromTier && to <= devconfig.BattlePassMaxTier && terms.FromFen >= 0 && terms.TotalFen > terms.FromFen &&
		terms.TotalFen <= 1_000_000_000 && amount == terms.TotalFen-terms.FromFen
}

// battlePassContract 固定该周期参与时所见的原价格和奖表。
func battlePassContract(season battlePassSeason) (string, error) {
	return bossDigest(struct {
		Start        string                     // 初次固定的运营日窗口。
		Tiers        []devconfig.BattlePassTier // 首次参与时确定的各档条款。
		ActivityGoal uint32                     // 冻结的该周期活跃条件。
	}{season.Start, season.Tiers, season.ActivityGoal})
}

// QuoteBattlePassPayment 提供给渠道服务的查价入口。原单建立时还会事务内重新核验。
func (s *Store) QuoteBattlePassPayment(ctx context.Context, auth Auth, start string, target uint32, providerID, merchantID string) (PaymentOrderQuote, error) {
	if !lowerHex(auth.AccountID, 16) || auth.AuthVersion <= 0 || !lowerHex(auth.RoleID, 16) || start == "" ||
		!validPaymentChannelID(providerID, 32) || !validPaymentChannelID(merchantID, 128) {
		return PaymentOrderQuote{}, Fail(p.CodeInvalidArgument, "战令支付身份或所选周期无效")
	}
	r, err := s.Role(ctx, auth.RoleID)
	if err != nil {
		return PaymentOrderQuote{}, err
	}
	if r.AccountID != auth.AccountID {
		return PaymentOrderQuote{}, Fail(p.CodeUnauthenticated, "战令支付角色不属于当前账号")
	}
	quote, _, err := s.battlePassPaymentQuote(ctx, r, start, target, providerID, merchantID, time.Now())
	return quote, err
}

// battlePassPaymentQuote 同时返回即将冻结的原周期和精确应付金额。
func (s *Store) battlePassPaymentQuote(ctx context.Context, r domain.Role, start string, target uint32, providerID, merchantID string, at time.Time) (PaymentOrderQuote, battlePassSeason, error) {
	f := &s.Cfg.Fixture
	current := battlePassWindow(f.BattlePass, at)
	if current == "" || start != current || target > devconfig.BattlePassMaxTier {
		return PaymentOrderQuote{}, battlePassSeason{}, Fail(p.CodeBattlePassUnavailable, "当前战令周期不能创建此现金订单")
	}
	season, err := s.loadBattlePassBoard(ctx, r, start, at)
	if err != nil {
		return PaymentOrderQuote{}, season, err
	}
	if target <= season.Tier {
		return PaymentOrderQuote{}, season, Fail(p.CodeBattlePassChanged, "角色已具备此战令档位")
	}
	contract, err := battlePassContract(season)
	if err != nil {
		return PaymentOrderQuote{}, season, err
	}
	terms := BattlePassOrderTerms{Start: start, FromTier: season.Tier, Contract: contract,
		FromFen: season.Tiers[season.Tier-1].CashFen, TotalFen: season.Tiers[target-1].CashFen}
	quote := PaymentOrderQuote{Kind: payment.KindBattlePass, ProductID: target, AmountFen: terms.TotalFen - terms.FromFen,
		RuleVersion: f.RuleVersion, ConfigHash: f.Hash, ProviderID: providerID, MerchantID: merchantID, BattlePass: terms}
	return quote, season, nil
}

// reserveBattlePassPayment 在原单写入事务锁住角色当前升档步骤，阻止两张可付款订单重叠。
func (s *Store) reserveBattlePassPayment(ctx context.Context, auth Auth, quote PaymentOrderQuote, at time.Time) error {
	r, err := s.Role(ctx, auth.RoleID)
	if err != nil {
		return err
	}
	current, season, err := s.battlePassPaymentQuote(ctx, r, quote.BattlePass.Start, quote.ProductID, quote.ProviderID, quote.MerchantID, at)
	if err != nil {
		return err
	}
	if current != quote {
		return Fail(p.CodeBattlePassChanged, "现金升档原价或当前档位已变化")
	}
	filter := bson.M{"role_id": r.ID, "quote.battle_pass.season_start": season.Start, "quote.battle_pass.from_tier": season.Tier,
		"battle_pass_reserved": true}
	err = s.DB.Collection(paymentOrderCollection).FindOne(ctx, filter).Err()
	if err == nil {
		return Fail(p.CodeBattlePassPaymentPending, "已有原现金升档步骤等待确认")
	}
	if !errors.Is(err, mongo.ErrNoDocuments) {
		return err
	}
	return s.persistBattlePassSeason(ctx, season)
}

// BattlePassPaymentGrant 是已兑现的原档权益，不包含任何钱包投放。
type BattlePassPaymentGrant struct {
	Terms BattlePassOrderTerms `bson:"terms"` // 锁定的原日期、角色档位基线和现金差额。
	Tier  uint32               `bson:"tier"`  // 已确认获得的新档位。
}

// applyBattlePassPayment 在原支付履约时推进相同周期。只以实际付款时间判断是否在原销售期内。
func (s *Store) applyBattlePassPayment(ctx context.Context, r domain.Role, order PaymentOrder) (*BattlePassPaymentGrant, error) {
	quote := order.Quote
	terms := quote.BattlePass
	if !order.BattlePassReserved || order.Verification == nil || !order.acceptsBattlePassPaidAt(order.Verification.PaidAt) {
		return nil, Fail(p.CodeOperationConflict, "战令实付已超出冻结周期或原单步骤")
	}
	season, err := s.loadBattlePassSeason(ctx, r, terms.Start)
	if err != nil {
		return nil, err
	}
	contract, err := battlePassContract(season)
	if err != nil {
		return nil, err
	}
	if season.Revision == 0 || contract != terms.Contract || season.Tier != terms.FromTier ||
		season.Tiers[terms.FromTier-1].CashFen != terms.FromFen || season.Tiers[quote.ProductID-1].CashFen != terms.TotalFen {
		return nil, Fail(p.CodeOperationConflict, "现金原单的战令基线或冻结奖表已变化")
	}
	season.Tier = quote.ProductID
	if err := s.persistBattlePassSeason(ctx, season); err != nil {
		return nil, err
	}
	return &BattlePassPaymentGrant{Terms: terms, Tier: season.Tier}, nil
}

// acceptsBattlePassPaidAt 拒绝已关闭步骤或周期外付款，原核验可晚于该周期。
func (order PaymentOrder) acceptsBattlePassPaidAt(at time.Time) bool {
	return !at.Before(order.CreatedAt) && battlePassDay(order.Quote.BattlePass.Start, shanghaiOperatingDay(at)) != 0
}
