// 本文件判定房间何时仍需保留，按闲置窗口回收，并在容量吃紧时回收已到期房间。
package game

import (
	"time"

	"minimir/internal/devconfig"
)

// 结清后保留一分钟健康重连窗口，覆盖客户端一次完整重连尝试。
// 房间配额吃紧时也只回收过期现场，避免把健康重连变成出生点初始化。
const offlineRoomRetention = time.Minute

// expireIdleRoom 更新闲置窗口，确认死亡标记后才删除已到期的房间。
// w 是调用该方法的房间执行器实例，roleID 标识当前业务使用的角色，r 指向当前执行器拥有并更新的目标房间，now 提供当前时点，作为状态有效期和计时判断的基准。
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
	// 死亡角色先保全可恢复的死亡事实，数据库未确认前继续持有现场。
	if r.Player.HP <= 0 && r.DeathID != "" && !w.retireRoomDeath(r) {
		return false
	}
	w.cancelTownNavigation(r)
	w.cancelMapTravelOnDetach(r)
	w.leaveSharedTown(r)
	delete(w.rooms, roleID)
	return true
}

// releaseIdleSlot 在房间配额吃紧时回收一个达到正常期限的现场。
// w 是调用该方法的房间执行器实例，now 提供当前时点，作为状态有效期和计时判断的基准。
func (w *worker) releaseIdleSlot(now time.Time) bool {
	// roleID 定位房间集合中正在处理的成员，r 接收遍历房间集合时当前的元素。
	for roleID, r := range w.rooms {
		if w.expireIdleRoom(roleID, r, now) {
			return true
		}
	}
	return false
}

// retainRoom 判定房间是否仍承载连接、在途事实或可运行的离线战斗。
// w 是调用该方法的房间执行器实例，r 指向当前执行器拥有并更新的目标房间。
func (w *worker) retainRoom(r *room) bool {
	// 只由房间所属 worker 检查。隔离待恢复、已受理操作与尚可产出永久事实的事件都保留。
	if r.Conn != nil || r.Scene == 0 || r.Isolated || r.Pending || r.PendingRetry != nil || r.personalBoss != nil || r.worldBossObservation != nil ||
		r.dungeonStart != nil || r.dungeonMatch != nil || r.mirrorMatch != nil ||
		r.PendingOperationID != "" || r.PendingBinding != nil || r.BindingCandidate != nil || r.hostSecondsOpen != 0 ||
		r.PendingPetBirth != nil || len(r.Waiting) != 0 || len(r.KillFacts) != 0 ||
		len(r.Wear.open) != 0 || len(r.Wear.inflight) != 0 || r.eventCount() != 0 {
		return true
	}
	if !r.Auto || r.Role.HostingSeconds <= 0 || r.Player.HP <= 0 {
		return false
	}
	// 城镇离线照常保留重连窗口，但没有挂机计费或自动战斗。
	// m 接收当前状态地图供后续步骤使用，known 保存由当前状态地图取得的已知状态。
	m, known := w.currentMap(r)
	return known && m.SceneKindValue() != devconfig.SceneKindTown
}
