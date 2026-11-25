// 本文件用独立真实 MongoDB 和 WebSocket 验证个人及共享首领中玩家战败的归属。
package integration

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"

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

// TestPersonalBossPlayerDefeatMongoWS 验证真实致死命中、一次失败回执和原挂机身体不变。
func TestPersonalBossPlayerDefeatMongoWS(t *testing.T) {
	ctx, store, endpoint, route := startChallengePlayerDeathServer(t, func(cfg *devconfig.Config) {
		boss := cfg.Fixture.PersonalBoss
		boss.Map.PlayerSpawn, boss.Map.MonsterSpawns = devconfig.Point{X: 5, Y: 5}, []devconfig.Point{{X: 6, Y: 5}}
		boss.Monster.Accuracy, boss.Monster.AttackMin, boss.Monster.AttackMax = 10_000, 500, 500
		if !cfg.Fixture.ValidPersonalBoss() {
			t.Fatal("个人首领战败的隔离模板无效")
		}
	})
	c, role, _ := joinWorldBossTestRole(t, endpoint, route)
	defer c.close()
	moveChallengePlayerDeathTest(t, c, &pb.GridPoint{X: 6, Y: 7})
	ordinaryScene, ordinaryMap := c.entry.SceneEpoch, c.entry.RoomId
	ordinaryBody := challengePlayerDeathBody(c)
	roleID := domain.Hex(role.Key.RoleId)
	before, err := store.Role(ctx, roleID)
	must(t, err)

	// 等待真实预约批准后再确认挑战基线，首领致死由正常攻击 AI 触发。
	template := store.Cfg.Fixture.PersonalBoss
	entry := &pb.PersonalBossEnter{SceneEpoch: ordinaryScene, ChallengeId: domain.ID(), TemplateId: template.TemplateID}
	request := c.send(p.MsgC2SPersonalBossEnter, entry)
	var booking pb.PersonalBossState
	travelUntilWithin(c, 5*time.Second, func(frame wire.Frame) bool {
		if frame.Request != request {
			return false
		}
		if frame.ID != p.MsgS2CPersonalBossState || frame.Error != 0 {
			t.Fatalf("个人首领预约没有返回状态：%+v", frame)
		}
		must(t, proto.Unmarshal(frame.Payload, &booking))
		return true
	})
	if booking.State != 1 || booking.ChallengeId == "" {
		t.Fatal("未获得个人首领预约", &booking)
	}
	travelUntilWithin(c, 5*time.Second, func(frame wire.Frame) bool {
		return frame.ID == p.MsgS2CSceneBaseline && c.entry.SceneKind == devconfig.SceneKindPersonalBoss && c.scene.SceneEpoch == c.entry.SceneEpoch
	})
	challengeScene, challengeBody := c.entry.SceneEpoch, challengePlayerDeathBody(c)
	monster := travelEntity(c.scene, 3)
	if c.entry.ServerTransitionId != booking.ChallengeId || monster.GetDefinitionId() != template.Monster.ID || challengeBody.GetHp() <= 0 {
		t.Fatal("玩家未进入具有活体首领的批准挑战", c.entry, c.scene)
	}
	travelAcknowledge(c, false, false)
	var outcome pb.PersonalBossState
	var damage int64
	travelUntilWithin(c, 5*time.Second, func(frame wire.Frame) bool {
		damage += challengePlayerDeathDamage(t, frame, challengeScene, monster, challengeBody)
		if frame.ID != p.MsgS2CPersonalBossState {
			return false
		}
		must(t, proto.Unmarshal(frame.Payload, &outcome))
		return outcome.State >= 4
	})
	if outcome.State != 7 || outcome.ErrorCode != 0 || outcome.ChallengeId != booking.ChallengeId || outcome.SceneEpoch != challengeScene || damage != challengeBody.Hp {
		t.Fatalf("未看到真实首领致死和同场玩家战败终态，累计承伤=%d 初始生命=%d 结果=%v", damage, challengeBody.Hp, &outcome)
	}
	returnFromWorldBossTest(c, booking.ChallengeId)
	assertChallengePlayerDeathReturned(t, c, ordinaryMap, ordinaryScene+2, ordinaryBody)

	// 原场次的迟到退出和当前冷却内的新预约都不得改写已确认的唯一失败。
	leave := c.send(p.MsgC2SPersonalBossLeave, &pb.PersonalBossLeave{SceneEpoch: challengeScene, ChallengeId: booking.ChallengeId})
	travelExpectError(c, p.MsgC2SPersonalBossLeave, leave, p.CodeInvalidArgument)
	entry.SceneEpoch, entry.ChallengeId = c.entry.SceneEpoch, domain.ID()
	retry := c.send(p.MsgC2SPersonalBossEnter, entry)
	travelUntilWithin(c, 5*time.Second, func(frame wire.Frame) bool {
		if frame.Request != retry || frame.ID != p.MsgS2CPersonalBossState {
			return false
		}
		var received pb.PersonalBossState
		must(t, proto.Unmarshal(frame.Payload, &received))
		if received.State == 1 {
			return false
		}
		if received.State != 6 || received.ErrorCode != p.CodeLineForbidden {
			t.Fatalf("失败重试冷却没有拒绝新场次：%v", &received)
		}
		return true
	})
	var terminal persistence.PersonalBossChallenge
	challenges := store.DB.Collection("personal_boss_challenges")
	must(t, challenges.FindOne(ctx, bson.M{"_id": booking.ChallengeId}).Decode(&terminal))
	if terminal.Outcome != persistence.PersonalBossPlayerDefeated || terminal.RoleID != roleID || terminal.Damage != 0 || terminal.HPRemaining != template.Monster.HP ||
		terminal.Successes != 0 || terminal.RewardGold != 0 || !terminal.AvailableAt.Equal(terminal.FinishedAt.Add(template.RetryCooldown)) || !terminal.FinishedAt.Before(terminal.ExpiresAt) {
		t.Fatalf("首领战败的原持久记录、失败冷却或收益错误：%+v", terminal)
	}
	challengeCount, err := challenges.CountDocuments(ctx, bson.M{"role_id": roleID})
	must(t, err)
	finishCount, err := store.DB.Collection("audit_outbox").CountDocuments(ctx, bson.M{"kind": "personal_boss_finish", "role_id": roleID})
	must(t, err)
	if challengeCount != 1 || finishCount != 1 {
		t.Fatalf("原角色出现重复预约或结束回执，场次=%d 结束=%d", challengeCount, finishCount)
	}
	query := c.send(p.MsgC2SPersonalBossQuery, &pb.PersonalBossQuery{})
	info := &pb.PersonalBossInfo{}
	frame := travelUntilWithin(c, 5*time.Second, func(frame wire.Frame) bool { return frame.Request == query && frame.ID == p.MsgS2CPersonalBossInfo })
	must(t, proto.Unmarshal(frame.Payload, info))
	if info.CanEnter || info.EligibilityReason != persistence.PersonalBossCoolingDown || info.Successes != 0 || info.ActiveChallengeId != "" ||
		info.AvailableAtUnixMs != terminal.AvailableAt.UnixMilli() || len(info.History) != 1 || info.History[0].ChallengeId != booking.ChallengeId ||
		info.History[0].Outcome != uint32(persistence.PersonalBossPlayerDefeated) {
		t.Fatal("首领战报或重试资格与唯一失败回执不符", info)
	}
	after, err := store.Role(ctx, roleID)
	must(t, err)
	travelAssertEconomy(t, before, after, "个人首领战败")
}

