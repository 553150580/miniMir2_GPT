package integration

import (
	"context"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/bootstrap"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
	"minimir/internal/wire"
	"os"
	"reflect"
	"sync"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"google.golang.org/protobuf/proto"
)

// 随机专属库、正常注册创角与真实100ms owner tick；不改临时角色/怪物坐标或生命。
func TestMapTravelMongoWS(t *testing.T) {
	if os.Getenv("MINIMIR_MONGO_E2E") != "1" {
		t.Skip("设置MINIMIR_MONGO_E2E=1连接已核验所有权的本任务专属副本集")
	}
	cfg, err := devconfig.Load("../../../conf/dev/server.json")
	must(t, err)
	if cfg.Fixture.TownTravel == nil || !cfg.Fixture.TownTravel.Enabled || cfg.Fixture.TownNavigation == nil || !cfg.Fixture.TownNavigation.Enabled {
		t.Fatal("真实城镇业务测试要求唯一Excel已显式启用城镇旅行/有界导航")
	}
	cfg.Database = "minimir_dev_game_test_town_" + domain.ID()
	cfg.AccountAddress, cfg.GameAddress, cfg.GMAddress = address(t), address(t), address(t)
	ctx := context.Background()
	inspection, err := persistence.Open(ctx, cfg)
	must(t, err)
	launch := func() func() {
		runCtx, cancel := context.WithCancel(context.Background())
		var group sync.WaitGroup
		for _, mode := range []string{"account", "game", "gm"} {
			group.Go(func() {
				if runErr := bootstrap.Run(runCtx, cfg, mode); runErr != nil && runCtx.Err() == nil {
					t.Errorf("%s节点: %v", mode, runErr)
				}
			})
		}
		for _, endpoint := range []string{cfg.AccountAddress, cfg.GameAddress, cfg.GMAddress} {
			healthy(t, endpoint)
		}
		return func() { cancel(); group.Wait() }
	}
	stop := launch()
	t.Cleanup(func() {
		stop()
		cleanup, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
		must(t, inspection.DB.Drop(cleanup))
		inspection.Close(cleanup)
	})
	accountURL := "http://" + cfg.AccountAddress + "/rpc"
	login := &pb.LoginResult{}
	rpc(t, accountURL, "", p.MsgC2SLocalRegister, &pb.LocalLoginRequest{Account: "town_" + domain.ID()[:10], Password: "test_123456"}, login, 0)
	role := &pb.RoleSummary{}
	rpc(t, accountURL, login.AccountToken, p.MsgC2SCreateRole, &pb.CreateRoleRequest{OriginServerId: 1, Slot: 1, Job: 2, Name: "城镇" + domain.ID()[:6], OpId: domain.Bytes(domain.ID())}, role, 0)
	roleID := domain.Hex(role.Key.RoleId)
	// 与首循环集成相同的合法永久前态：真正击杀第20只触发确定装备掉落，不造临时现场。
	_, err = inspection.DB.Collection("roles").UpdateOne(ctx, bson.M{"_id": roleID}, bson.M{"$set": bson.M{"no_equipment_kills": 19}})
	must(t, err)
	c := join(t, cfg.GameAddress, issue(t, accountURL, login.AccountToken, role.Key))
	defer func() { c.close() }()
	if c.roleBinding == nil || c.roleBinding.Binding != nil || c.roleBinding.Trusted || c.roleBinding.SourceSeq != 0 || c.roleBinding.Cause != 1 {
		t.Fatal("正常新角色未收到明确尚未绑定快照，或随机初始化被隐式开启", c.roleBinding)
	}
	c.auto(false, 1)
	claim := c.asset(p.MsgC2SClaimHostingTime, &pb.ClaimHostingTimeRequest{Operation: c.operation()})
	if claim.ErrorCode != 0 {
		t.Fatalf("正常领时失败: %v", claim)
	}
	c.inventoryAt(claim.NewRevision)
	c.auto(true, 1)
	travelUntilWithin(c, 30*time.Second, func(f wire.Frame) bool {
		return f.ID == p.MsgS2CSceneBaseline && travelEntity(c.scene, 3) != nil && travelEntity(c.scene, 3).Hp == 0 && travelEntity(c.scene, 4) != nil
	})
	c.auto(false, 1)
	travelUntilWithin(c, time.Second, func(f wire.Frame) bool {
		return f.ID == p.MsgS2CSceneBaseline && travelEntity(c.scene, 3) != nil && travelEntity(c.scene, 3).Hp == 0 && travelEntity(c.scene, 4) != nil
	})
	personal := proto.Clone(c.scene).(*pb.SceneBaselineChunk)
	personalPlayer := proto.Clone(c.player()).(*pb.EntityState)
	deadMonster := proto.Clone(travelEntity(personal, 3)).(*pb.EntityState)
	drop := proto.Clone(travelEntity(personal, 4)).(*pb.EntityState)
	if personalPlayer.Hp <= 0 {
		t.Fatal("真实战斗死亡，不能作为健康旅行前态")
	}
	initialScene := c.entry.SceneEpoch
	travelToBaseline(c, 1, 2)
	if c.entry.SceneKind != 2 || c.entry.Position.X != 633 || c.entry.Position.Y != 613 || c.player().Position.X != 633 || c.player().Position.Y != 613 || c.entry.SceneEpoch != initialScene+1 {
		t.Fatalf("城镇目录/来源入口错误: %v %v", c.entry, c.player())
	}
	if travelCount(c.scene, 3) != 0 || travelCount(c.scene, 4) != 0 {
		t.Fatal("城镇伪造怪物或带入个人图掉落")
	}
	if travelSafeZone(t, c.scene) != 0 {
		t.Fatal("开发入口被误当来源安全区中心")
	}
	if c.roleBinding == nil || c.roleBinding.SceneEpoch != c.entry.SceneEpoch || c.roleBinding.Binding != nil || c.roleBinding.Trusted {
		t.Fatal("健康进城未同步新scene绑定快照，或入城被当作Walk绑定", c.roleBinding)
	}
	travelAcknowledge(c, true)
	oldMove := c.send(p.MsgC2SMoveIntent, &pb.MoveIntent{SceneEpoch: initialScene, Direction: 2})
	travelExpectError(c, p.MsgC2SMoveIntent, oldMove, p.CodeInvalidArgument)
	inTown, err := inspection.Role(ctx, roleID)
	must(t, err)
	if inTown.HostingSeconds != 21600 {
		t.Fatalf("在线旅行扣了托管秒数: %d", inTown.HostingSeconds)
	}
	townMap, ok := cfg.Fixture.MapByID(2)
	if !ok {
		t.Fatal("唯一目录缺城镇")
	}
	target := travelNearbyGround(t, townMap, c.scene, c.player().Position)
	for _, zone := range townMap.SafeZones {
		if zone.ID == 106 {
			target = &pb.GridPoint{X: int32(zone.Center.X), Y: int32(zone.Center.Y)}
		}
	}
	if target.X != 650 || target.Y != 631 {
		t.Fatal("唯一目录未配置来源银杏安全区106中心650,631")
	}
	c.send(p.MsgC2SMoveIntent, &pb.MoveIntent{SceneEpoch: c.entry.SceneEpoch, Destination: target})
	travelUntilWithin(c, 18*time.Second, func(f wire.Frame) bool {
		return f.ID == p.MsgS2CSceneBaseline && proto.Equal(c.player().Position, target) && c.scene.NavigationState.GetStatus() == 2
	})
	if travelSafeZone(t, c.scene) != 106 {
		t.Fatal("正常逐tick走到来源安全区中心，未同步首匹配ID106")
	}
	if len(c.bindingCommits) != 1 {
		t.Fatalf("首次到访应仅产生一次绑定提交，实际%d", len(c.bindingCommits))
	}
	committed := proto.Clone(c.bindingCommits[0]).(*pb.BindingState)
	if !committed.Trusted || committed.Cause != 2 || committed.SafeZoneId != 106 || committed.Binding.GetMapId() != 2 || !proto.Equal(committed.Binding.GetCenter(), target) || committed.SceneEpoch != c.entry.SceneEpoch || domain.Hex(committed.Role.GetRoleId()) != roleID || committed.Role.GetOriginServerId() != 1 {
		t.Fatal("合法来源Walk没有产生正确的已提交中心/角色/scene证明", committed)
	}
	boundRole, err := inspection.Role(ctx, roleID)
	must(t, err)
	if boundRole.Binding == nil || *boundRole.Binding != (domain.RoleBinding{MapID: 2, X: 650, Y: 631}) || boundRole.BindingSourceSeq != boundRole.SourceSeq || boundRole.SourceSeq != inTown.SourceSeq+1 || boundRole.Revision != inTown.Revision+1 || boundRole.EconomicSeq != inTown.EconomicSeq || committed.SourceSeq != uint64(boundRole.BindingSourceSeq) || committed.RoleRevision != uint64(boundRole.Revision) {
		t.Fatal("绑定未独立原子推进一次SourceSeq/revision，或动了EconomicSeq", boundRole, committed)
	}
	// 在同安全区继续成功Walk，随后真实走出区外再返回，不重新提交同一可信中心。
	for _, next := range []*pb.GridPoint{travelNearbyGround(t, townMap, c.scene, target), {X: 633, Y: 613}, target} {
		c.send(p.MsgC2SMoveIntent, &pb.MoveIntent{SceneEpoch: c.entry.SceneEpoch, Destination: next})
		travelUntilWithin(c, 18*time.Second, func(f wire.Frame) bool {
			return f.ID == p.MsgS2CSceneBaseline && proto.Equal(c.player().Position, next) && c.scene.NavigationState.GetStatus() == 2
		})
		if next.X == 633 && travelSafeZone(t, c.scene) != 0 {
			t.Fatal("区外落点错误")
		}
	}
	if len(c.bindingCommits) != 1 {
		t.Fatal("可信同中心或区外Walk重复提交绑定")
	}
	afterWalks, err := inspection.Role(ctx, roleID)
	must(t, err)
	travelAssertEconomy(t, boundRole, afterWalks, "同中心及区外Walk")
	if afterWalks.BindingSourceSeq != boundRole.BindingSourceSeq || afterWalks.Revision != boundRole.Revision {
		t.Fatal("无新绑定事实仍推进永久版本")
	}
	standing := proto.Clone(c.player()).(*pb.EntityState)
	townScene, townBinding := c.entry.SceneEpoch, c.binding
	c.close()
	// 超过原怪物10秒重生截止；Town断线没有离线扣时、刷怪或收益。
	time.Sleep(11 * time.Second)
	offline, err := inspection.Role(ctx, roleID)
	must(t, err)
	travelAssertEconomy(t, boundRole, offline, "Town离线")
	c = join(t, cfg.GameAddress, issue(t, accountURL, login.AccountToken, role.Key))
	if c.entry.RoomId != 2 || c.entry.SceneKind != 2 || c.entry.SceneEpoch != townScene || c.binding <= townBinding || c.player().Life != standing.Life || c.player().Hp != standing.Hp || c.player().Mp != standing.Mp || !proto.Equal(c.player().Position, standing.Position) || travelCount(c.scene, 3) != 0 {
		t.Fatalf("新ticket健康重入重建了Town现场: before=%v entry=%v player=%v", standing, c.entry, c.player())
	}
	travelAssertBindingSnapshot(t, c, committed)
	travelToBaseline(c, 2, 1)
	returnedMonster, returnedDrop := travelEntity(c.scene, 3), travelEntity(c.scene, 4)
	if c.entry.SceneKind != 1 || !proto.Equal(c.player().Position, personalPlayer.Position) || c.player().Life != personalPlayer.Life {
		t.Fatal("返回未恢复原个人位置/生命")
	}
	if returnedMonster == nil || returnedMonster.EntityHandle != deadMonster.EntityHandle || returnedMonster.Life != deadMonster.Life || returnedMonster.Hp != 0 || !proto.Equal(returnedMonster.Position, deadMonster.Position) {
		t.Fatalf("ACK前原死怪现场被初始化或提前模拟: want=%v got=%v", deadMonster, returnedMonster)
	}
	if returnedDrop == nil || returnedDrop.EntityHandle != drop.EntityHandle || returnedDrop.DefinitionId != drop.DefinitionId || !proto.Equal(returnedDrop.Position, drop.Position) {
		t.Fatalf("休眠个人掉落未原样恢复: want=%v got=%v", drop, returnedDrop)
	}
	travelAcknowledge(c, false)
	// 原截止已过，正常tick应立刻处理一次；返图时重置重生/掉落期限会超出此窗口。
	travelUntilWithin(c, 2*time.Second, func(f wire.Frame) bool {
		m := travelEntity(c.scene, 3)
		return f.ID == p.MsgS2CSceneBaseline && m != nil && m.Life == deadMonster.Life+1 && m.Hp > 0 && travelCount(c.scene, 4) == 0
	})
	finalRole, err := inspection.Role(ctx, roleID)
	must(t, err)
	travelAssertEconomy(t, boundRole, finalRole, "健康往返")
	travelAssertBindingSnapshot(t, c, committed)
	var document bson.M
	must(t, inspection.DB.Collection("roles").FindOne(ctx, bson.M{"_id": roleID}).Decode(&document))
	for _, field := range []string{"hp", "mp", "position", "monsters", "cooldowns", "pet", "ground_items", "saved_personal", "town_navigation"} {
		if _, present := document[field]; present {
			t.Fatalf("临时现场%s被持久化", field)
		}
	}
	// 明确停止并等待旧执行退出后重建；只恢复永久绑定，不把旧Town脚点持久化。
	c.close()
	stop()
	stop = launch()
	c = join(t, cfg.GameAddress, issue(t, accountURL, login.AccountToken, role.Key))
	if c.entry.RoomId != cfg.Fixture.Map.ID || c.entry.SceneKind != devconfig.SceneKindPersonal || c.player().Position.X != int32(cfg.Fixture.Map.PlayerSpawn.X) || c.player().Position.Y != int32(cfg.Fixture.Map.PlayerSpawn.Y) {
		t.Fatal("冷重建未按个人初始世界创建，或把永久绑定当作瞬移入口", c.entry, c.player())
	}
	travelAssertBindingSnapshot(t, c, committed)
	t.Logf("通过: 随机库%s，真实击杀/掉落、Town700入口、Plan/Commit/ACK、旧scene/旧ACK门禁、有界导航、Walk一次可信绑定/同中心幂等/区外保持、11秒断线零收益零扣时、原生命健康重入、原怪物/掉落截止恢复、完整停止后冷建仍保留永久绑定且21关闭", cfg.Database)
}

