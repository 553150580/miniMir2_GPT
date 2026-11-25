// 本文件对拍讨伐链独立随机结算、击杀权益和整笔领取的原场证据。
package persistence

import (
	"context"
	"os"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	p "minimir/internal/protocol"
)

// TestDungeonRewardIndependentDraw 固定种子对拍三份独立的选项，连同空包和原击杀绑定。
func TestDungeonRewardIndependentDraw(t *testing.T) {
	start := time.Date(2026, 11, 24, 16, 0, 0, 0, time.UTC)
	finish := DungeonFinish{Outcome: DungeonSucceeded, RewardAt: start.Add(time.Second), FinishedAt: start.Add(3 * time.Second)}
	record := dungeonRecord{RewardSeed: "00112233445566778899aabbccddeeff",
		DungeonChallenge: DungeonChallenge{ChallengeID: strings.Repeat("a", 32), RunID: 11, TemplateID: 7, RuleVersion: 30,
			ConfigHash: strings.Repeat("b", 64), StartedAt: start, ExpiresAt: start.Add(time.Minute)}}
	for _, digit := range []string{"1", "3", "6"} {
		id := strings.Repeat(digit, 32)
		record.Members = append(record.Members, DungeonMember{RoleID: id, Origin: 4})
		finish.RewardEvidence = append(finish.RewardEvidence, DungeonRewardEvidence{RoleID: id, PrivilegeUntil: finish.RewardAt})
	}
	// 甲的权益在击杀后、清完这一段以前到期，仍按击杀时刻判定。
	finish.RewardEvidence[0].PrivilegeUntil = finish.RewardAt.Add(time.Second)
	fixture := &devconfig.Fixture{Items: []devconfig.Item{{ID: 8}, {ID: 9}}}
	killer := devconfig.DungeonRewardItem{ItemID: 8, Quantity: 1, BindPolicy: devconfig.DungeonRewardBindOnKill}
	bound := devconfig.DungeonRewardItem{ItemID: 9, Quantity: 1, BindPolicy: devconfig.DungeonRewardAlwaysBound}
	rules := &devconfig.DungeonRewards{Options: []devconfig.DungeonRewardOption{
		{Weight: 2}, {Weight: 1, Items: []devconfig.DungeonRewardItem{killer}}, {Weight: 4, Items: []devconfig.DungeonRewardItem{killer, bound}},
	}}
	rows, draws, err := freezeDungeonRewards(fixture, record, finish, rules)
	if err != nil {
		t.Fatal(err)
	}
	if len(draws) != 3 || draws[0].Option != 2 || draws[1].Option != 0 || draws[1].RewardID != "" || draws[2].Option != 1 ||
		len(rows) != 2 || rows[0].RoleID != draws[0].RoleID || rows[1].RoleID != draws[2].RoleID {
		t.Fatal("每位原队员应该各抽一次，抽空包也保留原选择", draws, rows)
	}
	if len(rows[0].Items) != 2 || len(rows[1].Items) != 1 || rows[0].Items[0].Bound || !rows[0].Items[1].Bound ||
		!rows[1].Items[0].Bound || !rows[0].EarnedAt.Equal(finish.RewardAt) || rows[0].Items[0].ID == rows[1].Items[0].ID {
		t.Fatal("到期等号、固定绑定或角色独立物品编号不符", rows)
	}
	again, history, err := freezeDungeonRewards(fixture, record, finish, rules)
	if err != nil || !reflect.DeepEqual(rows, again) || !reflect.DeepEqual(draws, history) {
		t.Fatal("同场重复生成了新抽选或新实例", err)
	}
	for _, bad := range []DungeonFinish{
		{RewardAt: start.Add(-time.Millisecond), FinishedAt: finish.FinishedAt, RewardEvidence: finish.RewardEvidence},
		{RewardAt: finish.FinishedAt.Add(time.Millisecond), FinishedAt: finish.FinishedAt, RewardEvidence: finish.RewardEvidence},
		{RewardAt: finish.RewardAt, FinishedAt: finish.FinishedAt, RewardEvidence: finish.RewardEvidence[1:]},
		{RewardAt: finish.RewardAt, FinishedAt: finish.FinishedAt,
			RewardEvidence: []DungeonRewardEvidence{finish.RewardEvidence[0], finish.RewardEvidence[0], finish.RewardEvidence[2]}},
	} {
		if _, _, err := freezeDungeonRewards(fixture, record, bad, rules); Code(err) != p.CodeInvalidArgument {
			t.Fatal("不应接受原场窗口外时间、缺员或重复角色", bad, err)
		}
	}
}

