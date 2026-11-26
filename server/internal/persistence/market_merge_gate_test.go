// 本文件以真实 MongoDB 快照复现跨越冻结的签票和后台事务竞争。
package persistence

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.mongodb.org/mongo-driver/v2/mongo/readconcern"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
)

// TestMarketMergeWriteFenceMongo 先建立两个旧快照，再提交冻结，并检查迟到的两类写入均冲突。
func TestMarketMergeWriteFenceMongo(t *testing.T) {
	ctx, account, openGame := goldMarketBackgroundSetup(t)
	game := openGame()
	runID, originID := game.Cfg.RunID, game.Cfg.OriginID
	otherRunID := runID + 1
	account.Cfg.OriginRoutes = []devconfig.OriginRouteSeed{{OriginID: originID + 1, RunID: otherRunID}}
	account.Cfg.RunServers = []devconfig.RunServerSeed{{RunID: otherRunID, GameNodeID: game.Cfg.NodeID,
		GameWSURL: "ws://" + game.Cfg.GameAddress + "/ws"}}
	goldMarketRequire(t, account.seedRoutingCatalog(ctx))
	plan, err := account.PrepareMarketMerge(ctx, domain.ID(), runID, otherRunID)
	goldMarketRequire(t, err)

	accountID := domain.ID()
	now := bossTime(time.Now())
	owner := MarketParty{RoleID: domain.ID(), AccountID: accountID, Origin: originID}
	order := seedGoldMarketOrder(t, ctx, game, owner, GoldMarketBuy, 91, 1, 1,
		now.Add(-2*time.Minute), now.Add(-time.Minute), false)
	makeSnapshot := func(store *Store) context.Context {
		t.Helper()
		session, err := store.Client.StartSession()
		goldMarketRequire(t, err)
		t.Cleanup(func() { session.EndSession(context.Background()) })
		goldMarketRequire(t, session.StartTransaction(options.Transaction().SetReadConcern(readconcern.Snapshot())))
		return mongo.NewSessionContext(ctx, session)
	}
	ticketSnap, marketSnap := makeSnapshot(account), makeSnapshot(game)
	destination, err := account.liveRoute(ticketSnap, originID)
	goldMarketRequire(t, err)
	frozen, err := game.marketMergeFrozen(marketSnap)
	goldMarketRequire(t, err)
	if frozen {
		t.Fatal("冻结前市场快照错误")
	}
	marketWrite, err := game.runFenceWrite(marketSnap, runGuardGroup(accountID))
	goldMarketRequire(t, err)

	_, err = account.FreezeMarketMerge(ctx, plan.ID, plan.Version)
	goldMarketRequire(t, err)
	assertConflict := func(err error) {
		t.Helper()
		var databaseErr mongo.ServerError
		if !errors.As(err, &databaseErr) || !databaseErr.HasErrorCode(112) || !databaseErr.HasErrorLabel("TransientTransactionError") {
			t.Fatalf("跨越冻结的原快照未发生写冲突，错误=%v", err)
		}
	}
	assertConflict(account.reserveTicketRoute(ticketSnap, destination.run, accountID))
	_, err = game.Client.BulkWrite(marketSnap, []mongo.ClientBulkWrite{marketWrite})
	assertConflict(err)

	// 实际后台入口重新读到门禁，不提交到期退款，也不向调度器报故障。
	more, err := game.AdvanceGoldMarket(ctx, now)
	goldMarketRequire(t, err)
	if more {
		t.Fatal("冻结时金币后台仍推进了到期退款")
	}
	var stored GoldMarketOrder
	goldMarketRequire(t, game.DB.Collection(goldMarketOrders).FindOne(ctx, bson.M{"_id": order.ID}).Decode(&stored))
	if stored.State != GoldMarketActive || stored.FrozenYuanbao != order.FrozenYuanbao {
		t.Fatalf("冻结时金币后台改动了托管：%+v", stored)
	}
}
