// 本文件用实际世界首领终态、角色新票和原资产回执验证任务、成就与图鉴来源。
package integration

import (
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
	"minimir/internal/wire"
)

// TestSharedWorldBossTaskProgressMongoWS 通过实际扣血建账，验证没有排名附件的合格玩家也能获得进度。
func TestSharedWorldBossTaskProgressMongoWS(t *testing.T) {
	ctx, store, endpoint, route := startChallengePlayerDeathServer(t, func(cfg *devconfig.Config) {
		start := time.Now().UTC().Truncate(time.Millisecond).Add(-2 * time.Second)
		setDurableWorldBossFixture(t, &cfg.Fixture, start, start.Add(10*time.Minute), 5*time.Minute)
		boss := cfg.Fixture.WorldBoss
		boss.Rewards = &devconfig.WorldBossRewards{MinimumDamage: 1, AllowDisconnected: true,
			Ranks: []devconfig.WorldBossRankReward{{FirstRank: 1, LastRank: 1, Items: []devconfig.ChallengeRewardItem{{ItemID: 95, Quantity: 1}}}}}
		cfg.Fixture.TasksCodex = &devconfig.TaskCodexRules{DailyPoolSize: 1, DailyClaimLimit: 1, DailyActivityTarget: 1,
			Definitions: []devconfig.TaskCodexDefinition{
				{ID: 1, Kind: devconfig.TaskCodexDaily, Name: "每日世界首领", Event: devconfig.TaskCodexWorldBossKill, TemplateID: boss.TemplateID, Activity: 1, Target: 1},
				{ID: 1, Kind: devconfig.TaskCodexAchievement, Name: "世界首领成就", Event: devconfig.TaskCodexWorldBossKill, Target: 2},
				{ID: 1, Kind: devconfig.TaskCodexMonster, Name: "世界首领收藏", Event: devconfig.TaskCodexMonsterKill, MonsterID: boss.Monster.ID, MapID: boss.Map.ID, Target: 1, Reward: devconfig.TaskCodexReward{BoundYuanbaoUnits: 13}},
				{ID: 1, Kind: devconfig.TaskCodexActivity, Name: "当日活跃", Target: 1},
			}}
		must(t, cfg.Fixture.ValidateTasksCodex())
	})
	trial := worldBossRewardTrial{ctx: ctx, store: store, endpoint: endpoint, route: route}
	privilege := time.Now().UTC().Add(time.Hour).Truncate(time.Millisecond)
	firstRole := trial.create(t, "任务首领甲", privilege, false)
	secondRole := trial.create(t, "任务首领乙", privilege, false)
	idleRole := trial.create(t, "任务首领丙", privilege, false)
	first, second, idle := trial.attach(t, &firstRole, true), trial.attach(t, &secondRole, true), trial.attach(t, &idleRole, false)
	open := queryWorldBossTest(t, first)
	enterWorldBossTest(t, first, open)
	enterWorldBossTest(t, second, open)
	enterWorldBossTest(t, idle, open)
	goal := &pb.GridPoint{X: 6, Y: 4}
	second.send(p.MsgC2SMoveIntent, &pb.MoveIntent{SceneEpoch: second.entry.SceneEpoch, Destination: goal})
	travelUntilWithin(second, 5*time.Second, func(frame wire.Frame) bool {
		return frame.ID == p.MsgS2CSceneBaseline && proto.Equal(second.player().Position, goal)
	})
	attackWorldBossTest(first)
	waitWorldBossTest(t, second, 0, func(state *pb.WorldBossState) bool { return state.State == 3 && state.Hp == 2 })
	first.close()
	travelUntilWithin(second, 5*time.Second, func(frame wire.Frame) bool {
		return frame.ID == p.MsgS2CSceneBaseline && townPeer(second, firstRole.summary.Name) == nil
	})
	time.Sleep(time.Duration(second.player().MoveIntervalMs)*time.Millisecond + 100*time.Millisecond)
	attackWorldBossTest(second)
	sealed := waitWorldBossTest(t, second, 0, func(state *pb.WorldBossState) bool { return state.State == 4 })
	requireDurableWorldBossRanks(t, sealed.Ranks, firstRole.summary.Name, secondRole.summary.Name)
	returnFromWorldBossTest(second, open.OccurrenceId)
	returnFromWorldBossTest(idle, open.OccurrenceId)
	first = trial.attach(t, &firstRole, false)
	result := readDurableWorldBossRow(t, store, route.origin.RunID, open.OccurrenceId)
	if result.Outcome != persistence.WorldBossKilled || len(result.Contributions) != 3 ||
		result.Contributions[0].RewardEvidence == nil || result.Contributions[0].RewardEvidence.Presence != persistence.WorldBossPresenceOffline ||
		result.Contributions[1].Damage != 2 || result.Contributions[2].Damage != 0 {
		t.Fatal("实际世界终态未记录三个原席位的离线与贡献证据", result)
	}
	if len(reconnectBossInfo(t, first).WorldPendingRewards) != 1 || len(reconnectBossInfo(t, second).WorldPendingRewards) != 0 {
		t.Fatal("隔离排名奖应只提供给第一名")
	}
	finishedDay := result.FinishedAt.In(time.FixedZone("Asia/Shanghai", 8*60*60)).Format(time.DateOnly)
	for _, participant := range []struct {
		conn *client
		role worldBossRewardPlayer
		want int64
	}{{first, firstRole, 1}, {second, secondRole, 1}, {idle, idleRole, 0}} {
		role, err := store.Role(ctx, domain.Hex(participant.role.summary.Key.RoleId))
		must(t, err)
		for range 2 {
			state := queryWorldBossTaskProgress(t, participant.conn)
			if state.RoleRevision != uint64(role.Revision) || len(state.Rows) != 4 || role.TasksCodex != nil {
				t.Fatal("世界进度查询改动或遗漏了原角色版本", state, role.TasksCodex)
			}
			for _, row := range state.Rows {
				want := participant.want
				if row.Kind == devconfig.TaskCodexActivity || (row.Kind == devconfig.TaskCodexDaily && state.Day != finishedDay) {
					want = 0
				}
				if row.Progress != want || row.Claimable != (want >= row.Target) || row.Claimed {
					t.Fatal("世界击杀按原席位应恰好累计一次任务、成就和图鉴", participant.role.summary.Name, row)
				}
			}
		}
	}

	// 没有拿到原排行附件的乙也能从初始进度领取一次图鉴，原请求重放不会再次到账。
	claim := &pb.TaskCodexClaimRequest{Operation: second.operation(), Kind: devconfig.TaskCodexMonster, Id: 1}
	accepted := second.asset(p.MsgC2STaskCodexClaim, claim)
	second.inventoryAt(accepted.NewRevision)
	if accepted.ErrorCode != 0 || second.asset(p.MsgC2STaskCodexClaim, claim).ErrorCode != 0 {
		t.Fatal("世界图鉴不能按原操作成功领取和查证", accepted)
	}
	second.close()
	second = trial.attach(t, &secondRole, false)
	state := queryWorldBossTaskProgress(t, second)
	for _, row := range state.Rows {
		if row.Kind == devconfig.TaskCodexMonster && (row.Progress != 1 || !row.Claimed || row.Claimable) ||
			row.Kind == devconfig.TaskCodexAchievement && row.Progress != 1 {
			t.Fatal("新票后世界计数重复或丢失领取事实", row)
		}
	}
	role, err := store.Role(ctx, domain.Hex(secondRole.summary.Key.RoleId))
	must(t, err)
	if role.Wallet.BoundYuanbao != 13 {
		t.Fatal("原图鉴领取未恰好确认一笔绑定元宝", role.Wallet)
	}
}

func queryWorldBossTaskProgress(t *testing.T, conn *client) *pb.TaskCodexStateResult {
	t.Helper()
	request := conn.send(p.MsgC2STaskCodexState, &pb.TaskCodexStateRequest{})
	frame := travelUntilWithin(conn, 3*time.Second, func(frame wire.Frame) bool { return frame.Request == request })
	state := &pb.TaskCodexStateResult{}
	if frame.ID != p.MsgS2CTaskCodexState || frame.Error != 0 {
		t.Fatal("查询世界首领任务返回了错误消息", frame)
	}
	must(t, proto.Unmarshal(frame.Payload, state))
	if state.ErrorCode != 0 {
		t.Fatal("查询世界首领任务失败", state)
	}
	return state
}
