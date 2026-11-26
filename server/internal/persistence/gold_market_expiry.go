// 本文件在金币委托持久期限过后，将未成交托管资产退回原角色。
package persistence

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// expireGoldMarketOrder 只结清已到期且修订未被成交或撤单消费的原余量。
func (s *Store) expireGoldMarketOrder(ctx context.Context, expected GoldMarketOrder, at time.Time) error {
	return s.marketSystemTx(ctx, "金币市场", []string{expected.Owner.AccountID}, func(tc context.Context) error {
		now := goldMarketClock(at)
		order, err := s.findGoldMarketOrder(tc, bson.M{
			"_id": expected.ID, "run_server_id": s.Cfg.RunID, "state": GoldMarketActive, "revision": expected.Revision,
			"expires_at": bson.M{"$lte": now},
		}, nil)
		if err != nil {
			return err
		}
		if !sameGoldMarketOrderVersion(order, expected) {
			return errGoldMarketWorkChanged
		}
		if err := validateGoldMarketOpenOrder(order); err != nil {
			return err
		}
		claim := goldMarketReturnClaim(order, GoldMarketReasonExpiry, now)
		if err := s.insertGoldMarketClaim(tc, claim); err != nil {
			return err
		}
		order.State, order.ClosedAt, order.PendingMatch = GoldMarketExpired, now, false
		order.FrozenGold, order.FrozenYuanbao = 0, 0
		if err := s.updateGoldMarketOrder(tc, order); err != nil {
			return err
		}
		_, err = s.DB.Collection("audit_outbox").InsertOne(tc, bson.M{
			"_id": "gold_market_expired:" + order.ID, "kind": "gold_market_expired", "order_id": order.ID,
			"role_id": order.Owner.RoleID, "account_id": order.Owner.AccountID, "origin_server_id": order.Owner.Origin,
			"run_server_id": order.RunID, "side": order.Side, "remaining_lots": order.RemainingLots,
			"gold": claim.Gold, "yuanbao": claim.Yuanbao, "claim_id": claim.ID,
			"expires_at": order.ExpiresAt, "rule_version": order.RuleVersion, "config_hash": order.ConfigHash, "at": now,
		})
		return err
	})
}