// TestSharedWorldBossPlayerDefeatMongoWS 核验战败席位归还且其它玩家继续在原开放场次行动。
func TestSharedWorldBossPlayerDefeatMongoWS(t *testing.T) {
	ctx, store, endpoint, route := startChallengePlayerDeathServer(t, func(cfg *devconfig.Config) {
		boss := cfg.Fixture.WorldBoss
		cfg.Fixture.WorldBossVariants = nil // 独立验证单场战败，无关真实运营轮换。
		boss.Map.PlayerSpawn, boss.Map.MonsterSpawns = devconfig.Point{X: 5, Y: 5}, []devconfig.Point{{X: 6, Y: 5}}
		// 第一个席位承担持续近战，第二个席位有时间在同场退到安全格。
		boss.Monster.Accuracy, boss.Monster.AttackMin, boss.Monster.AttackMax = 10_000, 5, 5
		boss.Monster.AttackMS, boss.Monster.MoveMS = 1_200, 60_000
		local := time.Now().In(time.FixedZone("Asia/Shanghai", 8*60*60))
		midnight := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, local.Location())
		boss.DailyOpenOffsets = []time.Duration{(local.Sub(midnight).Truncate(time.Millisecond) + 24*time.Hour - time.Minute) % (24 * time.Hour)}
		boss.Window = 3 * time.Minute
		if !cfg.Fixture.ValidWorldBoss() {
			t.Fatal("共享首领战败的隔离模板无效")
		}
	})
	first, role, token := joinWorldBossTestRole(t, endpoint, route)
	defer first.close()
	survivor, _, _ := joinWorldBossTestRole(t, endpoint, route)
	defer survivor.close()
	moveChallengePlayerDeathTest(t, first, &pb.GridPoint{X: 6, Y: 7})
	ordinaryScene, ordinaryMap := first.entry.SceneEpoch, first.entry.RoomId
	ordinaryBody := challengePlayerDeathBody(first)
	roleID := domain.Hex(role.Key.RoleId)
	before, err := store.Role(ctx, roleID)
	must(t, err)

	open := queryWorldBossTest(t, first)
	if open.State != 3 || open.OccurrenceId == "" {
		t.Fatal("隔离共享首领没有真实开放场次", open)
	}
	enterWorldBossTest(t, first, open)
	challengeScene, challengeBody := first.entry.SceneEpoch, challengePlayerDeathBody(first)
	monster := travelEntity(first.scene, 3)
	enterWorldBossTest(t, survivor, open)
	if townPeer(survivor, role.Name) == nil || challengeBody.GetHp() <= 0 || monster.GetDefinitionId() != store.Cfg.Fixture.WorldBoss.Monster.ID {
		t.Fatal("两人没有在同一个活动首领场景同步活体", first.scene, survivor.scene)
	}
	moveChallengePlayerDeathTest(t, survivor, &pb.GridPoint{X: 3, Y: 2})
	survivorBody := challengePlayerDeathBody(survivor)

	var damage int64
	travelUntilWithin(first, 8*time.Second, func(frame wire.Frame) bool {
		damage += challengePlayerDeathDamage(t, frame, challengeScene, monster, challengeBody)
		if frame.ID != p.MsgS2CWorldBossState || frame.Request != 0 {
			return false
		}
		var state pb.WorldBossState
		must(t, proto.Unmarshal(frame.Payload, &state))
		return state.State == 7 && state.SceneEpoch == challengeScene && state.OccurrenceId == open.OccurrenceId && state.Hp == open.Hp
	})
	if damage != challengeBody.Hp {
		t.Fatalf("共享首领未通过真实攻击打空战败玩家生命，累计承伤=%d 初始生命=%d", damage, challengeBody.Hp)
	}
	returnFromWorldBossTest(first, open.OccurrenceId)
	assertChallengePlayerDeathReturned(t, first, ordinaryMap, ordinaryScene+2, ordinaryBody)

	// 战败角色已返回再取健康新票。同一仍开放场次只能查询，重新入场必须拒绝。
	oldBinding := first.binding
	first.close()
	fresh := directoryTicket(t, endpoint, token, role.Key, route)
	returned := join(t, directoryTicketAddress(t, fresh), fresh.Ticket)
	defer returned.close()
	if returned.binding == oldBinding || returned.entry.SceneKind != devconfig.SceneKindPersonal || returned.entry.ServerTransitionId != "" {
		t.Fatal("健康重连未回原普通场景并更换会话绑定", returned.entry)
	}
	assertChallengePlayerDeathReturned(t, returned, ordinaryMap, returned.entry.SceneEpoch, ordinaryBody)
	stillOpen := queryWorldBossTest(t, returned)
	if stillOpen.State != 3 || stillOpen.OccurrenceId != open.OccurrenceId || stillOpen.Hp != open.Hp {
		t.Fatal("战败后的重入检查未落在仍开放的原场次", stillOpen)
	}
	denied := returned.send(p.MsgC2SWorldBossEnter, &pb.WorldBossEnter{SceneEpoch: returned.entry.SceneEpoch, TemplateId: open.TemplateId, OccurrenceId: open.OccurrenceId})
	travelExpectError(returned, p.MsgC2SWorldBossEnter, denied, p.CodeLineForbidden)
	if returned.entry.SceneKind != devconfig.SceneKindPersonal {
		t.Fatal("战败角色被重新附着到原首领场次", returned.entry)
	}

	// 用真实移动证明另一客户端仍能操作原共享身体，战败者也未重新发布到该场。
	survivorScene := survivor.entry.SceneEpoch
	moveChallengePlayerDeathTest(t, survivor, &pb.GridPoint{X: 4, Y: 2})
	state := queryWorldBossTest(t, survivor)
	activeBody := challengePlayerDeathBody(survivor)
	if state.State != 3 || state.OccurrenceId != open.OccurrenceId || state.Hp != open.Hp || survivor.entry.SceneEpoch != survivorScene ||
		survivor.entry.SceneKind != devconfig.SceneKindWorldBoss || townPeer(survivor, role.Name) != nil ||
		activeBody.EntityHandle != survivorBody.EntityHandle || activeBody.Life != survivorBody.Life || activeBody.Hp <= 0 {
		t.Fatal("战败席位未退场，或其它角色无法继续原共享场次", survivor.entry, state, survivor.scene)
	}
	after, err := store.Role(ctx, roleID)
	must(t, err)
	travelAssertEconomy(t, before, after, "世界首领战败")
}

