// 本文件用独立 MongoDB 和真实 WebSocket 验收托管卡后台挑战及普通地图的并行结算。
package integration

import (
	"context"
	"errors"
	"slices"
	"strconv"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"google.golang.org/protobuf/proto"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
	"minimir/internal/wire"
)

// TestPersonalBossAutoHostingCardMongoWS 核对报名资格、普通地图输入与重连、在线和离线的真实胜利及唯一收益。
func TestPersonalBossAutoHostingCardMongoWS(t *testing.T) {
	cfg := m1FieldConfig(t)
	fixture := &cfg.Fixture
	if fixture.HostingBenefits == nil {
		fixture.HostingBenefits = &devconfig.HostingBenefits{FreeDailySeconds: 21600, CardDailySeconds: 43200}
	}
	fixture.HostingBenefits.AutoPersonalBoss = true
	fixture.PersonalBossVariants = nil
	// 普通击杀实际累计经验。隔离一级升级门槛，避免基线职业在验收期间升级而正当失去原通关资格。
	fixture.Progression.ExpPerLevel[0] = max(fixture.Progression.ExpPerLevel[0], 1000)
	for i := range fixture.Monsters {
		ordinary := &fixture.Monsters[i]
		ordinary.HP, ordinary.MoveMS, ordinary.Agility, ordinary.DefenceMin, ordinary.DefenceMax = 1, 100, 0, 0, 0
		ordinary.AttackMin, ordinary.AttackMax, ordinary.AttackMS = 0, 0, 4000
		ordinary.Exp, ordinary.GoldMin, ordinary.GoldMax, ordinary.RespawnMS = 1, 0, 0, 300
	}
	for i := range fixture.Players {
		if fixture.Players[i].JobID == 0 {
			fixture.Players[i].Accuracy, fixture.Players[i].AttackMS = 255, 270
		}
	}
	// 保留真正格子战斗，只缩短隔离模板的等待，首领需多次攻击以便观察其启动和重连。
	monster := fixture.Monsters[0]
	monster.ID, monster.HP, monster.Exp, monster.RespawnMS = 80_992, 24, 0, 0
	boss := &devconfig.PersonalBossConfig{TemplateID: 1, DisplayName: "隔离托管首领", MinimumLevel: 1,
		Map: devconfig.MapTemplate{ID: 90_082, Width: 13, Height: 13, SceneKind: devconfig.SceneKindPersonalBoss,
			PlayerSpawn: devconfig.Point{X: 5, Y: 5}, MonsterSpawns: []devconfig.Point{{X: 6, Y: 5}}},
		Monster: monster, Duration: 22 * time.Second, SuccessCooldown: 2 * time.Second, RetryCooldown: time.Second,
		DailySuccessLimit: 5, PrivilegeSuccessBonus: 1, HistoryLimit: 20, KillRewardGold: 73,
		ItemReward: &devconfig.ChallengeItemReward{PendingLimit: 5, Items: []devconfig.ChallengeRewardItem{{ItemID: 35, Quantity: 2}}}}
	fixture.PersonalBoss = boss
	fixture.ChallengeMonsters = append(fixture.ChallengeMonsters, monster)
	if !fixture.ValidPersonalBoss() {
		t.Fatal("隔离托管首领模板未通过原格子战斗配置校验")
	}
	cfg, ctx, store := m1FieldServices(t, cfg)
	challenges := store.DB.Collection("personal_boss_challenges")
	audit := store.DB.Collection("audit_outbox")

	// attach 只种下卡与时长的测试前态，资格、装备、托管和战斗均由真实入口推进。
	attach := func(t *testing.T, activeCard bool, hostingSeconds int64) (*client, string, func() string) {
		t.Helper()
		endpoint, token, key := m1FieldRole(t, cfg, 0)
		roleID := domain.Hex(key.RoleId)
		seed := bson.M{"hosting_seconds": hostingSeconds}
		if activeCard {
			seed["hosting_card_until"] = time.Now().Add(time.Hour)
		}
		update, err := store.DB.Collection("roles").UpdateByID(ctx, roleID, bson.M{"$set": seed})
		must(t, err)
		if update.MatchedCount != 1 {
			t.Fatal("未找到本用例的测试角色", roleID)
		}
		newTicket := func() string { return issue(t, endpoint, token, key) }
		c := join(t, cfg.GameAddress, newTicket())
		t.Cleanup(c.close)
		c.auto(false, 1)
		if c.entry.RoomId != cfg.Fixture.Map.ID || c.entry.SceneKind != devconfig.SceneKindPersonal {
			t.Fatal("新角未从正常流程进入普通个人图", c.entry)
		}
		equipPersonalBossAutoFixture(t, c, &cfg.Fixture, 218)
		return c, roleID, newTicket
	}

	for _, tc := range []struct {
		name        string // 资格隔离条件的描述。
		activeCard  bool   // 是否种下生效的角色托管卡。
		manual      bool   // 是否实际通关本测试档位。
		changeKit   bool   // 是否在通关后通过客户端切换装备。
		disableAuto bool   // 是否保持普通地图的自动战斗关闭。
		offline     bool   // 是否在原冷却开始后断开无时长的客户端。
	}{
		{name: "有卡未通关", activeCard: true},
		{name: "通关但无卡", manual: true},
		{name: "通关后换装备", activeCard: true, manual: true, changeKit: true},
		{name: "通关但关闭普通自动战斗", activeCard: true, manual: true, disableAuto: true},
		{name: "合格但离线无时长", activeCard: true, manual: true, offline: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, roleID, _ := attach(t, tc.activeCard, 0)
			// 解析登录和真实建角按同一端点的账号限流串行，实际战斗和资格观察仍可并行。
			t.Parallel()
			var manual persistence.PersonalBossChallenge
			if tc.manual {
				manual = clearManualPersonalBossAutoFixture(t, ctx, store, c, boss, roleID)
			}
			if tc.changeKit {
				equipPersonalBossAutoFixture(t, c, &cfg.Fixture, 193)
			}
			c.auto(!tc.disableAuto, 1)
			if tc.offline {
				c.close()
			}
			watchUntil := time.Now().Add(3 * time.Second)
			if tc.manual && manual.AvailableAt.Add(2300*time.Millisecond).After(watchUntil) {
				watchUntil = manual.AvailableAt.Add(2300 * time.Millisecond)
			}
			for time.Now().Before(watchUntil) {
				err := challenges.FindOne(ctx, bson.M{"role_id": roleID, "automatic": true}).Err()
				if !errors.Is(err, mongo.ErrNoDocuments) {
					t.Fatalf("不符合现有资格的角色写入自动预约（查询错误：%v）", err)
				}
				time.Sleep(80 * time.Millisecond)
			}
			if !tc.offline {
				c.auto(false, 1)
			}
			actual, err := challenges.CountDocuments(ctx, bson.M{"role_id": roleID})
			must(t, err)
			if (!tc.manual && actual != 0) || (tc.manual && actual != 1) {
				t.Fatalf("不符合资格的普通图产生额外场次：%d", actual)
			}
		})
	}

	t.Run("在线后台重连后离线继续", func(t *testing.T) {
		const initialHosting = 45
		c, roleID, newTicket := attach(t, true, initialHosting)
		before, err := store.Role(ctx, roleID)
		must(t, err)
		manual := clearManualPersonalBossAutoFixture(t, ctx, store, c, boss, roleID)
		paid, err := store.Role(ctx, roleID)
		must(t, err)
		if paid.Wallet.BoundGold != before.Wallet.BoundGold+boss.KillRewardGold || paid.HostingSeconds != initialHosting {
			t.Fatal("真实手动胜利未入账，或在线提前扣除托管时长", before.Wallet, paid.Wallet, paid.HostingSeconds)
		}
		ordinary := protectPersonalBossAutoOrdinary(t, c, cfg.Fixture.Map.ID, manual.ChallengeID)
		c.auto(true, 1)
		automaticFilter := bson.M{"role_id": roleID, "automatic": true, "template_id": boss.TemplateID}
		running := waitPersonalBossAutoRecord(t, ctx, challenges, automaticFilter)
		if running.Outcome != persistence.PersonalBossActive || running.ChallengeID == manual.ChallengeID ||
			running.StartedAt.Before(manual.AvailableAt) || running.Qualification != manual.Qualification {
			t.Fatal("后台没有在原冷却后以原配置重新预约这一档", manual, running)
		}
		// 后台独立首领在执行期间仍可收取并确认原普通地图的挂机输入。
		c.auto(true, 2)
		travelUntilWithin(c, 3*time.Second, func(frame wire.Frame) bool {
			return frame.ID == p.MsgS2CSceneBaseline && travelCount(c.scene, 3) == 2
		})
		if !proto.Equal(c.entry, ordinary) {
			t.Fatal("后台预约把普通场景、基线或转场身份改了", c.entry)
		}
		oldBinding := c.binding
		c.close()
		resumed := join(t, cfg.GameAddress, newTicket())
		t.Cleanup(resumed.close)
		protectPersonalBossAutoOrdinary(t, resumed, cfg.Fixture.Map.ID, "")
		if resumed.binding <= oldBinding {
			t.Fatal("测试未取得全新的角色 WebSocket 会话", oldBinding, resumed.binding)
		}
		c = resumed
		c.auto(true, 2)
		count, err := challenges.CountDocuments(ctx, automaticFilter)
		must(t, err)
		if count != 1 {
			t.Fatalf("健康新票在后台另起原档位场次：%d", count)
		}
		terminal := successfulPersonalBossAutoRecord(t, ctx, challenges, running, boss, 2)
		assertOrdinaryPersonalBossAutoReward(t, ctx, audit, roleID, cfg.Fixture.Map.ID, terminal, false)
		withOnline, err := store.Role(ctx, roleID)
		must(t, err)
		if withOnline.Wallet.BoundGold != before.Wallet.BoundGold+2*boss.KillRewardGold ||
			withOnline.Wallet.Gold != before.Wallet.Gold || withOnline.PersonalBoss == nil || withOnline.PersonalBoss.Successes != 2 {
			t.Fatal("后台胜利未给原角色准确入账一次", withOnline.Wallet, withOnline.PersonalBoss)
		}

		// 保持普通自动开启后断开真实连接，整场离线胜利无需新入场或挑战 ACK。
		requireChallengeAuto(c, true)
		disconnected := time.Now().UTC().Truncate(time.Millisecond)
		c.close()
		defer func() {
			if !t.Failed() {
				return
			}
			current, readErr := store.Role(ctx, roleID)
			n, countErr := audit.CountDocuments(ctx, bson.M{"role_id": roleID, "kind": "settlement", "event_at": bson.M{"$gte": disconnected},
				"hosting_session": bson.M{"$type": "string", "$ne": ""}})
			t.Logf("离线排查：等级 %d→%d，托管秒数 %d→%d，普通离线结算 %d，成功次数 %v，角色/审计读取 %v/%v",
				withOnline.Level, current.Level, withOnline.HostingSeconds, current.HostingSeconds, n, current.PersonalBoss, readErr, countErr)
		}()
		offline := waitPersonalBossAutoRecord(t, ctx, challenges, bson.M{"role_id": roleID, "automatic": true,
			"template_id": boss.TemplateID, "_id": bson.M{"$ne": terminal.ChallengeID}})
		if offline.StartedAt.Before(disconnected) || offline.StartedAt.Before(terminal.AvailableAt) {
			t.Fatal("第三场未在原连接断开及冷却后由真实离线托管启动", disconnected, terminal, offline)
		}
		finishedOffline := successfulPersonalBossAutoRecord(t, ctx, challenges, offline, boss, 3)
		assertOrdinaryPersonalBossAutoReward(t, ctx, audit, roleID, cfg.Fixture.Map.ID, finishedOffline, true)
		// 立刻重新接回并关闭原图自动，避免验收读取期间新开下一个自然冷却档。
		finalConn := join(t, cfg.GameAddress, newTicket())
		t.Cleanup(finalConn.close)
		protectPersonalBossAutoOrdinary(t, finalConn, cfg.Fixture.Map.ID, "")
		finalConn.auto(false, 1)
		final, err := store.Role(ctx, roleID)
		must(t, err)
		if final.Wallet.BoundGold != before.Wallet.BoundGold+3*boss.KillRewardGold || final.Wallet.Gold != before.Wallet.Gold ||
			final.HostingSeconds < 0 || final.HostingSeconds >= withOnline.HostingSeconds ||
			final.PersonalBoss == nil || final.PersonalBoss.ActiveChallengeID != "" || final.PersonalBoss.Successes != 3 {
			t.Fatal("离线首领收益、次数或原普通托管扣时错误", final.Wallet, final.HostingSeconds, final.PersonalBoss)
		}
		if final.Level != before.Level || !slices.Equal(final.Skills, before.Skills) || !slices.Equal(final.SkillSlotIDs, before.SkillSlotIDs) {
			t.Fatal("后台场污染了原角色等级、已学技能或技能槽", before.Level, final.Level, before.Skills, final.Skills, before.SkillSlotIDs, final.SkillSlotIDs)
		}
		// 正常容量下，原胜利回执的每一份附件必须在原角色背包出现恰好一次。
		for _, victory := range []persistence.PersonalBossChallenge{manual, terminal, finishedOffline} {
			n := 0
			for _, item := range final.Items {
				if item.SourceKind == "personal_boss" && item.SourceID == victory.ChallengeID {
					n++
					if len(victory.RewardItems) != 1 || item.ID != victory.RewardItems[0].ID || !item.Bound || item.Quantity != 2 {
						t.Fatal("背包中的个人首领物品不对应原场次的确认回执", item, victory)
					}
				}
			}
			if n != 1 || victory.RewardQueued {
				t.Fatalf("正常容量下场次 %s 应给原角色一份实际入包附件，实收 %d", victory.ChallengeID, n)
			}
		}
		for _, phase := range []string{"begin", "finish"} {
			n, err := audit.CountDocuments(ctx, bson.M{"role_id": roleID, "kind": "personal_boss_" + phase})
			must(t, err)
			if n != 3 {
				t.Fatalf("三场首领的 %s 事务审计记录应各一次，实际 %d", phase, n)
			}
		}
		leaked, err := audit.CountDocuments(ctx, bson.M{"role_id": roleID, "kind": "settlement", "settlement.map_id": boss.Map.ID})
		must(t, err)
		if leaked != 0 {
			t.Fatal("首领格子战斗进入了普通挂机击杀账本", leaked)
		}
		t.Logf("手动、在线后台、离线后台各确认一次，角色普通地图=%d，绑定金币增量=%d，托管时长=%d→%d，在线和离线后台均保留普通击杀", cfg.Fixture.Map.ID, final.Wallet.BoundGold-before.Wallet.BoundGold, initialHosting, final.HostingSeconds)
	})
}

