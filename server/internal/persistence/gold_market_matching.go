// 本文件将当前运行服最先收到的金币委托与簿内最佳订单原子撮合。
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

// validateGoldMarketOpenOrder 要求未成交手数和原冻结金额完全对应，并在相乘前核对边界。
func validateGoldMarketOpenOrder(order GoldMarketOrder) error {
	if !lowerHex(order.ID, 16) || !validMarketSettlementParty(order.Owner) || order.RunID == 0 || order.Sequence <= 0 ||
		order.State != GoldMarketActive || order.Revision <= 0 || order.Revision == math.MaxInt64 ||
		order.RemainingLots <= 0 || order.InitialLots < order.RemainingLots || order.LimitPrice <= 0 || order.GoldPerLot <= 0 ||
		order.InitialLots > math.MaxInt64/order.LimitPrice || order.InitialLots > math.MaxInt64/order.GoldPerLot || order.TaxDivisor != 10 ||
		order.AcceptedAt.IsZero() || !order.ExpiresAt.After(order.AcceptedAt) || !order.ClosedAt.IsZero() ||
		order.RuleVersion == 0 || !lowerHex(order.ConfigHash, 32) {
		return Fail(p.CodeOperationConflict, "金币市场原委托资料无效")
	}
	switch order.Side {
	case GoldMarketBuy:
		if order.FrozenYuanbao == order.RemainingLots*order.LimitPrice && order.FrozenGold == 0 {
			return nil
		}
	case GoldMarketSell:
		if order.FrozenGold == order.RemainingLots*order.GoldPerLot && order.FrozenYuanbao == 0 {
			return nil
		}
	}
	return Fail(p.CodeOperationConflict, "金币市场原冻结资产与委托余量不符")
}

// matchGoldMarketIncoming 在涉及双方的账号组事务里重新选最优对手，再决定成交或将新单转为簿内单。
func (s *Store) matchGoldMarketIncoming(ctx context.Context, expectedIncoming, expectedMaker GoldMarketOrder, at time.Time) error {
	accounts := []string{expectedIncoming.Owner.AccountID}
	if expectedMaker.ID != "" {
		accounts = append(accounts, expectedMaker.Owner.AccountID)
	}
	return s.marketSystemTx(ctx, "金币市场", accounts, func(tc context.Context) error {
		now := goldMarketClock(at)
		incoming, err := s.nextGoldMarketIncoming(tc, now)
		if err != nil {
			return err
		}
		if !sameGoldMarketOrderVersion(incoming, expectedIncoming) {
			return errGoldMarketWorkChanged
		}
		if err := validateGoldMarketOpenOrder(incoming); err != nil {
			return err
		}
		maker, err := s.bestGoldMarketMaker(tc, incoming, now)
		if err != nil {
			return err
		}
		if !sameGoldMarketOrderVersion(maker, expectedMaker) {
			return errGoldMarketWorkChanged
		}
		if maker.ID == "" {
			// 入簿只清理内部标记，订单余量和用于原角色撤单的修订不变。
			changed, err := s.DB.Collection(goldMarketOrders).UpdateOne(tc,
				bson.M{"_id": incoming.ID, "run_server_id": incoming.RunID, "revision": incoming.Revision, "pending_match": true},
				bson.M{"$set": bson.M{"pending_match": false}})
			if err == nil && changed.MatchedCount != 1 {
				return errGoldMarketWorkChanged
			}
			return err
		}
		if err := validateGoldMarketOpenOrder(maker); err != nil {
			return err
		}
		return s.confirmGoldMarketFill(tc, maker, incoming, now)
	})
}

