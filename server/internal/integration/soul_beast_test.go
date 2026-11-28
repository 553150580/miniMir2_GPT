// 本文件用真实 MongoDB、账号票据、WebSocket 战斗和回执验证角色独有魂兽。
package integration

import (
	"encoding/json"
	"os"
	"slices"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"google.golang.org/protobuf/proto"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
	"minimir/internal/wire"
)

// TestSoulBeastPersonalMongoWS 用三次实际胜利取得本体及两笔重复碎片，再验证附体、养成和分解。
func TestSoulBeastPersonalMongoWS(t *testing.T) {
	trial := startSoulTrial(t, func(f *devconfig.Fixture) {
		boss := f.PersonalBossByTemplate(3)
		boss.Map.PlayerSpawn, boss.Map.MonsterSpawns = devconfig.Point{X: 5, Y: 5}, []devconfig.Point{{X: 6, Y: 5}}
		boss.Monster.HP, boss.Monster.Defence, boss.Monster.DefenceMin, boss.Monster.DefenceMax = 1, 0, 0, 0
		boss.Monster.Agility, boss.Monster.AttackMin, boss.Monster.AttackMax = 0, 0, 0
		boss.SuccessCooldown, boss.MinimumLevel = time.Millisecond, 1
		if !f.ValidPersonalBoss() || f.PersonalBossByTemplate(3) == nil {
			t.Fatal("隔离骷髅精灵模板无效")
		}
	})
	player := trial.create(t, "魂兽个人", time.Now().Add(time.Hour), false)
	c := trial.attach(t, &player, true)
	original := proto.CloneOf(challengePlayerDeathBody(c).GetCombatAttributes())
	if !proto.Equal(c.inventory.SoulCollection, &pb.SoulCollection{}) {
		t.Fatal("旧角色未初始化为空收藏", c.inventory.SoulCollection)
	}
	for i, delta := range []int64{0, 6, 6} {
		challenge := winSoulPersonal(t, c)
		t.Logf("骷髅精灵第 %d 次真实击杀并返回", i+1)
		info := reconnectBossInfo(t, c)
		if len(info.History) == 0 || info.History[0].ChallengeId != challenge || info.History[0].SoulTypeId != 1 || info.History[0].SoulFragments != delta {
			t.Fatal("胜利历史没有对应当场唯一魂兽奖励", info.History)
		}
		if revision := c.inventory.SoulCollection.GetRevision(); revision != uint64(i+1) {
			t.Fatal("返回原场景首帧后未收到实际到账的收藏", revision, i+1)
		}
	}
	if got := c.inventory.SoulCollection; len(got.Beasts) != 1 || got.Fragments != 12 || got.Beasts[0].Star != 1 {
		t.Fatal("三次胜利必须获得唯一一星和十二片", got)
	}
	attach := soulCommand(c, 1, 1, 0, 0)
	commitSoulTest(t, c, attach, 0)
	t.Log("附体已有持久回执")
	travelUntilWithin(c, 5*time.Second, func(f wire.Frame) bool {
		stats := challengePlayerDeathBody(c).GetCombatAttributes()
		return f.ID == p.MsgS2CSceneBaseline && stats.GetExpandedDamageBp() == original.GetExpandedDamageBp()+25 &&
			stats.GetPhysicalDefenceMax() == original.GetPhysicalDefenceMax()+1 && stats.GetAccuracy() == original.GetAccuracy()+1
	})
	choose := soulCommand(c, 3, 1, 19, 0)
	commitSoulTest(t, c, choose, 0)
	travelUntilWithin(c, 5*time.Second, func(f wire.Frame) bool { return f.ID == p.MsgS2CSceneBaseline })
	stats := challengePlayerDeathBody(c).GetCombatAttributes()
	if stats.GetCriticalBp() != original.GetCriticalBp()+15 || stats.GetAccuracy() != original.GetAccuracy() {
		t.Fatal("改选暴击后未替换同池准确贡献", original, stats)
	}
	stale := soulCommand(c, 4, 1, 0, -6)
	stale.SoulRevision--
	commitSoulTest(t, c, stale, p.CodeSoulChanged)
	commitSoulTest(t, c, soulCommand(c, 4, 1, 0, -5), p.CodeSoulChanged)
	commitSoulTest(t, c, soulCommand(c, 5, 1, 0, 4), p.CodeSoulChanged)
	upgrade := soulCommand(c, 4, 1, 0, -6)
	upgraded := commitSoulTest(t, c, upgrade, 0)
	if upgraded.GetSoul().GetCollection().GetFragments() != 6 || upgraded.Soul.Collection.Beasts[0].Star != 2 {
		t.Fatal("升到二星未消费原确认的六片", upgraded)
	}
	t.Log("二星和实际碎片扣除已确认")
	// 换 WebSocket 连接后携带同一操作编号，必须读取已写入的原回执。
	old := c.binding
	c.close()
	c = trial.attach(t, &player, false)
	replay := c.asset(p.MsgC2SSoulAction, upgrade)
	if c.binding == old || replay.ErrorCode != 0 || !proto.Equal(upgraded.Soul, replay.Soul) {
		t.Fatal("重连后升星原操作被重算或重扣", replay)
	}
	t.Log("重连已取得原升星回执")
	commitSoulTest(t, c, soulCommand(c, 2, 1, 0, 0), 0)
	commitSoulTest(t, c, soulCommand(c, 5, 1, 0, 7), 0)
	travelUntilWithin(c, 5*time.Second, func(f wire.Frame) bool { return f.ID == p.MsgS2CSceneBaseline })
	stats = challengePlayerDeathBody(c).GetCombatAttributes()
	if stats.GetCriticalBp() != original.GetCriticalBp() || stats.GetAccuracy() != original.GetAccuracy() ||
		stats.GetExpandedDamageBp() != original.GetExpandedDamageBp() || stats.GetPhysicalDefenceMax() != original.GetPhysicalDefenceMax() {
		t.Fatal("解除附体并分解后仍有魂兽属性", original, stats)
	}
	commitSoulTest(t, c, soulCommand(c, 6, 1, 0, -12), 0)
	roleID := domain.Hex(player.summary.Key.RoleId)
	stored, err := trial.store.Role(trial.ctx, roleID)
	must(t, err)
	collection := stored.SoulState()
	audits, err := trial.store.DB.Collection("audit_outbox").CountDocuments(trial.ctx,
		bson.M{"role_id": roleID, "kind": persistence.AssetSoulAction, "result_code": 0})
	must(t, err)
	if !collection.Valid() || collection.Revision != 9 || collection.Fragments != 1 || collection.Attached != 0 || len(collection.Beasts) != 1 ||
		collection.Beasts[0] != (domain.SoulBeast{TypeID: 1, Star: 1, Affix: [3]uint32{7, 8, 18}}) || audits != 6 {
		t.Fatal("原回执和重新合成后碎片、词条或成功操作数量不守恒", collection, audits)
	}
	t.Log("三次真实骷髅精灵胜利产出首只一星及十二片，升星、分解及合成后余一片，攻击、防御、命中与暴击快照已核对")
}

