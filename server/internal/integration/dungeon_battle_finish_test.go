// 本文件通过真实 WebSocket、运行服维护和 MongoDB 对账验证讨伐超时及中断的唯一终态。
package integration

import (
	"reflect"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"google.golang.org/protobuf/proto"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	"minimir/internal/persistence"
)

// TestDungeonBattleTimeoutMongoWS 用原倒计时触发失败，按唯一终态回原即时身体且保留入场扣次。
func TestDungeonBattleTimeoutMongoWS(t *testing.T) {
	const duration = 5 * time.Second
	store, endpoint, route, _, _ := startDungeonServer(t, true, func(cfg *devconfig.Config) {
		cfg.Fixture.DungeonReleases[0].DurationMS = duration.Milliseconds()
		monster := &cfg.Fixture.ChallengeMonsters[0]
		monster.AttackMin, monster.AttackMax, monster.AttackMS = 0, 0, 60_000
	})
	recruit := newDungeonRecruit(t, endpoint, route, 0)
	c := recruit.attach(t)
	original := proto.Clone(c.entry).(*pb.EnterLineResult)
	originalBody := proto.Clone(dungeonOwnBody(c)).(*pb.EntityState)
	roleID := domain.Hex(recruit.summary.Key.RoleId)
	before, err := store.Role(t.Context(), roleID)
	must(t, err)

	challenge := startSingleDungeonForFinish(t, c)
	confirmDungeonBattleArrival(c, challenge, original)
	waitDungeonBattleState(c, challenge, 1, 2)
	active := dungeonFinishRecord(t, store, challenge)
	if active.Outcome != persistence.DungeonActive || active.ExpiresAt.Sub(active.StartedAt) != duration || len(active.Members) != 1 ||
		active.Members[0].EntriesAfter != 1 || active.Members[0].RoleID != roleID {
		t.Fatal("客户端就绪后未保留原永久扣次、席位和时限", active)
	}

	waitDungeonBattleState(c, challenge, 1, 5)
	waitDungeonSceneBaseline(c, challenge, original.SceneKind)
	travelAcknowledge(c, false)
	body := dungeonOwnBody(c)
	if c.entry.RoomId != original.RoomId || c.entry.SceneKind != original.SceneKind || c.entry.SceneEpoch <= original.SceneEpoch ||
		body == nil || body.Life != originalBody.Life || !proto.Equal(body.Position, originalBody.Position) || c.hosting.AutoBattleEnabled {
		t.Fatal("正常超时没有恢复原普通地图、脚点、生命或原自动状态", original, c.entry, originalBody, body, c.hosting)
	}
	closed := dungeonFinishRecord(t, store, challenge)
	if closed.Outcome != persistence.DungeonFailed || closed.Refunded || closed.Recovered || !closed.FinishedAt.Equal(active.ExpiresAt) {
		t.Fatal("原倒计时未精确确认普通失败，或错误退款", active, closed)
	}
	after, err := store.Role(t.Context(), roleID)
	must(t, err)
	if after.Dungeon == nil || after.Dungeon.Entries != 1 || after.Dungeon.ActiveChallengeID != "" ||
		after.Dungeon.EntryDay != active.Members[0].EntryDay {
		t.Fatal("普通超时改变了原日已用次数或保留了原活动标记", after.Dungeon)
	}
	travelAssertEconomy(t, before, after, "讨伐普通超时")
	assertDungeonFinishAudits(t, store, closed, roleID)
	t.Logf("原倒计时 %s 到期后正常返场，原日 %s 持续保留 1 次入场扣次", duration, after.Dungeon.EntryDay)
}