func travelAssertBindingSnapshot(t *testing.T, c *client, committed *pb.BindingState) {
	t.Helper()
	state := c.roleBinding
	if state == nil || !state.Trusted || state.Cause != 1 || state.SceneEpoch != c.entry.SceneEpoch || state.SourceSeq != committed.SourceSeq || state.SafeZoneId != committed.SafeZoneId || !proto.Equal(state.Role, committed.Role) || !proto.Equal(state.Binding, committed.Binding) || len(c.bindingCommits) != 0 {
		t.Fatal("健康/冷入场或旅行未静默恢复原绑定证明", state, committed)
	}
}

func travelEntity(scene *pb.SceneBaselineChunk, kind uint32) *pb.EntityState {
	for _, e := range scene.GetEntities() {
		if e.EntityKind == kind {
			return e
		}
	}
	return nil
}
func travelCount(scene *pb.SceneBaselineChunk, kind uint32) int {
	count := 0
	for _, e := range scene.GetEntities() {
		if e.EntityKind == kind {
			count++
		}
	}
	return count
}

func travelUntilWithin(c *client, timeout time.Duration, predicate func(wire.Frame) bool) wire.Frame {
	c.t.Helper()
	previous := c.ctx
	ctx, cancel := context.WithTimeout(previous, timeout)
	c.ctx = ctx
	defer func() { c.ctx = previous; cancel() }()
	return c.until(predicate)
}

