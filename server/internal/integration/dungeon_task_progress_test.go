// 本文件为讨伐实战验证准备独立任务定义并通过原 WebSocket 查询角色进度。
package integration

import (
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	p "minimir/internal/protocol"
	"minimir/internal/wire"
)

// configureDungeonTrialTasks 只在隔离联机配置开放三类讨伐计数和一项普通对照。
func configureDungeonTrialTasks(cfg *devconfig.Config) {
	dungeon := cfg.Fixture.DungeonReleases[0]
	monster := dungeon.Stages[0].Enemies[0].MonsterID
	cfg.Fixture.TasksCodex = &devconfig.TaskCodexRules{
		DailyPoolSize: 1, DailyClaimLimit: 1, DailyActivityTarget: 100,
		Definitions: []devconfig.TaskCodexDefinition{
			{ID: 1, Kind: devconfig.TaskCodexDaily, Name: "隔离讨伐每日", Event: devconfig.TaskCodexDungeonClear,
				MapID: dungeon.Map.ID, TemplateID: dungeon.TemplateID, Target: 1, Activity: 100},
			{ID: 1, Kind: devconfig.TaskCodexAchievement, Name: "隔离讨伐成就", Event: devconfig.TaskCodexDungeonClear,
				TemplateID: dungeon.TemplateID, Target: 2},
			{ID: 1, Kind: devconfig.TaskCodexMonster, Name: "隔离讨伐图鉴", Event: devconfig.TaskCodexMonsterKill,
				MonsterID: monster, MapID: dungeon.Map.ID, Target: 8},
			{ID: 2, Kind: devconfig.TaskCodexAchievement, Name: "隔离普通击杀", Event: devconfig.TaskCodexMonsterKill, Target: 10},
			{ID: 1, Kind: devconfig.TaskCodexActivity, Name: "隔离讨伐活跃", Target: 100},
		},
	}
}

// assertDungeonTaskProgress 验证原连接能读到已确认的通关与各段击杀，普通击杀不受影响。
func assertDungeonTaskProgress(t *testing.T, c *client, clears, kills int64) {
	t.Helper()
	request := c.send(p.MsgC2STaskCodexState, &pb.TaskCodexStateRequest{})
	frame := travelUntilWithin(c, 3*time.Second, func(frame wire.Frame) bool { return frame.Request == request })
	state := &pb.TaskCodexStateResult{}
	if frame.ID != p.MsgS2CTaskCodexState || frame.Error != 0 {
		t.Fatalf("讨伐任务查询未返回原角色状态：%+v", frame)
	}
	must(t, proto.Unmarshal(frame.Payload, state))
	if state.ErrorCode != 0 || len(state.Rows) != 5 {
		t.Fatal("隔离任务目录未完整开放", state)
	}
	expected := map[[2]uint32]int64{
		{devconfig.TaskCodexDaily, 1}:       clears,
		{devconfig.TaskCodexAchievement, 1}: clears,
		{devconfig.TaskCodexMonster, 1}:     kills,
		{devconfig.TaskCodexAchievement, 2}: 0,
		{devconfig.TaskCodexActivity, 1}:    0,
	}
	for _, row := range state.Rows {
		key := [2]uint32{row.Kind, row.Id}
		progress, ok := expected[key]
		if !ok || row.Progress != progress || row.Claimed || row.Claimable != (row.Kind == devconfig.TaskCodexDaily && clears == 1) {
			t.Fatal("原真人讨伐进度、普通击杀或领奖状态不符", row)
		}
		delete(expected, key)
	}
}
