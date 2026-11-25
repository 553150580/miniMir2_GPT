// 本文件在真实隔离库核验个人首领奖励从预约、击杀、换代到领取的永久资产流转。
package integration

import (
	"context"
	"os"
	"slices"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
)

// TestPersonalBossPendingRewardMongo 通过真实票据和两代运行服所有者核对待领队列与原操作重放。
func TestPersonalBossPendingRewardMongo(t *testing.T) {
	ctx, authority, oldOwner, newOwner := openChallengeRewardStores(t)
	ownerAccount, ownerKey := createChallengeRewardRole(t, ctx, authority)
	otherAccount, otherKey := createChallengeRewardRole(t, ctx, authority)
	role, auth := enterChallengeRewardRole(t, ctx, authority, oldOwner, ownerAccount, ownerKey)
	other, otherAuth := enterChallengeRewardRole(t, ctx, authority, oldOwner, otherAccount, otherKey)
	firstInfo, err := oldOwner.QueryPersonalBoss(ctx, auth)
	must(t, err)
	if !firstInfo.Open || firstInfo.EligibilityReason != 0 || firstInfo.PendingRewardCapacity != 1 || len(firstInfo.PendingRewards) != 0 {
		t.Fatalf("空队列的首领资格或容量错误：%+v", firstInfo)
	}

	// 已经满包的角色不能发起挑战。确定拒绝不得改变角色状态或创建挑战记录。
	other, otherFirstFiller := addChallengeRewardFiller(t, ctx, oldOwner, otherAuth, other)
	other, otherSecondFiller := addChallengeRewardFiller(t, ctx, oldOwner, otherAuth, other)
	assertChallengeRewardAdmission(t, ctx, oldOwner, otherAuth, other, 7)
	other = recycleChallengeRewardFillers(t, ctx, oldOwner, otherAuth, other, otherFirstFiller, otherSecondFiller)
	other, interrupted := beginChallengeRewardBoss(t, ctx, oldOwner, otherAuth, other, 2)
	assertChallengeRewardFinishRollback(t, ctx, oldOwner, otherAuth, other, interrupted)

	// 同一个角色预约成功后，普通来源仍可先填满背包，随后首领终态必须保存原奖励。
	role, challenge := beginChallengeRewardBoss(t, ctx, oldOwner, auth, role, 1)
	role, firstFiller := addChallengeRewardFiller(t, ctx, oldOwner, auth, role)
	role, secondFiller := addChallengeRewardFiller(t, ctx, oldOwner, auth, role)
	if len(role.Items) != role.Capacity {
		t.Fatal("普通结算未填满背包")
	}
	finish := challengeRewardFinish(challenge)
	full, terminal, err := oldOwner.FinishPersonalBoss(ctx, auth, finish)
	must(t, err)
	if terminal.Outcome != persistence.PersonalBossDefeated || full.SourceSeq != role.SourceSeq ||
		full.EconomicSeq != role.EconomicSeq || !slices.Equal(full.Items, role.Items) || len(full.PendingChallengeRewards) != 1 {
		t.Fatalf("击杀终态、原流水或满包待领奖励错误：%+v，待领 %v", terminal, full.PendingChallengeRewards)
	}
	pending := full.PendingChallengeRewards[0]
	assertChallengeRewardIdentity(t, pending, terminal, full.Origin)
	replayed, replayedTerminal, err := oldOwner.FinishPersonalBoss(ctx, auth, finish)
	must(t, err)
	if replayed.Revision != full.Revision || replayedTerminal.FinishedAt.UnixMilli() != terminal.FinishedAt.UnixMilli() ||
		len(replayed.PendingChallengeRewards) != 1 || replayed.PendingChallengeRewards[0].RewardID != pending.RewardID ||
		!slices.Equal(replayed.PendingChallengeRewards[0].Items, pending.Items) {
		t.Fatal("同一原击杀终态被重发或物品实例变化")
	}
	finishAudit := bson.M{"_id": "personal_boss:" + challenge.ChallengeID + ":finish", "role_id": full.ID,
		"kind": "personal_boss_finish", "challenge.outcome": persistence.PersonalBossDefeated}
	var audited struct {
		Challenge persistence.PersonalBossChallenge `bson:"challenge"`
	}
	must(t, authority.DB.Collection("audit_outbox").FindOne(ctx, finishAudit).Decode(&audited))
	if !audited.Challenge.RewardQueued || !slices.Equal(audited.Challenge.RewardItems, pending.Items) {
		t.Fatal("击杀审计未保留已提交的待领物品")
	}

	// 停止原游戏所有者，再使用原账号所选角色签发新票。新执行可以读取待领但不能续写旧预约。
	must(t, oldOwner.Release(ctx, "game"))
	grant, err := authority.ApproveGame(ctx, newOwner.Cfg.NodeID, newOwner.Instance, newOwner.Cfg.RunID)
	must(t, err)
	newOwner.Epoch = grant.Epoch
	if newOwner.Epoch != oldOwner.Epoch+1 {
		t.Fatal("运行服没有推进所有权代次")
	}
	role, newAuth := enterChallengeRewardRole(t, ctx, authority, newOwner, ownerAccount, ownerKey)
	other, newOtherAuth := enterChallengeRewardRole(t, ctx, authority, newOwner, otherAccount, otherKey)
	if role.SessionEpoch <= full.SessionEpoch || len(role.PendingChallengeRewards) != 1 || !slices.Equal(role.PendingChallengeRewards[0].Items, pending.Items) {
		t.Fatal("新会话没有保留同一待领奖励")
	}
	info, err := newOwner.QueryPersonalBoss(ctx, newAuth)
	must(t, err)
	if info.PendingRewardCapacity != 1 || len(info.PendingRewards) != 1 || info.PendingRewards[0].RewardID != pending.RewardID ||
		!slices.Equal(info.PendingRewards[0].Items, pending.Items) || len(info.History) != 1 || info.History[0].ChallengeID != challenge.ChallengeID {
		t.Fatalf("新所有者查询未返回原奖励及原终态：%+v", info)
	}
	if _, _, err = oldOwner.FinishPersonalBoss(ctx, otherAuth, challengeRewardFinish(interrupted)); persistence.Code(err) != p.CodeRouteChanged {
		t.Fatalf("已失权的执行仍可提交首领击杀，或返回错误码不符：%v", err)
	}
	otherAfter, cancelled, err := newOwner.RecoverPersonalBoss(ctx, newOtherAuth)
	must(t, err)
	if cancelled.ChallengeID != interrupted.ChallengeID || cancelled.Outcome != persistence.PersonalBossInterrupted || !cancelled.Recovered ||
		otherAfter.SourceSeq != other.SourceSeq || otherAfter.PersonalBoss == nil || otherAfter.PersonalBoss.ActiveChallengeID != "" ||
		len(otherAfter.PendingChallengeRewards) != 0 || !slices.Equal(otherAfter.Items, other.Items) {
		t.Fatalf("换代取消未释放预约，或非法发放奖励：%+v，待领 %v", cancelled, otherAfter.PendingChallengeRewards)
	}
	waitChallengeRewardBoundary(t, ctx, cancelled.AvailableAt)
	_, secondBooking := beginChallengeRewardBoss(t, ctx, newOwner, newOtherAuth, otherAfter, 3)
	if secondBooking.ChallengeID == interrupted.ChallengeID {
		t.Fatal("恢复后未产生独立的新预约")
	}
	_, _, err = newOwner.FinishPersonalBoss(ctx, newOtherAuth, persistence.PersonalBossFinish{
		ChallengeID: secondBooking.ChallengeID, ReservationID: secondBooking.ReservationID, Execution: secondBooking.Execution,
		RuleVersion: secondBooking.RuleVersion, ConfigHash: secondBooking.ConfigHash, Outcome: persistence.PersonalBossInterrupted,
		FinishedAt: time.Now().UTC(), HPRemaining: secondBooking.BossHP,
	})
	must(t, err)
	other, err = newOwner.Role(ctx, other.ID)
	must(t, err)
	foreignClaim := challengeRewardAsset(newOwner, other, persistence.AssetChallengeRewardClaim, pending.RewardID)
	foreignRole, result, foreignReceipt, err := newOwner.AssetWithReceipt(ctx, newOtherAuth, foreignClaim)
	assertChallengeRewardCode(t, result, err, p.CodeOperationConflict)
	if foreignReceipt.ChallengeReward != nil || !slices.Equal(foreignRole.Items, other.Items) || foreignRole.SourceSeq != other.SourceSeq {
		t.Fatal("其他角色看到或领取了待领奖励")
	}

	// 模拟只剩一格。失败必须原子保留两件奖励，并沿原操作键重放原失败。
	role = recycleChallengeRewardFillers(t, ctx, newOwner, newAuth, role, firstFiller)
	fullClaim := challengeRewardAsset(newOwner, role, persistence.AssetChallengeRewardClaim, pending.RewardID)
	refused, result, refusedReceipt, err := newOwner.AssetWithReceipt(ctx, newAuth, fullClaim)
	assertChallengeRewardCode(t, result, err, p.CodeEquipmentBagFull)
	if refusedReceipt.ID == "" || refusedReceipt.Code != p.CodeEquipmentBagFull || refusedReceipt.ChallengeReward != nil ||
		!slices.Equal(refused.Items, role.Items) || refused.SourceSeq != role.SourceSeq || len(refused.PendingChallengeRewards) != 1 ||
		!slices.Equal(refused.PendingChallengeRewards[0].Items, pending.Items) {
		t.Fatal("领取失败后部分发奖、改动来源流水或丢失待领")
	}
	role = recycleChallengeRewardFillers(t, ctx, newOwner, newAuth, refused, secondFiller)
	waitChallengeRewardBoundary(t, ctx, terminal.AvailableAt)
	assertChallengeRewardAdmission(t, ctx, newOwner, newAuth, role, 8)
	_, result, oldFailure, err := newOwner.AssetWithReceipt(ctx, newAuth, fullClaim)
	assertChallengeRewardCode(t, result, err, p.CodeEquipmentBagFull)
	if oldFailure.ID != refusedReceipt.ID || oldFailure.Revision != refusedReceipt.Revision {
		t.Fatal("背包清理后原失败操作没有重放原回执")
	}

	claim := challengeRewardAsset(newOwner, role, persistence.AssetChallengeRewardClaim, pending.RewardID)
	claimed, result, success, err := newOwner.AssetWithReceipt(ctx, newAuth, claim)
	assertChallengeRewardCode(t, result, err, 0)
	if success.ID == "" || success.Kind != persistence.AssetChallengeRewardClaim || success.RoleID != role.ID || len(claimed.Items) != len(role.Items)+2 ||
		claimed.SourceSeq != role.SourceSeq || claimed.EconomicSeq != role.EconomicSeq+1 || len(claimed.PendingChallengeRewards) != 0 || claimed.Wallet != role.Wallet {
		t.Fatalf("领取没有原子移出队列并加入两件物品：%+v", success)
	}
	assertChallengeRewardClaimReceipt(t, success.ChallengeReward, pending)
	for _, item := range pending.Items {
		if index := slices.Index(claimed.Items, item); index < 0 || slices.Contains(claimed.Items[index+1:], item) {
			t.Fatalf("原实例的绑定、来源或数量变化，或重复发放：%+v", item)
		}
	}
	// 同一成功请求的响应即使没有被客户端采纳，也只能观察已经确认的那一份结果。
	recovered, result, same, err := newOwner.AssetWithReceipt(ctx, newAuth, claim)
	assertChallengeRewardCode(t, result, err, 0)
	if same.ID != success.ID || same.Revision != success.Revision || recovered.Revision != claimed.Revision || !slices.Equal(recovered.Items, claimed.Items) {
		t.Fatal("领取成功重放时发生第二次库存或回执变化")
	}
	assertChallengeRewardClaimReceipt(t, same.ChallengeReward, pending)
	duplicate := challengeRewardAsset(newOwner, recovered, persistence.AssetChallengeRewardClaim, pending.RewardID)
	final, result, duplicateReceipt, err := newOwner.AssetWithReceipt(ctx, newAuth, duplicate)
	assertChallengeRewardCode(t, result, err, p.CodeOperationConflict)
	if duplicateReceipt.ChallengeReward != nil || final.SourceSeq != role.SourceSeq || !slices.Equal(final.Items, claimed.Items) || len(final.PendingChallengeRewards) != 0 {
		t.Fatal("异操作编号领取旧奖励改变了物品、待领队列或普通来源水位")
	}
	assertChallengeRewardPersistence(t, ctx, authority, success, final, pending)
	info, err = newOwner.QueryPersonalBoss(ctx, newAuth)
	must(t, err)
	if len(info.PendingRewards) != 0 || len(info.History) != 1 || !info.History[0].RewardQueued || !slices.Equal(info.History[0].RewardItems, pending.Items) {
		t.Fatal("领取后原击杀历史被改写，或目录仍显示已经领取的资产")
	}
}

