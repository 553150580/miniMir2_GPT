// 在真实 MongoDB 中验证整组授权、同节点不同运行服围栏和节点续约回滚。

package persistence

import (
	"context"
	"os"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	p "minimir/internal/protocol"
)

func TestGameRunLeaseSetAndRoleFenceMongo(t *testing.T) {
	if os.Getenv("MINIMIR_MONGO_E2E") != "1" {
		t.Skip("需要专属开发 MongoDB 集成测试")
	}
	cfg, err := devconfig.Load("../../../conf/dev/server.json")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 25*time.Second)
	defer cancel()
	// require 终止测试并保留实际数据库错误。
	require := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	requireCode := func(err error, expected uint32) {
		t.Helper()
		if Code(err) != expected {
			t.Fatalf("错误码为 %d，期望 %d：%v", Code(err), expected, err)
		}
	}
	cfg.Database = "minimir_dev_game_test_game_runs_" + domain.ID()[:16]
	cfg.OriginRoutes = []devconfig.OriginRouteSeed{{OriginID: 11, RunID: 101, Name: "一服", RegionID: "runs", RegionName: "运行服测试区"}}
	cfg.RunServers = []devconfig.RunServerSeed{
		{RunID: 101, GameNodeID: "lease-multi", GameWSURL: "/game/ws"},
		{RunID: 102, GameNodeID: "lease-previous", GameWSURL: "ws://127.0.0.1:18423/ws"},
		{RunID: 103, GameNodeID: "lease-multi", GameWSURL: "/game/ws"},
	}
	account, err := Open(ctx, cfg)
	require(err)
	t.Cleanup(func() {
		cleanup, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		defer account.Close(cleanup)
		if err := account.DB.Drop(cleanup); err != nil {
			t.Error(err)
		}
	})
	require(account.Singleton(ctx, "account"))
	previousCfg := cfg
	previousCfg.NodeID, previousCfg.RunID = "lease-previous", 102
	old, err := Open(ctx, previousCfg)
	require(err)
	defer old.Close(context.Background())
	grant, err := account.ApproveGame(ctx, previousCfg.NodeID, old.Instance, previousCfg.RunID)
	require(err)
	old.Epoch = grant.Epoch
	// 以旧版单服字段重现升级前授权，续约和重放都应继续有效。
	_, err = account.DB.Collection("node_leases").UpdateOne(ctx, bson.M{"_id": old.Cfg.NodeID},
		bson.M{"$unset": bson.M{"run_memberships": ""}, "$set": bson.M{"run_server_id": grant.ID, "run_epoch": grant.Epoch}})
	require(err)
	require(old.Renew(ctx, "game"))
	_, err = account.ApproveGame(ctx, previousCfg.NodeID, old.Instance, previousCfg.RunID)
	require(err)
	// 改派第二个运行服的部署。原有效实例暂未释放，新的整组授权必须回滚。
	_, err = account.DB.Collection("run_servers").UpdateOne(ctx, bson.M{"_id": 102},
		bson.M{"$set": bson.M{"game_node_id": "lease-multi", "game_ws_url": "/game/ws"}, "$inc": bson.M{"assignment_version": 1}})
	require(err)
	gameCfg := cfg
	gameCfg.NodeID, gameCfg.RunID, gameCfg.RunServerIDs = "lease-multi", 101, []uint64{101, 102, 103}
	game, err := Open(ctx, gameCfg)
	require(err)
	defer game.Close(context.Background())
	readRun := func(id uint64) Run {
		t.Helper()
		var run Run
		require(account.DB.Collection("run_control").FindOne(ctx, bson.M{"_id": id}).Decode(&run))
		return run
	}
	if rejected, err := account.ApproveGameRuns(ctx, game.Cfg.NodeID, game.Instance, game.Cfg.RunServerIDs); rejected != nil || Code(err) != p.CodeServerBusy {
		t.Fatalf("争抢已有效占用的运行服没有整体拒绝：%v %v", rejected, err)
	}
	if first := readRun(101); first.Epoch != 0 || first.State != "stopped" || first.Owner != "" {
		t.Fatalf("第二服争抢失败后第一服残留授权：%+v", first)
	}
	if err = account.DB.Collection("node_leases").FindOne(ctx, bson.M{"_id": game.Cfg.NodeID}).Err(); err != mongo.ErrNoDocuments {
		t.Fatalf("冲突申请留下节点租约：%v", err)
	}
	if err = account.DB.Collection("game_boots").FindOne(ctx, bson.M{"_id": game.Instance}).Err(); err != mongo.ErrNoDocuments {
		t.Fatalf("冲突申请留下启动记录：%v", err)
	}
	require(old.Release(ctx, "game"))
	grants, err := account.ApproveGameRuns(ctx, game.Cfg.NodeID, game.Instance, game.Cfg.RunServerIDs)
	require(err)
	if len(grants) != 3 || grants[0].Epoch != grants[2].Epoch || grants[1].Epoch <= grants[0].Epoch {
		t.Fatalf("运行服独立代次错误：%v", grants)
	}
	replayed, err := account.ApproveGameRuns(ctx, game.Cfg.NodeID, game.Instance, []uint64{103, 101, 102})
	require(err)
	for i := range grants {
		if replayed[i] != grants[i] {
			t.Fatal("同一启动实例完整重放改变了授权")
		}
	}
	_, err = account.ApproveGameRuns(ctx, game.Cfg.NodeID, game.Instance, []uint64{101, 102})
	requireCode(err, p.CodeRouteChanged)
	views, err := game.GameRunStores(grants)
	require(err)
	require(game.Renew(ctx, "game"))
	login, err := account.Login(ctx, "runs_"+domain.ID()[:8], "test_123456", true)
	require(err)
	accountAuth, err := account.Authenticate(ctx, login.AccountToken)
	require(err)
	role, err := account.CreateRole(ctx, accountAuth, &pb.CreateRoleRequest{
		OriginServerId: 11, Slot: 1, Name: "运行服写围栏", OpId: domain.Bytes(domain.ID()),
	})
	require(err)
	ticket, err := account.IssueTicket(ctx, accountAuth, role.Key)
	require(err)
	admitted, binding, err := views[0].Enter(ctx, ticket.Ticket)
	require(err)
	deathID := domain.ID()
	requireCode(views[2].PreserveRetiredDeath(ctx, binding, deathID), p.CodeOperationConflict)
	beforeDeathMarker, err := views[0].Role(ctx, admitted.ID)
	require(err)
	if beforeDeathMarker.RetiredDeathID != "" {
		t.Fatal("等代次跨运行服写入了退役死亡标记")
	}
	require(views[0].PreserveRetiredDeath(ctx, binding, deathID))
	marked, err := views[0].Role(ctx, admitted.ID)
	require(err)
	if marked.RetiredDeathID != deathID {
		t.Fatal("角色所在运行服未能写入退役死亡标记")
	}
	settlement := Settlement{SourceSeq: admitted.SourceSeq + 1, Gold: 5, Bound: true}
	_, err = views[2].Settle(ctx, binding, settlement)
	requireCode(err, p.CodeStaleSession)
	if _, err = views[0].Settle(ctx, binding, settlement); err != nil {
		t.Fatalf("原运行服的业务提交失败：%v", err)
	}
	// 单独过期一个成员，确保同批续约前面已经更新的成员也回滚。
	_, err = account.DB.Collection("run_control").UpdateOne(ctx, bson.M{"_id": 102},
		bson.M{"$set": bson.M{"lease_until": time.Unix(0, 0)}})
	require(err)
	beforeRenew := readRun(101)
	requireCode(game.Renew(ctx, "game"), p.CodeRouteChanged)
	if readRun(101).LeaseUntil != beforeRenew.LeaseUntil {
		t.Fatal("运行服成员续约失权时留下部分延期")
	}
	require(game.Release(ctx, "game"))
	for _, runID := range game.Cfg.RunServerIDs {
		if run := readRun(runID); run.State != "stopped" || run.Owner != game.Instance {
			t.Fatalf("节点释放没有停止仍归原实例的运行服：%+v", run)
		}
	}
	freshBoot := domain.ID()
	_, err = account.ApproveGameRuns(ctx, game.Cfg.NodeID, freshBoot, game.Cfg.RunServerIDs)
	require(err)
	_, err = account.ApproveGameRuns(ctx, game.Cfg.NodeID, game.Instance, game.Cfg.RunServerIDs)
	requireCode(err, p.CodeRouteChanged)
}
