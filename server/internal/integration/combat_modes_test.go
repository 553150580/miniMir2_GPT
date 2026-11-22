// 验证八个技能的学习、战斗模式切换、施法和健康重连的实际消息及状态。

package integration

import (
	"context"
	"go.mongodb.org/mongo-driver/v2/bson"
	"google.golang.org/protobuf/proto"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/bootstrap"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
	"minimir/internal/wire"
	"os"
	"sync"
	"testing"
	"time"
)

// modeEntry 返回模式入场供调用方使用。
// state 记录状态，使用技能模式状态表示，id 区分当前步骤指向的目标对象。
func modeEntry(state *pb.SkillModeState, id uint32) *pb.SkillModeEntry {
	// entry 接收遍历“get”状态集合时当前的元素。
	for _, entry := range state.GetStates() {
		if entry.SkillId == id {
			return entry
		}
	}
	return nil
}

// modesUntil 返回模式集合有效截止时间供调用方使用。
// c 是调用该方法的客户端实例，predicate 记录“predicate”，使用供当前流程调用的函数表示。
func (c *client) modesUntil(predicate func(*pb.SkillModeState) bool) *pb.SkillModeState {
	c.t.Helper()
	// state 保存当前处理所用的状态。
	state := &pb.SkillModeState{}
	// frame 记录协议帧。
	c.until(func(frame wire.Frame) bool {
		if frame.ID != p.MsgS2CSkillModeState {
			return false
		}
		must(c.t, proto.Unmarshal(frame.Payload, state))
		return predicate(state)
	})
	return state
}

// attackAccepted 返回攻击已受理供调用方使用。
// c 是调用该方法的客户端实例，direction 记录朝向，使用数值表示，expected 记录预期结果，使用数值表示。
func (c *client) attackAccepted(direction, expected uint32) *pb.AttackAccepted {
	c.t.Helper()
	c.send(p.MsgC2SAttackIntent, &pb.AttackIntent{SceneEpoch: c.entry.SceneEpoch, Direction: direction, SkillId: 0})
	// accepted 保存当前处理所用的已受理。
	accepted := &pb.AttackAccepted{}
	// frame 记录协议帧。
	c.until(func(frame wire.Frame) bool {
		if frame.ID != p.MsgS2CAttackAccepted {
			return false
		}
		must(c.t, proto.Unmarshal(frame.Payload, accepted))
		return true
	})
	if accepted.SkillId != expected || accepted.CasterLife != c.player().Life || accepted.SkillLevel != 0 || accepted.DelayMs != 300 {
		c.t.Fatalf("本次攻击权威选择错误:%+v", accepted)
	}
	return accepted
}

