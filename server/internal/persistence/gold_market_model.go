// 本文件定义金币委托、定点成交和原角色的待领货币。
package persistence

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	p "minimir/internal/protocol"
)

const (
	goldMarketOrders = "gold_orders"
	goldMarketClaims = "gold_market_claims"
	goldMarketFills  = "gold_market_fills"
	goldMarketBooks  = "gold_market_books"
)

const (
	GoldMarketBuy  uint32 = iota + 1 // 买单冻结非绑定元宝。
	GoldMarketSell                   // 卖单冻结非绑定金币。
)

const (
	GoldMarketActive uint32 = iota + 1
	GoldMarketFilled
	GoldMarketCancelled
	GoldMarketExpired
)

const (
	GoldMarketClaimGold          uint32 = iota + 1 // 买方已成交金币。
	GoldMarketClaimProceeds                        // 卖方扣佣后的非绑元宝。
	GoldMarketClaimYuanbaoRefund                   // 价差或撤销、过期的非绑元宝返还。
	GoldMarketClaimGoldReturn                      // 撤销、过期的非绑金币返还。
)

const (
	GoldMarketReasonFill uint32 = iota + 1
	GoldMarketReasonPriceDifference
	GoldMarketReasonCancel
	GoldMarketReasonExpiry
)

// GoldMarketOrder 固定本次委托的原始价格、币种、期限与受理次序。
type GoldMarketOrder struct {
	ID            string      `bson:"_id"`
	RunID         uint64      `bson:"run_server_id"`
	Owner         MarketParty `bson:"owner"`
	Side          uint32      `bson:"side"`
	State         uint32      `bson:"state"`
	Sequence      int64       `bson:"sequence"` // 原运行服分配的严格时间优先水位。
	LimitPrice    int64       `bson:"limit_price"`
	InitialLots   int64       `bson:"initial_lots"`
	RemainingLots int64       `bson:"remaining_lots"`
	FrozenGold    int64       `bson:"frozen_gold"`
	FrozenYuanbao int64       `bson:"frozen_yuanbao"`
	Revision      int64       `bson:"revision"`
	TaxDivisor    int64       `bson:"tax_divisor"`
	GoldPerLot    int64       `bson:"gold_per_lot"`
	AcceptedAt    time.Time   `bson:"accepted_at"`
	ExpiresAt     time.Time   `bson:"expires_at"`
	ClosedAt      time.Time   `bson:"closed_at,omitempty"`
	PendingMatch  bool        `bson:"pending_match"` // 等待按原受理次序和较早在簿订单比较。
	RuleVersion   uint64      `bson:"rule_version"`
	ConfigHash    string      `bson:"config_hash"`
}

// GoldMarketClaim 的两种货币字段始终互斥，领取身份来自原交易事实。
type GoldMarketClaim struct {
	ID        string      `bson:"_id"`
	OrderID   string      `bson:"order_id"`
	FillID    string      `bson:"fill_id,omitempty"`
	Owner     MarketParty `bson:"owner"`
	Kind      uint32      `bson:"kind"`
	Reason    uint32      `bson:"reason"`
	Revision  int64       `bson:"revision"`
	Gold      int64       `bson:"gold,omitempty"`
	Yuanbao   int64       `bson:"yuanbao,omitempty"`
	CreatedAt time.Time   `bson:"created_at"`
	ClaimedAt time.Time   `bson:"claimed_at,omitempty"`
	ClaimOp   string      `bson:"claim_op,omitempty"`
}

// GoldMarketFill 同时固定撮合对手、价格、成交实收、价差及原规则身份。
type GoldMarketFill struct {
	ID                string      `bson:"_id"`
	BuyOrderID        string      `bson:"buy_order_id"`
	SellOrderID       string      `bson:"sell_order_id"`
	MakerOrderID      string      `bson:"maker_order_id"`
	TakerOrderID      string      `bson:"taker_order_id"`
	RunID             uint64      `bson:"run_server_id"`
	Sequence          int64       `bson:"sequence"`
	Buyer             MarketParty `bson:"buyer"`
	Seller            MarketParty `bson:"seller"`
	Lots              int64       `bson:"lots"`
	UnitPrice         int64       `bson:"unit_price"`
	Gross             int64       `bson:"gross"`
	Tax               int64       `bson:"tax"`
	BuyerRefund       int64       `bson:"buyer_refund"`
	Gold              int64       `bson:"gold"`
	MakerRuleVersion  uint64      `bson:"maker_rule_version"`
	TakerRuleVersion  uint64      `bson:"taker_rule_version"`
	MakerConfigHash   string      `bson:"maker_config_hash"`
	TakerConfigHash   string      `bson:"taker_config_hash"`
	ReferenceExcluded bool        `bson:"reference_excluded"`
	CreatedAt         time.Time   `bson:"created_at"`
}

