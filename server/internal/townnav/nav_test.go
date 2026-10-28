package townnav

import (
	"testing"
)

func testGrid(t *testing.T, width, height int, walls ...Point) *Grid {
	t.Helper()
	bits := make([]byte, (width*height+7)/8)
	for _, p := range walls {
		i := p.Y*width + p.X
		bits[i/8] |= 1 << uint(i%8)
	}
	g, err := NewGrid(2, "test-v1", width, height, bits)
	if err != nil {
		t.Fatal(err)
	}
	return g
}
func testLimits(cells int) Limits {
	return Limits{MaxExpanded: cells, MaxFrontier: cells, MaxRouteCells: cells, MaxReplans: 3, MaxDynamicCells: cells}
}
func testKey() Key {
	return Key{Scene: 20, MapID: 2, MapVersion: "test-v1", RequestRevision: 1, Life: 0}
}
func testTask(t *testing.T, g *Grid, start, goal Point, limits Limits, d Dynamic) (*Task, *Workspace) {
	t.Helper()
	w, err := NewWorkspace(g.width*g.height, limits.MaxFrontier)
	if err != nil {
		t.Fatal(err)
	}
	task, err := NewTask(g, w, make([]Point, limits.MaxRouteCells), testKey(), start, goal, d, limits)
	if err != nil {
		t.Fatal(err)
	}
	return task, w
}
func finish(t *testing.T, task *Task, budget int) {
	t.Helper()
	for i := 0; task.State() == Searching || task.State() == Building; i++ {
		if i > 1000000 {
			t.Fatal("有界推进未完成")
		}
		p, err := task.Advance(testKey(), budget)
		if err != nil {
			t.Fatal(err)
		}
		if p.WorkUsed > budget || p.Expanded > p.WorkUsed {
			t.Fatalf("推进越过显式预算：%+v", p)
		}
	}
}

// 独立的坐标邻接图BFS，不调用候选的Walkable/StepAllowed/heap或启发式。
func oracleDistance(width, height, mask int, start, goal Point) int {
	return oracleRadiusDistance(width, height, mask, start, goal, 0, false)
}

func oracleRadiusDistance(width, height, mask int, start, goal Point, radius int, occupiedGoal bool) int {
	valid := func(x, y int) bool {
		return x >= 0 && x < width && y >= 0 && y < height && mask&(1<<uint(y*width+x)) == 0
	}
	distance := make([]int, width*height)
	for i := range distance {
		distance[i] = -1
	}
	queue := [][2]int{{start.X, start.Y}}
	distance[start.Y*width+start.X] = 0
	for head := 0; head < len(queue); head++ {
		x, y := queue[head][0], queue[head][1]
		if max(abs(x-goal.X), abs(y-goal.Y)) <= radius {
			return distance[y*width+x]
		}
		for dy := -1; dy <= 1; dy++ {
			for dx := -1; dx <= 1; dx++ {
				if dx == 0 && dy == 0 || !valid(x+dx, y+dy) || dx != 0 && dy != 0 && (!valid(x+dx, y) || !valid(x, y+dy)) {
					continue
				}
				if occupiedGoal && x+dx == goal.X && y+dy == goal.Y {
					continue
				}
				index := (y+dy)*width + x + dx
				if distance[index] >= 0 {
					continue
				}
				distance[index] = distance[y*width+x] + 1
				queue = append(queue, [2]int{x + dx, y + dy})
			}
		}
	}
	return -1
}

