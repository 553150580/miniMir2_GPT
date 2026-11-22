package game

import (
	"math"
	"time"

	"minimir/internal/persistence"
)

// 架构方案第9.3节约每3秒确认一次。事务在途时暂停模拟，保留原未确认量。
const hostingCheckpointSeconds = int64(3)

func (w *worker) meterHosting(r *room, delta time.Duration, now time.Time) bool {
	remaining := r.Role.HostingSeconds - r.hostSecondsOpen
	if remaining <= 0 {
		w.flushHostingSeconds(r, now)
		return true
	}
	elapsed := delta.Milliseconds()
	if remaining == 1 {
		// 最后一秒到达门槛即暂停，不能把越过余额的时间记入下一次领取。
		elapsed = min(elapsed, 1000-r.HostMS)
	}
	r.HostMS += elapsed
	r.hostSecondsOpen += r.HostMS / 1000
	r.HostMS %= 1000
	if r.hostSecondsOpen >= hostingCheckpointSeconds || r.hostSecondsOpen >= r.Role.HostingSeconds {
		w.flushHostingSeconds(r, now)
		return true
	}
	return false
}

func (w *worker) flushHostingSeconds(r *room, now time.Time) bool {
	if r.hostSecondsOpen == 0 || r.Pending || r.Isolated {
		return false
	}
	if r.HostSession == "" || r.Role.SourceSeq == math.MaxInt64 {
		w.isolateRoom(r)
		return true
	}
	w.settle(r, persistence.Settlement{
		SourceSeq: r.Role.SourceSeq + 1, HostingSession: r.HostSession, EventAt: now,
	})
	return true
}

// 将整秒冻结到同一会话的击杀或磨损事务。旧会话事件继续沿原身份确认。
func attachHostingSeconds(r *room, q *persistence.Settlement) int64 {
	if r.Conn != nil || r.hostSecondsOpen == 0 || r.HostSession == "" {
		return 0
	}
	if q.HostingSession != r.HostSession {
		if q.HostingSession != "" || q.Kills != 0 || q.StopReason != 0 {
			return 0
		}
		q.HostingSession = r.HostSession
	}
	q.HostSeconds = r.hostSecondsOpen
	return q.HostSeconds
}

func (w *worker) queueEntryAfterSettlement(r *room, c command) bool {
	if !r.Pending && r.hostSecondsOpen == 0 {
		return false
	}
	if len(r.Waiting) >= 2 {
		c.conn.Close("role busy")
		return true
	}
	r.Waiting = append(r.Waiting, c)
	if !r.Pending {
		w.drainRoomSettlement(r, time.Now())
	}
	return true
}

// 只结清已经发生的事实。接管和受控停服都复用原任务，等待期间不再模拟战斗或扣时。
func (w *worker) drainRoomSettlement(r *room, now time.Time) bool {
	if r.Isolated {
		return false
	}
	if r.Pending {
		w.retryPendingRoom(r, now)
		return true
	}
	w.flushKillFacts(r)
	if r.Pending || len(r.KillFacts) > 0 || r.BindingCandidate != nil {
		return true
	}
	return w.flushWear(r, now, true) || w.flushHostingSeconds(r, now)
}

func (w *worker) retryPendingRoom(r *room, now time.Time) {
	if r.PendingRetry == nil || now.Before(r.RetryAt) {
		return
	}
	if w.submit(*r.PendingRetry) {
		r.PendingRetry = nil
	} else {
		// 给执行器留出回收队列槽位的时间，重试沿用原任务和流水。
		r.RetryAt = now.Add(250 * time.Millisecond)
	}
}