// startChallengePlayerDeathServer 保留发布模板，只隔离致死输入和不会干扰原身体观测的普通怪。
func startChallengePlayerDeathServer(t *testing.T, configure func(*devconfig.Config)) (context.Context, *persistence.Store, string, directoryTestEntry) {
	t.Helper()
	if os.Getenv("MINIMIR_MONGO_E2E") != "1" || os.Getenv("MINIMIR_INTEGRATION_TEST_MONGO_URI") == "" {
		t.Skip("需要已核验的专属 MongoDB 副本集")
	}
	cfg, err := loadIntegrationConfig()
	must(t, err)
	cfg.Database = "minimir_dev_game_test_bdeath_" + domain.ID()
	cfg.AccountAddress, cfg.GameAddress = address(t), address(t)
	cfg.OriginID, cfg.RunID, cfg.RunServerIDs = 1, 1, []uint64{1}
	route := makeDirectoryEntry(1, 1, cfg, "boss-death-test", "隔离首领", "一服", 0, 0)
	cfg.OriginRoutes, cfg.RunServers = []devconfig.OriginRouteSeed{route.origin}, []devconfig.RunServerSeed{route.assignment}
	for i := range cfg.Fixture.Monsters {
		monster := &cfg.Fixture.Monsters[i]
		monster.AttackMin, monster.AttackMax, monster.Accuracy, monster.MoveMS = 0, 0, 0, 60_000
	}
	configure(&cfg)
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
				t.Errorf("隔离 %s 节点启动失败：%v", mode, err)
			}
		})
	}
	healthy(t, cfg.AccountAddress)
	healthy(t, cfg.GameAddress)
	return ctx, store, "http://" + cfg.AccountAddress + "/rpc", route
}

