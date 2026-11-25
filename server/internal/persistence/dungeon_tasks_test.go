// 本文件核对讨伐原击杀顺序及清场所在的上海运营日。
package persistence

import (
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	"slices"
	"testing"
	"time"
)

// TestDungeonTaskProgressUsesActualKillDays 把两只早期敌人和清场分在不同运营日。
func TestDungeonTaskProgressUsesActualKillDays(t *testing.T) {
	midnight := time.Date(2026, 11, 26, 16, 0, 0, 0, time.UTC)
	enemy := devconfig.DungeonEnemy{Monster: devconfig.Monster{ID: 20}}
	template := devconfig.DungeonConfig{TemplateID: 1, Map: devconfig.MapTemplate{ID: 22}, Stages: []devconfig.DungeonStage{
		{Enemies: []devconfig.DungeonEnemy{enemy}}, {Enemies: []devconfig.DungeonEnemy{enemy}},
		{Enemies: []devconfig.DungeonEnemy{enemy}}, {Enemies: []devconfig.DungeonEnemy{enemy}},
	}}
	record := dungeonRecord{DungeonChallenge: DungeonChallenge{TemplateID: template.TemplateID,
		RuleVersion: 30, ConfigHash: "隔离规则", StartedAt: midnight.Add(-4 * time.Second)}}
	fixture := devconfig.Fixture{RuleVersion: record.RuleVersion, Hash: record.ConfigHash, Dungeons: []devconfig.DungeonConfig{template},
		TasksCodex: &devconfig.TaskCodexRules{Definitions: []devconfig.TaskCodexDefinition{
			{ID: 1, Kind: devconfig.TaskCodexDaily, Event: devconfig.TaskCodexDungeonClear, TemplateID: template.TemplateID, Target: 5},
			{ID: 2, Kind: devconfig.TaskCodexDaily, Event: devconfig.TaskCodexChallengeMonsterKill, MapID: template.Map.ID, Target: 10},
			{ID: 1, Kind: devconfig.TaskCodexMonster, Event: devconfig.TaskCodexMonsterKill, MonsterID: enemy.Monster.ID, Target: 20},
		}}}
	store := &Store{Cfg: devconfig.Config{Fixture: fixture}}
	q := DungeonFinish{Outcome: DungeonSucceeded, FinishedAt: midnight.Add(time.Second)}
	var err error
	q.KillEvidence, err = normalizeDungeonKills(q.Outcome, []DungeonKillEvidence{
		{Stage: 2, At: midnight}, {Stage: 1, At: midnight.Add(-time.Second)},
		{Stage: 3, At: q.FinishedAt}, {Stage: 0, At: midnight.Add(-2 * time.Second)},
	})
	if err != nil {
		t.Fatal(err)
	}
	roles := []domain.Role{{Level: 1}}
	if err = store.advanceDungeonTasks(&record, q, roles); err != nil {
		t.Fatal(err)
	}
	state := TaskCodexState(roles[0], &fixture, q.FinishedAt)
	if state.Day != "2026-11-27" || state.Rows[0].Progress != 1 || state.Rows[1].Progress != 2 || state.Rows[2].Progress != 4 {
		t.Fatal("清场归入新运营日，每日击杀只记新日两只，图鉴保留全部四只", state)
	}
	if !slices.Equal(record.KillEvidence, q.KillEvidence) || state.Rows[1].Claimed || state.Activity != 0 {
		t.Fatal("原证据应持久化，进度不得自动领取", record.KillEvidence, state.Activity)
	}

	badOrder := slices.Clone(q.KillEvidence)
	badOrder[2].Stage, badOrder[3].Stage = 3, 2
	q.KillEvidence = badOrder
	if validDungeonKills(record, q, &template) {
		t.Fatal("不能把下一段死亡放在上一段之前")
	}
}
