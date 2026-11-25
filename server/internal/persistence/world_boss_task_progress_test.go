// 本文件核对世界任务采用原场的获准贡献与席位，而非奖励附件或榜单名次。
package persistence

import (
	"maps"
	"testing"
	"time"

	"minimir/internal/devconfig"
	p "minimir/internal/protocol"
)

func TestWorldBossTaskEligibility(t *testing.T) {
	fixture := devconfig.Fixture{TasksCodex: &devconfig.TaskCodexRules{Definitions: []devconfig.TaskCodexDefinition{
		{ID: 1, Kind: devconfig.TaskCodexAchievement, Event: devconfig.TaskCodexWorldBossKill, TemplateID: 2, Target: 2},
	}}}
	template := &devconfig.WorldBossConfig{TemplateID: 2, Rewards: &devconfig.WorldBossRewards{MinimumDamage: 3, AllowDisconnected: true}}
	finish := WorldBossFinish{Outcome: WorldBossKilled}
	for i, contribution := range []struct {
		damage   int64
		presence WorldBossPresence
	}{{3, WorldBossPresenceOnline}, {4, WorldBossPresenceOffline}, {10, WorldBossPresenceLeft},
		{8, WorldBossPresenceDefeated}, {1, WorldBossPresenceOnline}, {0, WorldBossPresenceOnline}} {
		row := testWorldBossContribution(byte(i+1), contribution.damage, time.Now())
		row.RewardEvidence = &WorldBossRewardEvidence{Origin: 7, Presence: contribution.presence}
		finish.Contributions = append(finish.Contributions, row)
	}
	eligible, err := worldBossTaskParticipants(&fixture, template, finish)
	want := map[string]uint64{finish.Contributions[0].RoleID: 7, finish.Contributions[1].RoleID: 7}
	if err != nil || !maps.Equal(eligible, want) {
		t.Fatal("世界任务未沿用原有效贡献和席位", eligible, err)
	}
	for _, outcome := range []WorldBossOutcome{WorldBossExpired, WorldBossInterrupted} {
		finish.Outcome = outcome
		if got, err := worldBossTaskParticipants(&fixture, template, finish); err != nil || len(got) != 0 {
			t.Fatal("未击杀的原终态生成了任务进度", outcome, got, err)
		}
	}
	finish.Outcome = WorldBossKilled
	finish.Contributions[0].RewardEvidence = nil
	if _, err := worldBossTaskParticipants(&fixture, template, finish); Code(err) != p.CodeInvalidArgument {
		t.Fatal("缺少原席位和永久入口仍可累计世界进度", err)
	}
	template.TemplateID = 3
	if got, err := worldBossTaskParticipants(&fixture, template, finish); err != nil || len(got) != 0 {
		t.Fatal("未被配置目标消费的场次仍要求额外证据", got, err)
	}
}
