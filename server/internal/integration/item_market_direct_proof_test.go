// 本文件复用正常市场排程验收卖家原库存、直购、佣金、交付及收款重放。
package integration

import (
	"context"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"google.golang.org/protobuf/proto"
	"minimir/internal/domain"
	"minimir/internal/persistence"
)

// marketDirectSaleProof 关联卖家原材料实例、实际成交以及可重查的原买入和收款操作。
type marketDirectSaleProof struct {
	SourceItemID string                           `json:"source_item_instance_id"` // 卖家原库存实例。
	BuyOpID      string                           `json:"buy_operation_id"`        // 真实买入操作编号。
	PayoutOpID   string                           `json:"payout_operation_id"`     // 卖家净收原领取操作。
	Fact         persistence.ItemMarketSettlement `json:"settlement"`              // 原成交、物品、买卖双方、费用和时间。
}

// settleMarketDirectForTest 等真实市场进入直购并核对原成交、税、交付、到账和重放。
func settleMarketDirectForTest(t *testing.T, ctx context.Context, store *persistence.Store, seller, buyer *client, orderID []byte, original domain.Item, quantity, price, priorGross int64) marketDirectSaleProof {
	t.Helper()
	collection := store.DB.Collection("item_orders")
	key := bson.M{"_id": domain.Hex(orderID)}
	var order persistence.ItemMarketOrder
	must(t, collection.FindOne(ctx, key).Decode(&order))
	rules := store.Cfg.Fixture.ItemMarket
	if order.Phase != persistence.ItemMarketAuction || order.Item.Quantity != quantity || order.Item.Bound || order.Item.SourceID != original.SourceID ||
		order.Item.OrePurity != original.OrePurity ||
		order.UnitPrice != price || order.AuctionEndsAt.Sub(order.ListedAt) != time.Duration(rules.AuctionSeconds)*time.Second || order.TaxDivisor != rules.TaxDivisor {
		t.Fatal("原订单未保存实际来源、价格、佣金与配置期限", order)
	}
	// 仅在隔离库缩短确认过的竞购截止。由正常市场排程转直购，成交和收款均提交真实请求。
	key["revision"] = order.Revision
	changed, err := collection.UpdateOne(ctx, key, bson.M{"$set": bson.M{"auction_ends_at": time.Now().Add(time.Second).UTC().Truncate(time.Millisecond)}})
	must(t, err)
	if changed.ModifiedCount != 1 {
		t.Fatal("原竞购订单在隔离截止调整前已变化", order.ID)
	}
	view := marketOrderWS(buyer, orderID, persistence.ItemMarketDirect)
	buy := termsForMarketAction(view, marketDirectBuyAction, quantity)
	bought := marketActionWS(buyer, buy, 0)
	gross := quantity * price
	tax := (priorGross+gross)/order.TaxDivisor - priorGross/order.TaxDivisor
	if bought.Quantity != quantity || bought.YuanbaoDeltaUnits != -gross || bought.TaxYuanbaoUnits != tax ||
		!proto.Equal(bought, marketActionWS(buyer, buy, 0)) {
		t.Fatal("买入回执、原佣金或重放不一致", bought)
	}
	payout := itemMarketClaimForTest(marketEscrowWS(seller, orderID, persistence.ItemMarketClaimProceeds))
	paid := marketActionWS(seller, payout, 0)
	if paid.YuanbaoDeltaUnits != gross-tax || !proto.Equal(paid, marketActionWS(seller, payout, 0)) {
		t.Fatal("卖家净收或领取重放不一致", paid)
	}
	var fact persistence.ItemMarketSettlement
	facts := store.DB.Collection("item_market_settlements")
	filter := bson.M{"order_id": order.ID}
	must(t, facts.FindOne(ctx, filter).Decode(&fact))
	count, err := facts.CountDocuments(ctx, filter)
	must(t, err)
	delivered := marketBagItem(t, buyer, fact.Item.ID)
	if count != 1 || fact.Item.ItemID != original.ItemID || fact.Item.Quantity != quantity || fact.Item.Bound ||
		fact.Item.OrePurity != original.OrePurity || fact.Item.SourceKind != original.SourceKind || fact.Item.SourceID != original.SourceID || fact.Item.Quality != original.Quality ||
		fact.UnitPrice != price || fact.Gross != gross || fact.Tax != tax || !proto.Equal(delivered, fact.Item.Proto()) {
		t.Fatal("真实物品和成交资金缺少唯一的永久证据", fact, delivered, count)
	}
	return marketDirectSaleProof{SourceItemID: original.ID, BuyOpID: domain.Hex(buy.Operation.OpId), PayoutOpID: domain.Hex(payout.Operation.OpId), Fact: fact}
}
