// 验证登录后的区域与城镇目录，以及客户端可选的紧凑城镇地图与交互入口。

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

// TestCompactTownDirectoryMongoWS 验证服务器通过真实连接下发并接入紧凑城镇目录。
// t 提供断言失败报告、子用例和清理注册。
func TestCompactTownDirectoryMongoWS(t *testing.T) {
	if os.Getenv("MINIMIR_MONGO_E2E") != "1" {
		t.Skip("显式开启专属Mongo副本集，验证当前规则24地图目录与真实WS")
	}
	// cfg 保存由加载取得的服务配置，err 接收加载过程中可能产生的错误，供后续失败分支处理。
	cfg, err := loadIntegrationConfig()
	must(t, err)
	if cfg.Fixture.RuleVersion < 24 || len(cfg.Fixture.TownTravel.DestinationMapIDs) != 3 {
		t.Fatal("本组必须测试当前三座紧凑城镇")
	}
	cfg.Database = "minimir_dev_game_test_towns24_" + domain.ID()
	cfg.AccountAddress, cfg.GameAddress = address(t), address(t)
	// ctx 传递从调用方派生的上下文，供取消或控制截止时间，cancel 取消对应上下文或停止计时，释放其关联资源。
	ctx, cancel := context.WithCancel(context.Background())
	// store 保存由打开取得的持久层存储。
	store, err := persistence.Open(ctx, cfg)
	must(t, err)
	// nodes 记录节点集合，使用协程汇合计数器表示。
	var nodes sync.WaitGroup
	t.Cleanup(func() {
		cancel()
		nodes.Wait()
		// cleanup 传递从调用方派生的上下文，供取消或控制截止时间，stop 取消对应上下文或停止计时，释放其关联资源。
		cleanup, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
		must(t, store.DB.Drop(cleanup))
		store.Close(cleanup)
	})
	// mode 接收遍历文本列表时当前的元素。
	for _, mode := range []string{"account", "game"} {
		nodes.Go(func() {
			// err 接收运行批次过程中可能产生的错误，供后续失败分支处理。
			if err := bootstrap.Run(ctx, cfg, mode); err != nil && ctx.Err() == nil {
				t.Errorf("紧凑城镇%s节点：%v", mode, err)
			}
		})
	}
	healthy(t, cfg.AccountAddress)
	healthy(t, cfg.GameAddress)
	// endpoint 保存根据当前数据与账号地址的计算结果与当前数据计算的端点。
	endpoint := "http://" + cfg.AccountAddress + "/rpc"
	// login 保存当前处理所用的登录。
	login := &pb.LoginResult{}
	rpc(t, endpoint, "", p.MsgC2SLocalRegister, &pb.LocalLoginRequest{Account: "compact_" + domain.ID()[:8], Password: "test_123456"}, login, 0)
	// role 保存当前处理所用的角色。
	role := &pb.RoleSummary{}
	rpc(t, endpoint, login.AccountToken, p.MsgC2SCreateRole, &pb.CreateRoleRequest{
		OriginServerId: 1, Slot: 1, Job: 0, Name: "村镇" + domain.ID()[:6], OpId: domain.Bytes(domain.ID()),
	}, role, 0)
	// prefs 保存当前处理所用的“prefs”。
	prefs := &pb.AutoSkillSettings{}
	// c 接收加入带有自动技能集合供后续步骤使用。
	c := joinWithAutoSkills(t, cfg.GameAddress, issue(t, endpoint, login.AccountToken, role.Key), prefs)
	defer func() { c.close() }()
	c.auto(false, 1)
	// claim 保存由资产取得的领取。
	claim := c.asset(p.MsgC2SClaimHostingTime, &pb.ClaimHostingTimeRequest{Operation: c.operation()})
	if claim.ErrorCode != 0 {
		t.Fatal("正常领时失败", claim)
	}
	c.inventoryAt(claim.NewRevision)
	// personal 按目标类型读取当前接口值，得到“personal”。
	personal := proto.Clone(c.player()).(*pb.EntityState)
	// start 保存由角色取得的起点或开始时间。
	start, err := store.Role(ctx, domain.Hex(role.Key.RoleId))
	must(t, err)
	// rejectTravel 封装“reject”地图切换对应的后续处理，destination 记录目的地，使用数值表示，mapID 标识当前业务使用的地图。
	rejectTravel := func(destination uint32, mapID uint64) {
		t.Helper()
		// request 保存由发送取得的请求。
		request := c.send(p.MsgC2SMapTravelPrepare, &pb.MapTravelPrepare{
			SceneEpoch: c.entry.SceneEpoch, TravelId: domain.ID(), Destination: destination, TargetMapId: mapID,
		})
		travelExpectError(c, p.MsgC2SMapTravelPrepare, request, p.CodeLineForbidden)
	}
	rejectTravel(1, 2)
	rejectTravel(1, 0)

	// committed 记录已提交，使用会话绑定状态表示。
	var committed *pb.BindingState
	// id 接收遍历目的地地图i“ds”时当前的元素。
	for _, id := range cfg.Fixture.TownTravel.DestinationMapIDs {
		// m 接收地图“by”标识供后续步骤使用，known 保存由地图“by”标识取得的已知状态。
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
		if !proto.Equal(c.hosting.GetAutoSkillSettings(), prefs) {
			t.Fatal("旅行ACK未带偏好时覆盖了当前room的设置")
		}
		rejectTravel(1, id)
		rejectTravel(2, 99)
		// npc 保存位置，用于当前步骤中的NPC。
		npc := m.NPCs[0].Position
		// request 保存由发送取得的请求。
		request := c.send(p.MsgC2SMoveIntent, &pb.MoveIntent{
			SceneEpoch: c.entry.SceneEpoch, Destination: &pb.GridPoint{X: int32(npc.X), Y: int32(npc.Y)},
		})
		travelExpectError(c, p.MsgC2SMoveIntent, request, p.CodeInvalidArgument)
		if id == cfg.Fixture.TownTravel.DestinationMapIDs[0] {
			if c.roleBinding == nil || c.roleBinding.Trusted || c.roleBinding.Binding != nil {
				t.Fatal("入城及非法NPC落格伪造了绑定")
			}
			// zone 保存从安全区域集合取出的区域。
			zone := m.SafeZones[0]
			// point 保存当前处理所用的点位。
			point := &pb.GridPoint{X: int32(zone.Center.X), Y: int32(zone.Center.Y)}
			c.send(p.MsgC2SMoveIntent, &pb.MoveIntent{SceneEpoch: c.entry.SceneEpoch, Destination: point})
			// f 记录协议帧。
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
	// standing 按目标类型读取当前接口值，得到“standing”，scene 保存场景代次，用于当前步骤中的场景。
	standing, scene := proto.Clone(c.player()).(*pb.EntityState), c.entry.SceneEpoch
	// lastTown 保存房间标识，用于当前步骤中的上一次状态城镇。
	lastTown := c.entry.RoomId
	c.close()
	c = join(t, cfg.GameAddress, issue(t, endpoint, login.AccountToken, role.Key))
	if !proto.Equal(c.hosting.GetAutoSkillSettings(), prefs) {
		t.Fatal("旧客户端健康重连覆盖了当前room的设置")
	}
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
	// end 保存由角色取得的结束位置或时间。
	end, err := store.Role(ctx, domain.Hex(role.Key.RoleId))
	must(t, err)
	if end.HostingSeconds != start.HostingSeconds || end.Wallet != start.Wallet || end.EconomicSeq != start.EconomicSeq ||
		end.SourceSeq != start.SourceSeq+1 || end.BindingSafeZoneID != committed.SafeZoneId {
		t.Fatal("城镇旅行或重连改变托管、经济或已核绑定", start, end)
	}
}
