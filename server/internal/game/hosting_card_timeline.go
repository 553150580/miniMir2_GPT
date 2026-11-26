// 本文件仅为可能晚到的普通场景命中保留原托管卡期。
package game

import "time"

// hostingCardChange 记录购卡生效前对尚未处理命中适用的原卡期。
type hostingCardChange struct {
	at    time.Time // 本次购卡或续期的确认生效时刻。
	since time.Time // 上一个连续卡期的有效起点。
	until time.Time // 上一个连续卡期的有效终点。
}

// hostingCardUntilAt 从事件原时刻选择当时的托管卡期。
func (r *room) hostingCardUntilAt(at time.Time) time.Time {
	for _, change := range r.hostingCardHistory {
		if at.Before(change.at) {
			if change.until.After(at) && !at.Before(change.since) {
				return change.until
			}
			return time.Time{}
		}
	}
	if r.Role.HostingCardActiveAt(at) {
		return r.Role.HostingCardUntil
	}
	return time.Time{}
}

// earliestHostingCardEvent 查找仍可能在过去时刻命中的首个在途事件。
func (r *room) earliestHostingCardEvent(at time.Time) time.Time {
	for _, world := range r.worlds() {
		for _, attack := range world.Attacks {
			if attack.Due.Before(at) {
				at = attack.Due
			}
		}
		for _, cast := range world.Casts {
			if cast.Due.Before(at) {
				at = cast.Due
			}
		}
	}
	for _, event := range r.EventBatch {
		if event.At.Before(at) {
			at = event.At
		}
	}
	return at
}

// pruneHostingCardHistory 在命中越过购卡边界后清掉已无事件关联的原卡期。
func (r *room) pruneHostingCardHistory(now time.Time) {
	if len(r.hostingCardHistory) == 0 {
		return
	}
	earliest := r.earliestHostingCardEvent(now)
	for i, change := range r.hostingCardHistory {
		if earliest.Before(change.at) {
			r.hostingCardHistory = r.hostingCardHistory[i:]
			return
		}
	}
	r.hostingCardHistory = nil
}
