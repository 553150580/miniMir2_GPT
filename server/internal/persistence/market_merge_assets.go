// 本文件从一个已停服的运行服选取至多一笔需清退的市场托管。
package persistence

import (
	"context"
	"errors"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	p "minimir/internal/protocol"
)

const (
	marketMergeItemOrder = "item_order"      // 拍卖或直购原件余量返还。
	marketMergeItemBid   = "item_bid"        // 一笔仍被冻结的预购原路退款。
	marketMergeItemJob   = "item_refund_job" // 无剩余冻结款后清除旧的退款任务。
	marketMergeGoldOrder = "gold_order"      // 金币委托的未成交部分按原币种退回。
)

// MarketMergeAssetStep 固定合服清退实际推进的原资产身份，空 Kind 表示本服已排空。
type MarketMergeAssetStep struct {
	Kind     string `bson:"kind" json:"kind"`                             // 原托管资产或已清退款任务的类别。
	RunID    uint64 `bson:"run_server_id" json:"run_server_id"`           // 托管扣款或入单时所属的原运行服。
	SourceID string `bson:"source_id" json:"source_id"`                   // 本次关闭的原订单、预购或任务身份。
	ClaimID  string `bson:"claim_id,omitempty" json:"claim_id,omitempty"` // 能由原角色领取的原路返还凭据。
}

// drainMarketMergeRun 必须在计划和相应运行服维护屏障已实际确认的事务中调用。
func (s *Store) drainMarketMergeRun(ctx context.Context, planID string, runID uint64, at time.Time) (MarketMergeAssetStep, error) {
	if mongo.SessionFromContext(ctx) == nil || !lowerHex(planID, 16) || runID == 0 || at.IsZero() {
		return MarketMergeAssetStep{}, Fail(p.CodeOperationConflict, "市场合服清退缺少原计划、运行服或事务")
	}
	at = bossTime(at)
	// 先收回原拍品，使冻结预购无法再产生一笔新成交。
	for _, phase := range []uint32{ItemMarketAuction, ItemMarketDirect} {
		var order ItemMarketOrder
		err := s.DB.Collection(itemMarketOrders).FindOne(ctx, bson.M{"run_server_id": runID, "phase": phase},
			options.FindOne().SetSort(bson.D{{Key: "_id", Value: 1}})).Decode(&order)
		if !errors.Is(err, mongo.ErrNoDocuments) {
			if err != nil {
				return MarketMergeAssetStep{}, err
			}
			return s.returnMergeItemMarketOrder(ctx, planID, order, at)
		}
	}
	var bid ItemMarketBid
	err := s.DB.Collection(itemMarketBids).FindOne(ctx, bson.M{"run_server_id": runID, "state": itemBidFrozen},
		options.FindOne().SetSort(bson.D{{Key: "_id", Value: 1}})).Decode(&bid)
	if !errors.Is(err, mongo.ErrNoDocuments) {
		if err != nil {
			return MarketMergeAssetStep{}, err
		}
		return s.returnMergeItemMarketBid(ctx, planID, bid, at)
	}
	var job ItemMarketRefundJob
	err = s.DB.Collection(itemMarketRefundJobs).FindOne(ctx, bson.M{"run_server_id": runID}, options.FindOne().SetSort(
		bson.D{{Key: "next_check_at", Value: 1}, {Key: "_id", Value: 1}})).Decode(&job)
	if !errors.Is(err, mongo.ErrNoDocuments) {
		if err != nil {
			return MarketMergeAssetStep{}, err
		}
		return s.closeMergeItemMarketRefundJob(ctx, planID, job, at)
	}
	order, err := s.findGoldMarketOrder(ctx, bson.M{"run_server_id": runID, "state": GoldMarketActive},
		bson.D{{Key: "expires_at", Value: 1}, {Key: "sequence", Value: 1}})
	if err != nil || order.ID == "" {
		return MarketMergeAssetStep{}, err
	}
	return s.returnMergeGoldMarketOrder(ctx, planID, order, at)
}

// marketMergeRunHasDebt 在封账事务内核对当前活跃市场和退款义务是否已全部清退。
func (s *Store) marketMergeRunHasDebt(ctx context.Context, runID uint64) (bool, error) {
	queries := []struct {
		collection string // 保存原托管义务的集合。
		filter     bson.M // 可沿清退索引查询的具体待处理阶段。
	}{
		{itemMarketOrders, bson.M{"phase": bson.M{"$in": []uint32{ItemMarketAuction, ItemMarketDirect}}}},
		{itemMarketBids, bson.M{"state": itemBidFrozen}},
		{itemMarketRefundJobs, bson.M{}},
		{goldMarketOrders, bson.M{"state": GoldMarketActive}},
	}
	fields := options.FindOne().SetProjection(bson.M{"_id": 1})
	for _, query := range queries {
		query.filter["run_server_id"] = runID
		err := s.DB.Collection(query.collection).FindOne(ctx, query.filter, fields).Err()
		if !errors.Is(err, mongo.ErrNoDocuments) {
			return err == nil, err
		}
	}
	return false, nil
}

// marketMergeAssetIndexes 增加按运行服逐笔排空冻结预购的路径，其余市场清退复用原有索引。
func marketMergeAssetIndexes() []mongo.IndexModel {
	return []mongo.IndexModel{{Keys: bson.D{{Key: "run_server_id", Value: 1}, {Key: "state", Value: 1}, {Key: "_id", Value: 1}}}}
}

// writeMarketMergeAssetAudit 记录实际关闭的原订单、原债务及返还事实。
func (s *Store) writeMarketMergeAssetAudit(ctx context.Context, planID string, step MarketMergeAssetStep,
	owner MarketParty, at time.Time, details bson.M) error {
	row := bson.M{"_id": marketFactID("market-merge-audit", step.Kind, step.SourceID), "kind": "market_merge_asset",
		"plan_id": planID, "asset_kind": step.Kind, "source_id": step.SourceID, "run_server_id": step.RunID, "at": at, "details": details}
	if step.ClaimID != "" {
		row["claim_id"] = step.ClaimID
		row["role_id"], row["account_id"], row["origin_server_id"] = owner.RoleID, owner.AccountID, owner.Origin
	}
	_, err := s.DB.Collection("audit_outbox").InsertOne(ctx, row)
	return err
}
