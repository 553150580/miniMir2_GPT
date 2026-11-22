// 本文件在死亡房间回收前持久化复活所需的原死亡标记，确认后释放房间。
package game

import (
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
	"time"
)

// 死亡选择在世界退役前提交原身份，之后可释放世界且不触发免费复活。
// w 是调用该方法的房间执行器实例，r 指向当前执行器拥有并更新的目标房间。
func (w *worker) retireRoomDeath(r *room) bool {
	if r.Role.RetiredDeathID == r.DeathID {
		return true
	}
	// auth 保存当前处理所用的授权，deathID 标识当前业务使用的死亡，idleSince 保存当前处理所用的闲置“since”。
	auth, deathID, idleSince := r.Auth, r.DeathID, r.idleSince
	// j 记录当前位置的下标，使用任务或职业编号表示。
	var j job
	j = job{target: w, run: func() func(*worker) {
		// err 接收“preserve”已退役死亡过程中可能产生的错误，供后续失败分支处理。
		err := w.e.Store.PreserveRetiredDeath(w.e.ctx, auth, deathID)
		// w 记录房间执行器。
		return func(w *worker) {
			if w.rooms[auth.RoleID] != r || r.Isolated || r.Auth != auth || r.DeathID != deathID {
				return
			}
			if err != nil {
				// code 接收结果码供后续步骤使用。
				if code := persistence.Code(err); code == p.CodeOperationPending || code == p.CodeServerBusy {
					r.PendingRetry = &j
					r.RetryAt = time.Now().Add(500 * time.Millisecond)
				} else {
					w.isolateRoom(r)
				}
				return
			}
			r.Pending = false
			r.PendingRetry = nil
			r.Role.RetiredDeathID = deathID
			r.idleSince = idleSince
			w.after(r)
		}
	}}
	r.Pending = true
	if !w.submit(j) {
		r.PendingRetry = &j
		r.RetryAt = time.Now().Add(250 * time.Millisecond)
	}
	return false
}
