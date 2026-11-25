// 本文件记录世界首领的已确认席位和角色特权事实，并在场次终态按原业务时间冻结。
package game

import (
	"sync"
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

// worldBossPrivilegeKey 以角色和原运行服为键，跨健康重连和普通房间回收保留观察。
type worldBossPrivilegeKey struct {
	runID  uint64
	roleID string
}

// worldBossPrivilegeChange 保留原角色所有者已确认的特权有效期，以及本次确认的生效时刻。
type worldBossPrivilegeChange struct {
	at    time.Time
	until time.Time
}

// worldBossPrivilegeWitness 仅在一个原世界场次内收集特权事实。成员状态由观察簿互斥锁保护。
type worldBossPrivilegeWitness struct {
	key        worldBossPrivilegeKey
	occurrence string
	offers     int
	admitted   bool
	changes    []worldBossPrivilegeChange
}

// worldBossPrivilegeBook 协调角色房间和场次分片；没有世界首领席位的角色不占用条目。
type worldBossPrivilegeBook struct {
	mu      sync.Mutex
	entries map[worldBossPrivilegeKey]*worldBossPrivilegeWitness
}

// offer 在原角色所有者确认显示后开始采集，首基线未确认时只占一份临时引用。
func (b *worldBossPrivilegeBook) offer(key worldBossPrivilegeKey, occurrence string, at, until time.Time) *worldBossPrivilegeWitness {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.entries == nil {
		b.entries = make(map[worldBossPrivilegeKey]*worldBossPrivilegeWitness)
	}
	watch := b.entries[key]
	if watch == nil || watch.occurrence != occurrence {
		watch = &worldBossPrivilegeWitness{key: key, occurrence: occurrence}
		b.entries[key] = watch
	}
	watch.offers++
	watch.record(at, until)
	return watch
}

// record 只接受状态实际变化，挂接或普通资产重复回执无需保留冗余历史。
func (w *worldBossPrivilegeWitness) record(at, until time.Time) {
	if n := len(w.changes); n != 0 {
		last := w.changes[n-1]
		if at.Before(last.at) || until.Equal(last.until) {
			return
		}
	}
	w.changes = append(w.changes, worldBossPrivilegeChange{at: at, until: until})
}

// confirmOffer 回收一份临时显示引用，已确认的原参战证据保留到场次终态。
func (b *worldBossPrivilegeBook) confirmOffer(watch *worldBossPrivilegeWitness, admitted bool) {
	if watch == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	watch.offers--
	watch.admitted = watch.admitted || admitted
	if watch.offers == 0 && !watch.admitted && b.entries[watch.key] == watch {
		delete(b.entries, watch.key)
	}
}

// observe 由普通角色房间纳入已确认回执或健康新票时调用，与挑战场内临时身体分离。
func (b *worldBossPrivilegeBook) observe(key worldBossPrivilegeKey, at, until time.Time) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if watch := b.entries[key]; watch != nil {
		watch.record(at, until)
	}
}

// finish 在原事件时刻取最后一个已生效值，再释放本场观察。较新的场次可以保留自己的条目。
func (b *worldBossPrivilegeBook) finish(watch *worldBossPrivilegeWitness, at time.Time) time.Time {
	if watch == nil {
		return time.Time{}
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.entries[watch.key] == watch {
		delete(b.entries, watch.key)
	}
	for i := len(watch.changes) - 1; i >= 0; i-- {
		if change := watch.changes[i]; !change.at.After(at) {
			return change.until
		}
	}
	return time.Time{}
}

// observeWorldBossPrivilege 仅用当前权威角色确认后的资产视图刷新正在收集的席位。
func (w *worker) observeWorldBossPrivilege(r *room, at time.Time) {
	key := worldBossPrivilegeKey{runID: w.roomStore(r).Cfg.RunID, roleID: r.Role.ID}
	w.e.worldBossPrivileges.observe(key, at, r.Role.PrivilegeUntil)
}

// acceptRolePrivilege 同步普通场已受理动作和世界首领观察。日常结算未改变特权时无需访问观察簿。
func (w *worker) acceptRolePrivilege(r *room, at, previous time.Time) {
	transitionEventPrivilege(r, at, r.Role.PrivilegeUntil)
	if !previous.Equal(r.Role.PrivilegeUntil) {
		w.observeWorldBossPrivilege(r, at)
	}
}

// worldBossPresenceChange 保存已确认席位状态变更，按规则事件时间回溯终态资格。
type worldBossPresenceChange struct {
	at       time.Time
	presence persistence.WorldBossPresence
}

// worldBossRewardMember 独立于活体席位，玩家离场后仍保留原入场与后续确认事实。
type worldBossRewardMember struct {
	origin     uint64                     // 原角色的永久入口归属。
	watch      *worldBossPrivilegeWitness // 当前场次观察的角色特权变化。
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