// TestSoulBeastWorldMongoWS 用两个实战角色击杀同一只祖玛教主，再以满包、跨角色和重连回执核对附件归属。
func TestSoulBeastWorldMongoWS(t *testing.T) {
	trial := startSoulTrial(t, func(f *devconfig.Fixture) {
		index := slices.IndexFunc(f.WorldBossVariants, func(boss devconfig.WorldBossConfig) bool { return boss.TemplateID == 5 })
		if index < 0 {
			t.Fatal("隔离目录缺少祖玛教主")
		}
		boss := f.WorldBossVariants[index]
		start := time.Now().UTC().Truncate(time.Millisecond).Add(-2 * time.Second)
		setDurableWorldBossFixture(t, f, start, start.Add(10*time.Minute), 5*time.Minute)
		geometry := f.WorldBoss
		boss.MinimumLevel, boss.DailyOpenOffsets, boss.Window, boss.ReturnDelay = 1, geometry.DailyOpenOffsets, geometry.Window, geometry.ReturnDelay
		mapID := boss.Map.ID
		boss.Map = geometry.Map
		boss.Map.ID = mapID
		boss.Monster.HP, boss.Monster.AttackMin, boss.Monster.AttackMax = 4, 0, 0
		boss.Monster.Agility, boss.Monster.Defence, boss.Monster.DefenceMin, boss.Monster.DefenceMax = 0, 0, 0, 0
		boss.Monster.MoveMS, boss.Monster.AttackMS = 4000, 4000
		f.WorldBoss = &boss
		if !f.ValidWorldBoss() {
			t.Fatal("隔离祖玛教主模板无效")
		}
	})
	winner := trial.create(t, "魂兽世界甲", time.Now().Add(time.Hour), false)
	partner := trial.create(t, "魂兽世界乙", time.Now().Add(time.Hour), false)
	first, second := trial.attach(t, &winner, true), trial.attach(t, &partner, true)
	// 木剑通过原资产链穿戴后，在重新附着前将隔离背包缩为当前占用格数。
	second.close()
	partnerID := domain.Hex(partner.summary.Key.RoleId)
	before, err := trial.store.Role(trial.ctx, partnerID)
	must(t, err)
	change, err := trial.store.DB.Collection("roles").UpdateByID(trial.ctx, partnerID, bson.M{"$set": bson.M{"capacity": len(before.Items)}})
	must(t, err)
	if change.ModifiedCount != 1 {
		t.Fatal("隔离角色未建立满包前态")
	}
	second = trial.attach(t, &partner, false)
	open := queryWorldBossTest(t, first)
	if open.TemplateId != 5 {
		t.Fatal("真实开场缺少祖玛教主", open)
	}
	enterWorldBossTest(t, first, open)
	enterWorldBossTest(t, second, open)
	moveChallengePlayerDeathTest(t, second, &pb.GridPoint{X: 6, Y: 4})
	t.Log("两位角色已确认原共享地图与相邻攻击格")
	attackWorldBossTest(first)
	damaged := waitWorldBossTest(t, second, 0, func(state *pb.WorldBossState) bool { return state.Hp < 4 })
	if damaged.State != 3 || damaged.Hp != 1 {
		t.Fatal("首位玩家应扣除三点生命，为第二位贡献保留一点", damaged)
	}
	t.Log("第一位已产生三点实际贡献")
	time.Sleep(time.Duration(challengePlayerDeathBody(second).MoveIntervalMs)*time.Millisecond + 100*time.Millisecond)
	attackWorldBossTest(second)
	sealed := waitWorldBossTest(t, second, 0, func(state *pb.WorldBossState) bool { return state.State == 4 })
	t.Log("两位角色实际击杀已确认")
	if sealed.LatestResult == nil || len(sealed.Ranks) != 2 || sealed.Ranks[1].DisplayName != partner.summary.Name {
		t.Fatal("世界首领两位角色未按真实有效伤害结算", sealed)
	}
	returnFromWorldBossTest(first, open.OccurrenceId)
	returnFromWorldBossTest(second, open.OccurrenceId)
	firstReward, secondReward := soulWorldAttachment(t, first, 1), soulWorldAttachment(t, second, 2)
	foreign := second.asset(p.MsgC2SClaimChallengeReward, &pb.ClaimChallengeReward{RewardId: firstReward.RewardId, Operation: second.operation()})
	if foreign.ErrorCode != p.CodeOperationConflict || foreign.Soul != nil {
		t.Fatal("角色能够领取他人的原世界首领魂兽", foreign)
	}
	second.inventoryAt(foreign.NewRevision)
	claim := &pb.ClaimChallengeReward{RewardId: secondReward.RewardId, Operation: second.operation()}
	accepted := second.asset(p.MsgC2SClaimChallengeReward, claim)
	if accepted.ErrorCode != 0 || accepted.GetSoul().GetAction() != 7 || accepted.GetSoul().GetTypeId() != 3 || accepted.GetSoul().GetFragmentDelta() != 0 {
		t.Fatal("满包的普通参与者未获得祖玛教主一星", accepted)
	}
	second.close()
	second = trial.attach(t, &partner, false)
	replay := second.asset(p.MsgC2SClaimChallengeReward, claim)
	if !proto.Equal(replay.Soul, accepted.Soul) || len(reconnectBossInfo(t, second).WorldPendingRewards) != 0 {
		t.Fatal("重连后没有找回同一奖项或重复挂出原待领", replay)
	}
	roleID := domain.Hex(partner.summary.Key.RoleId)
	role, err := trial.store.Role(trial.ctx, roleID)
	must(t, err)
	if !proto.Equal(role.SoulState().Proto(), accepted.Soul.Collection) || len(role.Items) != role.Capacity || role.Soul.Fragments != 0 ||
		len(role.Soul.Beasts) != 1 || role.Soul.Beasts[0].TypeID != 3 || role.Soul.Revision != 1 {
		t.Fatal("世界奖项未原样到账，或修改了原背包容量", role.Soul, role.Capacity)
	}
	count, err := trial.store.DB.Collection("operations").CountDocuments(trial.ctx,
		bson.M{"role_id": roleID, "kind": persistence.AssetChallengeRewardClaim, "code": 0})
	must(t, err)
	if count != 1 {
		t.Fatal("重连重复领取形成了多笔成功入账", count)
	}
	t.Log("共享祖玛教主实际两位有效参与者均获独立附件，第二名满包可领且重连只入账一次")
}

