// 本文件按原截止结算整单竞购，并把无人竞购或剩余商品推进到下一阶段。
package persistence

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"math"
	"strconv"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	p "minimir/internal/protocol"
)

// chooseItemMarketOrdinal 用事务外取到的原熵等概率抽选连续预购序号。
func chooseItemMarketOrdinal(entropy [32]byte, count int64) int64 {
	modulus := uint64(count)
	threshold := -modulus % modulus
	var input [40]byte
	copy(input[:32], entropy[:])
	for counter := uint64(0); ; counter++ {
		binary.BigEndian.PutUint64(input[32:], counter)
		hash := sha256.Sum256(input[:])
		value := binary.BigEndian.Uint64(hash[:8])
		if value >= threshold {
			return int64(value%modulus) + 1
		}
	}
}

// validMarketSettlementParty 限制永久凭据，匿名连接或缺失入口不参与已有资产结算。
func validMarketSettlementParty(party MarketParty) bool {
	return lowerHex(party.RoleID, 16) && lowerHex(party.AccountID, 16) && party.Origin != 0
}

// finishItemMarketAuction 先持久冻结已到期的原赢家，随后按固定事实执行一次结算。
func (s *Store) finishItemMarketAuction(ctx context.Context, due ItemMarketOrder, entropy [32]byte, now time.Time) error {
	if due.ActiveBidCount < 0 || due.ActiveBidCount == 0 && due.WinnerBidID != "" ||
		!validMarketSettlementParty(due.Seller) || due.Round <= 0 {
		return Fail(p.CodeOperationConflict, "道具市场原竞购资料无效")
	}
	var winner ItemMarketBid
	bids := s.DB.Collection(itemMarketBids)
	accounts := []string{}
	if due.ActiveBidCount > 0 {
		filter := bson.M{"order_id": due.ID, "run_server_id": due.RunID, "round": due.Round, "state": itemBidFrozen}
		if due.WinnerBidID == "" {
			filter["ordinal"] = chooseItemMarketOrdinal(entropy, due.ActiveBidCount)
		} else {
			filter["_id"] = due.WinnerBidID
		}
		err := bids.FindOne(ctx, filter).Decode(&winner)
		if err != nil && !errors.Is(err, mongo.ErrNoDocuments) {
			return err
		}
		if winner.ID != "" {
			accounts = []string{due.Seller.AccountID, winner.Buyer.AccountID}
		}
	}
	return s.itemMarketSystemTx(ctx, accounts, func(tc context.Context) error {
		order, err := s.itemMarketOrderAtExpiry(tc, due, "auction_ends_at", now)
		if err != nil || order.ID == "" {
			return err
		}
		if order.Item.Quantity <= 0 || order.Round != due.Round || order.ActiveBidCount != due.ActiveBidCount || order.WinnerBidID != due.WinnerBidID {
			return Fail(p.CodeOperationConflict, "道具市场原竞购物品或预购状态失效")
		}
		if order.ActiveBidCount == 0 {
			if order.BuyoutDuration <= 0 || order.AuctionEndsAt.IsZero() {
				return Fail(p.CodeOperationConflict, "道具市场原直购期限无效")
			}
			if err := bids.FindOne(tc, bson.M{"order_id": order.ID, "run_server_id": order.RunID,
				"round": order.Round, "state": itemBidFrozen}).Err(); err == nil {
				return Fail(p.CodeOperationConflict, "竞购人数与原冻结预购不符")
			} else if !errors.Is(err, mongo.ErrNoDocuments) {
				return err
			}
			order.Phase, order.DirectEndsAt = ItemMarketDirect, bossTime(order.AuctionEndsAt.Add(order.BuyoutDuration))
			if err := s.updateItemMarketExpiredOrder(tc, &order); err != nil {
				return err
			}
			return s.writeItemMarketDeadlineAudit(tc, order, "item_market_open_direct", now, nil)
		}
		if winner.ID == "" {
			return Fail(p.CodeOperationConflict, "原竞购抽选结果缺少有效预购")
		}
		var confirmed ItemMarketBid
		if err := bids.FindOne(tc, bson.M{"_id": winner.ID, "order_id": order.ID, "run_server_id": order.RunID,
			"round": order.Round, "ordinal": winner.Ordinal, "state": itemBidFrozen}).Decode(&confirmed); err != nil {
			return authorityReadError(err, p.CodeOperationConflict, "原竞购抽选的预购已变化")
		}
		if winner.Buyer != confirmed.Buyer || !validMarketSettlementParty(confirmed.Buyer) ||
			confirmed.Buyer.AccountID == order.Seller.AccountID || order.UnitPrice <= 0 || order.Item.Quantity > math.MaxInt64/order.UnitPrice ||
			confirmed.Amount != order.UnitPrice*order.Item.Quantity {
			return Fail(p.CodeOperationConflict, "原竞购冻结数额或买卖归属无效")
		}
		if order.WinnerBidID == "" {
			order.WinnerBidID = confirmed.ID
			if err := s.updateItemMarketExpiredOrder(tc, &order); err != nil {
				return err
			}
			return s.writeItemMarketDeadlineAudit(tc, order, "item_market_auction_drawn", now, bson.M{
				"winner_bid_id": confirmed.ID, "winner_ordinal": confirmed.Ordinal, "participating_count": order.ActiveBidCount, "draw_entropy": entropy[:],
			})
		}
		return s.confirmItemMarketWinner(tc, &order, confirmed, now)
	})
}

