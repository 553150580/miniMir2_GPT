// 本文件记录世界首领的已确认席位和角色特权事实，并在场次终态按原业务时间冻结。
package game

import (
	"sync/atomic"
	"time"

	"minimir/internal/persistence"
)

// worldBossSuspension 带跨分片断线时钟，证据保留此值不会延长旧套接字的生命周期。
type worldBossSuspension struct {
	unixNano atomic.Int64 // 断线处理器读取的实际 Unix 时间，未确认时为零。
}

// at 返回连接控制面记录的原失效时刻，未收到事件时沿用调用者的观察时间。
func (s *worldBossSuspension) at(fallback time.Time) time.Time {
	if s != nil {
		if tick := s.unixNano.Load(); tick != 0 {
			return time.Unix(0, tick)
		}
	}
	return fallback
}

// worldBossPresenceChange 保存已确认席位状态变更，按规则事件时间回溯终态资格。
type worldBossPresenceChange struct {
	at       time.Time
	presence persistence.WorldBossPresence
}

// worldBossRewardMember 独立于活体席位，玩家离场后仍保留原入场与后续确认事实。
type worldBossRewardMember struct {
	origin     uint64                     // 原角色的永久入口归属。
	watch      *challengePrivilegeWitness // 当前场次观察的角色特权变化。
	suspension *worldBossSuspension       // 最近确认附着的断线时钟，可核对尚未路由到场次的断线通知。
	presence   []worldBossPresenceChange  // 保留离场前后状态及原模拟时间。
}

// confirmWorldBossRewardMember 只有原基线已确认后才建立本场参战证据，健康新票沿用原席位。
func (w *worker) confirmWorldBossRewardMember(arena *worldBossArena, seat *worldBossSeat, at time.Time) {
	watch := seat.privilegeOffer
	if watch == nil {
		return
	}
	seat.privilegeOffer = nil
	w.e.worldBossPrivileges.confirmOffer(watch, true)
	if arena.rewardMembers == nil {
		arena.rewardMembers = make(map[string]*worldBossRewardMember)
	}
	role := seat.room.Role
	if arena.rewardMembers[role.ID] == nil {
		arena.rewardMembers[role.ID] = &worldBossRewardMember{origin: role.Origin, watch: watch}
	}
	arena.rewardMembers[role.ID].suspension = seat.observation.suspension
	arena.noteRewardPresence(role.ID, persistence.WorldBossPresenceOnline, at)
}

// discardWorldBossRewardOffer 释放未确认的新附着，已入场的场次贡献由原参战条目继续保留。
func (w *worker) discardWorldBossRewardOffer(seat *worldBossSeat) {
	if watch := seat.privilegeOffer; watch != nil {
		seat.privilegeOffer = nil
		w.e.worldBossPrivileges.confirmOffer(watch, false)
	}
}

// noteRewardPresence 只更新首个终态之前的事实。旧连接的迟到信号不得倒拨已确认的重连。
func (a *worldBossArena) noteRewardPresence(roleID string, presence persistence.WorldBossPresence, at time.Time) {
	member := a.rewardMembers[roleID]
	if member == nil || a.challenge.finish != nil {
		return
	}
	if n := len(member.presence); n > 0 {
		last := member.presence[n-1]
		if last.presence == presence || last.presence == persistence.WorldBossPresenceDefeated {
			return
		}
		if at.Before(last.at) {
			at = last.at
		}
	}
	member.presence = append(member.presence, worldBossPresenceChange{at: at, presence: presence})
}

// rewardPresenceAt 返回原击杀或超时刻已受理的最后一个实际席位状态。
func (m *worldBossRewardMember) rewardPresenceAt(at time.Time) persistence.WorldBossPresence {
	for i := len(m.presence) - 1; i >= 0; i-- {
		if current := m.presence[i]; !current.at.After(at) {
			if current.presence == persistence.WorldBossPresenceOnline {
				// 其他席位的离线通知可能仍在跨分片排队，已记录的原断线时间仍具有权威性。
				if suspended := m.suspension.at(time.Time{}); !suspended.IsZero() && !suspended.After(at) {
					return persistence.WorldBossPresenceOffline
				}
			}
			return current.presence
		}
	}
	return 0
}

// freezeWorldBossRewardEvidence 在首次终态固定全体原参战证据，后续数据库重试沿用这个快照。
func (w *worker) freezeWorldBossRewardEvidence(a *worldBossArena) {
	finish := a.challenge.finish
	if finish == nil || len(a.rewardMembers) == 0 {
		return
	}
	if finish.kind == bossDefeated || finish.kind == bossEscaped {
		a.rewardEvidence = make(map[string]*persistence.WorldBossRewardEvidence, len(a.rewardMembers))
	}
	for roleID, member := range a.rewardMembers {
		until := w.e.worldBossPrivileges.finish(member.watch, finish.finishedAt)
		if a.rewardEvidence != nil {
			a.rewardEvidence[roleID] = &persistence.WorldBossRewardEvidence{
				Origin: member.origin, Presence: member.rewardPresenceAt(finish.finishedAt), PrivilegeUntil: until,
			}
		}
	}
	a.rewardMembers = nil
}