// TestDungeonBattleMaintenanceBeforeAckMongoWS 保留未确认副本首帧的连接，施加真实停服并验证原日退款。
func TestDungeonBattleMaintenanceBeforeAckMongoWS(t *testing.T) {
	store, endpoint, route, _, stopGame := startDungeonServer(t, true)
	recruit := newDungeonRecruit(t, endpoint, route, 0)
	roleID := domain.Hex(recruit.summary.Key.RoleId)
	// 前置正常使用两次的角色原日计数；实际第三次扣减及退款均经 WebSocket 与正式事务发生。
	day := time.Now().In(time.FixedZone("Asia/Shanghai", 8*60*60)).Format("2006-01-02")
	_, err := store.DB.Collection("roles").UpdateByID(t.Context(), roleID,
		bson.M{"$set": bson.M{"dungeon": domain.DungeonProgress{EntryDay: day, Entries: 2}}})
	must(t, err)
	c := recruit.attach(t)
	original := proto.Clone(c.entry).(*pb.EnterLineResult)
	originalBinding := c.binding
	before, err := store.Role(t.Context(), roleID)
	must(t, err)

	challenge := startSingleDungeonForFinish(t, c)
	waitDungeonSceneBaseline(c, challenge, devconfig.SceneKindDungeon)
	waitDungeonBattleState(c, challenge, 1, 1)
	active := dungeonFinishRecord(t, store, challenge)
	if active.Outcome != persistence.DungeonActive || len(active.Members) != 1 || active.Members[0].RoleID != roleID ||
		active.Members[0].EntriesAfter != 3 || active.Members[0].EntryDay != day || !time.Now().Before(active.ExpiresAt) {
		t.Fatal("未确认首帧的原场需要在时限内保留已提交的第三次日扣次", active)
	}
	charged, err := store.Role(t.Context(), roleID)
	must(t, err)
	if charged.Dungeon == nil || charged.Dungeon.Entries != 3 || charged.Dungeon.ActiveChallengeID != challenge {
		t.Fatal("原场次在停服前尚未实扣三次或没有原占用", charged.Dungeon)
	}

	stopGame()
	c.close()
	closed := dungeonFinishRecord(t, store, challenge)
	if closed.Outcome != persistence.DungeonSystemInterrupted || !closed.Refunded || closed.Recovered || closed.ReservationID != active.ReservationID ||
		closed.Execution != active.Execution || closed.FinishedAt.Before(active.StartedAt) || !closed.FinishedAt.Before(active.ExpiresAt) {
		t.Fatal("停服未在原运行代次将未确认首帧的活动场退回原日", active, closed)
	}
	after, err := store.Role(t.Context(), roleID)
	must(t, err)
	if after.Dungeon == nil || after.Dungeon.Entries != 2 || after.Dungeon.EntryDay != day ||
		after.Dungeon.ActiveChallengeID != "" || after.Revision != charged.Revision+1 {
		t.Fatal("维护退款未按原角色修订保留原两次用量", after)
	}
	travelAssertEconomy(t, before, after, "讨伐未确认首帧时维护")
	assertDungeonFinishAudits(t, store, closed, roleID)

	startDurableWorldBossNode(t, store.Cfg, "game", store.Cfg.GameAddress)
	newConn := recruit.attach(t)
	if newConn.entry.RoomId != original.RoomId || newConn.entry.SceneKind != original.SceneKind || newConn.entry.ServerTransitionId != "" ||
		newConn.binding <= originalBinding {
		t.Fatal("新运行服没有用新票回原普通地图，或重新附着了已确认中断", original, newConn.entry)
	}
	replayed := dungeonFinishRecord(t, store, challenge)
	reloaded, err := store.Role(t.Context(), roleID)
	must(t, err)
	if !reflect.DeepEqual(replayed, closed) || reloaded.RunEpoch <= active.RunEpoch || reloaded.Revision != after.Revision+1 ||
		!reflect.DeepEqual(reloaded.Dungeon, after.Dungeon) {
		t.Fatal("维护退次被下一代启动或新票重复提交", closed, replayed, after.Dungeon, reloaded.Dungeon)
	}
	assertDungeonFinishAudits(t, store, replayed, roleID)
	t.Logf("未确认首帧的原日 %s 扣次 2→3→2；运行服换代与新票未重复退款", day)
}

