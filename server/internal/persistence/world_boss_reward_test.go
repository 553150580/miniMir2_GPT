// 本文件验证世界首领附件的来源归属，并使用独立 MongoDB 对拍结算、领取和重连的永久事实。
package persistence

import (
	"context"
	"os"
	"reflect"
	"regexp"
	"slices"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	p "minimir/internal/protocol"
)

// TestWorldBossRewardEligibility 核对不同场次终态、席位和卡时限在不可变附件中的分离。
func TestWorldBossRewardEligibility(t *testing.T) {
	start := worldBossTime(time.Now().Add(-time.Minute))
	spec := WorldBossSpec{RunID: 1, TemplateID: 2, RuleVersion: 30, ConfigHash: "隔离奖表", OpensAt: start, ClosesAt: start.Add(time.Minute), HPMax: 100}
	spec.OccurrenceID = WorldBossOccurrenceID(spec.RunID, spec.TemplateID, start)
	finish := WorldBossFinish{Outcome: WorldBossKilled, FinishedAt: start.Add(time.Second)}
	for index, row := range []struct {
		damage   int64
		presence WorldBossPresence
		until    time.Time
	}{{40, WorldBossPresenceLeft, finish.FinishedAt.Add(time.Minute)}, {30, WorldBossPresenceOffline, finish.FinishedAt.Add(time.Millisecond)},
		{20, WorldBossPresenceOnline, finish.FinishedAt}, {9, WorldBossPresenceDefeated, time.Time{}}, {1, WorldBossPresenceOnline, time.Time{}}} {
		rowFact := testWorldBossContribution(byte(index+1), row.damage, start)
		rowFact.RewardEvidence = &WorldBossRewardEvidence{Origin: 2, Presence: row.presence, PrivilegeUntil: row.until}
		finish.Contributions = append(finish.Contributions, rowFact)
	}
	fixture := devconfig.Fixture{Items: []devconfig.Item{{ID: 35}, {ID: 36}, {ID: 218}}}
	rules := &devconfig.WorldBossRewards{MinimumDamage: 5, AllowDisconnected: true, Participation: []devconfig.ChallengeRewardItem{{ItemID: 35, Quantity: 2}},
		Ranks:                []devconfig.WorldBossRankReward{{FirstRank: 1, LastRank: 1, Items: []devconfig.ChallengeRewardItem{{ItemID: 218, Quantity: 1}}}},
		TimeoutParticipation: []devconfig.ChallengeRewardItem{{ItemID: 36, Quantity: 1}}}
	frozen, digest, err := normalizeWorldBossFinish(spec, finish)
	if err != nil || frozen.Contributions[0].RewardEvidence == finish.Contributions[0].RewardEvidence {
		t.Fatal("原奖励证据应复制且成功冻结", err)
	}
	rows, err := freezeWorldBossRewards(&fixture, spec, frozen, rules)
	if err != nil || len(rows) != 2 || rows[0].SourceRank != 2 || rows[1].SourceRank != 3 || len(rows[0].Items) != 1 || len(rows[1].Items) != 1 ||
		rows[0].Items[0].Bound || !rows[1].Items[0].Bound {
		t.Fatal("席位、原名次不递补或到期等号的绑定不一致", rows, err)
	}
	slices.Reverse(finish.Contributions)
	frozen, repeatDigest, err := normalizeWorldBossFinish(spec, finish)
	repeated, awardErr := freezeWorldBossRewards(&fixture, spec, frozen, rules)
	if err != nil || awardErr != nil || digest != repeatDigest || !reflect.DeepEqual(repeated, rows) {
		t.Fatal("提交时换序改变了原奖励、物品编号或结果摘要", err, awardErr)
	}
	frozen.Outcome, frozen.FinishedAt = WorldBossExpired, spec.ClosesAt
	timeout, err := freezeWorldBossRewards(&fixture, spec, frozen, rules)
	if err != nil || len(timeout) != 2 || timeout[0].SourceKind != worldBossTimeoutReward || timeout[0].Items[0].ItemID != 36 ||
		!timeout[0].Items[0].Bound || timeout[0].EarnedAt != spec.ClosesAt {
		t.Fatal("超时应只按独立配置发放绑定附件", timeout, err)
	}
	frozen.Outcome = WorldBossInterrupted
	if interrupted, err := freezeWorldBossRewards(&fixture, spec, frozen, rules); err != nil || len(interrupted) != 0 {
		t.Fatal("中断生成了正式击杀附件", interrupted, err)
	}
}