func TestAllThreeByThreeAdjacentGoalsPreserveCornersAndOccupiedCell(t *testing.T) {
	for mask := 0; mask < 512; mask++ {
		walls := []Point{}
		for i := 0; i < 9; i++ {
			if mask&(1<<uint(i)) != 0 {
				walls = append(walls, Point{i % 3, i / 3})
			}
		}
		g := testGrid(t, 3, 3, walls...)
		workspace, _ := NewWorkspace(9, 9)
		for a := 0; a < 9; a++ {
			for b := 0; b < 9; b++ {
				if a == b || mask&(1<<uint(a)) != 0 || mask&(1<<uint(b)) != 0 {
					continue
				}
				start, goal := Point{a % 3, a / 3}, Point{b % 3, b / 3}
				d := Dynamic{Revision: 1, Blocked: []Point{goal}}
				task, err := NewAdjacentTask(g, workspace, make([]Point, 9), testKey(), start, goal, d, testLimits(9))
				if err != nil {
					t.Fatal(err)
				}
				finish(t, task, 1)
				want := oracleRadiusDistance(3, 3, mask, start, goal, 1, true)
				if want < 0 {
					if task.State() != NoPath {
						t.Fatalf("mask%d %v→邻%v应无路: %s", mask, start, goal, task.State())
					}
					continue
				}
				position, steps := start, 0
				for task.State() == Ready {
					next, ok, err := task.Next(testKey(), position, d)
					if err != nil || !ok || next == goal {
						t.Fatal("邻接路线进入动态主人占格", err)
					}
					if err = task.CommitStep(testKey(), position, next); err != nil {
						t.Fatal(err)
					}
					position = next
					steps++
				}
				if task.State() != Arrived || distance(position, goal) > 1 || steps != want {
					t.Fatalf("mask%d %v→邻%v步数%d/%d 状态%s", mask, start, goal, steps, want, task.State())
				}
			}
		}
	}
}

func TestSuspensionReturnsBuffersAndReplanRetainsCumulativeBudget(t *testing.T) {
	g := testGrid(t, 20, 1)
	limits := testLimits(20)
	limits.MaxExpanded = 40
	task, workspace := testTask(t, g, Point{}, Point{19, 0}, limits, Dynamic{Revision: 1})
	_, _ = task.Advance(testKey(), 3)
	if !task.Suspend(testKey()) || workspace.InUse() || task.Progress().TotalExpanded != 3 {
		t.Fatal("挂起未归还工作区或清了累计预算")
	}
	storage := task.DetachRouteStorage()
	if len(storage) != 20 || len(task.route) != 0 || cap(task.route) != 0 {
		t.Fatal("等待状态仍引用独占路线缓冲")
	}
	other, err := NewTask(g, workspace, storage, Key{Scene: 20, MapID: 2, MapVersion: "test-v1", RequestRevision: 2}, Point{}, Point{2, 0}, Dynamic{Revision: 1}, testLimits(20))
	if err != nil {
		t.Fatal(err)
	}
	for other.State() == Searching || other.State() == Building {
		_, err = other.Advance(Key{Scene: 20, MapID: 2, MapVersion: "test-v1", RequestRevision: 2}, 20)
		if err != nil {
			t.Fatal(err)
		}
	}
	other.Cancel()
	if err = task.AttachRouteStorage(storage); err != nil {
		t.Fatal(err)
	}
	if err = task.Replan(workspace, testKey(), Point{}, Dynamic{Revision: 1}); err != nil {
		t.Fatal(err)
	}
	finish(t, task, 1)
	if task.Progress().TotalExpanded != 23 || task.Progress().Replans != 0 || task.State() != Ready {
		t.Fatal("恢复挂起另起了没有累计预算的新搜索")
	}
	if task.DetachRouteStorage() != nil {
		t.Fatal("Ready路线允许缓冲被另一请求覆盖")
	}
}

