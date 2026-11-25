// 本文件用独立 MongoDB 和真实 WebSocket 验证个人首领健康重连与离线到期。
package integration

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/coder/websocket"
	"go.mongodb.org/mongo-driver/v2/bson"
	"google.golang.org/protobuf/proto"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
	"minimir/internal/wire"
)

type reconnectBossRecord struct {
	persistence.PersonalBossChallenge `bson:",inline"`
	SessionEpoch                      int64  `bson:"session_epoch"`
	RunEpoch                          int64  `bson:"run_epoch"`
	OwnerInstance                     string `bson:"owner_instance"`
}

// TestPersonalBossHealthyReconnectMongoWS 核验断线续战、旧附着拦截及原场次的唯一终态。
func TestPersonalBossHealthyReconnectMongoWS(t *testing.T) {
	ctx, store, endpoint, route := startChallengePlayerDeathServer(t, func(cfg *devconfig.Config) {
		boss := cfg.Fixture.PersonalBoss
		boss.Map.PlayerSpawn, boss.Map.MonsterSpawns = devconfig.Point{X: 5, Y: 5}, []devconfig.Point{{X: 6, Y: 5}}
		boss.Monster.HP, boss.Monster.Agility, boss.Monster.MoveMS = 300, 0, 60_000
		boss.Monster.Defence, boss.Monster.DefenceMin, boss.Monster.DefenceMax = 0, 0, 0
		boss.Monster.AttackMin, boss.Monster.AttackMax, boss.Monster.Accuracy = 0, 0, 0
		boss.Duration, boss.RetryCooldown = 12*time.Second, 15*time.Second
		if !cfg.Fixture.ValidPersonalBoss() {
			t.Fatal("个人首领健康重连隔离模板不合法")
		}
	})

	t.Run("原受伤首领和角色现场续接，旧附着失权，主动退出只记原场一次", func(t *testing.T) {
		origin, role, token := joinWorldBossTestRole(t, endpoint, route)
		t.Cleanup(origin.close)
		roleID := domain.Hex(role.Key.RoleId)
		ordinaryMap := origin.entry.RoomId
		moveChallengePlayerDeathTest(t, origin, &pb.GridPoint{X: 6, Y: 7})
		ordinaryBody := proto.Clone(challengePlayerDeathBody(origin)).(*pb.EntityState)
		economy, err := store.Role(ctx, roleID)
		must(t, err)
		challenge, _ := enterReconnectBoss(t, origin, store.Cfg.Fixture.PersonalBoss.TemplateID, false)
		boss := travelEntity(origin.scene, 3)
		origin.send(p.MsgC2SCastIntent, &pb.CastIntent{SceneEpoch: origin.entry.SceneEpoch, TargetHandle: boss.EntityHandle})
		travelUntilWithin(origin, 4*time.Second, func(frame wire.Frame) bool {
			hp := travelEntity(origin.scene, 3).GetHp()
			return frame.ID == p.MsgS2CSceneBaseline && hp > 0 && hp < boss.Hp
		})
		moveChallengePlayerDeathTest(t, origin, &pb.GridPoint{X: 5, Y: 7})
		current := reconnectBossInfo(t, origin)
		initial := reconnectBossDocument(t, ctx, store, roleID, challenge.ChallengeId)
		if initial.Outcome != persistence.PersonalBossActive || current.ActiveChallengeId != challenge.ChallengeId ||
			current.ActiveExpiresAtUnixMs != initial.ExpiresAt.UnixMilli() || initial.CombatSnapshot == "" {
			t.Fatal("原场次未取得已落库、可继续的入场快照和时限", initial, current)
		}
		oldScene, oldBaseline, oldBinding := origin.entry.SceneEpoch, origin.entry.BaselineId, origin.binding
		oldBody := proto.Clone(challengePlayerDeathBody(origin)).(*pb.EntityState)
		oldBoss := proto.Clone(travelEntity(origin.scene, 3)).(*pb.EntityState)
		origin.close()
		// 等待多个场景 tick 后核对原预约仍活动，覆盖旧版掉线立即中断路径。
		time.Sleep(350 * time.Millisecond)
		if offline := reconnectBossDocument(t, ctx, store, roleID, challenge.ChallengeId); offline.Outcome != persistence.PersonalBossActive {
			t.Fatalf("健康连接中断直接改写了原挑战终态：%+v", offline)
		}

		ticket := directoryTicket(t, endpoint, token, role.Key, route)
		resumed := attachReconnectBoss(t, directoryTicketAddress(t, ticket), ticket.Ticket, challenge.ChallengeId)
		if resumed.entry.SceneEpoch <= oldScene || resumed.entry.BaselineId == oldBaseline || resumed.binding <= oldBinding {
			t.Fatal("健康重连未更新场景、基线和连接绑定三项观察身份", resumed.entry, resumed.binding)
		}
		body, retainedBoss := challengePlayerDeathBody(resumed), travelEntity(resumed.scene, 3)
		if retainedBoss.GetHp() != oldBoss.Hp || retainedBoss.GetLife() != oldBoss.Life || retainedBoss.GetEntityHandle() != oldBoss.EntityHandle ||
			body.Life != oldBody.Life || !proto.Equal(body.Position, oldBody.Position) || body.Hp != oldBody.Hp {
			t.Fatal("新票恢复了错误的角色身体、位置或首领生命", oldBody, body, oldBoss, retainedBoss)
		}
		updated := reconnectBossDocument(t, ctx, store, roleID, challenge.ChallengeId)
		if updated.Outcome != persistence.PersonalBossActive || !updated.ExpiresAt.Equal(initial.ExpiresAt) ||
			updated.CombatSnapshot != initial.CombatSnapshot || updated.Execution != initial.Execution || updated.ReservationID != initial.ReservationID ||
			updated.SessionEpoch <= initial.SessionEpoch || updated.RunEpoch != initial.RunEpoch || updated.OwnerInstance != initial.OwnerInstance {
			t.Fatal("续战未按原快照、原截止及原执行迁移当前会话归属", initial, updated)
		}
		// 新连接用旧场景的 ACK 无权开放动作；正确 ACK 后原自动关闭偏好仍在。
		resumed.send(p.MsgC2SEnterWorldAck, &pb.EnterWorldAck{SceneEpoch: oldScene, BaselineId: oldBaseline})
		premature := resumed.send(p.MsgC2SMoveIntent, &pb.MoveIntent{SceneEpoch: resumed.entry.SceneEpoch, Direction: 2})
		travelExpectError(resumed, p.MsgC2SMoveIntent, premature, p.CodeLineRecovering)
		travelAcknowledge(resumed, false, true)
		requireChallengeAuto(resumed, false)
		assertReconnectBossTicketConsumed(t, route.assignment.GameWSURL, ticket.Ticket)
		stale := resumed.send(p.MsgC2SPersonalBossLeave, &pb.PersonalBossLeave{SceneEpoch: oldScene, ChallengeId: challenge.ChallengeId})
		travelExpectError(resumed, p.MsgC2SPersonalBossLeave, stale, p.CodeInvalidArgument)
		active := reconnectBossInfo(t, resumed)
		if active.ActiveChallengeId != challenge.ChallengeId || active.ActiveExpiresAtUnixMs != current.ActiveExpiresAtUnixMs {
			t.Fatal("旧场景动作改变了挑战或重置了原倒计时", current, active)
		}
		moveChallengePlayerDeathTest(t, resumed, &pb.GridPoint{X: 5, Y: 8})
		resumed.send(p.MsgC2SPersonalBossLeave, &pb.PersonalBossLeave{SceneEpoch: resumed.entry.SceneEpoch, ChallengeId: challenge.ChallengeId})
		returnFromWorldBossTest(resumed, challenge.ChallengeId)
		finish := assertReconnectBossTerminal(t, ctx, store, resumed, roleID, challenge.ChallengeId, persistence.PersonalBossInterrupted)
		if finish.HPRemaining != oldBoss.Hp || finish.Damage != finish.BossHP-oldBoss.Hp || !finish.FinishedAt.Before(initial.ExpiresAt) {
			t.Fatal("主动退出未保留原受伤首领、累计伤害和实际退出时点", finish)
		}
		assertReconnectBossOrdinary(t, resumed, ordinaryMap, ordinaryBody)
		final, err := store.Role(ctx, roleID)
		must(t, err)
		travelAssertEconomy(t, economy, final, "个人首领健康续战后主动退出")
	})

	t.Run("未重新连接时仍按原时限结束，后来新票返回普通场景并可查唯一终态", func(t *testing.T) {
		origin, role, token := joinWorldBossTestRole(t, endpoint, route)
		t.Cleanup(origin.close)
		roleID := domain.Hex(role.Key.RoleId)
		ordinaryMap := origin.entry.RoomId
		ordinaryBody := proto.Clone(challengePlayerDeathBody(origin)).(*pb.EntityState)
		economy, err := store.Role(ctx, roleID)
		must(t, err)
		challenge, ackAt := enterReconnectBoss(t, origin, store.Cfg.Fixture.PersonalBoss.TemplateID, true)
		beforeDrop := travelEntity(origin.scene, 3).GetHp()
		fighter := challengePlayerDeathBody(origin)
		maxHit := fighter.GetCombatAttributes().GetPhysicalAttackMax()
		attackInterval := time.Duration(fighter.GetAttackIntervalMs()) * time.Millisecond
		online := time.Since(ackAt)
		origin.close()
		if origin.hosting.GetRemainHostingSec() != 0 || beforeDrop <= 0 || maxHit <= 0 || attackInterval <= 0 || online >= attackInterval {
			t.Fatalf("离线推进前态不满足零托管时间、活首领和最多一次在线普攻的条件：血量=%d 单次上限=%d 在线=%v 冷却=%v",
				beforeDrop, maxHit, online, attackInterval)
		}
		original := reconnectBossDocument(t, ctx, store, roleID, challenge.ChallengeId)
		time.Sleep(350 * time.Millisecond)
		if early := reconnectBossDocument(t, ctx, store, roleID, challenge.ChallengeId); early.Outcome != persistence.PersonalBossActive {
			t.Fatalf("已开始的离线场次在原截止前被提前取消：%+v", early)
		}
		// 新票前先从隔离库看到唯一终态，排除由新入场请求补做旧结果。
		var offline reconnectBossRecord
		for until := original.ExpiresAt.Add(5 * time.Second); time.Now().Before(until); {
			offline = reconnectBossDocument(t, ctx, store, roleID, challenge.ChallengeId)
			if offline.Outcome != persistence.PersonalBossActive {
				break
			}
			time.Sleep(75 * time.Millisecond)
		}
		if offline.Outcome != persistence.PersonalBossEscaped || !offline.FinishedAt.Equal(original.ExpiresAt) || offline.Recovered {
			t.Fatal("离线挑战未按原先截止取得自然逃走终态", original, offline)
		}
		if offline.Damage <= original.BossHP-beforeDrop+maxHit || offline.HPRemaining != original.BossHP-offline.Damage {
			t.Fatal("累计伤害没有超过断线前已观察伤害与至多一次在线普攻上限", beforeDrop, maxHit, offline)
		}
		t.Logf("零托管时间离线自动继续，断线前首领生命=%d 单次普攻上限=%d，原时限结束累计伤害=%d 剩余生命=%d",
			beforeDrop, maxHit, offline.Damage, offline.HPRemaining)
		ticket := directoryTicket(t, endpoint, token, role.Key, route)
		resumed := join(t, directoryTicketAddress(t, ticket), ticket.Ticket)
		t.Cleanup(resumed.close)
		assertReconnectBossOrdinary(t, resumed, ordinaryMap, ordinaryBody)
		finish := assertReconnectBossTerminal(t, ctx, store, resumed, roleID, challenge.ChallengeId, persistence.PersonalBossEscaped)
		if !reflect.DeepEqual(finish, offline) {
			t.Fatal("超时后健康新票改写了原终态", offline, finish)
		}
		final, err := store.Role(ctx, roleID)
		must(t, err)
		travelAssertEconomy(t, economy, final, "个人首领离线到期")
	})
}