// equipPersonalBossAutoFixture 用真实穿戴事务修改对应栏位，并等同一修订的背包确认。
func equipPersonalBossAutoFixture(t *testing.T, c *client, fixture *devconfig.Fixture, itemID uint32) {
	t.Helper()
	item := naturalGrowthItem(c, itemID, "")
	definition, known := fixture.Item(itemID)
	if item == nil || !known || !definition.IsEquipment {
		t.Fatal("首领用例缺少正常建角装备", itemID)
	}
	result := c.asset(p.MsgC2SEquip, &pb.EquipRequest{ItemInstanceId: item.InstanceId, ItemVersion: item.Version,
		Slot: definition.EquipmentSlot, Operation: c.operation()})
	if result.ErrorCode != 0 {
		t.Fatal("隔离角色正常穿戴失败", result)
	}
	c.inventoryAt(result.NewRevision)
}

// clearManualPersonalBossAutoFixture 经真实手动入口通关并核对刚持久化的资格证明。
func clearManualPersonalBossAutoFixture(t *testing.T, ctx context.Context, store *persistence.Store, c *client, boss *devconfig.PersonalBossConfig, roleID string) persistence.PersonalBossChallenge {
	t.Helper()
	ordinaryMap := c.entry.RoomId
	booking, _ := enterReconnectBoss(t, c, boss.TemplateID, true)
	if c.entry.RoomId != boss.Map.ID {
		t.Fatal("真实手动预约未进入指定首领地图", c.entry, booking)
	}
	travelUntilWithin(c, 19*time.Second, func(frame wire.Frame) bool {
		return frame.ID == p.MsgS2CSceneBaseline && c.entry.SceneKind == devconfig.SceneKindPersonal && c.entry.RoomId == ordinaryMap && c.scene.SceneEpoch == c.entry.SceneEpoch
	})
	travelAcknowledge(c, false)
	var done persistence.PersonalBossChallenge
	must(t, store.DB.Collection("personal_boss_challenges").FindOne(ctx, bson.M{"_id": booking.ChallengeId, "role_id": roleID}).Decode(&done))
	if done.Outcome != persistence.PersonalBossDefeated || done.Damage != boss.Monster.HP || done.HPRemaining != 0 ||
		done.Automatic || done.Successes != 1 || done.Qualification == "" {
		t.Fatal("手动场没有真实击杀或没有永久保存战斗条件", done)
	}
	role, err := store.Role(ctx, roleID)
	must(t, err)
	if role.PersonalBoss == nil {
		t.Fatal("角色缺失个人首领资格和次数")
	}
	proof := role.PersonalBoss.Qualifications[strconv.FormatUint(uint64(boss.TemplateID), 10)]
	if proof.ChallengeID != done.ChallengeID || proof.Fingerprint != done.Qualification ||
		proof.RuleVersion != done.RuleVersion || proof.ConfigHash != done.ConfigHash {
		t.Fatal("角色资格与手动通关的原档位或原战斗条件不一致", proof, done)
	}
	return done
}

