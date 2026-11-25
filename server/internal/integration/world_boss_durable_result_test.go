// 本文件用真实 MongoDB、游戏服换代及 WebSocket 验证世界首领永久结果。
package integration

import (
	"context"
	"errors"
	"net"
	"os"
	"os/exec"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"google.golang.org/protobuf/proto"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/bootstrap"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
	"minimir/internal/wire"
)

const (
	worldBossDurablePrefix = "minimir_dev_game_test_wbd_" // 硬退出子进程只允许新随机测试库。
	worldBossDurableSpan   = 40 * time.Second             // 同窗容纳真实租约过期及新节点认证。
)

// TestSharedWorldBossDurableResultMongoWS 复用同一账号服和数据，验证已结算与遗留活动场的独立恢复。
func TestSharedWorldBossDurableResultMongoWS(t *testing.T) {
	if opening := os.Getenv("MINIMIR_WORLD_BOSS_DURABLE_CHILD_OPENS"); opening != "" {
		runDurableWorldBossChild(t, opening)
		return
	}
	if os.Getenv("MINIMIR_MONGO_E2E") != "1" || os.Getenv("MINIMIR_INTEGRATION_TEST_MONGO_URI") == "" {
		t.Skip("世界首领永久结果需要任务专属 MongoDB 副本集")
	}
	start := time.Now().UTC().Truncate(time.Millisecond).Add(-2 * time.Second)
	closes, next := start.Add(worldBossDurableSpan), start.Add(worldBossDurableSpan+2*time.Second)
	cfg, firstRun, secondRun := durableWorldBossConfig(t, worldBossDurablePrefix+domain.ID(), address(t), address(t), start)
	ctx := t.Context()
	inspection, err := persistence.Open(ctx, cfg)
	must(t, err)
	t.Cleanup(func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		defer inspection.Close(cleanup)
		if err := inspection.DB.Drop(cleanup); err != nil {
			t.Error(err)
		}
	})
	startDurableWorldBossNode(t, cfg, "account", cfg.AccountAddress)
	crashOriginal := startDurableWorldBossCrashNode(t, cfg, start)
	verifyHostedRunIDs(t, cfg)
	endpoint := "http://" + cfg.AccountAddress + "/rpc"
	first, firstRole, firstToken := joinWorldBossTestRole(t, endpoint, firstRun)
	defer first.close()
	second, secondRole, _ := joinWorldBossTestRole(t, endpoint, firstRun)
	defer second.close()
	orphan, orphanRole, orphanToken := joinWorldBossTestRole(t, endpoint, secondRun)
	defer orphan.close()
	deadline := time.Now().Add(7 * time.Second)
	oldFirst := awaitDurableWorldBossState(t, first, deadline, func(state *pb.WorldBossState) bool { return state.State == 3 })
	oldOrphan := awaitDurableWorldBossState(t, orphan, deadline, func(state *pb.WorldBossState) bool { return state.State == 3 })
	template := cfg.Fixture.WorldBoss.TemplateID
	if oldFirst.OccurrenceId != persistence.WorldBossOccurrenceID(1, template, start) || oldOrphan.OccurrenceId != persistence.WorldBossOccurrenceID(2, template, start) ||
		oldFirst.LatestResult != nil || oldOrphan.LatestResult != nil {
		t.Fatal("新库两服应各有正确的原场编号且没有历史", oldFirst, oldOrphan)
	}
	enterWorldBossTest(t, first, oldFirst)
	enterWorldBossTest(t, second, oldFirst)
	enterWorldBossTest(t, orphan, oldOrphan)
	t.Log("两运行服三个真人已确认原共享场基线")
	goal := &pb.GridPoint{X: 6, Y: 4}
	second.send(p.MsgC2SMoveIntent, &pb.MoveIntent{SceneEpoch: second.entry.SceneEpoch, Destination: goal})
	travelUntilWithin(second, 5*time.Second, func(frame wire.Frame) bool {
		return frame.ID == p.MsgS2CSceneBaseline && proto.Equal(second.player().Position, goal)
	})
	attackWorldBossTest(first)
	firstHit := waitWorldBossTest(t, second, 0, func(state *pb.WorldBossState) bool { return state.State == 3 && state.Hp == 2 })
	requireDurableWorldBossRanks(t, firstHit.Ranks, firstRole.Name, "")
	attackWorldBossTest(orphan)
	orphanHit := waitWorldBossTest(t, orphan, 0, func(state *pb.WorldBossState) bool { return state.State == 3 && state.Hp == 2 })
	requireDurableWorldBossRanks(t, orphanHit.Ranks, orphanRole.Name, "")
	t.Log("一服和二服已分别通过真实命中把原首领打到两点生命")
	// 等角色移动动作时钟，第二个真人的命中由共享首领按剩余生命封顶。
	time.Sleep(time.Duration(second.player().MoveIntervalMs)*time.Millisecond + 100*time.Millisecond)
	attackWorldBossTest(second)
	sealed := waitWorldBossTest(t, second, 0, func(state *pb.WorldBossState) bool { return state.State == 4 })
	if sealed.LatestResult == nil || sealed.Hp != 0 || sealed.OccurrenceId != oldFirst.OccurrenceId ||
		!slices.EqualFunc(sealed.Ranks, sealed.LatestResult.Ranks, func(left, right *pb.WorldBossRank) bool { return proto.Equal(left, right) }) {
		t.Fatal("击杀终态排行与回传的永久结果不一致", sealed)
	}
	requireDurableWorldBossRanks(t, sealed.LatestResult.Ranks, firstRole.Name, secondRole.Name)
	returnFromWorldBossTest(first, oldFirst.OccurrenceId)
	returnFromWorldBossTest(second, oldFirst.OccurrenceId)
	priorKill := readDurableWorldBossRow(t, inspection, 1, oldFirst.OccurrenceId)
	priorActive := readDurableWorldBossRow(t, inspection, 2, oldOrphan.OccurrenceId)
	requireDurableWorldBossKill(t, priorKill, firstRole, secondRole, sealed.LatestResult, start, closes, cfg.Fixture)
	if priorActive.Outcome != persistence.WorldBossActive || priorActive.ProgressKnown || priorActive.OwnerInstance != priorKill.OwnerInstance ||
		priorActive.RunEpoch <= 0 || priorActive.RunID != 2 || !priorActive.FinishedAt.IsZero() || len(priorActive.Contributions) != 0 {
		t.Fatal("重启前第二服必须是具有真实但未持久战斗进度的活动场", priorActive)
	}
	t.Log("一服的击杀和双人排名已经持久保存，二服仍保留活动记录")
	oldFirstBinding, oldOrphanBinding := first.binding, orphan.binding
	// 只结束当前测试亲自启动的游戏子进程，跳过优雅收束以留下真实活动场记录。
	crashOriginal()
	first.close()
	second.close()
	orphan.close()
	if orphanAfterStop := readDurableWorldBossRow(t, inspection, 2, oldOrphan.OccurrenceId); orphanAfterStop.Outcome != persistence.WorldBossActive {
		t.Fatal("旧执行没有留下活动记录，无法验证新运行服接管", orphanAfterStop)
	}
	waitDurableWorldBossLeaseExpiry(t, inspection, priorActive.OwnerInstance)
	startDurableWorldBossNode(t, cfg, "game", cfg.GameAddress)
	verifyHostedRunIDs(t, cfg)
	firstAgain := joinDurableWorldBossRole(t, endpoint, firstToken, firstRole.Key, firstRun)
	defer firstAgain.close()
	orphanAgain := joinDurableWorldBossRole(t, endpoint, orphanToken, orphanRole.Key, secondRun)
	defer orphanAgain.close()
	if firstAgain.binding <= oldFirstBinding || orphanAgain.binding <= oldOrphanBinding || !time.Now().Before(closes) {
		t.Fatal("重启验收没有换发角色附着，或未能在同一个真实开放窗口内完成")
	}
	deadline = time.Now().Add(6 * time.Second)
	firstRecovered := awaitDurableWorldBossState(t, firstAgain, deadline, func(state *pb.WorldBossState) bool { return state.State == 4 && state.LatestResult != nil })
	orphanRecovered := awaitDurableWorldBossState(t, orphanAgain, deadline, func(state *pb.WorldBossState) bool { return state.State == 8 && state.LatestResult != nil })
	if firstRecovered.OccurrenceId != oldFirst.OccurrenceId || !proto.Equal(firstRecovered.LatestResult, sealed.LatestResult) ||
		orphanRecovered.OccurrenceId != oldOrphan.OccurrenceId || orphanRecovered.LatestResult.OccurrenceId != oldOrphan.OccurrenceId ||
		orphanRecovered.LatestResult.State != 8 || orphanRecovered.LatestResult.TemplateId != template || len(orphanRecovered.Ranks) != 0 || len(orphanRecovered.LatestResult.Ranks) != 0 {
		t.Fatal("重启后击杀被改写、运行服历史串写或旧活动场出现无证明排行", firstRecovered, orphanRecovered)
	}
	for _, check := range []struct {
		conn *client
		old  *pb.WorldBossState
	}{{firstAgain, oldFirst}, {orphanAgain, oldOrphan}} {
		request := check.conn.send(p.MsgC2SWorldBossEnter, &pb.WorldBossEnter{SceneEpoch: check.conn.entry.SceneEpoch, TemplateId: check.old.TemplateId, OccurrenceId: check.old.OccurrenceId})
		travelExpectError(check.conn, p.MsgC2SWorldBossEnter, request, p.CodeLineForbidden)
	}
	// 两服的当前节点租约确已换代，且只能确认原执行留下的中断，旧击杀结果与时间完全保留。
	for _, previous := range []persistence.WorldBossResult{priorKill, priorActive} {
		current := &struct {
			Epoch int64  `bson:"run_epoch"`
			Owner string `bson:"owner_instance"`
		}{}
		must(t, inspection.DB.Collection("run_control").FindOne(ctx, bson.M{"_id": previous.RunID}).Decode(current))
		if current.Epoch <= previous.RunEpoch || current.Owner == previous.OwnerInstance {
			t.Fatal("游戏节点重启未实际更换运行服租约", previous.RunID, previous.RunEpoch, current)
		}
	}
	storedKilled := readDurableWorldBossRow(t, inspection, 1, oldFirst.OccurrenceId)
	storedInterrupted := readDurableWorldBossRow(t, inspection, 2, oldOrphan.OccurrenceId)
	requireDurableWorldBossKill(t, storedKilled, firstRole, secondRole, firstRecovered.LatestResult, start, closes, cfg.Fixture)
	if priorKill.WorldBossSpec != storedKilled.WorldBossSpec || priorKill.FinishedAt != storedKilled.FinishedAt || priorKill.RunEpoch != storedKilled.RunEpoch ||
		storedInterrupted.WorldBossSpec != priorActive.WorldBossSpec || storedInterrupted.OwnerInstance != priorActive.OwnerInstance || storedInterrupted.RunEpoch != priorActive.RunEpoch ||
		storedInterrupted.Outcome != persistence.WorldBossInterrupted || storedInterrupted.ProgressKnown || storedInterrupted.HPRemaining != 0 || len(storedInterrupted.Contributions) != 0 ||
		storedInterrupted.FinishedAt.UnixMilli() != orphanRecovered.LatestResult.FinishedAtUnixMs || storedInterrupted.FinishedAt.Before(priorActive.OpensAt) {
		t.Fatal("真实库没有保存旧身份、原击杀时钟与无进度中断", storedKilled, storedInterrupted)
	}
	t.Log("旧子进程强制退出、两服原租约自然过期，新游戏节点同窗读回各自结果")

	for _, check := range []struct {
		conn   *client
		run    uint64
		latest *pb.WorldBossResult
	}{{firstAgain, 1, firstRecovered.LatestResult}, {orphanAgain, 2, orphanRecovered.LatestResult}} {
		expected := persistence.WorldBossOccurrenceID(check.run, template, next)
		upcoming := awaitDurableWorldBossState(t, check.conn, next.Add(8*time.Second), func(state *pb.WorldBossState) bool {
			return state.State == 3 && state.OccurrenceId == expected
		})
		if upcoming.Hp != 5 || len(upcoming.Ranks) != 0 || !proto.Equal(upcoming.LatestResult, check.latest) {
			t.Fatal("独立新窗口借用了旧生命、旧排行或丢失原永久结果", upcoming)
		}
		enterWorldBossTest(t, check.conn, upcoming)
		inside := queryWorldBossTest(t, check.conn)
		if inside.SceneEpoch != check.conn.entry.SceneEpoch || inside.OccurrenceId != expected || !proto.Equal(inside.LatestResult, check.latest) {
			t.Fatal("新场实际入场后读取到错误的历史结果", inside)
		}
		newRecord := readDurableWorldBossRow(t, inspection, check.run, expected)
		if newRecord.Outcome != persistence.WorldBossActive || newRecord.OwnerInstance == priorActive.OwnerInstance {
			t.Fatal("独立新窗口没有由新执行预约", newRecord)
		}
	}
	count, err := inspection.DB.Collection("world_boss_results").CountDocuments(ctx, bson.M{})
	must(t, err)
	if count != 4 {
		t.Fatal("两服两窗口未各自保留唯一记录", count)
	}
	assetEvents, err := inspection.DB.Collection("audit_outbox").CountDocuments(ctx, bson.M{"kind": "settlement", "$or": bson.A{
		bson.M{"settlement.map_id": cfg.Fixture.WorldBoss.Map.ID}, bson.M{"settlement.monster_id": cfg.Fixture.WorldBoss.Monster.ID},
	}})
	must(t, err)
	if assetEvents != 0 {
		t.Fatal("世界首领永久结果误触发普通击杀收益", assetEvents)
	}
	t.Logf("两服两窗口保留四条唯一事实，一服原榜 %d+%d，二服原终态为中断，两个新场均可正常入场", sealed.Ranks[0].Damage, sealed.Ranks[1].Damage)
}

