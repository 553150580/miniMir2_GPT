package game

import (
	"minimir/internal/devconfig"
	"testing"
	"time"
)

func TestTeleport21SameWorldKeepsOriginalGroundGoalAndAccumulatedNavigationBudget(t *testing.T) {
	w, r, _, _, now := teleport21Worker(t)
	start := devconfig.Point{X: 650, Y: 631}
	if !w.activateWorld(r, r.TownWorld, start, nil) {
		t.Fatal("模型未移交到已配置Town")
	}
	w.e.workers = []*worker{w}
	if err := w.e.initializeTownNavigation(); err != nil {
		t.Fatal(err)
	}
	goal := devconfig.Point{X: 633, Y: 613}
	requestTownGround(r, goal)
	w.moveTown(r, &r.Player, goal, now, 600)
	w.beginTownNavigationTick(now)
	w.endTownNavigationTick()
	request := r.TownNavigation[0]
	if request == nil || request.task == nil {
		t.Fatal("正常地面目标没有取得有界任务")
	}
	task := request.task
	before := task.Progress()
	revision := r.Navigation.Revision
	draws := 0
	w.roll = func(n int64) int64 {
		draws++
		if draws%2 == 1 {
			return n/2 + 1
		}
		return n / 2
	}
	if code := w.cast(r, 21, 0, 1, now); code != 0 {
		t.Fatal("正常受理21失败", code)
	}
	w.advanceEvents(r, now.Add(200*time.Millisecond))
	if r.Player.Pos != (devconfig.Point{X: 651, Y: 631}) || r.Destination == nil || *r.Destination != goal || r.Navigation.Revision != revision || r.TownNavigation[0] != request || request.task != task {
		t.Fatal("同World瞬移清除了原地面目标或重建了预算任务", r.Player.Pos, r.Navigation)
	}
	for i := 0; i < 4; i++ {
		navigationTownTick(w, r, goal, now.Add(time.Duration(700+i)*time.Millisecond))
	}
	after := task.Progress()
	if after.Replans != before.Replans+1 || after.TotalExpanded < before.TotalExpanded || r.Destination == nil || *r.Destination != goal || r.Navigation.Revision != revision {
		t.Fatal("真实新格未按原目标重规划，或累计预算被刷新", before, after, r.Navigation)
	}
}