// protectPersonalBossAutoOrdinary 在每个真实连接内监测自动场不会改变任何原场景或基线。
func protectPersonalBossAutoOrdinary(t *testing.T, c *client, mapID uint64, transition string) *pb.EnterLineResult {
	t.Helper()
	entry := proto.Clone(c.entry).(*pb.EnterLineResult)
	if entry.SceneKind != devconfig.SceneKindPersonal || entry.RoomId != mapID || entry.ServerTransitionId != transition {
		t.Fatal("后台阶段接入收到特殊场景或错误普通地图", entry)
	}
	ordinaryPlayer := proto.Clone(c.player()).(*pb.EntityState)
	c.onFrame = func(conn *client, frame wire.Frame) {
		if frame.Error != 0 {
			return
		}
		switch frame.ID {
		case p.MsgS2CEnterLine:
			if !proto.Equal(conn.entry, entry) {
				t.Fatal("后台战斗在原连接替换了普通入场", conn.entry)
			}
		case p.MsgS2CSceneBaseline:
			if conn.scene.SceneEpoch != entry.SceneEpoch || conn.scene.BaselineId != entry.BaselineId {
				t.Fatal("后台战斗在原连接替换了普通场景基线", conn.scene)
			}
			player := conn.player()
			if player.EntityHandle != ordinaryPlayer.EntityHandle || player.Life != ordinaryPlayer.Life || player.Hp <= 0 ||
				!proto.Equal(player.Position, ordinaryPlayer.Position) {
				t.Fatal("后台场干预了原普通地图的玩家、生命代次或固定挂机位置", ordinaryPlayer, player)
			}
		}
	}
	return entry
}

