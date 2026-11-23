// 本文件用显式注入的开发首领模板验收实际格子战斗和角色 owner 的持久预约、终态。
package integration

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"google.golang.org/protobuf/proto"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/bootstrap"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
	"minimir/internal/wire"
)

// TestPersonalBossGridMongoWS 核验原普通地图继续实际产出，首领单独战斗并只确认一次独立结果。
func TestPersonalBossGridMongoWS(t *testing.T) {
	if os.Getenv("MINIMIR_MONGO_E2E") != "1" {
		t.Skip("需要已核验的专属 MongoDB 副本集")
	}
	cfg, err := devconfig.Load("../../../conf/dev/server.json")
	must(t, err)
	cfg.Database = "minimir_dev_game_test_pb_" + domain.ID()
	cfg.AccountAddress, cfg.GameAddress = address(t), address(t)
	ordinary := &cfg.Fixture.Monsters[0]
	ordinary.HP, ordinary.MoveMS, ordinary.Agility, ordinary.DefenceMin, ordinary.DefenceMax = 1, 250, 0, 0, 0
	ordinary.AttackMin, ordinary.AttackMax, ordinary.AttackMS = 0, 0, 4000
	monster := *ordinary
	// 让可区分的首领怪物编号和独立地图编号同时出现在禁止普通奖励的断言中。
	monster.ID = 80_001
	monster.AttackMin, monster.AttackMax, monster.AttackMS, monster.Exp = 0, 0, 4000, 0
	monster.GoldMin, monster.GoldMax = 0, 0
	cfg.Fixture.PersonalBoss = &devconfig.PersonalBossConfig{TemplateID: 1,
		Map: devconfig.MapTemplate{ID: 80, Width: 13, Height: 13, SceneKind: devconfig.SceneKindPersonalBoss,
			PlayerSpawn: devconfig.Point{X: 5, Y: 5}, MonsterSpawns: []devconfig.Point{{X: 6, Y: 5}}},
		Monster: monster, Duration: 30 * time.Second, SuccessCooldown: time.Minute, RetryCooldown: time.Second}
	if !cfg.Fixture.ValidPersonalBoss() {
		t.Fatal("隔离首领模板未通过配置校验")
	}
	ctx, cancel := context.WithCancel(context.Background())
	store, err := persistence.Open(ctx, cfg)
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
		defer store.Close(cleanup)
		must(t, store.DB.Drop(cleanup))
	})
	for _, mode := range []string{"account", "game"} {
		nodes.Go(func() {
			if err := bootstrap.Run(ctx, cfg, mode); err != nil && ctx.Err() == nil {
				t.Errorf("隔离 %s 节点：%v", mode, err)
			}
		})
	}
	healthy(t, cfg.AccountAddress)
	healthy(t, cfg.GameAddress)
	endpoint := "http://" + cfg.AccountAddress + "/rpc"
	login, role := &pb.LoginResult{}, &pb.RoleSummary{}
	rpc(t, endpoint, "", p.MsgC2SLocalRegister, &pb.LocalLoginRequest{Account: "boss_" + domain.ID()[:8], Password: "test_123456"}, login, 0)
	rpc(t, endpoint, login.AccountToken, p.MsgC2SCreateRole, &pb.CreateRoleRequest{
		OriginServerId: 1, Slot: 1, Job: 0, Name: "首领" + domain.ID()[:6], OpId: domain.Bytes(domain.ID()),
	}, role, 0)
	roleID := domain.Hex(role.Key.RoleId)
	c := join(t, cfg.GameAddress, issue(t, endpoint, login.AccountToken, role.Key))
	defer c.close()
	c.auto(false, 1)
	var weapon *pb.ItemView
	for _, item := range c.inventory.Items {
		if item.ItemId == 218 {
			weapon = item
			break
		}
	}
	if weapon == nil {
		t.Fatal("原建角流程没有提供新手木剑")
	}
	equipped := c.asset(p.MsgC2SEquip, &pb.EquipRequest{ItemInstanceId: weapon.InstanceId, ItemVersion: weapon.Version, Slot: 0, Operation: c.operation()})
	if equipped.ErrorCode != 0 {
		t.Fatal("正常穿戴新手木剑失败", equipped)
	}
	c.inventoryAt(equipped.NewRevision)
	c.auto(true, 1)
	readRole := func() domain.Role {
		t.Helper()
		out, readErr := store.Role(ctx, roleID)
		must(t, readErr)
		return out
	}
	ordinaryScene := c.entry.SceneEpoch
	var booking *pb.PersonalBossState
	startUntil := time.Now().Add(5 * time.Second)
	for booking == nil && time.Now().Before(startUntil) {
		request := c.send(p.MsgC2SPersonalBossEnter, &pb.PersonalBossEnter{SceneEpoch: ordinaryScene, ChallengeId: domain.ID(), TemplateId: 1})
		for {
			frame := c.readFrame()
			if frame.Request != request {
				continue
			}
			if frame.ID == p.MsgS2CPersonalBossState && frame.Error == 0 {
				booking = &pb.PersonalBossState{}
				must(t, proto.Unmarshal(frame.Payload, booking))
			} else if frame.ID != p.MsgC2SPersonalBossEnter || frame.Error != p.CodeLineRecovering {
				t.Fatalf("普通图发起首领挑战失败：%v", frame)
			}
			break
		}
		if booking == nil {
			time.Sleep(100 * time.Millisecond)
		}
	}
	if booking == nil || booking.State != 1 || booking.ChallengeId == "" {
		t.Fatal("未收到角色 owner 分配的预约身份", booking)
	}
	travelUntilWithin(c, 5*time.Second, func(frame wire.Frame) bool {
		return frame.ID == p.MsgS2CSceneBaseline && c.entry.SceneKind == devconfig.SceneKindPersonalBoss && c.scene.SceneEpoch == c.entry.SceneEpoch
	})
	boss := travelEntity(c.scene, 3)
	if c.entry.RoomId != cfg.Fixture.PersonalBoss.Map.ID || c.entry.ServerTransitionId != booking.ChallengeId ||
		c.entry.SceneEpoch != ordinaryScene+1 || boss == nil || boss.Hp != 1 || travelCount(c.scene, 3) != 1 {
		t.Fatal("客户端未接到独立首领、真实预约或独立地图", c.entry, c.scene)
	}
	approved := readRole()
	if approved.PersonalBoss == nil || approved.PersonalBoss.ActiveChallengeID != booking.ChallengeId {
		t.Fatal("首领场景在真实持久预约之前开放")
	}
	c.send(p.MsgC2SEnterWorldAck, &pb.EnterWorldAck{SceneEpoch: ordinaryScene, BaselineId: c.entry.BaselineId})
	tooEarly := c.send(p.MsgC2SCastIntent, &pb.CastIntent{SceneEpoch: c.entry.SceneEpoch, TargetHandle: boss.EntityHandle})
	travelExpectError(c, p.MsgC2SCastIntent, tooEarly, p.CodeLineRecovering)
	travelAcknowledge(c, false)
	// 核验原角落的普通怪在新场景已确认后继续真实格子战斗。耐久、用药等
	// 其他合法来源也推进 SourceSeq，本断言必须读取明确属于原地图的击杀回执。
	activeAt := time.Now().UTC().Truncate(time.Millisecond)
	audit := store.DB.Collection("audit_outbox")
	ordinaryKill := bson.M{"kind": "settlement", "role_id": roleID, "event_at": bson.M{"$gte": activeAt},
		"kills": bson.M{"$gt": 0}, "settlement.map_id": cfg.Fixture.Map.ID, "settlement.monster_id": ordinary.ID}
	waitForOrdinaryBossTestKill(t, ctx, audit, ordinaryKill)
	if background := readRole(); background.SourceSeq <= approved.SourceSeq || background.PersonalBoss == nil || background.PersonalBoss.ActiveChallengeID != booking.ChallengeId {
		t.Fatal("原个人图的击杀未在首领挑战期间持久确认", approved.SourceSeq, background.SourceSeq)
	}
	// 角色出手使用真实格距和普通攻击入口。终态前没有任何手工降低首领生命的动作。
	c.send(p.MsgC2SCastIntent, &pb.CastIntent{SceneEpoch: c.entry.SceneEpoch, TargetHandle: boss.EntityHandle})
	var outcome pb.PersonalBossState
	travelUntilWithin(c, 5*time.Second, func(frame wire.Frame) bool {
		if frame.ID != p.MsgS2CPersonalBossState {
			return false
		}
		must(t, proto.Unmarshal(frame.Payload, &outcome))
		return outcome.State >= 4
	})
	if outcome.State != 4 || outcome.ChallengeId != booking.ChallengeId || outcome.ErrorCode != 0 {
		t.Fatal("真实攻击没有确认首领击杀终态", &outcome)
	}
	travelUntilWithin(c, 3*time.Second, func(frame wire.Frame) bool {
		return frame.ID == p.MsgS2CSceneBaseline && c.entry.SceneKind == devconfig.SceneKindPersonal && c.scene.SceneEpoch == c.entry.SceneEpoch
	})
	travelAcknowledge(c, false)
	var terminal persistence.PersonalBossChallenge
	must(t, store.DB.Collection("personal_boss_challenges").FindOne(ctx, bson.M{"_id": booking.ChallengeId}).Decode(&terminal))
	if terminal.Outcome != persistence.PersonalBossDefeated || terminal.Damage != 1 || terminal.HPRemaining != 0 || terminal.Successes != 1 ||
		c.entry.SceneEpoch != ordinaryScene+2 || c.entry.RoomId != cfg.Fixture.Map.ID {
		t.Fatal("首领场次终局、计数或返回原个人场景不正确", terminal, c.entry)
	}
	if final := readRole(); final.PersonalBoss == nil || final.PersonalBoss.ActiveChallengeID != "" {
		t.Fatal("首领击杀后未退役独享预约")
	}
	count, err := audit.CountDocuments(ctx, bson.M{"kind": "settlement", "role_id": roleID, "$or": bson.A{
		bson.M{"settlement.map_id": cfg.Fixture.PersonalBoss.Map.ID}, bson.M{"settlement.monster_id": monster.ID},
	}})
	must(t, err)
	if count != 0 {
		t.Fatal("首领地图或模板写入了普通挂机收益", count)
	}
}

// waitForOrdinaryBossTestKill 等待隔离数据库中的真实普通击杀回执。
func waitForOrdinaryBossTestKill(t *testing.T, ctx context.Context, audit *mongo.Collection, filter bson.M) {
	t.Helper()
	deadline, cancel := context.WithTimeout(ctx, 18*time.Second)
	defer cancel()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		err := audit.FindOne(deadline, filter).Err()
		if err == nil {
			return
		}
		if !errors.Is(err, mongo.ErrNoDocuments) && deadline.Err() == nil {
			t.Fatal("查询普通击杀永久回执失败", err)
		}
		select {
		case <-deadline.Done():
			t.Fatal("首领活跃期间未收到原个人地图的真实普通击杀回执")
		case <-ticker.C:
		}
	}
}
