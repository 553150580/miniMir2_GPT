// 本文件用隔离真实 MongoDB 和 WebSocket 验证世界首领的共享格战、榜单和运行服边界。
package integration

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
	"go.mongodb.org/mongo-driver/v2/bson"
	"google.golang.org/protobuf/proto"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/bootstrap"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
	"minimir/internal/wire"
)

// TestSharedWorldBossMongoWS 验证真正的两角色同场作战和第三个角色的逻辑运行服隔离。
func TestSharedWorldBossMongoWS(t *testing.T) {
	if os.Getenv("MINIMIR_MONGO_E2E") != "1" {
		t.Skip("需要已核验的专属 MongoDB 副本集")
	}
	cfg, err := loadIntegrationConfig()
	must(t, err)
	cfg.Database = "minimir_dev_game_test_wb_" + domain.ID()
	cfg.AccountAddress, cfg.GameAddress = address(t), address(t)
	cfg.OriginID, cfg.RunID = 1, 1
	cfg.RunServerIDs = []uint64{1, 2}
	firstRun := makeDirectoryEntry(1, 1, cfg, "world-test", "共享首领", "一服", 0, 0)
	secondRun := makeDirectoryEntry(2, 2, cfg, "world-test", "共享首领", "二服", 0, 1)
	cfg.OriginRoutes = []devconfig.OriginRouteSeed{firstRun.origin, secondRun.origin}
	cfg.RunServers = []devconfig.RunServerSeed{firstRun.assignment, secondRun.assignment}
	// 固定隔离武器的随机区间，命中仍必须经真实地格、方向、攻击时钟和防御结算。
	for i, item := range cfg.Fixture.Items {
		if item.ID == 218 {
			cfg.Fixture.Items[i].AttackMinBonus, cfg.Fixture.Items[i].AttackMaxBonus = 3, 3
		}
	}
	monster := cfg.Fixture.Monsters[0]
	monster.ID, monster.HP, monster.AttackMin, monster.AttackMax, monster.Agility = 80_002, 9, 0, 0, 0
	monster.MoveMS, monster.AttackMS, monster.DefenceMin, monster.DefenceMax = 4_000, 4_000, 0, 0
	monster.Exp, monster.GoldMin, monster.GoldMax, monster.RespawnMS = 0, 0, 0, 0
	local := time.Now().In(time.FixedZone("Asia/Shanghai", 8*60*60))
	midnight := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, local.Location())
	offset := (local.Sub(midnight).Truncate(time.Millisecond) + 24*time.Hour - time.Minute) % (24 * time.Hour)
	cfg.Fixture.WorldBoss = &devconfig.WorldBossConfig{TemplateID: 2, MinimumLevel: 1,
		Map: devconfig.MapTemplate{ID: 81, Width: 13, Height: 13, SceneKind: devconfig.SceneKindWorldBoss,
			PlayerSpawn: devconfig.Point{X: 5, Y: 5}, MonsterSpawns: []devconfig.Point{{X: 6, Y: 5}}},
		Monster: monster, DailyOpenOffsets: []time.Duration{offset}, Window: 3 * time.Minute, ReturnDelay: 300 * time.Millisecond, ParticipantLimit: 8}
	cfg.Fixture.WorldBossVariants = nil
	if !cfg.Fixture.ValidWorldBoss() {
		t.Fatal("共享首领隔离配置无效")
	}
	ctx, cancel := context.WithCancel(context.Background())
	inspection, err := persistence.Open(ctx, cfg)
	if err != nil {
		cancel()
		must(t, err)
	}
	var nodes sync.WaitGroup
	t.Cleanup(func() {
		cancel()
		nodes.Wait()
		cleanup, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
		defer inspection.Close(cleanup)
		must(t, inspection.DB.Drop(cleanup))
	})
	for _, mode := range []string{"account", "game"} {
		nodes.Go(func() {
			if err := bootstrap.Run(ctx, cfg, mode); err != nil && ctx.Err() == nil {
				t.Errorf("隔离 %s：%v", mode, err)
			}
		})
	}
	healthy(t, cfg.AccountAddress)
	healthy(t, cfg.GameAddress)
	verifyHostedRunIDs(t, cfg)
	endpoint := "http://" + cfg.AccountAddress + "/rpc"
	first, firstRole, firstToken := joinWorldBossTestRole(t, endpoint, firstRun)
	defer func() { first.close() }()
	second, secondRole, _ := joinWorldBossTestRole(t, endpoint, firstRun)
	defer second.close()
	third, thirdRole, _ := joinWorldBossTestRole(t, endpoint, secondRun)
	defer third.close()
	firstOpen, secondOpen, thirdOpen := queryWorldBossTest(t, first), queryWorldBossTest(t, second), queryWorldBossTest(t, third)
	if firstOpen.State != 3 || secondOpen.State != 3 || thirdOpen.State != 3 || firstOpen.OccurrenceId != secondOpen.OccurrenceId || firstOpen.OccurrenceId == thirdOpen.OccurrenceId {
		t.Fatal("场次编号或开放期没有按运行服隔离", firstOpen, secondOpen, thirdOpen)
	}
	// 不同服有效场次号不能授权原运行服进入。
	invalid := third.send(p.MsgC2SWorldBossEnter, &pb.WorldBossEnter{SceneEpoch: third.entry.SceneEpoch, TemplateId: firstOpen.TemplateId, OccurrenceId: firstOpen.OccurrenceId})
	travelExpectError(third, p.MsgC2SWorldBossEnter, invalid, p.CodeLineForbidden)
	enterWorldBossTest(t, first, firstOpen)
	enterWorldBossTest(t, second, secondOpen)
	enterWorldBossTest(t, third, thirdOpen)
	travelUntilWithin(first, 3*time.Second, func(frame wire.Frame) bool {
		return frame.ID == p.MsgS2CSceneBaseline && townPeer(first, secondRole.Name) != nil
	})
	if peer := townPeer(first, secondRole.Name); proto.Equal(peer.Position, first.player().Position) || peer.EntityHandle == first.entry.PlayerHandle || townPeer(first, thirdRole.Name) != nil {
		t.Fatal("共享在场角色占格、句柄或异服隔离错误", peer)
	}
	if townPeer(third, firstRole.Name) != nil || townPeer(third, secondRole.Name) != nil {
		t.Fatal("同进程的第二运行服看到了第一运行服角色")
	}
	// 第二个角色需要亲自移动到共享实体附近，越过首个角色的占格。
	goal := &pb.GridPoint{X: 6, Y: 4}
	second.send(p.MsgC2SMoveIntent, &pb.MoveIntent{SceneEpoch: second.entry.SceneEpoch, Destination: goal})
	travelUntilWithin(second, 5*time.Second, func(frame wire.Frame) bool {
		return frame.ID == p.MsgS2CSceneBaseline && proto.Equal(second.player().Position, goal)
	})
	attackWorldBossTest(first)
	stateOne := waitWorldBossTest(t, second, 0, func(state *pb.WorldBossState) bool { return state.State == 3 && state.Hp == 6 })
	if len(stateOne.Ranks) != 1 || stateOne.Ranks[0].DisplayName != firstRole.Name || stateOne.Ranks[0].Damage != 3 {
		t.Fatal("首个真实命中没有写入共享生命及归属排行", stateOne)
	}
	travelUntilWithin(second, 3*time.Second, func(frame wire.Frame) bool {
		return frame.ID == p.MsgS2CSceneBaseline && travelEntity(second.scene, 3).Hp == 6
	})
	travelUntilWithin(first, 3*time.Second, func(frame wire.Frame) bool {
		return frame.ID == p.MsgS2CSceneBaseline && first.player().AttackSequence > 0
	})
	beforeDrop := proto.Clone(first.player()).(*pb.EntityState)
	oldBinding, oldBaseline := first.binding, first.entry.BaselineId
	first.close()
	travelUntilWithin(second, 3*time.Second, func(frame wire.Frame) bool {
		return frame.ID == p.MsgS2CSceneBaseline && townPeer(second, firstRole.Name) == nil
	})
	fresh := directoryTicket(t, endpoint, firstToken, firstRole.Key, firstRun)
	first = rejoinWorldBossTest(t, directoryTicketAddress(t, fresh), fresh.Ticket, firstOpen.OccurrenceId)
	if restored := first.player(); first.binding == oldBinding || first.entry.BaselineId <= oldBaseline || restored.EntityHandle != beforeDrop.EntityHandle ||
		restored.Life != beforeDrop.Life || restored.Hp != beforeDrop.Hp || restored.Mp != beforeDrop.Mp || restored.AttackSequence != beforeDrop.AttackSequence ||
		!proto.Equal(restored.Position, beforeDrop.Position) || travelEntity(first.scene, 3).Hp != 6 {
		t.Fatal("掉线重进未复用原世界首领席位或重置了战斗状态", restored, beforeDrop)
	}
	// 普通行走动作锁到期后发出第二个玩家的攻击。
	time.Sleep(650 * time.Millisecond)
	attackWorldBossTest(second)
	stateTwo := waitWorldBossTest(t, first, 0, func(state *pb.WorldBossState) bool { return state.State == 3 && state.Hp == 3 })
	if len(stateTwo.Ranks) != 2 || stateTwo.Ranks[0].Damage+stateTwo.Ranks[1].Damage != 6 {
		t.Fatal("两个角色的真实扣血没有合并到唯一首领实体", stateTwo)
	}
	first.send(p.MsgC2SWorldBossLeave, &pb.WorldBossLeave{SceneEpoch: first.entry.SceneEpoch, OccurrenceId: firstOpen.OccurrenceId})
	waitWorldBossTest(t, first, 0, func(state *pb.WorldBossState) bool { return state.State == 6 })
	returnFromWorldBossTest(first, firstOpen.OccurrenceId)
	left := first.send(p.MsgC2SWorldBossEnter, &pb.WorldBossEnter{SceneEpoch: first.entry.SceneEpoch, TemplateId: firstOpen.TemplateId, OccurrenceId: firstOpen.OccurrenceId})
	travelExpectError(first, p.MsgC2SWorldBossEnter, left, p.CodeLineForbidden)
	travelUntilWithin(second, 3*time.Second, func(frame wire.Frame) bool {
		return frame.ID == p.MsgS2CSceneBaseline && townPeer(second, firstRole.Name) == nil
	})
	time.Sleep(1400 * time.Millisecond)
	attackWorldBossTest(second)
	final := waitWorldBossTest(t, second, 0, func(state *pb.WorldBossState) bool { return state.State == 4 })
	if second.entry.SceneKind != devconfig.SceneKindWorldBoss || final.SceneEpoch != second.entry.SceneEpoch || final.Hp != 0 ||
		len(final.Ranks) != 2 || final.Ranks[0].DisplayName != secondRole.Name || final.Ranks[0].Damage != 6 ||
		final.Ranks[1].DisplayName != firstRole.Name || final.Ranks[1].Damage != 3 {
		t.Fatal("击杀终态、排行或已离场参与者的归属丢失", final)
	}
	returnFromWorldBossTest(second, firstOpen.OccurrenceId)
	if firstFinal := queryWorldBossTest(t, first); firstFinal.State != 4 || firstFinal.Hp != 0 || len(firstFinal.Ranks) != 2 {
		t.Fatal("同一场击杀后重新查询时首领复活或结果丢失", firstFinal)
	}
	if other := queryWorldBossTest(t, third); other.State != 3 || other.Hp != 9 || len(other.Ranks) != 0 {
		t.Fatal("运行服间串了首领生命或排行榜", other)
	}
	// 第二运行服用首领的独立自动开关产生真实排行，未发送手动攻击。
	setChallengeAuto(third, true)
	active := waitWorldBossTest(t, third, 0, func(state *pb.WorldBossState) bool { return state.Hp < 9 && len(state.Ranks) != 0 })
	if active.Ranks[0].DisplayName != thirdRole.Name || active.Ranks[0].Damage != 9-active.Hp {
		t.Fatal("首领自动攻击没有计入真实共享伤害和角色排行", active)
	}
	count, err := inspection.DB.Collection("audit_outbox").CountDocuments(ctx, bson.M{"kind": "settlement", "$or": bson.A{
		bson.M{"settlement.map_id": cfg.Fixture.WorldBoss.Map.ID}, bson.M{"settlement.monster_id": monster.ID},
	}})
	must(t, err)
	if count != 0 {
		t.Fatal("共享首领进入了普通挂机资产结算", count)
	}
}