// runDurableWorldBossChild 是父测试专用游戏子进程入口，联机矩阵只登记父用例。
func runDurableWorldBossChild(t *testing.T, opening string) {
	t.Helper()
	if os.Getenv("MINIMIR_MONGO_E2E") != "1" || !isolatedMongoURI.MatchString(os.Getenv("MINIMIR_INTEGRATION_TEST_MONGO_URI")) {
		t.Fatal("拒绝在任务专属副本集外启动故障注入子进程")
	}
	database := os.Getenv("MINIMIR_WORLD_BOSS_DURABLE_CHILD_DB")
	suffix := strings.TrimPrefix(database, worldBossDurablePrefix)
	if database == suffix || len(suffix) != 32 || strings.Trim(suffix, "0123456789abcdef") != "" {
		t.Fatal("故障注入子进程需要父测试分配的随机数据库")
	}
	account, game := os.Getenv("MINIMIR_WORLD_BOSS_DURABLE_CHILD_ACCOUNT"), os.Getenv("MINIMIR_WORLD_BOSS_DURABLE_CHILD_GAME")
	for _, endpoint := range []string{account, game} {
		host, port, err := net.SplitHostPort(endpoint)
		p, invalidPort := strconv.ParseUint(port, 10, 16)
		if err != nil || invalidPort != nil || host != "127.0.0.1" || p < 1024 || p == 18080 {
			t.Fatal("故障注入子进程需要父测试分配的回环端口", endpoint)
		}
	}
	openingMS, err := strconv.ParseInt(opening, 10, 64)
	must(t, err)
	cfg, _, _ := durableWorldBossConfig(t, database, account, game, time.UnixMilli(openingMS))
	must(t, bootstrap.Run(t.Context(), cfg, "game"))
}

