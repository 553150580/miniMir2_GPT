// 本文件按原币种退回合服范围内金币委托的未成交部分。
package persistence

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// returnMergeGoldMarketOrder 使用普通撤单的原资产身份，先前成交和佣金沿用永久事实。
func (s *Store) returnMergeGoldMarketOrder(ctx context.Context, planID string, order GoldMarketOrder, at time.Time) (MarketMergeAssetStep, error) {
	if err := validateGoldMarketOpenOrder(order); err != nil {
		return MarketMergeAssetStep{}, err
	}
	claim := goldMarketReturnClaim(order, GoldMarketReasonCancel, at)
	order.State, order.ClosedAt, order.PendingMatch = GoldMarketCancelled, at, false
	order.FrozenGold, order.FrozenYuanbao = 0, 0
	if err := s.updateGoldMarketOrder(ctx, order); err != nil {
		return MarketMergeAssetStep{}, err
	}
	if err := s.insertGoldMarketClaim(ctx, claim); err != nil {
		return MarketMergeAssetStep{}, err
	}
	step := MarketMergeAssetStep{Kind: marketMergeGoldOrder, RunID: order.RunID, SourceID: order.ID, ClaimID: claim.ID}
	err := s.writeMarketMergeAssetAudit(ctx, planID, step, claim.Owner, at, bson.M{
		"side": order.Side, "remaining_lots": order.RemainingLots, "revision": order.Revision + 1,
		"gold": claim.Gold, "yuanbao": claim.Yuanbao, "rule_version": order.RuleVersion, "config_hash": order.ConfigHash})
	return step, err
}
