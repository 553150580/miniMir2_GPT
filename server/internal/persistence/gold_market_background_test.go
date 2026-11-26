// 本文件在独立 MongoDB 副本集验收金币撮合顺序、永久资产与换代清偿。
package persistence

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	p "minimir/internal/protocol"
)

// goldMarketBackgroundSetup 为每个用例申请独立数据库和真实运行租约。
func goldMarketBackgroundSetup(t *testing.T) (context.Context, *Store, func() *Store) {
	t.Helper()
	uri := os.Getenv("MINIMIR_INTEGRATION_TEST_MONGO_URI")
	if os.Getenv("MINIMIR_MONGO_E2E") != "1" || uri == "" {
		t.Skip("需要独立 MongoDB 副本集")
	}
	cfg, err := devconfig.Load("../../../conf/dev/server.json")
	goldMarketRequire(t, err)
	cfg.MongoURI, cfg.Database = uri, "minimir_dev_game_test_gold_market_bg_"+domain.ID()[:16]
	// 开发入口关闭，证明后台只读取已确认委托的原规则。
	cfg.Fixture.GoldMarket = nil
	ctx, cancel := context.WithTimeout(t.Context(), 45*time.Second)
	t.Cleanup(cancel)
	account, err := Open(ctx, cfg)
	goldMarketRequire(t, err)
	t.Cleanup(func() {
		cleanup, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		defer account.Close(cleanup)
		goldMarketRequire(t, account.DB.Drop(cleanup))
	})
	goldMarketRequire(t, account.Singleton(ctx, "account"))
	openGame := func() *Store {
		t.Helper()
		game, err := Open(ctx, cfg)
		goldMarketRequire(t, err)
		t.Cleanup(func() { game.Close(context.Background()) })
		lease, err := account.ApproveGame(ctx, cfg.NodeID, game.Instance, cfg.RunID)
		goldMarketRequire(t, err)
		game.Epoch = lease.Epoch
		return game
	}
	return ctx, account, openGame
}

