// 本文件累计真实离线托管时间，并把整秒并入原会话收益或独立检查点，在重连和停服前结清。
package game

import (
	"math"
	"time"

	"minimir/internal/persistence"
)

// 架构方案第9.3节约每3秒确认一次。事务在途时暂停模拟，保留原未确认量。
const hostingCheckpointSeconds = int64(3)

// meterHosting 累加已执行的离线托管时间，满足结算条件时暂停本帧新增行为。
// w 持有房间 r 的计量状态，delta 是本帧允许推进的真实时间，now 作为需要落盘时的来源时刻。
func (w *worker) meterHosting(r *room, delta time.Duration, now time.Time) bool {
	// 持久余额仍包含积攒的整秒；先扣出等待写入的部分才得到可用计费容量。
	// remaining 等于已持久余额减去尚未写入的整秒，限定当前还能继续推进的托管时间。
	remaining := r.Role.HostingSeconds - r.hostSecondsOpen
	if remaining <= 0 {
		w.flushHostingSeconds(r, now)
		return true
	}
	// elapsed 把本帧允许推进的时长换算为毫秒，余额只剩最后一秒时还要截断。
	elapsed := delta.Milliseconds()
	if remaining == 1 {
		// 最后一秒到达门槛即暂停，不能把越过余额的时间记入下一次领取。
		elapsed = min(elapsed, 1000-r.HostMS)
	}
	r.HostMS += elapsed
	r.hostSecondsOpen += r.HostMS / 1000
	// 余量留在同一健康房间，整秒进入下一笔可追溯结算。
	r.HostMS %= 1000
	if r.hostSecondsOpen >= hostingCheckpointSeconds || r.hostSecondsOpen >= r.Role.HostingSeconds {
		w.flushHostingSeconds(r, now)
		return true
	}
	return false
}

// flushHostingSeconds 用下一来源序号确认当前会话整秒，授权或序号不可信时隔离房间。
// w 是调用该方法的房间执行器实例，r 指向当前执行器拥有并更新的目标房间，now 提供当前时点，作为状态有效期和计时判断的基准。
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
// r 提供本次房间的托管会话与待结整秒，q 是等待提交的收益；旧会话已经产生的击杀不合并当前整秒。
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

// queueEntryAfterSettlement 在原托管结算未完成时暂存健康接管请求，避免换代后无法确认旧事实。
// w 是调用该方法的房间执行器实例，r 指向当前执行器拥有并更新的目标房间，c 保存等待接管房间的原入场请求，结清旧会话后交回分片继续处理。
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
// w 是调用该方法的房间执行器实例，r 指向当前执行器拥有并更新的目标房间，now 提供当前时点，作为状态有效期和计时判断的基准。
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

// retryPendingRoom 在退避到期后重投原任务，避免重新计算来源内容和随机结果。
// w 是调用该方法的房间执行器实例，r 指向当前执行器拥有并更新的目标房间，now 提供当前时点，作为状态有效期和计时判断的基准。
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
