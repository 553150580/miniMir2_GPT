// 本文件对拍讨伐链 AI 开战费用、普通终态和三条系统退款路径。
package persistence

import (
	"context"
	"math"
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

// TestDungeonHireBeginDigest 既保留无 AI 的原开始摘要，也要求付费角色及附加席顺序固定。
func TestDungeonHireBeginDigest(t *testing.T) {
	type legacyDungeonBegin struct {
		ChallengeID string
		Execution   uint64
		TemplateID  uint32
		RuleVersion uint64
		ConfigHash  string
		StartedAt   time.Time
		Duration    time.Duration
		Members     []DungeonAdmissionMember
	}
	q := DungeonBegin{ChallengeID: domain.ID(), Execution: 1, TemplateID: 1, RuleVersion: 30, ConfigHash: strings.Repeat("c", 64),
		StartedAt: time.Now(), Duration: time.Minute, Members: []DungeonAdmissionMember{{Auth: Auth{RoleID: domain.ID()},
			ExpectedRevision: 3, CombatSnapshot: strings.Repeat("a", 64)}}, HireOptionIDs: []uint32{}}
	normal, err := normalizeDungeonBegin(q)
	if err != nil || normal.HireOptionIDs != nil {
		t.Fatal("旧无 AI 请求归一化发生变化", normal, err)
	}
	legacy := legacyDungeonBegin{normal.ChallengeID, normal.Execution, normal.TemplateID, normal.RuleVersion, normal.ConfigHash,
		normal.StartedAt, normal.Duration, normal.Members}
	want, err := bossDigest(legacy)
	if err != nil {
		t.Fatal(err)
	}
	got, err := bossDigest(normal)
	if err != nil || got != want {
		t.Fatal("已有无 AI 入场请求摘要被改变", got, want, err)
	}
	for _, tc := range []struct {
		leader string
		hires  []uint32
	}{
		{q.Members[0].Auth.RoleID, nil},
		{q.Members[0].Auth.RoleID, []uint32{0}},
		{q.Members[0].Auth.RoleID, []uint32{1, 2, 3}},
		{domain.ID(), []uint32{1}},
		{"", []uint32{1}},
	} {
		q.LeaderRoleID, q.HireOptionIDs = tc.leader, tc.hires
		if _, err = normalizeDungeonBegin(q); Code(err) != p.CodeInvalidArgument {
			t.Fatal("应拒绝原真人名单之外的付款者或非法 AI 席", tc, err)
		}
	}
	q.LeaderRoleID, q.HireOptionIDs = q.Members[0].Auth.RoleID, []uint32{2, 1}
	normal, err = normalizeDungeonBegin(q)
	if err != nil {
		t.Fatal(err)
	}
	paid, err := bossDigest(normal)
	if err != nil || paid == got {
		t.Fatal("AI 名单和付款者未进入原摘要", paid, err)
	}
	q.HireOptionIDs[0], q.HireOptionIDs[1] = q.HireOptionIDs[1], q.HireOptionIDs[0]
	reordered, err := normalizeDungeonBegin(q)
	if err != nil {
		t.Fatal(err)
	}
	changed, err := bossDigest(reordered)
	if err != nil || changed == paid || normal.HireOptionIDs[0] != 2 {
		t.Fatal("原附加席顺序被改写或未固定", changed, paid, err)
	}
}

// TestDungeonHirePaymentAndRefundMongo 验证原事务仅从队长扣费，三个中断入口原路退款且拒绝重放重复写入。
func TestDungeonHirePaymentAndRefundMongo(t *testing.T) {
	uri := os.Getenv("MINIMIR_INTEGRATION_TEST_MONGO_URI")
	if os.Getenv("MINIMIR_MONGO_E2E") != "1" || uri == "" {
		t.Skip("需要本任务专属 MongoDB 副本集")
	}
	if !regexp.MustCompile(`^mongodb://127\.0\.0\.1:[1-9][0-9]{3,4}/\?replicaSet=minimirLoad_[a-f0-9]{32}&directConnection=true$`).MatchString(uri) {
		t.Fatal("联机测试须由任务专属控制器提供回环副本集")
	}
	ctx, stop := context.WithTimeout(t.Context(), 40*time.Second)
	defer stop()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	wantCode := func(err error, status uint32) {
		t.Helper()
		if Code(err) != status {
			t.Fatalf("处理结果=%d 预期=%d 错误=%v", Code(err), status, err)
		}
	}
	cfg, err := devconfig.Load("../../../conf/dev/server.json")
	must(err)
	cfg.MongoURI, cfg.Database = uri, "minimir_dev_game_test_hires_"+domain.ID()[:16]
	account, err := Open(ctx, cfg)
	must(err)
	t.Cleanup(func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		defer account.Close(cleanup)
		if err := account.DB.Drop(cleanup); err != nil {
			t.Error(err)
		}
	})
	must(account.Singleton(ctx, "account"))
	openGame := func() *Store {
		t.Helper()
		game, openErr := Open(ctx, cfg)
		must(openErr)
		t.Cleanup(func() { game.Close(context.Background()) })
		lease, approveErr := account.ApproveGame(ctx, cfg.NodeID, game.Instance, cfg.RunID)
		must(approveErr)
		game.Epoch = lease.Epoch
		return game
	}
	game := openGame()
	// 隔离环境只安装两档人工测试费，生产发布规则和工作簿保持原状。
	game.Cfg.Fixture.Dungeons = []devconfig.DungeonConfig{{TemplateID: 1, MinimumLevel: 1, Duration: time.Minute,
		Hires: []devconfig.DungeonHireOption{{ID: 1, FeeGold: 11}, {ID: 2, FeeGold: 19}}}}
	register := func(name string, wallet domain.Wallet) Auth {
		t.Helper()
		login, createErr := account.Login(ctx, "dhire_"+domain.ID()[:8], "test_123456", true)
		must(createErr)
		customer, authErr := account.Authenticate(ctx, login.AccountToken)
		must(authErr)
		created, roleErr := account.CreateRole(ctx, customer, &pb.CreateRoleRequest{OriginServerId: cfg.OriginID,
			Slot: 1, Name: name, OpId: domain.Bytes(domain.ID())})
		must(roleErr)
		ticket, ticketErr := account.IssueTicket(ctx, customer, created.Key)
		must(ticketErr)
		role, auth, enterErr := game.Enter(ctx, ticket.Ticket)
		must(enterErr)
		_, walletErr := account.DB.Collection("roles").UpdateByID(ctx, role.ID, bson.M{"$set": bson.M{"wallet": wallet}})
		must(walletErr)
		return auth
	}
	aWallet := domain.Wallet{Gold: 90, BoundGold: 7, Yuanbao: 12, BoundYuanbao: 3}
	bWallet := domain.Wallet{Gold: math.MaxInt64 - 1, BoundGold: 5}
	cWallet := domain.Wallet{Gold: 33, BoundGold: 13}
	poorWallet := domain.Wallet{Gold: 3, BoundGold: 2}
	a, b := register("队长甲", aWallet), register("队友乙", bWallet)
	c, poor := register("队友丙", cWallet), register("欠费队长", poorWallet)
	loaded := func(auth Auth) domain.Role {
		t.Helper()
		role, roleErr := account.Role(ctx, auth.RoleID)
		must(roleErr)
		return role
	}
	request := func(leader Auth, hires []uint32, players ...Auth) DungeonBegin {
		t.Helper()
		begin := DungeonBegin{ChallengeID: domain.ID(), Execution: 1, TemplateID: 1, RuleVersion: cfg.Fixture.RuleVersion,
			ConfigHash: cfg.Fixture.Hash, Duration: time.Minute, StartedAt: bossTime(time.Now()), LeaderRoleID: leader.RoleID, HireOptionIDs: hires}
		for _, auth := range players {
			begin.Members = append(begin.Members, DungeonAdmissionMember{Auth: auth, ExpectedRevision: loaded(auth).Revision,
				CombatSnapshot: strings.Repeat("a", 64)})
		}
		return begin
	}
	wantSource := make(map[string]int64) // 无关收益来源仅在下方并发入账边界样本推进。
	checkState := func(auth Auth, wallet domain.Wallet, entries uint32, active string) {
		t.Helper()
		role := loaded(auth)
		progress := dungeonProgress(role)
		if role.Wallet != wallet || progress.Entries != entries || progress.ActiveChallengeID != active || role.EconomicSeq != 0 || role.SourceSeq != wantSource[auth.RoleID] ||
			active == "" && (progress.RefundableHireGold != 0 || progress.RefundableHireBoundGold != 0) {
			t.Fatal("原扣次、费用、独占状态或资产序号不一致", role.Wallet, progress, role.EconomicSeq, role.SourceSeq)
		}
	}
	checkAudit := func(grant DungeonChallenge, phase string, payer Auth, refunded bool) {
		t.Helper()
		for _, member := range grant.Members {
			var audit struct {
				Hires    []uint32           `bson:"hire_option_ids"`
				Charge   *DungeonHireCharge `bson:"hire_charge"`
				Refunded bool               `bson:"hire_refunded"`
			}
			id := "dungeon:" + grant.ChallengeID + ":" + phase + ":" + member.RoleID
			must(account.DB.Collection("audit_outbox").FindOne(ctx, bson.M{"_id": id}).Decode(&audit))
			if !slices.Equal(audit.Hires, grant.HireOptionIDs) || member.RoleID != payer.RoleID && audit.Charge != nil ||
				member.RoleID == payer.RoleID && (!reflect.DeepEqual(audit.Charge, grant.HireCharge) || audit.Refunded != refunded) {
				t.Fatal("场次审计缺少原 AI 名单、付款归属或原路退款事实", audit)
			}
		}
	}
	finish := func(grant DungeonChallenge, outcome DungeonOutcome) DungeonFinish {
		return DungeonFinish{ChallengeID: grant.ChallengeID, ReservationID: grant.ReservationID, Execution: grant.Execution,
			Outcome: outcome, FinishedAt: bossTime(time.Now())}
	}
	// 队友富有也不可代付；资金或候选错误均不占持久名额。
	short := request(poor, []uint32{1}, a, poor)
	_, _, err = game.BeginDungeon(ctx, short)
	wantCode(err, p.CodeVendorFunds)
	undefined := request(a, []uint32{99}, a, b)
	_, _, err = game.BeginDungeon(ctx, undefined)
	wantCode(err, p.CodeLineForbidden)
	checkState(poor, poorWallet, 0, "")
	checkState(a, aWallet, 0, "")
	checkState(b, bWallet, 0, "")
	count, err := account.DB.Collection(dungeonCollection).CountDocuments(ctx, bson.M{})
	must(err)
	if count != 0 {
		t.Fatal("未受理费用仍生成持久挑战", count)
	}

	// 同一开始请求响应未知时可并发重放；普通失败照常保留原扣次和原费用。
	begin := request(a, []uint32{1}, b, a)
	var starts [2]DungeonChallenge
	var failures [2]error
	var wait sync.WaitGroup
	for i := range starts {
		wait.Go(func() { starts[i], _, failures[i] = game.BeginDungeon(ctx, begin) })
	}
	wait.Wait()
	for _, trialErr := range failures {
		must(trialErr)
	}
	paid := starts[0]
	if !reflect.DeepEqual(paid, starts[1]) || len(paid.Members) != 2 || !slices.Equal(paid.HireOptionIDs, []uint32{1}) ||
		paid.HireCharge == nil || *paid.HireCharge != (DungeonHireCharge{RoleID: a.RoleID, Gold: 4, BoundGold: 7}) {
		t.Fatal("双钱包顺序、AI 与真人分离或并发原回执不一致", paid, starts[1])
	}
	aRemaining := aWallet
	aRemaining.Gold, aRemaining.BoundGold = 86, 0
	checkState(a, aRemaining, 1, paid.ChallengeID)
	checkState(b, bWallet, 1, paid.ChallengeID)
	checkAudit(paid, "begin", a, false)
	changed := begin
	changed.LeaderRoleID = b.RoleID
	_, _, err = game.BeginDungeon(ctx, changed)
	wantCode(err, p.CodeOperationConflict)
	changed = begin
	changed.HireOptionIDs = []uint32{2}
	_, _, err = game.BeginDungeon(ctx, changed)
	wantCode(err, p.CodeOperationConflict)
	loss := finish(paid, DungeonFailed)
	closed, _, err := game.FinishOwnedDungeon(ctx, loss)
	must(err)
	if closed.Refunded || !reflect.DeepEqual(closed.HireCharge, paid.HireCharge) {
		t.Fatal("正常战败改写了 AI 原费用", closed)
	}
	loss.Outcome = DungeonSystemInterrupted
	_, _, err = game.FinishOwnedDungeon(ctx, loss)
	wantCode(err, p.CodeOperationConflict)
	checkState(a, aRemaining, 1, "")
	checkState(b, bWallet, 1, "")
	checkAudit(closed, "finish", a, false)

	// 原执行明确中断后，两个附加席的费用各归原钱包；重复确认仍看同一金额。
	graceful := request(b, []uint32{2, 1}, b)
	grant, _, err := game.BeginDungeon(ctx, graceful)
	must(err)
	if !slices.Equal(grant.HireOptionIDs, graceful.HireOptionIDs) || len(grant.Members) != 1 ||
		grant.HireCharge == nil || *grant.HireCharge != (DungeonHireCharge{RoleID: b.RoleID, Gold: 25, BoundGold: 5}) {
		t.Fatal("未按开始顺序和两个 AI 报价确认", grant)
	}
	bRemaining := bWallet
	bRemaining.Gold, bRemaining.BoundGold = bWallet.Gold-25, 0
	checkState(b, bRemaining, 2, grant.ChallengeID)
	progress := dungeonProgress(loaded(b))
	if progress.RefundableHireGold != 25 || progress.RefundableHireBoundGold != 5 {
		t.Fatal("两个钱包未固定各自的退款容量", progress)
	}
	// 普通资产更新和真实批量收益写入都不能消耗某个原钱包的可退容量。
	invalid := loaded(b)
	invalid.Wallet.BoundGold = math.MaxInt64 - 4
	invalid.Revision++
	wantCode(game.replace(ctx, invalid, invalid.Revision-1), p.CodeInvalidArgument)
	_, err = game.Settle(ctx, b, Settlement{SourceSeq: 1, Gold: 2})
	wantCode(err, p.CodeInvalidArgument)
	checkState(b, bRemaining, 2, grant.ChallengeID)
	settled, err := game.Settle(ctx, b, Settlement{SourceSeq: 1, Gold: 1})
	must(err)
	if settled.Wallet.Gold != bRemaining.Gold+1 || settled.SourceSeq != 1 {
		t.Fatal("未超过预留容量的合法普通收益没有真正入账", settled.Wallet, settled.SourceSeq)
	}
	bWallet.Gold++
	wantSource[b.RoleID] = 1
	checkAudit(grant, "begin", b, false)
	gracefulResult := finish(grant, DungeonSystemInterrupted)
	for range 2 {
		refund, _, finishErr := game.FinishOwnedDungeon(ctx, gracefulResult)
		must(finishErr)
		if !refund.Refunded || refund.Recovered || !reflect.DeepEqual(refund.HireCharge, grant.HireCharge) {
			t.Fatal("原执行中断未确认退款", refund)
		}
		checkState(b, bWallet, 1, "")
		checkAudit(refund, "finish", b, true)
	}

	// 原账号换代且还未安装战斗时，只有同一起始摘要能退款，旧开始不能补扣。
	unattached := request(c, []uint32{2}, a, c)
	grant, _, err = game.BeginDungeon(ctx, unattached)
	must(err)
	_, err = account.DB.Collection("accounts").UpdateByID(ctx, c.AccountID, bson.M{"$inc": bson.M{"auth_version": 1}})
	must(err)
	_, _, err = game.BeginDungeon(ctx, unattached)
	wantCode(err, p.CodeUnauthenticated)
	changed = unattached
	changed.HireOptionIDs = []uint32{1}
	_, _, err = game.AbortUnattachedDungeon(ctx, changed)
	wantCode(err, p.CodeOperationConflict)
	for range 2 {
		refund, _, abortErr := game.AbortUnattachedDungeon(ctx, unattached)
		must(abortErr)
		if !refund.Refunded || refund.ReservationID != grant.ReservationID {
			t.Fatal("未实际装场的已确认开战未退原费用", refund)
		}
		checkState(c, cWallet, 0, "")
		checkState(a, aRemaining, 1, "")
		checkAudit(refund, "finish", c, true)
	}

	// 冷换代保持费用原出处，不要求原付费队长重新登录，亦不重新读取已关闭的报价。
	coldBegin := request(b, []uint32{1}, a, b)
	grant, _, err = game.BeginDungeon(ctx, coldBegin)
	must(err)
	must(game.Release(ctx, "game"))
	cold := openGame()
	if len(cold.Cfg.Fixture.Dungeons) != 0 {
		t.Fatal("冷恢复应使用已关闭的配置表，退款仍须沿原冻结费用")
	}
	for range 2 {
		refund, _, recoverErr := cold.RecoverDungeon(ctx, grant.ChallengeID)
		must(recoverErr)
		if !refund.Refunded || !refund.Recovered || !reflect.DeepEqual(refund.HireCharge, grant.HireCharge) {
			t.Fatal("新代次未按旧回执仅退一次", refund)
		}
		checkState(a, aRemaining, 1, "")
		checkState(b, bWallet, 1, "")
		checkAudit(refund, "finish", b, true)
	}
	count, err = account.DB.Collection(dungeonCollection).CountDocuments(ctx, bson.M{})
	must(err)
	if count != 4 {
		t.Fatal("并发原请求、退款重试或真人扣次产生额外场次", count)
	}
}