// startDurableWorldBossCrashNode 用当前 Go 测试二进制运行真实初始节点，并保留本次子进程句柄供故障注入。
func startDurableWorldBossCrashNode(t *testing.T, cfg devconfig.Config, opens time.Time) func() {
	t.Helper()
	executable, err := os.Executable()
	must(t, err)
	node := exec.Command(executable, "-test.run=^TestSharedWorldBossDurableResultMongoWS$")
	node.Env = append(os.Environ(), "MINIMIR_WORLD_BOSS_DURABLE_CHILD_OPENS="+strconv.FormatInt(opens.UnixMilli(), 10),
		"MINIMIR_WORLD_BOSS_DURABLE_CHILD_DB="+cfg.Database, "MINIMIR_WORLD_BOSS_DURABLE_CHILD_ACCOUNT="+cfg.AccountAddress, "MINIMIR_WORLD_BOSS_DURABLE_CHILD_GAME="+cfg.GameAddress)
	node.Stdout, node.Stderr = os.Stdout, os.Stderr
	must(t, node.Start())
	result := make(chan error, 1)
	go func() { result <- node.Wait() }()
	crash := sync.OnceFunc(func() {
		if err := node.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
			t.Errorf("无法结束本测试启动的游戏子进程：%v", err)
			return
		}
		if err := <-result; err == nil {
			t.Error("原游戏子进程未通过硬退出结束")
		}
	})
	t.Cleanup(crash)
	healthy(t, cfg.GameAddress)
	return crash
}