// TestDungeonRewardLegacyFinishDigest 保留已经提交且无奖表的旧场次重放。
func TestDungeonRewardLegacyFinishDigest(t *testing.T) {
	original := struct {
		ChallengeID   string
		ReservationID string
		Execution     uint64
		Outcome       DungeonOutcome
		FinishedAt    time.Time
		Members       []Auth
	}{ChallengeID: strings.Repeat("a", 32), ReservationID: strings.Repeat("b", 32), Execution: 1,
		Outcome: DungeonSucceeded, FinishedAt: time.Date(2026, 11, 24, 16, 0, 0, 0, time.UTC)}
	current := DungeonFinish{ChallengeID: original.ChallengeID, ReservationID: original.ReservationID,
		Execution: original.Execution, Outcome: original.Outcome, FinishedAt: original.FinishedAt}
	old, err := bossDigest(original)
	received, digestErr := bossDigest(current)
	if err != nil || digestErr != nil || old != received {
		t.Fatal("无奖励字段改变了旧终态摘要", old, received, err, digestErr)
	}
}

// TestDungeonRewardDurableMongo 通过真实账号与新票验证原终态、原附件和领取操作原子对账。
func TestDungeonRewardDurableMongo(t *testing.T) {
	uri := os.Getenv("MINIMIR_INTEGRATION_TEST_MONGO_URI")
	if os.Getenv("MINIMIR_MONGO_E2E") != "1" || uri == "" {
		t.Skip("需要本任务专属 MongoDB 副本集")
	}
	if !regexp.MustCompile(`^mongodb://127\.0\.0\.1:[1-9][0-9]{3,4}/\?replicaSet=minimirLoad_[a-f0-9]{32}&directConnection=true$`).MatchString(uri) {
		t.Fatal("联机测试须由任务专属控制器提供回环副本集")
	}
	ctx, stop := context.WithTimeout(t.Context(), 38*time.Second)
	defer stop()
	require := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	cfg, err := devconfig.Load("../../../conf/dev/server.json")
	require(err)
	cfg.MongoURI, cfg.Database = uri, "minimir_dev_game_test_dungeon_reward_"+domain.ID()[:12]
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
	// 隔离模板使用一份确定会抽中的附件，绑定及交易验证不依赖测试随机结果。
	gameCfg := cfg
	gameCfg.Fixture.Dungeons = []devconfig.DungeonConfig{{TemplateID: 1, MinimumLevel: 1, Duration: time.Minute,
		Rewards: &devconfig.DungeonRewards{Options: []devconfig.DungeonRewardOption{{Weight: 1, Items: []devconfig.DungeonRewardItem{
			{ItemID: 218, Quantity: 1, BindPolicy: devconfig.DungeonRewardBindOnKill},
			{ItemID: 35, Quantity: 1, BindPolicy: devconfig.DungeonRewardAlwaysBound},
		}}}}}}
	game := open(gameCfg)
	type player struct {
		account Auth        // 负责签发实际角色票的本测试账号。
		key     *pb.RoleKey // 该账号新建的永久角色键。
		auth    Auth        // 当前角色在本运行服的真实会话。
		role    domain.Role // 最近读取的原角色。
	}
	attach := func(owner *Store, p *player) {
		t.Helper()
		ticket, err := account.IssueTicket(ctx, p.account, p.key)
		require(err)
		p.role, p.auth, err = owner.Enter(ctx, ticket.Ticket)
		require(err)
	}
	players := make(map[string]*player, 3)
	q := DungeonBegin{ChallengeID: domain.ID(), TemplateID: 1, Execution: 1, RuleVersion: cfg.Fixture.RuleVersion,
		ConfigHash: cfg.Fixture.Hash, StartedAt: bossTime(time.Now().Add(-2 * time.Second)), Duration: time.Minute}
	for _, name := range []string{"讨伐奖励甲", "讨伐奖励乙", "讨伐奖励丙"} {
		login, err := account.Login(ctx, "dreward_"+domain.ID()[:8], "test_123456", true)
		require(err)
		player := &player{}
		player.account, err = account.Authenticate(ctx, login.AccountToken)
		require(err)
		role, err := account.CreateRole(ctx, player.account, &pb.CreateRoleRequest{
			OriginServerId: cfg.OriginID, Slot: 1, Name: name, OpId: domain.Bytes(domain.ID())})
		require(err)
		player.key = role.Key
		attach(game, player)
		players[player.role.ID] = player
		q.Members = append(q.Members, DungeonAdmissionMember{Auth: player.auth, ExpectedRevision: player.role.Revision,
			CombatSnapshot: strings.Repeat("a", 64)})
	}
	begin, _, err := game.BeginDungeon(ctx, q)
	require(err)
	proposal := DungeonFinish{ChallengeID: begin.ChallengeID, ReservationID: begin.ReservationID, Execution: begin.Execution,
		Outcome: DungeonSucceeded, RewardAt: begin.StartedAt.Add(time.Second), FinishedAt: begin.StartedAt.Add(1500 * time.Millisecond)}
	for i, member := range begin.Members {
		until := proposal.RewardAt
		if i == 0 {
			until = proposal.RewardAt.Add(time.Millisecond)
		}
		proposal.RewardEvidence = append(proposal.RewardEvidence, DungeonRewardEvidence{RoleID: member.RoleID, PrivilegeUntil: until})
	}
	// 缺少证据应拒绝，无权在失败时添加通关附件。
	for _, invalid := range []DungeonFinish{
		{ChallengeID: begin.ChallengeID, ReservationID: begin.ReservationID, Execution: begin.Execution,
			Outcome: DungeonSucceeded, FinishedAt: proposal.FinishedAt},
		{ChallengeID: begin.ChallengeID, ReservationID: begin.ReservationID, Execution: begin.Execution,
			Outcome: DungeonFailed, FinishedAt: proposal.FinishedAt, RewardAt: proposal.RewardAt, RewardEvidence: proposal.RewardEvidence},
	} {
		if _, _, err := game.FinishOwnedDungeon(ctx, invalid); Code(err) != p.CodeInvalidArgument {
			t.Fatal("非法奖励提案未拒绝", invalid, err)
		}
	}
	record, err := game.readDungeon(ctx, begin.ChallengeID)
	require(err)
	rows, decisions, err := freezeDungeonRewards(&game.Cfg.Fixture, record, proposal, game.Cfg.Fixture.Dungeons[0].Rewards)
	require(err)
	if len(rows) != 3 || len(decisions) != 3 || record.RewardSeed == record.ReservationID || len(record.RewardSeed) != 32 ||
		rows[0].Items[0].Bound || !rows[0].Items[1].Bound || !rows[1].Items[0].Bound {
		t.Fatal("原真人种子、击杀权益及独立附件不符", rows, decisions)
	}
	collection := account.DB.Collection(dungeonRewardCollection)
	// 强制第二份附件写失败，核对首份、扣次、原终态及审计一起回滚。
	_, err = collection.InsertOne(ctx, bson.M{"_id": rows[1].ID})
	require(err)
	if got, _, err := game.FinishOwnedDungeon(ctx, proposal); err == nil || got.ChallengeID != "" {
		t.Fatal("奖励写入失败却确认了场次", got, err)
	}
	active, err := game.readDungeon(ctx, begin.ChallengeID)
	require(err)
	count, err := collection.CountDocuments(ctx, bson.M{"_id": rows[0].ID})
	require(err)
	if active.Outcome != DungeonActive || !active.RewardAt.IsZero() || count != 0 {
		t.Fatal("失败事务留下了部分终态或部分奖励", active, count)
	}
	for _, member := range begin.Members {
		role, err := game.Role(ctx, member.RoleID)
		require(err)
		if role.Revision != players[member.RoleID].role.Revision+1 || role.Dungeon.ActiveChallengeID != begin.ChallengeID {
			t.Fatal("失败事务写完了某个原队员的终态", role.ID, role.Revision, role.Dungeon)
		}
	}
	_, err = collection.DeleteOne(ctx, bson.M{"_id": rows[1].ID})
	require(err)
	// 同一份终态可能有两个任务并发重放，证据换序也不重复生成附件。
	reordered := proposal
	reordered.RewardEvidence = slices.Clone(proposal.RewardEvidence)
	slices.Reverse(reordered.RewardEvidence)
	var group sync.WaitGroup
	for _, attempt := range []DungeonFinish{proposal, reordered} {
		group.Go(func() {
			result, roles, err := game.FinishOwnedDungeon(ctx, attempt)
			if err != nil || result.Outcome != DungeonSucceeded || !result.RewardAt.Equal(proposal.RewardAt) || len(roles) != 3 {
				t.Errorf("原通关确认失败：%v %d %v", result, len(roles), err)
			}
		})
	}
	group.Wait()
	terminal, err := game.readDungeon(ctx, begin.ChallengeID)
	require(err)
	count, err = collection.CountDocuments(ctx, bson.M{"source_id": begin.ChallengeID})
	require(err)
	auditCount, err := account.DB.Collection("audit_outbox").CountDocuments(ctx, bson.M{"challenge_id": begin.ChallengeID, "kind": "dungeon_finish"})
	require(err)
	if !reflect.DeepEqual(terminal.RewardDecisions, decisions) || count != 3 || auditCount != 3 {
		t.Fatal("原多人抽选或审计被重复生成", terminal.RewardDecisions, count, auditCount)
	}
	invalidReplay := proposal
	invalidReplay.RewardAt = proposal.RewardAt.Add(time.Millisecond)
	if _, _, err := game.FinishOwnedDungeon(ctx, invalidReplay); Code(err) != p.CodeOperationConflict {
		t.Fatal("同场已确认击杀时刻被改写", err)
	}

	first, other := players[rows[0].RoleID], players[rows[1].RoleID]
	setCapacity := func(capacity int) {
		t.Helper()
		_, err := account.DB.Collection("roles").UpdateOne(ctx, bson.M{"_id": first.role.ID}, bson.M{"$set": bson.M{"capacity": capacity}})
		require(err)
	}
	request := func(role domain.Role, reward string) AssetRequest {
		id := domain.ID()
		return AssetRequest{Kind: AssetChallengeRewardClaim, ChallengeRewardID: reward, Fingerprint: Hash([]byte(id + reward)),
			Context: &pb.OperationContext{OpId: domain.Bytes(id), CommandSeq: uint64(role.EconomicSeq + 1),
				ExpectedRevision: uint64(role.Revision), ExpectedRuleVersion: cfg.Fixture.RuleVersion}}
	}
	other.role, err = game.Role(ctx, other.role.ID)
	require(err)
	_, foreign, receipt, err := game.AssetWithReceipt(ctx, other.auth, request(other.role, rows[0].ID))
	require(err)
	if foreign.ErrorCode != p.CodeOperationConflict || receipt.ChallengeReward != nil {
		t.Fatal("跨角色可领原奖包", foreign, receipt)
	}
	setCapacity(len(first.role.Items))
	first.role, err = game.Role(ctx, first.role.ID)
	require(err)
	originalItems, originalWallet, originalSeq := slices.Clone(first.role.Items), first.role.Wallet, first.role.SourceSeq
	full := request(first.role, rows[0].ID)
	_, denied, receipt, err := game.AssetWithReceipt(ctx, first.auth, full)
	require(err)
	if denied.ErrorCode != p.CodeEquipmentBagFull || receipt.ChallengeReward != nil {
		t.Fatal("满包情况下产生奖励回执", denied, receipt)
	}
	// 关闭当前奖表并换运行服所有者后，原附件及旧满包的确定回执仍可观察。
	require(game.Release(ctx, "game"))
	cold := open(cfg)
	attach(cold, first)
	setCapacity(len(originalItems) + 2)
	first.role, err = cold.Role(ctx, first.role.ID)
	require(err)
	info, err := cold.QueryPersonalBoss(ctx, first.auth)
	require(err)
	if !reflect.DeepEqual(info.DungeonPendingRewards, []domain.ChallengePendingReward{rows[0].ChallengePendingReward}) ||
		len(info.WorldPendingRewards) != 0 || len(info.PendingRewards) != 0 || info.DungeonPendingRewardsMore {
		t.Fatal("原讨伐待领混入其他来源、换代后丢失或重新抽选", info.DungeonPendingRewards)
	}
	_, replayedFull, _, err := cold.AssetWithReceipt(ctx, first.auth, full)
	require(err)
	if replayedFull.ErrorCode != p.CodeEquipmentBagFull {
		t.Fatal("改变背包空间改判了原满包操作", replayedFull)
	}
	claim := request(first.role, rows[0].ID)
	booked, accepted, permanent, err := cold.AssetWithReceipt(ctx, first.auth, claim)
	require(err)
	if accepted.ErrorCode != 0 || !reflect.DeepEqual(permanent.ChallengeReward, &rows[0].ChallengePendingReward) ||
		!slices.Equal(booked.Items[:len(originalItems)], originalItems) || !slices.Equal(booked.Items[len(originalItems):], rows[0].Items) ||
		booked.Wallet != originalWallet || booked.SourceSeq != originalSeq {
		t.Fatal("整包原样入库、钱包或来源水位不符", accepted, permanent.ChallengeReward)
	}
	attach(cold, first)
	_, again, replay, err := cold.AssetWithReceipt(ctx, first.auth, claim)
	require(err)
	if again.ErrorCode != 0 || replay.ID != permanent.ID || !reflect.DeepEqual(replay.ChallengeReward, permanent.ChallengeReward) {
		t.Fatal("重连后没有复用原成果及唯一回执", again, replay)
	}
	_, newOp, replay, err := cold.AssetWithReceipt(ctx, first.auth, request(first.role, rows[0].ID))
	require(err)
	if newOp.ErrorCode != p.CodeOperationConflict || replay.ChallengeReward != nil {
		t.Fatal("换新操作号重复领取", newOp, replay)
	}
	var audit struct {
		Reward *domain.ChallengePendingReward `bson:"challenge_reward_claim"`
	}
	require(account.DB.Collection("audit_outbox").FindOne(ctx, bson.M{"_id": permanent.ID}).Decode(&audit))
	if !reflect.DeepEqual(audit.Reward, permanent.ChallengeReward) {
		t.Fatal("原领取附件未保存在审计", audit.Reward)
	}

	// 独立角色待领视图只读最早一页。这里仅为翻页边界准备历史附件。
	oldest := ""
	for index := range DungeonPendingRewardPageSize + 1 {
		history := rows[0]
		history.ID = domain.ID()
		history.RewardID = history.ID
		history.EarnedAt = history.EarnedAt.Add(time.Duration(index+1) * time.Second)
		if index == 0 {
			oldest = history.ID
		}
		_, err = cold.DB.Collection(dungeonRewardCollection).InsertOne(ctx, history)
		require(err)
	}
	page, err := cold.QueryPersonalBoss(ctx, first.auth)
	require(err)
	if len(page.DungeonPendingRewards) != DungeonPendingRewardPageSize || !page.DungeonPendingRewardsMore ||
		page.DungeonPendingRewards[0].RewardID != oldest {
		t.Fatal("讨伐附件首屏没有按权威归属、最早时间及有界数量返回", page.DungeonPendingRewards)
	}
}