// confirmItemMarketWinner 在同一事务中消费赢家冻结款、登记卖方净收入与物品待领。
func (s *Store) confirmItemMarketWinner(ctx context.Context, order *ItemMarketOrder, winner ItemMarketBid, at time.Time) error {
	tax, err := s.chargeMarketSellerTax(ctx, order.Seller, winner.Amount, order.TaxDivisor)
	if err != nil {
		return err
	}
	fact := ItemMarketSettlement{ID: marketFactID("market-settlement", order.ID, "auction", strconv.FormatInt(order.Round, 10)),
		OrderID: order.ID, RunID: order.RunID, Seller: order.Seller, Buyer: winner.Buyer, WinnerBidID: winner.ID,
		Item: order.Item, UnitPrice: order.UnitPrice, Gross: winner.Amount, Tax: tax,
		RuleVersion: order.RuleVersion, ConfigHash: order.ConfigHash, CreatedAt: at}
	if err := s.insertItemMarketSettlement(ctx, fact); err != nil {
		return err
	}
	asset := order.Item
	if err := s.insertItemMarketClaim(ctx, ItemMarketClaim{ID: marketFactID("market-claim", fact.ID, "auction-item"),
		OrderID: order.ID, Owner: winner.Buyer, Kind: ItemMarketClaimItem, Revision: 1, Item: &asset, CreatedAt: at}); err != nil {
		return err
	}
	if proceeds := winner.Amount - tax; proceeds > 0 {
		if err := s.insertItemMarketClaim(ctx, ItemMarketClaim{ID: marketFactID("market-claim", fact.ID, "auction-proceeds"),
			OrderID: order.ID, Owner: order.Seller, Kind: ItemMarketClaimProceeds, Revision: 1, Yuanbao: proceeds, CreatedAt: at}); err != nil {
			return err
		}
	}
	changed, err := s.DB.Collection(itemMarketBids).UpdateOne(ctx, bson.M{"_id": winner.ID, "state": itemBidFrozen},
		bson.M{"$set": bson.M{"state": itemBidPaid, "ended_at": at}})
	if err != nil {
		return err
	}
	if changed.MatchedCount != 1 {
		return Fail(p.CodeOperationConflict, "原竞购赢家的冻结款已执行")
	}
	if order.ActiveBidCount > 1 {
		if err := s.queueItemMarketRefund(ctx, *order, at); err != nil {
			return err
		}
	}
	evidence := bson.M{"settlement_id": fact.ID, "winner_bid_id": winner.ID, "winner_ordinal": winner.Ordinal,
		"participating_count": order.ActiveBidCount, "gross": fact.Gross, "tax": fact.Tax}
	order.Phase, order.ClosedAt, order.WinnerBidID, order.ActiveBidCount = ItemMarketSold, at, winner.ID, 0
	order.Item.Quantity = 0
	if err := s.updateItemMarketExpiredOrder(ctx, order); err != nil {
		return err
	}
	return s.writeItemMarketDeadlineAudit(ctx, *order, "item_market_auction_sold", at, evidence)
}