// goldMarketRequire 保留发生 MongoDB 错误的原测试行。
func goldMarketRequire(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

// seedGoldMarketOrder 写入测试的已托管委托，受理次序使用真实运行服计数器。
func seedGoldMarketOrder(t *testing.T, ctx context.Context, s *Store, owner MarketParty, side uint32,
	price, lots, remaining int64, accepted, expires time.Time, pending bool) GoldMarketOrder {
	t.Helper()
	next, err := s.nextGoldMarketSequence(ctx, "order_sequence")
	goldMarketRequire(t, err)
	order := GoldMarketOrder{ID: domain.ID(), RunID: s.Cfg.RunID, Owner: owner, Side: side, State: GoldMarketActive,
		Sequence: next, LimitPrice: price, InitialLots: lots, RemainingLots: remaining, Revision: 1,
		TaxDivisor: 10, GoldPerLot: 10_000, AcceptedAt: accepted, ExpiresAt: expires, PendingMatch: pending,
		RuleVersion: s.Cfg.Fixture.RuleVersion, ConfigHash: s.Cfg.Fixture.Hash}
	if side == GoldMarketBuy {
		order.FrozenYuanbao = price * remaining
	} else {
		order.FrozenGold = order.GoldPerLot * remaining
	}
	_, err = s.DB.Collection(goldMarketOrders).InsertOne(ctx, order)
	goldMarketRequire(t, err)
	return order
}

// readGoldMarketTestOrder 读取订单实际提交的修订和资产余量。
func readGoldMarketTestOrder(t *testing.T, ctx context.Context, s *Store, id string) GoldMarketOrder {
	t.Helper()
	var order GoldMarketOrder
	goldMarketRequire(t, s.DB.Collection(goldMarketOrders).FindOne(ctx, bson.M{"_id": id}).Decode(&order))
	return order
}

// advanceGoldMarketTest 验证存在可推进工作时后台实际选择了一个步骤。
func advanceGoldMarketTest(t *testing.T, ctx context.Context, s *Store) {
	t.Helper()
	more, err := s.AdvanceGoldMarket(ctx, time.Now())
	goldMarketRequire(t, err)
	if !more {
		t.Fatal("原委托未取得后台处理步骤")
	}
}

// TestGoldMarketBackgroundPriorityMongo 双向验证更优价格、同价时间、原价与原角色资产。
func TestGoldMarketBackgroundPriorityMongo(t *testing.T) {
	for _, tc := range []struct {
		name                     string
		makerSide, takerSide     uint32
		own, better, worse, take int64
		refunds, taxes           [3]int64
	}{
		{name: "新买单先吃低卖价", makerSide: GoldMarketSell, takerSide: GoldMarketBuy, own: 1, better: 7,
			worse: 9, take: 10, refunds: [3]int64{3, 6, 1}, taxes: [3]int64{1, 2, 0}},
		{name: "新卖单先吃高买价", makerSide: GoldMarketBuy, takerSide: GoldMarketSell, own: 12, better: 9,
			worse: 7, take: 6, taxes: [3]int64{1, 2, 1}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, account, open := goldMarketBackgroundSetup(t)
			game := open()
			party := func() MarketParty {
				return MarketParty{RoleID: domain.ID(), AccountID: domain.ID(), Origin: game.Cfg.OriginID}
			}
			owner, bestA, worseOwner := party(), party(), party()
			bestB, ownOtherRole := bestA, owner
			bestB.RoleID, ownOtherRole.RoleID = domain.ID(), domain.ID()
			accepted := bossTime(time.Now())
			expires := accepted.Add(time.Hour)
			seed := func(party MarketParty, side uint32, price, lots int64) GoldMarketOrder {
				return seedGoldMarketOrder(t, ctx, game, party, side, price, lots, lots, accepted, expires, true)
			}
			own := seed(ownOtherRole, tc.makerSide, tc.own, 1)
			worse := seed(worseOwner, tc.makerSide, tc.worse, 2)
			betterA := seed(bestA, tc.makerSide, tc.better, 1)
			betterB := seed(bestB, tc.makerSide, tc.better, 2)
			taker := seed(owner, tc.takerSide, tc.take, 4)
			foreign := own
			foreign.ID, foreign.RunID, foreign.Owner = domain.ID(), own.RunID+1, party()
			_, err := account.DB.Collection(goldMarketOrders).InsertOne(ctx, foreign)
			goldMarketRequire(t, err)

			sharedSeller := bestA
			if tc.takerSide == GoldMarketSell {
				sharedSeller = owner
			}
			sharedSeller.RoleID = domain.ID()
			goldMarketRequire(t, game.itemMarketSystemTx(ctx, []string{sharedSeller.AccountID}, func(tx context.Context) error {
				_, err := game.chargeMarketSellerTax(tx, sharedSeller, 9, 10)
				return err
			}))
			id, first, second := goldMarketReferencePair(worseOwner.AccountID, owner.AccountID)
			_, err = account.DB.Collection(goldMarketReferenceExclusions).InsertOne(ctx, bson.M{
				"_id": id, "account_a": first, "account_b": second, "reason": "隔离验收关联账号"})
			goldMarketRequire(t, err)

			// 同一毫秒收到四笔簿内单，按持久序号而非随机订单号决出前后。
			for range 4 {
				advanceGoldMarketTest(t, ctx, game)
			}
			read := func(id string) GoldMarketOrder { return readGoldMarketTestOrder(t, ctx, game, id) }
			maker := read(betterA.ID)
			if maker.PendingMatch || maker.Revision != betterA.Revision {
				t.Fatal("入簿改变原角色用于撤单的订单修订", maker)
			}
			done := make(chan error, 2)
			for range 2 {
				go func() { done <- game.matchGoldMarketIncoming(ctx, taker, maker, accepted) }()
			}
			var committed, stale int
			for range 2 {
				err := <-done
				switch {
				case err == nil:
					committed++
				case errors.Is(err, errGoldMarketWorkChanged):
					stale++
				default:
					t.Fatal(err)
				}
			}
			partial := read(taker.ID)
			if committed != 1 || stale != 1 || partial.RemainingLots != 3 || !partial.PendingMatch || !partial.ExpiresAt.Equal(expires) ||
				read(betterA.ID).State != GoldMarketFilled {
				t.Fatal("原成交在并发重放出现重领，或部分委托失去期限和原优先级", committed, stale, partial)
			}
			advanceGoldMarketTest(t, ctx, game)
			advanceGoldMarketTest(t, ctx, game)
			if finished := read(taker.ID); finished.State != GoldMarketFilled || finished.RemainingLots != 0 ||
				finished.PendingMatch || finished.FrozenGold != 0 || finished.FrozenYuanbao != 0 {
				t.Fatal("完整成交留下未清托管", finished)
			}
			if tail := read(worse.ID); tail.RemainingLots != 1 || tail.PendingMatch || !tail.ExpiresAt.Equal(expires) ||
				read(own.ID).RemainingLots != 1 || read(foreign.ID).RemainingLots != 1 {
				t.Fatal("部分成交、同账不同角色或外服过滤出现异动", tail)
			}

			cursor, err := account.DB.Collection(goldMarketFills).Find(ctx, bson.M{"run_server_id": game.Cfg.RunID},
				options.Find().SetSort(bson.D{{Key: "sequence", Value: 1}}))
			goldMarketRequire(t, err)
			var fills []GoldMarketFill
			goldMarketRequire(t, cursor.All(ctx, &fills))
			if len(fills) != 3 {
				t.Fatalf("逐对永久成交记录有重复或遗漏：%+v", fills)
			}
			for i, direction := range []GoldMarketOrder{betterA, betterB, worse} {
				fill, lots := fills[i], [3]int64{1, 2, 1}[i]
				if fill.Sequence != int64(i+1) || fill.MakerOrderID != direction.ID || fill.TakerOrderID != taker.ID ||
					fill.UnitPrice != direction.LimitPrice || fill.Lots != lots || fill.Gross != direction.LimitPrice*lots ||
					fill.Tax != tc.taxes[i] || fill.BuyerRefund != tc.refunds[i] || fill.Gold != 10_000*lots || fill.ReferenceExcluded != (i == 2) {
					t.Fatalf("价格、同价次序、关联标记或共享税账不符：%+v", fill)
				}
				wantBuyer, wantSeller := taker, direction
				if tc.takerSide == GoldMarketSell {
					wantBuyer, wantSeller = direction, taker
				}
				want := []GoldMarketClaim{
					{OrderID: wantBuyer.ID, Owner: wantBuyer.Owner, Kind: GoldMarketClaimGold, Reason: GoldMarketReasonFill, Gold: fill.Gold},
					{OrderID: wantSeller.ID, Owner: wantSeller.Owner, Kind: GoldMarketClaimProceeds, Reason: GoldMarketReasonFill, Yuanbao: fill.Gross - fill.Tax},
				}
				if fill.BuyerRefund > 0 {
					want = append(want, GoldMarketClaim{OrderID: wantBuyer.ID, Owner: wantBuyer.Owner,
						Kind: GoldMarketClaimYuanbaoRefund, Reason: GoldMarketReasonPriceDifference, Yuanbao: fill.BuyerRefund})
				}
				for _, claim := range want {
					var observed GoldMarketClaim
					goldMarketRequire(t, account.DB.Collection(goldMarketClaims).FindOne(ctx, bson.M{"fill_id": fill.ID, "kind": claim.Kind}).Decode(&observed))
					if observed.Owner != claim.Owner || observed.OrderID != claim.OrderID || observed.Revision != 1 ||
						observed.Reason != claim.Reason || observed.Yuanbao != claim.Yuanbao || observed.Gold != claim.Gold {
						t.Fatalf("资金或金币未原路记入归属角色：%+v", observed)
					}
				}
				var audit struct {
					Sequence int64  `bson:"fill_sequence"`              // 成交的真实运行服序号。
					Reason   string `bson:"reference_exclusion_reason"` // 已审核的行情排除原因。
				}
				goldMarketRequire(t, account.DB.Collection("audit_outbox").FindOne(ctx, bson.M{"_id": "gold_market_fill:" + fill.ID}).Decode(&audit))
				if audit.Sequence != fill.Sequence || (audit.Reason != "") != fill.ReferenceExcluded {
					t.Fatal("原成交事实未进入审计或未记录排除原因", audit)
				}
			}
			if more, err := game.AdvanceGoldMarket(ctx, time.Now()); err != nil || more {
				t.Fatal("等待中原订单重复入队", more, err)
			}
		})
	}
}

