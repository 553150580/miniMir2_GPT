// 本文件在隔离 MongoDB 复盘旧赛季尾局、并发换代、有界派发及永久资产领取。
package persistence

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"sync"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	p "minimir/internal/protocol"
)

func TestMirrorSeasonSettlementMongo(t *testing.T) {
	uri := os.Getenv("MINIMIR_INTEGRATION_TEST_MONGO_URI")
	if os.Getenv("MINIMIR_MONGO_E2E") != "1" || uri == "" {
		t.Skip("需要本任务专属回环 MongoDB 副本集")
	}
	if !regexp.MustCompile(`^mongodb://127\.0\.0\.1:[1-9][0-9]{3,4}/\?replicaSet=minimirLoad_[a-f0-9]{32}&directConnection=true$`).MatchString(uri) {
		t.Fatal("仅允许隔离副本集控制器生成的端点")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 75*time.Second)
	defer cancel()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	cfg, err := devconfig.Load("../../../conf/dev/server.json")
	must(err)
	cfg.MongoURI, cfg.Database = uri, "minimir_dev_game_test_mirror_season_"+domain.ID()[:16]
	local := time.Now().In(shanghaiOperatingZone)
	begin := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, shanghaiOperatingZone).AddDate(0, 0, -16)
	end := begin.AddDate(0, 0, 15)
	cfg.Fixture.MirrorArena = &devconfig.MirrorArenaConfig{TemplateID: 700, DisplayName: "隔离赛季结算", SeasonAnchor: begin, SeasonDays: 15,
		DailyLimit: 5, OpponentLimit: 3, AttackHistoryLimit: 20, DefenseHistoryLimit: 20, Duration: time.Minute, WinBasePoints: 5, LossPoints: 1,
		Settlement: &devconfig.MirrorArenaSeasonSettlement{RankingTiebreak: devconfig.MirrorArenaRankingEarlierLastChallenge, ParticipationMinChallenges: 10,
			Participation: devconfig.TaskCodexReward{BoundYuanbaoUnits: 3}, Ranks: []devconfig.MirrorArenaSeasonRank{
				{FirstRank: 1, LastRank: 1, Reward: devconfig.TaskCodexReward{BoundYuanbaoUnits: 8}},
				{FirstRank: 2, LastRank: 2, Reward: devconfig.TaskCodexReward{BoundYuanbaoUnits: 5}},
			}}}
	account, err := Open(ctx, cfg)
	must(err)
	t.Cleanup(func() {
		cleanup, stop := context.WithTimeout(context.Background(), 8*time.Second)
		defer stop()
		defer account.Close(cleanup)
		if err := account.DB.Drop(cleanup); err != nil {
			t.Error(err)
		}
	})
	must(account.Singleton(ctx, "account"))
	newOwner := func() *Store {
		t.Helper()
		store, err := Open(ctx, cfg)
		must(err)
		t.Cleanup(func() { store.Close(context.Background()) })
		lease, err := account.ApproveGame(ctx, cfg.NodeID, store.Instance, cfg.RunID)
		must(err)
		store.Epoch = lease.Epoch
		return store
	}
	owner := newOwner()
	type participant struct {
		auth    Auth
		account Auth
		role    domain.Role
		key     *pb.RoleKey
	}
	attach := func(store *Store, player *participant) {
		t.Helper()
		ticket, err := account.IssueTicket(ctx, player.account, player.key)
		must(err)
		player.role, player.auth, err = store.Enter(ctx, ticket.Ticket)
		must(err)
	}
	create := func(name string) *participant {
		t.Helper()
		login, err := account.Login(ctx, "mseason_"+domain.ID()[:8], "test_123456", true)
		must(err)
		bound, err := account.Authenticate(ctx, login.AccountToken)
		must(err)
		info, err := account.CreateRole(ctx, bound, &pb.CreateRoleRequest{OriginServerId: cfg.OriginID, Slot: 1, Job: 2, Name: name, OpId: domain.Bytes(domain.ID())})
		must(err)
		player := &participant{account: bound, key: info.Key}
		attach(owner, player)
		return player
	}
	champion, tail, stale := create("镜像结算榜首"), create("镜像结算尾局"), create("镜像失权回收")
	scope, activeScope := mirrorScope(cfg.RunID, cfg.OriginID, begin), mirrorScope(cfg.RunID, cfg.OriginID, end)
	scores := owner.DB.Collection(mirrorSeasonCollection)
	timeEarly, timeLate := end.Add(-3*time.Hour), end.Add(-time.Hour)
	// 为复现自然等待十五天才会出现的边界，隔离库预置旧版本结束记录和超过三批的原成绩。
	addScore := func(scope, role, name string, points int64, matches uint32, last time.Time) {
		t.Helper()
		_, err := scores.InsertOne(ctx, mirrorSeasonScore{ID: scope + ":" + role, Scope: scope, RoleID: role, Name: name,
			Points: points, Matches: matches, Streak: 1, LastMatchAt: last})
		must(err)
	}
	addScore(scope, champion.role.ID, champion.role.Name, 44, 10, timeEarly)
	for i := range 25 {
		id := fmt.Sprintf("ffffffffffffffffffffffff%08x", i)
		points, matches, clock := int64(5), uint32(1), timeLate
		if i == 0 {
			points, matches, clock = 44, 9, timeEarly
		}
		if i == 1 {
			points, matches = 44, 9
		}
		addScore(scope, id, fmt.Sprintf("赛季原记录%d", i), points, matches, clock)
		addScore(activeScope, id, fmt.Sprintf("现季原记录%d", i), 5, 1, end.Add(time.Hour))
	}
	addScore(activeScope, champion.role.ID, champion.role.Name, 4, 2, end.Add(time.Hour))
	challenges := owner.DB.Collection(mirrorChallengeCollection)
	makeActive := func(player *participant, owns bool) mirrorRecord {
		t.Helper()
		row := mirrorRecord{MirrorChallenge: MirrorChallenge{MirrorSummary: MirrorSummary{ChallengeID: domain.ID(), AttackerRoleID: player.role.ID,
			AttackerName: player.role.Name, DefenderRoleID: champion.role.ID, DefenderName: champion.role.Name, StartedAt: timeLate},
			RunID: cfg.RunID, SeasonStart: begin, SeasonEnd: end, ExpiresAt: timeLate.Add(time.Minute), Execution: 1, ReservationID: domain.ID()},
			Origin: player.role.Origin, OwnerInstance: owner.Instance, RunEpoch: owner.Epoch, SessionEpoch: player.auth.SessionEpoch,
			ScoreKey: scope + ":" + player.role.ID, DayKey: mirrorDayKey(scope, player.role.ID, timeLate), WinPoints: 5, LossPoints: 1}
		if !owns {
			row.OwnerInstance, row.RunEpoch = domain.ID(), owner.Epoch-1
		}
		_, err := challenges.InsertOne(ctx, row)
		must(err)
		_, err = owner.DB.Collection(mirrorDayCollection).InsertOne(ctx, mirrorDay{ID: row.DayKey, Entries: 1})
		must(err)
		_, err = owner.DB.Collection(mirrorActiveCollection).InsertOne(ctx, mirrorActive{ID: mirrorRoleKey(row.Origin, row.AttackerRoleID), ChallengeID: row.ChallengeID})
		must(err)
		return row
	}
	ongoing, expired := makeActive(tail, true), makeActive(stale, false)
	journals := owner.DB.Collection(mirrorSeasonJournalCollection)
	readJournal := func() mirrorSeasonJournal {
		t.Helper()
		value, err := mirrorOptional[mirrorSeasonJournal](ctx, journals, scope)
		must(err)
		return value
	}
	at := bossTime(time.Now())
	advance := func(store *Store, now time.Time) {
		t.Helper()
		_, err := store.AdvanceMirrorSeason(ctx, now)
		must(err)
	}
	advance(owner, at) // 无赛季指针的旧挑战由有界迁移登记。
	advance(owner, at) // 原健康尾局保留，只有失权预占退次。
	if journal := readJournal(); journal.Phase != mirrorSeasonCollecting || !journal.NextCheckAt.After(at) || journal.Ranked != 0 {
		t.Fatalf("原活动到期时错误地提前冻结排名：%+v", journal)
	}
	old, err := owner.readMirrorRecord(ctx, expired.ChallengeID)
	must(err)
	priorDay, err := mirrorOptional[mirrorDay](ctx, owner.DB.Collection(mirrorDayCollection), expired.DayKey)
	must(err)
	if old.Outcome != MirrorInterrupted || priorDay.Entries != 0 {
		t.Fatal("失权旧挑战未沿原日退次")
	}
	finish := MirrorFinish{ChallengeID: ongoing.ChallengeID, ReservationID: ongoing.ReservationID, Execution: ongoing.Execution,
		Outcome: MirrorLost, FinishedAt: timeLate.Add(time.Second)}
	ended, err := owner.FinishMirrorChallenge(ctx, tail.auth, finish)
	must(err)
	if ended.Points != 1 {
		t.Fatal("原尾局在有权时未入账", ended)
	}
	for range 12 {
		if readJournal().Phase == mirrorSeasonCrediting {
			break
		}
		advance(owner, at.Add(2*time.Second))
	}
	journal := readJournal()
	rewards := owner.DB.Collection(mirrorSeasonRewardCollection)
	awards, err := rewards.CountDocuments(ctx, bson.M{})
	must(err)
	coins, err := owner.ReadMirrorExchangePoints(ctx, champion.role.Origin, champion.role.ID)
	must(err)
	untouched, err := mirrorOptional[mirrorSeasonScore](ctx, scores, scope+":"+champion.role.ID)
	must(err)
	if journal.Phase != mirrorSeasonCrediting || journal.Ranked != 27 || journal.Credited != 0 || awards != 3 || coins != 0 || untouched.Points != 44 {
		t.Fatalf("整榜附件应先确认，之后才允许结转清零：phase=%d rank=%d credit=%d awards=%d balance=%d score=%d",
			journal.Phase, journal.Ranked, journal.Credited, awards, coins, untouched.Points)
	}
	advance(owner, at.Add(3*time.Second))
	if entry := readJournal(); entry.Credited != mirrorSeasonBatch || entry.Phase != mirrorSeasonCrediting {
		t.Fatalf("结转批次未保持明确边界：%+v", entry)
	}
	must(owner.Release(ctx, "game"))
	replacement := newOwner()
	if _, err := owner.AdvanceMirrorSeason(ctx, at.Add(4*time.Second)); Code(err) != p.CodeRouteChanged {
		t.Fatalf("已撤销旧运行所有权仍能推进：%v", err)
	}
	for range 5 {
		var concurrent sync.WaitGroup
		for range 2 {
			concurrent.Go(func() {
				_, err := replacement.AdvanceMirrorSeason(ctx, at.Add(4*time.Second))
				if err != nil {
					t.Error(err)
				}
			})
		}
		concurrent.Wait()
	}
	closed := readJournal()
	remaining, err := scores.CountDocuments(ctx, bson.M{"scope": scope, "points": bson.M{"$ne": 0}})
	must(err)
	if closed.Phase != mirrorSeasonClosed || closed.Credited != 27 || remaining != 0 {
		t.Fatalf("换代后未一次性整榜归零：%+v 剩余 %d", closed, remaining)
	}
	journalAudit, err := owner.DB.Collection("audit_outbox").CountDocuments(ctx, bson.M{"season_id": mirrorSeasonPublicID(scope)})
	must(err)
	if journalAudit != 30 {
		t.Fatalf("三笔附件与二十七笔结转未独立且只写一次审计：%d", journalAudit)
	}
	attach(replacement, champion)
	info, err := replacement.QueryMirrorArena(ctx, champion.auth)
	must(err)
	if info.OwnRank != 26 || len(info.Rankings) != mirrorRankingPageSize || info.SeasonPoints != 4 || info.ExchangePoints != 44 ||
		len(info.SeasonPendingRewards) != 2 || info.LatestSettlement == nil || info.LatestSettlement.Rank != 1 ||
		info.LatestSettlement.Points != 44 || info.LatestSettlement.ExchangeCredit != 44 || info.LatestSettlement.Matches != 10 {
		t.Fatalf("原入口角色未读回真实榜外名次、余额、结转和两类奖：%+v", info)
	}
	var second mirrorSeasonPlacement
	must(owner.DB.Collection(mirrorSeasonRankCollection).FindOne(ctx, bson.M{"scope": scope, "rank": 2}).Decode(&second))
	if second.RoleID != "ffffffffffffffffffffffff00000000" {
		t.Fatal("同分同毫秒未按永久角色编号形成总序", second)
	}
	var third mirrorSeasonPlacement
	must(owner.DB.Collection(mirrorSeasonRankCollection).FindOne(ctx, bson.M{"scope": scope, "rank": 3}).Decode(&third))
	if third.RoleID != "ffffffffffffffffffffffff00000001" {
		t.Fatal("同分未优先最近挑战较早者", third)
	}
	attachment := info.SeasonPendingRewards[0]
	request := AssetRequest{Context: &pb.OperationContext{OpId: domain.Bytes(domain.ID()), CommandSeq: uint64(champion.role.EconomicSeq + 1),
		ExpectedRevision: uint64(champion.role.Revision), ExpectedRuleVersion: cfg.Fixture.RuleVersion},
		Kind: AssetChallengeRewardClaim, Fingerprint: Hash([]byte(attachment.RewardID)), ChallengeRewardID: attachment.RewardID}
	after, result, _, err := replacement.AssetWithReceipt(ctx, champion.auth, request)
	must(err)
	if result.ErrorCode != 0 || after.Wallet.BoundYuanbao-champion.role.Wallet.BoundYuanbao != attachment.MirrorAssets.BoundIngot {
		t.Fatal("原赛季绑定元宝未由既有 40030 事务领取", result)
	}
	replayed, result, _, err := replacement.AssetWithReceipt(ctx, champion.auth, request)
	must(err)
	if result.ErrorCode != 0 || replayed.Wallet != after.Wallet || replayed.EconomicSeq != after.EconomicSeq {
		t.Fatal("重放原赛季奖励增加了资产")
	}
}
