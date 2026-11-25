// 本文件验证任务图鉴的来源边界、跨日切换和领取原子性。
package persistence

import (
	"context"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	p "minimir/internal/protocol"
	"os"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func TestChallengeTaskProgressCombinesSourcesWithoutCopyingClaimedCounts(t *testing.T) {
	at := time.Date(2026, 11, 26, 15, 59, 59, 0, time.UTC)
	f := &devconfig.Fixture{TasksCodex: &devconfig.TaskCodexRules{DailyClaimLimit: 1, DailyActivityTarget: 100, Definitions: []devconfig.TaskCodexDefinition{
		{ID: 1, Kind: devconfig.TaskCodexAchievement, Event: devconfig.TaskCodexMonsterKill, Target: 4},
		{ID: 2, Kind: devconfig.TaskCodexAchievement, Event: devconfig.TaskCodexChallengeMonsterKill, Target: 2},
		{ID: 1, Kind: devconfig.TaskCodexMonster, Event: devconfig.TaskCodexMonsterKill, MonsterID: 80001, MapID: 80, Target: 2,
			Reward: devconfig.TaskCodexReward{BoundYuanbaoUnits: 11}},
		{ID: 1, Kind: devconfig.TaskCodexDaily, Event: devconfig.TaskCodexWorldBossKill, TemplateID: 1, Target: 1, Activity: 100},
	}}}
	role := domain.Role{Revision: 7, Level: 1}
	role = advanceChallengeTasks(role, f, devconfig.TaskCodexPersonalBossClear, 1, 80, 80001, at)
	world := advanceWorldBossTaskProgress(role, domain.TaskCodexProgress{}, f, 1, 81, 80001, at)
	state := TaskCodexStateWithWorldBossProgress(role, f, world, at)
	if state.RoleRevision != 7 || state.Rows[0].Progress != 0 || state.Rows[1].Progress != 2 || !state.Rows[2].Claimable || !state.Rows[3].Claimable {
		t.Fatalf("挑战事件混入野外或未按原身份合并：%+v", state)
	}
	request := &pb.TaskCodexClaimRequest{Kind: devconfig.TaskCodexMonster, Id: 1}
	claimed, _, code := claimTaskCodex(role, f, request, at, "三类源验证", world)
	key := taskCodexKey(devconfig.TaskCodexMonster, 1)
	if code != 0 || claimed.Wallet.BoundYuanbao != 11 || claimed.TasksCodex.Permanent[key].Count != 1 || !claimed.TasksCodex.Permanent[key].Claimed || world.Permanent[key].Count != 1 {
		t.Fatalf("独立世界计数进入了原角色账或领奖失败：%+v %d", claimed.TasksCodex, code)
	}
	if _, _, code = claimTaskCodex(claimed, f, request, at, "重复图鉴", world); code != p.CodeTaskCodexUnavailable {
		t.Fatalf("已领取项目再次进入奖励事务：%d", code)
	}
	afterMidnight := at.Add(2 * time.Second)
	state = TaskCodexStateWithWorldBossProgress(claimed, f, world, afterMidnight)
	if state.Rows[2].Progress != 2 || !state.Rows[2].Claimed || state.Rows[3].Progress != 0 {
		t.Fatalf("跨日未正确保留永久图鉴并清零昨日世界任务：%+v", state)
	}
}

// TestTaskCodexQualifiedKillsAcrossMaps 确认全局怪物图鉴与指定地图主线采用各自资格。
func TestTaskCodexQualifiedKillsAcrossMaps(t *testing.T) {
	at := time.Date(2026, 11, 23, 10, 0, 0, 0, time.UTC)
	f := &devconfig.Fixture{TasksCodex: &devconfig.TaskCodexRules{Definitions: []devconfig.TaskCodexDefinition{
		{ID: 1, Kind: devconfig.TaskCodexMain, Event: devconfig.TaskCodexMonsterKill, Target: 1, MonsterID: 9, MapID: 4},
		{ID: 2, Kind: devconfig.TaskCodexMain, Event: devconfig.TaskCodexLevelReached, Target: 5, PrerequisiteID: 1},
		{ID: 1, Kind: devconfig.TaskCodexMonster, Event: devconfig.TaskCodexMonsterKill, Target: 2, MonsterID: 9, MapID: 4},
	}}}
	before := domain.Role{Level: 50, Revision: 1}
	wrongMap := advanceTasksCodex(before, f, taskCodexEvent{kind: devconfig.TaskCodexMonsterKill, count: 1, monsterID: 9, mapID: 1, at: at})
	confirm := advanceTasksCodex(wrongMap, f, taskCodexEvent{kind: devconfig.TaskCodexMonsterKill, count: 1, monsterID: 9, mapID: 4, at: at})
	old, next := TaskCodexState(wrongMap, f, at), TaskCodexState(confirm, f, at)
	if before.TasksCodex != nil || old.Rows[0].Progress != 0 || old.Rows[2].Progress != 1 || next.Rows[0].Progress != 1 || next.Rows[2].Progress != 2 || next.Rows[1].Available {
		t.Fatalf("跨地图击杀、原状态或前置不正确：原 %v，新 %v", old, next)
	}
	completed, _, code := claimTaskCodex(confirm, f, &pb.TaskCodexClaimRequest{Kind: devconfig.TaskCodexMain, Id: 1}, at, "quest", domain.TaskCodexProgress{})
	if state := TaskCodexState(completed, f, at); code != 0 || !state.Rows[1].Claimable || !state.Rows[0].Claimed {
		t.Fatalf("完成前置后未识别角色已有等级：结果 %d，%v", code, state)
	}
}

// TestTaskCodexShanghaiDailyClaims 验证日活上限、上海零点和晚到事件隔离。
func TestTaskCodexShanghaiDailyClaims(t *testing.T) {
	beforeMidnight := time.Date(2026, 11, 23, 15, 59, 59, 0, time.UTC)
	afterMidnight := beforeMidnight.Add(time.Second)
	rules := &devconfig.TaskCodexRules{DailyPoolSize: 3, DailyClaimLimit: 2, DailyActivityTarget: 35}
	f := &devconfig.Fixture{TasksCodex: rules}
	for id := uint32(1); id <= rules.DailyPoolSize; id++ {
		f.TasksCodex.Definitions = append(f.TasksCodex.Definitions, devconfig.TaskCodexDefinition{
			ID: id, Kind: devconfig.TaskCodexDaily, Name: "隔离日常", Event: devconfig.TaskCodexItemRecycle, Target: 1, Activity: 20,
		})
	}
	f.TasksCodex.Definitions = append(f.TasksCodex.Definitions, devconfig.TaskCodexDefinition{
		ID: 1, Kind: devconfig.TaskCodexActivity, Name: "隔离活跃", Target: 35, Reward: devconfig.TaskCodexReward{BoundGold: 50},
	})
	if err := f.ValidateTasksCodex(); err != nil {
		t.Fatal(err)
	}
	r := advanceTasksCodex(domain.Role{Level: 5}, f, taskCodexEvent{kind: devconfig.TaskCodexItemRecycle, count: 1, at: beforeMidnight})
	for id := uint32(1); id <= rules.DailyClaimLimit; id++ {
		var code uint32
		r, _, code = claimTaskCodex(r, f, &pb.TaskCodexClaimRequest{Kind: devconfig.TaskCodexDaily, Id: id, Day: "2026-11-23"}, beforeMidnight, "daily", domain.TaskCodexProgress{})
		if code != 0 {
			t.Fatalf("第 %d 个每日任务未成功：%d", id, code)
		}
	}
	state := TaskCodexState(r, f, beforeMidnight)
	if state.Day != "2026-11-23" || state.DailyClaimed != 2 || state.DailyClaimLimit != 2 || state.Activity != 35 || state.Rows[2].Claimable || !state.Rows[3].Claimable {
		t.Fatalf("每日领取或活跃上限有误：%v", state)
	}
	activity := &pb.TaskCodexClaimRequest{Kind: devconfig.TaskCodexActivity, Id: 1, Day: state.Day}
	r, receipt, code := claimTaskCodex(r, f, activity, beforeMidnight, "activity", domain.TaskCodexProgress{})
	if code != 0 || receipt.Day != state.Day || r.Wallet.BoundGold != 50 {
		t.Fatal("活跃奖励没有进入绑定余额", code, receipt)
	}
	if _, _, code = claimTaskCodex(r, f, activity, beforeMidnight, "activity-duplicate", domain.TaskCodexProgress{}); code != p.CodeTaskCodexUnavailable {
		t.Fatal("已经领取的活跃资格被再次使用", code)
	}
	state = TaskCodexState(r, f, afterMidnight)
	if state.Day != "2026-11-24" || state.DailyClaimed != 0 || state.Activity != 0 || state.Rows[0].Progress != 0 || state.Rows[3].Claimed {
		t.Fatalf("上海零点没有切换到新运营日：%v", state)
	}
	if _, _, code = claimTaskCodex(r, f, activity, afterMidnight, "late-activity", domain.TaskCodexProgress{}); code != p.CodeTaskCodexDayChanged {
		t.Fatal("跨日沿用了旧日期的奖励领取", code)
	}
	r = advanceTasksCodex(r, f, taskCodexEvent{kind: devconfig.TaskCodexItemRecycle, count: 1, at: afterMidnight})
	late := advanceTasksCodex(r, f, taskCodexEvent{kind: devconfig.TaskCodexItemRecycle, count: 1, at: beforeMidnight})
	if late.TasksCodex != r.TasksCodex || TaskCodexState(late, f, afterMidnight).Rows[0].Progress != 1 {
		t.Fatal("晚到事件覆盖了当前运营日任务")
	}
}

// TestTaskCodexRewardBoundAndBagAtomic 确认奖励按来源绑定，任一奖励放不下时原角色和资格保持原状。
func TestTaskCodexRewardBoundAndBagAtomic(t *testing.T) {
	at := time.Now()
	f := &devconfig.Fixture{Items: []devconfig.Item{{ID: 1, Stackable: true, MaxStack: 5}}, TasksCodex: &devconfig.TaskCodexRules{Definitions: []devconfig.TaskCodexDefinition{
		{ID: 1, Kind: devconfig.TaskCodexMain, Event: devconfig.TaskCodexLevelReached, Target: 1,
			Reward: devconfig.TaskCodexReward{BoundGold: 7, BoundYuanbaoUnits: 9, Items: []devconfig.TaskCodexRewardItem{{ItemID: 1, Quantity: 6}}}},
	}}}
	q := &pb.TaskCodexClaimRequest{Kind: devconfig.TaskCodexMain, Id: 1}
	r := domain.Role{Level: 1, Capacity: 1, PrivilegeUntil: at.Add(time.Hour)}
	if got, receipt, code := claimTaskCodex(r, f, q, at, "original-op", domain.TaskCodexProgress{}); code != p.CodeEquipmentBagFull || receipt != nil || got.TasksCodex != nil || got.Wallet != r.Wallet || len(got.Items) != 0 {
		t.Fatalf("背包不足时出现部分领奖：%+v %+v %d", got, receipt, code)
	}
	r.Capacity = 2
	next, receipt, code := claimTaskCodex(r, f, q, at, "success-op", domain.TaskCodexProgress{})
	if code != 0 || len(next.Items) != 2 || len(receipt.Items) != 2 || next.Wallet.BoundGold != 7 || next.Wallet.BoundYuanbao != 9 || next.Wallet.Gold != 0 || next.Wallet.Yuanbao != 0 {
		t.Fatalf("绑定奖励到账有误：%+v %+v %d", next, receipt, code)
	}
	for index, item := range next.Items {
		if item.ID == "" || !item.Bound || item.SourceKind != "task_reward" || item.SourceID != "success-op" || item.Quantity != []int64{5, 1}[index] {
			t.Fatalf("奖励物品身份、来源或堆叠有误：%+v", item)
		}
	}
}

// TestTaskCodexReplayAndReconnectMongo 用隔离数据库验证击杀和奖励确认重放、重进场及双角色隔离。
func TestTaskCodexReplayAndReconnectMongo(t *testing.T) {
	if os.Getenv("MINIMIR_MONGO_E2E") != "1" {
		t.Skip("需要专属开发 MongoDB 集成测试")
	}
	require := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	cfg, err := devconfig.Load("../../../conf/dev/server.json")
	require(err)
	cfg.Database = "minimir_dev_game_test_tasks_codex_" + domain.ID()[:16]
	monsterID := cfg.Fixture.Monsters[0].ID
	cfg.Fixture.TasksCodex = &devconfig.TaskCodexRules{Definitions: []devconfig.TaskCodexDefinition{
		{ID: 1, Kind: devconfig.TaskCodexMonster, Name: "隔离测试图鉴", Event: devconfig.TaskCodexMonsterKill, MonsterID: monsterID, Target: 2,
			Reward: devconfig.TaskCodexReward{BoundYuanbaoUnits: 11}},
	}}
	require(cfg.Fixture.ValidateTasksCodex())
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
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
	grant, err := account.ApproveGame(ctx, cfg.NodeID, game.Instance, cfg.RunID)
	require(err)
	game.Epoch = grant.Epoch
	login, err := account.Login(ctx, "codex_"+domain.ID()[:8], "test_123456", true)
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
	key := create(1, "图鉴主角色")
	otherKey := create(2, "图鉴第二角色")
	enter := func() (domain.Role, Auth) {
		t.Helper()
		ticket, err := account.IssueTicket(ctx, accountAuth, key)
		require(err)
		role, auth, err := game.Enter(ctx, ticket.Ticket)
		require(err)
		return role, auth
	}
	r, auth := enter()
	at := time.Now()
	first := Settlement{SourceSeq: r.SourceSeq + 1, Kills: 1, MonsterID: monsterID, MapID: cfg.Fixture.Map.ID, EventAt: at, HostingSession: domain.ID()}
	settled, firstOp, err := game.SettleWithReceipt(ctx, auth, first)
	require(err)
	replayed, replayOp, err := game.SettleWithReceipt(ctx, auth, first)
	require(err)
	if replayOp.ID != firstOp.ID || firstOp.Settlement.MonsterID != monsterID || replayed.Revision != settled.Revision || TaskCodexState(replayed, &cfg.Fixture, at).Rows[0].Progress != 1 {
		t.Fatal("相同击杀来源被再次计入图鉴")
	}
	changed := first
	changed.MonsterID++
	if _, _, err = game.SettleWithReceipt(ctx, auth, changed); Code(err) != p.CodeOperationConflict {
		t.Fatal("同一来源允许更换击杀对象", err)
	}
	second := first
	second.SourceSeq++
	second.HostingSession = ""
	settled, _, err = game.SettleWithReceipt(ctx, auth, second)
	require(err)
	if !TaskCodexState(settled, &cfg.Fixture, at).Rows[0].Claimable {
		t.Fatal("在线击杀或托管击杀未计入图鉴")
	}
	request := func(r domain.Role, fingerprint string) AssetRequest {
		op := &pb.OperationContext{OpId: domain.Bytes(domain.ID()), ExpectedRevision: uint64(r.Revision),
			CommandSeq: uint64(r.EconomicSeq + 1), ExpectedRuleVersion: cfg.Fixture.RuleVersion}
		return AssetRequest{Kind: AssetTaskCodexClaim, Fingerprint: fingerprint, Context: op,
			TaskCodex: &pb.TaskCodexClaimRequest{Operation: op, Kind: devconfig.TaskCodexMonster, Id: 1}}
	}
	claim := request(settled, "codex-claim")
	claimed, result, claimOp, err := game.AssetWithReceipt(ctx, auth, claim)
	require(err)
	if result.ErrorCode != 0 || claimOp.TaskCodex == nil || claimOp.TaskCodex.BoundYuanbaoUnits != 11 || claimed.Wallet.BoundYuanbao != settled.Wallet.BoundYuanbao+11 {
		t.Fatalf("首次图鉴领奖未提交：%v %+v", result, claimOp)
	}
	var audit struct {
		Receipt *TaskCodexClaimReceipt `bson:"tasks_codex_claim"` // 业务事务记录的原领取明细。
	}
	require(game.DB.Collection("audit_outbox").FindOne(ctx, bson.M{"_id": claimOp.ID}).Decode(&audit))
	if audit.Receipt == nil || audit.Receipt.BoundYuanbaoUnits != 11 {
		t.Fatal("任务图鉴回执未与奖励一起保存到审计")
	}
	resumed, nextAuth := enter()
	if nextAuth.SessionEpoch <= auth.SessionEpoch || !TaskCodexState(resumed, &cfg.Fixture, at).Rows[0].Claimed {
		t.Fatal("重进场没有恢复原角色图鉴")
	}
	if _, _, _, err = game.AssetWithReceipt(ctx, auth, claim); Code(err) != p.CodeStaleSession {
		t.Fatal("旧角色会话仍能读取或更改领取", err)
	}
	entryRevision := resumed.Revision
	resumed, result, sameOp, err := game.AssetWithReceipt(ctx, nextAuth, claim)
	require(err)
	if result.ErrorCode != 0 || sameOp.ID != claimOp.ID || sameOp.Revision != claimOp.Revision || sameOp.TaskCodex == nil || resumed.Wallet != claimed.Wallet || resumed.Revision != entryRevision {
		t.Fatal("重连后重放原领取没有得到同一确认或重复发放", result)
	}
	remaining, result, sameOp, err := game.AssetWithReceipt(ctx, nextAuth, request(resumed, "duplicate-claim"))
	require(err)
	if result.ErrorCode != p.CodeTaskCodexUnavailable || sameOp.TaskCodex != nil || remaining.Wallet != claimed.Wallet {
		t.Fatal("更换操作编号后重复领取同一资格", result)
	}
	other, err := game.Role(ctx, domain.Hex(otherKey.RoleId))
	require(err)
	if s := TaskCodexState(other, &cfg.Fixture, at); s.Rows[0].Progress != 0 || s.Rows[0].Claimed || other.Wallet.BoundYuanbao != r.Wallet.BoundYuanbao {
		t.Fatal("同账号第二角色获得了图鉴进度或奖励")
	}
}
