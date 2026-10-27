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

func modeEntry(state *pb.SkillModeState, id uint32) *pb.SkillModeEntry {
	for _, entry := range state.GetStates() {
		if entry.SkillId == id {
			return entry
		}
	}
	return nil
}
func (c *client) modesUntil(predicate func(*pb.SkillModeState) bool) *pb.SkillModeState {
	c.t.Helper()
	state := &pb.SkillModeState{}
	c.until(func(frame wire.Frame) bool {
		if frame.ID != p.MsgS2CSkillModeState {
			return false
		}
		must(c.t, proto.Unmarshal(frame.Payload, state))
		return predicate(state)
	})
	return state
}
func (c *client) attackAccepted(direction, expected uint32) *pb.AttackAccepted {
	c.t.Helper()
	c.send(p.MsgC2SAttackIntent, &pb.AttackIntent{SceneEpoch: c.entry.SceneEpoch, Direction: direction, SkillId: 0})
	accepted := &pb.AttackAccepted{}
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

func TestCombatModesEightSkillsLearningModesCastingAndReconnectMongoWS(t *testing.T) {
	if os.Getenv("MINIMIR_MONGO_E2E") != "1" {
		t.Skip("设置MINIMIR_MONGO_E2E=1验证八项技能真实购书/学习/模式/命中/重连")
	}
	cfg, err := devconfig.Load("../../../conf/dev/server.json")
	must(t, err)
	cfg.Database = "minimir_dev_game_test_" + domain.ID()
	cfg.AccountAddress, cfg.GameAddress, cfg.GMAddress = address(t), address(t), address(t)
	// 隔离随机测试库使用固定战斗格，所有敌人无伤害且不移动，避免站位依赖网络时机。
	positions := []devconfig.Point{{X: 8, Y: 7}, {X: 8, Y: 6}, {X: 7, Y: 7}, {X: 9, Y: 7}, {X: 9, Y: 8}, {X: 1, Y: 1}, {X: 2, Y: 1}, {X: 3, Y: 1}}
	cfg.Fixture.Map.PlayerSpawn = devconfig.Point{X: 8, Y: 8}
	cfg.Fixture.Map.MonsterSpawns = positions
	valid := map[devconfig.Point]bool{cfg.Fixture.Map.PlayerSpawn: true}
	for _, point := range positions {
		valid[point] = true
	}
	cfg.Fixture.Map.Blocked = nil
	for y := 0; y < cfg.Fixture.Map.Height; y++ {
		for x := 0; x < cfg.Fixture.Map.Width; x++ {
			point := devconfig.Point{X: x, Y: y}
			if !valid[point] {
				cfg.Fixture.Map.Blocked = append(cfg.Fixture.Map.Blocked, point)
			}
		}
	}
	for i := range cfg.Fixture.Monsters {
		m := &cfg.Fixture.Monsters[i]
		m.HP, m.AttackMin, m.AttackMax = 1000, 0, 0
		m.Agility, m.DefenceMin, m.DefenceMax, m.MagicDefenceMin, m.MagicDefenceMax = 0, 0, 0, 0, 0
		m.MoveMS, m.AttackMS = 600000, 600000
	}
	ctx, cancel := context.WithCancel(context.Background())
	inspection, err := persistence.Open(ctx, cfg)
	must(t, err)
	var wg sync.WaitGroup
	for _, mode := range []string{"account", "game", "gm"} {
		wg.Go(func() {
			if err := bootstrap.Run(ctx, cfg, mode); err != nil && ctx.Err() == nil {
				t.Errorf("模式测试%s:%v", mode, err)
			}
		})
	}
	t.Cleanup(func() {
		cancel()
		wg.Wait()
		cleanup, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
		must(t, inspection.DB.Drop(cleanup))
		inspection.Close(cleanup)
	})
	for _, addr := range []string{cfg.AccountAddress, cfg.GameAddress, cfg.GMAddress} {
		healthy(t, addr)
	}
	endpoint := "http://" + cfg.AccountAddress + "/rpc"
	login := &pb.LoginResult{}
	rpc(t, endpoint, "", p.MsgC2SLocalRegister, &pb.LocalLoginRequest{Account: "modes_" + domain.ID()[:10], Password: "test_123456"}, login, 0)
	create := func(slot, job uint32) *pb.RoleSummary {
		role := &pb.RoleSummary{}
		rpc(t, endpoint, login.AccountToken, p.MsgC2SCreateRole, &pb.CreateRoleRequest{OriginServerId: 1, Slot: slot, Job: job, Name: "模式" + domain.ID()[:7], OpId: domain.Bytes(domain.ID())}, role, 0)
		_, err := inspection.DB.Collection("roles").UpdateOne(ctx, bson.M{"_id": domain.Hex(role.Key.RoleId)}, bson.M{"$set": bson.M{"level": 40, "wallet": domain.Wallet{Gold: 1000000, BoundGold: 1000000}, "privilege_until": time.Now().Add(time.Minute)}})
		must(t, err)
		return role
	}

	warriorRole := create(1, 0)
	warrior := join(t, cfg.GameAddress, issue(t, endpoint, login.AccountToken, warriorRole.Key))
	defer func() { warrior.close() }()
	warrior.auto(false, 5)
	for _, id := range []uint32{7, 12, 25, 26} {
		warrior.buyAndLearnSkill(cfg.Fixture, id)
	}
	watermark := warrior.watermark
	opCount, err := inspection.DB.Collection("operations").CountDocuments(ctx, bson.M{"role_id": domain.Hex(warriorRole.Key.RoleId)})
	must(t, err)
	warrior.send(p.MsgC2SSkillModeIntent, &pb.SkillModeIntent{SceneEpoch: warrior.entry.SceneEpoch, SkillId: 12, Enabled: true})
	warrior.modesUntil(func(s *pb.SkillModeState) bool { return modeEntry(s, 12).Enabled })
	time.Sleep(1100 * time.Millisecond)
	warrior.attackAccepted(0, 12)
	if hit := warrior.combatImpact(12, 11); hit.Amount <= 0 || hit.Outcome != 1 {
		t.Fatal("刺杀第二格没有真实即时伤害")
	}
	warrior.send(p.MsgC2SSkillModeIntent, &pb.SkillModeIntent{SceneEpoch: warrior.entry.SceneEpoch, SkillId: 25, Enabled: true})
	warrior.modesUntil(func(s *pb.SkillModeState) bool { return modeEntry(s, 12).Enabled && modeEntry(s, 25).Enabled })
	time.Sleep(1100 * time.Millisecond)
	warrior.attackAccepted(0, 25)
	hits := map[uint32]bool{}
	for len(hits) < 4 {
		frame := warrior.read()
		if frame.ID == p.MsgS2CCombatImpact {
			var hit pb.CombatImpact
			must(t, proto.Unmarshal(frame.Payload, &hit))
			if hit.SkillId == 25 && hit.Amount > 0 {
				hits[hit.TargetHandle] = true
			}
		}
	}
	for _, target := range []uint32{10, 12, 13, 14} {
		if !hits[target] {
			t.Fatal("半月真实WS未命中精确四格")
		}
	}
	charge := &pb.SkillModeIntent{SceneEpoch: warrior.entry.SceneEpoch, SkillId: 26, Enabled: true}
	warrior.send(p.MsgC2SSkillModeIntent, charge)
	state := warrior.modesUntil(func(s *pb.SkillModeState) bool { return modeEntry(s, 26).Ready })
	if modeEntry(state, 26).RemainingMs == 0 {
		t.Fatal("烈火没有权威剩余窗口")
	}
	time.Sleep(1100 * time.Millisecond)
	warrior.attackAccepted(0, 26)
	if hit := warrior.combatImpact(26, 10); hit.Amount <= 0 {
		t.Fatal("烈火未命中合法前格")
	}
	state = warrior.scene.SkillModeState
	// 同输入序号重传充能不能再次扣MP；错误帧后仍回权威状态。
	request := warrior.send(p.MsgC2SSkillModeIntent, charge)
	warrior.expectError(request, p.CodeStaleInput)
	state = warrior.modesUntil(func(s *pb.SkillModeState) bool {
		return !modeEntry(s, 26).Ready && modeEntry(s, 26).CooldownRemainingMs > 0
	})
	beforeRemaining := modeEntry(state, 26).CooldownRemainingMs
	scene := warrior.entry.SceneEpoch
	warrior.close()
	warrior = join(t, cfg.GameAddress, issue(t, endpoint, login.AccountToken, warriorRole.Key))
	state = warrior.scene.SkillModeState
	if warrior.entry.SceneEpoch != scene || !modeEntry(state, 12).Enabled || !modeEntry(state, 25).Enabled || modeEntry(state, 26).Ready || modeEntry(state, 26).CooldownRemainingMs == 0 || modeEntry(state, 26).CooldownRemainingMs > beforeRemaining {
		t.Fatal("健康重连重置模式/烈火CD/权威场景")
	}
	warrior.send(p.MsgC2SSkillModeIntent, &pb.SkillModeIntent{SceneEpoch: scene, SkillId: 12, Enabled: false, InputSequence: 1})
	warrior.modesUntil(func(s *pb.SkillModeState) bool { return !modeEntry(s, 12).Enabled && modeEntry(s, 25).Enabled })
	afterCount, err := inspection.DB.Collection("operations").CountDocuments(ctx, bson.M{"role_id": domain.Hex(warriorRole.Key.RoleId)})
	must(t, err)
	if warrior.watermark != watermark || afterCount != opCount {
		t.Fatal("纯临时模式/攻击错误写入永久经济回执")
	}

	mageRole := create(2, 1)
	mage := join(t, cfg.GameAddress, issue(t, endpoint, login.AccountToken, mageRole.Key))
	defer mage.close()
	mage.auto(false, 5)
	for _, id := range []uint32{9, 10, 24, 33} {
		mage.buyAndLearnSkill(cfg.Fixture, id)
	}
	for i, id := range []uint32{9, 10, 24, 33} {
		if i == 0 {
			time.Sleep(1100 * time.Millisecond)
		} else {
			time.Sleep(1850 * time.Millisecond)
		}
		grid := mage.player().Position
		if id == 33 {
			grid = &pb.GridPoint{X: 8, Y: 7}
		}
		mage.send(p.MsgC2SCastIntent, &pb.CastIntent{SceneEpoch: mage.entry.SceneEpoch, SkillId: id, TargetGrid: grid, Direction: 0})
		accepted := &pb.CastAccepted{}
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
			step := &pb.CastStep{}
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
		if hit := mage.combatImpact(id, 10); hit.Amount <= 0 || hit.Outcome != 1 {
			t.Fatalf("技能%d没有真实伤害结果", id)
		}
	}
	for _, role := range []*pb.RoleSummary{warriorRole, mageRole} {
		stored, err := inspection.Role(ctx, domain.Hex(role.Key.RoleId))
		must(t, err)
		want := 4
		if len(stored.Skills) != want {
			t.Fatal("真实学习没有持久保存全部新增技能")
		}
	}
}
