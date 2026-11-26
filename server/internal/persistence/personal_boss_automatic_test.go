// 本文件检验自动挑战使用真实通关证明、角色卡权益与原终态回执。
package persistence

import (
	"context"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	p "minimir/internal/protocol"
)

// TestPersonalBossBeginLegacyDigest 确认历史手动预约保持原编码，自动来源和证明参与新摘要。
func TestPersonalBossBeginLegacyDigest(t *testing.T) {
	q := PersonalBossBegin{ChallengeID: "original", Execution: 2, TemplateID: 3, CombatSnapshot: "snapshot", ExpectedRevision: 5,
		RuleVersion: 6, ConfigHash: "fixture", StartedAt: time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC),
		Duration: 8, SuccessCooldown: 9, RetryCooldown: 10, BossHP: 11}
	legacy := `{"ChallengeID":"original","Execution":2,"TemplateID":3,"CombatSnapshot":"snapshot","ExpectedRevision":5,"RuleVersion":6,"ConfigHash":"fixture","StartedAt":"2026-11-24T12:00:00Z","Duration":8,"SuccessCooldown":9,"RetryCooldown":10,"BossHP":11}`
	before, err := bossDigest(q)
	if err != nil || before != Hash([]byte(legacy)) {
		t.Fatalf("旧预约的 JSON 摘要变化：%s %v", before, err)
	}
	q.Qualification = Hash([]byte("原通关条件"))
	qualified, err := bossDigest(q)
	if err != nil || qualified == before {
		t.Fatalf("通关条件未影响预约摘要：%s %v", qualified, err)
	}
	q.Automatic = true
	automatic, err := bossDigest(q)
	if err != nil || automatic == qualified {
		t.Fatalf("自动发起未影响预约摘要：%s %v", automatic, err)
	}
}

// TestPersonalBossAutomaticQualificationScope 精确核验卡到期、证据版本和模板范围。
func TestPersonalBossAutomaticQualificationScope(t *testing.T) {
	at := time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC)
	fingerprint, config := Hash([]byte("配置")), Hash([]byte("角色条件"))
	r := domain.Role{HostingCardUntil: at.Add(time.Millisecond)}
	q := PersonalBossBegin{Automatic: true, Qualification: fingerprint, RuleVersion: 30, ConfigHash: config, TemplateID: 3}
	state := domain.PersonalBossProgress{Qualifications: map[string]domain.PersonalBossQualification{
		"3": {Fingerprint: fingerprint, RuleVersion: 30, ConfigHash: config},
	}}
	fixture := &devconfig.Fixture{RuleVersion: q.RuleVersion, Hash: config, HostingBenefits: &devconfig.HostingBenefits{AutoPersonalBoss: true}}
	if err := checkAutomaticPersonalBoss(r, state, fixture, q, at); err != nil {
		t.Fatal(err)
	}
	if err := checkAutomaticPersonalBoss(r, state, fixture, q, r.HostingCardUntil); Code(err) != p.CodeLineForbidden {
		t.Fatalf("托管卡到期时仍允许自动预约：%v", err)
	}
	for _, proof := range []domain.PersonalBossQualification{
		{Fingerprint: fingerprint, RuleVersion: 31, ConfigHash: config},
		{Fingerprint: fingerprint, RuleVersion: 30, ConfigHash: Hash([]byte("另一个版本摘要"))},
		{Fingerprint: Hash([]byte("角色换装")), RuleVersion: 30, ConfigHash: config},
	} {
		state.Qualifications["3"] = proof
		if err := checkAutomaticPersonalBoss(r, state, fixture, q, at); Code(err) != p.CodeLineForbidden {
			t.Fatalf("接受不同战斗条件的通关证明：%+v %v", proof, err)
		}
	}
	state.Qualifications["4"] = domain.PersonalBossQualification{Fingerprint: fingerprint, RuleVersion: 30, ConfigHash: config}
	if err := checkAutomaticPersonalBoss(r, state, fixture, q, at); Code(err) != p.CodeLineForbidden {
		t.Fatalf("接受其他档位的通关证明：%v", err)
	}
	// 预约使用当前完整摘要，只允许已核对的前一份规则 30 夹具沿用原战斗条件证明。
	fixture.Hash = "6a421806cf5d45a8ead84bfda80be4a57eebcac00d3344140825fc860bfec9a6"
	q.ConfigHash = fixture.Hash
	state.Qualifications["3"] = domain.PersonalBossQualification{Fingerprint: fingerprint, RuleVersion: 30,
		ConfigHash: "4bb20bf3e8d235db1061e3223b8b28169f662d3e7e334291f6f65c09400acb1a"}
	if err := checkAutomaticPersonalBoss(r, state, fixture, q, at); err != nil {
		t.Fatalf("双卡配置没有保留原通关证明：%v", err)
	}
}

