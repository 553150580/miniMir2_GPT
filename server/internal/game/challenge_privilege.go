// 本文件按原角色确认时间维护共享挑战需要的权益有效期。
package game

import (
	"sync"
	"time"
)

// challengePrivilegeKey 以角色和原运行服为键，跨健康重连和普通房间回收保留观察。
type challengePrivilegeKey struct {
	runID  uint64
	roleID string
}

// challengePrivilegeChange 保留原角色所有者已确认的特权有效期，以及本次确认的生效时刻。
type challengePrivilegeChange struct {
	at    time.Time
	until time.Time
}

// challengePrivilegeWitness 仅在同一挑战场次内收集特权事实。成员状态由观察簿互斥锁保护。
type challengePrivilegeWitness struct {
	key        challengePrivilegeKey
	occurrence string
	offers     int
	admitted   bool
	changes    []challengePrivilegeChange
}

// challengePrivilegeBook 协调角色房间和场次分片；没有本类挑战席位的角色不占用条目。
type challengePrivilegeBook struct {
	mu      sync.Mutex
	entries map[challengePrivilegeKey]*challengePrivilegeWitness
}

// offer 在原角色所有者确认显示后开始采集，首基线未确认时只占一份临时引用。
func (b *challengePrivilegeBook) offer(key challengePrivilegeKey, occurrence string, at, until time.Time) *challengePrivilegeWitness {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.entries == nil {
		b.entries = make(map[challengePrivilegeKey]*challengePrivilegeWitness)
	}
	watch := b.entries[key]
	if watch == nil || watch.occurrence != occurrence {
		watch = &challengePrivilegeWitness{key: key, occurrence: occurrence}
		b.entries[key] = watch
	}
	watch.offers++
	watch.record(at, until)
	return watch
}

// record 只接受状态实际变化，挂接或普通资产重复回执无需保留冗余历史。
func (w *challengePrivilegeWitness) record(at, until time.Time) {
	if n := len(w.changes); n != 0 {
		last := w.changes[n-1]
		if at.Before(last.at) || until.Equal(last.until) {
			return
		}
	}
	w.changes = append(w.changes, challengePrivilegeChange{at: at, until: until})
}

// confirmOffer 回收一份临时显示引用，已确认的原参战证据保留到场次终态。
func (b *challengePrivilegeBook) confirmOffer(watch *challengePrivilegeWitness, admitted bool) {
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
func (b *challengePrivilegeBook) observe(key challengePrivilegeKey, at, until time.Time) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if watch := b.entries[key]; watch != nil {
		watch.record(at, until)
	}
}

// finish 在原事件时刻取最后一个已生效值，再释放本场观察。较新的场次可以保留自己的条目。
func (b *challengePrivilegeBook) finish(watch *challengePrivilegeWitness, at time.Time) time.Time {
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

// observeChallengePrivilege 只向活动场次记录原角色已确认的权益。
func (w *worker) observeChallengePrivilege(r *room, at time.Time) {
	key := challengePrivilegeKey{runID: w.roomStore(r).Cfg.RunID, roleID: r.Role.ID}
	w.e.worldBossPrivileges.observe(key, at, r.Role.PrivilegeUntil)
	w.e.dungeonPrivileges.observe(key, at, r.Role.PrivilegeUntil)
}

// acceptRolePrivilege 同步普通场已受理动作和挑战权益观察。日常结算未改变特权时无需访问观察簿。
func (w *worker) acceptRolePrivilege(r *room, at, previous time.Time) {
	transitionEventPrivilege(r, at, r.Role.PrivilegeUntil)
	if !previous.Equal(r.Role.PrivilegeUntil) {
		w.observeChallengePrivilege(r, at)
	}
}
