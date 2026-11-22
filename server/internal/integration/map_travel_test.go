// 通过真实 WebSocket 检查地图切换、到达确认、场景代次和旧场景消息隔离。

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
// TestMapTravelMongoWS 检查真实场景切换的请求、执行和到达确认，并比对新旧场景身份。
// t 提供断言失败报告、子用例和清理注册。
func TestMapTravelMongoWS(t *testing.T) {
	if os.Getenv("MINIMIR_MONGO_E2E") != "1" {
		t.Skip("设置MINIMIR_MONGO_E2E=1连接已核验所有权的本任务专属副本集")
	}
	// cfg 保存由“historical”来源城镇配置取得的服务配置。
	cfg := historicalSourceTownConfig(t)
	if cfg.Fixture.TownTravel == nil || !cfg.Fixture.TownTravel.Enabled || cfg.Fixture.TownNavigation == nil || !cfg.Fixture.TownNavigation.Enabled {
		t.Fatal("真实城镇业务测试要求唯一Excel已显式启用城镇旅行/有界导航")
	}
	cfg.Database = "minimir_dev_game_test_town_" + domain.ID()
	cfg.AccountAddress, cfg.GameAddress, cfg.GMAddress = address(t), address(t), address(t)
	// ctx 保存由“background”取得的上下文。
	ctx := context.Background()
	// inspection 保存由打开取得的检查，err 接收打开过程中可能产生的错误，供后续失败分支处理。
	inspection, err := persistence.Open(ctx, cfg)
	must(t, err)
	// launch 封装启动对应的后续处理。
	launch := func() func() {
		// runCtx 传递从调用方派生的上下文，供取消或控制截止时间，cancel 取消对应上下文或停止计时，释放其关联资源。
		runCtx, cancel := context.WithCancel(context.Background())
		// group 记录分组，使用协程汇合计数器表示。
		var group sync.WaitGroup
		// mode 接收遍历文本列表时当前的元素。
		for _, mode := range []string{"account", "game", "gm"} {
			group.Go(func() {
				// runErr 保存由运行批次取得的运行批次错误。
				if runErr := bootstrap.Run(runCtx, cfg, mode); runErr != nil && runCtx.Err() == nil {
					t.Errorf("%s节点: %v", mode, runErr)
				}
			})
		}
		// endpoint 接收遍历文本列表时当前的元素。
		for _, endpoint := range []string{cfg.AccountAddress, cfg.GameAddress, cfg.GMAddress} {
			healthy(t, endpoint)
		}
		return func() { cancel(); group.Wait() }
	}
	// stop 保存由启动取得的停止。
	stop := launch()
	t.Cleanup(func() {
		stop()
		// cleanup 传递从调用方派生的上下文，供取消或控制截止时间，stop 取消对应上下文或停止计时，释放其关联资源。
		cleanup, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
		must(t, inspection.DB.Drop(cleanup))
		inspection.Close(cleanup)
	})
	// accountURL 保存根据当前数据与账号地址的计算结果与当前数据计算的账号URL。
	accountURL := "http://" + cfg.AccountAddress + "/rpc"
	// login 保存当前处理所用的登录。
	login := &pb.LoginResult{}
	rpc(t, accountURL, "", p.MsgC2SLocalRegister, &pb.LocalLoginRequest{Account: "town_" + domain.ID()[:10], Password: "test_123456"}, login, 0)
	// role 保存当前处理所用的角色。
	role := &pb.RoleSummary{}
	rpc(t, accountURL, login.AccountToken, p.MsgC2SCreateRole, &pb.CreateRoleRequest{OriginServerId: 1, Slot: 1, Job: 2, Name: "城镇" + domain.ID()[:6], OpId: domain.Bytes(domain.ID())}, role, 0)
	// roleID 标识当前业务使用的角色。
	roleID := domain.Hex(role.Key.RoleId)
	// 与首循环集成相同的合法永久前态：真正击杀第20只触发确定装备掉落，不造临时现场。
	_, err = inspection.DB.Collection("roles").UpdateOne(ctx, bson.M{"_id": roleID}, bson.M{"$set": bson.M{"no_equipment_kills": 19}})
	must(t, err)
	// c 接收加入供后续步骤使用。
	c := join(t, cfg.GameAddress, issue(t, accountURL, login.AccountToken, role.Key))
	defer func() { c.close() }()
	if c.roleBinding == nil || c.roleBinding.Binding != nil || c.roleBinding.Trusted || c.roleBinding.SourceSeq != 0 || c.roleBinding.Cause != 1 {
		t.Fatal("正常新角色未收到明确尚未绑定快照，或随机初始化被隐式开启", c.roleBinding)
	}
	c.auto(false, 1)
	// claim 保存由资产取得的领取。
	claim := c.asset(p.MsgC2SClaimHostingTime, &pb.ClaimHostingTimeRequest{Operation: c.operation()})
	if claim.ErrorCode != 0 {
		t.Fatalf("正常领时失败: %v", claim)
	}
	c.inventoryAt(claim.NewRevision)
	c.auto(true, 1)
	// f 记录协议帧。
	travelUntilWithin(c, 30*time.Second, func(f wire.Frame) bool {
		return f.ID == p.MsgS2CSceneBaseline && travelEntity(c.scene, 3) != nil && travelEntity(c.scene, 3).Hp == 0 && travelEntity(c.scene, 4) != nil
	})
	c.auto(false, 1)
	// f 记录协议帧。
	travelUntilWithin(c, time.Second, func(f wire.Frame) bool {
		return f.ID == p.MsgS2CSceneBaseline && travelEntity(c.scene, 3) != nil && travelEntity(c.scene, 3).Hp == 0 && travelEntity(c.scene, 4) != nil
	})
	// personal 按目标类型读取当前接口值，得到“personal”。
	personal := proto.Clone(c.scene).(*pb.SceneBaselineChunk)
	// personalPlayer 按目标类型读取当前接口值，得到“personal”玩家。
	personalPlayer := proto.Clone(c.player()).(*pb.EntityState)
	// deadMonster 按目标类型读取当前接口值，得到死亡状态怪物。
	deadMonster := proto.Clone(travelEntity(personal, 3)).(*pb.EntityState)
	// drop 按目标类型读取当前接口值，得到掉落。
	drop := proto.Clone(travelEntity(personal, 4)).(*pb.EntityState)
	if personalPlayer.Hp <= 0 {
		t.Fatal("真实战斗死亡，不能作为健康旅行前态")
	}
	// initialScene 保存场景代次，用于当前步骤中的初始值场景。
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
	// oldMove 保存由发送取得的原状态移动。
	oldMove := c.send(p.MsgC2SMoveIntent, &pb.MoveIntent{SceneEpoch: initialScene, Direction: 2})
	travelExpectError(c, p.MsgC2SMoveIntent, oldMove, p.CodeInvalidArgument)
	// inTown 保存由角色取得的输入城镇。
	inTown, err := inspection.Role(ctx, roleID)
	must(t, err)
	if inTown.HostingSeconds != 21600 {
		t.Fatalf("在线旅行扣了托管秒数: %d", inTown.HostingSeconds)
	}
	// townMap 保存由地图“by”标识取得的城镇地图，ok 表明查询、校验或类型转换是否成功。
	townMap, ok := cfg.Fixture.MapByID(2)
	if !ok {
		t.Fatal("唯一目录缺城镇")
	}
	// target 保存由地图切换“nearby”地面取得的目标。
	target := travelNearbyGround(t, townMap, c.scene, c.player().Position)
	// zone 接收遍历安全区域集合时当前的元素。
	for _, zone := range townMap.SafeZones {
		if zone.ID == 106 {
			target = &pb.GridPoint{X: int32(zone.Center.X), Y: int32(zone.Center.Y)}
		}
	}
	if target.X != 650 || target.Y != 631 {
		t.Fatal("唯一目录未配置来源银杏安全区106中心650,631")
	}
	c.send(p.MsgC2SMoveIntent, &pb.MoveIntent{SceneEpoch: c.entry.SceneEpoch, Destination: target})
	// f 记录协议帧。
	travelUntilWithin(c, 18*time.Second, func(f wire.Frame) bool {
		return f.ID == p.MsgS2CSceneBaseline && proto.Equal(c.player().Position, target) && c.scene.NavigationState.GetStatus() == 2
	})
	if travelSafeZone(t, c.scene) != 106 {
		t.Fatal("正常逐tick走到来源安全区中心，未同步首匹配ID106")
	}
	if len(c.bindingCommits) != 1 {
		t.Fatalf("首次到访应仅产生一次绑定提交，实际%d", len(c.bindingCommits))
	}
	// committed 按目标类型读取当前接口值，得到“committed”。
	committed := proto.Clone(c.bindingCommits[0]).(*pb.BindingState)
	if !committed.Trusted || committed.Cause != 2 || committed.SafeZoneId != 106 || committed.Binding.GetMapId() != 2 || !proto.Equal(committed.Binding.GetCenter(), target) || committed.SceneEpoch != c.entry.SceneEpoch || domain.Hex(committed.Role.GetRoleId()) != roleID || committed.Role.GetOriginServerId() != 1 {
		t.Fatal("合法来源Walk没有产生正确的已提交中心/角色/scene证明", committed)
	}
	// boundRole 保存由角色取得的绑定角色。
	boundRole, err := inspection.Role(ctx, roleID)
	must(t, err)
	if boundRole.Binding == nil || *boundRole.Binding != (domain.RoleBinding{MapID: 2, X: 650, Y: 631}) || boundRole.BindingSourceSeq != boundRole.SourceSeq || boundRole.SourceSeq != inTown.SourceSeq+1 || boundRole.Revision != inTown.Revision+1 || boundRole.EconomicSeq != inTown.EconomicSeq || committed.SourceSeq != uint64(boundRole.BindingSourceSeq) || committed.RoleRevision != uint64(boundRole.Revision) {
		t.Fatal("绑定未独立原子推进一次SourceSeq/revision，或动了EconomicSeq", boundRole, committed)
	}
	// 在同安全区继续成功Walk，随后真实走出区外再返回，不重新提交同一可信中心。
	// next 接收遍历网格点位列表时当前的元素。
	for _, next := range []*pb.GridPoint{travelNearbyGround(t, townMap, c.scene, target), {X: 633, Y: 613}, target} {
		c.send(p.MsgC2SMoveIntent, &pb.MoveIntent{SceneEpoch: c.entry.SceneEpoch, Destination: next})
		// f 记录协议帧。
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
	// afterWalks 保存由角色取得的操作后的状态行走集合。
	afterWalks, err := inspection.Role(ctx, roleID)
	must(t, err)
	travelAssertEconomy(t, boundRole, afterWalks, "同中心及区外Walk")
	if afterWalks.BindingSourceSeq != boundRole.BindingSourceSeq || afterWalks.Revision != boundRole.Revision {
		t.Fatal("无新绑定事实仍推进永久版本")
	}
	// standing 按目标类型读取当前接口值，得到“standing”。
	standing := proto.Clone(c.player()).(*pb.EntityState)
	// townScene 保存场景代次，用于当前步骤中的城镇场景，townBinding 保存会话绑定，用于当前步骤中的城镇会话绑定。
	townScene, townBinding := c.entry.SceneEpoch, c.binding
	c.close()
	// 超过原怪物10秒重生截止；Town断线没有离线扣时、刷怪或收益。
	time.Sleep(11 * time.Second)
	// offline 保存由角色取得的离线。
	offline, err := inspection.Role(ctx, roleID)
	must(t, err)
	travelAssertEconomy(t, boundRole, offline, "Town离线")
	c = join(t, cfg.GameAddress, issue(t, accountURL, login.AccountToken, role.Key))
	if c.entry.RoomId != 2 || c.entry.SceneKind != 2 || c.entry.SceneEpoch != townScene || c.binding <= townBinding || c.player().Life != standing.Life || c.player().Hp != standing.Hp || c.player().Mp != standing.Mp || !proto.Equal(c.player().Position, standing.Position) || travelCount(c.scene, 3) != 0 {
		t.Fatalf("新ticket健康重入重建了Town现场: before=%v entry=%v player=%v", standing, c.entry, c.player())
	}
	travelAssertBindingSnapshot(t, c, committed)
	travelToBaseline(c, 2, 1)
	// returnedMonster 保存由地图切换实体取得的已返回值怪物，returnedDrop 保存由地图切换实体取得的已返回值掉落。
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
	// f 记录协议帧。
	travelUntilWithin(c, 2*time.Second, func(f wire.Frame) bool {
		// m 接收地图切换实体供后续步骤使用。
		m := travelEntity(c.scene, 3)
		return f.ID == p.MsgS2CSceneBaseline && m != nil && m.Life == deadMonster.Life+1 && m.Hp > 0 && travelCount(c.scene, 4) == 0
	})
	// finalRole 保存由角色取得的最终状态角色。
	finalRole, err := inspection.Role(ctx, roleID)
	must(t, err)
	travelAssertEconomy(t, boundRole, finalRole, "健康往返")
	travelAssertBindingSnapshot(t, c, committed)
	// document 记录文档，使用MongoDB BSON 文档表示。
	var document bson.M
	must(t, inspection.DB.Collection("roles").FindOne(ctx, bson.M{"_id": roleID}).Decode(&document))
	// field 接收遍历文本列表时当前的元素。
	for _, field := range []string{"hp", "mp", "position", "monsters", "cooldowns", "pet", "ground_items", "saved_personal", "town_navigation"} {
		// present 保存从文档取出的“present”。
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
	t.Logf("通过: 随机库%s，真实击杀/掉落、Town700入口、Plan/Commit/ACK、旧scene/旧ACK门禁、有界导航、Walk一次可信绑定/同中心幂等/区外保持、11秒断线零收益零扣时、原生命健康重入、原怪物/掉落截止恢复、完整停止后冷建仍保留永久绑定；本用例不施放21", cfg.Database)
}

// travelAssertBindingSnapshot 处理地图切换“assert”会话绑定快照。
// t 提供断言失败报告、子用例和清理注册，c 记录客户端，committed 记录已提交，使用会话绑定状态表示。
func travelAssertBindingSnapshot(t *testing.T, c *client, committed *pb.BindingState) {
	t.Helper()
	// state 保存角色会话绑定，用于当前步骤中的状态。
	state := c.roleBinding
	if state == nil || !state.Trusted || state.Cause != 1 || state.SceneEpoch != c.entry.SceneEpoch || state.SourceSeq != committed.SourceSeq || state.SafeZoneId != committed.SafeZoneId || !proto.Equal(state.Role, committed.Role) || !proto.Equal(state.Binding, committed.Binding) || len(c.bindingCommits) != 0 {
		t.Fatal("健康/冷入场或旅行未静默恢复原绑定证明", state, committed)
	}
}

// travelEntity 返回地图切换实体供调用方使用。
// scene 记录场景，使用场景场景基线分片表示，kind 记录类型，使用数值表示。
func travelEntity(scene *pb.SceneBaselineChunk, kind uint32) *pb.EntityState {
	// e 接收遍历“get”实体时当前的元素。
	for _, e := range scene.GetEntities() {
		if e.EntityKind == kind {
			return e
		}
	}
	return nil
}

// travelCount 返回地图切换数量供调用方使用。
// scene 记录场景，使用场景场景基线分片表示，kind 记录类型，使用数值表示。
func travelCount(scene *pb.SceneBaselineChunk, kind uint32) int {
	// count 规定数量的初始取值。
	count := 0
	// e 接收遍历“get”实体时当前的元素。
	for _, e := range scene.GetEntities() {
		if e.EntityKind == kind {
			count++
		}
	}
	return count
}

// travelUntilWithin 返回地图切换有效截止时间“within”供调用方使用。
// c 记录客户端，timeout 记录超时，使用时长表示，predicate 记录“predicate”，使用供当前流程调用的函数表示。
func travelUntilWithin(c *client, timeout time.Duration, predicate func(wire.Frame) bool) wire.Frame {
	c.t.Helper()
	// previous 保存上下文，用于当前步骤中的上一步状态。
	previous := c.ctx
	// ctx 传递从调用方派生的上下文，供取消或控制截止时间，cancel 取消对应上下文或停止计时，释放其关联资源。
	ctx, cancel := context.WithTimeout(previous, timeout)
	c.ctx = ctx
	defer func() { c.ctx = previous; cancel() }()
	return c.until(predicate)
}

// travelExpectError 处理地图切换“expect”错误。
// c 记录客户端，id 区分当前步骤指向的目标对象，request 关联客户端请求与服务端的对应响应，code 记录结果码，使用数值表示。
func travelExpectError(c *client, id uint16, request uint32, code uint32) {
	c.t.Helper()
	for {
		// f 接收读取协议帧供后续步骤使用。
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

// travelToBaseline 处理地图切换目标场景基线。
// c 记录客户端，destination 记录目的地，使用数值表示，mapID 标识当前业务使用的地图。
func travelToBaseline(c *client, destination uint32, mapID uint64) {
	c.t.Helper()
	// from 按目标类型读取当前接口值，得到来源。
	from := proto.Clone(c.entry).(*pb.EnterLineResult)
	// id 区分当前步骤指向的目标对象。
	id := domain.ID()
	// 历史测试继续验证旧客户端不带目标字段；当前目录直接提交选中的地图身份。
	// targetMapID 标识当前业务使用的目标地图。
	targetMapID := mapID
	if mapID == 2 || from.RoomId == 2 {
		targetMapID = 0
	}
	// request 保存由发送取得的请求。
	request := c.send(p.MsgC2SMapTravelPrepare, &pb.MapTravelPrepare{SceneEpoch: from.SceneEpoch, TravelId: id, Destination: destination, TargetMapId: targetMapID})
	// f 接收地图切换有效截止时间“within”供后续步骤使用，f 记录协议帧。
	f := travelUntilWithin(c, 3*time.Second, func(f wire.Frame) bool { return f.ID == p.MsgS2CMapTravelPlan && f.Request == request })
	// plan 保存当前处理所用的计划。
	plan := &pb.MapTravelPlan{}
	must(c.t, proto.Unmarshal(f.Payload, plan))
	if plan.TravelId != id || plan.FromSceneEpoch != from.SceneEpoch || plan.TargetMapId != mapID || plan.ExpiresAtUnixMs <= time.Now().UnixMilli() {
		c.t.Fatalf("实际Plan身份/目录错误: %v", plan)
	}
	c.send(p.MsgC2SMapTravelCommit, &pb.MapTravelCommit{SceneEpoch: from.SceneEpoch, TravelId: id})
	// quiescing 保存当前处理所用的“quiescing”，committed 保存当前处理所用的已提交。
	quiescing, committed := false, false
	// f 记录协议帧。
	travelUntilWithin(c, 12*time.Second, func(f wire.Frame) bool {
		if f.ID == p.MsgS2CMapTravelState {
			// state 保存当前处理所用的状态。
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
	// f 记录协议帧。
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
			// result 保存当前处理所用的结果。
			result := &pb.OperationResult{}
			must(c.t, proto.Unmarshal(f.Payload, result))
			if result.ErrorCode != p.CodeLineRecovering || result.CommandSeq != c.watermark {
				c.t.Fatalf("ACK前经济越过门禁/推进水位: %v", result)
			}
			break
		}
	}
}

// travelAcknowledge 处理地图切换确认。
// c 记录客户端，assertTown 标记是否处于“assert”城镇状态。
func travelAcknowledge(c *client, assertTown bool) {
	c.t.Helper()
	c.send(p.MsgC2SEnterWorldAck, &pb.EnterWorldAck{SceneEpoch: c.entry.SceneEpoch, BaselineId: c.entry.BaselineId})
	// f 接收地图切换有效截止时间“within”供后续步骤使用，f 记录协议帧。
	f := travelUntilWithin(c, 3*time.Second, func(f wire.Frame) bool { return f.ID == p.MsgS2CSyncReady })
	// ready 保存当前处理所用的就绪。
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

// travelSafeZone 返回地图切换安全区域供调用方使用。
// t 提供断言失败报告、子用例和清理注册，scene 记录场景，使用场景场景基线分片表示。
func travelSafeZone(t *testing.T, scene *pb.SceneBaselineChunk) uint64 {
	t.Helper()
	// field 保存由“by”名称取得的字段。
	field := scene.ProtoReflect().Descriptor().Fields().ByName("safe_zone_id")
	if field == nil {
		t.Fatal("公共协议缺少实际安全区观察字段")
	}
	return scene.ProtoReflect().Get(field).Uint()
}

// travelNearbyGround 返回地图切换“nearby”地面供调用方使用。
// t 提供断言失败报告、子用例和清理注册，m 记录地图模板，scene 记录场景，使用场景场景基线分片表示，origin 记录大区或起点，使用网格点位表示。
func travelNearbyGround(t *testing.T, m *devconfig.MapTemplate, scene *pb.SceneBaselineChunk, origin *pb.GridPoint) *pb.GridPoint {
	t.Helper()
	// direction 接收遍历点位列表时当前的元素。
	for _, direction := range []devconfig.Point{{X: 1}, {Y: 1}, {X: -1}, {Y: -1}} {
		// valid 保存当前处理所用的有效。
		valid := true
		// step 规定步骤的初始取值。
		for step := 1; step <= 3; step++ {
			// point 组装点位所需的数据。
			point := devconfig.Point{X: int(origin.X) + direction.X*step, Y: int(origin.Y) + direction.Y*step}
			if !m.Walkable(point) {
				valid = false
				break
			}
			// e 接收遍历实体时当前的元素。
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

// travelAssertEconomy 处理地图切换“assert”经济。
// t 提供断言失败报告、子用例和清理注册，before 记录操作前的状态，使用角色表示，after 记录操作后的状态，使用角色表示，phase 记录阶段，使用文本表示。
func travelAssertEconomy(t *testing.T, before, after domain.Role, phase string) {
	t.Helper()
	if before.EconomicSeq != after.EconomicSeq || before.SourceSeq != after.SourceSeq || before.Wallet != after.Wallet || before.HostingSeconds != after.HostingSeconds || before.Experience != after.Experience || before.Level != after.Level || !reflect.DeepEqual(before.Items, after.Items) || !reflect.DeepEqual(before.Report, after.Report) {
		t.Fatalf("%s产生旅行收费、离线托管收益或额外经济提交: before(seq=%d/%d host=%d wallet=%+v) after(seq=%d/%d host=%d wallet=%+v)", phase, before.EconomicSeq, before.SourceSeq, before.HostingSeconds, before.Wallet, after.EconomicSeq, after.SourceSeq, after.HostingSeconds, after.Wallet)
	}
}
