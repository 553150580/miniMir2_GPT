// 本文件验证市场原时钟、持久赢家、换代续结、有界退款与资产守恒。
package persistence

import (
	"context"
	"math"
	"os"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	p "minimir/internal/protocol"
)

// TestItemMarketRandomOrdinalAndCommission 核对抽选择的整数范围和大额时的抗拆单佣金。
func TestItemMarketRandomOrdinalAndCommission(t *testing.T) {
	var first, second [32]byte
	first[31], second[31] = 1, 2
	for _, count := range []int64{1, 2, 3, 9, 32768, math.MaxInt64} {
		for _, seed := range [][32]byte{first, second} {
			ordinal := chooseItemMarketOrdinal(seed, count)
			if ordinal < 1 || ordinal > count || ordinal != chooseItemMarketOrdinal(seed, count) {
				t.Fatalf("原随机选择越界或不能重放：%d %d", ordinal, count)
			}
		}
	}
	if tax, residue := marketCommission(math.MaxInt64, 9); tax != 922337203685477581 || residue != 6 {
		t.Fatalf("累积税额在最大成交数溢出：%d %d", tax, residue)
	}
	var taxed, residual int64
	for _, addition := range []int64{1, 17, 301, 22, 503, 10, 11} {
		later, remainder := marketCommission(addition, residual)
		taxed, residual = taxed+later, remainder
	}
	all, leftover := marketCommission(865, 0)
	if taxed != all || residual != leftover {
		t.Fatal("拆分成交改变累计税")
	}
}