// TestPersonalBossAutomaticDurableMongo 验证有效卡与真实胜利的原子门禁，以及自动奖励判绑和重放。
func TestPersonalBossAutomaticDurableMongo(t *testing.T) {
	uri := os.Getenv("MINIMIR_INTEGRATION_TEST_MONGO_URI")
	if os.Getenv("MINIMIR_MONGO_E2E") != "1" || uri == "" {
		t.Skip("需要独立 MongoDB 副本集")
	}
	require := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	cfg, err := devconfig.Load("../../../conf/dev/server.json")
	require(err)
	cfg.MongoURI, cfg.Database = uri, "minimir_dev_game_test_boss_auto_"+domain.ID()[:16]
	template := cfg.Fixture.PersonalBoss
	if template == nil || cfg.Fixture.HostingBenefits == nil {
		t.Fatal("缺少原个人首领或托管卡开发夹具")
	}
	template.KillRewardGold = 11
	template.ItemReward = &devconfig.ChallengeItemReward{PendingLimit: 2, Items: []devconfig.ChallengeRewardItem{
		{ItemID: cfg.Fixture.InitialInventory[0], Quantity: 1},
	}}
	cfg.Fixture.HostingBenefits.AutoPersonalBoss = false

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	account, err := Open(ctx, cfg)
	require(err)
	t.Cleanup(func() {
		cleanup, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		defer account.Close(cleanup)
		require(account.DB.Drop(cleanup))
	})
	require(account.Singleton(ctx, "account"))
	game, err := Open(ctx, cfg)
	require(err)
	defer game.Close(context.Background())
	lease, err := account.ApproveGame(ctx, cfg.NodeID, game.Instance, cfg.RunID)
	require(err)
	game.Epoch = lease.Epoch
	login, err := account.Login(ctx, "auto_"+domain.ID()[:8], "test_123456", true)
	require(err)
	accountAuth, err := account.Authenticate(ctx, login.AccountToken)
	require(err)
	createRole := func(slot uint32, name string) (domain.Role, Auth) {
		t.Helper()
		summary, err := account.CreateRole(ctx, accountAuth, &pb.CreateRoleRequest{
			OriginServerId: cfg.OriginID, Slot: slot, Name: name, OpId: domain.Bytes(domain.ID()),
		})
		require(err)
		ticket, err := account.IssueTicket(ctx, accountAuth, summary.Key)
		require(err)
		role, auth, err := game.Enter(ctx, ticket.Ticket)
		require(err)
		return role, auth
	}
	r, auth := createRole(1, "托管首领原角色")
	other, otherAuth := createRole(2, "托管首领同账号")
	seed := func(r domain.Role, changes bson.M) domain.Role {
		t.Helper()
		_, err := game.DB.Collection("roles").UpdateOne(ctx, bson.M{"_id": r.ID}, bson.M{"$set": changes})
		require(err)
		r, err = game.Role(ctx, r.ID)
		require(err)
		return r
	}
	key := bossTemplateKey(template.TemplateID)
	clearCooldown := func(r domain.Role) domain.Role {
		return seed(r, bson.M{"personal_boss.cooldowns." + key: bossTime(time.Now().Add(-time.Second))})
	}
	proof := Hash([]byte("原角色条件证明"))
	begin := func(r domain.Role, automatic bool) PersonalBossBegin {
		return PersonalBossBegin{ChallengeID: domain.ID(), Execution: 42, TemplateID: template.TemplateID,
			CombatSnapshot: Hash([]byte("该场冻结战斗状态")), ExpectedRevision: r.Revision,
			RuleVersion: cfg.Fixture.RuleVersion, ConfigHash: cfg.Fixture.Hash, StartedAt: time.Now(),
			Duration: template.Duration, SuccessCooldown: template.SuccessCooldown, RetryCooldown: template.RetryCooldown,
			BossHP: template.Monster.HP, Qualification: proof, Automatic: automatic}
	}
	finish := func(grant PersonalBossChallenge) PersonalBossFinish {
		return PersonalBossFinish{ChallengeID: grant.ChallengeID, ReservationID: grant.ReservationID, Execution: grant.Execution,
			RuleVersion: grant.RuleVersion, ConfigHash: grant.ConfigHash, Outcome: PersonalBossDefeated, FinishedAt: time.Now(), Damage: grant.BossHP}
	}
	reject := func(a Auth, request PersonalBossBegin, code uint32, clue string) {
		t.Helper()
		_, _, err := game.BeginPersonalBoss(ctx, a, request)
		if err == nil || Code(err) != code || !strings.Contains(err.Error(), clue) {
			t.Fatalf("期待拒绝 %d %s，收到 %v", code, clue, err)
		}
	}

	validUntil := bossTime(time.Now().Add(time.Hour))
	r = seed(r, bson.M{"hosting_card_until": validUntil})
	reject(auth, begin(r, true), p.CodeLineForbidden, "未开放")
	game.Cfg.Fixture.HostingBenefits.AutoPersonalBoss = true
	reject(auth, begin(r, true), p.CodeLineForbidden, "完成实战")
	bad := begin(r, true)
	bad.Qualification = strings.ToUpper(proof)
	reject(auth, bad, p.CodeInvalidArgument, "摘要无效")

	r, lost, err := game.BeginPersonalBoss(ctx, auth, begin(r, false))
	require(err)
	fail := finish(lost)
	fail.Outcome, fail.Damage, fail.HPRemaining = PersonalBossPlayerDefeated, 0, lost.BossHP
	r, _, err = game.FinishPersonalBoss(ctx, auth, fail)
	require(err)
	if r.PersonalBoss.Successes != 0 || len(r.PersonalBoss.Qualifications) != 0 || CanBookPersonalBoss(r, template, fail.FinishedAt) {
		t.Fatal("战败写入了证明或跳过了原失败冷却")
	}
	r = clearCooldown(r)
	r, trained, err := game.BeginPersonalBoss(ctx, auth, begin(r, false))
	require(err)
	r, victory, err := game.FinishPersonalBoss(ctx, auth, finish(trained))
	require(err)
	expected := domain.PersonalBossQualification{Fingerprint: proof, RuleVersion: cfg.Fixture.RuleVersion, ConfigHash: cfg.Fixture.Hash, ChallengeID: trained.ChallengeID}
	if r.PersonalBoss.Qualifications[key] != expected || victory.Automatic || !victory.RewardBound || !victory.RewardItems[0].Bound {
		t.Fatal("手动真实胜利未产生本档证明或改变原奖励判绑", victory)
	}
	r = clearCooldown(r)
	wrong := begin(r, true)
	wrong.Qualification = Hash([]byte("换装备后的条件"))
	reject(auth, wrong, p.CodeLineForbidden, "完成实战")
	other = seed(other, bson.M{"hosting_card_until": validUntil})
	reject(otherAuth, begin(other, true), p.CodeLineForbidden, "完成实战")
	r = seed(r, bson.M{"hosting_card_until": time.Now().Add(-time.Second), "privilege_until": validUntil})
	reject(auth, begin(r, true), p.CodeLineForbidden, "托管卡")

	r = seed(r, bson.M{"hosting_card_until": validUntil, "privilege_until": time.Now().Add(-time.Second)})
	auto := begin(r, true)
	r, active, err := game.BeginPersonalBoss(ctx, auth, auto)
	require(err)
	if !active.Automatic || active.Qualification != proof || active.DailyLimit != template.DailySuccessLimit {
		t.Fatal("自动预约没有固化来源或增加了日成功额度", active)
	}
	game.Cfg.Fixture.HostingBenefits.AutoPersonalBoss = false
	r = seed(r, bson.M{"hosting_card_until": time.Now().Add(-time.Second), "privilege_until": validUntil})
	retryRole, retryGrant, err := game.BeginPersonalBoss(ctx, auth, auto)
	require(err)
	if retryRole.Revision != r.Revision || !reflect.DeepEqual(active, retryGrant) {
		t.Fatal("重试没有返回原预约")
	}
	conflict := auto
	conflict.Automatic = false
	reject(auth, conflict, p.CodeOperationConflict, "身份或内容不符")
	win := finish(active)
	after, terminal, err := game.FinishPersonalBoss(ctx, auth, win)
	require(err)
	expected.ChallengeID = active.ChallengeID
	if !terminal.Automatic || terminal.RewardBound || terminal.RewardItems[0].Bound || terminal.RewardGold != template.KillRewardGold ||
		after.Wallet.Gold-r.Wallet.Gold != template.KillRewardGold || after.Wallet.BoundGold != r.Wallet.BoundGold ||
		after.SourceSeq != r.SourceSeq || after.PersonalBoss.Successes != 2 || after.PersonalBoss.Qualifications[key] != expected {
		t.Fatal("自动胜利未按当时特权发奖、只记一次共用次数并保存证明", terminal)
	}
	seed(after, bson.M{"privilege_until": time.Now().Add(-time.Second)})
	replayed, recorded, err := game.FinishPersonalBoss(ctx, auth, win)
	require(err)
	if !reflect.DeepEqual(recorded, terminal) || replayed.Wallet != after.Wallet || replayed.Revision != after.Revision {
		t.Fatal("重放根据后来权益重新计算了原终态")
	}
	audits, err := game.DB.Collection("audit_outbox").CountDocuments(ctx, bson.M{"_id": "personal_boss:" + terminal.ChallengeID + ":finish"})
	require(err)
	if audits != 1 {
		t.Fatalf("自动终态审计次数为 %d", audits)
	}
}
