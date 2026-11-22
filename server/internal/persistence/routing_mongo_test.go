// 验证多入口角色隔离、目录实际可用性、票据版本一致性与原入场尝试幂等。

package persistence

import (
	"context"
	"os"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	p "minimir/internal/protocol"
)

func TestDirectoryAndAdmissionUsePersistedRoutingMongo(t *testing.T) {
	if os.Getenv("MINIMIR_MONGO_E2E") != "1" {
		t.Skip("需要专属开发 MongoDB 集成测试")
	}
	cfg, err := devconfig.Load("../../../conf/dev/server.json")
	if err != nil {
		t.Fatal(err)
	}
	cfg.Database = "minimir_dev_game_test_routing_" + domain.ID()[:12]
	cfg.OriginRoutes = []devconfig.OriginRouteSeed{
		{OriginID: 11, RunID: 101, RegionID: "east", RegionName: "东区", Name: "东区一服", RegionOrder: 2},
		{OriginID: 12, RunID: 102, RegionID: "west", RegionName: "西区", Name: "西区一服", RegionOrder: 1},
	}
	cfg.RunServers = []devconfig.RunServerSeed{
		{RunID: 101, GameNodeID: "routing-east", GameWSURL: "ws://127.0.0.1:18401/ws"},
		{RunID: 102, GameNodeID: "routing-west", GameWSURL: "ws://127.0.0.1:18402/ws"},
	}
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	require := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	requireCode := func(err error, expect uint32) {
		t.Helper()
		if Code(err) != expect {
			t.Fatalf("错误码为 %d，期望 %d：%v", Code(err), expect, err)
		}
	}
	account, err := Open(ctx, cfg)
	require(err)
	t.Cleanup(func() {
		closeCtx, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		if err := account.DB.Drop(closeCtx); err != nil {
			t.Error(err)
		}
		account.Close(closeCtx)
	})
	require(account.Singleton(ctx, "account"))
	login, err := account.Login(ctx, "catalog_"+domain.ID()[:8], "test_123456", true)
	require(err)
	auth, err := account.Authenticate(ctx, login.AccountToken)
	require(err)
	startNode := func(run devconfig.RunServerSeed) *Store {
		t.Helper()
		nodeCfg := cfg
		nodeCfg.NodeID, nodeCfg.RunID = run.GameNodeID, run.RunID
		game, err := Open(ctx, nodeCfg)
		require(err)
		t.Cleanup(func() { game.Close(context.Background()) })
		grant, err := account.ApproveGame(ctx, nodeCfg.NodeID, game.Instance, nodeCfg.RunID)
		require(err)
		game.Epoch = grant.Epoch
		return game
	}
	east := startNode(cfg.RunServers[0])
	directory, err := account.ServerDirectory(ctx, auth)
	require(err)
	if len(directory.Regions) != 2 || directory.Regions[0].RegionId != "west" ||
		len(directory.Regions[0].Servers) != 1 || directory.Regions[0].Servers[0].Available ||
		directory.Regions[1].RegionId != "east" || len(directory.Regions[1].Servers) != 1 ||
		!directory.Regions[1].Servers[0].Available {
		t.Fatalf("目录排序、分区或节点可用性错误：%v", directory)
	}
	west := startNode(cfg.RunServers[1])
	newRole := func(origin uint64, name string) *pb.RoleKey {
		t.Helper()
		role, err := account.CreateRole(ctx, auth, &pb.CreateRoleRequest{OriginServerId: origin, Slot: 1, Name: name, OpId: domain.Bytes(domain.ID())})
		require(err)
		return role.Key
	}
	eastRole := newRole(11, "路由东服")
	westRole := newRole(12, "路由西服")
	for _, key := range []*pb.RoleKey{eastRole, westRole} {
		roles, err := account.Roles(ctx, auth, key.OriginServerId)
		require(err)
		if len(roles.Roles) != 1 || domain.Hex(roles.Roles[0].Key.RoleId) != domain.Hex(key.RoleId) {
			t.Fatalf("角色目录窜服：%v", roles)
		}
	}
	_, err = account.CreateRole(ctx, auth, &pb.CreateRoleRequest{OriginServerId: 99, Slot: 1, Name: "不存在服", OpId: domain.Bytes(domain.ID())})
	requireCode(err, p.CodeRouteChanged)
	issued, err := account.IssueTicket(ctx, auth, westRole)
	require(err)
	if issued.GameNodeId != "routing-west" || issued.RunServerId != 102 || issued.GameWssUrl != cfg.RunServers[1].GameWSURL || issued.RouteVersion != 1 {
		t.Fatalf("签票未使用持久部署：%v", issued)
	}
	_, _, err = east.Enter(ctx, issued.Ticket)
	requireCode(err, p.CodeRouteChanged)
	set := func(collection string, id any, fields bson.M) {
		t.Helper()
		result, err := account.DB.Collection(collection).UpdateOne(ctx, bson.M{"_id": id}, bson.M{"$set": fields})
		require(err)
		if result.MatchedCount != 1 {
			t.Fatalf("找不到测试记录 %s/%v", collection, id)
		}
	}
	set("origin_routes", uint64(12), bson.M{"route_version": int64(2), "name": "运营服名"})
	require(account.Migrate(ctx))
	directory, err = account.ServerDirectory(ctx, auth)
	require(err)
	if directory.Regions[0].Servers[0].Name != "运营服名" {
		t.Fatal("重启初始化覆盖了已有的运营目录")
	}
	_, _, err = west.Enter(ctx, issued.Ticket)
	requireCode(err, p.CodeRouteChanged)
	old, err := west.TicketRole(ctx, issued.Ticket)
	require(err)
	old.RouteVersion = 2
	_, _, err = west.EnterTicket(ctx, old, domain.ID(), true)
	requireCode(err, p.CodeUnauthenticated)
	fresh, err := account.IssueTicket(ctx, auth, westRole)
	require(err)
	if fresh.RouteVersion != 2 {
		t.Fatalf("重新签票的入口版本为 %d", fresh.RouteVersion)
	}
	verified, err := west.TicketRole(ctx, fresh.Ticket)
	require(err)
	attempt := domain.ID()
	admitted, binding, err := west.EnterTicket(ctx, verified, attempt, true)
	require(err)
	replayed, replayBinding, err := west.EnterTicket(ctx, verified, attempt, false)
	require(err)
	if admitted.Origin != 12 || admitted.SessionEpoch != 1 || replayed.Revision != admitted.Revision || replayBinding != binding {
		t.Fatalf("原入场尝试的回查改变角色归属、修订或会话：%+v", binding)
	}
	staleAssignment, err := account.IssueTicket(ctx, auth, westRole)
	require(err)
	set("run_servers", uint64(102), bson.M{"assignment_version": int64(2)})
	_, _, err = west.Enter(ctx, staleAssignment.Ticket)
	requireCode(err, p.CodeRouteChanged)
	_, err = account.IssueTicket(ctx, auth, westRole)
	requireCode(err, p.CodeLineRecovering)
	set("node_leases", east.Cfg.NodeID, bson.M{"lease_until": time.Unix(0, 0)})
	_, err = account.IssueTicket(ctx, auth, eastRole)
	requireCode(err, p.CodeLineRecovering)
}
