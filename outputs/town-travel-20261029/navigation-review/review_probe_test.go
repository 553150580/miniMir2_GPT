package game

import (
	"minimir/internal/devconfig"
	"minimir/internal/townnav"
	"testing"
	"time"
)

// 独立只读审查：用合法冻结模拟纯暂停，不改变请求身份、位置或占格。
func TestReviewPendingPauseDoesNotClearUnchangedGroundTarget(t *testing.T) {
	w, r := navigationTownWorker(t, 32, 32, 128, 256, 1, 2)
	now := time.Unix(1, 0)
	target := devconfig.Point{X: 10, Y: 1}
	requestTownGround(r, target)
	r.Player.NextMove = now.Add(24 * time.Hour)
	w.move(r, &r.Player, target, now, 1)
	w.beginTownNavigationTick(now)
	q := r.TownNavigation[0]
	if q == nil || q.state != townnav.Ready {
		t.Fatal("审查前置：未生成路线")
	}
	for i := 0; i < 4; i++ {
		r.Pending = true
		w.beginTownNavigationTick(now.Add(time.Duration(i+1) * time.Second))
		r.Pending = false
		w.move(r, &r.Player, target, now.Add(time.Duration(i+1)*time.Second), 1)
		w.endTownNavigationTick()
		w.move(r, &r.Player, target, now.Add(time.Duration(i+1)*time.Second), 1)
		if r.Destination == nil || r.Navigation.Status != navigationActive {
			t.Fatalf("第%d次纯Pending暂停恢复清除地面目标: status=%d state=%s position=%v target=%v", i+1, r.Navigation.Status, q.state, r.Player.Pos, target)
		}
	}
}

// 收束期间只能等已有事件，不能开始新的路径搜索或继续占住共享大缓冲。
func TestReviewQuiescingSuspendsNavigationBeforeWorkerAdvance(t *testing.T) {
	w, r := navigationTownWorker(t, 32, 32, 2, 4, 1, 2)
	now := time.Unix(1, 0)
	target := devconfig.Point{X: 30, Y: 1}
	requestTownGround(r, target)
	w.move(r, &r.Player, target, now, 1)
	r.Travel = &mapTravelPlan{Phase: travelQuiescing}
	w.beginTownNavigationTick(now)
	q := r.TownNavigation[0]
	if w.townNavigation.used != 0 || q.storage != nil || w.townNavigation.workspaces[0].InUse() {
		t.Fatalf("旅行收束仍开始规划并占用共享池: used=%d state=%s routeHeld=%t workspaceBusy=%t", w.townNavigation.used, q.state, q.storage != nil, w.townNavigation.workspaces[0].InUse())
	}
}
