package game

import (
	"minimir/internal/devconfig"
	"testing"
	"time"
)

func TestTownUnreachableTargetEndsMatchingNavigationWithoutClaimingArrival(t *testing.T) {
	w, r := navigationTownWorker(t, 5, 5, 2, 4, 1, 2, devconfig.Point{X: 3, Y: 3}, devconfig.Point{X: 3, Y: 4}, devconfig.Point{X: 4, Y: 3})
	goal := devconfig.Point{X: 4, Y: 4}
	requestTownGround(r, goal)
	w.beginTownNavigationTick(time.Now())
	w.move(r, &r.Player, goal, time.Now(), 1)
	w.endTownNavigationTick()
	w.move(r, &r.Player, goal, time.Now(), 1)
	if r.Destination != nil || r.Navigation.Status != navigationBlocked || r.Navigation.Destination == nil || *r.Navigation.Destination != goal || r.Player.Pos == goal {
		t.Fatal("不可达目标未以原目标身份结束，或被误报为到达")
	}
	if r.TownNavigation[0] != nil {
		t.Fatal("终态仍占用工作区或大路径缓冲")
	}
}