// confirmGoldMarketFill 消耗两单对应的冻结部分，原子写入三类待领、两笔余量、共享税余数和成交审计。
func (s *Store) confirmGoldMarketFill(ctx context.Context, maker, taker GoldMarketOrder, at time.Time) error {
	buy, sell := maker, taker
	if taker.Side == GoldMarketBuy {
		buy, sell = taker, maker
	}
	if buy.Side != GoldMarketBuy || sell.Side != GoldMarketSell || buy.LimitPrice < sell.LimitPrice ||
		maker.RunID != taker.RunID || maker.Sequence >= taker.Sequence || maker.PendingMatch || !taker.PendingMatch ||
		buy.Owner.AccountID == sell.Owner.AccountID || buy.GoldPerLot != sell.GoldPerLot {
		return Fail(p.CodeOperationConflict, "金币市场原撮合对手、时序或每手规模无效")
	}
	lots := min(buy.RemainingLots, sell.RemainingLots)
	// 每对订单首笔成交后至少耗尽一单，所以原订单对即可稳定标识成交。
	fact := GoldMarketFill{ID: marketFactID("gold-market-fill", buy.ID, sell.ID), RunID: maker.RunID,
		BuyOrderID: buy.ID, SellOrderID: sell.ID, MakerOrderID: maker.ID, TakerOrderID: taker.ID,
		Buyer: buy.Owner, Seller: sell.Owner, Lots: lots, UnitPrice: maker.LimitPrice,
		Gross: maker.LimitPrice * lots, BuyerRefund: (buy.LimitPrice - maker.LimitPrice) * lots, Gold: sell.GoldPerLot * lots,
		MakerRuleVersion: maker.RuleVersion, MakerConfigHash: maker.ConfigHash,
		TakerRuleVersion: taker.RuleVersion, TakerConfigHash: taker.ConfigHash, CreatedAt: at}
	reason, err := s.goldMarketReferenceExclusion(ctx, buy.Owner.AccountID, sell.Owner.AccountID)
	if err != nil {
		return err
	}
	fact.ReferenceExcluded = reason != ""
	fact.Tax, err = s.chargeMarketSellerTax(ctx, sell.Owner, fact.Gross, sell.TaxDivisor)
	if err != nil {
		return err
	}
	if err := s.insertGoldMarketFill(ctx, &fact); err != nil {
		return err
	}
	if err := s.writeGoldMarketFillClaims(ctx, fact); err != nil {
		return err
	}
	buy.FrozenYuanbao -= fact.Gross + fact.BuyerRefund
	sell.FrozenGold -= fact.Gold
	for _, order := range []*GoldMarketOrder{&buy, &sell} {
		order.RemainingLots -= lots
		if order.RemainingLots == 0 {
			order.State, order.ClosedAt, order.PendingMatch = GoldMarketFilled, at, false
		}
		if err := s.updateGoldMarketOrder(ctx, *order); err != nil {
			return err
		}
	}
	_, err = s.DB.Collection("audit_outbox").InsertOne(ctx, bson.M{
		"_id": "gold_market_fill:" + fact.ID, "kind": "gold_market_fill", "run_server_id": fact.RunID,
		"role_id": fact.Seller.RoleID, "account_id": fact.Seller.AccountID, "origin_server_id": fact.Seller.Origin,
		"fill_id": fact.ID, "fill_sequence": fact.Sequence, "buy_order_id": fact.BuyOrderID, "sell_order_id": fact.SellOrderID,
		"maker_order_id": fact.MakerOrderID, "taker_order_id": fact.TakerOrderID, "lots": fact.Lots,
		"unit_price": fact.UnitPrice, "gross": fact.Gross, "tax": fact.Tax, "buyer_refund": fact.BuyerRefund, "gold": fact.Gold,
		"reference_excluded": fact.ReferenceExcluded, "reference_exclusion_reason": reason,
		"maker_rule_version": fact.MakerRuleVersion, "maker_config_hash": fact.MakerConfigHash,
		"taker_rule_version": fact.TakerRuleVersion, "taker_config_hash": fact.TakerConfigHash, "at": at,
	})
	return err
}

// updateGoldMarketOrder 以原修订竞争成交、到期或用户撤单。
func (s *Store) updateGoldMarketOrder(ctx context.Context, order GoldMarketOrder) error {
	oldRevision := order.Revision
	order.Revision++
	return s.replaceGoldMarketOrder(ctx, order, oldRevision)
}

// writeGoldMarketFillClaims 分别把金币、税后元宝和价格改善退额交给各自的原角色。
func (s *Store) writeGoldMarketFillClaims(ctx context.Context, fill GoldMarketFill) error {
	claims := []GoldMarketClaim{
		{ID: marketFactID("gold-market-claim", fill.ID, "bought-gold"), OrderID: fill.BuyOrderID, FillID: fill.ID,
			Owner: fill.Buyer, Kind: GoldMarketClaimGold, Reason: GoldMarketReasonFill, Revision: 1, Gold: fill.Gold, CreatedAt: fill.CreatedAt},
		{ID: marketFactID("gold-market-claim", fill.ID, "seller-proceeds"), OrderID: fill.SellOrderID, FillID: fill.ID,
			Owner: fill.Seller, Kind: GoldMarketClaimProceeds, Reason: GoldMarketReasonFill, Revision: 1, Yuanbao: fill.Gross - fill.Tax, CreatedAt: fill.CreatedAt},
		{ID: marketFactID("gold-market-claim", fill.ID, "price-difference"), OrderID: fill.BuyOrderID, FillID: fill.ID,
			Owner: fill.Buyer, Kind: GoldMarketClaimYuanbaoRefund, Reason: GoldMarketReasonPriceDifference, Revision: 1, Yuanbao: fill.BuyerRefund, CreatedAt: fill.CreatedAt},
	}
	for _, claim := range claims {
		if claim.Gold == 0 && claim.Yuanbao == 0 {
			continue
		}
		if !validGoldMarketClaim(claim) {
			return Fail(p.CodeOperationConflict, "金币市场成交应领资产无效")
		}
		if err := s.insertGoldMarketClaim(ctx, claim); err != nil {
			return err
		}
	}
	return nil
}

const goldMarketReferenceExclusions = "gold_market_reference_exclusions" // 经审核仅排除行情取样的关联账号对。

// goldMarketReferencePair 以相同持久身份识别交换买卖方向后的关联账号。
func goldMarketReferencePair(first, second string) (id, accountA, accountB string) {
	accountA, accountB = min(first, second), max(first, second)
	return marketFactID("gold-market-reference-exclusion", accountA, accountB), accountA, accountB
}

// goldMarketReferenceExclusion 保留正常交付，同时冻结该成交能否参与后续加权行情计算。
func (s *Store) goldMarketReferenceExclusion(ctx context.Context, first, second string) (string, error) {
	id, accountA, accountB := goldMarketReferencePair(first, second)
	var exclusion struct {
		AccountA string `bson:"account_a"` // 排序后的第一账号。
		AccountB string `bson:"account_b"` // 排序后的第二账号。
		Reason   string `bson:"reason"`    // 审核确认采用的关联原因。
	}
	err := s.DB.Collection(goldMarketReferenceExclusions).FindOne(ctx, bson.M{"_id": id}).Decode(&exclusion)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	if exclusion.AccountA != accountA || exclusion.AccountB != accountB || exclusion.Reason == "" {
		return "", Fail(p.CodeOperationConflict, "金币市场关联账号审核资料无效")
	}
	return exclusion.Reason, nil
}
