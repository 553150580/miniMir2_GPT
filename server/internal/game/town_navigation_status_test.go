// 本文件回归城镇无路时结束匹配导航并向客户端报告失败。
package game

import (
	"minimir/internal/devconfig"
	"testing"
	"time"
)

// TestTownUnreachableTargetEndsMatchingNavigationWithoutClaimingArrival 核对「不可达目标未以原目标身份结束，或被误报为到达」与「终态仍占用工作区或大路径缓冲」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestTownUnreachableTargetEndsMatchingNavigationWithoutClaimingArrival(t *testing.T) {
	// w 接收导航城镇房间执行器供后续步骤使用，r 接收导航城镇房间执行器供后续步骤使用。
	w, r := navigationTownWorker(t, 5, 5, 2, 4, 1, 2, devconfig.Point{X: 3, Y: 3}, devconfig.Point{X: 3, Y: 4}, devconfig.Point{X: 4, Y: 3})
	// goal 建立带初始值的点位，供本段继续处理。
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
