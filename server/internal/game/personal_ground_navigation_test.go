// 本文件核验点地和原追击首步分别使用正确的终点语义。
package game

import (
	"testing"
	"time"

	"minimir/internal/devconfig"
)

// TestPersonalGroundNavigationFollowsClearDirections 验证全八个方向的直线路径。
func TestPersonalGroundNavigationFollowsClearDirections(t *testing.T) {
	w := newPersonalNavigationTestWorker(13, nil)
	m := &w.e.Store.Cfg.Fixture.Map
	start := devconfig.Point{X: 6, Y: 6}
	for _, d := range directions {
		target := devconfig.Point{X: start.X + 3*d.X, Y: start.Y + 3*d.Y}
		r := newPersonalNavigationTestRoom(start)
		r.Destination, r.Navigation.Status = &target, navigationActive
		for i := 1; i <= 3; i++ {
			want := devconfig.Point{X: start.X + i*d.X, Y: start.Y + i*d.Y}
			step, found := w.personalNavigation.findFirstStep(m, r, &r.Player, target)
			if !found || step != want {
				t.Fatalf("直走到格%+v的第%d步=%+v，预期%+v", target, i, step, want)
			}
			r.Player.Pos = step
		}
	}
}

// TestPersonalGroundNavigationUsesShortestLegalPath 核验已复现的旧绕行和相邻阻挡。
func TestPersonalGroundNavigationUsesShortestLegalPath(t *testing.T) {
	start, target := devconfig.Point{X: 6, Y: 9}, devconfig.Point{X: 6, Y: 5}
	blocked := []devconfig.Point{{X: 6, Y: 6}, {X: 7, Y: 5}}
	w := newPersonalNavigationTestWorker(13, blocked)
	m := &w.e.Store.Cfg.Fixture.Map
	r := newPersonalNavigationTestRoom(start)
	r.Destination, r.Navigation.Status = &target, navigationActive
	for i := 1; i <= 5; i++ {
		from := r.Player.Pos
		step, found := w.personalNavigation.findFirstStep(m, r, &r.Player, target)
		if !found || distance(from, step) != 1 || !m.Walkable(step) || step == target && i < 5 {
			t.Fatalf("第%d步没有采用合法的五步路线：%+v→%+v", i, from, step)
		}
		if from.X != step.X && from.Y != step.Y &&
			(!m.Walkable(devconfig.Point{X: from.X, Y: step.Y}) || !m.Walkable(devconfig.Point{X: step.X, Y: from.Y})) {
			t.Fatalf("第%d步穿过静态阻挡夹角：%+v→%+v", i, from, step)
		}
		r.Player.Pos = step
	}
	if r.Player.Pos != target {
		t.Fatalf("五步后未到达目标，停在%+v", r.Player.Pos)
	}

	// 真实目标本身被活怪占用时，原靠近行为仍按旧优先级运行。
	r = newPersonalNavigationTestRoom(start)
	r.Destination, r.Navigation.Status = &target, navigationActive
	r.Monsters, r.MonsterCount = []*entity{{HP: 1, Pos: target}}, 1
	want, ok := referencePersonalNavigation(m, r, &r.Player, target)
	got, found := w.personalNavigation.findFirstStep(m, r, &r.Player, target)
	if got != want || found != ok {
		t.Fatal("显式点地改变了暂时占位目标的原接近规则")
	}

	// 玩家已经相邻时没有可提交的首步，将当前操作结束为阻挡并保留原目标。
	r.Player.Pos = devconfig.Point{X: 5, Y: 5}
	r.Navigation.Destination = &target
	w.move(r, &r.Player, target, time.Unix(1, 0), 100)
	if r.Player.Pos != (devconfig.Point{X: 5, Y: 5}) || r.Destination != nil ||
		r.Navigation.Status != navigationBlocked || r.Navigation.Destination == nil || *r.Navigation.Destination != target {
		t.Fatalf("相邻活怪阻挡时未保留原点击目标或未按原规则结束：%+v", r.Navigation)
	}
}