func enterReconnectBoss(t *testing.T, c *client, template uint32, automatic bool) (*pb.PersonalBossState, time.Time) {
	t.Helper()
	request := c.send(p.MsgC2SPersonalBossEnter, &pb.PersonalBossEnter{SceneEpoch: c.entry.SceneEpoch, ChallengeId: domain.ID(), TemplateId: template})
	frame := travelUntilWithin(c, 5*time.Second, func(frame wire.Frame) bool { return frame.Request == request })
	state := &pb.PersonalBossState{}
	if frame.ID != p.MsgS2CPersonalBossState || frame.Error != 0 {
		t.Fatal("未收到原个人首领预约", frame.ID, frame.Error)
	}
	must(t, proto.Unmarshal(frame.Payload, state))
	if state.State != 1 || state.ChallengeId == "" {
		t.Fatal("个人首领预约状态无效", state)
	}
	travelUntilWithin(c, 5*time.Second, func(frame wire.Frame) bool {
		return frame.ID == p.MsgS2CSceneBaseline && c.entry.SceneKind == devconfig.SceneKindPersonalBoss && c.scene.SceneEpoch == c.entry.SceneEpoch
	})
	ackAt := time.Now()
	travelAcknowledge(c, false, automatic)
	requireChallengeAuto(c, automatic)
	return state, ackAt
}

