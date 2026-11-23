// 本文件在隔离 MongoDB 中验证个人首领的持久预约、终态和次数围栏。
package persistence

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	p "minimir/internal/protocol"
)

// TestPersonalBossEndRespectsDeadline 判断首领击杀、逃走和角色阵亡的精确时钟边界。
func TestPersonalBossEndRespectsDeadline(t *testing.T) {
	start := time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC)
	end := start.Add(time.Minute)
	original := personalBossRecord{PersonalBossChallenge: PersonalBossChallenge{StartedAt: start, ExpiresAt: end, BossHP: 35}}
	preDeadline := PersonalBossFinish{Outcome: PersonalBossDefeated, FinishedAt: end.Add(-time.Millisecond), Damage: 35}
	if !validPersonalBossFinish(original, preDeadline, end) {
		t.Fatal("原时限前的击杀未确认")
	}
	preDeadline.FinishedAt = end
	if validPersonalBossFinish(original, preDeadline, end) {
		t.Fatal("首领逃走后接受了迟到击杀")
	}
	escaped := PersonalBossFinish{Outcome: PersonalBossEscaped, FinishedAt: end, Damage: 15, HPRemaining: 20}
	if !validPersonalBossFinish(original, escaped, end.Add(time.Second)) || validPersonalBossFinish(original, escaped, end.Add(-time.Millisecond)) {
		t.Fatal("首领逃走的准确时限未执行")
	}
	escaped.Outcome, escaped.FinishedAt = PersonalBossPlayerDefeated, start.Add(time.Second)
	if !validPersonalBossFinish(original, escaped, end) {
		t.Fatal("首领仍存活的角色阵亡没有被接纳")
	}
}