// TestItemMarketBackgroundMongo 用隔离副本集推进真实时钟上已到期的双阶段订单。
func TestItemMarketBackgroundMongo(t *testing.T) {
	uri := os.Getenv("MINIMIR_INTEGRATION_TEST_MONGO_URI")
	if os.Getenv("MINIMIR_MONGO_E2E") != "1" || uri == "" {
		t.Skip("需要独立 MongoDB 副本集")
	}
	require := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	cfg, err := devconfig.Load("../../../conf/dev/server.json")
	require(err)
	cfg.MongoURI, cfg.Database = uri, "minimir_dev_game_test_item_market_bg_"+domain.ID()[:16]
	ctx, cancel := context.WithTimeout(t.Context(), 45*time.Second)
	defer cancel()
	account, err := Open(ctx, cfg)
	require(err)
	t.Cleanup(func() {
		cleanup, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		defer account.Close(cleanup)
		require(account.DB.Drop(cleanup))
	})
	require(account.Singleton(ctx, "account"))
	openGame := func() *Store {
		t.Helper()
		game, err := Open(ctx, cfg)
		require(err)
		t.Cleanup(func() { game.Close(context.Background()) })
		lease, err := account.ApproveGame(ctx, cfg.NodeID, game.Instance, cfg.RunID)
		require(err)
		game.Epoch = lease.Epoch
		return game
	}
	game := openGame()
	now := bossTime(time.Now())
	newParty := func() MarketParty {
		return MarketParty{RoleID: domain.ID(), AccountID: domain.ID(), Origin: cfg.OriginID}
	}
	seller := newParty()
	newOrder := func(ends time.Time, count int64) ItemMarketOrder {
		return ItemMarketOrder{ID: domain.ID(), RunID: cfg.RunID, Seller: seller,
			Item: domain.Item{ID: domain.ID(), ItemID: cfg.Fixture.InitialInventory[0], Quantity: 3, Version: 2,
				SourceKind: "drop", SourceID: domain.ID()},
			UnitPrice: 167, Revision: 1, Round: 1, ActiveBidCount: count, Phase: ItemMarketAuction,
			ListedAt: ends.Add(-2 * time.Hour), AuctionEndsAt: ends, BuyoutDuration: 24 * time.Hour,
			RuleVersion: cfg.Fixture.RuleVersion, ConfigHash: cfg.Fixture.Hash, TaxDivisor: 10}
	}
	auction := newOrder(now.Add(-26*time.Hour), itemMarketRefundBatch+3)
	emptyAuction := newOrder(now.Add(-25*time.Hour), 0)
	_, err = game.DB.Collection(itemMarketOrders).InsertMany(ctx, []ItemMarketOrder{auction, emptyAuction})
	require(err)
	participants := make(map[string]ItemMarketBid)
	for ordinal := int64(1); ordinal <= auction.ActiveBidCount; ordinal++ {
		bid := ItemMarketBid{ID: domain.ID(), OrderID: auction.ID, RunID: auction.RunID, Buyer: newParty(),
			Round: auction.Round, Ordinal: ordinal, Amount: auction.UnitPrice * auction.Item.Quantity, State: itemBidFrozen,
			CreatedAt: auction.AuctionEndsAt.Add(-time.Hour)}
		participants[bid.ID] = bid
		_, err := game.DB.Collection(itemMarketBids).InsertOne(ctx, bid)
		require(err)
	}
	otherSellerRole := seller
	otherSellerRole.RoleID = domain.ID()
	require(game.itemMarketSystemTx(ctx, []string{seller.AccountID}, func(tc context.Context) error {
		tax, err := game.chargeMarketSellerTax(tc, otherSellerRole, 9, 10)
		if err == nil && tax != 0 {
			t.Fatalf("原卖方的第一笔小额成交税错：%d", tax)
		}
		return err
	}))
	advance := func(s *Store) {
		t.Helper()
		advanced, err := s.AdvanceItemMarket(ctx, time.Now())
		require(err)
		if !advanced {
			t.Fatal("已到期的原市场步骤遗漏")
		}
	}
	orderByID := func(id string) ItemMarketOrder {
		t.Helper()
		var value ItemMarketOrder
		require(account.DB.Collection(itemMarketOrders).FindOne(ctx, bson.M{"_id": id}).Decode(&value))
		return value
	}
	count := func(collection string, filter bson.M) int64 {
		t.Helper()
		value, err := account.DB.Collection(collection).CountDocuments(ctx, filter)
		require(err)
		return value
	}

	advance(game)
	drawn := orderByID(auction.ID)
	winner, ok := participants[drawn.WinnerBidID]
	if !ok || drawn.Phase != ItemMarketAuction || count(itemMarketClaims, bson.M{"order_id": auction.ID}) != 0 {
		t.Fatal("随机赢家未在发放前优先冻结", drawn)
	}
	var audit struct {
		Draw struct {
			Winner  string `bson:"winner_bid_id"`       // 已冻结的原预购。
			Count   int64  `bson:"participating_count"` // 原轮参与人数。
			Entropy []byte `bson:"draw_entropy"`        // 可重新核对原抽样的熵。
		} `bson:"details"` // 原发放前已提交的证据。
	}
	require(account.DB.Collection("audit_outbox").FindOne(ctx, bson.M{"_id": "item_market_auction_drawn:" + auction.ID}).Decode(&audit))
	if audit.Draw.Winner != winner.ID || audit.Draw.Count != auction.ActiveBidCount || len(audit.Draw.Entropy) != 32 ||
		chooseItemMarketOrdinal([32]byte(audit.Draw.Entropy), audit.Draw.Count) != winner.Ordinal {
		t.Fatal("持久抽选证据与确定赢家不符", audit)
	}

	require(game.Release(ctx, "game"))
	replacement := openGame()
	if replacement.Epoch <= game.Epoch {
		t.Fatal("未获得新的运行服代次")
	}
	if _, err := game.AdvanceItemMarket(ctx, time.Now()); Code(err) != p.CodeRouteChanged {
		t.Fatalf("已失权执行器仍能写原赢家结算：%v", err)
	}
	advance(replacement)
	sold := orderByID(auction.ID)
	if sold.WinnerBidID != winner.ID || sold.Phase != ItemMarketSold || sold.Item.Quantity != 0 ||
		count(itemMarketBids, bson.M{"order_id": auction.ID, "state": itemBidPaid}) != 1 {
		t.Fatal("新运行服代次重抽或重复消费冻结款", sold)
	}
	var settlement ItemMarketSettlement
	require(account.DB.Collection(itemMarketSettlements).FindOne(ctx, bson.M{"order_id": auction.ID}).Decode(&settlement))
	if settlement.WinnerBidID != winner.ID || settlement.Item != auction.Item || settlement.Gross != 501 || settlement.Tax != 51 {
		t.Fatal("金钱、物品或同账号同入口的持续税账异常", settlement)
	}
	var delivery, proceeds ItemMarketClaim
	require(account.DB.Collection(itemMarketClaims).FindOne(ctx, bson.M{"order_id": auction.ID, "kind": ItemMarketClaimItem}).Decode(&delivery))
	require(account.DB.Collection(itemMarketClaims).FindOne(ctx, bson.M{"order_id": auction.ID, "kind": ItemMarketClaimProceeds}).Decode(&proceeds))
	if delivery.Owner != winner.Buyer || delivery.Item == nil || *delivery.Item != auction.Item ||
		proceeds.Owner != seller || proceeds.Yuanbao+settlement.Tax != settlement.Gross {
		t.Fatal("整单交付和原卖家收入未守恒", delivery, proceeds)
	}
	filter := bson.M{"order_id": auction.ID, "kind": ItemMarketClaimRefund}
	if count(itemMarketClaims, filter) != 0 {
		t.Fatal("大名单退款没有切出独立有界步骤")
	}

	advance(replacement)
	direct := orderByID(emptyAuction.ID)
	if direct.Phase != ItemMarketDirect || !direct.DirectEndsAt.Equal(emptyAuction.AuctionEndsAt.Add(24*time.Hour)) {
		t.Fatal("重启后已过去的直购窗口重新起算", direct)
	}
	advance(replacement)
	if expired := orderByID(emptyAuction.ID); expired.Phase != ItemMarketExpired || expired.Item.Quantity != 0 {
		t.Fatal("到期直购未推进唯一退货阶段", expired)
	}
	var returned ItemMarketClaim
	require(account.DB.Collection(itemMarketClaims).FindOne(ctx, bson.M{"order_id": emptyAuction.ID}).Decode(&returned))
	if returned.Owner != seller || returned.Item == nil || *returned.Item != emptyAuction.Item || returned.Yuanbao != 0 {
		t.Fatal("零成交退货改变原物、归属或金额", returned)
	}
	advance(replacement)
	if count(itemMarketClaims, filter) != itemMarketRefundBatch {
		t.Fatal("首步退款超过原批次预算")
	}
	advance(replacement)
	if count(itemMarketClaims, filter) != auction.ActiveBidCount-1 ||
		count(itemMarketRefundJobs, bson.M{"order_id": auction.ID}) != 0 ||
		count(itemMarketSettlements, bson.M{"order_id": auction.ID}) != 1 {
		t.Fatal("多批退款或重放出现重复或漏单")
	}
	for _, bidder := range participants {
		want := int64(1)
		if bidder.ID == winner.ID {
			want = 0
		}
		if count(itemMarketClaims, bson.M{"_id": marketFactID("market-claim", auction.ID, "refund", bidder.ID),
			"owner.role_id": bidder.Buyer.RoleID, "owner.origin_server_id": bidder.Buyer.Origin, "yuanbao": bidder.Amount}) != want {
			t.Fatal("原被冻结的非绑定元宝未逐笔回原角色", bidder.ID)
		}
	}
	raised := newOrder(now.Add(time.Hour), 1)
	raised.Round, raised.UnitPrice = 2, 184
	_, err = account.DB.Collection(itemMarketOrders).InsertOne(ctx, raised)
	require(err)
	carried := ItemMarketBid{ID: domain.ID(), OrderID: raised.ID, RunID: raised.RunID, Buyer: newParty(),
		Round: 1, Ordinal: 1, Amount: 501, State: itemBidCarried, CreatedAt: raised.ListedAt}
	loser := ItemMarketBid{ID: domain.ID(), OrderID: raised.ID, RunID: raised.RunID, Buyer: newParty(),
		Round: 1, Ordinal: 2, Amount: 501, State: itemBidFrozen, CreatedAt: raised.ListedAt}
	rebid := ItemMarketBid{ID: domain.ID(), OrderID: raised.ID, RunID: raised.RunID, Buyer: carried.Buyer,
		Round: raised.Round, Ordinal: 1, Amount: 552, State: itemBidFrozen, CreatedAt: now}
	_, err = account.DB.Collection(itemMarketBids).InsertMany(ctx, []ItemMarketBid{carried, loser, rebid})
	require(err)
	archived := raised
	archived.Round = 1
	require(replacement.itemMarketSystemTx(ctx, []string{seller.AccountID}, func(tc context.Context) error {
		return replacement.queueItemMarketRefund(tc, archived, time.Now())
	}))
	advance(replacement)
	if count(itemMarketClaims, bson.M{"order_id": raised.ID, "kind": ItemMarketClaimRefund}) != 1 ||
		count(itemMarketClaims, bson.M{"order_id": raised.ID, "owner.role_id": loser.Buyer.RoleID, "yuanbao": loser.Amount}) != 1 ||
		count(itemMarketBids, bson.M{"_id": carried.ID, "state": itemBidCarried}) != 1 ||
		count(itemMarketBids, bson.M{"_id": rebid.ID, "state": itemBidFrozen, "amount": rebid.Amount}) != 1 {
		t.Fatal("加价旧轮退款错误处理了保留预购或新轮冻结款")
	}
	if active, err := replacement.AdvanceItemMarket(ctx, time.Now()); err != nil || active {
		t.Fatal("永久终态又被重复处理", active, err)
	}
}