// waitDurableWorldBossLeaseExpiry 等待本次经过真实时钟的租约过期，不人工改写所有权。
func waitDurableWorldBossLeaseExpiry(t *testing.T, store *persistence.Store, oldOwner string) {
	t.Helper()
	deadline := time.Now().Add(persistence.NodeLeaseDuration + 4*time.Second)
	live := bson.M{"owner_instance": oldOwner, "$expr": bson.M{"$gt": bson.A{"$lease_until", "$$NOW"}}}
	for {
		nodes, err := store.DB.Collection("node_leases").CountDocuments(t.Context(), live)
		must(t, err)
		runs, err := store.DB.Collection("run_control").CountDocuments(t.Context(), live)
		must(t, err)
		if nodes == 0 && runs == 0 {
			return
		}
		if !time.Now().Before(deadline) {
			t.Fatal("原游戏进程退出后租约未自然过期", nodes, runs)
		}
		time.Sleep(120 * time.Millisecond)
	}
}

func durableWorldBossConfig(t *testing.T, database, account, game string, opens time.Time) (devconfig.Config, directoryTestEntry, directoryTestEntry) {
	t.Helper()
	cfg, err := loadIntegrationConfig()
	must(t, err)
	cfg.Database, cfg.AccountAddress, cfg.GameAddress = database, account, game
	cfg.OriginID, cfg.RunID, cfg.RunServerIDs = 1, 1, []uint64{1, 2}
	firstRun := makeDirectoryEntry(1, 1, cfg, "durable-boss", "首领结果验收区", "一服", 0, 0)
	secondRun := makeDirectoryEntry(2, 2, cfg, "durable-boss", "首领结果验收区", "二服", 0, 1)
	cfg.OriginRoutes = []devconfig.OriginRouteSeed{firstRun.origin, secondRun.origin}
	cfg.RunServers = []devconfig.RunServerSeed{firstRun.assignment, secondRun.assignment}
	setDurableWorldBossFixture(t, &cfg.Fixture, opens, opens.Add(worldBossDurableSpan+2*time.Second), worldBossDurableSpan)
	return cfg, firstRun, secondRun
}

