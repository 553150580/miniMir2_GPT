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