// TestWorldBossRewardDurableMongo 核对同场落库、跨角色围栏和接管后原操作重放。
func TestWorldBossRewardDurableMongo(t *testing.T) {
	uri := os.Getenv("MINIMIR_INTEGRATION_TEST_MONGO_URI")
	if os.Getenv("MINIMIR_MONGO_E2E") != "1" || uri == "" {
		t.Skip("只允许在当前任务独立 MongoDB 副本集运行")
	}
	if !regexp.MustCompile(`^mongodb://127\.0\.0\.1:[1-9][0-9]{3,4}/\?replicaSet=minimirLoad_[a-f0-9]{32}&directConnection=true$`).MatchString(uri) {
		t.Fatal("缺少隔离控制器的专属副本集地址")
	}
	require := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	ctx, stop := context.WithTimeout(t.Context(), 30*time.Second)
	defer stop()
	cfg, err := devconfig.Load("../../../conf/dev/server.json")
	require(err)
	cfg.MongoURI, cfg.Database = uri, "minimir_dev_game_test_world_reward_"+domain.ID()[:14]
	opensAt := worldBossTime(time.Now().Add(-time.Second))
	local := opensAt.In(time.FixedZone("Asia/Shanghai", 8*60*60))
	day := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, local.Location())
	cfg.Fixture.WorldBossVariants = nil
	template := cfg.Fixture.WorldBoss
	template.DailyOpenOffsets, template.Window = []time.Duration{local.Sub(day)}, 2*time.Minute
	template.Rewards = &devconfig.WorldBossRewards{MinimumDamage: 1, AllowDisconnected: true,
		Participation: []devconfig.ChallengeRewardItem{{ItemID: 35, Quantity: 2}},
		Ranks:         []devconfig.WorldBossRankReward{{FirstRank: 1, LastRank: 1, Items: []devconfig.ChallengeRewardItem{{ItemID: 218, Quantity: 1}}}}}
	account, err := Open(ctx, cfg)
	require(err)
	t.Cleanup(func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		defer account.Close(cleanup)
		if err := account.DB.Drop(cleanup); err != nil {
			t.Error(err)
		}
	})
	require(account.Singleton(ctx, "account"))
	open := func(settings devconfig.Config) *Store {
		t.Helper()
		game, err := Open(ctx, settings)
		require(err)
		t.Cleanup(func() { game.Close(context.Background()) })
		lease, err := account.ApproveGame(ctx, settings.NodeID, game.Instance, settings.RunID)
		require(err)
		game.Epoch = lease.Epoch
		return game
	}
	game := open(cfg)
	register := func(label string) (Auth, *pb.RoleKey) {
		t.Helper()
		login, err := account.Login(ctx, "bossreward_"+domain.ID()[:8], "test_123456", true)
		require(err)
		customer, err := account.Authenticate(ctx, login.AccountToken)
		require(err)
		created, err := account.CreateRole(ctx, customer, &pb.CreateRoleRequest{OriginServerId: cfg.OriginID, Slot: 1, Job: 0, Name: label, OpId: domain.Bytes(domain.ID())})
		require(err)
		return customer, created.Key
	}
	attach := func(owner *Store, customer Auth, key *pb.RoleKey) (domain.Role, Auth) {
		t.Helper()
		ticket, err := account.IssueTicket(ctx, customer, key)
		require(err)
		role, auth, err := owner.Enter(ctx, ticket.Ticket)
		require(err)
		return role, auth
	}
	firstAccount, firstKey := register("首领奖励甲")
	lastAccount, lastKey := register("首领奖励乙")
	first, firstAuth := attach(game, firstAccount, firstKey)
	last, lastAuth := attach(game, lastAccount, lastKey)
	spec := WorldBossSpec{RunID: cfg.RunID, TemplateID: template.TemplateID, Execution: 1, RuleVersion: cfg.Fixture.RuleVersion,
		ConfigHash: cfg.Fixture.Hash, OpensAt: opensAt, ClosesAt: opensAt.Add(template.Window), HPMax: template.Monster.HP}
	spec.OccurrenceID = WorldBossOccurrenceID(spec.RunID, spec.TemplateID, opensAt)
	_, err = game.BeginWorldBoss(ctx, spec)
	require(err)
	finish := WorldBossFinish{Outcome: WorldBossKilled, FinishedAt: worldBossTime(time.Now())}
	for index, role := range []domain.Role{first, last} {
		row := testWorldBossContribution(byte(index+1), 1, opensAt)
		row.RoleID, row.DisplayName = role.ID, role.Name
		row.RewardEvidence = &WorldBossRewardEvidence{Origin: role.Origin, Presence: WorldBossPresenceOnline, PrivilegeUntil: finish.FinishedAt}
		if index == 0 {
			row.Damage, row.RewardEvidence.Presence = spec.HPMax-1, WorldBossPresenceOffline
			row.RewardEvidence.PrivilegeUntil = finish.FinishedAt.Add(time.Millisecond)
		}
		finish.Contributions = append(finish.Contributions, row)
	}
	rows, err := freezeWorldBossRewards(&cfg.Fixture, spec, finish, template.Rewards)
	require(err)
	if len(rows) != 2 || rows[0].Items[0].Bound || !rows[1].Items[0].Bound {
		t.Fatal("原击杀权益证据有误", rows)
	}
	collection := account.DB.Collection(worldBossRewardCollection)
	_, err = collection.InsertOne(ctx, bson.M{"_id": rows[1].ID})
	require(err)
	if result, err := game.FinishWorldBoss(ctx, spec, finish); err == nil || result != nil {
		t.Fatal("部分附件冲突仍宣布首领结果成功", result, err)
	}
	active, err := game.readWorldBoss(ctx, bson.M{"_id": worldBossKey(spec.RunID, spec.OccurrenceID)})
	require(err)
	count, err := collection.CountDocuments(ctx, bson.M{"_id": rows[0].ID})
	require(err)
	if active.Outcome != WorldBossActive || count != 0 {
		t.Fatal("首领结果或附件部分写入未回滚", active.Outcome, count)
	}
	_, err = collection.DeleteOne(ctx, bson.M{"_id": rows[1].ID})
	require(err)
	_, err = game.FinishWorldBoss(ctx, spec, finish)
	require(err)
	_, err = game.FinishWorldBoss(ctx, spec, finish)
	require(err)
	count, err = collection.CountDocuments(ctx, bson.M{})
	require(err)
	if count != 2 {
		t.Fatal("同场重放重复生成附件", count)
	}

	// 缩到恰好满包是本测试实例的前态。真实领取和所有账本写均走正常资产事务。
	setCapacity := func(capacity int) {
		t.Helper()
		_, err := account.DB.Collection("roles").UpdateOne(ctx, bson.M{"_id": first.ID}, bson.M{"$set": bson.M{"capacity": capacity}})
		require(err)
	}
	setCapacity(len(first.Items))
	request := func(role domain.Role, reward string) AssetRequest {
		id := domain.ID()
		return AssetRequest{Kind: AssetChallengeRewardClaim, ChallengeRewardID: reward, Fingerprint: Hash([]byte(id + reward)),
			Context: &pb.OperationContext{OpId: domain.Bytes(id), CommandSeq: uint64(role.EconomicSeq + 1), ExpectedRevision: uint64(role.Revision), ExpectedRuleVersion: cfg.Fixture.RuleVersion}}
	}
	initial := first
	_, denied, foreign, err := game.AssetWithReceipt(ctx, lastAuth, request(last, rows[0].ID))
	require(err)
	if denied.ErrorCode != p.CodeOperationConflict || foreign.ChallengeReward != nil {
		t.Fatal("其它角色取得原附件", denied, foreign)
	}
	full := request(first, rows[0].ID)
	first, denied, booked, err := game.AssetWithReceipt(ctx, firstAuth, full)
	require(err)
	if denied.ErrorCode != p.CodeEquipmentBagFull || booked.ChallengeReward != nil || !slices.Equal(first.Items, initial.Items) || len(first.PendingChallengeRewards) != 0 {
		t.Fatal("满包领取改变了原库存或个人待领", denied, booked)
	}
	require(game.Release(ctx, "game"))
	coldSettings := cfg
	coldTemplate := *template
	coldTemplate.Rewards, coldSettings.Fixture.WorldBoss = nil, &coldTemplate
	cold := open(coldSettings)
	setCapacity(len(first.Items) + 2)
	first, firstAuth = attach(cold, firstAccount, firstKey)
	info, err := cold.QueryPersonalBoss(ctx, firstAuth)
	require(err)
	if len(info.WorldPendingRewards) != 1 || !reflect.DeepEqual(info.WorldPendingRewards[0], rows[0].ChallengePendingReward) || info.WorldPendingRewardsMore {
		t.Fatal("当前奖表关闭或冷重启影响了原附件", info.WorldPendingRewards)
	}
	_, denied, _, err = cold.AssetWithReceipt(ctx, firstAuth, full)
	require(err)
	if denied.ErrorCode != p.CodeEquipmentBagFull {
		t.Fatal("旧满包操作重放被改判", denied)
	}
	claim := request(first, rows[0].ID)
	claimed, result, receipt, err := cold.AssetWithReceipt(ctx, firstAuth, claim)
	require(err)
	if result.ErrorCode != 0 || receipt.ChallengeReward == nil || !reflect.DeepEqual(*receipt.ChallengeReward, rows[0].ChallengePendingReward) ||
		!slices.Equal(claimed.Items[len(initial.Items):], rows[0].Items) || claimed.SourceSeq != initial.SourceSeq || claimed.Wallet != initial.Wallet {
		t.Fatal("附件没有原样入包或影响了钱包及来源流水", result, receipt.ChallengeReward)
	}
	first, firstAuth = attach(cold, firstAccount, firstKey)
	_, result, replay, err := cold.AssetWithReceipt(ctx, firstAuth, claim)
	require(err)
	if result.ErrorCode != 0 || replay.ID != receipt.ID || !reflect.DeepEqual(replay.ChallengeReward, receipt.ChallengeReward) {
		t.Fatal("健康重连后原操作没有重放原回执", result, replay)
	}
	_, result, replay, err = cold.AssetWithReceipt(ctx, firstAuth, request(first, rows[0].ID))
	require(err)
	if result.ErrorCode != p.CodeOperationConflict || replay.ChallengeReward != nil {
		t.Fatal("换新操作后重复领取成功", result, replay)
	}
	var audit struct {
		Reward *domain.ChallengePendingReward `bson:"challenge_reward_claim"`
	}
	require(account.DB.Collection("audit_outbox").FindOne(ctx, bson.M{"_id": receipt.ID}).Decode(&audit))
	if !reflect.DeepEqual(audit.Reward, receipt.ChallengeReward) {
		t.Fatal("永久审计与原领取回执不相同", audit.Reward, receipt.ChallengeReward)
	}
}
