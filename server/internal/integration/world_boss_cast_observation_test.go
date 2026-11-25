// 本文件通过真实双端网络核对共享首领施法、命中及异服隔离。
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

// TestSharedWorldBossCastObservationMongoWS 核对不同本地场景和附着代次共见同一施法身份。
func TestSharedWorldBossCastObservationMongoWS(t *testing.T) {
	if os.Getenv("MINIMIR_MONGO_E2E") != "1" {
		t.Skip("共享施法表现需要真实独立 MongoDB")
	}
	cfg, err := loadIntegrationConfig()
	must(t, err)
	cfg.Database = "minimir_dev_game_test_wb_cast_" + domain.ID()
	cfg.AccountAddress, cfg.GameAddress = address(t), address(t)
	cfg.OriginID, cfg.RunID, cfg.RunServerIDs = 1, 1, []uint64{1, 2}
	run := makeDirectoryEntry(1, 1, cfg, "effect-test", "共享施法", "一服", 0, 0)
	otherRun := makeDirectoryEntry(2, 2, cfg, "effect-test", "共享施法", "二服", 0, 1)
	cfg.OriginRoutes = []devconfig.OriginRouteSeed{run.origin, otherRun.origin}
	cfg.RunServers = []devconfig.RunServerSeed{run.assignment, otherRun.assignment}
	boss := cfg.Fixture.Monsters[0]
	boss.ID, boss.HP, boss.AttackMin, boss.AttackMax, boss.MagicDefenceMin, boss.MagicDefenceMax, boss.MagicResist = 80002, 3000, 0, 0, 0, 0, 0
	boss.MoveMS, boss.AttackMS, boss.Exp, boss.GoldMin, boss.GoldMax, boss.RespawnMS, boss.DropPoolID = 10_000, 10_000, 0, 0, 0, 0, 0
	shanghai := time.FixedZone("Asia/Shanghai", 8*60*60)
	local := time.Now().In(shanghai)
	midnight := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, shanghai)
	opens := (local.Sub(midnight).Truncate(time.Millisecond) + 24*time.Hour - time.Minute) % (24 * time.Hour)
	cfg.Fixture.WorldBoss = &devconfig.WorldBossConfig{TemplateID: 2, MinimumLevel: 1,
		Map: devconfig.MapTemplate{ID: 81, Width: 13, Height: 13, SceneKind: devconfig.SceneKindWorldBoss,
			PlayerSpawn: devconfig.Point{X: 5, Y: 5}, MonsterSpawns: []devconfig.Point{{X: 6, Y: 5}}},
		Monster: boss, DailyOpenOffsets: []time.Duration{opens}, Window: 3 * time.Minute, ReturnDelay: 300 * time.Millisecond, ParticipantLimit: 8}
	cfg.Fixture.WorldBossVariants = nil
	if !cfg.Fixture.ValidWorldBoss() {
		t.Fatal("共享施法隔离首领配置无效")
	}
	fireball, exists := cfg.Fixture.Skill(1)
	if !exists || fireball.Behavior != "fireball" {
		t.Fatal("缺少当前候选的火球术")
	}

	ctx, cancel := context.WithCancel(context.Background())
	store, err := persistence.Open(ctx, cfg)
	must(t, err)
	var nodes sync.WaitGroup
	for _, mode := range []string{"account", "game"} {
		nodes.Go(func() {
			if err := bootstrap.Run(ctx, cfg, mode); err != nil && ctx.Err() == nil {
				t.Errorf("隔离 %s：%v", mode, err)
			}
		})
	}
	t.Cleanup(func() {
		cancel()
		nodes.Wait()
		cleanup, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
		defer store.Close(cleanup)
		must(t, store.DB.Drop(cleanup))
	})
	healthy(t, cfg.AccountAddress)
	healthy(t, cfg.GameAddress)
	endpoint := "http://" + cfg.AccountAddress + "/rpc"
	// 将角色置于已学测试技能前态，之后切场、受理、结算和观察均通过真实 WebSocket。
	createMage := func(selection directoryTestEntry) *client {
		login, role := &pb.LoginResult{}, &pb.RoleSummary{}
		rpc(t, endpoint, "", p.MsgC2SLocalRegister, &pb.LocalLoginRequest{Account: "wbfx_" + domain.ID()[:8], Password: "test_123456"}, login, 0)
		rpc(t, endpoint, login.AccountToken, p.MsgC2SCreateRole, &pb.CreateRoleRequest{OriginServerId: selection.origin.OriginID,
			Slot: 1, Job: 1, Name: "观法" + domain.ID()[:7], OpId: domain.Bytes(domain.ID())}, role, 0)
		_, err := store.DB.Collection("roles").UpdateOne(ctx, bson.M{"_id": domain.Hex(role.Key.RoleId)}, bson.M{
			"$set": bson.M{"level": fireball.LearnLevel, "learned_skills": []domain.LearnedSkill{{SkillID: fireball.ID}}},
		})
		must(t, err)
		ticket := directoryTicket(t, endpoint, login.AccountToken, role.Key, selection)
		c := join(t, directoryTicketAddress(t, ticket), ticket.Ticket)
		c.auto(false, 1)
		enterWorldBossTest(t, c, queryWorldBossTest(t, c))
		return c
	}
	first, second, abroad := createMage(run), createMage(run), createMage(otherRun)
	defer first.close()
	defer second.close()
	defer abroad.close()
	for _, pair := range [][2]*client{{first, second}, {second, first}} {
		travelUntilWithin(pair[0], 4*time.Second, func(frame wire.Frame) bool {
			return frame.ID == p.MsgS2CSceneBaseline && pair[0].entity(pair[1].entry.PlayerHandle) != nil
		})
	}

	// 两个发送者的第一个技能都应取得场次内不同的唯一编号。
	castAndObserve := func(caster, observer *client, sequence uint32) (uint64, uint64) {
		caster.t.Helper()
		target := travelEntity(caster.scene, 3)
		caster.send(p.MsgC2SCastIntent, &pb.CastIntent{SceneEpoch: caster.entry.SceneEpoch, InputSequence: sequence,
			SkillId: fireball.ID, TargetHandle: target.EntityHandle})
		var acceptedID, impactID uint64
		for index, c := range []*client{caster, observer} {
			start := &pb.CastAccepted{}
			frame := travelUntilWithin(c, 4*time.Second, func(frame wire.Frame) bool {
				if frame.ID != p.MsgS2CCastAccepted {
					return false
				}
				must(t, proto.Unmarshal(frame.Payload, start))
				return start.CasterHandle == caster.entry.PlayerHandle
			})
			expectedInput := sequence
			if index == 1 {
				expectedInput = 0
			}
			if frame.Binding != c.binding || frame.Request != 0 || start.SceneEpoch != c.entry.SceneEpoch ||
				start.InputSequence != expectedInput || start.CasterLife != caster.player().Life ||
				start.TargetHandle != target.EntityHandle || start.TargetLife != target.Life || start.CastId == 0 {
				t.Fatalf("施法表现串了原身体、意图或观察代次：%+v", start)
			}
			hit := &pb.CombatImpact{}
			frame = travelUntilWithin(c, 5*time.Second, func(frame wire.Frame) bool {
				if frame.ID != p.MsgS2CCombatImpact {
					return false
				}
				must(t, proto.Unmarshal(frame.Payload, hit))
				return hit.CasterHandle == start.CasterHandle && hit.CastId == start.CastId
			})
			if frame.Binding != c.binding || hit.SceneEpoch != c.entry.SceneEpoch || hit.SkillId != fireball.ID ||
				hit.TargetHandle != target.EntityHandle || hit.TargetLife != target.Life || hit.Outcome != 1 || hit.Amount <= 0 {
				t.Fatalf("共享命中表现不匹配原施法：%+v", hit)
			}
			if index == 0 {
				acceptedID, impactID = start.CastId, hit.EventId
			} else if start.CastId != acceptedID || hit.EventId != impactID {
				t.Fatal("同一真实施法在旁观端重新编号")
			}
		}
		return acceptedID, impactID
	}
	firstCast, firstImpact := castAndObserve(first, second, 1)
	secondCast, secondImpact := castAndObserve(second, first, 1)
	if secondCast <= firstCast || secondImpact <= firstImpact {
		t.Fatal("不同施法者复用了前端的同场去重编号", firstCast, secondCast, firstImpact, secondImpact)
	}
	inOtherRun := 0
	abroad.onFrame = func(_ *client, frame wire.Frame) {
		if frame.ID == p.MsgS2CCastAccepted {
			inOtherRun++
		}
		if frame.ID == p.MsgS2CCombatImpact {
			hit := &pb.CombatImpact{}
			must(t, proto.Unmarshal(frame.Payload, hit))
			if hit.SkillId == fireball.ID {
				inOtherRun++
			}
		}
	}
	// 独立运行服查询回执排在可能串发的表现之后，并且它自己的首领未失血。
	if state := queryWorldBossTest(t, abroad); state.Hp != boss.HP || inOtherRun != 0 {
		t.Fatal("另一个运行服收到了本场施法或其命中结果", state, inOtherRun)
	}
}
