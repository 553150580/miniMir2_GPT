// 本文件记录一件拍品、各轮预购、待领资产与跨市场的交易事实。
package persistence

import (
	"context"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"minimir/internal/domain"
	p "minimir/internal/protocol"
)

const (
	itemMarketOrders          = "item_orders"
	itemMarketBids            = "item_bids"
	itemMarketClaims          = "item_market_claims"
	itemMarketRefundJobs      = "item_market_refund_jobs"
	itemMarketSettlements     = "item_market_settlements"
	marketSellerTaxRemainders = "seller_tax_remainders"
)

const (
	ItemMarketAuction   uint32 = iota + 1 // 竞购时只接受全单冻结。
	ItemMarketDirect                      // 无预购后允许部分直购。
	ItemMarketSold                        // 已全数成交。
	ItemMarketCancelled                   // 无预购时卖方撤回。
	ItemMarketExpired                     // 到期且仍有余量，已转给卖家待领。
)

const (
	itemBidFrozen   uint32 = iota + 1 // 原钱包资金仍由本轮预购冻结。
	itemBidCarried                    // 加价者原冻结已转入下一轮。
	itemBidPaid                       // 原冻结已用于成交。
	itemBidRefunded                   // 原冻结已转入同角色的退款待领。
)

const (
	ItemMarketClaimItem     uint32 = iota + 1 // 赢家交付或卖方返物。
	ItemMarketClaimProceeds                   // 实收进入原卖方非绑定钱包。
	ItemMarketClaimRefund                     // 原冻结资金返回原买方非绑定钱包。
)

// MarketParty 固定原角色、原账号和永久来源服，交易后仍用原身份交付。
type MarketParty struct {
	RoleID    string `bson:"role_id"`
	AccountID string `bson:"account_id"`
	Origin    uint64 `bson:"origin_server_id"`
}

func marketParty(r domain.Role) MarketParty {
	return MarketParty{RoleID: r.ID, AccountID: r.AccountID, Origin: r.Origin}
}

// ItemMarketOrder 是当前运行服对一件托管货物的独占所有权。
type ItemMarketOrder struct {
	ID             string        `bson:"_id"`
	RunID          uint64        `bson:"run_server_id"`
	Seller         MarketParty   `bson:"seller"`
	Item           domain.Item   `bson:"item"` // 只在活跃阶段保留可购买的余量。
	UnitPrice      int64         `bson:"unit_price"`
	Revision       int64         `bson:"revision"`
	Round          int64         `bson:"round"`
	ActiveBidCount int64         `bson:"active_bid_count"`
	Phase          uint32        `bson:"phase"`
	ListedAt       time.Time     `bson:"listed_at"`
	AuctionEndsAt  time.Time     `bson:"auction_ends_at"`
	DirectEndsAt   time.Time     `bson:"direct_ends_at,omitempty"`
	ClosedAt       time.Time     `bson:"closed_at,omitempty"`
	BuyoutDuration time.Duration `bson:"buyout_duration"` // 发布变更后旧拍品仍用上架时的期限。
	TaxDivisor     int64         `bson:"tax_divisor"`
	RuleVersion    uint64        `bson:"rule_version"`
	ConfigHash     string        `bson:"config_hash"`
	WinnerBidID    string        `bson:"winner_bid_id,omitempty"`
}

// ItemMarketBid 只对一轮的一次冻结负责。连续序号使随机抽取无需扫描名单。
type ItemMarketBid struct {
	ID        string      `bson:"_id"`
	OrderID   string      `bson:"order_id"`
	RunID     uint64      `bson:"run_server_id"`
	Buyer     MarketParty `bson:"buyer"`
	Round     int64       `bson:"round"`
	Ordinal   int64       `bson:"ordinal"`
	Amount    int64       `bson:"amount"`
	State     uint32      `bson:"state"`
	CreatedAt time.Time   `bson:"created_at"`
	EndedAt   time.Time   `bson:"ended_at,omitempty"`
}

// ItemMarketClaim 记录实际可领取的原资产，领取后保留永久证据。
type ItemMarketClaim struct {
	ID        string       `bson:"_id"`
	OrderID   string       `bson:"order_id"`
	Owner     MarketParty  `bson:"owner"`
	Kind      uint32       `bson:"kind"`
	Revision  int64        `bson:"revision"`
	Item      *domain.Item `bson:"item,omitempty"`
	Yuanbao   int64        `bson:"yuanbao,omitempty"`
	CreatedAt time.Time    `bson:"created_at"`
	ClaimedAt time.Time    `bson:"claimed_at,omitempty"`
	ClaimOp   string       `bson:"claim_op,omitempty"`
}