// finishItemMarketDirectExpiry 把未售完的原物完整返到原角色交易待领。
func (s *Store) finishItemMarketDirectExpiry(ctx context.Context, due ItemMarketOrder, now time.Time) error {
	return s.itemMarketSystemTx(ctx, []string{due.Seller.AccountID}, func(tc context.Context) error {
		order, err := s.itemMarketOrderAtExpiry(tc, due, "direct_ends_at", now)
		if err != nil || order.ID == "" {
			return err
		}
		if order.Item.Quantity <= 0 || !validMarketSettlementParty(order.Seller) {
			return Fail(p.CodeOperationConflict, "到期订单缺少原剩余物品或卖方归属")
		}
		item := order.Item
		if err := s.insertItemMarketClaim(tc, ItemMarketClaim{ID: marketFactID("market-claim", order.ID, "expired-item"),
			OrderID: order.ID, Owner: order.Seller, Kind: ItemMarketClaimItem, Revision: 1, Item: &item, CreatedAt: now}); err != nil {
			return err
		}
		order.Phase, order.ClosedAt = ItemMarketExpired, now
		order.Item.Quantity = 0
		if err := s.updateItemMarketExpiredOrder(tc, &order); err != nil {
			return err
		}
		return s.writeItemMarketDeadlineAudit(tc, order, "item_market_direct_expired", now, bson.M{"quantity": item.Quantity})
	})
}

// itemMarketOrderAtExpiry 核对调度窗口中的原阶段和修订，以免与正在确认的用户交易竞争。
func (s *Store) itemMarketOrderAtExpiry(ctx context.Context, due ItemMarketOrder, deadline string, at time.Time) (ItemMarketOrder, error) {
	var latest ItemMarketOrder
	err := s.DB.Collection(itemMarketOrders).FindOne(ctx, bson.M{"_id": due.ID, "run_server_id": s.Cfg.RunID,
		"phase": due.Phase, "revision": due.Revision, deadline: bson.M{"$lte": at}}).Decode(&latest)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return ItemMarketOrder{}, nil
	}
	if err == nil && (latest.Revision <= 0 || latest.Revision == math.MaxInt64) {
		return ItemMarketOrder{}, Fail(p.CodeOperationConflict, "道具市场原订单修订耗尽")
	}
	return latest, err
}

// updateItemMarketExpiredOrder 以原修订确认唯一阶段推进。
func (s *Store) updateItemMarketExpiredOrder(ctx context.Context, order *ItemMarketOrder) error {
	revision := order.Revision
	order.Revision++
	return s.replaceItemMarketOrder(ctx, *order, revision)
}

// writeItemMarketDeadlineAudit 审计原到期时钟、配置身份和不可变的成交或退货事实。
func (s *Store) writeItemMarketDeadlineAudit(ctx context.Context, order ItemMarketOrder, kind string, at time.Time, evidence bson.M) error {
	_, err := s.DB.Collection("audit_outbox").InsertOne(ctx, bson.M{"_id": kind + ":" + order.ID,
		"kind": kind, "order_id": order.ID, "role_id": order.Seller.RoleID, "account_id": order.Seller.AccountID,
		"origin_server_id": order.Seller.Origin, "run_server_id": order.RunID,
		"round": order.Round, "revision": order.Revision, "rule_version": order.RuleVersion, "config_hash": order.ConfigHash,
		"auction_ends_at": order.AuctionEndsAt, "direct_ends_at": order.DirectEndsAt, "details": evidence, "at": at})
	return err
}
