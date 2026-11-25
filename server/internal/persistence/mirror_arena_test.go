// 本文件用真实账号、绑定、写围栏与上海日账验证镜像的永久来源，和临时战斗分开。
package persistence

import (
	"context"
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	p "minimir/internal/protocol"
)

func TestMirrorArenaDurableMongo(t *testing.T) {
	uri := os.Getenv("MINIMIR_INTEGRATION_TEST_MONGO_URI")
	if os.Getenv("MINIMIR_MONGO_E2E") != "1" || uri == "" {
		t.Skip("需要本任务专属回环 MongoDB 副本集")
	}
	if !regexp.MustCompile(`^mongodb://127\.0\.0\.1:[1-9][0-9]{3,4}/\?replicaSet=minimirLoad_[a-f0-9]{32}&directConnection=true$`).MatchString(uri) {
		t.Fatal("仅允许隔离副本集控制器生成的端点")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 35*time.Second)
	defer cancel()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	wantCode := func(err error, want uint32) {
		t.Helper()
		if Code(err) != want {
			t.Fatalf("预期错误码 %d，实际 %v", want, err)
		}
	}
	cfg, err := devconfig.Load("../../../conf/dev/server.json")
	must(err)
	cfg.MongoURI, cfg.Database = uri, "minimir_dev_game_test_mirror_"+domain.ID()[:16]
	local := time.Now().In(shanghaiOperatingZone)
	anchored := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, shanghaiOperatingZone).AddDate(0, 0, -1)
	template := &devconfig.MirrorArenaConfig{TemplateID: 700, DisplayName: "镜像限定账本", SeasonAnchor: anchored, SeasonDays: 15,
		DailyLimit: 5, OpponentLimit: 3, AttackHistoryLimit: 20, DefenseHistoryLimit: 20, Duration: time.Minute,
		WinBasePoints: 5, LossPoints: 1, StreakPoints: []devconfig.MirrorArenaStreakPoints{{Streak: 3, Points: 6}, {Streak: 5, Points: 8}},
		MatchPolicy: devconfig.MirrorArenaMatchCrossJobLevelWindow, MaxLevelGap: 2}
	cfg.Fixture.MirrorArena = template
	account, err := Open(ctx, cfg)
	must(err)
	t.Cleanup(func() {
		cleanup, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		defer account.Close(cleanup)
		if err := account.DB.Drop(cleanup); err != nil {
			t.Error(err)
		}
	})
	must(account.Singleton(ctx, "account"))
	newGame := func() *Store {
		t.Helper()
		owner, err := Open(ctx, cfg)
		must(err)
		t.Cleanup(func() { owner.Close(context.Background()) })
		leased, err := account.ApproveGame(ctx, cfg.NodeID, owner.Instance, cfg.RunID)
		must(err)
		owner.Epoch = leased.Epoch
		return owner
	}
	owner := newGame()
	type participant struct {
		account Auth
		key     *pb.RoleKey
		auth    Auth
		role    domain.Role
	}
	register := func() Auth {
		t.Helper()
		login, err := account.Login(ctx, "mirror_"+domain.ID()[:8], "test_123456", true)
		must(err)
		customer, err := account.Authenticate(ctx, login.AccountToken)
		must(err)
		return customer
	}
	attach := func(store *Store, player *participant) {
		t.Helper()
		ticket, err := account.IssueTicket(ctx, player.account, player.key)
		must(err)
		player.role, player.auth, err = store.Enter(ctx, ticket.Ticket)
		must(err)
	}
	create := func(customer Auth, slot, job uint32, name string) *participant {
		t.Helper()
		info, err := account.CreateRole(ctx, customer, &pb.CreateRoleRequest{OriginServerId: cfg.OriginID, Slot: slot,
			Job: job, Name: name, OpId: domain.Bytes(domain.ID())})
		must(err)
		player := &participant{account: customer, key: info.Key}
		attach(owner, player)
		return player
	}
	snapshot := func(role domain.Role) domain.MirrorCombatSnapshot {
		t.Helper()
		fighter, ok := cfg.Fixture.PlayerAtLevel(role.Job, role.Level)
		if !ok {
			t.Fatal("缺少原职业属性")
		}
		return domain.MirrorCombatSnapshot{Job: role.Job, Gender: role.Gender, Level: role.Level, Name: role.Name,
			Fighter: fighter, Skills: slices.Clone(role.Skills), SkillSlotIDs: slices.Clone(role.SkillSlotIDs)}
	}
	publish := func(player *participant, snap domain.MirrorCombatSnapshot) MirrorDefense {
		t.Helper()
		defense, err := owner.PublishMirrorDefense(ctx, player.auth, MirrorPublish{ExpectedRevision: player.role.Revision, Snapshot: snap})
		must(err)
		return defense
	}
	info := func(game *Store, player *participant) MirrorInfo {
		t.Helper()
		read, err := game.QueryMirrorArena(ctx, player.auth)
		must(err)
		return read
	}
	makeBegin := func(player *participant, offer string, execution uint64) MirrorBegin {
		return MirrorBegin{ChallengeID: domain.ID(), OfferID: offer, Execution: execution,
			ExpectedRevision: player.role.Revision, AttackerSnapshot: snapshot(player.role)}
	}
	makeFinish := func(grant MirrorChallenge, outcome MirrorOutcome) MirrorFinish {
		return MirrorFinish{ChallengeID: grant.ChallengeID, ReservationID: grant.ReservationID, Execution: grant.Execution,
			Outcome: outcome, FinishedAt: bossTime(time.Now())}
	}

	a, b := create(register(), 1, 0, "镜像攻方"), create(register(), 1, 2, "镜像守方")
	alt := create(a.account, 2, 1, "同账号镜像")
	aWallet, bWallet := a.role.Wallet, b.role.Wallet
	if result := info(owner, a); len(result.Offers) != 0 || result.EntriesUsed != 0 || result.DailyLimit != 5 {
		t.Fatalf("空镜像池不应分配玩家：%+v", result)
	}
	publish(a, snapshot(a.role))
	publish(alt, snapshot(alt.role))
	first := publish(b, snapshot(b.role))
	if repeated := publish(b, snapshot(b.role)); repeated.ID != first.ID || repeated.Version != 1 {
		t.Fatalf("同一输入发布重复版本：%+v", repeated)
	}
	aOffers := info(owner, a).Offers
	if len(aOffers) != 1 || aOffers[0].DefenderName != b.role.Name || aOffers[0].DefenseVersion != first.Version {
		t.Fatalf("候选必须排除同账号和本人，保留实际跨职业镜像：%+v", aOffers)
	}
	newBody := snapshot(b.role)
	newBody.Fighter.HP++ // 限定测试中仅改变服务端输入，核对已发候选版本不跟随。
	if latest := publish(b, newBody); latest.ID == first.ID || latest.Version != 2 {
		t.Fatalf("改动后未获得新防守版本：%+v", latest)
	}
	if got := info(owner, a).Offers; !slices.Equal(got, aOffers) {
		t.Fatalf("已发凭证被重新发布的镜像更换：%+v", got)
	}
	bOffers := info(owner, b).Offers
	if len(bOffers) != 2 {
		t.Fatalf("守方真实系统候选缺少另一账号角色：%+v", bOffers)
	}
	_, err = owner.BeginMirrorChallenge(ctx, alt.auth, makeBegin(alt, aOffers[0].ID, 88))
	wantCode(err, p.CodeLineForbidden)
	_, err = owner.BeginMirrorChallenge(ctx, a.auth, makeBegin(a, domain.ID(), 90))
	wantCode(err, p.CodeLineForbidden)

	bRound, err := owner.BeginMirrorChallenge(ctx, b.auth, makeBegin(b, bOffers[0].ID, 103))
	must(err)
	bWin, err := owner.FinishMirrorChallenge(ctx, b.auth, makeFinish(bRound, MirrorWon))
	must(err)
	if bWin.Points != 5 || bWin.Streak != 1 {
		t.Fatalf("守方先前自己的进攻结果异常：%+v", bWin)
	}

	req := makeBegin(a, aOffers[0].ID, 200)
	firstMatch, err := owner.BeginMirrorChallenge(ctx, a.auth, req)
	must(err)
	if firstMatch.DefenseSnapshot.Fighter.HP != first.Snapshot.Fighter.HP || firstMatch.DefenseVersion != 1 {
		t.Fatal("原挑战把最新防守版本替换了已发的原版本")
	}
	var stored bson.M
	must(owner.DB.Collection(mirrorChallengeCollection).FindOne(ctx, bson.M{"_id": firstMatch.ChallengeID}).Decode(&stored))
	if stored["attacker_snapshot"] != nil || stored["defense_snapshot"] != nil || stored["defense_id"] != first.ID || stored["begin_digest"] == nil {
		t.Fatal("场次永久记录应保留原防守版本与攻方快照指纹，不能保存可重建临时世界的双方属性")
	}
	if replay, err := owner.BeginMirrorChallenge(ctx, a.auth, req); err != nil || replay.ReservationID != firstMatch.ReservationID ||
		replay.AttackerSnapshot.Fighter.HP != firstMatch.AttackerSnapshot.Fighter.HP || replay.DefenseSnapshot.Fighter.HP != firstMatch.DefenseSnapshot.Fighter.HP {
		t.Fatal("同会话未知提交无法核对原预占和临时启动输入", err)
	}
	_, err = owner.BeginMirrorChallenge(ctx, a.auth, makeBegin(a, aOffers[0].ID, 201))
	wantCode(err, p.CodeLineForbidden)
	personal := owner.Cfg.Fixture.PersonalBoss
	if personal == nil || !owner.Cfg.Fixture.ValidPersonalBoss() {
		t.Fatal("隔离测试缺少规则 30 的原首领目录")
	}
	_, _, err = owner.BeginPersonalBoss(ctx, a.auth, PersonalBossBegin{ChallengeID: domain.ID(), TemplateID: personal.TemplateID, Execution: 7,
		ExpectedRevision: a.role.Revision, CombatSnapshot: Hash([]byte("独占对照")), RuleVersion: cfg.Fixture.RuleVersion, ConfigHash: cfg.Fixture.Hash,
		StartedAt: time.Now(), Duration: personal.Duration, SuccessCooldown: personal.SuccessCooldown, RetryCooldown: personal.RetryCooldown, BossHP: personal.Monster.HP})
	if Code(err) != p.CodeLineForbidden || !strings.Contains(err.Error(), "尚未结算的镜像") {
		t.Fatal("已预占镜像的角色仍能预约个人首领", err)
	}
	owner.Cfg.Fixture.Dungeons = []devconfig.DungeonConfig{{TemplateID: 701, Duration: time.Minute, MinimumLevel: 1}}
	party := DungeonBegin{ChallengeID: domain.ID(), Execution: 8, TemplateID: 701, RuleVersion: cfg.Fixture.RuleVersion,
		ConfigHash: cfg.Fixture.Hash, StartedAt: time.Now(), Duration: time.Minute}
	for _, player := range []*participant{a, b, alt} {
		party.Members = append(party.Members, DungeonAdmissionMember{Auth: player.auth, ExpectedRevision: player.role.Revision,
			CombatSnapshot: Hash([]byte(player.role.ID))})
	}
	_, _, err = owner.BeginDungeon(ctx, party)
	if Code(err) != p.CodeLineForbidden || !strings.Contains(err.Error(), "尚未结算的镜像") {
		t.Fatal("包含镜像攻方的讨伐队仍获准扣次", err)
	}
	_, err = owner.RecoverMirrorChallenge(ctx, a.auth)
	wantCode(err, p.CodeOperationConflict)

	oldAuth := a.auth
	attach(owner, a)
	finished := makeFinish(firstMatch, MirrorWon)
	_, err = owner.FinishMirrorChallenge(ctx, oldAuth, finished)
	wantCode(err, p.CodeStaleSession)
	proof := MirrorResume{ChallengeID: firstMatch.ChallengeID, ReservationID: firstMatch.ReservationID, Execution: firstMatch.Execution,
		PreviousSessionEpoch: oldAuth.SessionEpoch}
	for range 2 {
		resumed, err := owner.ResumeMirrorChallenge(ctx, a.auth, proof)
		must(err)
		if resumed.ReservationID != firstMatch.ReservationID || !resumed.ExpiresAt.Equal(firstMatch.ExpiresAt) {
			t.Fatal("健康接回改动原预占或截止时刻")
		}
	}
	for range 2 {
		win, err := owner.FinishMirrorChallenge(ctx, a.auth, finished)
		must(err)
		if win.Points != 5 || win.SeasonPoints != 5 || win.Streak != 1 {
			t.Fatalf("比赛结束重放增加重复积分：%+v", win)
		}
	}
	foreign := finished
	foreign.Outcome = MirrorLost
	_, err = owner.FinishMirrorChallenge(ctx, a.auth, foreign)
	wantCode(err, p.CodeOperationConflict)
	finishedEpoch := a.auth.SessionEpoch
	attach(owner, a)
	proof.PreviousSessionEpoch = finishedEpoch
	terminal, err := owner.ResumeMirrorChallenge(ctx, a.auth, proof)
	must(err)
	original, err := owner.readMirrorRecord(ctx, firstMatch.ChallengeID)
	must(err)
	if terminal.Outcome != MirrorWon || terminal.Points != 5 || original.SessionEpoch != finishedEpoch {
		t.Fatal("终态丢响应后新票未读到原结果，或改变了原终态会话", terminal)
	}
	readBack, err := owner.BeginMirrorChallenge(ctx, a.auth, MirrorBegin{ChallengeID: req.ChallengeID, OfferID: req.OfferID})
	must(err)
	if readBack.Outcome != MirrorWon || readBack.ReservationID != firstMatch.ReservationID {
		t.Fatal("完成比赛重连后原操作号未返回原结果")
	}
	for i, expected := range []int64{5, 6, 6, 8} {
		match, err := owner.BeginMirrorChallenge(ctx, a.auth, makeBegin(a, aOffers[0].ID, uint64(301+i)))
		must(err)
		win, err := owner.FinishMirrorChallenge(ctx, a.auth, makeFinish(match, MirrorWon))
		must(err)
		if win.Points != expected || win.Streak != uint32(i+2) {
			t.Fatalf("实际连胜档 %d 的本场积分异常：%+v", i+2, win)
		}
	}
	_, err = owner.BeginMirrorChallenge(ctx, a.auth, makeBegin(a, aOffers[0].ID, 401))
	wantCode(err, p.CodeLineForbidden)
	aState, bState := info(owner, a), info(owner, b)
	if aState.EntriesUsed != 5 || aState.SeasonPoints != 30 || len(aState.AttackHistory) != 5 ||
		bState.SeasonPoints != 5 || bState.Streak != 1 || len(bState.DefenseHistory) != 5 {
		t.Fatalf("每日配额或攻防独立积分不一致：攻=%+v 守=%+v", aState, bState)
	}
	visible, err := owner.mirrorHistory(ctx, "defender_role_id", b.role, 2)
	must(err)
	totalDefended, err := owner.DB.Collection(mirrorChallengeCollection).CountDocuments(ctx, bson.M{"defender_role_id": b.role.ID})
	must(err)
	if len(visible) != 2 || !slices.Equal(visible, bState.DefenseHistory[:2]) || totalDefended != 5 {
		t.Fatal("战报窗口应按时间截取，不应删除原场次永久回执")
	}

	bLossRound, err := owner.BeginMirrorChallenge(ctx, b.auth, makeBegin(b, bOffers[0].ID, 501))
	must(err)
	bLoss, err := owner.FinishMirrorChallenge(ctx, b.auth, makeFinish(bLossRound, MirrorLost))
	must(err)
	if bLoss.Points != 1 || bLoss.SeasonPoints != 6 || bLoss.Streak != 0 {
		t.Fatalf("失败未增加 1 分并清除自身进攻连胜：%+v", bLoss)
	}
	maintenanceRound, err := owner.BeginMirrorChallenge(ctx, b.auth, makeBegin(b, bOffers[0].ID, 502))
	must(err)
	maintenanceFinish := makeFinish(maintenanceRound, MirrorInterrupted)
	for range 2 {
		result, err := owner.FinishMirrorChallenge(ctx, b.auth, maintenanceFinish)
		must(err)
		if result.Points != 0 || result.SeasonPoints != 6 || info(owner, b).EntriesUsed != 2 {
			t.Fatal("维护中断未一次回退原预占，或改变原赛季分")
		}
	}
	orphan, err := owner.BeginMirrorChallenge(ctx, b.auth, makeBegin(b, bOffers[0].ID, 503))
	must(err)
	beforeFault := b.auth
	must(owner.Release(ctx, "game"))
	takeover := newGame()
	attach(takeover, b)
	_, err = owner.FinishMirrorChallenge(ctx, beforeFault, makeFinish(orphan, MirrorWon))
	wantCode(err, p.CodeRouteChanged)
	recovered, err := takeover.RecoverMirrorChallenge(ctx, b.auth)
	must(err)
	if recovered.ChallengeID != orphan.ChallengeID || recovered.Outcome != MirrorInterrupted || recovered.Points != 0 ||
		info(takeover, b).EntriesUsed != 2 || info(takeover, b).SeasonPoints != 6 {
		t.Fatalf("原游戏节点换代后未中断退次并保留原已确认积分：%+v", recovered)
	}
	noReplay, err := takeover.RecoverMirrorChallenge(ctx, b.auth)
	must(err)
	if noReplay.ChallengeID != "" {
		t.Fatal("已回收原预占又被结算一次")
	}
	for _, player := range []*participant{a, b} {
		role, err := takeover.Role(ctx, player.role.ID)
		must(err)
		if role.Wallet != map[string]domain.Wallet{a.role.ID: aWallet, b.role.ID: bWallet}[player.role.ID] {
			t.Fatal("测试镜像积分改变了实际钱包")
		}
	}
}