// nextGoldMarketSequence 用同一运行服计数器确定同毫秒内的委托和成交次序。
func (s *Store) nextGoldMarketSequence(ctx context.Context, field string) (int64, error) {
	var state bson.M
	err := s.DB.Collection(goldMarketBooks).FindOneAndUpdate(ctx, bson.M{"_id": s.Cfg.RunID},
		bson.M{"$inc": bson.M{field: int64(1)}}, options.FindOneAndUpdate().SetUpsert(true).
			SetReturnDocument(options.After).SetProjection(bson.M{field: 1})).Decode(&state)
	if mongo.IsDuplicateKeyError(err) {
		err = Fail(p.CodeOperationPending, "金币委托次序待确认")
	}
	if err != nil {
		return 0, err
	}
	next, valid := state[field].(int64)
	if !valid || next <= 0 {
		return 0, Fail(p.CodeOperationConflict, "金币市场原序号无效")
	}
	return next, nil
}

func (s *Store) replaceGoldMarketOrder(ctx context.Context, order GoldMarketOrder, oldRevision int64) error {
	result, err := s.DB.Collection(goldMarketOrders).ReplaceOne(ctx, bson.M{
		"_id": order.ID, "run_server_id": order.RunID, "revision": oldRevision,
	}, order)
	if err == nil && result.MatchedCount != 1 {
		return Fail(p.CodeOperationConflict, "原金币委托修订已变化")
	}
	return err
}

func (s *Store) insertGoldMarketClaim(ctx context.Context, claim GoldMarketClaim) error {
	_, err := s.DB.Collection(goldMarketClaims).InsertOne(ctx, claim)
	return err
}

func (s *Store) insertGoldMarketFill(ctx context.Context, fill *GoldMarketFill) error {
	sequence, err := s.nextGoldMarketSequence(ctx, "fill_sequence")
	if err != nil {
		return err
	}
	fill.Sequence = sequence
	_, err = s.DB.Collection(goldMarketFills).InsertOne(ctx, fill)
	return err
}

// goldMarketIndexes 支持价格和原受理次序、公私分页、到期和参考成交窗口。
func goldMarketIndexes() map[string][]mongo.IndexModel {
	return map[string][]mongo.IndexModel{
		goldMarketOrders: {
			{Keys: bson.D{{Key: "run_server_id", Value: 1}, {Key: "sequence", Value: 1}}, Options: options.Index().SetUnique(true)},
			{Keys: bson.D{{Key: "run_server_id", Value: 1}, {Key: "state", Value: 1}, {Key: "pending_match", Value: 1}, {Key: "sequence", Value: 1}}},
			{Keys: bson.D{{Key: "run_server_id", Value: 1}, {Key: "state", Value: 1}, {Key: "expires_at", Value: 1}, {Key: "sequence", Value: 1}}},
			{Keys: bson.D{{Key: "run_server_id", Value: 1}, {Key: "side", Value: 1}, {Key: "state", Value: 1}, {Key: "limit_price", Value: 1}, {Key: "sequence", Value: 1}}},
			{Keys: bson.D{{Key: "run_server_id", Value: 1}, {Key: "side", Value: 1}, {Key: "state", Value: 1}, {Key: "limit_price", Value: -1}, {Key: "sequence", Value: 1}}},
			{Keys: bson.D{{Key: "run_server_id", Value: 1}, {Key: "owner.account_id", Value: 1}, {Key: "side", Value: 1}, {Key: "state", Value: 1}}},
			{Keys: bson.D{{Key: "owner.role_id", Value: 1}, {Key: "owner.origin_server_id", Value: 1}, {Key: "_id", Value: 1}}},
		},
		goldMarketClaims: {
			{Keys: bson.D{{Key: "owner.role_id", Value: 1}, {Key: "owner.origin_server_id", Value: 1}, {Key: "claimed_at", Value: 1}, {Key: "_id", Value: 1}}},
		},
		goldMarketFills: {
			{Keys: bson.D{{Key: "run_server_id", Value: 1}, {Key: "sequence", Value: -1}}, Options: options.Index().SetUnique(true)},
			{Keys: bson.D{{Key: "run_server_id", Value: 1}, {Key: "reference_excluded", Value: 1}, {Key: "sequence", Value: -1}}},
		},
	}
}