// TestGoldMarketBackgroundExpiryMongo 验证跨真实截止的撮合被拒，原配置关闭和节点换代仍原路退余量。
func TestGoldMarketBackgroundExpiryMongo(t *testing.T) {
	ctx, account, open := goldMarketBackgroundSetup(t)
	game := open()
	party := func() MarketParty {
		return MarketParty{RoleID: domain.ID(), AccountID: domain.ID(), Origin: game.Cfg.OriginID}
	}
	cutoff := bossTime(time.Now())
	accepted := cutoff.Add(-time.Minute)
	maker := seedGoldMarketOrder(t, ctx, game, party(), GoldMarketSell, 5, 2, 2, accepted, cutoff.Add(time.Hour), false)
	buy := seedGoldMarketOrder(t, ctx, game, party(), GoldMarketBuy, 8, 3, 2, accepted, cutoff, true)
	sell := seedGoldMarketOrder(t, ctx, game, party(), GoldMarketSell, 6, 5, 3, accepted, cutoff, false)
	if err := game.matchGoldMarketIncoming(ctx, buy, maker, cutoff.Add(-time.Millisecond)); !errors.Is(err, errGoldMarketWorkChanged) {
		t.Fatalf("过期前入队的候选在围栏事务里越过真实截止：%v", err)
	}
	goldMarketRequire(t, game.Release(ctx, "game"))
	replacement := open()
	if replacement.Epoch <= game.Epoch {
		t.Fatal("新节点缺少更高的运行代次")
	}
	if _, err := game.AdvanceGoldMarket(ctx, time.Now()); Code(err) != p.CodeRouteChanged {
		t.Fatalf("旧运行代次还能确认到期退款：%v", err)
	}
	if readGoldMarketTestOrder(t, ctx, account, buy.ID).State != GoldMarketActive {
		t.Fatal("被拒绝的旧节点修改了原托管委托")
	}
	for _, original := range []GoldMarketOrder{buy, sell} {
		advanceGoldMarketTest(t, ctx, replacement)
		state := readGoldMarketTestOrder(t, ctx, account, original.ID)
		if state.State != GoldMarketExpired || state.FrozenYuanbao != 0 || state.FrozenGold != 0 || state.PendingMatch ||
			state.RemainingLots != original.RemainingLots || !state.ExpiresAt.Equal(cutoff) || state.ClosedAt.Before(cutoff) {
			t.Fatal("到期未保留原截止和未成交手数，或托管资产未清空", state)
		}
		kind := GoldMarketClaimGoldReturn
		if original.Side == GoldMarketBuy {
			kind = GoldMarketClaimYuanbaoRefund
		}
		var claim GoldMarketClaim
		goldMarketRequire(t, account.DB.Collection(goldMarketClaims).FindOne(ctx, bson.M{"order_id": original.ID}).Decode(&claim))
		if claim.Owner != original.Owner || claim.Kind != kind || claim.Reason != GoldMarketReasonExpiry || claim.FillID != "" ||
			claim.Yuanbao != original.FrozenYuanbao || claim.Gold != original.FrozenGold {
			t.Fatal("过期部分余额不是原买卖角色原币种", claim)
		}
		goldMarketRequire(t, account.DB.Collection("audit_outbox").FindOne(ctx, bson.M{
			"_id": "gold_market_expired:" + original.ID, "claim_id": claim.ID, "expires_at": cutoff}).Err())
	}
	if more, err := replacement.AdvanceGoldMarket(ctx, time.Now()); err != nil || more {
		t.Fatal("换代恢复后重复退回已清托管", more, err)
	}
	claims, err := account.DB.Collection(goldMarketClaims).CountDocuments(ctx, bson.M{})
	goldMarketRequire(t, err)
	fills, err := account.DB.Collection(goldMarketFills).CountDocuments(ctx, bson.M{})
	goldMarketRequire(t, err)
	if claims != 2 || fills != 0 || readGoldMarketTestOrder(t, ctx, account, maker.ID).RemainingLots != 2 {
		t.Fatal("到期、换代或重试改变了退款数和未成交簿内余额", claims, fills)
	}
}
