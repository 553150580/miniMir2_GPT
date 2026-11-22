package game

import (
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
	"time"
)

// 死亡选择在世界退役前提交原身份，之后可释放世界且不触发免费复活。
func (w *worker) retireRoomDeath(r *room) bool {
	if r.Role.RetiredDeathID == r.DeathID {
		return true
	}
	auth, deathID, idleSince := r.Auth, r.DeathID, r.idleSince
	var j job
	j = job{target: w, run: func() func(*worker) {
		err := w.e.Store.PreserveRetiredDeath(w.e.ctx, auth, deathID)
		return func(w *worker) {
			if w.rooms[auth.RoleID] != r || r.Isolated || r.Auth != auth || r.DeathID != deathID {
				return
			}
			if err != nil {
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
