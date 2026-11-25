// 本文件核对公共首领的证据边界和真实 MongoDB 中的唯一结果、续租与接管。
package persistence

import (
	"context"
	"os"
	"reflect"
	"regexp"
	"slices"
	"sync"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	p "minimir/internal/protocol"
)

func TestWorldBossResultEvidenceBounds(t *testing.T) {
	start := time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC)
	spec := WorldBossSpec{OpensAt: start, ClosesAt: start.Add(time.Minute), HPMax: 50}
	first, second := testWorldBossContribution(1, 25, start), testWorldBossContribution(2, 25, start.Add(time.Second))
	kill := WorldBossFinish{Outcome: WorldBossKilled, FinishedAt: start.Add(20 * time.Second), Contributions: []WorldBossContribution{second, first}}
	frozen, initialHash, err := normalizeWorldBossFinish(spec, kill)
	if err != nil || frozen.Contributions[0].RoleID != first.RoleID || kill.Contributions[0] != second {
		t.Fatal("同伤害排名不稳定或原输入被修改", frozen, err)
	}
	replay := kill
	replay.Contributions = []WorldBossContribution{first, second}
	replay.FinishedAt = replay.FinishedAt.Add(500 * time.Microsecond).In(time.FixedZone("Asia/Shanghai", 8*60*60))
	if _, digest, err := normalizeWorldBossFinish(spec, replay); err != nil || digest != initialHash {
		t.Fatal("同一毫秒和相同贡献换序重放改变了证据", digest, err)
	}
	cases := map[string]WorldBossFinish{
		"到期后击杀":     {Outcome: WorldBossKilled, FinishedAt: spec.ClosesAt, Contributions: kill.Contributions},
		"缺少够量的伤害":   {Outcome: WorldBossKilled, FinishedAt: kill.FinishedAt, Contributions: []WorldBossContribution{first}},
		"重复角色":      {Outcome: WorldBossKilled, FinishedAt: kill.FinishedAt, Contributions: []WorldBossContribution{first, first}},
		"选择性推迟到期时刻": {Outcome: WorldBossExpired, FinishedAt: spec.ClosesAt.Add(time.Millisecond), HPRemaining: 25, Contributions: []WorldBossContribution{first}},
		"同一预约给不同角色": {Outcome: WorldBossKilled, FinishedAt: kill.FinishedAt, Contributions: []WorldBossContribution{first, {RoleID: second.RoleID,
			DisplayName: second.DisplayName, ReservationID: first.ReservationID, CombatSnapshot: second.CombatSnapshot, JoinedAt: second.JoinedAt, Damage: second.Damage}}},
		"无终态的场次": {Outcome: WorldBossActive, FinishedAt: kill.FinishedAt, HPRemaining: 50},
	}
	for name, value := range cases {
		t.Run(name, func(t *testing.T) {
			if _, _, err := normalizeWorldBossFinish(spec, value); Code(err) != p.CodeInvalidArgument {
				t.Fatalf("无效终态证据未拒绝：%v", err)
			}
		})
	}
}