func attachReconnectBoss(t *testing.T, address, ticket, challenge string) *client {
	t.Helper()
	ctx, stop := context.WithTimeout(context.Background(), 25*time.Second)
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
	state := &pb.PersonalBossState{}
	travelUntilWithin(c, 5*time.Second, func(frame wire.Frame) bool {
		if frame.ID != p.MsgS2CPersonalBossState {
			return false
		}
		must(t, proto.Unmarshal(frame.Payload, state))
		return state.State == 2
	})
	travelUntilWithin(c, 5*time.Second, func(frame wire.Frame) bool {
		return frame.ID == p.MsgS2CSceneBaseline && c.entry.SceneKind == devconfig.SceneKindPersonalBoss && c.scene.SceneEpoch == c.entry.SceneEpoch
	})
	if state.ChallengeId != challenge || state.SceneEpoch != c.entry.SceneEpoch || c.entry.ServerTransitionId != challenge {
		t.Fatal("原普通场景 ACK 后没有按同一 challenge_id 续接个人首领", state, c.entry)
	}
	return c
}

// 成功附着消耗了入口票据。重放同一张票不能再次接管原挑战。
func assertReconnectBossTicketConsumed(t *testing.T, address, ticket string) {
	t.Helper()
	ctx, stop := context.WithTimeout(context.Background(), 5*time.Second)
	defer stop()
	ws, _, err := websocket.Dial(ctx, address, nil)
	must(t, err)
	defer ws.CloseNow()
	must(t, ws.Write(ctx, websocket.MessageBinary, wire.Encode(wire.Message(
		p.MsgC2SEnterLine, 1, 0, &pb.EnterLineRequest{AccountTicket: ticket},
	))))
	_, raw, err := ws.Read(ctx)
	must(t, err)
	frame, err := wire.Decode(raw)
	must(t, err)
	if frame.ID != p.MsgS2CEnterLine || frame.Request != 1 || frame.Error != p.CodeUnauthenticated {
		t.Fatalf("用掉的一次性票没有在首基线前拒绝：消息=%d 请求=%d 错误=%d", frame.ID, frame.Request, frame.Error)
	}
}

