// 本文件验证任务和图鉴发布配置的目标来源、链路及奖励边界。
package devconfig

import "testing"

// TestTaskCodexConfigRequiresConfirmedDefinitions 禁止悬空日活、环形主线、重复怪物和背包图鉴卡。
func TestTaskCodexConfigRequiresConfirmedDefinitions(t *testing.T) {
	valid := func() Fixture {
		return Fixture{
			Monsters: []Monster{{ID: 7}}, Items: []Item{{ID: 9}}, TasksCodex: &TaskCodexRules{Definitions: []TaskCodexDefinition{
				{ID: 1, Kind: TaskCodexMain, Name: "先做主线", Event: TaskCodexSkillLearn, Target: 1},
				{ID: 2, Kind: TaskCodexMain, Name: "下一步", Event: TaskCodexLevelReached, Target: 7, PrerequisiteID: 1},
				{ID: 1, Kind: TaskCodexMonster, Name: "隔离图鉴", Event: TaskCodexMonsterKill, Target: 3, MonsterID: 7},
			}},
		}
	}
	f := valid()
	if err := f.ValidateTasksCodex(); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name   string                // 被拒绝的配置错误。
		mutate func(*TaskCodexRules) // 只更改该用例针对的定义。
	}{
		{"主线循环", func(r *TaskCodexRules) { r.Definitions[0].PrerequisiteID = 2 }},
		{"非可信事件", func(r *TaskCodexRules) { r.Definitions[0].Event = "client_reports" }},
		{"同怪物重复图鉴", func(r *TaskCodexRules) {
			definition := r.Definitions[2]
			definition.ID++
			r.Definitions = append(r.Definitions, definition)
		}},
		{"图鉴奖励污染背包", func(r *TaskCodexRules) {
			r.Definitions[2].Reward.Items = []TaskCodexRewardItem{{ItemID: 9, Quantity: 1}}
		}},
		{"孤立活跃奖励", func(r *TaskCodexRules) {
			r.Definitions = append(r.Definitions, TaskCodexDefinition{ID: 1, Kind: TaskCodexActivity, Name: "活跃", Target: 100})
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			f := valid()
			test.mutate(f.TasksCodex)
			if err := f.ValidateTasksCodex(); err == nil {
				t.Fatal("无效定义没有被拒绝")
			}
		})
	}
}

func TestChallengeTaskConfigUsesDeclaredTemplatesAndParticipation(t *testing.T) {
	personal := &PersonalBossConfig{TemplateID: 1, Map: MapTemplate{ID: 80}, Monster: Monster{ID: 80001}}
	world := &WorldBossConfig{TemplateID: 1, Map: MapTemplate{ID: 81}, Monster: Monster{ID: 80002}}
	fixture := Fixture{PersonalBoss: personal, WorldBoss: world}
	personalQuest := TaskCodexDefinition{Kind: TaskCodexAchievement, ID: 1, Name: "个人胜利", Target: 2,
		Event: TaskCodexPersonalBossClear, TemplateID: 1, MapID: 80}
	worldQuest := TaskCodexDefinition{Kind: TaskCodexAchievement, ID: 2, Name: "世界参战", Target: 2,
		Event: TaskCodexWorldBossKill, TemplateID: 1, MapID: 81}
	fixture.TasksCodex = &TaskCodexRules{Definitions: []TaskCodexDefinition{personalQuest, worldQuest}}
	if err := fixture.ValidateTasksCodex(); err == nil {
		t.Fatalf("未配置有效世界参与资格却开启了任务进度：%v", err)
	}
	world.Rewards = &WorldBossRewards{}
	fixture.TasksCodex.Definitions = append(fixture.TasksCodex.Definitions, TaskCodexDefinition{Kind: TaskCodexMonster, ID: 1,
		Name: "世界首领", Target: 2, Event: TaskCodexMonsterKill, MapID: 81, MonsterID: 80002})
	if err := fixture.ValidateTasksCodex(); err != nil || !fixture.HasWorldBossTaskProgress() {
		t.Fatalf("有效地图、怪物和资格未注册：%v", err)
	}
	fixture.TasksCodex.Definitions[1].MapID = 80
	if err := fixture.ValidateTasksCodex(); err == nil {
		t.Fatal("同模板编号的另一玩法地图不能作为世界击杀来源")
	}
	fixture.WorldBoss = nil
	fixture.TasksCodex.Definitions = []TaskCodexDefinition{personalQuest, {Kind: TaskCodexAchievement, ID: 2,
		Name: "跨挑战累计", Target: 2, Event: TaskCodexChallengeMonsterKill}}
	if err := fixture.ValidateTasksCodex(); err != nil || !fixture.HasWorldBossTaskProgress() {
		t.Fatalf("关闭世界首领后仍需合并既有跨挑战计数：%v", err)
	}
}
