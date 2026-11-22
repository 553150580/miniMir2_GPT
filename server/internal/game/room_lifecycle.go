package game

import (
	"time"

	"minimir/internal/devconfig"
)

// 结清后保留一分钟健康重连窗口，覆盖客户端一次完整重连尝试。
// 房间配额吃紧时也只回收过期现场，避免把健康重连变成出生点初始化。
const offlineRoomRetention = time.Minute

func (w *worker) expireIdleRoom(roleID string, r *room, now time.Time) bool {
	if w.retainRoom(r) {
		r.idleSince = time.Time{}
		return false
	}
	if r.idleSince.IsZero() {
		r.idleSince = now
		return false
	}
	if now.Sub(r.idleSince) < offlineRoomRetention || w.e.stopped.Load() {
		return false
	}
	if r.Player.HP <= 0 && r.DeathID != "" && !w.retireRoomDeath(r) {
		return false
	}
	w.cancelTownNavigation(r)
	w.cancelMapTravelOnDetach(r)
	delete(w.rooms, roleID)
	return true
}

func (w *worker) releaseIdleSlot(now time.Time) bool {
	for roleID, r := range w.rooms {
		if w.expireIdleRoom(roleID, r, now) {
			return true
		}
	}
	return false
}

func (w *worker) retainRoom(r *room) bool {
	// 只由房间所属 worker 检查。隔离待恢复、已受理操作与尚可产出永久事实的事件都保留。
	if r.Conn != nil || r.Scene == 0 || r.Isolated || r.Pending || r.PendingRetry != nil ||
		r.PendingOperationID != "" || r.PendingBinding != nil || r.BindingCandidate != nil ||
		r.PendingPetBirth != nil || len(r.Waiting) != 0 || len(r.KillFacts) != 0 ||
		len(r.Wear.open) != 0 || len(r.Wear.inflight) != 0 || r.eventCount() != 0 {
		return true
	}
	if !r.Auto || r.Role.HostingSeconds <= 0 || r.Player.HP <= 0 {
		return false
	}
	// 城镇离线照常保留重连窗口，但没有挂机计费或自动战斗。
	m, known := w.currentMap(r)
	return known && m.SceneKindValue() != devconfig.SceneKindTown
}
