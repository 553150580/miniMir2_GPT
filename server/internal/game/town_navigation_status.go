// 本文件报告城镇寻路终止，并按当前输入与移动修订维护导航反映。
package game

import "minimir/internal/townnav"

// navigationBlocked 规定导航阻挡的初始取值。
const navigationBlocked uint32 = 5

// 只终结当前目标身份；不可达与预算耗尽都不能伪报到达或每tick重扫整图。
// w 是调用该方法的房间执行器实例，r 指向当前执行器拥有并更新的目标房间。
func (w *worker) finishFailedTownNavigation(r *room) {
	// q 保存从城镇导航取出的城镇导航中的指定项。
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
