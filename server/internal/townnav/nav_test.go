// 本文件验证城镇连通性、堆优先搜索、动态占位、分片预算和路径存储生命周期。

package townnav

import (
	"testing"
)

// testGrid 验证网格。
// t 提供断言失败报告、子用例和清理注册，width 记录宽度，使用数值表示，height 记录高度，使用数值表示，walls 记录“walls”，使用可变数量的点位表示。
func testGrid(t *testing.T, width, height int, walls ...Point) *Grid {
	t.Helper()
	// bits 创建用于维护二进制位的容器或通道。
	bits := make([]byte, (width*height+7)/8)
	// p 接收遍历“walls”时当前的元素。
	for _, p := range walls {
		// i 保存根据纵坐标与宽度的计算结果与横坐标计算的当前位置的下标。
		i := p.Y*width + p.X
		bits[i/8] |= 1 << uint(i%8)
	}
	// g 保存创建网格返回的结果，err 接收创建网格过程中可能产生的错误，供后续失败分支处理。
	g, err := NewGrid(2, "test-v1", width, height, bits)
	if err != nil {
		t.Fatal(err)
	}
	return g
}

// testLimits 验证上限集合。
// cells 记录地图格集合，使用数值表示。
func testLimits(cells int) Limits {
	return Limits{MaxExpanded: cells, MaxFrontier: cells, MaxRouteCells: cells, MaxReplans: 3, MaxDynamicCells: cells}
}

// testKey 验证键。
func testKey() Key {
	return Key{Scene: 20, MapID: 2, MapVersion: "test-v1", RequestRevision: 1, Life: 0}
}

// testTask 验证任务。
// t 提供断言失败报告、子用例和清理注册，g 记录g，使用网格表示，start 记录起点或开始时间，使用点位表示，goal 记录目标格，使用点位表示，limits 记录上限集合。
// d 记录动态占位。
func testTask(t *testing.T, g *Grid, start, goal Point, limits Limits, d Dynamic) (*Task, *Workspace) {
	t.Helper()
	// w 接收创建搜索工作区供后续步骤使用，err 接收创建搜索工作区过程中可能产生的错误，供后续失败分支处理。
	w, err := NewWorkspace(g.width*g.height, limits.MaxFrontier)
	if err != nil {
		t.Fatal(err)
	}
	// task 保存由创建任务取得的任务。
	task, err := NewTask(g, w, make([]Point, limits.MaxRouteCells), testKey(), start, goal, d, limits)
	if err != nil {
		t.Fatal(err)
	}
	return task, w
}