// joinWorldBossTestRole 走正常登录、认证票、角色创建和穿戴流程。
func joinWorldBossTestRole(t *testing.T, endpoint string, destination directoryTestEntry) (*client, *pb.RoleSummary, string) {
	t.Helper()
	login := &pb.LoginResult{}
	rpc(t, endpoint, "", p.MsgC2SLocalRegister, &pb.LocalLoginRequest{Account: "world_" + domain.ID()[:8], Password: "test_123456"}, login, 0)
	role := createDirectoryRole(t, endpoint, login.AccountToken, destination.origin.OriginID, "世界首领")
	ticket := directoryTicket(t, endpoint, login.AccountToken, role.Key, destination)
	c := join(t, directoryTicketAddress(t, ticket), ticket.Ticket)
	c.auto(false, 1)
	for _, item := range c.inventory.Items {
		if item.ItemId != 218 {
			continue
		}
		result := c.asset(p.MsgC2SEquip, &pb.EquipRequest{ItemInstanceId: item.InstanceId, ItemVersion: item.Version, Slot: 0, Operation: c.operation()})
		if result.ErrorCode != 0 {
			t.Fatal("世界首领测试角色正常装备失败", result)
		}
		c.inventoryAt(result.NewRevision)
		return c, role, login.AccountToken
	}
	t.Fatal("世界首领测试角色缺少木剑")
	return nil, nil, ""
}