// TestDungeonBattleOrdinaryHostingMongoWS 让普通托管与同一玩家的特殊挑战并行，核对扣时交易和原截止。
func TestDungeonBattleOrdinaryHostingMongoWS(t *testing.T) {
	const duration, initialHosting = 8 * time.Second, int64(17)
	store, endpoint, route, _, _ := startDungeonServer(t, true, func(cfg *devconfig.Config) {
		cfg.Fixture.DungeonReleases[0].DurationMS = duration.Milliseconds()
		monster := &cfg.Fixture.ChallengeMonsters[0]
		monster.HP, monster.AttackMin, monster.AttackMax, monster.AttackMS = 100_000, 0, 0, 60_000
	})
	recruit := newDungeonRecruit(t, endpoint, route, 0)
	roleID := domain.Hex(recruit.summary.Key.RoleId)
	// 种下已合法持有时长的前态，实际托管计费仍从原连接断开后开始。
	_, err := store.DB.Collection("roles").UpdateByID(t.Context(), roleID, bson.M{"$set": bson.M{"hosting_seconds": initialHosting}})
	must(t, err)
	c := recruit.attach(t)
	c.auto(true, 1)
	originalMap, originalBinding := c.entry.RoomId, c.binding
	originalLife := dungeonOwnBody(c).Life
	original := proto.Clone(c.entry).(*pb.EnterLineResult)
	challenge := startSingleDungeonForFinish(t, c)
	confirmDungeonBattleArrival(c, challenge, original)
	waitDungeonBattleState(c, challenge, 1, 2)
	active := dungeonFinishRecord(t, store, challenge)
	before, err := store.Role(t.Context(), roleID)
	must(t, err)
	if before.HostingSeconds != initialHosting || active.Outcome != persistence.DungeonActive ||
		before.Dungeon == nil || before.Dungeon.ActiveChallengeID != challenge {
		t.Fatal("普通在线时长提前扣减，或原副本尚未开始", before.HostingSeconds, active)
	}
	disconnected := time.Now()
	c.close()

	// 不发重连票，不从客户端制造结束；等待同一副本自然到期以及独立普通托管的真实写入。
	for time.Now().Before(active.ExpiresAt.Add(5 * time.Second)) {
		if dungeonFinishRecord(t, store, challenge).Outcome != persistence.DungeonActive {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	closed := dungeonFinishRecord(t, store, challenge)
	if closed.Outcome != persistence.DungeonFailed || closed.Refunded || !closed.FinishedAt.Equal(active.ExpiresAt) {
		t.Fatal("普通离线托管改变了特殊挑战时限或无故退款", active, closed)
	}
	resumed := recruit.attach(t)
	maximumElapsed := int64(time.Since(disconnected)/time.Second) + 1
	resumedBody := dungeonOwnBody(resumed)
	if resumed.entry.RoomId != originalMap || resumed.entry.SceneKind != devconfig.SceneKindPersonal ||
		resumed.entry.ServerTransitionId != "" || resumed.binding <= originalBinding || resumedBody == nil || resumedBody.Life != originalLife {
		t.Fatal("原副本终态失去独立普通房间或让新票续进已结束场", original, resumed.entry)
	}
	after, err := store.Role(t.Context(), roleID)
	must(t, err)
	if after.Dungeon == nil || after.Dungeon.Entries != 1 || after.Dungeon.ActiveChallengeID != "" ||
		after.EconomicSeq != before.EconomicSeq || after.SourceSeq <= before.SourceSeq {
		t.Fatal("普通托管没有真实收益水位，或副本后处理错误修改角色账", after.Dungeon, before, after)
	}
	type hostingDebit struct {
		Seconds int64  `bson:"host_seconds"`    // 本次确认扣减的原普通托管整秒。
		Session string `bson:"hosting_session"` // 由这次普通连接断开建立的唯一计费会话。
	}
	cur, err := store.DB.Collection("audit_outbox").Find(t.Context(), bson.M{"kind": "settlement", "role_id": roleID, "host_seconds": bson.M{"$gt": 0}})
	must(t, err)
	var debits []hostingDebit
	must(t, cur.All(t.Context(), &debits))
	sessions := make(map[string]bool)
	var charged int64
	for _, debit := range debits {
		sessions[debit.Session] = true
		charged += debit.Seconds
	}
	if len(sessions) != 1 || sessions[""] || charged != before.HostingSeconds-after.HostingSeconds || charged < 3 || charged > maximumElapsed {
		t.Fatal("普通托管扣时与持久余额或唯一断线会话不一致", charged, before.HostingSeconds, after.HostingSeconds, debits)
	}
	assertDungeonFinishAudits(t, store, closed, roleID)
	t.Logf("同一离线角色的普通托管结清 %d 秒，原讨伐仍按自己的 %s 截止失败，两个账本无多扣", charged, duration)
}

// startSingleDungeonForFinish 真实创建队伍并锁定相同招募修订，不构造战斗或写预约。
func startSingleDungeonForFinish(t *testing.T, c *client) string {
	t.Helper()
	partyID := domain.Bytes(domain.ID())
	party := requireDungeonParty(t, dungeonPartyRPC(c, &pb.DungeonPartyRequest{Action: 2, PartyId: partyID, TemplateId: 3001}), partyID, 1)
	started := dungeonPartyRPC(c, &pb.DungeonPartyRequest{Action: 7, PartyId: partyID, ExpectedRevision: party.Revision})
	if started.ErrorCode != 0 || len(started.ChallengeId) != 16 || started.Starting {
		t.Fatal("真实开战请求没有返回已提交的原场编号", started)
	}
	return domain.Hex(started.ChallengeId)
}

// dungeonFinishRecord 读取测试场次的原持久身份和终态，用于与网络观察对拍。
func dungeonFinishRecord(t *testing.T, store *persistence.Store, challenge string) persistence.DungeonChallenge {
	t.Helper()
	var result persistence.DungeonChallenge
	must(t, store.DB.Collection("dungeon_challenges").FindOne(t.Context(), bson.M{"_id": challenge}).Decode(&result))
	return result
}

// assertDungeonFinishAudits 核验唯一扣次和唯一终态审计都指向原角色、原场、原运营日。
func assertDungeonFinishAudits(t *testing.T, store *persistence.Store, result persistence.DungeonChallenge, roleID string) {
	t.Helper()
	filter := bson.M{"challenge_id": result.ChallengeID, "role_id": roleID, "kind": bson.M{"$in": []string{"dungeon_begin", "dungeon_finish"}}}
	count, err := store.DB.Collection("audit_outbox").CountDocuments(t.Context(), filter)
	must(t, err)
	if count != 2 {
		t.Fatal("原扣次和终态没有各保留一条审计", count, result)
	}
	filter["kind"], filter["reservation_id"], filter["entry.entry_day"], filter["refunded"] =
		"dungeon_finish", result.ReservationID, result.Members[0].EntryDay, result.Refunded
	if err := store.DB.Collection("audit_outbox").FindOne(t.Context(), filter).Err(); err != nil {
		t.Fatal("终态审计没有核对原预约、原运营日和退款性质", err, result)
	}
}
