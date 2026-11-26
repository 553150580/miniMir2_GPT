// 本文件按运行服和持久序号有界推进金币委托的到期或撮合。
package persistence

import (
	"context"
	"errors"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// errGoldMarketWorkChanged 表示事务内发现原候选已变，回滚围栏写入并尽快重新选择。
var errGoldMarketWorkChanged = errors.New("金币市场候选已变化")

// AdvanceGoldMarket 推进一笔到期、一对成交或一单入簿。关闭新委托仍可按原订单清偿。
func (s *Store) AdvanceGoldMarket(ctx context.Context, now time.Time) (bool, error) {
	if now.IsZero() || s.Cfg.RunID == 0 {
		return false, nil
	}
	ctx, stop := context.WithTimeout(ctx, transactionTimeout)
	defer stop()
	now = goldMarketClock(now)
	due, err := s.findGoldMarketOrder(ctx, bson.M{
		"run_server_id": s.Cfg.RunID, "state": GoldMarketActive, "expires_at": bson.M{"$lte": now},
	}, bson.D{{Key: "expires_at", Value: 1}, {Key: "sequence", Value: 1}})
	if err != nil {
		return false, err
	}
	if due.ID != "" {
		return goldMarketAdvanceResult(s.expireGoldMarketOrder(ctx, due, now))
	}
	incoming, err := s.nextGoldMarketIncoming(ctx, now)
	if err != nil || incoming.ID == "" {
		return false, err
	}
	if err := validateGoldMarketOpenOrder(incoming); err != nil {
		return false, err
	}
	maker, err := s.bestGoldMarketMaker(ctx, incoming, now)
	if err != nil {
		return false, err
	}
	return goldMarketAdvanceResult(s.matchGoldMarketIncoming(ctx, incoming, maker, now))
}

// goldMarketAdvanceResult 提醒运行服尽快重选已经被并发更新的候选。
func goldMarketAdvanceResult(err error) (bool, error) {
	if errors.Is(err, errGoldMarketWorkChanged) {
		return true, nil
	}
	return err == nil, err
}

// goldMarketClock 涵盖任务排队和事务回调重试的时间，旧时点不能延长订单期限。
func goldMarketClock(at time.Time) time.Time {
	if now := time.Now(); now.After(at) {
		at = now
	}
	return bossTime(at)
}

// nextGoldMarketIncoming 先处理已经通过持久序号排序的最早有效新委托。
func (s *Store) nextGoldMarketIncoming(ctx context.Context, at time.Time) (GoldMarketOrder, error) {
	return s.findGoldMarketOrder(ctx, bson.M{
		"run_server_id": s.Cfg.RunID, "state": GoldMarketActive, "pending_match": true, "expires_at": bson.M{"$gt": at},
	}, bson.D{{Key: "sequence", Value: 1}})
}

// bestGoldMarketMaker 只看更早、有效、价格可成交且属于其他账号的一侧委托。
func (s *Store) bestGoldMarketMaker(ctx context.Context, incoming GoldMarketOrder, at time.Time) (GoldMarketOrder, error) {
	side, priceOp, priceSort := GoldMarketBuy, "$gte", -1
	if incoming.Side == GoldMarketBuy {
		side, priceOp, priceSort = GoldMarketSell, "$lte", 1
	}
	return s.findGoldMarketOrder(ctx, bson.M{
		"run_server_id": s.Cfg.RunID, "state": GoldMarketActive, "side": side,
		"sequence": bson.M{"$lt": incoming.Sequence}, "expires_at": bson.M{"$gt": at},
		"limit_price": bson.M{priceOp: incoming.LimitPrice}, "owner.account_id": bson.M{"$ne": incoming.Owner.AccountID},
	}, bson.D{{Key: "limit_price", Value: priceSort}, {Key: "sequence", Value: 1}})
}

// findGoldMarketOrder 读取有序单笔候选，统一将没有可用委托解释为空结果。
func (s *Store) findGoldMarketOrder(ctx context.Context, filter bson.M, order bson.D) (GoldMarketOrder, error) {
	var found GoldMarketOrder
	query := options.FindOne()
	if len(order) > 0 {
		query.SetSort(order)
	}
	err := s.DB.Collection(goldMarketOrders).FindOne(ctx, filter, query).Decode(&found)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return GoldMarketOrder{}, nil
	}
	return found, err
}

// sameGoldMarketOrderVersion 同时确认原版本和受围栏保护的账号归属，空候选也可比较。
func sameGoldMarketOrderVersion(current, expected GoldMarketOrder) bool {
	return current.ID == expected.ID && current.Revision == expected.Revision && current.Owner == expected.Owner
}