func travelExpectError(c *client, id uint16, request uint32, code uint32) {
	c.t.Helper()
	for {
		f := c.readFrame()
		if f.Request == request && f.ID == id {
			if f.Error != code {
				c.t.Fatalf("请求%d/%d错误=%d，期望%d", id, request, f.Error, code)
			}
			return
		}
		if f.Error != 0 {
			c.t.Fatalf("非预期WS拒绝: %v", f)
		}
	}
}

func travelToBaseline(c *client, destination uint32, mapID uint64) {
	c.t.Helper()
	from := proto.Clone(c.entry).(*pb.EnterLineResult)
	id := domain.ID()
	request := c.send(p.MsgC2SMapTravelPrepare, &pb.MapTravelPrepare{SceneEpoch: from.SceneEpoch, TravelId: id, Destination: destination})
	f := travelUntilWithin(c, 3*time.Second, func(f wire.Frame) bool { return f.ID == p.MsgS2CMapTravelPlan && f.Request == request })
	plan := &pb.MapTravelPlan{}
	must(c.t, proto.Unmarshal(f.Payload, plan))
	if plan.TravelId != id || plan.FromSceneEpoch != from.SceneEpoch || plan.TargetMapId != mapID || plan.ExpiresAtUnixMs <= time.Now().UnixMilli() {
		c.t.Fatalf("实际Plan身份/目录错误: %v", plan)
	}
	c.send(p.MsgC2SMapTravelCommit, &pb.MapTravelCommit{SceneEpoch: from.SceneEpoch, TravelId: id})
	quiescing, committed := false, false
	travelUntilWithin(c, 12*time.Second, func(f wire.Frame) bool {
		if f.ID == p.MsgS2CMapTravelState {
			state := &pb.MapTravelState{}
			must(c.t, proto.Unmarshal(f.Payload, state))
			if state.TravelId == id {
				if state.Phase == 1 {
					quiescing = true
				}
				if state.Phase == 2 {
					committed = true
				}
				if state.Phase >= 3 {
					c.t.Fatalf("实际旅行未完成: %v", state)
				}
			}
		}
		return f.ID == p.MsgS2CEnterLine && c.entry.RoomId == mapID
	})
	if !quiescing || !committed || c.entry.SceneEpoch != from.SceneEpoch+1 || c.entry.BaselineId <= from.BaselineId {
		c.t.Fatal("缺少真实收束/提交状态或scene/baseline未推进")
	}
	travelUntilWithin(c, time.Second, func(f wire.Frame) bool {
		return f.ID == p.MsgS2CSceneBaseline && c.scene.SceneEpoch == c.entry.SceneEpoch && c.scene.BaselineId == c.entry.BaselineId
	})
	if c.scene.ChunkCount != 1 || c.player() == nil {
		c.t.Fatal("未收到目标完整基线")
	}
	// 旧scene/旧baseline ACK不能激活当前图，经济仍须等待真正目标ACK。
	c.send(p.MsgC2SEnterWorldAck, &pb.EnterWorldAck{SceneEpoch: from.SceneEpoch, BaselineId: from.BaselineId})
	request = c.send(p.MsgC2SClaimHostingTime, &pb.ClaimHostingTimeRequest{Operation: c.operation()})
	for {
		f = c.readFrame()
		if f.Error != 0 {
			c.t.Fatalf("ACK前经济未返回标准资产结果: %v", f)
		}
		if f.ID == p.MsgS2CSyncReady || f.ID == p.MsgS2CInventory {
			c.t.Fatal("旧ACK激活目标或ACK前发送资产")
		}
		if f.ID == p.MsgS2COperationResult && f.Request == request {
			result := &pb.OperationResult{}
			must(c.t, proto.Unmarshal(f.Payload, result))
			if result.ErrorCode != p.CodeLineRecovering || result.CommandSeq != c.watermark {
				c.t.Fatalf("ACK前经济越过门禁/推进水位: %v", result)
			}
			break
		}
	}
}