// rejoinWorldBossTest 先用新票确认原角色，再接受服务器自动恢复的旧场次席位和新基线。
func rejoinWorldBossTest(t *testing.T, address, ticket, occurrence string) *client {
	t.Helper()
	ctx, stop := context.WithTimeout(context.Background(), 20*time.Second)
	ws, _, err := websocket.Dial(ctx, "ws://"+address+"/ws", nil)
	if err != nil {
		stop()
		must(t, err)
	}
	c := &client{t: t, ws: ws, ctx: ctx, stop: stop}
	t.Cleanup(c.close)
	c.send(p.MsgC2SEnterLine, &pb.EnterLineRequest{AccountTicket: ticket})
	travelUntilWithin(c, 5*time.Second, func(frame wire.Frame) bool {
		return frame.ID == p.MsgS2CSceneBaseline && c.entry.SceneKind == devconfig.SceneKindPersonal && c.scene.SceneEpoch == c.entry.SceneEpoch
	})
	travelAcknowledge(c, false)
	entry := waitWorldBossTest(t, c, 0, func(state *pb.WorldBossState) bool { return state.State == 2 })
	travelUntilWithin(c, 5*time.Second, func(frame wire.Frame) bool {
		return frame.ID == p.MsgS2CSceneBaseline && c.entry.SceneKind == devconfig.SceneKindWorldBoss && c.scene.SceneEpoch == c.entry.SceneEpoch
	})
	if entry.OccurrenceId != occurrence || c.entry.ServerTransitionId != occurrence || c.entry.SceneEpoch != entry.SceneEpoch {
		t.Fatal("健康重连恢复了错误的活动场次", entry, c.entry)
	}
	travelAcknowledge(c, false)
	requireChallengeAuto(c, false)
	return c
}