func TestRepeatedUnchangedSuspensionDoesNotSpendReplansButRetainsExpansionLimit(t *testing.T) {
	g := testGrid(t, 20, 1)
	limits := testLimits(20)
	limits.MaxExpanded, limits.MaxReplans = 8, 0
	dynamic := Dynamic{Revision: 1}
	task, workspace := testTask(t, g, Point{}, Point{19, 0}, limits, dynamic)
	_, _ = task.Advance(testKey(), 1)
	for i := 0; i < 4; i++ {
		if !task.Suspend(testKey()) || workspace.InUse() {
			t.Fatal("暂停未归还工作区")
		}
		storage := task.DetachRouteStorage()
		if err := task.AttachRouteStorage(storage); err != nil {
			t.Fatal(err)
		}
		if err := task.Replan(workspace, testKey(), Point{}, dynamic); err != nil {
			t.Fatal(err)
		}
		progress, err := task.Advance(testKey(), 1)
		if err != nil || progress.WorkUsed != 1 || progress.TotalExpanded != i+2 || progress.Replans != 0 {
			t.Fatalf("纯暂停消耗重规划次数、重置累计额度或绕过每次工作额度: %+v %v", progress, err)
		}
	}
	finish(t, task, 1)
	if task.State() != Limited || task.Progress().TotalExpanded != 8 || workspace.InUse() {
		t.Fatal("纯暂停恢复绕过原累计展开上限", task.Progress())
	}
}

func TestSuspendedResumeAfterActualChangeStillSpendsReplans(t *testing.T) {
	for _, change := range []string{"position", "dynamic_revision", "dynamic_contents"} {
		t.Run(change, func(t *testing.T) {
			g := testGrid(t, 10, 3)
			limits := testLimits(30)
			limits.MaxExpanded, limits.MaxReplans = 50, 1
			dynamic := Dynamic{Revision: 1}
			position := Point{0, 1}
			task, workspace := testTask(t, g, position, Point{9, 1}, limits, dynamic)
			_, _ = task.Advance(testKey(), 2)
			if !task.Suspend(testKey()) {
				t.Fatal("未暂停搜索")
			}
			switch change {
			case "position":
				position = Point{0, 2}
			case "dynamic_revision":
				dynamic.Revision++
			case "dynamic_contents":
				dynamic.Blocked = []Point{{5, 2}}
			}
			if err := task.Replan(workspace, testKey(), position, dynamic); err != nil {
				t.Fatal(err)
			}
			if task.Progress().Replans != 1 || task.Progress().TotalExpanded != 2 {
				t.Fatal("真实变化借暂停豁免次数或重置了累计展开", task.Progress())
			}
			_, _ = task.Advance(testKey(), 1)
			if !task.Suspend(testKey()) {
				t.Fatal("未暂停第二次搜索")
			}
			dynamic.Revision++
			if err := task.Replan(workspace, testKey(), position, dynamic); err != nil {
				t.Fatal(err)
			}
			if task.State() != Limited || task.Progress().Replans != 1 || task.Progress().TotalExpanded != 3 || workspace.InUse() {
				t.Fatal("第二次真实变化越过MaxReplans", task.Progress())
			}
		})
	}
}

func TestSuspendedReadyRouteResumesFromLastCommittedStepAndRejectsOldScene(t *testing.T) {
	g := testGrid(t, 6, 1)
	limits := testLimits(6)
	limits.MaxExpanded, limits.MaxReplans = 20, 0
	dynamic := Dynamic{Revision: 1}
	task, workspace := testTask(t, g, Point{}, Point{5, 0}, limits, dynamic)
	finish(t, task, 1)
	next, ok, err := task.Next(testKey(), Point{}, dynamic)
	if err != nil || !ok || task.CommitStep(testKey(), Point{}, next) != nil || !task.Suspend(testKey()) {
		t.Fatal("未建立已行走一步后的暂停路线", err)
	}
	if err = task.Replan(workspace, testKey(), next, dynamic); err != nil {
		t.Fatal(err)
	}
	finish(t, task, 1)
	if task.State() != Ready || task.Progress().Replans != 0 || task.Progress().TotalExpanded != 11 {
		t.Fatal("正常行走后的纯暂停误当外部位移", task.Progress())
	}
	if !task.Suspend(testKey()) {
		t.Fatal("未暂停恢复后的路线")
	}
	oldScene := testKey()
	oldScene.Scene++
	if task.Replan(workspace, oldScene, next, dynamic) == nil || task.State() != Cancelled || workspace.InUse() {
		t.Fatal("暂停恢复允许旧scene写入新场景")
	}
}