func travelAcknowledge(c *client, assertTown bool) {
	c.t.Helper()
	c.send(p.MsgC2SEnterWorldAck, &pb.EnterWorldAck{SceneEpoch: c.entry.SceneEpoch, BaselineId: c.entry.BaselineId})
	f := travelUntilWithin(c, 3*time.Second, func(f wire.Frame) bool { return f.ID == p.MsgS2CSyncReady })
	ready := &pb.SyncReady{}
	must(c.t, proto.Unmarshal(f.Payload, ready))
	if ready.SceneEpoch != c.entry.SceneEpoch || ready.BaselineId != c.entry.BaselineId || ready.NextEconomicCommandSeq != c.entry.NextEconomicCommandSeq {
		c.t.Fatalf("SyncReady未确认真实目标/水位: %v", ready)
	}
	c.watermark = ready.NextEconomicCommandSeq - 1
	if assertTown && (c.hosting == nil || c.hosting.AutoBattleEnabled || travelCount(c.scene, 3) != 0) {
		c.t.Fatalf("Town托管显示伪造怪物/自动战斗: %v", c.hosting)
	}
}

func travelSafeZone(t *testing.T, scene *pb.SceneBaselineChunk) uint64 {
	t.Helper()
	field := scene.ProtoReflect().Descriptor().Fields().ByName("safe_zone_id")
	if field == nil {
		t.Fatal("公共协议缺少实际安全区观察字段")
	}
	return scene.ProtoReflect().Get(field).Uint()
}

