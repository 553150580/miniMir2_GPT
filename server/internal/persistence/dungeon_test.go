// 本文件对拍讨伐链原运营日和三人扣次事务，并验证同场新票及运行服换代边界。
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

// TestDungeonEntryOriginalOperatingDay 验证上海零点、卡到期等号和旧日退款对新日的隔离。
func TestDungeonEntryOriginalOperatingDay(t *testing.T) {
	rollover := time.Date(2026, time.November, 25, 16, 0, 0, 0, time.UTC)
	before := rollover.Add(-time.Millisecond)
	role := domain.Role{ID: "原角色", AccountID: "原账号", Origin: 10, Level: 30, Revision: 4,
		HostingCardUntil: rollover, PrivilegeUntil: rollover.Add(time.Hour),
		Dungeon: &domain.DungeonProgress{EntryDay: "2026-11-25", Entries: 3}}
	if dungeonEntryLimit(role, before) != 4 || dungeonEntryLimit(role, rollover) != 3 {
		t.Fatal("托管到期等号或特权卡限制错误")
	}
	q := DungeonBegin{ChallengeID: "第一次原场次", StartedAt: before}
	member, err := chargeDungeonMember(&role, q, DungeonAdmissionMember{ExpectedRevision: 4}, 30, shanghaiOperatingDay(before))
	if err != nil || role.Dungeon.Entries != 4 || member.EntryDay != "2026-11-25" {
		t.Fatal("上海零点前最后一扣未进入原运营日", role.Dungeon, err)
	}
	if err = closeDungeonMember(&role, DungeonChallenge{ChallengeID: q.ChallengeID, FinishedAt: rollover, Refunded: true}, member); err != nil ||
		role.Dungeon.Entries != 3 || role.Dungeon.EntryDay != "2026-11-25" {
		t.Fatal("跨日系统中断未退回原扣次日期", role.Dungeon, err)
	}
	q = DungeonBegin{ChallengeID: "第二次原场次", StartedAt: rollover}
	member, err = chargeDungeonMember(&role, q, DungeonAdmissionMember{ExpectedRevision: 6}, 30, shanghaiOperatingDay(rollover))
	if err != nil || role.Dungeon.Entries != 1 || member.EntryDay != "2026-11-26" || member.DailyLimit != 3 {
		t.Fatal("跨日后新角色扣次不正确", role.Dungeon, member, err)
	}
	if err = closeDungeonMember(&role, DungeonChallenge{ChallengeID: q.ChallengeID, Outcome: DungeonFailed, FinishedAt: rollover.Add(time.Minute)}, member); err != nil ||
		role.Dungeon.Entries != 1 || role.Dungeon.ActiveChallengeID != "" {
		t.Fatal("普通失败不应退次数或保留活动占用", role.Dungeon, err)
	}
}