// ItemMarketRefundJob 以原轮次标识全部仍冻结的输家，逐笔推进。
type ItemMarketRefundJob struct {
	ID          string    `bson:"_id"`
	OrderID     string    `bson:"order_id"`
	RunID       uint64    `bson:"run_server_id"`
	Round       int64     `bson:"round"`
	CreatedAt   time.Time `bson:"created_at"`
	NextCheckAt time.Time `bson:"next_check_at"`
}

// ItemMarketSettlement 固定成交的资产、费用、原买卖双方以及确定的赢家。
type ItemMarketSettlement struct {
	ID          string      `bson:"_id"`
	OrderID     string      `bson:"order_id"`
	RunID       uint64      `bson:"run_server_id"`
	Seller      MarketParty `bson:"seller"`
	Buyer       MarketParty `bson:"buyer"`
	WinnerBidID string      `bson:"winner_bid_id,omitempty"`
	Item        domain.Item `bson:"item"`
	UnitPrice   int64       `bson:"unit_price"`
	Gross       int64       `bson:"gross"`
	Tax         int64       `bson:"tax"`
	RuleVersion uint64      `bson:"rule_version"`
	ConfigHash  string      `bson:"config_hash"`
	CreatedAt   time.Time   `bson:"created_at"`
}

// marketFactID 按原操作或固定来源生成十六字节身份，事务重试不改身份。
func marketFactID(parts ...string) string {
	return Hash([]byte(strings.Join(parts, "\x00")))[:32]
}

func (s *Store) insertItemMarketClaim(ctx context.Context, claim ItemMarketClaim) error {
	_, err := s.DB.Collection(itemMarketClaims).InsertOne(ctx, claim)
	return err
}

func (s *Store) insertItemMarketSettlement(ctx context.Context, fact ItemMarketSettlement) error {
	_, err := s.DB.Collection(itemMarketSettlements).InsertOne(ctx, fact)
	return err
}

// replaceItemMarketOrder 即使后台或原角色同时唤醒，也只允许在原版本上推进。
func (s *Store) replaceItemMarketOrder(ctx context.Context, order ItemMarketOrder, revision int64) error {
	out, err := s.DB.Collection(itemMarketOrders).ReplaceOne(ctx, bson.M{
		"_id": order.ID, "run_server_id": order.RunID, "revision": revision,
	}, order)
	if err == nil && out.MatchedCount != 1 {
		return Fail(p.CodeOperationConflict, "原市场订单版本已变化")
	}
	return err
}

// itemMarketIndexes 为公开分页、账号上架额度、赢家选取及后台退款提供确定的访问路径。
func itemMarketIndexes() map[string][]mongo.IndexModel {
	index := func(names ...string) mongo.IndexModel {
		keys := bson.D{}
		for _, name := range names {
			keys = append(keys, bson.E{Key: name, Value: 1})
		}
		return mongo.IndexModel{Keys: keys}
	}
	unique := func(names ...string) mongo.IndexModel {
		model := index(names...)
		model.Options = options.Index().SetUnique(true)
		return model
	}
	return map[string][]mongo.IndexModel{
		itemMarketOrders: {
			index("run_server_id", "phase", "auction_ends_at", "_id"),
			index("run_server_id", "phase", "direct_ends_at", "_id"),
			index("run_server_id", "phase", "_id"),
			index("run_server_id", "phase", "item.item_id", "_id"),
			index("run_server_id", "seller.account_id", "phase"),
			index("run_server_id", "seller.role_id", "seller.origin_server_id", "_id"),
		},
		itemMarketBids: {
			unique("order_id", "round", "ordinal"),
			unique("order_id", "round", "buyer.account_id"),
			index("order_id", "round", "state", "_id"),
			index("run_server_id", "buyer.role_id", "buyer.origin_server_id", "state", "order_id"),
		},
		itemMarketClaims: {
			index("owner.role_id", "owner.origin_server_id", "claimed_at", "_id"),
		},
		itemMarketRefundJobs: {
			index("run_server_id", "next_check_at", "_id"),
		},
		itemMarketSettlements: {
			index("order_id", "created_at", "_id"),
		},
	}
}
