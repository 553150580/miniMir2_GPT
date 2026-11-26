// 本文件返还合服清退中的托管道具、冻结预购，并收尾已清的旧退款任务。
package persistence

import (
	"context"
	"errors"
	"math"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	p "minimir/internal/protocol"
)

// returnMergeItemMarketOrder 收回拍卖或直购的原件余量，未付款的抽签本身不具备交付资格。
func (s *Store) returnMergeItemMarketOrder(ctx context.Context, planID string, order ItemMarketOrder, at time.Time) (MarketMergeAssetStep, error) {
	if !lowerHex(order.ID, 16) || !validMarketSettlementParty(order.Seller) || order.Revision <= 0 || order.Revision == math.MaxInt64 ||
		(order.Phase != ItemMarketAuction && order.Phase != ItemMarketDirect) || order.Round <= 0 ||
		!lowerHex(order.Item.ID, 16) || order.Item.ItemID == 0 || order.Item.Version <= 0 || order.Item.Quantity <= 0 ||
		order.ActiveBidCount < 0 || !order.ClosedAt.IsZero() || order.RuleVersion == 0 || !lowerHex(order.ConfigHash, 32) {
		return MarketMergeAssetStep{}, Fail(p.CodeOperationConflict, "合服清退的原道具订单或托管物品无效")
	}
	item := order.Item
	claim := ItemMarketClaim{ID: marketFactID("market-claim", order.ID, "cancel"), OrderID: order.ID,
		Owner: order.Seller, Kind: ItemMarketClaimItem, Revision: 1, Item: &item, CreatedAt: at}
	beforePhase, beforeWinner, beforeBids, revision := order.Phase, order.WinnerBidID, order.ActiveBidCount, order.Revision
	order.Phase, order.ClosedAt, order.Revision = ItemMarketCancelled, at, revision+1
	order.Item.Quantity, order.ActiveBidCount, order.WinnerBidID = 0, 0, ""
	if err := s.replaceItemMarketOrder(ctx, order, revision); err != nil {
		return MarketMergeAssetStep{}, err
	}
	if err := s.insertItemMarketClaim(ctx, claim); err != nil {
		return MarketMergeAssetStep{}, err
	}
	step := MarketMergeAssetStep{Kind: marketMergeItemOrder, RunID: order.RunID, SourceID: order.ID, ClaimID: claim.ID}
	err := s.writeMarketMergeAssetAudit(ctx, planID, step, claim.Owner, at, bson.M{
		"phase_before": beforePhase, "unsettled_drawn_bid_id": beforeWinner, "current_round_bids": beforeBids,
		"round": order.Round, "revision": order.Revision, "quantity": item.Quantity, "item_instance_id": item.ID,
		"rule_version": order.RuleVersion, "config_hash": order.ConfigHash})
	return step, err
}

// returnMergeItemMarketBid 沿原冻结款返回原角色，早轮输家和已成交订单的冻结输家都能复用。
func (s *Store) returnMergeItemMarketBid(ctx context.Context, planID string, bid ItemMarketBid, at time.Time) (MarketMergeAssetStep, error) {
	if !lowerHex(bid.ID, 16) || !lowerHex(bid.OrderID, 16) || bid.Round <= 0 || bid.Ordinal <= 0 || bid.State != itemBidFrozen {
		return MarketMergeAssetStep{}, Fail(p.CodeOperationConflict, "合服清退的原预购身份或阶段无效")
	}
	var order ItemMarketOrder
	err := s.DB.Collection(itemMarketOrders).FindOne(ctx, bson.M{"_id": bid.OrderID, "run_server_id": bid.RunID}).Decode(&order)
	if err != nil {
		return MarketMergeAssetStep{}, authorityReadError(err, p.CodeOperationConflict, "合服清退的预购缺少原订单")
	}
	if order.Round < bid.Round || (order.Phase != ItemMarketSold && order.Phase != ItemMarketCancelled && order.Phase != ItemMarketExpired) ||
		order.Phase == ItemMarketSold && order.WinnerBidID == bid.ID {
		return MarketMergeAssetStep{}, Fail(p.CodeOperationConflict, "原预购尚可成交或与已成交的赢家冲突")
	}
	// 原退款入口保留相同的待领 ID、原币种、归属和资金流水。
	job := ItemMarketRefundJob{OrderID: bid.OrderID, RunID: bid.RunID, Round: bid.Round}
	if err := s.refundItemMarketBid(ctx, job, order, bid, at); err != nil {
		return MarketMergeAssetStep{}, err
	}
	step := MarketMergeAssetStep{Kind: marketMergeItemBid, RunID: bid.RunID, SourceID: bid.ID,
		ClaimID: marketFactID("market-claim", order.ID, "refund", bid.ID)}
	err = s.writeMarketMergeAssetAudit(ctx, planID, step, bid.Buyer, at, bson.M{
		"order_id": order.ID, "round": bid.Round, "yuanbao": bid.Amount,
		"rule_version": order.RuleVersion, "config_hash": order.ConfigHash})
	return step, err
}

// closeMergeItemMarketRefundJob 仅在原轮已无冻结资产时删除不再需要的调度任务。
func (s *Store) closeMergeItemMarketRefundJob(ctx context.Context, planID string, job ItemMarketRefundJob, at time.Time) (MarketMergeAssetStep, error) {
	if !lowerHex(job.ID, 16) || !lowerHex(job.OrderID, 16) || job.Round <= 0 || job.NextCheckAt.IsZero() {
		return MarketMergeAssetStep{}, Fail(p.CodeOperationConflict, "合服清退的旧道具退款任务无效")
	}
	err := s.DB.Collection(itemMarketBids).FindOne(ctx, marketRefundableFilter(job)).Err()
	if !errors.Is(err, mongo.ErrNoDocuments) {
		if err == nil {
			err = Fail(p.CodeOperationConflict, "旧道具退款任务仍持有未退资金")
		}
		return MarketMergeAssetStep{}, err
	}
	removed, err := s.DB.Collection(itemMarketRefundJobs).DeleteOne(ctx, bson.M{"_id": job.ID, "run_server_id": job.RunID,
		"order_id": job.OrderID, "round": job.Round, "next_check_at": job.NextCheckAt})
	if err != nil {
		return MarketMergeAssetStep{}, err
	}
	if removed.DeletedCount != 1 {
		return MarketMergeAssetStep{}, Fail(p.CodeOperationConflict, "旧道具退款任务已变化")
	}
	step := MarketMergeAssetStep{Kind: marketMergeItemJob, RunID: job.RunID, SourceID: job.ID}
	err = s.writeMarketMergeAssetAudit(ctx, planID, step, MarketParty{}, at, bson.M{"order_id": job.OrderID, "round": job.Round})
	return step, err
}