func TestAllThreeByThreeObstacleGraphsMatchIndependentShortestPath(t *testing.T) {
	for mask := 0; mask < 512; mask++ {
		walls := []Point{}
		for i := 0; i < 9; i++ {
			if mask&(1<<uint(i)) != 0 {
				walls = append(walls, Point{i % 3, i / 3})
			}
		}
		g := testGrid(t, 3, 3, walls...)
		workspace, _ := NewWorkspace(9, 9)
		for a := 0; a < 9; a++ {
			if mask&(1<<uint(a)) != 0 {
				continue
			}
			for b := 0; b < 9; b++ {
				if mask&(1<<uint(b)) != 0 {
					continue
				}
				start, goal := Point{a % 3, a / 3}, Point{b % 3, b / 3}
				task, err := NewTask(g, workspace, make([]Point, 9), testKey(), start, goal, Dynamic{Revision: 1}, testLimits(9))
				if err != nil {
					t.Fatal(err)
				}
				finish(t, task, 1)
				want := oracleDistance(3, 3, mask, start, goal)
				if want < 0 {
					if task.State() != NoPath {
						t.Fatalf("mask%d %v→%v应不可达：%s", mask, start, goal, task.State())
					}
					continue
				}
				actual, position := 0, start
				for task.State() == Ready {
					next, ok, err := task.Next(testKey(), position, Dynamic{Revision: 1})
					if err != nil || !ok {
						t.Fatal("合法路径缺下一步", err)
					}
					if err = task.CommitStep(testKey(), position, next); err != nil {
						t.Fatal(err)
					}
					position = next
					actual++
				}
				if task.State() != Arrived || position != goal || actual != want {
					t.Fatalf("mask%d %v→%v路径%d，独立最短%d，状态%s", mask, start, goal, actual, want, task.State())
				}
				if workspace.InUse() {
					t.Fatal("路径完成后工作区没有归还")
				}
			}
		}
	}
}

func TestExpansionAndRouteReconstructionBothHonorOneUnitBudget(t *testing.T) {
	g := testGrid(t, 400, 1)
	task, w := testTask(t, g, Point{}, Point{399, 0}, testLimits(400), Dynamic{Revision: 1})
	expanded, work, buildingRounds := 0, 0, 0
	for task.State() == Searching || task.State() == Building {
		p, err := task.Advance(testKey(), 1)
		if err != nil {
			t.Fatal(err)
		}
		if p.WorkUsed > 1 {
			t.Fatal("单轮回溯或展开超过预算")
		}
		work += p.WorkUsed
		expanded += p.Expanded
		if p.State == Building {
			buildingRounds++
		}
	}
	if task.State() != Ready || expanded != 400 || work != 799 || buildingRounds < 399 || w.InUse() {
		t.Fatalf("展开/回溯分摊不正确：%+v expanded%d work%d building%d", task.Progress(), expanded, work, buildingRounds)
	}
	// 搜索工作区已经能用于另一个请求，旧路线仍独立保留且无需重搜。
	second, err := NewTask(g, w, make([]Point, 400), testKey(), Point{10, 0}, Point{20, 0}, Dynamic{Revision: 1}, testLimits(400))
	if err != nil {
		t.Fatal(err)
	}
	if next, ok, err := task.Next(testKey(), Point{}, Dynamic{Revision: 1}); err != nil || !ok || next != (Point{1, 0}) {
		t.Fatal("复用工作区破坏旧路线", next, err)
	}
	task.Cancel()
	if !w.InUse() {
		t.Fatal("旧路线取消错误释放另一个任务的工作区")
	}
	second.Cancel()
}