func reconnectBossInfo(t *testing.T, c *client) *pb.PersonalBossInfo {
	t.Helper()
	request := c.send(p.MsgC2SPersonalBossQuery, &pb.PersonalBossQuery{})
	frame := travelUntilWithin(c, 5*time.Second, func(frame wire.Frame) bool {
		return frame.Request == request && frame.ID == p.MsgS2CPersonalBossInfo
	})
	info := &pb.PersonalBossInfo{}
	must(t, proto.Unmarshal(frame.Payload, info))
	return info
}

func reconnectBossDocument(t *testing.T, ctx context.Context, store *persistence.Store, role, challenge string) reconnectBossRecord {
	t.Helper()
	var record reconnectBossRecord
	must(t, store.DB.Collection("personal_boss_challenges").FindOne(ctx, bson.M{"_id": challenge, "role_id": role}).Decode(&record))
	return record
}

func assertReconnectBossTerminal(t *testing.T, ctx context.Context, store *persistence.Store, c *client,
	role, challenge string, outcome persistence.PersonalBossOutcome) reconnectBossRecord {
	t.Helper()
	record := reconnectBossDocument(t, ctx, store, role, challenge)
	info := reconnectBossInfo(t, c)
	if record.Outcome != outcome || record.Successes != 0 || record.Recovered || record.RewardGold != 0 || len(record.RewardItems) != 0 ||
		!record.AvailableAt.Equal(record.FinishedAt.Add(record.RetryCooldown)) ||
		info.ActiveChallengeId != "" || info.CanEnter || info.EligibilityReason != persistence.PersonalBossCoolingDown ||
		info.AvailableAtUnixMs != record.AvailableAt.UnixMilli() || len(info.History) != 1 ||
		info.History[0].ChallengeId != challenge || info.History[0].Outcome != uint32(outcome) || info.History[0].Damage != record.Damage {
		t.Fatal("原场次唯一结算、零发放、冷却或查询战报不一致", record, info)
	}
	for _, phase := range []string{"begin", "finish"} {
		count, err := store.DB.Collection("audit_outbox").CountDocuments(ctx, bson.M{"kind": "personal_boss_" + phase, "role_id": role})
		must(t, err)
		if count != 1 {
			t.Fatalf("原角色的首领审计 %s=%d，期待一次", phase, count)
		}
	}
	return record
}

func assertReconnectBossOrdinary(t *testing.T, c *client, mapID uint64, body *pb.EntityState) {
	t.Helper()
	current := challengePlayerDeathBody(c)
	if c.entry.SceneKind != devconfig.SceneKindPersonal || c.entry.RoomId != mapID || current.Life != body.Life ||
		!proto.Equal(current.Position, body.Position) || current.Hp != body.Hp {
		t.Fatal("结果返回未回到原普通挂机身体和地块", c.entry, current, body)
	}
}