// TestWorldBossDurableResultMongo 用真实注册及运行服换代验证世界结果事务无角色资产变更。
func TestWorldBossDurableResultMongo(t *testing.T) {
	uri := os.Getenv("MINIMIR_INTEGRATION_TEST_MONGO_URI")
	if os.Getenv("MINIMIR_MONGO_E2E") != "1" || uri == "" {
		t.Skip("仅在当前任务专属 MongoDB 副本集运行")
	}
	owned := regexp.MustCompile(`^mongodb://127\.0\.0\.1:[1-9][0-9]{3,4}/\?replicaSet=minimirLoad_[a-f0-9]{32}&directConnection=true$`)
	if !owned.MatchString(uri) {
		t.Fatal("世界首领验收库不在独立副本集控制器的授权范围")
	}
	require := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	requireCode := func(err error, want uint32) {
		t.Helper()
		if Code(err) != want {
			t.Fatalf("预期业务码 %d，得到 %d：%v", want, Code(err), err)
		}
	}
	ctx, cancel := context.WithTimeout(t.Context(), 38*time.Second)
	defer cancel()
	cfg, err := devconfig.Load("../../../conf/dev/server.json")
	require(err)
	cfg.MongoURI, cfg.Database = uri, "minimir_dev_game_test_world_boss_"+domain.ID()[:16]
	cfg.NodeID, cfg.RunID, cfg.RunServerIDs = "world-result", 11, []uint64{11, 12, 13}
	cfg.OriginRoutes = []devconfig.OriginRouteSeed{{OriginID: 1, RunID: 11, Name: "隔离一服", RegionID: "world-result", RegionName: "首领结果验收区"}}
	cfg.RunServers = []devconfig.RunServerSeed{
		{RunID: 11, GameNodeID: cfg.NodeID, GameWSURL: "/game/ws"},
		{RunID: 12, GameNodeID: cfg.NodeID, GameWSURL: "/game/ws"},
		{RunID: 13, GameNodeID: cfg.NodeID, GameWSURL: "/game/ws"},
	}
	start := worldBossTime(time.Now().Add(-time.Second))
	local := start.In(time.FixedZone("Asia/Shanghai", 8*60*60))
	midnight := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, local.Location())
	cfg.Fixture.WorldBossVariants = nil
	cfg.Fixture.WorldBoss.DailyOpenOffsets = []time.Duration{local.Sub(midnight)}
	cfg.Fixture.WorldBoss.Window, cfg.Fixture.WorldBoss.ReturnDelay = 8*time.Second, time.Second
	if !cfg.Fixture.ValidWorldBoss() {
		t.Fatal("隔离首领的原日程与地图无效")
	}
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
	openGame := func() (*Store, []*Store) {
		t.Helper()
		game, err := Open(ctx, cfg)
		require(err)
		t.Cleanup(func() { game.Close(context.Background()) })
		grants, err := account.ApproveGameRuns(ctx, cfg.NodeID, game.Instance, cfg.RunServerIDs)
		require(err)
		views, err := game.GameRunStores(grants)
		require(err)
		return game, views
	}
	old, views := openGame()
	first, second, third := views[0], views[1], views[2]
	template := cfg.Fixture.WorldBoss
	spec := func(run, execution uint64) WorldBossSpec {
		return WorldBossSpec{RunID: run, OccurrenceID: WorldBossOccurrenceID(run, template.TemplateID, start), TemplateID: template.TemplateID,
			Execution: execution, RuleVersion: cfg.Fixture.RuleVersion, ConfigHash: cfg.Fixture.Hash,
			OpensAt: start, ClosesAt: start.Add(template.Window), HPMax: template.Monster.HP}
	}
	killed, orphan, timed := spec(11, 1), spec(12, 1), spec(13, 1)
	_, err = second.BeginWorldBoss(ctx, killed)
	requireCode(err, p.CodeInvalidArgument)
	if absent, err := first.RecoverWorldBoss(ctx); absent != nil || err != nil {
		t.Fatal("新运行服不应有世界首领历史", absent, err)
	}
	_, err = first.BeginWorldBoss(ctx, killed)
	require(err)
	active, err := first.BeginWorldBoss(ctx, killed)
	require(err)
	if active.Outcome != WorldBossActive || active.ProgressKnown || active.OwnerInstance != old.Instance || active.RunEpoch != first.Epoch {
		t.Fatal("同场重放未返回原预约", active)
	}
	unexpected := killed
	unexpected.Execution++
	_, err = first.BeginWorldBoss(ctx, unexpected)
	requireCode(err, p.CodeOperationConflict)
	_, err = second.BeginWorldBoss(ctx, orphan)
	require(err)
	_, err = third.BeginWorldBoss(ctx, timed)
	require(err)
	if result, err := second.RecoverWorldBoss(ctx); result != nil || err != nil {
		t.Fatal("同租约已开启的原场被恢复操作中断", result, err)
	}

	// 两份各自合理但互不相同的证据同时提交。首次写入必须唯一且完全不影响别服。
	firstHit, secondHit := testWorldBossContribution(1, killed.HPMax/2, start), testWorldBossContribution(2, killed.HPMax-killed.HPMax/2, start)
	dead := WorldBossFinish{Outcome: WorldBossKilled, FinishedAt: worldBossTime(time.Now()),
		Contributions: []WorldBossContribution{secondHit, firstHit, testWorldBossContribution(3, 0, start)}}
	otherEvidence := dead
	otherEvidence.Contributions = slices.Clone(dead.Contributions)
	otherEvidence.Contributions[0].DisplayName = "发生冲突的同角色显示名"
	type contender struct {
		request WorldBossFinish
		result  *WorldBossResult
		err     error
	}
	var workers sync.WaitGroup
	results := make(chan contender, 2)
	for _, candidate := range []WorldBossFinish{dead, otherEvidence} {
		workers.Go(func() {
			result, err := first.FinishWorldBoss(ctx, killed, candidate)
			results <- contender{candidate, result, err}
		})
	}
	workers.Wait()
	close(results)
	var winner contender
	var committed, conflicted int
	for result := range results {
		switch Code(result.err) {
		case 0:
			winner = result
			committed++
		case p.CodeOperationConflict:
			conflicted++
		default:
			t.Fatal("竞争产生非预期错误", result.err)
		}
	}
	if committed != 1 || conflicted != 1 || winner.result.Outcome != WorldBossKilled || !winner.result.ProgressKnown ||
		winner.result.Contributions[2].Damage != 0 ||
		firstHit.Damage == secondHit.Damage && winner.result.Contributions[0].RoleID != firstHit.RoleID {
		t.Fatal("结果竞争没有形成唯一稳定排名", committed, conflicted, winner.result)
	}
	slices.Reverse(winner.request.Contributions)
	replayed, err := first.FinishWorldBoss(ctx, killed, winner.request)
	require(err)
	if !reflect.DeepEqual(replayed, winner.result) {
		t.Fatal("原终态重放改变了已保存事实", winner.result, replayed)
	}
	afterKill, err := first.BeginWorldBoss(ctx, unexpected)
	require(err)
	if !reflect.DeepEqual(afterKill, winner.result) {
		t.Fatal("已结算窗口重新接受预约", afterKill)
	}
	early := WorldBossFinish{Outcome: WorldBossExpired, FinishedAt: timed.ClosesAt, HPRemaining: timed.HPMax}
	_, err = third.FinishWorldBoss(ctx, timed, early)
	requireCode(err, p.CodeInvalidArgument)
	if wait := time.Until(timed.ClosesAt.Add(25 * time.Millisecond)); wait > 0 {
		time.Sleep(wait)
	}
	require(old.Renew(ctx, "game"))
	expiry, err := third.FinishWorldBoss(ctx, timed, early)
	require(err)
	if !expiry.ProgressKnown || !expiry.FinishedAt.Equal(timed.ClosesAt) || expiry.HPRemaining != timed.HPMax || len(expiry.Contributions) != 0 {
		t.Fatal("超时未保存真实原截止、满血和空参与", expiry)
	}

	// 原游戏实例完成停止屏障后，新实例只能确认原编号中断，不能复刻原战斗或编造排行。
	require(old.Release(ctx, "game"))
	_, err = first.FinishWorldBoss(ctx, killed, winner.request)
	requireCode(err, p.CodeRouteChanged)
	_, err = second.RecoverWorldBoss(ctx)
	requireCode(err, p.CodeRouteChanged)
	_, newViews := openGame()
	resettled, err := newViews[0].RecoverWorldBoss(ctx)
	require(err)
	if !reflect.DeepEqual(resettled, winner.result) {
		t.Fatal("新所有者改写了已确认击杀", resettled)
	}
	interrupted, err := newViews[1].RecoverWorldBoss(ctx)
	require(err)
	if interrupted.Outcome != WorldBossInterrupted || interrupted.ProgressKnown || interrupted.HPRemaining != 0 ||
		len(interrupted.Contributions) != 0 || interrupted.WorldBossSpec != orphan || interrupted.OwnerInstance != old.Instance ||
		interrupted.RunEpoch != second.Epoch || interrupted.FinishedAt.Before(orphan.ClosesAt) {
		t.Fatal("新所有者没有诚实保留原场身份和未知战斗进度", interrupted)
	}
	recovered, err := newViews[1].RecoverWorldBoss(ctx)
	require(err)
	if !reflect.DeepEqual(recovered, interrupted) {
		t.Fatal("重复恢复改写中断回执", recovered)
	}
	orphan.Execution++
	afterRecovery, err := newViews[1].BeginWorldBoss(ctx, orphan)
	require(err)
	if !reflect.DeepEqual(afterRecovery, interrupted) {
		t.Fatal("中断的原窗口被新所有者重复开放", afterRecovery)
	}
	for _, name := range []string{"roles", "operations"} {
		count, err := account.DB.Collection(name).CountDocuments(ctx, bson.M{})
		require(err)
		if count != 0 {
			t.Fatalf("世界终态不应创造角色或永久资产操作：%s=%d", name, count)
		}
	}
	count, err := account.DB.Collection(worldBossResultCollection).CountDocuments(ctx, bson.M{})
	require(err)
	if count != 3 {
		t.Fatalf("三个独立运行服没有分别保留唯一记录：%d", count)
	}
	t.Log("同证据幂等、冲突唯一胜者、三服隔离、原窗逃走及旧租约失权均经实际多数派事务确认")
}

func testWorldBossContribution(id byte, damage int64, joined time.Time) WorldBossContribution {
	role := make([]byte, 16)
	role[15] = id
	reservation := slices.Clone(role)
	reservation[0] = 1
	return WorldBossContribution{RoleID: domain.Hex(role), DisplayName: "原参战者", ReservationID: domain.Hex(reservation),
		CombatSnapshot: Hash(role), JoinedAt: joined, Damage: damage}
}