// setDurableWorldBossFixture 仅修改隔离库战斗条件和上海窗口，方便尽快确认两次原始开场。
func setDurableWorldBossFixture(t *testing.T, fixture *devconfig.Fixture, start, next time.Time, span time.Duration) {
	t.Helper()
	for i := range fixture.Items {
		if fixture.Items[i].ID == 218 {
			fixture.Items[i].AttackMinBonus, fixture.Items[i].AttackMaxBonus = 3, 3
		}
	}
	monster := fixture.Monsters[0]
	monster.ID, monster.HP, monster.AttackMin, monster.AttackMax, monster.Agility = 80002, 5, 0, 0, 0
	monster.MoveMS, monster.AttackMS, monster.DefenceMin, monster.DefenceMax = 4000, 4000, 0, 0
	monster.Exp, monster.GoldMin, monster.GoldMax, monster.RespawnMS, monster.DropPoolID = 0, 0, 0, 0, 0
	shanghai := time.FixedZone("Asia/Shanghai", 8*60*60)
	offsets := make([]time.Duration, 0, 2)
	for _, opening := range []time.Time{start, next} {
		local := opening.In(shanghai)
		midnight := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, shanghai)
		offsets = append(offsets, local.Sub(midnight))
	}
	slices.Sort(offsets)
	fixture.WorldBossVariants = nil
	fixture.WorldBoss = &devconfig.WorldBossConfig{TemplateID: 2, MinimumLevel: 1, Monster: monster, DailyOpenOffsets: offsets,
		Map: devconfig.MapTemplate{ID: 81, Width: 13, Height: 13, SceneKind: devconfig.SceneKindWorldBoss,
			PlayerSpawn: devconfig.Point{X: 5, Y: 5}, MonsterSpawns: []devconfig.Point{{X: 6, Y: 5}}},
		Window: span, ReturnDelay: 300 * time.Millisecond, ParticipantLimit: 8}
	if !fixture.ValidWorldBoss() {
		t.Fatal("隔离世界首领地图或原日程无效")
	}
}