// startSoulTrial 只读隔离投影并冻结其身份，默认试玩规则由原回归覆盖。
func startSoulTrial(t *testing.T, configure func(*devconfig.Fixture)) worldBossRewardTrial {
	t.Helper()
	ctx, store, endpoint, route := startChallengePlayerDeathServer(t, func(cfg *devconfig.Config) {
		path := os.Getenv("MINIMIR_SOUL_TEST_FIXTURE")
		raw, err := os.ReadFile(path)
		must(t, err)
		var f devconfig.Fixture
		must(t, json.Unmarshal(raw, &f))
		must(t, f.Validate())
		f.Hash = persistence.Hash(raw)
		if f.Souls == nil {
			t.Fatal("隔离候选没有魂兽配置")
		}
		for i := range f.Items {
			if f.Items[i].ID == 218 {
				f.Items[i].AttackMinBonus, f.Items[i].AttackMaxBonus = 3, 3
			}
		}
		if configure != nil {
			configure(&f)
		}
		cfg.Fixture, cfg.FixtureFile = f, path
	})
	return worldBossRewardTrial{ctx: ctx, store: store, endpoint: endpoint, route: route}
}

// winSoulPersonal 以原场景确认和默认自动攻击完成同一场实际挑战。
func winSoulPersonal(t *testing.T, c *client) string {
	t.Helper()
	request := c.send(p.MsgC2SPersonalBossEnter, &pb.PersonalBossEnter{SceneEpoch: c.entry.SceneEpoch, ChallengeId: domain.ID(), TemplateId: 3})
	frame := travelUntilWithin(c, 6*time.Second, func(f wire.Frame) bool { return f.Request == request })
	if frame.ID != p.MsgS2CPersonalBossState || frame.Error != 0 {
		t.Fatalf("骷髅精灵预约失败：%+v", frame)
	}
	var booking, result pb.PersonalBossState
	must(t, proto.Unmarshal(frame.Payload, &booking))
	travelUntilWithin(c, 6*time.Second, func(f wire.Frame) bool {
		return f.ID == p.MsgS2CSceneBaseline && c.entry.SceneKind == devconfig.SceneKindPersonalBoss && c.scene.SceneEpoch == c.entry.SceneEpoch
	})
	travelAcknowledge(c, false)
	t.Logf("已确认原骷髅精灵首基线，角色状态 %v，首领状态 %v", challengePlayerDeathBody(c).Position, travelEntity(c.scene, 3))
	travelUntilWithin(c, 6*time.Second, func(f wire.Frame) bool {
		if f.ID != p.MsgS2CPersonalBossState {
			return false
		}
		must(t, proto.Unmarshal(f.Payload, &result))
		return result.State >= 4
	})
	if result.State != 4 || result.ChallengeId != booking.ChallengeId {
		t.Fatal("原骷髅精灵未在同一真实场次中被击杀", &result)
	}
	returnFromWorldBossTest(c, booking.ChallengeId)
	return booking.ChallengeId
}