// finish 完成当前请求或状态，并把结果交给调用方。
// t 提供断言失败报告、子用例和清理注册，task 记录任务，budget 记录预算，使用数值表示。
func finish(t *testing.T, task *Task, budget int) {
	t.Helper()
	// i 标记循环或列表中的当前位置。
	for i := 0; task.State() == Searching || task.State() == Building; i++ {
		if i > 1000000 {
			t.Fatal("有界推进未完成")
		}
		// p 接收推进供后续步骤使用，err 接收推进过程中可能产生的错误，供后续失败分支处理。
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
// width 记录宽度，使用数值表示，height 记录高度，使用数值表示，mask 记录掩码，使用数值表示，start 记录起点或开始时间，使用点位表示。
// goal 记录目标格，使用点位表示。
func oracleDistance(width, height, mask int, start, goal Point) int {
	return oracleRadiusDistance(width, height, mask, start, goal, 0, false)
}

// oracleRadiusDistance 返回“oracle”“radius”距离供调用方使用。
// width 记录宽度，使用数值表示，height 记录高度，使用数值表示，mask 记录掩码，使用数值表示，start 记录起点或开始时间，使用点位表示。
// goal 记录目标格，使用点位表示，radius 记录“radius”，使用数值表示，occupiedGoal 标记是否处于已占用目标格状态。
func oracleRadiusDistance(width, height, mask int, start, goal Point, radius int, occupiedGoal bool) int {
	// valid 封装有效对应的后续处理，x 记录数值，y 记录数值。
	valid := func(x, y int) bool {
		return x >= 0 && x < width && y >= 0 && y < height && mask&(1<<uint(y*width+x)) == 0
	}
	// distance 创建用于维护距离的容器或通道。
	distance := make([]int, width*height)
	// i 标记遍历距离时访问的下标。
	for i := range distance {
		distance[i] = -1
	}
	// queue 初始化用作队列的数值列表列表。
	queue := [][2]int{{start.X, start.Y}}
	distance[start.Y*width+start.X] = 0
	// head 规定头部的初始取值。
	for head := 0; head < len(queue); head++ {
		// x 保存从队列中的指定项取出的队列中的指定项中的指定项，y 保存从队列中的指定项取出的队列中的指定项中的指定项。
		x, y := queue[head][0], queue[head][1]
		if max(abs(x-goal.X), abs(y-goal.Y)) <= radius {
			return distance[y*width+x]
		}
		// dy 保存当前处理所用的纵向差值。
		for dy := -1; dy <= 1; dy++ {
			// dx 保存当前处理所用的横向差值。
			for dx := -1; dx <= 1; dx++ {
				if dx == 0 && dy == 0 || !valid(x+dx, y+dy) || dx != 0 && dy != 0 && (!valid(x+dx, y) || !valid(x, y+dy)) {
					continue
				}
				if occupiedGoal && x+dx == goal.X && y+dy == goal.Y {
					continue
				}
				// index 保存根据纵坐标与纵向差值的计算结果与宽度的计算结果与横坐标的计算结果与横向差值计算的下标。
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

// TestAllThreeByThreeAdjacentGoalsPreserveCornersAndOccupiedCell 遍历 3×3 网格的障碍布局，检查邻接导航与独立最短路一致，拐角约束和目标占格全程有效。
// t 提供断言失败报告、子用例和清理注册。
func TestAllThreeByThreeAdjacentGoalsPreserveCornersAndOccupiedCell(t *testing.T) {
	// mask 规定掩码的初始取值。
	for mask := 0; mask < 512; mask++ {
		// walls 建立带初始值的点位列表，供本段继续处理。
		walls := []Point{}
		// i 标记循环或列表中的当前位置。
		for i := 0; i < 9; i++ {
			if mask&(1<<uint(i)) != 0 {
				walls = append(walls, Point{i % 3, i / 3})
			}
		}
		// g 保存由测试网格取得的g。
		g := testGrid(t, 3, 3, walls...)
		// workspace 保存由创建搜索工作区取得的搜索工作区。
		workspace, _ := NewWorkspace(9, 9)
		// a 规定a的初始取值。
		for a := 0; a < 9; a++ {
			// b 规定b的初始取值。
			for b := 0; b < 9; b++ {
				if a == b || mask&(1<<uint(a)) != 0 || mask&(1<<uint(b)) != 0 {
					continue
				}
				// start 初始化用作起点或开始时间的点位，goal 建立带初始值的点位，供本段继续处理。
				start, goal := Point{a % 3, a / 3}, Point{b % 3, b / 3}
				// d 建立带初始值的“dynamic”，供本段继续处理。
				d := Dynamic{Revision: 1, Blocked: []Point{goal}}
				// task 保存由创建相邻任务取得的任务，err 接收创建相邻任务过程中可能产生的错误，供后续失败分支处理。
				task, err := NewAdjacentTask(g, workspace, make([]Point, 9), testKey(), start, goal, d, testLimits(9))
				if err != nil {
					t.Fatal(err)
				}
				finish(t, task, 1)
				// want 保存由“oracle”“radius”距离取得的期望值。
				want := oracleRadiusDistance(3, 3, mask, start, goal, 1, true)
				if want < 0 {
					if task.State() != NoPath {
						t.Fatalf("mask%d %v→邻%v应无路: %s", mask, start, goal, task.State())
					}
					continue
				}
				// position 保存当前处理所用的位置，steps 规定步骤集合的初始取值。
				position, steps := start, 0
				for task.State() == Ready {
					// next 接收下一状态供后续步骤使用，ok 表明查询、校验或类型转换是否成功，err 接收下一状态过程中可能产生的错误，供后续失败分支处理。
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

// TestSuspensionReturnsBuffersAndReplanRetainsCumulativeBudget 验证暂停搜索会归还临时缓冲，用新阻挡恢复时仍累计已消耗的扩展量和重规划预算。
// t 提供断言失败报告、子用例和清理注册。
func TestSuspensionReturnsBuffersAndReplanRetainsCumulativeBudget(t *testing.T) {
	// g 保存由测试网格取得的g。
	g := testGrid(t, 20, 1)
	// limits 保存由测试上限集合取得的上限集合。
	limits := testLimits(20)
	limits.MaxExpanded = 40
	// task 保存由测试任务取得的任务，workspace 保存由测试任务取得的搜索工作区。
	task, workspace := testTask(t, g, Point{}, Point{19, 0}, limits, Dynamic{Revision: 1})
	_, _ = task.Advance(testKey(), 3)
	if !task.Suspend(testKey()) || workspace.InUse() || task.Progress().TotalExpanded != 3 {
		t.Fatal("挂起未归还工作区或清了累计预算")
	}
	// storage 保存由解除附着路由存储缓冲取得的存储缓冲。
	storage := task.DetachRouteStorage()
	if len(storage) != 20 || len(task.route) != 0 || cap(task.route) != 0 {
		t.Fatal("等待状态仍引用独占路线缓冲")
	}
	// other 保存由创建任务取得的另一项，err 接收创建任务过程中可能产生的错误，供后续失败分支处理。
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

// TestRepeatedUnchangedSuspensionDoesNotSpendReplansButRetainsExpansionLimit 验证重复暂停而条件未变不会消耗重规划次数，恢复后仍沿用原扩展上限。
// t 提供断言失败报告、子用例和清理注册。
func TestRepeatedUnchangedSuspensionDoesNotSpendReplansButRetainsExpansionLimit(t *testing.T) {
	// g 保存由测试网格取得的g。
	g := testGrid(t, 20, 1)
	// limits 保存由测试上限集合取得的上限集合。
	limits := testLimits(20)
	limits.MaxExpanded, limits.MaxReplans = 8, 0
	// dynamic 建立带初始值的“dynamic”，供本段继续处理。
	dynamic := Dynamic{Revision: 1}
	// task 保存由测试任务取得的任务，workspace 保存由测试任务取得的搜索工作区。
	task, workspace := testTask(t, g, Point{}, Point{19, 0}, limits, dynamic)
	_, _ = task.Advance(testKey(), 1)
	// i 标记循环或列表中的当前位置。
	for i := 0; i < 4; i++ {
		if !task.Suspend(testKey()) || workspace.InUse() {
			t.Fatal("暂停未归还工作区")
		}
		// storage 保存由解除附着路由存储缓冲取得的存储缓冲。
		storage := task.DetachRouteStorage()
		// err 接收附着路由存储缓冲过程中可能产生的错误，供后续失败分支处理。
		if err := task.AttachRouteStorage(storage); err != nil {
			t.Fatal(err)
		}
		// err 接收重新规划过程中可能产生的错误，供后续失败分支处理。
		if err := task.Replan(workspace, testKey(), Point{}, dynamic); err != nil {
			t.Fatal(err)
		}
		// progress 保存由推进取得的进度，err 接收推进过程中可能产生的错误，供后续失败分支处理。
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

// TestSuspendedResumeAfterActualChangeStillSpendsReplans 验证暂停期间地图占位发生真实变化，恢复后消耗一次重规划额度并受次数上限约束。
// t 提供断言失败报告、子用例和清理注册。
func TestSuspendedResumeAfterActualChangeStillSpendsReplans(t *testing.T) {
	// change 接收遍历文本列表时当前的元素。
	for _, change := range []string{"position", "dynamic_revision", "dynamic_contents"} {
		// t 提供断言失败报告、子用例和清理注册。
		t.Run(change, func(t *testing.T) {
			// g 保存由测试网格取得的g。
			g := testGrid(t, 10, 3)
			// limits 保存由测试上限集合取得的上限集合。
			limits := testLimits(30)
			limits.MaxExpanded, limits.MaxReplans = 50, 1
			// dynamic 建立带初始值的“dynamic”，供本段继续处理。
			dynamic := Dynamic{Revision: 1}
			// position 初始化用作位置的点位。
			position := Point{0, 1}
			// task 保存由测试任务取得的任务，workspace 保存由测试任务取得的搜索工作区。
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
			// err 接收重新规划过程中可能产生的错误，供后续失败分支处理。
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
			// err 接收重新规划过程中可能产生的错误，供后续失败分支处理。
			if err := task.Replan(workspace, testKey(), position, dynamic); err != nil {
				t.Fatal(err)
			}
			if task.State() != Limited || task.Progress().Replans != 1 || task.Progress().TotalExpanded != 3 || workspace.InUse() {
				t.Fatal("第二次真实变化越过MaxReplans", task.Progress())
			}
		})
	}
}

// TestSuspendedReadyRouteResumesFromLastCommittedStepAndRejectsOldScene 验证已就绪路线在暂停后从最后一次确认移动恢复，旧场景的请求无法推进路线。
// t 提供断言失败报告、子用例和清理注册。
func TestSuspendedReadyRouteResumesFromLastCommittedStepAndRejectsOldScene(t *testing.T) {
	// g 保存由测试网格取得的g。
	g := testGrid(t, 6, 1)
	// limits 保存由测试上限集合取得的上限集合。
	limits := testLimits(6)
	limits.MaxExpanded, limits.MaxReplans = 20, 0
	// dynamic 建立带初始值的“dynamic”，供本段继续处理。
	dynamic := Dynamic{Revision: 1}
	// task 保存由测试任务取得的任务，workspace 保存由测试任务取得的搜索工作区。
	task, workspace := testTask(t, g, Point{}, Point{5, 0}, limits, dynamic)
	finish(t, task, 1)
	// next 接收下一状态供后续步骤使用，ok 表明查询、校验或类型转换是否成功，err 接收下一状态过程中可能产生的错误，供后续失败分支处理。
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
	// oldScene 保存由测试键取得的原状态场景。
	oldScene := testKey()
	oldScene.Scene++
	if task.Replan(workspace, oldScene, next, dynamic) == nil || task.State() != Cancelled || workspace.InUse() {
		t.Fatal("暂停恢复允许旧scene写入新场景")
	}
}

// TestAllThreeByThreeObstacleGraphsMatchIndependentShortestPath 穷举 3×3 网格的障碍组合和可用起终点，与独立最短路比较可达性及实际步数。
// t 提供断言失败报告、子用例和清理注册。
func TestAllThreeByThreeObstacleGraphsMatchIndependentShortestPath(t *testing.T) {
	// mask 规定掩码的初始取值。
	for mask := 0; mask < 512; mask++ {
		// walls 建立带初始值的点位列表，供本段继续处理。
		walls := []Point{}
		// i 标记循环或列表中的当前位置。
		for i := 0; i < 9; i++ {
			if mask&(1<<uint(i)) != 0 {
				walls = append(walls, Point{i % 3, i / 3})
			}
		}
		// g 保存由测试网格取得的g。
		g := testGrid(t, 3, 3, walls...)
		// workspace 保存由创建搜索工作区取得的搜索工作区。
		workspace, _ := NewWorkspace(9, 9)
		// a 规定a的初始取值。
		for a := 0; a < 9; a++ {
			if mask&(1<<uint(a)) != 0 {
				continue
			}
			// b 规定b的初始取值。
			for b := 0; b < 9; b++ {
				if mask&(1<<uint(b)) != 0 {
					continue
				}
				// start 初始化用作起点或开始时间的点位，goal 建立带初始值的点位，供本段继续处理。
				start, goal := Point{a % 3, a / 3}, Point{b % 3, b / 3}
				// task 保存由创建任务取得的任务，err 接收创建任务过程中可能产生的错误，供后续失败分支处理。
				task, err := NewTask(g, workspace, make([]Point, 9), testKey(), start, goal, Dynamic{Revision: 1}, testLimits(9))
				if err != nil {
					t.Fatal(err)
				}
				finish(t, task, 1)
				// want 保存由“oracle”距离取得的期望值。
				want := oracleDistance(3, 3, mask, start, goal)
				if want < 0 {
					if task.State() != NoPath {
						t.Fatalf("mask%d %v→%v应不可达：%s", mask, start, goal, task.State())
					}
					continue
				}
				// actual 规定实测的初始取值，position 保存当前处理所用的位置。
				actual, position := 0, start
				for task.State() == Ready {
					// next 接收下一状态供后续步骤使用，ok 表明查询、校验或类型转换是否成功，err 接收下一状态过程中可能产生的错误，供后续失败分支处理。
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

// TestExpansionAndRouteReconstructionBothHonorOneUnitBudget 验证每次仅分配一个工作量单位时，搜索扩展和路线回溯均不会突破该帧预算。
// t 提供断言失败报告、子用例和清理注册。
func TestExpansionAndRouteReconstructionBothHonorOneUnitBudget(t *testing.T) {
	// g 保存由测试网格取得的g。
	g := testGrid(t, 400, 1)
	// task 保存由测试任务取得的任务，w 接收测试任务供后续步骤使用。
	task, w := testTask(t, g, Point{}, Point{399, 0}, testLimits(400), Dynamic{Revision: 1})
	// expanded 规定“expanded”的初始取值，work 规定工作量的初始取值，buildingRounds 规定构建“rounds”的初始取值。
	expanded, work, buildingRounds := 0, 0, 0
	for task.State() == Searching || task.State() == Building {
		// p 接收推进供后续步骤使用，err 接收推进过程中可能产生的错误，供后续失败分支处理。
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
	// second 保存由创建任务取得的秒，err 接收创建任务过程中可能产生的错误，供后续失败分支处理。
	second, err := NewTask(g, w, make([]Point, 400), testKey(), Point{10, 0}, Point{20, 0}, Dynamic{Revision: 1}, testLimits(400))
	if err != nil {
		t.Fatal(err)
	}
	// next 接收下一状态供后续步骤使用，ok 表明查询、校验或类型转换是否成功，err 接收下一状态过程中可能产生的错误，供后续失败分支处理。
	if next, ok, err := task.Next(testKey(), Point{}, Dynamic{Revision: 1}); err != nil || !ok || next != (Point{1, 0}) {
		t.Fatal("复用工作区破坏旧路线", next, err)
	}
	task.Cancel()
	if !w.InUse() {
		t.Fatal("旧路线取消错误释放另一个任务的工作区")
	}
	second.Cancel()
}

// TestSceneMapVersionRequestAndLifeChangesCancelWithoutOldWork 分别变更场景、地图版本、请求代次和生命代次，确认旧导航立即取消并释放工作区。
// t 提供断言失败报告、子用例和清理注册。
func TestSceneMapVersionRequestAndLifeChangesCancelWithoutOldWork(t *testing.T) {
	// name 定位按文本索引的供当前流程调用的函数映射中正在处理的成员，change 接收遍历按文本索引的供当前流程调用的函数映射时当前的元素，k 记录当前位置的下标，使用键表示。
	// k 记录当前位置的下标，使用键表示，k 记录当前位置的下标，使用键表示，k 记录当前位置的下标，使用键表示，k 记录当前位置的下标，使用键表示。
	for name, change := range map[string]func(*Key){"scene": func(k *Key) { k.Scene++ }, "map": func(k *Key) { k.MapID++ }, "version": func(k *Key) { k.MapVersion = "new" }, "request": func(k *Key) { k.RequestRevision++ }, "life": func(k *Key) { k.Life++ }} {
		// t 提供断言失败报告、子用例和清理注册。
		t.Run(name, func(t *testing.T) {
			// g 保存由测试网格取得的g。
			g := testGrid(t, 20, 20)
			// task 保存由测试任务取得的任务，w 接收测试任务供后续步骤使用。
			task, w := testTask(t, g, Point{}, Point{19, 19}, testLimits(400), Dynamic{Revision: 1})
			_, _ = task.Advance(testKey(), 1)
			// key 保存由测试键取得的键。
			key := testKey()
			change(&key)
			// p 接收推进供后续步骤使用，err 接收推进过程中可能产生的错误，供后续失败分支处理。
			p, err := task.Advance(key, 1)
			if err != nil || p.State != Cancelled || p.WorkUsed != 0 || w.InUse() {
				t.Fatal("过期请求继续搜索或保留工作区", p, err)
			}
		})
	}
}

// TestStaticUnreachableIsTerminalAndNeverRestartsOnTick 确认静态不可达目标进入终止状态，后续模拟帧不会重启同一导航。
// t 提供断言失败报告、子用例和清理注册。
func TestStaticUnreachableIsTerminalAndNeverRestartsOnTick(t *testing.T) {
	// g 保存由测试网格取得的g。
	g := testGrid(t, 3, 3, Point{1, 0}, Point{0, 1})
	// task 保存由测试任务取得的任务，w 接收测试任务供后续步骤使用。
	task, w := testTask(t, g, Point{}, Point{2, 2}, testLimits(9), Dynamic{Revision: 1})
	// i 标记循环或列表中的当前位置。
	for i := 0; i < 100; i++ {
		// p 接收推进供后续步骤使用，err 接收推进过程中可能产生的错误，供后续失败分支处理。
		p, err := task.Advance(testKey(), 1)
		if err != nil || p.State != NoPath || p.WorkUsed != 0 || p.TotalExpanded != 0 || w.InUse() {
			t.Fatal("静态不可达每tick仍在重算", p, err)
		}
	}
	if task.Replan(w, testKey(), Point{}, Dynamic{Revision: 2}) == nil {
		t.Fatal("静态不可达被动态变化自动重启")
	}
}

// TestDynamicBlockageWaitsForChangeAndReplansWithoutLosingTarget 检查动态阻挡出现时导航保留目标并等待，在占位改变后可重新寻路。
// t 提供断言失败报告、子用例和清理注册。
func TestDynamicBlockageWaitsForChangeAndReplansWithoutLosingTarget(t *testing.T) {
	// g 保存由测试网格取得的g。
	g := testGrid(t, 5, 1)
	// task 保存由测试任务取得的任务，w 接收测试任务供后续步骤使用。
	task, w := testTask(t, g, Point{}, Point{4, 0}, testLimits(5), Dynamic{Revision: 1, Blocked: []Point{{2, 0}}})
	finish(t, task, 1)
	if task.State() != WaitingDynamic || w.InUse() {
		t.Fatal("动态堵路未终止当前搜索")
	}
	// before 保存合计值“expanded”，用于当前步骤中的操作前的状态。
	before := task.Progress().TotalExpanded
	// i 标记循环或列表中的当前位置。
	for i := 0; i < 10; i++ {
		if task.Replan(w, testKey(), Point{}, Dynamic{Revision: 1, Blocked: []Point{{2, 0}}}) == nil {
			t.Fatal("未变化的占格触发重复搜索")
		}
	}
	if task.Progress().TotalExpanded != before || w.InUse() {
		t.Fatal("等待状态仍占用搜索预算")
	}
	// 累计预算由本请求所有尝试共享，不能重置成新5次。
	// err 接收重新规划过程中可能产生的错误，供后续失败分支处理。
	if err := task.Replan(w, testKey(), Point{}, Dynamic{Revision: 2}); err != nil {
		t.Fatal(err)
	}
	finish(t, task, 1)
	if task.State() != Limited || task.Progress().TotalExpanded != 5 {
		t.Fatal("重规划重置了累计展开预算", task.Progress())
	}
}

// TestReadyRouteChecksOccupancyAndOnlyCommitsSuccessfulStep 检验下一步提交前会重新验证占位，实际移动成功才消费路线，并拒绝无效回报。
// t 提供断言失败报告、子用例和清理注册。
func TestReadyRouteChecksOccupancyAndOnlyCommitsSuccessfulStep(t *testing.T) {
	// g 保存由测试网格取得的g。
	g := testGrid(t, 5, 3)
	// limits 保存由测试上限集合取得的上限集合。
	limits := testLimits(15)
	limits.MaxExpanded = 60
	// task 保存由测试任务取得的任务，w 接收测试任务供后续步骤使用。
	task, w := testTask(t, g, Point{0, 1}, Point{4, 1}, limits, Dynamic{Revision: 1})
	finish(t, task, 2)
	// first 保存由下一状态取得的第一项，ok 表明查询、校验或类型转换是否成功，err 接收下一状态过程中可能产生的错误，供后续失败分支处理。
	first, ok, err := task.Next(testKey(), Point{0, 1}, Dynamic{Revision: 1})
	if err != nil || !ok {
		t.Fatal(err)
	}
	if task.CommitStep(testKey(), Point{0, 1}, Point{0, 2}) == nil {
		t.Fatal("未成功的其它格被当成路径推进")
	}
	// again 保存由下一状态取得的“again”。
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
	// position 初始化用作位置的点位。
	position := Point{0, 1}
	for task.State() == Ready {
		// next 接收下一状态供后续步骤使用，ok 表明查询、校验或类型转换是否成功，e 接收下一状态供后续步骤使用。
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

// TestReplansFrontierAndRouteCapsHaveExplicitLimitOutcome 分别触发重规划次数、搜索开放集、路线长度的上限，核验可区分的限制结果。
// t 提供断言失败报告、子用例和清理注册。
func TestReplansFrontierAndRouteCapsHaveExplicitLimitOutcome(t *testing.T) {
	// t 提供断言失败报告、子用例和清理注册。
	t.Run("frontier", func(t *testing.T) {
		// g 保存由测试网格取得的g。
		g := testGrid(t, 3, 3)
		// l 保存由测试上限集合取得的l。
		l := testLimits(9)
		l.MaxFrontier = 1
		// task 保存由测试任务取得的任务，w 接收测试任务供后续步骤使用。
		task, w := testTask(t, g, Point{}, Point{2, 2}, l, Dynamic{Revision: 1})
		finish(t, task, 1)
		if task.State() != Limited || w.InUse() {
			t.Fatal("frontier容量被放大或伪报不可达")
		}
	})
	// t 提供断言失败报告、子用例和清理注册。
	t.Run("route", func(t *testing.T) {
		// g 保存由测试网格取得的g。
		g := testGrid(t, 5, 1)
		// l 保存由测试上限集合取得的l。
		l := testLimits(5)
		l.MaxRouteCells = 2
		// task 保存由测试任务取得的任务，w 接收测试任务供后续步骤使用。
		task, w := testTask(t, g, Point{}, Point{4, 0}, l, Dynamic{Revision: 1})
		finish(t, task, 1)
		if task.State() != Limited || w.InUse() || task.Progress().RouteRemaining != 0 {
			t.Fatal("路径容量被放大或暴露部分完成路线")
		}
	})
	// t 提供断言失败报告、子用例和清理注册。
	t.Run("replans", func(t *testing.T) {
		// g 保存由测试网格取得的g。
		g := testGrid(t, 5, 1)
		// l 保存由测试上限集合取得的l。
		l := testLimits(5)
		l.MaxReplans = 0
		// task 保存由测试任务取得的任务，w 接收测试任务供后续步骤使用。
		task, w := testTask(t, g, Point{}, Point{4, 0}, l, Dynamic{Revision: 1, Blocked: []Point{{4, 0}}})
		if task.State() != WaitingDynamic {
			t.Fatal(task.State())
		}
		// err 接收重新规划过程中可能产生的错误，供后续失败分支处理。
		if err := task.Replan(w, testKey(), Point{}, Dynamic{Revision: 2}); err != nil {
			t.Fatal(err)
		}
		if task.State() != Limited || w.InUse() {
			t.Fatal("零次重规划限制被忽略")
		}
	})
}

// TestUnexpectedPoseRequiresReplanAndBusyWorkspaceDoesNotConsumeAttempt 验证角色脱离预计位置后必须重新寻路，工作区暂忙不会提前扣除重规划次数。
// t 提供断言失败报告、子用例和清理注册。
func TestUnexpectedPoseRequiresReplanAndBusyWorkspaceDoesNotConsumeAttempt(t *testing.T) {
	// g 保存由测试网格取得的g。
	g := testGrid(t, 6, 2)
	// l 保存由测试上限集合取得的l。
	l := testLimits(12)
	l.MaxExpanded = 30
	// task 保存由测试任务取得的任务，w 接收测试任务供后续步骤使用。
	task, w := testTask(t, g, Point{}, Point{5, 0}, l, Dynamic{Revision: 1})
	finish(t, task, 2)
	// ok 表明查询、校验或类型转换是否成功，err 接收下一状态过程中可能产生的错误，供后续失败分支处理。
	_, ok, err := task.Next(testKey(), Point{0, 1}, Dynamic{Revision: 1})
	if err != nil || ok || task.State() != NeedsReplan {
		t.Fatal("推移后仍从旧起点取路径", err)
	}
	// other 保存由创建任务取得的另一项，e 接收创建任务供后续步骤使用。
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

// TestBudgetsAndTerrainDoNotAcquireHiddenDefaults 检验导航要求显式配置有效的工作预算和地形参数，缺失或越界配置会被拒绝。
// t 提供断言失败报告、子用例和清理注册。
func TestBudgetsAndTerrainDoNotAcquireHiddenDefaults(t *testing.T) {
	// err 接收创建网格过程中可能产生的错误，供后续失败分支处理。
	if _, err := NewGrid(2, "v", 2, 2, nil); err == nil {
		t.Fatal("缺碰撞位图被默认成全可走")
	}
	// err 接收创建网格过程中可能产生的错误，供后续失败分支处理。
	if _, err := NewGrid(2, "v", 1, 1, []byte{2}); err == nil {
		t.Fatal("尾部补位未拒绝")
	}
	// err 接收创建搜索工作区过程中可能产生的错误，供后续失败分支处理。
	if _, err := NewWorkspace(0, 1); err == nil {
		t.Fatal("无效工作区容量未拒绝")
	}
	// g 保存由测试网格取得的g。
	g := testGrid(t, 3, 3)
	// w 接收创建搜索工作区供后续步骤使用。
	w, _ := NewWorkspace(9, 9)
	// l 接收遍历上限集合列表时当前的元素。
	for _, l := range []Limits{{}, {MaxExpanded: 9, MaxFrontier: 0, MaxRouteCells: 9}, {MaxExpanded: 9, MaxFrontier: 9, MaxRouteCells: 0}} {
		// err 接收创建任务过程中可能产生的错误，供后续失败分支处理。
		if _, err := NewTask(g, w, make([]Point, 9), testKey(), Point{}, Point{2, 2}, Dynamic{Revision: 1}, l); err == nil {
			t.Fatal("缺预算未拒绝")
		}
	}
	// task 保存由创建任务取得的任务，err 接收创建任务过程中可能产生的错误，供后续失败分支处理。
	task, err := NewTask(g, w, make([]Point, 9), testKey(), Point{}, Point{2, 2}, Dynamic{Revision: 1}, testLimits(9))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = task.Advance(testKey(), 0); err == nil || task.Progress().TotalExpanded != 0 {
		t.Fatal("零本轮预算仍推进")
	}
	task.Cancel()
	// bits 初始化用作二进制位的字节序列。
	bits := []byte{0}
	// copied 保存由创建网格取得的“copied”。
	copied, err := NewGrid(2, "v", 2, 2, bits)
	if err != nil {
		t.Fatal(err)
	}
	bits[0] = 15
	if !copied.Walkable(Point{}) {
		t.Fatal("可变来源切片污染了共享静态地形")
	}
}

// TestStaticCornerRulesAndDynamicPetSideCellsMatchExistingMovement 比较路线步骤和现有移动规则，覆盖墙角夹缝、宠物阻挡侧格及斜走。
// t 提供断言失败报告、子用例和清理注册。
func TestStaticCornerRulesAndDynamicPetSideCellsMatchExistingMovement(t *testing.T) {
	// scenario 接收遍历临时结构列表时当前的元素。
	for _, scenario := range []struct {
		// name 记录名称，使用文本表示。
		name string
		// walls 记录“walls”，使用点位列表表示。
		// dynamic 记录动态占位，使用点位列表表示。
		walls, dynamic []Point
		// steps 记录步骤集合，使用数值表示。
		steps int
	}{
		{"open_diagonal", nil, nil, 1},
		{"one_static_side_requires_detour", []Point{{1, 0}}, nil, 2},
		{"two_static_sides_forbid_corner_cut", []Point{{1, 0}, {0, 1}}, nil, -1},
		// 现有neighbor只检查对角侧格的静态地形，动态实体仅检查目标格。
		{"pet_on_side_does_not_become_static_wall", nil, []Point{{1, 0}, {0, 1}}, 1},
		{"pet_overlapping_start_can_leave", nil, []Point{{0, 0}}, 1},
	} {
		// t 提供断言失败报告、子用例和清理注册。
		t.Run(scenario.name, func(t *testing.T) {
			// g 保存由测试网格取得的g。
			g := testGrid(t, 2, 2, scenario.walls...)
			// task 保存由测试任务取得的任务。
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

// TestPetOccupyingDestinationWaitsWithoutFalseArrival 验证宠物占住终点时导航进入等待状态，角色尚未到目标格时不会报告到达。
// t 提供断言失败报告、子用例和清理注册。
func TestPetOccupyingDestinationWaitsWithoutFalseArrival(t *testing.T) {
	// g 保存由测试网格取得的g。
	g := testGrid(t, 2, 2)
	// task 保存由测试任务取得的任务，w 接收测试任务供后续步骤使用。
	task, w := testTask(t, g, Point{}, Point{1, 1}, testLimits(4), Dynamic{Revision: 1, Blocked: []Point{{1, 1}}})
	if task.State() != WaitingDynamic || w.InUse() {
		t.Fatal("宠物占目标格仍开始搜索或假到达")
	}
	// i 标记循环或列表中的当前位置。
	for i := 0; i < 20; i++ {
		// p 接收推进供后续步骤使用，err 接收推进过程中可能产生的错误，供后续失败分支处理。
		p, err := task.Advance(testKey(), 1)
		if err != nil || p.State != WaitingDynamic || p.WorkUsed != 0 {
			t.Fatal("占位未变化仍重复搜索")
		}
	}
	// err 接收重新规划过程中可能产生的错误，供后续失败分支处理。
	if err := task.Replan(w, testKey(), Point{}, Dynamic{Revision: 2}); err != nil {
		t.Fatal(err)
	}
	finish(t, task, 1)
	if task.State() != Ready || task.Progress().RouteRemaining != 1 {
		t.Fatal("宠物离开后未按原目标继续规划")
	}
}