// TestCombatModesEightSkillsLearningModesCastingAndReconnectMongoWS 检查技能学习、模式切换、真实施法与恢复连接后的技能状态。
// t 提供断言失败报告、子用例和清理注册。
func TestCombatModesEightSkillsLearningModesCastingAndReconnectMongoWS(t *testing.T) {
	if os.Getenv("MINIMIR_MONGO_E2E") != "1" {
		t.Skip("设置MINIMIR_MONGO_E2E=1验证八项技能真实购书/学习/模式/命中/重连")
	}
	// cfg 保存由加载取得的服务配置，err 接收加载过程中可能产生的错误，供后续失败分支处理。
	cfg, err := devconfig.Load("../../../conf/dev/server.json")
	must(t, err)
	cfg.Database = "minimir_dev_game_test_" + domain.ID()
	cfg.AccountAddress, cfg.GameAddress, cfg.GMAddress = address(t), address(t), address(t)
	// 隔离随机测试库使用固定战斗格，所有敌人无伤害且不移动，避免站位依赖网络时机。
	// positions 初始化用作位置集合的点位列表。
	positions := []devconfig.Point{{X: 8, Y: 7}, {X: 8, Y: 6}, {X: 7, Y: 7}, {X: 9, Y: 7}, {X: 9, Y: 8}, {X: 1, Y: 1}, {X: 2, Y: 1}, {X: 3, Y: 1}}
	cfg.Fixture.Map.PlayerSpawn = devconfig.Point{X: 8, Y: 8}
	cfg.Fixture.Map.MonsterSpawns = positions
	// valid 初始化用作有效的按点位索引的布尔状态映射。
	valid := map[devconfig.Point]bool{cfg.Fixture.Map.PlayerSpawn: true}
	// point 接收遍历位置集合时当前的元素。
	for _, point := range positions {
		valid[point] = true
	}
	cfg.Fixture.Map.Blocked = nil
	// y 规定纵坐标的初始取值。
	for y := 0; y < cfg.Fixture.Map.Height; y++ {
		// x 规定横坐标的初始取值。
		for x := 0; x < cfg.Fixture.Map.Width; x++ {
			// point 组装点位所需的数据。
			point := devconfig.Point{X: x, Y: y}
			if !valid[point] {
				cfg.Fixture.Map.Blocked = append(cfg.Fixture.Map.Blocked, point)
			}
		}
	}
	// i 标记遍历怪物集合时访问的下标。
	for i := range cfg.Fixture.Monsters {
		// m 保存当前处理所用的怪物集合中的指定项。
		m := &cfg.Fixture.Monsters[i]
		m.HP, m.AttackMin, m.AttackMax = 1000, 0, 0
		m.Agility, m.DefenceMin, m.DefenceMax, m.MagicDefenceMin, m.MagicDefenceMax = 0, 0, 0, 0, 0
		m.MoveMS, m.AttackMS = 600000, 600000
	}
	// ctx 传递从调用方派生的上下文，供取消或控制截止时间，cancel 取消对应上下文或停止计时，释放其关联资源。
	ctx, cancel := context.WithCancel(context.Background())
	// inspection 保存由打开取得的检查。
	inspection, err := persistence.Open(ctx, cfg)
	must(t, err)
	// wg 记录“wg”，使用协程汇合计数器表示。
	var wg sync.WaitGroup
	// mode 接收遍历文本列表时当前的元素。
	for _, mode := range []string{"account", "game", "gm"} {
		wg.Go(func() {
			// err 接收运行批次过程中可能产生的错误，供后续失败分支处理。
			if err := bootstrap.Run(ctx, cfg, mode); err != nil && ctx.Err() == nil {
				t.Errorf("模式测试%s:%v", mode, err)
			}
		})
	}
	t.Cleanup(func() {
		cancel()
		wg.Wait()
		// cleanup 传递从调用方派生的上下文，供取消或控制截止时间，stop 取消对应上下文或停止计时，释放其关联资源。
		cleanup, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
		must(t, inspection.DB.Drop(cleanup))
		inspection.Close(cleanup)
	})
	// addr 接收遍历文本列表时当前的元素。
	for _, addr := range []string{cfg.AccountAddress, cfg.GameAddress, cfg.GMAddress} {
		healthy(t, addr)
	}
	// endpoint 保存根据当前数据与账号地址的计算结果与当前数据计算的端点。
	endpoint := "http://" + cfg.AccountAddress + "/rpc"
	// login 保存当前处理所用的登录。
	login := &pb.LoginResult{}
	rpc(t, endpoint, "", p.MsgC2SLocalRegister, &pb.LocalLoginRequest{Account: "modes_" + domain.ID()[:10], Password: "test_123456"}, login, 0)
	// create 封装创建对应的后续处理，slot 记录槽位，使用数值表示，job 记录任务或职业编号，使用数值表示。
	create := func(slot, job uint32) *pb.RoleSummary {
		// role 保存当前处理所用的角色。
		role := &pb.RoleSummary{}
		rpc(t, endpoint, login.AccountToken, p.MsgC2SCreateRole, &pb.CreateRoleRequest{OriginServerId: 1, Slot: slot, Job: job, Name: "模式" + domain.ID()[:7], OpId: domain.Bytes(domain.ID())}, role, 0)
		// err 接收更新“one”过程中可能产生的错误，供后续失败分支处理。
		_, err := inspection.DB.Collection("roles").UpdateOne(ctx, bson.M{"_id": domain.Hex(role.Key.RoleId)}, bson.M{"$set": bson.M{"level": 40, "wallet": domain.Wallet{Gold: 1000000, BoundGold: 1000000}, "privilege_until": time.Now().Add(time.Minute)}})
		must(t, err)
		return role
	}

	// warriorRole 保存由创建取得的“warrior”角色。
	warriorRole := create(1, 0)
	// warrior 保存由加入取得的“warrior”。
	warrior := join(t, cfg.GameAddress, issue(t, endpoint, login.AccountToken, warriorRole.Key))
	defer func() { warrior.close() }()
	warrior.auto(false, 5)
	// id 接收遍历数值列表时当前的元素。
	for _, id := range []uint32{7, 12, 25, 26} {
		// previousRevision 保存由“get”修订号取得的上一步状态修订号。
		previousRevision := warrior.scene.GetSkillModeState().GetRevision()
		warrior.buyAndLearnSkill(cfg.Fixture, id)
		// s 记录技能模式状态。
		warrior.modesUntil(func(s *pb.SkillModeState) bool {
			return s.Revision > previousRevision && modeEntry(s, id) != nil
		})
	}
	// watermark 保存当前处理所用的最高已推进序号。
	watermark := warrior.watermark
	// opCount 保存由数量文档取得的操作数量。
	opCount, err := inspection.DB.Collection("operations").CountDocuments(ctx, bson.M{"role_id": domain.Hex(warriorRole.Key.RoleId)})
	must(t, err)
	// modeRequest 保存由发送取得的模式请求。
	modeRequest := warrior.send(p.MsgC2SSkillModeIntent, &pb.SkillModeIntent{SceneEpoch: warrior.entry.SceneEpoch, SkillId: 12, Enabled: true})
	// reply 保存由有效截止时间取得的“reply”，frame 记录协议帧。
	reply := warrior.until(func(frame wire.Frame) bool { return frame.ID == p.MsgS2CSkillModeState && frame.Request == modeRequest })
	// state 保存当前处理所用的状态。
	state := &pb.SkillModeState{}
	must(t, proto.Unmarshal(reply.Payload, state))
	if !modeEntry(state, 12).Enabled {
		t.Fatal("刺杀开启请求没有收到同一请求号的权威状态")
	}
	time.Sleep(1100 * time.Millisecond)
	warrior.attackAccepted(0, 12)
	// hit 保存由战斗命中影响取得的命中。
	if hit := warrior.combatImpact(12, 11); hit.Amount <= 0 || hit.Outcome != 1 {
		t.Fatal("刺杀第二格没有真实即时伤害")
	}
	warrior.send(p.MsgC2SSkillModeIntent, &pb.SkillModeIntent{SceneEpoch: warrior.entry.SceneEpoch, SkillId: 25, Enabled: true})
	// s 记录技能模式状态。
	warrior.modesUntil(func(s *pb.SkillModeState) bool { return modeEntry(s, 12).Enabled && modeEntry(s, 25).Enabled })
	time.Sleep(1100 * time.Millisecond)
	warrior.attackAccepted(0, 25)
	// hits 初始化用作命中集合的按数值索引的布尔状态映射。
	hits := map[uint32]bool{}
	for len(hits) < 4 {
		// frame 保存由读取取得的协议帧。
		frame := warrior.read()
		if frame.ID == p.MsgS2CCombatImpact {
			// hit 记录命中，使用战斗命中影响表示。
			var hit pb.CombatImpact
			must(t, proto.Unmarshal(frame.Payload, &hit))
			if hit.SkillId == 25 && hit.Amount > 0 {
				hits[hit.TargetHandle] = true
			}
		}
	}
	// target 接收遍历数值列表时当前的元素。
	for _, target := range []uint32{10, 12, 13, 14} {
		if !hits[target] {
			t.Fatal("半月真实WS未命中精确四格")
		}
	}
	// charge 保存当前处理所用的“charge”。
	charge := &pb.SkillModeIntent{SceneEpoch: warrior.entry.SceneEpoch, SkillId: 26, Enabled: true}
	warrior.send(p.MsgC2SSkillModeIntent, charge)
	// s 记录技能模式状态。
	state = warrior.modesUntil(func(s *pb.SkillModeState) bool { return modeEntry(s, 26).Ready })
	if modeEntry(state, 26).RemainingMs == 0 {
		t.Fatal("烈火没有权威剩余窗口")
	}
	time.Sleep(1100 * time.Millisecond)
	warrior.attackAccepted(0, 26)
	// hit 保存由战斗命中影响取得的命中。
	if hit := warrior.combatImpact(26, 10); hit.Amount <= 0 {
		t.Fatal("烈火未命中合法前格")
	}
	state = warrior.scene.SkillModeState
	// 同输入序号重传充能不能再次扣MP；错误帧后仍回权威状态。
	// request 保存由发送取得的请求。
	request := warrior.send(p.MsgC2SSkillModeIntent, charge)
	warrior.expectError(request, p.CodeStaleInput)
	// s 记录技能模式状态。
	state = warrior.modesUntil(func(s *pb.SkillModeState) bool {
		return !modeEntry(s, 26).Ready && modeEntry(s, 26).CooldownRemainingMs > 0
	})
	// beforeRemaining 保存冷却剩余额度毫秒，用于当前步骤中的操作前的状态剩余额度。
	beforeRemaining := modeEntry(state, 26).CooldownRemainingMs
	// scene 保存场景代次，用于当前步骤中的场景。
	scene := warrior.entry.SceneEpoch
	warrior.close()
	warrior = join(t, cfg.GameAddress, issue(t, endpoint, login.AccountToken, warriorRole.Key))
	state = warrior.scene.SkillModeState
	if warrior.entry.SceneEpoch != scene || !modeEntry(state, 12).Enabled || !modeEntry(state, 25).Enabled || modeEntry(state, 26).Ready || modeEntry(state, 26).CooldownRemainingMs == 0 || modeEntry(state, 26).CooldownRemainingMs > beforeRemaining {
		t.Fatal("健康重连重置模式/烈火CD/权威场景")
	}
	warrior.send(p.MsgC2SSkillModeIntent, &pb.SkillModeIntent{SceneEpoch: scene, SkillId: 12, Enabled: false, InputSequence: 1})
	// s 记录技能模式状态。
	warrior.modesUntil(func(s *pb.SkillModeState) bool { return !modeEntry(s, 12).Enabled && modeEntry(s, 25).Enabled })
	// afterCount 保存由数量文档取得的操作后的状态数量。
	afterCount, err := inspection.DB.Collection("operations").CountDocuments(ctx, bson.M{"role_id": domain.Hex(warriorRole.Key.RoleId)})
	must(t, err)
	if warrior.watermark != watermark || afterCount != opCount {
		t.Fatal("纯临时模式/攻击错误写入永久经济回执")
	}

	// mageRole 保存由创建取得的“mage”角色。
	mageRole := create(2, 1)
	// mage 保存由加入取得的“mage”。
	mage := join(t, cfg.GameAddress, issue(t, endpoint, login.AccountToken, mageRole.Key))
	defer mage.close()
	mage.auto(false, 5)
	// id 接收遍历数值列表时当前的元素。
	for _, id := range []uint32{9, 10, 24, 33} {
		mage.buyAndLearnSkill(cfg.Fixture, id)
	}
	// i 定位数值列表中正在处理的成员，id 接收遍历数值列表时当前的元素。
	for i, id := range []uint32{9, 10, 24, 33} {
		if i == 0 {
			time.Sleep(1100 * time.Millisecond)
		} else {
			time.Sleep(1850 * time.Millisecond)
		}
		// grid 保存位置，用于当前步骤中的网格。
		grid := mage.player().Position
		if id == 33 {
			grid = &pb.GridPoint{X: 8, Y: 7}
		}
		mage.send(p.MsgC2SCastIntent, &pb.CastIntent{SceneEpoch: mage.entry.SceneEpoch, SkillId: id, TargetGrid: grid, Direction: 0})
		// accepted 保存当前处理所用的已受理。
		accepted := &pb.CastAccepted{}
		// frame 记录协议帧。
		mage.until(func(frame wire.Frame) bool {
			if frame.ID != p.MsgS2CCastAccepted {
				return false
			}
			must(t, proto.Unmarshal(frame.Payload, accepted))
			return accepted.SkillId == id
		})
		if accepted.SkillLevel != 0 || accepted.CasterLife != mage.player().Life {
			t.Fatal("新法术受理缺原生零阶/生命代次")
		}
		if id == 9 {
			// step 保存当前处理所用的步骤。
			step := &pb.CastStep{}
			// frame 记录协议帧。
			mage.until(func(frame wire.Frame) bool {
				if frame.ID != p.MsgS2CCastStep {
					return false
				}
				must(t, proto.Unmarshal(frame.Payload, step))
				return step.SkillId == 9
			})
			if step.StepIndex != 1 || step.CastId != accepted.CastId || step.CasterLife != accepted.CasterLife || step.Grid.X != 8 || step.Grid.Y != 7 {
				t.Fatal("真实地狱火首步缺权威格与执行代次")
			}
		}
		// hit 保存由战斗命中影响取得的命中。
		if hit := mage.combatImpact(id, 10); hit.Amount <= 0 || hit.Outcome != 1 {
			t.Fatalf("技能%d没有真实伤害结果", id)
		}
	}
	// role 接收遍历角色“summary”列表时当前的元素。
	for _, role := range []*pb.RoleSummary{warriorRole, mageRole} {
		// stored 保存由角色取得的已持久保存，err 接收角色过程中可能产生的错误，供后续失败分支处理。
		stored, err := inspection.Role(ctx, domain.Hex(role.Key.RoleId))
		must(t, err)
		// want 规定期望值的初始取值。
		want := 4
		if len(stored.Skills) != want {
			t.Fatal("真实学习没有持久保存全部新增技能")
		}
	}
}