// openChallengeRewardStores 只连接已登记的独立 Windows 副本集，并清理本例的随机数据库。
func openChallengeRewardStores(t *testing.T) (context.Context, *persistence.Store, *persistence.Store, *persistence.Store) {
	t.Helper()
	if os.Getenv("MINIMIR_MONGO_E2E") != "1" || os.Getenv("MINIMIR_INTEGRATION_TEST_MONGO_URI") == "" {
		t.Skip("待领奖励验收需要独立的真实 MongoDB 副本集")
	}
	cfg, err := loadIntegrationConfig()
	must(t, err)
	cfg.Database = "minimir_dev_game_test_pending_" + domain.ID()
	cfg.Fixture.InitialInventory = []uint32{218}
	cfg.Fixture.InitialSkillBooks = nil
	cfg.Fixture.InventoryCapacity = 3
	template := cfg.Fixture.PersonalBoss
	if template == nil {
		t.Fatal("缺少首领地图和基础模板")
	}
	template.SuccessCooldown, template.RetryCooldown = 20*time.Millisecond, 20*time.Millisecond
	template.ItemReward = &devconfig.ChallengeItemReward{PendingLimit: 1, Items: []devconfig.ChallengeRewardItem{
		{ItemID: 218, Quantity: 1}, {ItemID: 35, Quantity: 2},
	}}
	if !cfg.Fixture.ValidPersonalBoss() {
		t.Fatal("待领奖励的测试配置未通过校验")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	t.Cleanup(cancel)
	authority, err := persistence.Open(ctx, cfg)
	must(t, err)
	t.Cleanup(func() {
		cleanup, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
		defer authority.Close(cleanup)
		must(t, authority.DB.Drop(cleanup))
	})
	oldOwner, err := persistence.Open(ctx, cfg)
	must(t, err)
	t.Cleanup(func() { oldOwner.Close(context.Background()) })
	newOwner, err := persistence.Open(ctx, cfg)
	must(t, err)
	t.Cleanup(func() { newOwner.Close(context.Background()) })
	must(t, authority.Singleton(ctx, "account"))
	grant, err := authority.ApproveGame(ctx, cfg.NodeID, oldOwner.Instance, cfg.RunID)
	must(t, err)
	oldOwner.Epoch = grant.Epoch
	return ctx, authority, oldOwner, newOwner
}

// createChallengeRewardRole 通过正式建角入口生成独立账号与角色键。
func createChallengeRewardRole(t *testing.T, ctx context.Context, authority *persistence.Store) (persistence.Auth, *pb.RoleKey) {
	t.Helper()
	login, err := authority.Login(ctx, "pending_"+domain.ID()[:8], "test_123456", true)
	must(t, err)
	account, err := authority.Authenticate(ctx, login.AccountToken)
	must(t, err)
	created, err := authority.CreateRole(ctx, account, &pb.CreateRoleRequest{
		OriginServerId: 1, Slot: 1, Job: 0, Name: "待领" + domain.ID()[:6], OpId: domain.Bytes(domain.ID()),
	})
	must(t, err)
	return account, created.Key
}

// enterChallengeRewardRole 使用当前路由的新签票获取角色会话和所有权。
func enterChallengeRewardRole(t *testing.T, ctx context.Context, authority, owner *persistence.Store, account persistence.Auth, key *pb.RoleKey) (domain.Role, persistence.Auth) {
	t.Helper()
	ticket, err := authority.IssueTicket(ctx, account, key)
	must(t, err)
	role, auth, err := owner.Enter(ctx, ticket.Ticket)
	must(t, err)
	return role, auth
}

// challengeRewardBegin 构造当前角色版本下的独立首领预约。
func challengeRewardBegin(owner *persistence.Store, role domain.Role, execution uint64) persistence.PersonalBossBegin {
	f := owner.Cfg.Fixture
	cfg := f.PersonalBoss
	return persistence.PersonalBossBegin{ChallengeID: domain.ID(), Execution: execution, TemplateID: cfg.TemplateID,
		CombatSnapshot: persistence.Hash([]byte("待领奖励测试的挑战快照")), ExpectedRevision: role.Revision,
		RuleVersion: f.RuleVersion, ConfigHash: f.Hash, StartedAt: time.Now().UTC(), Duration: cfg.Duration,
		SuccessCooldown: cfg.SuccessCooldown, RetryCooldown: cfg.RetryCooldown, BossHP: cfg.Monster.HP}
}

// beginChallengeRewardBoss 只接受真正写入了活动预约的入场结果。
func beginChallengeRewardBoss(t *testing.T, ctx context.Context, owner *persistence.Store, auth persistence.Auth, role domain.Role, execution uint64) (domain.Role, persistence.PersonalBossChallenge) {
	t.Helper()
	request := challengeRewardBegin(owner, role, execution)
	entered, challenge, err := owner.BeginPersonalBoss(ctx, auth, request)
	must(t, err)
	if challenge.ChallengeID != request.ChallengeID || challenge.Outcome != persistence.PersonalBossActive || entered.SourceSeq != role.SourceSeq ||
		entered.PersonalBoss == nil || entered.PersonalBoss.ActiveChallengeID != challenge.ChallengeID {
		t.Fatalf("未确认原活动预约：%+v", challenge)
	}
	return entered, challenge
}

// challengeRewardFinish 保留原预约身份以及一次明确的击杀终态。
func challengeRewardFinish(challenge persistence.PersonalBossChallenge) persistence.PersonalBossFinish {
	return persistence.PersonalBossFinish{ChallengeID: challenge.ChallengeID, ReservationID: challenge.ReservationID,
		Execution: challenge.Execution, RuleVersion: challenge.RuleVersion, ConfigHash: challenge.ConfigHash,
		Outcome: persistence.PersonalBossDefeated, FinishedAt: time.Now().UTC(), Damage: challenge.BossHP}
}

// assertChallengeRewardAdmission 核对只读理由和写入口的拒绝，保证拒绝没有消耗预约容量。
func assertChallengeRewardAdmission(t *testing.T, ctx context.Context, owner *persistence.Store, auth persistence.Auth, role domain.Role, reason uint32) {
	t.Helper()
	info, err := owner.QueryPersonalBoss(ctx, auth)
	must(t, err)
	if info.EligibilityReason != reason {
		t.Fatalf("预约资格原因=%d，期望 %d", info.EligibilityReason, reason)
	}
	request := challengeRewardBegin(owner, role, 4)
	if _, _, err = owner.BeginPersonalBoss(ctx, auth, request); persistence.Code(err) != p.CodeEquipmentBagFull {
		t.Fatalf("拒绝预约的错误码不符：%v", err)
	}
	count, err := owner.DB.Collection("personal_boss_challenges").CountDocuments(ctx, bson.M{"_id": request.ChallengeID})
	must(t, err)
	after, err := owner.Role(ctx, role.ID)
	must(t, err)
	if count != 0 || after.Revision != role.Revision || after.SourceSeq != role.SourceSeq {
		t.Fatal("拒绝预约仍生成记录或修改角色水位")
	}
}

// assertChallengeRewardFinishRollback 使击杀审计的唯一键写失败，核对同事务已经处理的角色和挑战更新均回滚。
func assertChallengeRewardFinishRollback(t *testing.T, ctx context.Context, owner *persistence.Store, auth persistence.Auth, role domain.Role, challenge persistence.PersonalBossChallenge) {
	t.Helper()
	audit := owner.DB.Collection("audit_outbox")
	collision := bson.M{"_id": "personal_boss:" + challenge.ChallengeID + ":finish", "purpose": "test_pending_reward_rollback"}
	_, err := audit.InsertOne(ctx, collision)
	must(t, err)
	if _, _, err = owner.FinishPersonalBoss(ctx, auth, challengeRewardFinish(challenge)); err == nil {
		t.Fatal("缺少审计写入时仍确认了击杀")
	}
	stored, err := owner.Role(ctx, role.ID)
	must(t, err)
	if stored.Revision != role.Revision || stored.SourceSeq != role.SourceSeq || !slices.Equal(stored.Items, role.Items) ||
		stored.PersonalBoss == nil || stored.PersonalBoss.ActiveChallengeID != challenge.ChallengeID || len(stored.PendingChallengeRewards) != 0 {
		t.Fatal("审计失败后角色、原预约或奖励未回滚")
	}
	filter := bson.M{"_id": challenge.ChallengeID, "outcome": persistence.PersonalBossActive}
	must(t, owner.DB.Collection("personal_boss_challenges").FindOne(ctx, filter).Err())
	_, err = audit.DeleteOne(ctx, collision)
	must(t, err)
}

// addChallengeRewardFiller 从普通收益入口加入一件以后可手动回收的装备。
func addChallengeRewardFiller(t *testing.T, ctx context.Context, owner *persistence.Store, auth persistence.Auth, role domain.Role) (domain.Role, string) {
	t.Helper()
	reward := domain.Item{ID: domain.ID(), ItemID: 218, Quantity: 1, Version: 1, Bound: true, SourceKind: "monster_drop", SourceID: domain.ID()}
	after, receipt, err := owner.SettleWithReceipt(ctx, auth, persistence.Settlement{
		SourceSeq: role.SourceSeq + 1, Item: &reward, Bound: true, Kills: 1,
		MapID: owner.Cfg.Fixture.Map.ID, MonsterID: owner.Cfg.Fixture.Monsters[0].ID, EventAt: time.Now().UTC(),
	})
	must(t, err)
	if receipt.Settlement == nil || !receipt.Settlement.GrantedItem || after.SourceSeq != role.SourceSeq+1 || len(after.Items) != len(role.Items)+1 {
		t.Fatalf("普通来源未确认填入背包：%+v", receipt.Settlement)
	}
	return after, reward.ID
}

// recycleChallengeRewardFillers 通过玩家回收命令实际释放背包格。
func recycleChallengeRewardFillers(t *testing.T, ctx context.Context, owner *persistence.Store, auth persistence.Auth, role domain.Role, items ...string) domain.Role {
	t.Helper()
	request := challengeRewardAsset(owner, role, "recycle", items[0])
	request.ItemIDs = items
	after, result, err := owner.Asset(ctx, auth, request)
	assertChallengeRewardCode(t, result, err, 0)
	if after.SourceSeq != role.SourceSeq || len(after.Items) != len(role.Items)-len(items) {
		t.Fatal("实际回收未释放对应的背包格，或改动普通来源水位")
	}
	return after
}

// challengeRewardAsset 固定原操作身份和当前版本，用于提交和原请求重放。
func challengeRewardAsset(owner *persistence.Store, role domain.Role, kind, target string) persistence.AssetRequest {
	request := persistence.AssetRequest{Kind: kind, Fingerprint: persistence.Hash([]byte(kind + ":" + target)),
		Context: &pb.OperationContext{OpId: domain.Bytes(domain.ID()), CommandSeq: uint64(role.EconomicSeq + 1),
			ExpectedRevision: uint64(role.Revision), ExpectedRuleVersion: owner.Cfg.Fixture.RuleVersion}}
	if kind == persistence.AssetChallengeRewardClaim {
		request.ChallengeRewardID = target
	}
	return request
}

// assertChallengeRewardCode 同时兼容事务拒绝和已持久化的业务错误码。
func assertChallengeRewardCode(t *testing.T, result *pb.OperationResult, err error, expected uint32) {
	t.Helper()
	code := persistence.Code(err)
	if err == nil && result != nil {
		code = result.ErrorCode
	}
	if code != expected || expected == 0 && (err != nil || result == nil) {
		t.Fatalf("资产操作返回码=%d，期望 %d，错误=%v", code, expected, err)
	}
}

// assertChallengeRewardIdentity 核对原场次、规则、获得时刻和两件奖励的永久实例。
func assertChallengeRewardIdentity(t *testing.T, reward domain.ChallengePendingReward, terminal persistence.PersonalBossChallenge, origin uint64) {
	t.Helper()
	if reward.RewardID != terminal.ChallengeID || reward.SourceKind != "personal_boss" || reward.SourceID != terminal.ChallengeID || reward.Origin != origin ||
		reward.TemplateID != terminal.TemplateID || reward.RuleVersion != terminal.RuleVersion || !reward.EarnedAt.Equal(terminal.FinishedAt) ||
		len(reward.Items) != 2 || reward.Items[0].ID == reward.Items[1].ID || !terminal.RewardQueued || !slices.Equal(terminal.RewardItems, reward.Items) {
		t.Fatalf("待领奖励没有保留原身份：%+v", reward)
	}
	for _, item := range reward.Items {
		count, ok := map[uint32]int64{218: 1, 35: 2}[item.ItemID]
		if !ok || item.Quantity != count || item.ID == "" || !item.Bound || item.SourceKind != reward.SourceKind || item.SourceID != reward.SourceID {
			t.Fatalf("奖励实例未保留配置或击杀绑定：%+v", item)
		}
	}
	if reward.Items[0].ItemID == reward.Items[1].ItemID {
		t.Fatal("奖励包含重复模板，未确认完整组合")
	}
}

// assertChallengeRewardClaimReceipt 核对回执中的原获奖时间、来源及已实际领取的实例。
func assertChallengeRewardClaimReceipt(t *testing.T, actual *domain.ChallengePendingReward, pending domain.ChallengePendingReward) {
	t.Helper()
	if actual == nil || actual.RewardID != pending.RewardID || actual.SourceKind != pending.SourceKind || actual.SourceID != pending.SourceID ||
		actual.Origin != pending.Origin || actual.TemplateID != pending.TemplateID || actual.RuleVersion != pending.RuleVersion ||
		!actual.EarnedAt.Equal(pending.EarnedAt) || !slices.Equal(actual.Items, pending.Items) {
		t.Fatalf("领取回执没有记录原奖励实际入包的实例：%+v", actual)
	}
}

// waitChallengeRewardBoundary 等候真实冷却截止，使容量断言免受上一场冷却干扰。
func waitChallengeRewardBoundary(t *testing.T, ctx context.Context, availableAt time.Time) {
	t.Helper()
	timer := time.NewTimer(time.Until(availableAt.Add(time.Millisecond)))
	defer timer.Stop()
	select {
	case <-timer.C:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
}

// assertChallengeRewardPersistence 从独立客户端核对库存、原成功回执和对应审计只有一份。
func assertChallengeRewardPersistence(t *testing.T, ctx context.Context, authority *persistence.Store, success persistence.Operation, final domain.Role, pending domain.ChallengePendingReward) {
	t.Helper()
	stored, err := authority.Role(ctx, final.ID)
	must(t, err)
	if stored.SourceSeq != final.SourceSeq || stored.Revision != final.Revision || len(stored.PendingChallengeRewards) != 0 || !slices.Equal(stored.Items, final.Items) {
		t.Fatal("数据库库存、队列或游标与当前角色结果不一致")
	}
	operation := bson.M{"role_id": final.ID, "kind": persistence.AssetChallengeRewardClaim, "code": 0}
	count, err := authority.DB.Collection("operations").CountDocuments(ctx, operation)
	must(t, err)
	if count != 1 {
		t.Fatalf("成功领取永久回执数=%d", count)
	}
	operation["_id"] = success.ID
	var storedOperation persistence.Operation
	must(t, authority.DB.Collection("operations").FindOne(ctx, operation).Decode(&storedOperation))
	assertChallengeRewardClaimReceipt(t, storedOperation.ChallengeReward, pending)
	audit := bson.M{"role_id": final.ID, "kind": persistence.AssetChallengeRewardClaim, "result_code": 0}
	count, err = authority.DB.Collection("audit_outbox").CountDocuments(ctx, audit)
	must(t, err)
	if count != 1 {
		t.Fatalf("成功领取审计数=%d", count)
	}
	audit["_id"] = success.ID
	var storedAudit struct {
		ChallengeReward *domain.ChallengePendingReward `bson:"challenge_reward_claim"`
	}
	must(t, authority.DB.Collection("audit_outbox").FindOne(ctx, audit).Decode(&storedAudit))
	assertChallengeRewardClaimReceipt(t, storedAudit.ChallengeReward, pending)
}
