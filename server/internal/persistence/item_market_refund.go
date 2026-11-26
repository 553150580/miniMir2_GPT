// 本文件把已结算或被加价替代的原冻结款按有界批次返还到原角色待领。
package persistence

import (
	"context"
	"errors"
	"strconv"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	p "minimir/internal/protocol"
)

// itemMarketRefundBatch 限制一次事务返还的原冻结款数量及账号围栏数。
const itemMarketRefundBatch = 8

// queueItemMarketRefund 在订单切换预购轮或确认唯一赢家时冻结待退的原轮。
func (s *Store) queueItemMarketRefund(ctx context.Context, order ItemMarketOrder, now time.Time) error {
	if !lowerHex(order.ID, 16) || order.RunID != s.Cfg.RunID || order.Round <= 0 || now.IsZero() {
		return Fail(p.CodeOperationConflict, "道具市场退款缺少原运行服、订单或预购轮")
	}
	job := ItemMarketRefundJob{ID: marketFactID("market-refund-job", order.ID, strconv.FormatInt(order.Round, 10)),
		OrderID: order.ID, RunID: order.RunID, Round: order.Round, CreatedAt: bossTime(now), NextCheckAt: bossTime(now)}
	_, err := s.DB.Collection(itemMarketRefundJobs).UpdateOne(ctx,
		bson.M{"_id": job.ID, "run_server_id": job.RunID, "order_id": job.OrderID, "round": job.Round},
		bson.M{"$setOnInsert": job}, options.UpdateOne().SetUpsert(true))
	return err
}

// refundableMarketBids 只选一个原轮仍未执行的原冻结款批次。已成交、已迁移或已退款的行全部排除。
func (s *Store) refundableMarketBids(ctx context.Context, job ItemMarketRefundJob) ([]ItemMarketBid, error) {
	cursor, err := s.DB.Collection(itemMarketBids).Find(ctx, marketRefundableFilter(job),
		options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}).SetLimit(itemMarketRefundBatch))
	if err != nil {
		return nil, err
	}
	var batch []ItemMarketBid
	err = cursor.All(ctx, &batch)
	return batch, err
}

// marketRefundableFilter 钉住原订单、运行服、预购轮和仍占有资金的状态。
func marketRefundableFilter(job ItemMarketRefundJob) bson.M {
	return bson.M{"run_server_id": job.RunID, "order_id": job.OrderID, "round": job.Round, "state": itemBidFrozen}
}

// advanceItemMarketRefund 在同一事务逐笔写原入账凭据和防重复标记。
func (s *Store) advanceItemMarketRefund(ctx context.Context, due ItemMarketRefundJob, now time.Time) error {
	batch, err := s.refundableMarketBids(ctx, due)
	if err != nil {
		return err
	}
	accounts := make([]string, 0, len(batch))
	for _, bid := range batch {
		accounts = append(accounts, bid.Buyer.AccountID)
	}
	return s.itemMarketSystemTx(ctx, accounts, func(tc context.Context) error {
		jobs := s.DB.Collection(itemMarketRefundJobs)
		var job ItemMarketRefundJob
		err := jobs.FindOne(tc, bson.M{"_id": due.ID, "run_server_id": s.Cfg.RunID, "next_check_at": due.NextCheckAt}).Decode(&job)
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil
		}
		if err != nil {
			return err
		}
		if job.OrderID != due.OrderID || job.Round != due.Round || job.Round <= 0 {
			return Fail(p.CodeOperationConflict, "道具市场退款原轮已变化")
		}
		var order ItemMarketOrder
		err = s.DB.Collection(itemMarketOrders).FindOne(tc, bson.M{"_id": job.OrderID, "run_server_id": job.RunID}).Decode(&order)
		if err != nil {
			return authorityReadError(err, p.CodeOperationConflict, "退款缺少原订单")
		}
		if order.Round < job.Round || order.Round == job.Round && order.Phase != ItemMarketSold {
			return Fail(p.CodeOperationConflict, "原预购轮尚未完成成交或加价")
		}
		for _, bid := range batch {
			if err := s.refundItemMarketBid(tc, job, order, bid, now); err != nil {
				return err
			}
		}
		filter := bson.M{"_id": job.ID, "next_check_at": job.NextCheckAt}
		err = s.DB.Collection(itemMarketBids).FindOne(tc, marketRefundableFilter(job)).Err()
		if errors.Is(err, mongo.ErrNoDocuments) {
			_, err = jobs.DeleteOne(tc, filter)
			return err
		}
		if err != nil {
			return err
		}
		// 让出一个持久时钟单位，保证同一毫秒内到期的其它订单也有推进机会。
		_, err = jobs.UpdateOne(tc, filter, bson.M{"$set": bson.M{"next_check_at": now.Add(time.Millisecond)}})
		return err
	})
}

// refundItemMarketBid 只返原冻结款，不读取当前月卡或向同账号其他角色发款。
func (s *Store) refundItemMarketBid(ctx context.Context, job ItemMarketRefundJob, order ItemMarketOrder, candidate ItemMarketBid, at time.Time) error {
	bids := s.DB.Collection(itemMarketBids)
	filter := marketRefundableFilter(job)
	filter["_id"] = candidate.ID
	var bid ItemMarketBid
	err := bids.FindOne(ctx, filter).Decode(&bid)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil
	}
	if err != nil {
		return err
	}
	if candidate.Buyer != bid.Buyer || !validMarketSettlementParty(bid.Buyer) || bid.Buyer.AccountID == order.Seller.AccountID || bid.Amount <= 0 {
		return Fail(p.CodeOperationConflict, "道具市场退款原角色或金额无效")
	}
	claim := ItemMarketClaim{ID: marketFactID("market-claim", order.ID, "refund", bid.ID), OrderID: order.ID,
		Owner: bid.Buyer, Kind: ItemMarketClaimRefund, Revision: 1, Yuanbao: bid.Amount, CreatedAt: at}
	if err := s.insertItemMarketClaim(ctx, claim); err != nil {
		return err
	}
	changed, err := bids.UpdateOne(ctx, filter, bson.M{"$set": bson.M{"state": itemBidRefunded, "ended_at": at}})
	if err != nil {
		return err
	}
	if changed.MatchedCount != 1 {
		return Fail(p.CodeOperationConflict, "原冻结款已处理")
	}
	_, err = s.DB.Collection("audit_outbox").InsertOne(ctx, bson.M{"_id": "item_market_refund:" + bid.ID,
		"kind": "item_market_refund", "order_id": order.ID, "bid_id": bid.ID, "round": bid.Round,
		"role_id": bid.Buyer.RoleID, "account_id": bid.Buyer.AccountID, "origin_server_id": bid.Buyer.Origin,
		"run_server_id": job.RunID, "claim_id": claim.ID, "yuanbao": bid.Amount,
		"rule_version": order.RuleVersion, "config_hash": order.ConfigHash, "at": at})
	return err
}
