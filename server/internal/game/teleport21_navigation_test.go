// 本文件回归同地图瞬移保留原地面目标和已消耗导航预算。
package game

import (
	"minimir/internal/devconfig"
	"testing"
	"time"
)

// TestTeleport21SameWorldKeepsOriginalGroundGoalAndAccumulatedNavigationBudget 核验正常地面目标应取得有界任务，用诊断「真实新格未按原目标重规划，或累计预算被刷新」定位对应断言。
// t 提供断言失败报告、子用例和清理注册。
func TestTeleport21SameWorldKeepsOriginalGroundGoalAndAccumulatedNavigationBudget(t *testing.T) {
	// w 接收瞬移“21”房间执行器供后续步骤使用，r 接收瞬移“21”房间执行器供后续步骤使用，now 保存由瞬移“21”房间执行器取得的当前时间。
	w, r, _, _, now := teleport21Worker(t)
	// start 初始化用作起点或开始时间的点位。
	start := devconfig.Point{X: 650, Y: 631}
	if !w.activateWorld(r, r.TownWorld, start, nil) {
		t.Fatal("模型未移交到已配置Town")
	}
	w.e.workers = []*worker{w}
	// err 接收“initialize”城镇导航过程中可能产生的错误，供后续失败分支处理。
	if err := w.e.initializeTownNavigation(); err != nil {
		t.Fatal(err)
	}
	// goal 建立带初始值的点位，供本段继续处理。
	goal := devconfig.Point{X: 633, Y: 613}
	requestTownGround(r, goal)
	w.moveTown(r, &r.Player, goal, now, 600)
	w.beginTownNavigationTick(now)
	w.endTownNavigationTick()
	// request 保存从城镇导航取出的请求。
	request := r.TownNavigation[0]
	if request == nil || request.task == nil {
		t.Fatal("正常地面目标没有取得有界任务")
	}
	// task 保存当前处理所用的任务。
	task := request.task
	// before 保存由进度取得的操作前的状态。
	before := task.Progress()
	// revision 保存当前处理所用的修订号。
	revision := r.Navigation.Revision
	// draws 规定“draws”的初始取值。
	draws := 0
	// n 记录数值。
	w.roll = func(n int64) int64 {
		draws++
		if draws%2 == 1 {
			return n/2 + 1
		}
		return n / 2
	}
	// code 保存由施法取得的结果码。
	if code := w.cast(r, 21, 0, 1, now); code != 0 {
		t.Fatal("正常受理21失败", code)
	}
	w.advanceEvents(r, now.Add(200*time.Millisecond))
	if r.Player.Pos != (devconfig.Point{X: 651, Y: 631}) || r.Destination == nil || *r.Destination != goal || r.Navigation.Revision != revision || r.TownNavigation[0] != request || request.task != task {
		t.Fatal("同World瞬移清除了原地面目标或重建了预算任务", r.Player.Pos, r.Navigation)
	}
	// i 标记循环或列表中的当前位置。
	for i := 0; i < 4; i++ {
		navigationTownTick(w, r, goal, now.Add(time.Duration(700+i)*time.Millisecond))
	}
	// after 保存由进度取得的操作后的状态。
	after := task.Progress()
	if after.Replans != before.Replans+1 || after.TotalExpanded < before.TotalExpanded || r.Destination == nil || *r.Destination != goal || r.Navigation.Revision != revision {
		t.Fatal("真实新格未按原目标重规划，或累计预算被刷新", before, after, r.Navigation)
	}
}
