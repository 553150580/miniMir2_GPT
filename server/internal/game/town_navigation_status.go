package game

import "minimir/internal/townnav"

const navigationBlocked uint32 = 5

// 只终结当前目标身份；不可达与预算耗尽都不能伪报到达或每tick重扫整图。
func (w *worker) finishFailedTownNavigation(r *room) {
	q := r.TownNavigation[0]
	if q == nil || r.Destination == nil || q.inputRevision == 0 || q.inputRevision != r.Navigation.Revision || q.target != *r.Destination || r.Navigation.Status != navigationActive {
		return
	}
	if q.state != townnav.NoPath && q.state != townnav.Limited {
		return
	}
	r.Destination = nil
	r.Navigation.Status = navigationBlocked
	w.cancelTownRequest(q)
	w.snapshot(r)
}