// TestPersonalBossDurableAuthorityMongo 验证独占、次数、特权、竞争终态与接管对账。
func TestPersonalBossDurableAuthorityMongo(t *testing.T) {
	if os.Getenv("MINIMIR_MONGO_E2E") != "1" {
		t.Skip("需要专属开发 MongoDB 集成测试")
	}
	require := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	requireCode := func(err error, code uint32, detail string) {
		t.Helper()
		if Code(err) != code || !strings.Contains(err.Error(), detail) {
			t.Fatalf("预期错误码 %d 和信息 %s，实际 %v", code, detail, err)
		}
	}
	cfg, err := devconfig.Load("../../../conf/dev/server.json")
	require(err)
	cfg.Database = "minimir_dev_game_test_personal_boss_" + domain.ID()[:16]
	grid := cfg.Fixture.Map
	grid.ID, grid.SceneKind = 777, devconfig.SceneKindPersonalBoss
	grid.MonsterSpawns = grid.MonsterSpawns[:1]
	grid.SafeZones, grid.NPCs = nil, nil
	template := &devconfig.PersonalBossConfig{TemplateID: 13, DisplayName: "隔离首领", MinimumLevel: 1, Map: grid, Monster: cfg.Fixture.Monsters[0],
		Duration: time.Minute, SuccessCooldown: time.Minute, RetryCooldown: time.Second, DailySuccessLimit: 5, PrivilegeSuccessBonus: 1, HistoryLimit: 2,
		KillRewardGold: 11}
	cfg.Fixture.PersonalBoss = template
	if !cfg.Fixture.ValidPersonalBoss() {
		t.Fatal("隔离个人首领模板无效")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	account, err := Open(ctx, cfg)
	require(err)
	t.Cleanup(func() {
		cleanup, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		defer account.Close(cleanup)
		if err := account.DB.Drop(cleanup); err != nil {
			t.Error(err)
		}
	})
	require(account.Singleton(ctx, "account"))
	game, err := Open(ctx, cfg)
	require(err)
	defer game.Close(context.Background())
	lease, err := account.ApproveGame(ctx, cfg.NodeID, game.Instance, cfg.RunID)
	require(err)
	game.Epoch = lease.Epoch
	login, err := account.Login(ctx, "boss_"+domain.ID()[:8], "test_123456", true)
	require(err)
	accountAuth, err := account.Authenticate(ctx, login.AccountToken)
	require(err)
	create := func(slot uint32, name string) *pb.RoleKey {
		t.Helper()
		role, err := account.CreateRole(ctx, accountAuth, &pb.CreateRoleRequest{
			OriginServerId: cfg.OriginID, Slot: slot, Name: name, OpId: domain.Bytes(domain.ID()),
		})
		require(err)
		return role.Key
	}
	enter := func(key *pb.RoleKey) (domain.Role, Auth) {
		t.Helper()
		ticket, err := account.IssueTicket(ctx, accountAuth, key)
		require(err)
		role, auth, err := game.Enter(ctx, ticket.Ticket)
		require(err)
		return role, auth
	}
	seed := func(id string, fields bson.M) domain.Role {
		t.Helper()
		_, err := game.DB.Collection("roles").UpdateOne(ctx, bson.M{"_id": id}, bson.M{"$set": fields})
		require(err)
		role, err := game.Role(ctx, id)
		require(err)
		return role
	}
	makeBegin := func(role domain.Role) PersonalBossBegin {
		return PersonalBossBegin{ChallengeID: domain.ID(), Execution: 42, TemplateID: template.TemplateID, CombatSnapshot: Hash([]byte("原战斗快照")),
			ExpectedRevision: role.Revision, RuleVersion: cfg.Fixture.RuleVersion, ConfigHash: cfg.Fixture.Hash, StartedAt: time.Now().Add(-50 * time.Millisecond),
			Duration: template.Duration, SuccessCooldown: template.SuccessCooldown, RetryCooldown: template.RetryCooldown, BossHP: template.Monster.HP}
	}
	makeFinish := func(grant PersonalBossChallenge) PersonalBossFinish {
		return PersonalBossFinish{ChallengeID: grant.ChallengeID, ReservationID: grant.ReservationID, Execution: grant.Execution,
			RuleVersion: grant.RuleVersion, ConfigHash: grant.ConfigHash, Outcome: PersonalBossDefeated, FinishedAt: time.Now(), Damage: grant.BossHP}
	}

	key := create(1, "首领第一角色")
	r, auth := enter(key)
	day := shanghaiOperatingDay(time.Now())
	r = seed(r.ID, bson.M{"personal_boss": domain.PersonalBossProgress{SuccessDay: day, Successes: 4}})
	request := makeBegin(r)
	game.Cfg.Fixture.PersonalBoss = nil
	_, _, err = game.BeginPersonalBoss(ctx, auth, request)
	requireCode(err, p.CodeLineForbidden, "尚未开放")
	closed, err := game.QueryPersonalBoss(ctx, auth)
	require(err)
	if closed.Open || closed.EligibilityReason != PersonalBossUnavailable {
		t.Fatalf("关闭时仍向角色展示可报名资格：%+v", closed)
	}
	game.Cfg.Fixture.PersonalBoss = template
	template.MinimumLevel = r.Level + 1
	locked, err := game.QueryPersonalBoss(ctx, auth)
	require(err)
	if !locked.Open || locked.EligibilityReason != PersonalBossLevelRequired || locked.MinimumLevel != r.Level+1 {
		t.Fatalf("角色等级未限制首领资格：%+v", locked)
	}
	_, _, err = game.BeginPersonalBoss(ctx, auth, request)
	requireCode(err, p.CodeLineForbidden, "所需等级")
	template.MinimumLevel = 1
	beforeAdmission, err := game.QueryPersonalBoss(ctx, auth)
	require(err)
	if !beforeAdmission.Open || beforeAdmission.EligibilityReason != PersonalBossEligible || beforeAdmission.Successes != 4 || beforeAdmission.DailyLimit != 5 || len(beforeAdmission.History) != 0 {
		t.Fatalf("角色当前资格与次数没有使用确认状态：%+v", beforeAdmission)
	}
	badTemplate := request
	badTemplate.BossHP++
	_, _, err = game.BeginPersonalBoss(ctx, auth, badTemplate)
	requireCode(err, p.CodeOperationConflict, "版本变化")
	r, grant, err := game.BeginPersonalBoss(ctx, auth, request)
	require(err)
	if grant.Outcome != PersonalBossActive || grant.DailyLimit != 5 || grant.ExpiresAt.Sub(grant.StartedAt) != template.Duration || r.PersonalBoss.ActiveChallengeID != grant.ChallengeID {
		t.Fatalf("预约槽或原挑战身份错误：%+v", grant)
	}
	activeInfo, err := game.QueryPersonalBoss(ctx, auth)
	require(err)
	if activeInfo.EligibilityReason != PersonalBossAlreadyActive || activeInfo.ActiveChallengeID != grant.ChallengeID || !activeInfo.ActiveExpiresAt.Equal(grant.ExpiresAt) || len(activeInfo.History) != 0 {
		t.Fatalf("查询没有返回原活动预约：%+v", activeInfo)
	}
	replayed, same, err := game.BeginPersonalBoss(ctx, auth, request)
	require(err)
	if same != grant || replayed.Revision != r.Revision {
		t.Fatal("原预约重放改变了活动场次或角色版本")
	}
	_, _, err = game.BeginPersonalBoss(ctx, auth, makeBegin(r))
	requireCode(err, p.CodeLineForbidden, "已有个人首领场次")
	finish := makeFinish(grant)
	invalid := finish
	invalid.Execution++
	_, _, err = game.FinishPersonalBoss(ctx, auth, invalid)
	requireCode(err, p.CodeOperationConflict, "原执行")
	invalid = finish
	invalid.Damage--
	_, _, err = game.FinishPersonalBoss(ctx, auth, invalid)
	requireCode(err, p.CodeInvalidArgument, "战斗或时限")
	before := r
	r, result, err := game.FinishPersonalBoss(ctx, auth, finish)
	require(err)
	boundReward := before.Wallet
	boundReward.BoundGold += template.KillRewardGold
	if result.Outcome != PersonalBossDefeated || result.SuccessDay != day || result.Successes != 5 || result.AvailableAt.Sub(result.FinishedAt) != template.SuccessCooldown ||
		r.PersonalBoss.ActiveChallengeID != "" || r.PersonalBoss.Successes != 5 || r.Wallet != boundReward || r.SourceSeq != before.SourceSeq ||
		result.RewardGold != template.KillRewardGold || !result.RewardBound {
		t.Fatalf("胜利次数、冷却和绑定金币未原子确认，或产生了普通收益来源：%+v", result)
	}
	cooled, err := game.QueryPersonalBoss(ctx, auth)
	require(err)
	if cooled.EligibilityReason != PersonalBossCoolingDown || cooled.Successes != 5 || cooled.AvailableAt != result.AvailableAt || cooled.ActiveChallengeID != "" ||
		len(cooled.History) != 1 || cooled.History[0] != result {
		t.Fatalf("冷却、成功次数或历史没有反映首次真实结算：%+v", cooled)
	}
	replayed, same, err = game.FinishPersonalBoss(ctx, auth, finish)
	require(err)
	if same != result || replayed.Revision != r.Revision || replayed.Wallet != r.Wallet {
		t.Fatal("原击杀结果重放重复变更了角色")
	}
	_, _, err = game.BeginPersonalBoss(ctx, auth, makeBegin(r))
	requireCode(err, p.CodeLineForbidden, "仍在冷却")
	clearCooldown := bossTime(time.Now().Add(-time.Second))
	r = seed(r.ID, bson.M{"personal_boss.cooldowns." + bossTemplateKey(template.TemplateID): clearCooldown})
	_, _, err = game.BeginPersonalBoss(ctx, auth, makeBegin(r))
	requireCode(err, p.CodeLineForbidden, "成功次数上限")
	r = seed(r.ID, bson.M{"privilege_until": time.Now().Add(time.Hour)})
	r, grant, err = game.BeginPersonalBoss(ctx, auth, makeBegin(r))
	require(err)
	if grant.DailyLimit != 6 {
		t.Fatalf("特权角色的第六次预约未按规则获准：%+v", grant)
	}
	finish = makeFinish(grant)
	// 取消与击杀竞争同一活动终态，验证只能留下一种永久结果。
	interrupt := finish
	interrupt.Outcome, interrupt.Damage, interrupt.HPRemaining = PersonalBossInterrupted, 0, grant.BossHP
	terminal := make(chan error, 2)
	for _, proposal := range []PersonalBossFinish{finish, interrupt} {
		go func(q PersonalBossFinish) {
			_, _, err := game.FinishPersonalBoss(ctx, auth, q)
			terminal <- err
		}(proposal)
	}
	first, second := <-terminal, <-terminal
	if first == nil && Code(second) != p.CodeOperationConflict || second == nil && Code(first) != p.CodeOperationConflict || first != nil && second != nil {
		t.Fatalf("取消与击杀没有唯一的终态：%v %v", first, second)
	}
	stored, err := game.readPersonalBoss(ctx, grant.ChallengeID)
	require(err)
	r, err = game.Role(ctx, r.ID)
	require(err)
	if stored.Outcome == PersonalBossInterrupted && (r.PersonalBoss.Successes != 5 || stored.AvailableAt.Sub(stored.FinishedAt) != template.RetryCooldown) ||
		stored.Outcome == PersonalBossDefeated && r.PersonalBoss.Successes != 6 {
		t.Fatalf("未成功的终态改变次数或未盖上重试冷却：%+v", stored)
	}
	audits, err := game.DB.Collection("audit_outbox").CountDocuments(ctx, bson.M{"_id": "personal_boss:" + stored.ChallengeID + ":finish"})
	require(err)
	if audits != 1 {
		t.Fatal("永久终态与审计未原子去重")
	}

	// 次日重新按上海日计数，原场次回执保留，用独立角色验证会话接管的取消。
	yesterday := shanghaiOperatingDay(time.Now().Add(-24 * time.Hour))
	r = seed(r.ID, bson.M{"personal_boss.success_day": yesterday, "personal_boss.successes": 6, "personal_boss.cooldowns." + bossTemplateKey(template.TemplateID): clearCooldown})
	priorWallet := r.Wallet
	_, tomorrowGrant, err := game.BeginPersonalBoss(ctx, auth, makeBegin(r))
	require(err)
	credit := makeFinish(tomorrowGrant)
	// 模拟击杀后当前角色卡状态改变，账本仍应使用预约冻结的原击杀时间判绑。
	seed(r.ID, bson.M{"privilege_until": credit.FinishedAt.Add(-time.Second)})
	r, win, err := game.FinishPersonalBoss(ctx, auth, credit)
	require(err)
	priorWallet.Gold += template.KillRewardGold
	if r.PersonalBoss.SuccessDay != day || r.PersonalBoss.Successes != 1 || win.RewardBound || win.RewardGold != template.KillRewardGold || r.Wallet != priorWallet {
		t.Fatal("上海运营日或原击杀时间的特权金币确认异常")
	}
	latest, err := game.QueryPersonalBoss(ctx, auth)
	require(err)
	total, err := game.DB.Collection(personalBossCollection).CountDocuments(ctx, bson.M{"role_id": r.ID})
	require(err)
	if total != 3 || len(latest.History) != 2 || latest.History[0].ChallengeID != tomorrowGrant.ChallengeID || latest.History[1].ChallengeID != stored.ChallengeID {
		t.Fatalf("历史没有按配置限制为最近记录，或永久场次遭到截断：%d %+v", total, latest.History)
	}
	otherKey := create(2, "首领第二角色")
	other, oldAuth := enter(otherKey)
	other, orphan, err := game.BeginPersonalBoss(ctx, oldAuth, makeBegin(other))
	require(err)
	_, _, err = game.RecoverPersonalBoss(ctx, oldAuth)
	requireCode(err, p.CodeOperationConflict, "仍为当前执行")
	other, newAuth := enter(otherKey)
	if other.PersonalBoss.ActiveChallengeID != orphan.ChallengeID {
		t.Fatal("连接接管时意外遗漏了待对账预约")
	}
	late := makeFinish(orphan)
	_, _, err = game.FinishPersonalBoss(ctx, oldAuth, late)
	requireCode(err, p.CodeStaleSession, "失效")
	_, err = game.QueryPersonalBoss(ctx, oldAuth)
	requireCode(err, p.CodeStaleSession, "失效")
	_, _, err = game.FinishPersonalBoss(ctx, newAuth, late)
	requireCode(err, p.CodeOperationConflict, "原执行")
	other, recovered, err := game.RecoverPersonalBoss(ctx, newAuth)
	require(err)
	if !recovered.Recovered || recovered.Outcome != PersonalBossInterrupted || other.PersonalBoss.ActiveChallengeID != "" || other.PersonalBoss.Successes != 0 {
		t.Fatalf("新会话未可靠取消失权的挑战：%+v", recovered)
	}
	unchanged, _, err := game.RecoverPersonalBoss(ctx, newAuth)
	require(err)
	if unchanged.Revision != other.Revision {
		t.Fatal("取消恢复重放再次写入角色")
	}
}
