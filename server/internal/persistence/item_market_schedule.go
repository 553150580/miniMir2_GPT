// 本文件以真实市场时钟，从当前运行服推进一笔到期订单或退款批次。
package persistence

import (
	"context"
	"crypto/rand"
	"errors"
	"slices"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	p "minimir/internal/protocol"
)

// AdvanceItemMarket 尝试推进一个到期步骤。没有已到期工作时不写围栏。
// 已收取的托管资金按订单的原规则完成，即使当前发布关闭新交易。
func (s *Store) AdvanceItemMarket(ctx context.Context, now time.Time) (bool, error) {
	if now.IsZero() || s.Cfg.RunID == 0 {
		return false, nil
	}
	ctx, stop := context.WithTimeout(ctx, transactionTimeout)
	defer stop()
	now = bossTime(now)
	auction, err := s.dueItemMarketOrder(ctx, ItemMarketAuction, "auction_ends_at", now)
	if err != nil {
		return false, err
	}
	direct, err := s.dueItemMarketOrder(ctx, ItemMarketDirect, "direct_ends_at", now)
	if err != nil {
		return false, err
	}
	var refund ItemMarketRefundJob
	err = s.DB.Collection(itemMarketRefundJobs).FindOne(ctx, bson.M{"run_server_id": s.Cfg.RunID, "next_check_at": bson.M{"$lte": now}},
		options.FindOne().SetSort(bson.D{{Key: "next_check_at", Value: 1}, {Key: "_id", Value: 1}})).Decode(&refund)
	if err != nil && !errors.Is(err, mongo.ErrNoDocuments) {
		return false, err
	}
	// 退款批次完成后更新自己的下次水位，旧订单的大名单不会独占到期调度。
	if refund.ID != "" && (auction.ID == "" || !auction.AuctionEndsAt.Before(refund.NextCheckAt)) &&
		(direct.ID == "" || !direct.DirectEndsAt.Before(refund.NextCheckAt)) {
		err = s.advanceItemMarketRefund(ctx, refund, now)
		return err == nil, err
	}
	if direct.ID != "" && (auction.ID == "" || direct.DirectEndsAt.Before(auction.AuctionEndsAt)) {
		err = s.finishItemMarketDirectExpiry(ctx, direct, now)
		return err == nil, err
	}
	if auction.ID != "" {
		var entropy [32]byte
		if auction.ActiveBidCount > 0 && auction.WinnerBidID == "" {
			if _, err = rand.Read(entropy[:]); err != nil {
				return false, err
			}
		}
		err = s.finishItemMarketAuction(ctx, auction, entropy, now)
		return err == nil, err
	}
	return false, nil
}

// dueItemMarketOrder 使用原阶段的有序截止索引读取最先到期的一单。
func (s *Store) dueItemMarketOrder(ctx context.Context, phase uint32, deadline string, now time.Time) (ItemMarketOrder, error) {
	var order ItemMarketOrder
	err := s.DB.Collection(itemMarketOrders).FindOne(ctx, bson.M{"run_server_id": s.Cfg.RunID, "phase": phase, deadline: bson.M{"$lte": now}},
		options.FindOne().SetSort(bson.D{{Key: deadline, Value: 1}, {Key: "_id", Value: 1}})).Decode(&order)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return ItemMarketOrder{}, nil
	}
	return order, err
}

// itemMarketSystemTx 使用真实运行服屏障和永久账号分组，完成无需在线会话的旧托管结算。
func (s *Store) itemMarketSystemTx(ctx context.Context, accounts []string, fn func(context.Context) error) error {
	if s.Cfg.RunID == 0 || s.Epoch <= 0 || s.runGuardSlots == nil {
		return Fail(p.CodeRouteChanged, "道具市场当前运行服未取得结算授权")
	}
	ctx, stop := context.WithTimeout(ctx, transactionTimeout)
	defer stop()
	var groups []uint64
	for _, account := range accounts {
		if !lowerHex(account, 16) {
			return Fail(p.CodeOperationConflict, "道具市场原结算账号无效")
		}
		groups = append(groups, runGuardGroup(account))
	}
	if len(groups) == 0 {
		groups = append(groups, 0)
	}
	slices.Sort(groups)
	groups = slices.Compact(groups)
	for _, group := range groups {
		slot, err := s.runGuardSlots.acquire(ctx, group)
		if err != nil {
			return Fail(p.CodeOperationPending, "道具市场原结算待查证")
		}
		defer func() { <-slot }()
	}
	return s.tx(ctx, func(tc context.Context) error {
		writes := make([]mongo.ClientBulkWrite, 0, len(groups))
		for _, group := range groups {
			write, err := s.runFenceWrite(tc, group)
			if err != nil {
				return err
			}
			writes = append(writes, write)
		}
		result, err := s.Client.BulkWrite(tc, writes, options.ClientBulkWrite().SetVerboseResults(true))
		if err != nil {
			return transactionBulkWriteError(err)
		}
		if !result.Acknowledged {
			return Fail(p.CodeOperationPending, "道具市场原结算未确认")
		}
		for index := range writes {
			if result.UpdateResults[index].MatchedCount != 1 {
				return Fail(p.CodeRouteChanged, "道具市场运行服写屏障失效")
			}
		}
		return fn(tc)
	})
}