// startDurableWorldBossNode 启动真实节点监听，停完原 gameNode 才批准新运行服租约。
func startDurableWorldBossNode(t *testing.T, cfg devconfig.Config, mode, listen string) func() {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- bootstrap.Run(ctx, cfg, mode) }()
	stop := sync.OnceFunc(func() {
		cancel()
		if err := <-done; err != nil {
			t.Errorf("隔离节点 %s 退出异常：%v", mode, err)
		}
	})
	t.Cleanup(stop)
	healthy(t, listen)
	return stop
}

func joinDurableWorldBossRole(t *testing.T, endpoint, token string, key *pb.RoleKey, run directoryTestEntry) *client {
	t.Helper()
	ticket := directoryTicket(t, endpoint, token, key, run)
	return join(t, directoryTicketAddress(t, ticket), ticket.Ticket)
}

func readDurableWorldBossRow(t *testing.T, store *persistence.Store, run uint64, occurrence string) persistence.WorldBossResult {
	t.Helper()
	var row persistence.WorldBossResult
	must(t, store.DB.Collection("world_boss_results").FindOne(t.Context(), bson.M{"run_server_id": run, "occurrence_id": occurrence}).Decode(&row))
	return row
}

// awaitDurableWorldBossState 以实际查询观察恢复确认或原计划开场。
func awaitDurableWorldBossState(t *testing.T, c *client, deadline time.Time, ready func(*pb.WorldBossState) bool) *pb.WorldBossState {
	t.Helper()
	for {
		state := queryWorldBossTest(t, c)
		if ready(state) {
			return state
		}
		if !time.Now().Before(deadline) {
			t.Fatal("等待世界首领状态超时", state)
		}
		time.Sleep(80 * time.Millisecond)
	}
}

func requireDurableWorldBossRanks(t *testing.T, ranks []*pb.WorldBossRank, first, second string) {
	t.Helper()
	want := []*pb.WorldBossRank{{Position: 1, DisplayName: first, Damage: 3}}
	if second != "" {
		want = append(want, &pb.WorldBossRank{Position: 2, DisplayName: second, Damage: 2})
	}
	if !slices.EqualFunc(ranks, want, func(left, right *pb.WorldBossRank) bool { return proto.Equal(left, right) }) {
		t.Fatal("真实玩家贡献或实际伤害排名不符", ranks, want)
	}
}

func requireDurableWorldBossKill(t *testing.T, row persistence.WorldBossResult, first, second *pb.RoleSummary, observed *pb.WorldBossResult,
	start, closes time.Time, fixture devconfig.Fixture) {
	t.Helper()
	if row.RunID != 1 || row.OccurrenceID != observed.OccurrenceId || row.TemplateID != observed.TemplateId || row.Outcome != persistence.WorldBossKilled ||
		observed.State != 4 || !row.ProgressKnown || row.HPMax != 5 || row.HPRemaining != 0 || len(row.Contributions) != 2 ||
		row.RuleVersion != fixture.RuleVersion || row.ConfigHash != fixture.Hash || !row.OpensAt.Equal(start) || !row.ClosesAt.Equal(closes) ||
		row.FinishedAt.UnixMilli() != observed.FinishedAtUnixMs || row.FinishedAt.Before(start) || !row.FinishedAt.Before(closes) {
		t.Fatal("MongoDB 原击杀证据与联机可查结果不符", row, observed)
	}
	for i, role := range []*pb.RoleSummary{first, second} {
		entry := row.Contributions[i]
		if entry.RoleID != domain.Hex(role.Key.RoleId) || entry.DisplayName != role.Name || entry.Damage != int64(3-i) ||
			entry.ReservationID == "" || entry.CombatSnapshot == "" || entry.JoinedAt.Before(start) || entry.JoinedAt.After(row.FinishedAt) {
			t.Fatal("MongoDB 没有完整保存两个真实角色及其有效贡献", entry)
		}
	}
}