// challengePlayerDeathBody 按服务器分配的本角色句柄解析场景身体。
func challengePlayerDeathBody(c *client) *pb.EntityState {
	c.t.Helper()
	for _, entity := range c.scene.GetEntities() {
		if entity.EntityKind == 1 && entity.EntityHandle == c.entry.PlayerHandle {
			return entity
		}
	}
	c.t.Fatal("首基线缺少当前角色", c.entry, c.scene)
	return nil
}

// moveChallengePlayerDeathTest 等待正常格子移动产生的基线。
func moveChallengePlayerDeathTest(t *testing.T, c *client, target *pb.GridPoint) {
	t.Helper()
	c.send(p.MsgC2SMoveIntent, &pb.MoveIntent{SceneEpoch: c.entry.SceneEpoch, Destination: target})
	travelUntilWithin(c, 5*time.Second, func(frame wire.Frame) bool {
		return frame.ID == p.MsgS2CSceneBaseline && c.scene.SceneEpoch == c.entry.SceneEpoch && proto.Equal(challengePlayerDeathBody(c).Position, target)
	})
}

// challengePlayerDeathDamage 只统计原首领命中同场角色原生命时的真实扣血。
func challengePlayerDeathDamage(t *testing.T, frame wire.Frame, scene uint64, monster, player *pb.EntityState) int64 {
	t.Helper()
	if frame.ID != p.MsgS2CCombatImpact {
		return 0
	}
	var impact pb.CombatImpact
	must(t, proto.Unmarshal(frame.Payload, &impact))
	if impact.SceneEpoch == scene && impact.CasterHandle == monster.EntityHandle && impact.TargetHandle == player.EntityHandle && impact.TargetLife == player.Life && !impact.Healing {
		return impact.Amount
	}
	return 0
}

// assertChallengePlayerDeathReturned 校验挑战临时战败不会污染原普通场景的身体或位置。
func assertChallengePlayerDeathReturned(t *testing.T, c *client, room, scene uint64, before *pb.EntityState) {
	t.Helper()
	after := challengePlayerDeathBody(c)
	if c.entry.SceneKind != devconfig.SceneKindPersonal || c.entry.RoomId != room || c.entry.SceneEpoch != scene ||
		after.EntityHandle != before.EntityHandle || after.Life != before.Life || after.Hp != before.Hp || after.Mp != before.Mp || after.Hp <= 0 ||
		!proto.Equal(after.Position, before.Position) || c.death != nil {
		t.Fatal("挑战战败污染原普通角色或未返回原普通地图", c.entry, before, after, c.death)
	}
}
