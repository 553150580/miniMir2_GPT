package integration

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/bootstrap"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
	"minimir/internal/wire"
)

func TestCompactTownDirectoryMongoWS(t *testing.T) {
	if os.Getenv("MINIMIR_MONGO_E2E") != "1" {
		t.Skip("显式开启专属Mongo副本集，验证当前规则24地图目录与真实WS")
	}
	cfg, err := devconfig.Load("../../../conf/dev/server.json")
	must(t, err)
	if cfg.Fixture.RuleVersion < 24 || len(cfg.Fixture.TownTravel.DestinationMapIDs) != 3 {
		t.Fatal("本组必须测试当前三座紧凑城镇")
	}
	cfg.Database = "minimir_dev_game_test_towns24_" + domain.ID()
	cfg.AccountAddress, cfg.GameAddress = address(t), address(t)
	ctx, cancel := context.WithCancel(context.Background())
	store, err := persistence.Open(ctx, cfg)
	must(t, err)
	var nodes sync.WaitGroup
	t.Cleanup(func() {
		cancel()
		nodes.Wait()
		cleanup, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
		must(t, store.DB.Drop(cleanup))
		store.Close(cleanup)
	})
	for _, mode := range []string{"account", "game"} {
		nodes.Go(func() {
			if err := bootstrap.Run(ctx, cfg, mode); err != nil && ctx.Err() == nil {
				t.Errorf("紧凑城镇%s节点：%v", mode, err)
			}
		})
	}
	healthy(t, cfg.AccountAddress)
	healthy(t, cfg.GameAddress)
	endpoint := "http://" + cfg.AccountAddress + "/rpc"
	login := &pb.LoginResult{}
	rpc(t, endpoint, "", p.MsgC2SLocalRegister, &pb.LocalLoginRequest{Account: "compact_" + domain.ID()[:8], Password: "test_123456"}, login, 0)
	role := &pb.RoleSummary{}
	rpc(t, endpoint, login.AccountToken, p.MsgC2SCreateRole, &pb.CreateRoleRequest{
		OriginServerId: 1, Slot: 1, Job: 0, Name: "村镇" + domain.ID()[:6], OpId: domain.Bytes(domain.ID()),
	}, role, 0)
	c := join(t, cfg.GameAddress, issue(t, endpoint, login.AccountToken, role.Key))
	defer func() { c.close() }()
	c.auto(false, 1)
	claim := c.asset(p.MsgC2SClaimHostingTime, &pb.ClaimHostingTimeRequest{Operation: c.operation()})
	if claim.ErrorCode != 0 {
		t.Fatal("正常领时失败", claim)
	}
	c.inventoryAt(claim.NewRevision)
	personal := proto.Clone(c.player()).(*pb.EntityState)
	start, err := store.Role(ctx, domain.Hex(role.Key.RoleId))
	must(t, err)
	rejectTravel := func(destination uint32, mapID uint64) {
		t.Helper()
		request := c.send(p.MsgC2SMapTravelPrepare, &pb.MapTravelPrepare{
			SceneEpoch: c.entry.SceneEpoch, TravelId: domain.ID(), Destination: destination, TargetMapId: mapID,
		})
		travelExpectError(c, p.MsgC2SMapTravelPrepare, request, p.CodeLineForbidden)
	}
	rejectTravel(1, 2)
	rejectTravel(1, 0)

	var committed *pb.BindingState
	for _, id := range cfg.Fixture.TownTravel.DestinationMapIDs {
		m, known := cfg.Fixture.MapByID(id)
		if !known || !m.CompactTown || len(m.NPCs) == 0 {
			t.Fatalf("地图%d缺少紧凑布局和NPC", id)
		}
		travelToBaseline(c, 1, id)
		if c.entry.SceneKind != devconfig.SceneKindTown || c.player().Position.X != int32(m.PlayerSpawn.X) ||
			c.player().Position.Y != int32(m.PlayerSpawn.Y) || travelCount(c.scene, 3) != 0 || travelCount(c.scene, 4) != 0 {
			t.Fatalf("地图%d的权威入场点、类型或战斗实体错误：%v", id, c.entry)
		}
		travelAcknowledge(c, true)
		rejectTravel(1, id)
		rejectTravel(2, 99)
		npc := m.NPCs[0].Position
		request := c.send(p.MsgC2SMoveIntent, &pb.MoveIntent{
			SceneEpoch: c.entry.SceneEpoch, Destination: &pb.GridPoint{X: int32(npc.X), Y: int32(npc.Y)},
		})
		travelExpectError(c, p.MsgC2SMoveIntent, request, p.CodeInvalidArgument)
		if id == cfg.Fixture.TownTravel.DestinationMapIDs[0] {
			if c.roleBinding == nil || c.roleBinding.Trusted || c.roleBinding.Binding != nil {
				t.Fatal("入城及非法NPC落格伪造了绑定")
			}
			zone := m.SafeZones[0]
			point := &pb.GridPoint{X: int32(zone.Center.X), Y: int32(zone.Center.Y)}
			c.send(p.MsgC2SMoveIntent, &pb.MoveIntent{SceneEpoch: c.entry.SceneEpoch, Destination: point})
			travelUntilWithin(c, 10*time.Second, func(f wire.Frame) bool {
				return f.ID == p.MsgS2CSceneBaseline && proto.Equal(c.player().Position, point) && c.scene.NavigationState.GetStatus() == 2
			})
			if len(c.bindingCommits) != 1 || !c.roleBinding.Trusted || c.roleBinding.SafeZoneId != zone.ID ||
				c.roleBinding.Binding.GetMapId() != id || !proto.Equal(c.roleBinding.Binding.GetCenter(), point) {
				t.Fatal("实际行走未提交当前地图的安全区绑定", c.roleBinding)
			}
			committed = proto.Clone(c.roleBinding).(*pb.BindingState)
			c.bindingCommits = nil
		} else {
			travelAssertBindingSnapshot(t, c, committed)
		}
	}
	standing, scene := proto.Clone(c.player()).(*pb.EntityState), c.entry.SceneEpoch
	lastTown := c.entry.RoomId
	c.close()
	c = join(t, cfg.GameAddress, issue(t, endpoint, login.AccountToken, role.Key))
	if c.entry.RoomId != lastTown || c.entry.SceneEpoch != scene || c.player().Life != standing.Life ||
		!proto.Equal(c.player().Position, standing.Position) {
		t.Fatal("健康重连重建了最后选择的城镇", c.entry)
	}
	travelAssertBindingSnapshot(t, c, committed)
	travelToBaseline(c, 2, cfg.Fixture.Map.ID)
	travelAcknowledge(c, false)
	if c.player().Life != personal.Life || !proto.Equal(c.player().Position, personal.Position) {
		t.Fatal("城镇间切换后没有返回原挂机地图和角色脚点", c.entry, c.player())
	}
	end, err := store.Role(ctx, domain.Hex(role.Key.RoleId))
	must(t, err)
	if end.HostingSeconds != start.HostingSeconds || end.Wallet != start.Wallet || end.EconomicSeq != start.EconomicSeq ||
		end.SourceSeq != start.SourceSeq+1 || end.BindingSafeZoneID != committed.SafeZoneId {
		t.Fatal("城镇旅行或重连改变托管、经济或已核绑定", start, end)
	}
}