func travelNearbyGround(t *testing.T, m *devconfig.MapTemplate, scene *pb.SceneBaselineChunk, origin *pb.GridPoint) *pb.GridPoint {
	t.Helper()
	for _, direction := range []devconfig.Point{{X: 1}, {Y: 1}, {X: -1}, {Y: -1}} {
		valid := true
		for step := 1; step <= 3; step++ {
			point := devconfig.Point{X: int(origin.X) + direction.X*step, Y: int(origin.Y) + direction.Y*step}
			if !m.Walkable(point) {
				valid = false
				break
			}
			for _, e := range scene.Entities {
				if e.EntityKind == 2 && e.Hp > 0 && e.Position.X == int32(point.X) && e.Position.Y == int32(point.Y) {
					valid = false
					break
				}
			}
		}
		if valid {
			return &pb.GridPoint{X: origin.X + int32(direction.X*3), Y: origin.Y + int32(direction.Y*3)}
		}
	}
	t.Fatal("入口附近没有可证明静态可走的三格真实导航目标")
	return nil
}

func travelAssertEconomy(t *testing.T, before, after domain.Role, phase string) {
	t.Helper()
	if before.EconomicSeq != after.EconomicSeq || before.SourceSeq != after.SourceSeq || before.Wallet != after.Wallet || before.HostingSeconds != after.HostingSeconds || before.Experience != after.Experience || before.Level != after.Level || !reflect.DeepEqual(before.Items, after.Items) || !reflect.DeepEqual(before.Report, after.Report) {
		t.Fatalf("%s产生旅行收费、离线托管收益或额外经济提交: before(seq=%d/%d host=%d wallet=%+v) after(seq=%d/%d host=%d wallet=%+v)", phase, before.EconomicSeq, before.SourceSeq, before.HostingSeconds, before.Wallet, after.EconomicSeq, after.SourceSeq, after.HostingSeconds, after.Wallet)
	}
}