func TestSceneMapVersionRequestAndLifeChangesCancelWithoutOldWork(t *testing.T) {
	for name, change := range map[string]func(*Key){"scene": func(k *Key) { k.Scene++ }, "map": func(k *Key) { k.MapID++ }, "version": func(k *Key) { k.MapVersion = "new" }, "request": func(k *Key) { k.RequestRevision++ }, "life": func(k *Key) { k.Life++ }} {
		t.Run(name, func(t *testing.T) {
			g := testGrid(t, 20, 20)
			task, w := testTask(t, g, Point{}, Point{19, 19}, testLimits(400), Dynamic{Revision: 1})
			_, _ = task.Advance(testKey(), 1)
			key := testKey()
			change(&key)
			p, err := task.Advance(key, 1)
			if err != nil || p.State != Cancelled || p.WorkUsed != 0 || w.InUse() {
				t.Fatal("过期请求继续搜索或保留工作区", p, err)
			}
		})
	}
}

func TestStaticUnreachableIsTerminalAndNeverRestartsOnTick(t *testing.T) {
	g := testGrid(t, 3, 3, Point{1, 0}, Point{0, 1})
	task, w := testTask(t, g, Point{}, Point{2, 2}, testLimits(9), Dynamic{Revision: 1})
	for i := 0; i < 100; i++ {
		p, err := task.Advance(testKey(), 1)
		if err != nil || p.State != NoPath || p.WorkUsed != 0 || p.TotalExpanded != 0 || w.InUse() {
			t.Fatal("静态不可达每tick仍在重算", p, err)
		}
	}
	if task.Replan(w, testKey(), Point{}, Dynamic{Revision: 2}) == nil {
		t.Fatal("静态不可达被动态变化自动重启")
	}
}

func TestDynamicBlockageWaitsForChangeAndReplansWithoutLosingTarget(t *testing.T) {
	g := testGrid(t, 5, 1)
	task, w := testTask(t, g, Point{}, Point{4, 0}, testLimits(5), Dynamic{Revision: 1, Blocked: []Point{{2, 0}}})
	finish(t, task, 1)
	if task.State() != WaitingDynamic || w.InUse() {
		t.Fatal("动态堵路未终止当前搜索")
	}
	before := task.Progress().TotalExpanded
	for i := 0; i < 10; i++ {
		if task.Replan(w, testKey(), Point{}, Dynamic{Revision: 1, Blocked: []Point{{2, 0}}}) == nil {
			t.Fatal("未变化的占格触发重复搜索")
		}
	}
	if task.Progress().TotalExpanded != before || w.InUse() {
		t.Fatal("等待状态仍占用搜索预算")
	}
	// 累计预算由本请求所有尝试共享，不能重置成新5次。
	if err := task.Replan(w, testKey(), Point{}, Dynamic{Revision: 2}); err != nil {
		t.Fatal(err)
	}
	finish(t, task, 1)
	if task.State() != Limited || task.Progress().TotalExpanded != 5 {
		t.Fatal("重规划重置了累计展开预算", task.Progress())
	}
}

func TestReadyRouteChecksOccupancyAndOnlyCommitsSuccessfulStep(t *testing.T) {
	g := testGrid(t, 5, 3)
	limits := testLimits(15)
	limits.MaxExpanded = 60
	task, w := testTask(t, g, Point{0, 1}, Point{4, 1}, limits, Dynamic{Revision: 1})
	finish(t, task, 2)
	first, ok, err := task.Next(testKey(), Point{0, 1}, Dynamic{Revision: 1})
	if err != nil || !ok {
		t.Fatal(err)
	}
	if task.CommitStep(testKey(), Point{0, 1}, Point{0, 2}) == nil {
		t.Fatal("未成功的其它格被当成路径推进")
	}
	if again, _, _ := task.Next(testKey(), Point{0, 1}, Dynamic{Revision: 1}); again != first {
		t.Fatal("失败移动消耗了路径")
	}
	if _, ok, err = task.Next(testKey(), Point{0, 1}, Dynamic{Revision: 2, Blocked: []Point{first}}); err != nil || ok || task.State() != NeedsReplan {
		t.Fatal("动态占位未阻止下一步", err)
	}
	if err = task.Replan(w, testKey(), Point{0, 1}, Dynamic{Revision: 2, Blocked: []Point{first}}); err != nil {
		t.Fatal(err)
	}
	finish(t, task, 2)
	position := Point{0, 1}
	for task.State() == Ready {
		next, ok, e := task.Next(testKey(), position, Dynamic{Revision: 2, Blocked: []Point{first}})
		if e != nil || !ok || next == first {
			t.Fatal("重新规划仍穿占位格", e)
		}
		if e = task.CommitStep(testKey(), position, next); e != nil {
			t.Fatal(e)
		}
		position = next
	}
	if task.State() != Arrived || position != (Point{4, 1}) || task.Progress().Replans != 1 {
		t.Fatal("重规划丢失原目标", task.Progress())
	}
}

