// 本文件用显式注入的开发首领模板验收实际格子战斗和角色 owner 的持久预约、终态。
package integration

import (
	"bytes"
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
	cfg, err := loadIntegrationConfig()
	must(t, err)
	cfg.Database = "minimir_dev_game_test_pb_" + domain.ID()
	cfg.AccountAddress, cfg.GameAddress = address(t), address(t)
	ordinary := &cfg.Fixture.Monsters[0]
	ordinary.HP, ordinary.MoveMS, ordinary.Agility, ordinary.DefenceMin, ordinary.DefenceMax = 1, 250, 0, 0, 0
	ordinary.AttackMin, ordinary.AttackMax, ordinary.AttackMS = 0, 0, 4000
	monster := *ordinary
	// 让可区分的首领怪物编号和独立地图编号同时出现在禁止普通奖励的断言中。
	monster.ID = 80_991
	monster.AttackMin, monster.AttackMax, monster.AttackMS, monster.Exp = 0, 0, 4000, 0
	monster.GoldMin, monster.GoldMax, monster.RespawnMS = 0, 0, 0
	cfg.Fixture.PersonalBoss = &devconfig.PersonalBossConfig{TemplateID: 1, DisplayName: "隔离首领", MinimumLevel: 1,
		Map: devconfig.MapTemplate{ID: 80, Width: 13, Height: 13, SceneKind: devconfig.SceneKindPersonalBoss,
			PlayerSpawn: devconfig.Point{X: 5, Y: 5}, MonsterSpawns: []devconfig.Point{{X: 6, Y: 5}}},
		Monster: monster, Duration: 30 * time.Second, SuccessCooldown: time.Minute, RetryCooldown: time.Second,
		DailySuccessLimit: 5, PrivilegeSuccessBonus: 1, HistoryLimit: 20}
	if !cfg.Fixture.ValidPersonalBoss() {
		t.Fatal("隔离首领模板未通过配置校验")
	}
	// 仅为本轮隔离验收开放通关目标和图鉴，首领击杀没有普通挂机奖励。
	bossTemplate := cfg.Fixture.PersonalBoss
	cfg.Fixture.ChallengeMonsters = append(cfg.Fixture.ChallengeMonsters, monster)
	cfg.Fixture.TasksCodex = &devconfig.TaskCodexRules{DailyPoolSize: 1, DailyClaimLimit: 1, DailyActivityTarget: 100,
		Definitions: []devconfig.TaskCodexDefinition{
			{ID: 1, Kind: devconfig.TaskCodexDaily, Name: "今日挑战", Event: devconfig.TaskCodexPersonalBossClear, TemplateID: bossTemplate.TemplateID, Target: 1, Activity: 100},
			{ID: 1, Kind: devconfig.TaskCodexAchievement, Name: "个人首领二胜", Event: devconfig.TaskCodexPersonalBossClear, Target: 2},
			{ID: 1, Kind: devconfig.TaskCodexMonster, Name: "隔离首领图鉴", Event: devconfig.TaskCodexMonsterKill, MonsterID: monster.ID, MapID: bossTemplate.Map.ID, Target: 1,
				Reward: devconfig.TaskCodexReward{BoundYuanbaoUnits: 11}},
			{ID: 1, Kind: devconfig.TaskCodexActivity, Name: "今日活跃", Target: 100},
		}}
	must(t, cfg.Fixture.ValidateTasksCodex())
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
	queryTasks := func(conn *client) *pb.TaskCodexStateResult {
		t.Helper()
		request := conn.send(p.MsgC2STaskCodexState, &pb.TaskCodexStateRequest{})
		frame := travelUntilWithin(conn, 3*time.Second, func(frame wire.Frame) bool { return frame.Request == request })
		state := &pb.TaskCodexStateResult{}
		if frame.ID != p.MsgS2CTaskCodexState || frame.Error != 0 {
			t.Fatalf("挑战任务查询未返回同一请求的状态：%+v", frame)
		}
		must(t, proto.Unmarshal(frame.Payload, state))
		if state.ErrorCode != 0 || len(state.Rows) != len(cfg.Fixture.TasksCodex.Definitions) {
			t.Fatalf("隔离的挑战任务、成就和图鉴未开放：%v", state)
		}
		return state
	}
	for _, row := range queryTasks(c).Rows {
		if row.Progress != 0 || row.Claimable || row.Claimed {
			t.Fatalf("未进场的角色提前收到挑战进度：%v", row)
		}
	}
	queryBoss := func() *pb.PersonalBossInfo {
		t.Helper()
		request := c.send(p.MsgC2SPersonalBossQuery, &pb.PersonalBossQuery{})
		info := &pb.PersonalBossInfo{}
		travelUntilWithin(c, 3*time.Second, func(frame wire.Frame) bool {
			if frame.Request != request {
				return false
			}
			if frame.ID != p.MsgS2CPersonalBossInfo || frame.Error != 0 {
				t.Fatalf("首领目录没有返回同一原请求：%v", frame)
			}
			must(t, proto.Unmarshal(frame.Payload, info))
			return true
		})
		return info
	}
	initialInfo := queryBoss()
	if !initialInfo.Open || !initialInfo.CanEnter || initialInfo.ErrorCode != 0 || initialInfo.TemplateId != 1 || initialInfo.MonsterId != monster.ID ||
		initialInfo.MapId != cfg.Fixture.PersonalBoss.Map.ID || initialInfo.DailyLimit != 5 || len(initialInfo.History) != 0 {
		t.Fatalf("新角色没有拿到服务器权威首领资格：%v", initialInfo)
	}
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
	travelAcknowledge(c, false, false)
	requireChallengeAuto(c, false)
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
	finishedInfo := queryBoss()
	if finishedInfo.ErrorCode != 0 || finishedInfo.CanEnter || finishedInfo.EligibilityReason != persistence.PersonalBossCoolingDown ||
		finishedInfo.Successes != 1 || finishedInfo.ActiveChallengeId != "" || finishedInfo.AvailableAtUnixMs != terminal.AvailableAt.UnixMilli() ||
		len(finishedInfo.History) != 1 || finishedInfo.History[0].ChallengeId != booking.ChallengeId || finishedInfo.History[0].RewardGold != 0 {
		t.Fatalf("查询没有反映真实挑战终态，或默认候选发放了金币：%v", finishedInfo)
	}
	count, err := audit.CountDocuments(ctx, bson.M{"kind": "settlement", "role_id": roleID, "$or": bson.A{
		bson.M{"settlement.map_id": cfg.Fixture.PersonalBoss.Map.ID}, bson.M{"settlement.monster_id": monster.ID},
	}})
	must(t, err)
	if count != 0 {
		t.Fatal("首领地图或模板写入了普通挂机收益", count)
	}
	c.auto(false, 1)
	confirmed := readRole()
	state := queryTasks(c)
	durable := persistence.TaskCodexState(confirmed, &cfg.Fixture, terminal.FinishedAt)
	if confirmed.TasksCodex == nil || confirmed.TasksCodex.Daily.Day != terminal.SuccessDay ||
		durable.Rows[0].Kind != devconfig.TaskCodexDaily || durable.Rows[0].Progress != 1 {
		t.Fatalf("胜利未按原击杀运营日持久计入每日任务：%+v", confirmed.TasksCodex)
	}
	for _, row := range state.Rows {
		switch row.Kind {
		case devconfig.TaskCodexDaily:
			if state.Day == terminal.SuccessDay && (row.Progress != 1 || !row.Claimable) {
				t.Fatal("实际首领击杀未计入当日任务", row)
			}
		case devconfig.TaskCodexAchievement:
			if row.Progress != 1 || row.Claimable {
				t.Fatal("通关成就没记录一次真实胜利", row)
			}
		case devconfig.TaskCodexMonster:
			if row.Progress != 1 || !row.Claimable || row.MapId != bossTemplate.Map.ID || row.MonsterId != monster.ID {
				t.Fatal("图鉴没有收到原首领地图和唯一击杀", row)
			}
		case devconfig.TaskCodexActivity:
			if row.Progress != 0 || row.Claimable {
				t.Fatal("首领死亡时擅自确认活跃奖励", row)
			}
		}
	}
	claim := &pb.TaskCodexClaimRequest{Operation: c.operation(), Kind: devconfig.TaskCodexMonster, Id: 1}
	claim.Operation.ExpectedRevision = state.RoleRevision
	first := c.asset(p.MsgC2STaskCodexClaim, claim)
	if first.ErrorCode != 0 || first.Pending || readRole().Wallet.BoundYuanbao != confirmed.Wallet.BoundYuanbao+11 {
		t.Fatal("首领图鉴未成功发放原绑定奖励", first)
	}
	c.close()
	resumed := join(t, cfg.GameAddress, issue(t, endpoint, login.AccountToken, role.Key))
	defer resumed.close()
	resumed.auto(false, 1)
	for _, row := range queryTasks(resumed).Rows {
		if row.Kind == devconfig.TaskCodexMonster && (row.Progress != 1 || row.Claimable || !row.Claimed) {
			t.Fatal("新票入场没有恢复已领取的个人首领图鉴", row)
		}
	}
	replayed := resumed.asset(p.MsgC2STaskCodexClaim, claim)
	if replayed.ErrorCode != 0 || replayed.Pending || !bytes.Equal(first.OpId, replayed.OpId) || replayed.CommandSeq != first.CommandSeq ||
		readRole().Wallet.BoundYuanbao != confirmed.Wallet.BoundYuanbao+11 {
		t.Fatal("跨重连重放的图鉴奖励回执不一致或重复发放", first, replayed)
	}
	t.Log("个人首领真实击杀只记一次每日、成就和图鉴；绑定领奖经过新票重连不重发")
}

// requireChallengeAuto 核对已确认挑战附着的独立自动战斗开关。
func requireChallengeAuto(c *client, expected bool) {
	if c.hosting == nil || c.hosting.AutoBattleEnabled != expected {
		c.t.Fatalf("首领开关不同步：期望%t，收到%v", expected, c.hosting)
	}
}

// setChallengeAuto 经真实协议切换挑战开关，保留固定首领数量。
func setChallengeAuto(c *client, enabled bool) {
	c.t.Helper()
	request := c.send(p.MsgC2SAutoBattle, &pb.AutoBattleIntent{SceneEpoch: c.entry.SceneEpoch, Enabled: enabled})
	travelUntilWithin(c, 3*time.Second, func(frame wire.Frame) bool {
		return frame.ID == p.MsgS2CHostingState && frame.Request == request
	})
	requireChallengeAuto(c, enabled)
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