// waitPersonalBossAutoRecord 等待 Mongo 看到指定场次变化，观察期间不修改首领或角色状态。
func waitPersonalBossAutoRecord(t *testing.T, ctx context.Context, challenges *mongo.Collection, filter bson.M) persistence.PersonalBossChallenge {
	t.Helper()
	deadline, cancel := context.WithTimeout(ctx, 17*time.Second)
	defer cancel()
	poll := time.NewTicker(35 * time.Millisecond)
	defer poll.Stop()
	for {
		var record persistence.PersonalBossChallenge
		err := challenges.FindOne(deadline, filter).Decode(&record)
		if err == nil {
			return record
		}
		if !errors.Is(err, mongo.ErrNoDocuments) && deadline.Err() == nil {
			t.Fatal("读取后台首领永久记录失败", err)
		}
		select {
		case <-deadline.Done():
			t.Fatalf("未收到后台首领的预期永久记录：%v", filter)
		case <-poll.C:
		}
	}
}

// successfulPersonalBossAutoRecord 校验真实后台击杀、冷却、同一角色和唯一一份金币及物品回执。
func successfulPersonalBossAutoRecord(t *testing.T, ctx context.Context, challenges *mongo.Collection,
	begin persistence.PersonalBossChallenge, boss *devconfig.PersonalBossConfig, successes uint32) persistence.PersonalBossChallenge {
	t.Helper()
	finished := waitPersonalBossAutoRecord(t, ctx, challenges, bson.M{"_id": begin.ChallengeID, "outcome": bson.M{"$ne": persistence.PersonalBossActive}})
	if !finished.Automatic || finished.Outcome != persistence.PersonalBossDefeated || finished.RoleID != begin.RoleID || finished.RunID != begin.RunID ||
		finished.TemplateID != boss.TemplateID || finished.Damage != boss.Monster.HP || finished.HPRemaining != 0 ||
		finished.Successes != successes || !finished.AvailableAt.Equal(finished.FinishedAt.Add(boss.SuccessCooldown)) ||
		finished.RewardGold != boss.KillRewardGold || !finished.RewardBound || len(finished.RewardItems) != 1 ||
		finished.RewardItems[0].ItemID != boss.ItemReward.Items[0].ItemID || finished.RewardItems[0].Quantity != boss.ItemReward.Items[0].Quantity {
		t.Fatal("后台没有返回原档位实际击杀的完整奖励、次数、冷却和同角色回执", begin, finished)
	}
	return finished
}

// assertOrdinaryPersonalBossAutoReward 核对后台战斗的精确时间内仍有普通地图的权威击杀经验。
func assertOrdinaryPersonalBossAutoReward(t *testing.T, ctx context.Context, audit *mongo.Collection, roleID string,
	mapID uint64, fight persistence.PersonalBossChallenge, hosting bool) {
	t.Helper()
	filter := bson.M{"role_id": roleID, "kind": "settlement", "settlement.map_id": mapID,
		"event_at": bson.M{"$gte": fight.StartedAt, "$lte": fight.FinishedAt},
		"kills":    bson.M{"$gt": 0}, "experience": bson.M{"$gt": 0}}
	if hosting {
		filter["hosting_session"] = bson.M{"$type": "string", "$ne": ""}
	}
	waitForOrdinaryBossTestKill(t, ctx, audit, filter)
}
