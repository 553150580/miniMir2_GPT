// 本文件把已经受原玩法事务确认的挑战终态和击杀投影为任务事实。
package persistence

import (
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	"time"
)

// matches 让已有野外任务保持原范围，按编号配置的怪物目标则可接收同一怪物的合格挑战击杀。
func (event taskCodexEvent) matches(def devconfig.TaskCodexDefinition) bool {
	if event.templateID != 0 {
		return def.MatchesChallengeTaskEvent(event.kind, event.templateID, event.mapID, event.monsterID)
	}
	return def.Event == event.kind && (def.MonsterID == 0 || def.MonsterID == event.monsterID) &&
		(def.Kind == devconfig.TaskCodexMonster || def.MapID == 0 || def.MapID == event.mapID)
}

func challengeTaskEvent(kind string, templateID uint32, mapID uint64, monsterID uint32, at time.Time) taskCodexEvent {
	return taskCodexEvent{kind: kind, count: 1, templateID: templateID, mapID: mapID, monsterID: monsterID, at: at}
}

// advanceChallengeTasks 同时记录玩法成功与按实际死亡确认的首领击杀；多人分段击杀由原场次单独提交。
func advanceChallengeTasks(r domain.Role, f *devconfig.Fixture, kind string, templateID uint32, mapID uint64, monsterID uint32, at time.Time) domain.Role {
	if templateID == 0 || mapID == 0 || at.IsZero() {
		return r
	}
	r = advanceTasksCodex(r, f, challengeTaskEvent(kind, templateID, mapID, 0, at))
	return advanceChallengeMonsterTasks(r, f, templateID, mapID, monsterID, at)
}

// advanceChallengeMonsterTasks 逐只处理原实例的真实击杀，而不从入场次数、掉落或胜负推导怪物数量。
func advanceChallengeMonsterTasks(r domain.Role, f *devconfig.Fixture, templateID uint32, mapID uint64, monsterID uint32, at time.Time) domain.Role {
	if templateID == 0 || mapID == 0 || monsterID == 0 || at.IsZero() {
		return r
	}
	return advanceTasksCodex(r, f, challengeTaskEvent(devconfig.TaskCodexChallengeMonsterKill, templateID, mapID, monsterID, at))
}

// advanceWorldBossTaskProgress 只向独立世界计数器增量写入，不借用瞬时场景或角色修订。
func advanceWorldBossTaskProgress(r domain.Role, progress domain.TaskCodexProgress, f *devconfig.Fixture, templateID uint32, mapID uint64, monsterID uint32, at time.Time) domain.TaskCodexProgress {
	finish := challengeTaskEvent(devconfig.TaskCodexWorldBossKill, templateID, mapID, 0, at)
	progress, _ = advanceTaskCodexProgress(r, progress, f, finish)
	kill := challengeTaskEvent(devconfig.TaskCodexChallengeMonsterKill, templateID, mapID, monsterID, at)
	progress, _ = advanceTaskCodexProgress(r, progress, f, kill)
	return progress
}