func TestReplansFrontierAndRouteCapsHaveExplicitLimitOutcome(t *testing.T) {
	t.Run("frontier", func(t *testing.T) {
		g := testGrid(t, 3, 3)
		l := testLimits(9)
		l.MaxFrontier = 1
		task, w := testTask(t, g, Point{}, Point{2, 2}, l, Dynamic{Revision: 1})
		finish(t, task, 1)
		if task.State() != Limited || w.InUse() {
			t.Fatal("frontier容量被放大或伪报不可达")
		}
	})
	t.Run("route", func(t *testing.T) {
		g := testGrid(t, 5, 1)
		l := testLimits(5)
		l.MaxRouteCells = 2
		task, w := testTask(t, g, Point{}, Point{4, 0}, l, Dynamic{Revision: 1})
		finish(t, task, 1)
		if task.State() != Limited || w.InUse() || task.Progress().RouteRemaining != 0 {
			t.Fatal("路径容量被放大或暴露部分完成路线")
		}
	})
	t.Run("replans", func(t *testing.T) {
		g := testGrid(t, 5, 1)
		l := testLimits(5)
		l.MaxReplans = 0
		task, w := testTask(t, g, Point{}, Point{4, 0}, l, Dynamic{Revision: 1, Blocked: []Point{{4, 0}}})
		if task.State() != WaitingDynamic {
			t.Fatal(task.State())
		}
		if err := task.Replan(w, testKey(), Point{}, Dynamic{Revision: 2}); err != nil {
			t.Fatal(err)
		}
		if task.State() != Limited || w.InUse() {
			t.Fatal("零次重规划限制被忽略")
		}
	})
}

func TestUnexpectedPoseRequiresReplanAndBusyWorkspaceDoesNotConsumeAttempt(t *testing.T) {
	g := testGrid(t, 6, 2)
	l := testLimits(12)
	l.MaxExpanded = 30
	task, w := testTask(t, g, Point{}, Point{5, 0}, l, Dynamic{Revision: 1})
	finish(t, task, 2)
	_, ok, err := task.Next(testKey(), Point{0, 1}, Dynamic{Revision: 1})
	if err != nil || ok || task.State() != NeedsReplan {
		t.Fatal("推移后仍从旧起点取路径", err)
	}
	other, e := NewTask(g, w, make([]Point, 12), testKey(), Point{}, Point{5, 1}, Dynamic{Revision: 1}, l)
	if e != nil {
		t.Fatal(e)
	}
	if task.Replan(w, testKey(), Point{0, 1}, Dynamic{Revision: 1}) == nil || task.Progress().Replans != 0 || !w.InUse() {
		t.Fatal("抢占其它任务工作区或失败抢占消耗重规划次数")
	}
	other.Cancel()
	if err = task.Replan(w, testKey(), Point{0, 1}, Dynamic{Revision: 1}); err != nil {
		t.Fatal(err)
	}
	finish(t, task, 2)
	if task.State() != Ready {
		t.Fatal(task.Progress())
	}
}

