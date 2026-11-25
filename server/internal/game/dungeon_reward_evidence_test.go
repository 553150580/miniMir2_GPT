// 本文件核对最终首领与最终小区结束分开时，奖励采用的仍是击杀时权益。
package game

import (
	"testing"
	"time"

	"minimir/internal/devconfig"
	"minimir/internal/persistence"
)

func TestDungeonRewardsFreezeAtBossDeath(t *testing.T) {
	w, arena, first, second, _, started := dungeonBattleFixture(t)
	arena.grant.RunID = w.roomStore(first).Cfg.RunID
	arena.template.Rewards = &devconfig.DungeonRewards{}
	arena.stage = len(arena.template.Stages) - 1
	final := &arena.template.Stages[arena.stage]
	final.Enemies = append(final.Enemies, final.Enemies[0])
	arena.openStage()
	for _, seat := range arena.seats {
		seat.started = true
	}
	killed := started.Add(3 * time.Second)
	beforeExpiry := killed.Add(time.Millisecond)
	first.Role.PrivilegeUntil = beforeExpiry
	w.observeDungeonRewards(arena)

	// 分片处理延迟时，普通房间可能已经确认了后续续卡。击杀仍采用逻辑命中时刻。
	second.Role.PrivilegeUntil = killed.Add(time.Hour)
	w.observeChallengePrivilege(second, killed.Add(time.Millisecond))
	boss, extra := arena.monsters[0], arena.monsters[1]
	w.applyDungeonDamage(first, &first.Player, boss, boss.HP, killed.Add(time.Second), []hitContext{{At: killed}})
	if arena.outcome != persistence.DungeonActive || !arena.rewardAt.Equal(killed) || len(arena.rewardEvidence) != 2 ||
		arena.rewardEvidence[0].RoleID != first.Role.ID || !arena.rewardEvidence[0].PrivilegeUntil.Equal(beforeExpiry) ||
		arena.rewardEvidence[1].RoleID != second.Role.ID || !arena.rewardEvidence[1].PrivilegeUntil.IsZero() {
		t.Fatalf("最终首领死亡应立即冻结原真人的原权益：%v", arena.rewardEvidence)
	}
	finished := killed.Add(2 * time.Second)
	w.applyDungeonDamage(second, &second.Player, extra, extra.HP, finished, nil)
	w.freezeDungeonRewards(arena, time.Time{})
	if arena.outcome != persistence.DungeonSucceeded || !arena.finishedAt.Equal(finished) ||
		!arena.rewardAt.Equal(killed) || len(w.e.dungeonPrivileges.entries) != 0 {
		t.Fatal("最终清场改写了原击杀时刻或留下观察状态")
	}
}

func TestDungeonFailedBeforeKillReleasesRewardWatch(t *testing.T) {
	w, arena, first, _, _, _ := dungeonBattleFixture(t)
	arena.grant.RunID = w.roomStore(first).Cfg.RunID
	arena.template.Rewards = &devconfig.DungeonRewards{}
	w.observeDungeonRewards(arena)
	if len(w.e.dungeonPrivileges.entries) != len(arena.grant.Members) {
		t.Fatal("有奖模板未开始观察真人权益")
	}
	w.freezeDungeonRewards(arena, time.Time{})
	if len(w.e.dungeonPrivileges.entries) != 0 || len(arena.rewardEvidence) != 0 || !arena.rewardAt.IsZero() {
		t.Fatal("未击杀清理不得创建原奖励证据")
	}
}
