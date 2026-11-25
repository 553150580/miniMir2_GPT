// 本文件验证受控停服使用真实隔离 MongoDB 结清托管整秒和已裁决首领结果。
package game

import (
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	"minimir/internal/persistence"
)

// TestHostingDrainDuringControlledShutdownMongo 使用真实 MongoDB 检查受控停服结清已经计量的整秒，重复执行收束不产生第二笔扣时。
// t 提供断言失败报告、子用例和清理注册。
func TestHostingDrainDuringControlledShutdownMongo(t *testing.T) {
	// cfg 保存由加载取得的服务配置，err 接收加载过程中可能产生的错误，供后续失败分支处理。
	cfg, err := devconfig.Load("../../../conf/dev/server.json")
	if err != nil {
		t.Fatal(err)
	}
	// fixture 保存由创建离线托管MongoDB开发测试配置取得的开发测试配置。
	fixture := newHostingMongoFixture(t, cfg)
	// e 接收创建供后续步骤使用。
	e := New(fixture.ctx, fixture.owner)
	t.Cleanup(e.Stop)
	// w 接收房间执行器供后续步骤使用。
	w := e.worker(fixture.role.ID)
	// session 保存由标识取得的会话。
	session := domain.ID()
	// prepared 创建用于维护“prepared”的容器或通道。
	prepared := make(chan bool, 1)
	// w 记录房间执行器。
	w.commands <- command{done: func(w *worker) {
		// r 保存当前处理所用的房间。
		r := &room{Auth: fixture.roleAuth, worldState: &worldState{}}
		w.initialize(r, fixture.role)
		// 常规计时帧暂不推进测试现场，确认只由受控停服发起。
		r.LastTick = time.Now().Add(time.Minute)
		r.HostSession = session
		for range 11 {
			w.meterHosting(r, 200*time.Millisecond, r.LastTick)
		}
		w.rooms[r.Role.ID] = r
		prepared <- !r.Pending && r.hostSecondsOpen == 2 && r.hostRemainder == 200*time.Millisecond
	}}
	select {
	// ok 表明查询、校验或类型转换是否成功。
	case ok := <-prepared:
		if !ok {
			t.Fatal("停服前应持有两整秒及200毫秒余量，且未提交检查点")
		}
	case <-fixture.ctx.Done():
		t.Fatal(fixture.ctx.Err())
	}
	if err = e.DrainSettlements(fixture.ctx); err != nil {
		t.Fatal(err)
	}
	if err = e.DrainSettlements(fixture.ctx); err != nil {
		t.Fatal(err)
	}
	// role 接收角色供后续步骤使用。
	role, err := fixture.owner.Role(fixture.ctx, fixture.role.ID)
	if err != nil {
		t.Fatal(err)
	}
	if role.HostingSeconds != 3 || role.SourceSeq != fixture.role.SourceSeq+1 || role.Report.SessionID != session || role.Report.UsedSeconds != 2 || role.Report.Kills != 0 {
		t.Fatalf("受控停服未确认恰好两整秒: 余额=%d 来源=%d 战报=%+v", role.HostingSeconds, role.SourceSeq, role.Report)
	}
	// filter 初始化用作筛选条件的MongoDB BSON 文档。
	filter := bson.M{"role_id": role.ID, "source_seq": role.SourceSeq, "host_seconds": int64(2), "hosting_session": session}
	// count 保存由数量文档取得的数量。
	count, err := fixture.owner.DB.Collection("audit_outbox").CountDocuments(fixture.ctx, filter)
	if err != nil || count != 1 {
		t.Fatalf("重复收束应仍只有一笔托管审计: count=%d err=%v", count, err)
	}
}

// TestPersonalBossDrainRetainsResolvedOutcomeMongo 验证停服等待原操作槽空出，再提交已经裁决的首领胜利。
func TestPersonalBossDrainRetainsResolvedOutcomeMongo(t *testing.T) {
	cfg, err := devconfig.Load("../../../conf/dev/server.json")
	if err != nil {
		t.Fatal(err)
	}
	cfg.Fixture.PersonalBoss.KillRewardGold = 11 // 隔离奖励可核对真实到账和重放去重。
	fixture := newHostingMongoFixture(t, cfg)
	template := fixture.owner.Cfg.Fixture.PersonalBoss
	begin := persistence.PersonalBossBegin{ChallengeID: domain.ID(), Execution: 4, TemplateID: template.TemplateID,
		CombatSnapshot: persistence.Hash([]byte("停服前首领入场快照")), ExpectedRevision: fixture.role.Revision,
		RuleVersion: cfg.Fixture.RuleVersion, ConfigHash: cfg.Fixture.Hash, StartedAt: time.Now().Add(-100 * time.Millisecond).UTC().Truncate(time.Millisecond),
		Duration: template.Duration, SuccessCooldown: template.SuccessCooldown, RetryCooldown: template.RetryCooldown, BossHP: template.Monster.HP}
	role, grant, err := fixture.owner.BeginPersonalBoss(fixture.ctx, fixture.roleAuth, begin)
	if err != nil {
		t.Fatal(err)
	}
	e := New(fixture.ctx, fixture.owner)
	t.Cleanup(e.Stop)
	e.stopped.Store(true)
	w := e.worker(role.ID)
	prepared := make(chan bool, 1)
	w.commands <- command{done: func(w *worker) {
		r := &room{Auth: fixture.roleAuth, worldState: &worldState{}}
		w.initialize(r, role)
		run := &personalBossRun{begin: begin, auth: r.Auth, started: true}
		r.personalBoss, r.Pending = run, true
		w.installPersonalBoss(r, run, template, w.freezePersonalBossSnapshot(r, template), grant, time.Now())
		w.rooms[r.Role.ID] = r
		if run.battle == nil {
			prepared <- false
			return
		}
		battle := run.battle.bossBattle
		w.applyPersonalBossDamage(run.battle, &run.battle.Player, battle.boss, battle.boss.HP, time.Now(), nil)
		prepared <- battle.challenge.finish != nil
	}}
	select {
	case ready := <-prepared:
		if !ready {
			t.Fatal("停服前的测试战斗未由原首领裁决为胜利")
		}
	case <-fixture.ctx.Done():
		t.Fatal(fixture.ctx.Err())
	}
	drained := make(chan error, 1)
	go func() { drained <- e.DrainSettlements(fixture.ctx) }()
	select {
	case err := <-drained:
		t.Fatalf("原操作槽仍被占用，停服提前结束：%v", err)
	case <-time.After(100 * time.Millisecond):
	}
	w.commands <- command{done: func(w *worker) { w.rooms[role.ID].Pending = false }}
	if err := <-drained; err != nil {
		t.Fatal(err)
	}
	if err := e.DrainSettlements(fixture.ctx); err != nil {
		t.Fatal(err)
	}
	confirmed, err := fixture.owner.Role(fixture.ctx, role.ID)
	if err != nil {
		t.Fatal(err)
	}
	info, err := fixture.owner.QueryPersonalBoss(fixture.ctx, fixture.roleAuth)
	if err != nil || len(info.History) != 1 || info.History[0].ChallengeID != grant.ChallengeID ||
		info.History[0].Outcome != persistence.PersonalBossDefeated || info.History[0].RewardGold != 11 ||
		confirmed.Wallet.BoundGold != role.Wallet.BoundGold+11 || confirmed.PersonalBoss.Successes != 1 || confirmed.PersonalBoss.ActiveChallengeID != "" {
		t.Fatalf("已裁决的胜利与首领奖励应恰好确认一次：战报=%+v 钱包=%+v 错误=%v", info.History, confirmed.Wallet, err)
	}
}