func TestBudgetsAndTerrainDoNotAcquireHiddenDefaults(t *testing.T) {
	if _, err := NewGrid(2, "v", 2, 2, nil); err == nil {
		t.Fatal("缺碰撞位图被默认成全可走")
	}
	if _, err := NewGrid(2, "v", 1, 1, []byte{2}); err == nil {
		t.Fatal("尾部补位未拒绝")
	}
	if _, err := NewWorkspace(0, 1); err == nil {
		t.Fatal("无效工作区容量未拒绝")
	}
	g := testGrid(t, 3, 3)
	w, _ := NewWorkspace(9, 9)
	for _, l := range []Limits{{}, {MaxExpanded: 9, MaxFrontier: 0, MaxRouteCells: 9}, {MaxExpanded: 9, MaxFrontier: 9, MaxRouteCells: 0}} {
		if _, err := NewTask(g, w, make([]Point, 9), testKey(), Point{}, Point{2, 2}, Dynamic{Revision: 1}, l); err == nil {
			t.Fatal("缺预算未拒绝")
		}
	}
	task, err := NewTask(g, w, make([]Point, 9), testKey(), Point{}, Point{2, 2}, Dynamic{Revision: 1}, testLimits(9))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = task.Advance(testKey(), 0); err == nil || task.Progress().TotalExpanded != 0 {
		t.Fatal("零本轮预算仍推进")
	}
	task.Cancel()
	bits := []byte{0}
	copied, err := NewGrid(2, "v", 2, 2, bits)
	if err != nil {
		t.Fatal(err)
	}
	bits[0] = 15
	if !copied.Walkable(Point{}) {
		t.Fatal("可变来源切片污染了共享静态地形")
	}
}

func TestStaticCornerRulesAndDynamicPetSideCellsMatchExistingMovement(t *testing.T) {
	for _, scenario := range []struct {
		name           string
		walls, dynamic []Point
		steps          int
	}{
		{"open_diagonal", nil, nil, 1},
		{"one_static_side_requires_detour", []Point{{1, 0}}, nil, 2},
		{"two_static_sides_forbid_corner_cut", []Point{{1, 0}, {0, 1}}, nil, -1},
		// 现有neighbor只检查对角侧格的静态地形，动态实体仅检查目标格。
		{"pet_on_side_does_not_become_static_wall", nil, []Point{{1, 0}, {0, 1}}, 1},
		{"pet_overlapping_start_can_leave", nil, []Point{{0, 0}}, 1},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			g := testGrid(t, 2, 2, scenario.walls...)
			task, _ := testTask(t, g, Point{}, Point{1, 1}, testLimits(4), Dynamic{Revision: 1, Blocked: scenario.dynamic})
			finish(t, task, 1)
			if scenario.steps < 0 {
				if task.State() != NoPath {
					t.Fatal("静态封角应在连通分量预检拒绝")
				}
				return
			}
			if task.State() != Ready || task.Progress().RouteRemaining != scenario.steps {
				t.Fatal("静态/动态侧格语义改变", task.Progress())
			}
		})
	}
}

func TestPetOccupyingDestinationWaitsWithoutFalseArrival(t *testing.T) {
	g := testGrid(t, 2, 2)
	task, w := testTask(t, g, Point{}, Point{1, 1}, testLimits(4), Dynamic{Revision: 1, Blocked: []Point{{1, 1}}})
	if task.State() != WaitingDynamic || w.InUse() {
		t.Fatal("宠物占目标格仍开始搜索或假到达")
	}
	for i := 0; i < 20; i++ {
		p, err := task.Advance(testKey(), 1)
		if err != nil || p.State != WaitingDynamic || p.WorkUsed != 0 {
			t.Fatal("占位未变化仍重复搜索")
		}
	}
	if err := task.Replan(w, testKey(), Point{}, Dynamic{Revision: 2}); err != nil {
		t.Fatal(err)
	}
	finish(t, task, 1)
	if task.State() != Ready || task.Progress().RouteRemaining != 1 {
		t.Fatal("宠物离开后未按原目标继续规划")
	}
}