// soulCommand 填入原面板所见的魂兽修订和有符号碎片变化。
func soulCommand(c *client, action, kind, affix uint32, delta int64) *pb.SoulActionRequest {
	return &pb.SoulActionRequest{Action: action, TypeId: kind, AffixId: affix, FragmentDelta: delta,
		SoulRevision: c.inventory.GetSoulCollection().GetRevision(), Operation: c.operation()}
}

// commitSoulTest 核对事务回执与同一角色修订推送，失败时不得返回成功收藏。
func commitSoulTest(t *testing.T, c *client, request *pb.SoulActionRequest, code uint32) *pb.OperationResult {
	t.Helper()
	result := c.asset(p.MsgC2SSoulAction, request)
	if result.ErrorCode != code || (result.Soul != nil) != (code == 0) {
		t.Fatal("魂兽操作返回了意外结果", request, result)
	}
	if c.inventory.Revision < result.NewRevision {
		c.inventoryAt(result.NewRevision)
	}
	if result.Soul != nil && (!proto.Equal(result.Soul.Collection, c.inventory.SoulCollection) || result.Soul.FragmentDelta != request.FragmentDelta) {
		t.Fatal("魂兽回执与实际角色修订不同", result, c.inventory.SoulCollection)
	}
	return result
}

// soulWorldAttachment 校验任何已达有效门槛名次的独立魂兽附件。
func soulWorldAttachment(t *testing.T, c *client, rank uint32) *pb.ChallengePendingReward {
	t.Helper()
	rewards := reconnectBossInfo(t, c).WorldPendingRewards
	if len(rewards) != 1 || rewards[0].SourceKind != "world_boss_soul" || rewards[0].SourceRank != rank || rewards[0].SoulTypeId != 3 ||
		rewards[0].SoulDuplicateFragments != 6 || len(rewards[0].Items) != 0 {
		t.Fatal("世界首领未提供该角色的原魂兽附件", rewards)
	}
	return rewards[0]
}