// TestDungeonPartyDurableMongo 用真实账号、游戏票、角色与运行服写围栏验证原扣次及终态。
func TestDungeonPartyDurableMongo(t *testing.T) {
	uri := os.Getenv("MINIMIR_INTEGRATION_TEST_MONGO_URI")
	if os.Getenv("MINIMIR_MONGO_E2E") != "1" || uri == "" {
		t.Skip("需要本任务专属 MongoDB 副本集")
	}
	if !regexp.MustCompile(`^mongodb://127\.0\.0\.1:[1-9][0-9]{3,4}/\?replicaSet=minimirLoad_[a-f0-9]{32}&directConnection=true$`).MatchString(uri) {
		t.Fatal("联机测试须由任务专属控制器提供回环副本集")
	}
	ctx, stop := context.WithTimeout(t.Context(), 35*time.Second)
	defer stop()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	code := func(err error, want uint32) {
		t.Helper()
		if Code(err) != want {
			t.Fatalf("处理结果=%d 预期=%d 错误=%v", Code(err), want, err)
		}
	}
	cfg, err := devconfig.Load("../../../conf/dev/server.json")
	must(err)
	cfg.MongoURI, cfg.Database = uri, "minimir_dev_game_test_dungeon_"+domain.ID()[:16]
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
	open := func() *Store {
		t.Helper()
		game, err := Open(ctx, cfg)
		must(err)
		t.Cleanup(func() { game.Close(context.Background()) })
		lease, err := account.ApproveGame(ctx, cfg.NodeID, game.Instance, cfg.RunID)
		must(err)
		game.Epoch = lease.Epoch
		return game
	}
	game := open()
	type player struct {
		customer Auth        // 本测试账号持久登录授权。
		key      *pb.RoleKey // 该账号实际新建的永久角色键。
		auth     Auth        // 当前角色真实游戏票所绑定的会话。
		role     domain.Role // 最近从正常角色读取取得的权威修订。
	}
	attach := func(owner *Store, p *player) {
		t.Helper()
		ticket, err := account.IssueTicket(ctx, p.customer, p.key)
		must(err)
		p.role, p.auth, err = owner.Enter(ctx, ticket.Ticket)
		must(err)
	}
	register := func(name string) *player {
		t.Helper()
		login, err := account.Login(ctx, "dungeon_"+domain.ID()[:8], "test_123456", true)
		must(err)
		customer, err := account.Authenticate(ctx, login.AccountToken)
		must(err)
		created, err := account.CreateRole(ctx, customer, &pb.CreateRoleRequest{OriginServerId: cfg.OriginID, Slot: 1, Name: name, OpId: domain.Bytes(domain.ID())})
		must(err)
		p := &player{customer: customer, key: created.Key}
		attach(game, p)
		return p
	}
	a, b, c := register("讨伐队员甲"), register("讨伐队员乙"), register("讨伐队员丙")
	loaded := func(p *player) domain.Role {
		t.Helper()
		r, err := game.Role(ctx, p.role.ID)
		must(err)
		return r
	}
	begin := func(players ...*player) DungeonBegin {
		t.Helper()
		request := DungeonBegin{ChallengeID: domain.ID(), TemplateID: 1, Execution: 1, RuleVersion: cfg.Fixture.RuleVersion,
			ConfigHash: cfg.Fixture.Hash, StartedAt: bossTime(time.Now()), Duration: 5 * time.Minute}
		for _, player := range players {
			request.Members = append(request.Members, DungeonAdmissionMember{
				Auth: player.auth, ExpectedRevision: loaded(player).Revision, CombatSnapshot: strings.Repeat("a", 64)})
		}
		return request
	}
	q := begin(a, b, c)
	_, _, err = game.BeginDungeon(ctx, q)
	code(err, p.CodeLineForbidden)
	if a.role.Dungeon != nil || b.role.Dungeon != nil || c.role.Dungeon != nil {
		t.Fatal("新角色不应拥有预置讨伐扣次")
	}
	// 隔离测试直接安装编译后的偏小模板和明确的权益、额度前态。实际业务只走原方法。
	game.Cfg.Fixture.Dungeons = []devconfig.DungeonConfig{{TemplateID: 1, MinimumLevel: 1, Duration: q.Duration}}
	day := shanghaiOperatingDay(q.StartedAt)
	seed := func(player *player, used uint32, hosting, privilege time.Time) {
		t.Helper()
		_, err := account.DB.Collection("roles").UpdateOne(ctx, bson.M{"_id": player.role.ID}, bson.M{"$set": bson.M{
			"dungeon": domain.DungeonProgress{EntryDay: day, Entries: used}, "hosting_card_until": hosting, "privilege_until": privilege}})
		must(err)
	}
	seed(b, 3, q.StartedAt.Add(time.Hour), time.Time{})
	seed(c, 3, q.StartedAt, q.StartedAt.Add(time.Hour))
	_, _, err = game.BeginDungeon(ctx, q)
	code(err, p.CodeLineForbidden)
	if loaded(a).Dungeon != nil || loaded(b).Dungeon.Entries != 3 || loaded(c).Dungeon.Entries != 3 {
		t.Fatal("成员次数不足时写入了队友扣次")
	}
	count, err := account.DB.Collection(dungeonCollection).CountDocuments(ctx, bson.M{})
	must(err)
	if count != 0 {
		t.Fatal("成员拒绝却保留了场次记录", count)
	}
	seed(c, 2, q.StartedAt, q.StartedAt.Add(time.Hour))
	// 两个不同场次同时申请同一队，只有一个原队能在全部角色的同一修订下成功。
	other := q
	other.ChallengeID = domain.ID()
	type attempt struct {
		request DungeonBegin
		result  DungeonChallenge
		err     error
	}
	attempts := make(chan attempt, 2)
	var workers sync.WaitGroup
	for _, request := range []DungeonBegin{q, other} {
		workers.Go(func() {
			result, _, err := game.BeginDungeon(ctx, request)
			attempts <- attempt{request, result, err}
		})
	}
	workers.Wait()
	close(attempts)
	var original DungeonChallenge
	for trial := range attempts {
		if trial.err != nil {
			code(trial.err, p.CodeOperationConflict)
			continue
		}
		if original.ChallengeID != "" {
			t.Fatal("同原角色版本出现两个活动讨伐场次")
		}
		original, q = trial.result, trial.request
	}
	if original.ChallengeID == "" || len(original.Members) != 3 || loaded(a).Dungeon.Entries != 1 || loaded(b).Dungeon.Entries != 4 || loaded(c).Dungeon.Entries != 3 {
		t.Fatal("实际三人扣次或托管卡+1 失败", original)
	}
	if info, err := game.QueryPersonalBoss(ctx, a.auth); err != nil || info.EligibilityReason != PersonalBossSceneForbidden {
		t.Fatal("原讨伐活动未排斥个人首领", info, err)
	}
	q.Members = slices.Clone(q.Members)
	slices.Reverse(q.Members)
	replay, _, err := game.BeginDungeon(ctx, q)
	must(err)
	if !reflect.DeepEqual(replay, original) {
		t.Fatal("全队原请求重放未返回相同场次", replay)
	}
	changed := q
	changed.Execution++
	_, _, err = game.BeginDungeon(ctx, changed)
	code(err, p.CodeOperationConflict)

	previous := b.auth
	attach(game, b)
	finish := DungeonFinish{ChallengeID: original.ChallengeID, ReservationID: original.ReservationID, Execution: original.Execution,
		Outcome: DungeonFailed, FinishedAt: bossTime(time.Now()), Members: []Auth{a.auth, previous, c.auth}}
	_, _, err = game.FinishDungeon(ctx, finish)
	code(err, p.CodeStaleSession)
	finish.Members[1] = b.auth
	_, _, err = game.FinishDungeon(ctx, finish)
	code(err, p.CodeStaleSession)
	proof := DungeonResume{ChallengeID: original.ChallengeID, ReservationID: original.ReservationID, Execution: original.Execution, PreviousSessionEpoch: previous.SessionEpoch}
	wrongProof := proof
	wrongProof.Execution++
	_, err = game.ResumeDungeon(ctx, b.auth, wrongProof)
	code(err, p.CodeOperationConflict)
	resumed, err := game.ResumeDungeon(ctx, b.auth, proof)
	must(err)
	readback, err := game.ResumeDungeon(ctx, b.auth, proof)
	must(err)
	if !reflect.DeepEqual(resumed, readback) || resumed.ReservationID != original.ReservationID {
		t.Fatal("同场健康接回重放修改了原回执", resumed, readback)
	}
	closed, _, err := game.FinishDungeon(ctx, finish)
	must(err)
	finalReplay, _, err := game.FinishDungeon(ctx, finish)
	must(err)
	if closed.Refunded || closed.Outcome != DungeonFailed || !reflect.DeepEqual(closed, finalReplay) || loaded(a).Dungeon.Entries != 1 || loaded(b).Dungeon.Entries != 4 {
		t.Fatal("正常失败不应退款或被重放改判", closed)
	}
	finish.Outcome = DungeonSystemInterrupted
	_, _, err = game.FinishDungeon(ctx, finish)
	code(err, p.CodeOperationConflict)

	// 实际战斗终态属于运行服原执行，新票在场次结束前后接管不会让原结果无法结算。
	e := register("讨伐队员戊")
	owned, _, err := game.BeginDungeon(ctx, begin(e))
	must(err)
	attach(game, e) // 此时新票已生效，但新连接还没有确认原场席位。
	settle := DungeonFinish{ChallengeID: owned.ChallengeID, ReservationID: owned.ReservationID,
		Execution: owned.Execution, Outcome: DungeonSucceeded, FinishedAt: bossTime(time.Now())}
	withSession := settle
	withSession.Members = []Auth{e.auth}
	_, _, err = game.FinishDungeon(ctx, withSession)
	code(err, p.CodeStaleSession)
	wrongExecution := settle
	wrongExecution.Execution++
	_, _, err = game.FinishOwnedDungeon(ctx, wrongExecution)
	code(err, p.CodeOperationConflict)
	settled, roles, err := game.FinishOwnedDungeon(ctx, settle)
	must(err)
	settledAgain, again, err := game.FinishOwnedDungeon(ctx, settle)
	must(err)
	if settled.Outcome != DungeonSucceeded || settled.Refunded || !reflect.DeepEqual(settled, settledAgain) || len(roles) != 1 ||
		len(again) != 1 || roles[0].Revision != again[0].Revision || roles[0].SessionEpoch != e.auth.SessionEpoch ||
		loaded(e).Dungeon.ActiveChallengeID != "" || loaded(e).Dungeon.Entries != 1 {
		t.Fatal("原执行未在重连后一次确认成功终态", settled, roles)
	}

	// 未知扣次后原身份换代时，未安装战斗的协调者只能撤销原始扣次，并且原队员一起退款。
	f := register("讨伐队员己")
	unattached := begin(e, f)
	empty, emptyRoles, err := game.AbortUnattachedDungeon(ctx, unattached)
	must(err)
	if empty.ChallengeID != "" || len(emptyRoles) != 0 {
		t.Fatal("没有发生的原扣次不应生成退款记录", empty, emptyRoles)
	}
	beforeAbort, _, err := game.BeginDungeon(ctx, unattached)
	must(err)
	_, err = account.DB.Collection("accounts").UpdateOne(ctx, bson.M{"_id": f.customer.AccountID}, bson.M{"$inc": bson.M{"auth_version": 1}})
	must(err)
	_, _, err = game.BeginDungeon(ctx, unattached)
	code(err, p.CodeUnauthenticated)
	wrongBegin := unattached
	wrongBegin.Execution++
	_, _, err = game.AbortUnattachedDungeon(ctx, wrongBegin)
	code(err, p.CodeOperationConflict)
	unattached.Members = slices.Clone(unattached.Members)
	slices.Reverse(unattached.Members)
	aborted, abortRoles, err := game.AbortUnattachedDungeon(ctx, unattached)
	must(err)
	abortedAgain, abortRolesAgain, err := game.AbortUnattachedDungeon(ctx, unattached)
	must(err)
	if !aborted.Refunded || aborted.Recovered || aborted.Outcome != DungeonSystemInterrupted || aborted.ReservationID != beforeAbort.ReservationID ||
		!reflect.DeepEqual(aborted, abortedAgain) || len(abortRoles) != 2 || !reflect.DeepEqual(abortRoles, abortRolesAgain) ||
		loaded(e).Dungeon.Entries != 1 || loaded(f).Dungeon.Entries != 0 || loaded(f).Dungeon.ActiveChallengeID != "" {
		t.Fatal("未安装原场未在账号换代后一次确认并退款", aborted, abortRoles)
	}

	// 另一个未完成原场使用一名仍在旧绑定的队员与一名新票回来的队员。
	d := register("讨伐队员丁")
	q = begin(a, d)
	active, _, err := game.BeginDungeon(ctx, q)
	must(err)
	_, _, err = game.RecoverDungeon(ctx, active.ChallengeID)
	code(err, p.CodeRouteChanged)
	must(game.Release(ctx, "game"))
	cold := open() // 规则目录现已关闭，也应能退款原已确认的扣次。
	attach(cold, a)
	must(cold.RecoverInterruptedDungeons(ctx))
	must(cold.RecoverInterruptedDungeons(ctx))
	rollback, roles, err := cold.RecoverDungeon(ctx, active.ChallengeID)
	must(err)
	rolledAgain, againRoles, err := cold.RecoverDungeon(ctx, active.ChallengeID)
	must(err)
	if !rollback.Refunded || !rollback.Recovered || rollback.Outcome != DungeonSystemInterrupted || len(roles) != 0 || len(againRoles) != 0 ||
		!reflect.DeepEqual(rollback, rolledAgain) || loaded(a).Dungeon.Entries != 1 || loaded(d).Dungeon.Entries != 0 || loaded(d).Dungeon.EntryDay != day {
		t.Fatal("同运行服冷恢复未按原队、原日一次性退款", rollback, roles)
	}
	_, _, err = game.FinishDungeon(ctx, DungeonFinish{ChallengeID: active.ChallengeID, ReservationID: active.ReservationID, Execution: active.Execution,
		Outcome: DungeonSucceeded, FinishedAt: bossTime(time.Now()), Members: []Auth{a.auth, d.auth}})
	code(err, p.CodeRouteChanged)
	count, err = account.DB.Collection(dungeonCollection).CountDocuments(ctx, bson.M{})
	must(err)
	audits, err := account.DB.Collection("audit_outbox").CountDocuments(ctx, bson.M{"kind": bson.M{"$in": []string{"dungeon_begin", "dungeon_finish"}}})
	must(err)
	if count != 4 || audits != 16 || loaded(a).EconomicSeq != a.role.EconomicSeq || loaded(d).SourceSeq != d.role.SourceSeq {
		t.Fatal("场次与审计未保持单次，或改变了普通资产序号", count, audits)
	}
}