// waitWorldBossTest 等待真实连接上对应请求的场次状态。
func waitWorldBossTest(t *testing.T, c *client, request uint32, accept func(*pb.WorldBossState) bool) *pb.WorldBossState {
	t.Helper()
	var outcome *pb.WorldBossState
	travelUntilWithin(c, 5*time.Second, func(frame wire.Frame) bool {
		if frame.ID != p.MsgS2CWorldBossState || frame.Request != request {
			return false
		}
		state := &pb.WorldBossState{}
		must(t, proto.Unmarshal(frame.Payload, state))
		if !accept(state) {
			return false
		}
		outcome = state
		return true
	})
	return outcome
}

// queryWorldBossTest 查阅当前场次元信息，不承担入场权限。
func queryWorldBossTest(t *testing.T, c *client) *pb.WorldBossState {
	t.Helper()
	request := c.send(p.MsgC2SWorldBossQuery, &pb.WorldBossQuery{SceneEpoch: c.entry.SceneEpoch})
	return waitWorldBossTest(t, c, request, func(*pb.WorldBossState) bool { return true })
}

// enterWorldBossTest 确认共享地图的真实握手，旧场景 ACK 不能允许攻击。
func enterWorldBossTest(t *testing.T, c *client, open *pb.WorldBossState) {
	t.Helper()
	old := c.entry.SceneEpoch
	request := c.send(p.MsgC2SWorldBossEnter, &pb.WorldBossEnter{SceneEpoch: old, TemplateId: open.TemplateId, OccurrenceId: open.OccurrenceId})
	entering := waitWorldBossTest(t, c, request, func(state *pb.WorldBossState) bool { return state.State == 2 })
	travelUntilWithin(c, 5*time.Second, func(frame wire.Frame) bool {
		return frame.ID == p.MsgS2CSceneBaseline && c.entry.SceneKind == devconfig.SceneKindWorldBoss && c.scene.SceneEpoch == c.entry.SceneEpoch
	})
	if entering.SceneEpoch != c.entry.SceneEpoch || c.entry.ServerTransitionId != open.OccurrenceId || c.entry.PlayerHandle <= 10 {
		t.Fatal("共享地图未下发正确的新场景、落点和玩家句柄", c.entry)
	}
	c.send(p.MsgC2SEnterWorldAck, &pb.EnterWorldAck{SceneEpoch: old, BaselineId: c.entry.BaselineId})
	tooEarly := attackWorldBossTest(c)
	travelExpectError(c, p.MsgC2SCastIntent, tooEarly, p.CodeLineRecovering)
	travelAcknowledge(c, false, false)
	requireChallengeAuto(c, false)
}

// attackWorldBossTest 从已有场景中引用真实首领句柄，而不是提交伤害或排行榜字段。
func attackWorldBossTest(c *client) uint32 {
	boss := travelEntity(c.scene, 3)
	return c.send(p.MsgC2SCastIntent, &pb.CastIntent{SceneEpoch: c.entry.SceneEpoch, TargetHandle: boss.EntityHandle})
}

// returnFromWorldBossTest 确认真正恢复原普通地图的附着和基线。
func returnFromWorldBossTest(c *client, occurrence string) {
	travelUntilWithin(c, 5*time.Second, func(frame wire.Frame) bool {
		return frame.ID == p.MsgS2CSceneBaseline && c.entry.SceneKind == devconfig.SceneKindPersonal && c.scene.SceneEpoch == c.entry.SceneEpoch
	})
	if c.entry.ServerTransitionId != occurrence || c.entry.PlayerHandle != 1 {
		c.t.Fatal("返回场景未恢复原角色而是复用共享场次身体", c.entry)
	}
	travelAcknowledge(c, false)
}
