// 本文件追踪讨伐链真人席位的已确认特权，按最终首领原死亡时刻冻结。
package game

import (
	"time"

	"minimir/internal/persistence"
)

// observeDungeonRewards 从通过扣次的真人快照开始观察，不为无奖表场次分配状态。
func (w *worker) observeDungeonRewards(arena *dungeonArena) {
	if arena.outcome != persistence.DungeonActive || arena.template.Rewards == nil {
		return
	}
	book := &w.e.dungeonPrivileges
	for _, member := range arena.grant.Members {
		role := arena.seats[member.RoleID].room.Role
		key := challengePrivilegeKey{runID: arena.grant.RunID, roleID: member.RoleID}
		watch := book.offer(key, arena.grant.ChallengeID, arena.grant.StartedAt, role.PrivilegeUntil)
		book.confirmOffer(watch, true)
		arena.rewardWatches = append(arena.rewardWatches, watch)
	}
}

// freezeDungeonRewards 取原首领死亡时每位真人的权益，零时间仅释放失败场次的观察。
func (w *worker) freezeDungeonRewards(arena *dungeonArena, at time.Time) {
	if len(arena.rewardWatches) == 0 {
		return
	}
	arena.rewardAt = at
	for index, watch := range arena.rewardWatches {
		until := w.e.dungeonPrivileges.finish(watch, at)
		if !at.IsZero() {
			member := arena.grant.Members[index]
			arena.rewardEvidence = append(arena.rewardEvidence, persistence.DungeonRewardEvidence{
				RoleID: member.RoleID, PrivilegeUntil: until,
			})
		}
	}
	arena.rewardWatches = nil
}
