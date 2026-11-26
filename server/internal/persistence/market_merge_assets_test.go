// 本文件使用独立 MongoDB 副本集对账合服清退的原托管、已成交事实和事务重试。
package persistence

import (
	"bytes"
	"context"
	"errors"
	"strconv"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"minimir/internal/domain"
	p "minimir/internal/protocol"
)

// TestMarketMergeAssetsMongo 沿两个市场的真实持久结构清退，不重算成交、不重发资产。
func TestMarketMergeAssetsMongo(t *testing.T) {
	ctx, s, _ := goldMarketBackgroundSetup(t)
	require := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	run, origin, plan := s.Cfg.RunID, s.Cfg.OriginID, domain.ID()
	at := bossTime(time.Now())
	party := func(origin uint64) MarketParty {
		return MarketParty{RoleID: domain.ID(), AccountID: domain.ID(), Origin: origin}
	}
	seller, buyer, loser := party(origin), party(origin+1), party(origin)
	otherRole := MarketParty{RoleID: domain.ID(), AccountID: loser.AccountID, Origin: origin + 1}
	item := func(quantity int64) domain.Item {
		return domain.Item{ID: domain.ID(), ItemID: 35, Quantity: quantity, Version: 7, SourceKind: "monster_drop", SourceID: domain.ID()}
	}
	itemOrder := func(owner MarketParty, state uint32, quantity, round, bids int64) ItemMarketOrder {
		return ItemMarketOrder{ID: domain.ID(), RunID: run, Seller: owner, Item: item(quantity), UnitPrice: 91, Revision: 3,
			Round: round, ActiveBidCount: bids, Phase: state, ListedAt: at.Add(-time.Hour), AuctionEndsAt: at.Add(time.Hour),
			BuyoutDuration: 24 * time.Hour, TaxDivisor: 10, RuleVersion: s.Cfg.Fixture.RuleVersion, ConfigHash: s.Cfg.Fixture.Hash}
	}
	drawn := itemOrder(seller, ItemMarketAuction, 3, 2, 2)
	drawn.AuctionEndsAt = at.Add(-2 * time.Minute)
	direct := itemOrder(otherRole, ItemMarketDirect, 2, 1, 0)
	direct.AuctionEndsAt, direct.DirectEndsAt = at.Add(-time.Minute), at.Add(24*time.Hour-time.Minute)
	sold := itemOrder(seller, ItemMarketSold, 0, 1, 0)
	sold.AuctionEndsAt, sold.ClosedAt = at.Add(-2*time.Minute), at.Add(-time.Minute)
	foreign := itemOrder(seller, ItemMarketAuction, 1, 1, 0)
	foreign.RunID++
	newBid := func(order ItemMarketOrder, bidder MarketParty, round, ordinal, amount int64, state uint32) ItemMarketBid {
		return ItemMarketBid{ID: domain.ID(), OrderID: order.ID, RunID: order.RunID, Buyer: bidder,
			Round: round, Ordinal: ordinal, Amount: amount, State: state, CreatedAt: at.Add(-time.Hour)}
	}
	chosen := newBid(drawn, buyer, 2, 1, 273, itemBidFrozen)
	other := newBid(drawn, otherRole, 2, 2, 273, itemBidFrozen)
	carried := newBid(drawn, buyer, 1, 1, 246, itemBidCarried)
	oldLoser := newBid(drawn, loser, 1, 2, 246, itemBidFrozen)
	paid := newBid(sold, buyer, 1, 1, 91, itemBidPaid)
	soldLoser := newBid(sold, loser, 1, 2, 91, itemBidFrozen)
	drawn.WinnerBidID, sold.WinnerBidID = chosen.ID, paid.ID
	_, err := s.DB.Collection(itemMarketOrders).InsertMany(ctx, []ItemMarketOrder{drawn, direct, sold, foreign})
	require(err)
	_, err = s.DB.Collection(itemMarketBids).InsertMany(ctx, []ItemMarketBid{chosen, other, carried, oldLoser, paid, soldLoser})
	require(err)
	job := func(order ItemMarketOrder, round int64) ItemMarketRefundJob {
		return ItemMarketRefundJob{ID: domain.ID(), OrderID: order.ID, RunID: order.RunID, Round: round,
			CreatedAt: at.Add(-time.Minute), NextCheckAt: at.Add(-time.Minute)}
	}
	oldJob, soldJob, foreignJob := job(drawn, 1), job(sold, 1), job(foreign, 1)
	_, err = s.DB.Collection(itemMarketRefundJobs).InsertMany(ctx, []ItemMarketRefundJob{oldJob, soldJob, foreignJob})
	require(err)
	// 已成交拍卖的永久记录与已部分消费的旧直购所得保留独立证据。
	itemFact := ItemMarketSettlement{ID: domain.ID(), OrderID: sold.ID, RunID: run, Seller: seller, Buyer: buyer,
		WinnerBidID: paid.ID, Item: item(1), UnitPrice: 91, Gross: 91, Tax: 9,
		RuleVersion: sold.RuleVersion, ConfigHash: sold.ConfigHash, CreatedAt: sold.ClosedAt}
	require(s.insertItemMarketSettlement(ctx, itemFact))
	directFact := ItemMarketSettlement{ID: domain.ID(), OrderID: direct.ID, RunID: run, Seller: direct.Seller, Buyer: buyer,
		Item: item(1), UnitPrice: 91, Gross: 91, Tax: 9, RuleVersion: direct.RuleVersion, ConfigHash: direct.ConfigHash, CreatedAt: at.Add(-time.Second)}
	require(s.insertItemMarketSettlement(ctx, directFact))
	ownerClaim := ItemMarketClaim{ID: domain.ID(), OrderID: direct.ID, Owner: direct.Seller, Kind: ItemMarketClaimProceeds, Revision: 2,
		Yuanbao: 82, CreatedAt: directFact.CreatedAt, ClaimedAt: at, ClaimOp: domain.ID()}
	delivery := ItemMarketClaim{ID: domain.ID(), OrderID: sold.ID, Owner: buyer, Kind: ItemMarketClaimItem,
		Revision: 1, Item: &itemFact.Item, CreatedAt: sold.ClosedAt}
	require(s.insertItemMarketClaim(ctx, ownerClaim))
	require(s.insertItemMarketClaim(ctx, delivery))
	drawAudit := "item_market_auction_drawn:" + drawn.ID
	_, err = s.DB.Collection("audit_outbox").InsertOne(ctx, bson.M{"_id": drawAudit, "winner_bid_id": chosen.ID, "at": at.Add(-time.Minute)})
	require(err)

	goldOrder := func(owner MarketParty, side uint32, price, lots, remaining int64, pending bool) GoldMarketOrder {
		return seedGoldMarketOrder(t, ctx, s, owner, side, price, lots, remaining, at.Add(-time.Hour), at.Add(time.Hour), pending)
	}
	goldSell := goldOrder(seller, GoldMarketSell, 110, 4, 2, false)
	filledBuyer := goldOrder(buyer, GoldMarketBuy, 115, 2, 2, false)
	filledSeller := goldOrder(seller, GoldMarketSell, 91, 2, 2, false)
	goldBuy := goldOrder(buyer, GoldMarketBuy, 100, 5, 3, true)
	for _, order := range []GoldMarketOrder{filledBuyer, filledSeller} {
		order.State, order.ClosedAt, order.RemainingLots = GoldMarketFilled, at.Add(-time.Minute), 0
		order.FrozenGold, order.FrozenYuanbao = 0, 0
		require(s.updateGoldMarketOrder(ctx, order))
	}
	// 每次既有成交都耗尽对手单，留下两份不同委托上的真实历史余量。
	fill := func(buy, sell, maker, taker GoldMarketOrder) GoldMarketFill {
		fact := GoldMarketFill{ID: marketFactID("gold-market-fill", buy.ID, sell.ID), BuyOrderID: buy.ID, SellOrderID: sell.ID, RunID: run,
			MakerOrderID: maker.ID, TakerOrderID: taker.ID, Buyer: buy.Owner, Seller: sell.Owner,
			Lots: 2, UnitPrice: maker.LimitPrice, Gross: maker.LimitPrice * 2, Tax: maker.LimitPrice * 2 / 10,
			BuyerRefund: (buy.LimitPrice - maker.LimitPrice) * 2, Gold: 20_000, CreatedAt: at.Add(-time.Minute),
			MakerRuleVersion: maker.RuleVersion, MakerConfigHash: maker.ConfigHash, TakerRuleVersion: taker.RuleVersion, TakerConfigHash: taker.ConfigHash}
		require(s.insertGoldMarketFill(ctx, &fact))
		return fact
	}
	secondGoldFill := fill(filledBuyer, goldSell, goldSell, filledBuyer)
	goldFill := fill(goldBuy, filledSeller, filledSeller, goldBuy)
	goldClaim := GoldMarketClaim{ID: domain.ID(), OrderID: goldBuy.ID, FillID: goldFill.ID, Owner: buyer, Kind: GoldMarketClaimGold,
		Reason: GoldMarketReasonFill, Revision: 1, Gold: goldFill.Gold, CreatedAt: goldFill.CreatedAt}
	require(s.insertGoldMarketClaim(ctx, goldClaim))
	taxID := seller.AccountID + ":" + strconv.FormatUint(seller.Origin, 10)
	_, err = s.DB.Collection(marketSellerTaxRemainders).InsertOne(ctx, marketSellerTax{
		ID: taxID, AccountID: seller.AccountID, Origin: seller.Origin, Remainder: 3})
	require(err)

	// 保存逐字节的已产生凭据和相邻服资料，清退结果不能覆盖其中任一字段。
	type identity struct{ collection, id string }
	unchanged := []identity{{itemMarketOrders, sold.ID}, {itemMarketOrders, foreign.ID}, {itemMarketBids, carried.ID}, {itemMarketBids, paid.ID},
		{itemMarketRefundJobs, foreignJob.ID}, {itemMarketSettlements, itemFact.ID}, {itemMarketSettlements, directFact.ID}, {itemMarketClaims, ownerClaim.ID},
		{itemMarketClaims, delivery.ID}, {goldMarketOrders, filledBuyer.ID}, {goldMarketOrders, filledSeller.ID},
		{goldMarketFills, goldFill.ID}, {goldMarketFills, secondGoldFill.ID}, {goldMarketClaims, goldClaim.ID},
		{marketSellerTaxRemainders, taxID}, {"audit_outbox", drawAudit}}
	read := func(key identity) bson.Raw {
		t.Helper()
		value, err := s.DB.Collection(key.collection).FindOne(ctx, bson.M{"_id": key.id}).Raw()
		require(err)
		return value
	}
	saved := make(map[identity]bson.Raw, len(unchanged))
	for _, key := range unchanged {
		saved[key] = read(key)
	}
	if _, err := s.drainMarketMergeRun(ctx, plan, run, at); Code(err) != p.CodeOperationConflict {
		t.Fatal("没有 Mongo 会话的清退意外获得写权限", err)
	}
	rollback := errors.New("主动回滚本次隔离清退事务")
	if err := s.tx(ctx, func(tx context.Context) error {
		step, err := s.drainMarketMergeRun(tx, plan, run, at)
		if err != nil {
			return err
		}
		if step.SourceID != drawn.ID {
			t.Fatal("第一笔未选到仍持有拍品的订单", step)
		}
		return rollback
	}); !errors.Is(err, rollback) {
		t.Fatal("清退事务未进入原子回滚路径", err)
	}
	count := func(collection string, filter bson.M) int64 {
		t.Helper()
		n, err := s.DB.Collection(collection).CountDocuments(ctx, filter)
		require(err)
		return n
	}
	if count(itemMarketClaims, bson.M{"order_id": drawn.ID}) != 0 ||
		count(itemMarketOrders, bson.M{"_id": drawn.ID, "phase": ItemMarketAuction, "revision": drawn.Revision}) != 1 {
		t.Fatal("模拟未提交事务留下部分退款或修改原订单")
	}
	stepByID := map[string]MarketMergeAssetStep{}
	kinds := map[string]int{}
	for range 10 {
		var step MarketMergeAssetStep
		require(s.tx(ctx, func(tx context.Context) error {
			var err error
			step, err = s.drainMarketMergeRun(tx, plan, run, at)
			return err
		}))
		if step.RunID != run || step.Kind == "" || stepByID[step.SourceID].Kind != "" {
			t.Fatal("单笔清退越过本运行服、提前完成或重复处理", step)
		}
		stepByID[step.SourceID], kinds[step.Kind] = step, kinds[step.Kind]+1
	}
	if kinds[marketMergeItemOrder] != 2 || kinds[marketMergeItemBid] != 4 || kinds[marketMergeItemJob] != 2 || kinds[marketMergeGoldOrder] != 2 {
		t.Fatal("四组原义务处理次数不符", kinds)
	}
	for range 2 {
		require(s.tx(ctx, func(tx context.Context) error {
			step, err := s.drainMarketMergeRun(tx, plan, run, at)
			if step.Kind != "" {
				t.Fatal("重试又选中已处理原托管", step)
			}
			return err
		}))
	}
	for _, current := range []uint64{run, foreign.RunID} {
		debt, err := s.marketMergeRunHasDebt(ctx, current)
		require(err)
		if debt != (current == foreign.RunID) {
			t.Fatal("原运行服封账证明漏检或混入相邻服", current, debt)
		}
	}
	for _, order := range []ItemMarketOrder{drawn, direct} {
		claimID := stepByID[order.ID].ClaimID
		var claim ItemMarketClaim
		require(s.DB.Collection(itemMarketClaims).FindOne(ctx, bson.M{"_id": claimID}).Decode(&claim))
		if claim.Owner != order.Seller || claim.Item == nil || *claim.Item != order.Item || claim.OrderID != order.ID {
			t.Fatal("卖方没有收到原实例、原版本、原归属和未成交余量", claim)
		}
		if count(itemMarketOrders, bson.M{"_id": order.ID, "phase": ItemMarketCancelled, "revision": order.Revision + 1,
			"item.quantity": 0, "active_bid_count": 0}) != 1 {
			t.Fatal("订单原托管未清空", order.ID)
		}
	}
	for _, bid := range []ItemMarketBid{chosen, other, oldLoser, soldLoser} {
		claimID := marketFactID("market-claim", bid.OrderID, "refund", bid.ID)
		var claim ItemMarketClaim
		require(s.DB.Collection(itemMarketClaims).FindOne(ctx, bson.M{"_id": claimID}).Decode(&claim))
		if stepByID[bid.ID].ClaimID != claimID || claim.Kind != ItemMarketClaimRefund || claim.Yuanbao != bid.Amount || claim.Owner != bid.Buyer ||
			count(itemMarketBids, bson.M{"_id": bid.ID, "state": itemBidRefunded}) != 1 {
			t.Fatal("冻结款返还错误或未标记为已退款", bid.ID, claim)
		}
	}
	for _, order := range []GoldMarketOrder{goldBuy, goldSell} {
		var claim GoldMarketClaim
		require(s.DB.Collection(goldMarketClaims).FindOne(ctx, bson.M{"_id": stepByID[order.ID].ClaimID}).Decode(&claim))
		closed := readGoldMarketTestOrder(t, ctx, s, order.ID)
		if claim.Owner != order.Owner || claim.Gold != order.FrozenGold || claim.Yuanbao != order.FrozenYuanbao || claim.Reason != GoldMarketReasonCancel ||
			closed.State != GoldMarketCancelled || closed.PendingMatch || closed.Revision != order.Revision+1 ||
			closed.InitialLots != order.InitialLots || closed.RemainingLots != order.RemainingLots || !closed.ExpiresAt.Equal(order.ExpiresAt) ||
			closed.FrozenGold != 0 || closed.FrozenYuanbao != 0 {
			t.Fatal("金币买卖余单返款改了原币、原归属、部分成交手数或期限", claim, closed)
		}
	}
	if count("audit_outbox", bson.M{"kind": "market_merge_asset", "plan_id": plan, "run_server_id": run}) != int64(len(stepByID)) ||
		count("audit_outbox", bson.M{"kind": "market_merge_asset", "source_id": drawn.ID, "details.unsettled_drawn_bid_id": chosen.ID}) != 1 ||
		count(itemMarketRefundJobs, bson.M{"run_server_id": run}) != 0 ||
		count(itemMarketSettlements, bson.M{}) != 2 || count(goldMarketFills, bson.M{}) != 2 {
		t.Fatal("审计、退款任务或原成交永久事实有遗漏或重复")
	}
	for key, original := range saved {
		if current := read(key); !bytes.Equal(current, original) {
			t.Fatal("不属于未成交原托管的永久数据被修改", key)
		}
	}
	// 故意损坏一行标记时，已成交的原赢家仍不能再得到退款。
	_, err = s.DB.Collection(itemMarketBids).UpdateByID(ctx, paid.ID, bson.M{"$set": bson.M{"state": itemBidFrozen}})
	require(err)
	err = s.tx(ctx, func(tx context.Context) error {
		_, err := s.drainMarketMergeRun(tx, plan, run, at)
		return err
	})
	if Code(err) != p.CodeOperationConflict || count(itemMarketClaims, bson.M{
		"_id": marketFactID("market-claim", paid.OrderID, "refund", paid.ID)}) != 0 {
		t.Fatal("不一致的旧状态让已成交赢家又得到原扣款", err)
	}
}
