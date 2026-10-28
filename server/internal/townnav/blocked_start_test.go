package townnav

import "testing"

// 独立BFS只放宽实体已占起点；后续格及斜角仍是未阻挡格，不调用候选邻接/连通/启发式。
func oracleOccupiedStart(mask int, start, goal Point) int {
	valid := func(p Point) bool {
		return p.X >= 0 && p.Y >= 0 && p.X < 3 && p.Y < 3 && mask&(1<<uint(p.Y*3+p.X)) == 0
	}
	dist := [9]int{-1, -1, -1, -1, -1, -1, -1, -1, -1}
	dist[start.Y*3+start.X] = 0
	queue := []Point{start}
	for head := 0; head < len(queue); head++ {
		from := queue[head]
		if from == goal {
			return dist[from.Y*3+from.X]
		}
		for dy := -1; dy <= 1; dy++ {
			for dx := -1; dx <= 1; dx++ {
				to := Point{from.X + dx, from.Y + dy}
				if dx == 0 && dy == 0 || !valid(to) || dx != 0 && dy != 0 && (!valid(Point{from.X + dx, from.Y}) || !valid(Point{from.X, from.Y + dy})) {
					continue
				}
				i := to.Y*3 + to.X
				if dist[i] < 0 {
					dist[i] = dist[from.Y*3+from.X] + 1
					queue = append(queue, to)
				}
			}
		}
	}
	return -1
}

func TestAuthorizedOccupiedStartMatchesIndependentBFSAndDoesNotOpenWalls(t *testing.T) {
	for mask := 1; mask < 512; mask++ {
		walls := []Point{}
		for i := 0; i < 9; i++ {
			if mask&(1<<uint(i)) != 0 {
				walls = append(walls, Point{i % 3, i / 3})
			}
		}
		grid := testGrid(t, 3, 3, walls...)
		for startIndex := 0; startIndex < 9; startIndex++ {
			if mask&(1<<uint(startIndex)) == 0 {
				continue
			}
			start := Point{startIndex % 3, startIndex / 3}
			for goalIndex := 0; goalIndex < 9; goalIndex++ {
				if mask&(1<<uint(goalIndex)) != 0 {
					continue
				}
				goal := Point{goalIndex % 3, goalIndex / 3}
				limits := testLimits(9)
				workspace, err := NewWorkspace(9, 9)
				if err != nil {
					t.Fatal(err)
				}
				if _, err = NewTask(grid, workspace, make([]Point, 9), testKey(), start, goal, Dynamic{Revision: 1}, limits); err == nil {
					t.Fatal("普通请求误获blocked起点豁免")
				}
				task, err := NewTaskFromBlockedStart(grid, workspace, make([]Point, 9), testKey(), start, goal, Dynamic{Revision: 1}, limits)
				if err != nil {
					t.Fatal(err)
				}
				finish(t, task, 1)
				expected := oracleOccupiedStart(mask, start, goal)
				if expected < 0 {
					if task.State() != NoPath {
						t.Fatalf("mask=%d %v->%v 不可达却得到%s", mask, start, goal, task.State())
					}
					continue
				}
				if task.State() != Ready {
					t.Fatalf("mask=%d %v->%v 应可达却得到%s", mask, start, goal, task.State())
				}
				position := start
				steps := 0
				for position != goal {
					next, ok, err := task.Next(testKey(), position, Dynamic{Revision: 1})
					if err != nil || !ok {
						t.Fatal("合法起脚路线未给下一步", mask, position, goal, err)
					}
					if !grid.Walkable(next) {
						t.Fatal("首步豁免泄漏到路线阻挡", next)
					}
					if err = task.CommitStep(testKey(), position, next); err != nil {
						t.Fatal(err)
					}
					position = next
					steps++
					if steps > 9 {
						t.Fatal("路线未有界结束")
					}
				}
				if steps != expected || grid.Walkable(start) {
					t.Fatalf("路径长度/全局阻挡被改写 mask=%d got=%d want=%d", mask, steps, expected)
				}
			}
		}
	}
}

func TestOccupiedStartReplanRetainsExistingGoalWorkAndReplanBudget(t *testing.T) {
	grid := testGrid(t, 5, 5, Point{2, 2})
	task, workspace := testTask(t, grid, Point{}, Point{4, 4}, testLimits(25), Dynamic{Revision: 1})
	finish(t, task, 2)
	before := task.Progress()
	_, ok, err := task.Next(testKey(), Point{2, 2}, Dynamic{Revision: 1})
	if err != nil || ok || task.State() != NeedsReplan {
		t.Fatal("真实位置改变未保留原目标转重规划", err)
	}
	if err = task.Replan(workspace, testKey(), Point{2, 2}, Dynamic{Revision: 1}); err == nil {
		t.Fatal("普通重规划静默取得起脚豁免")
	}
	if err = task.ReplanFromBlockedStart(workspace, testKey(), Point{2, 2}, Dynamic{Revision: 1}); err != nil {
		t.Fatal(err)
	}
	after := task.Progress()
	if after.Replans != before.Replans+1 || after.TotalExpanded != before.TotalExpanded {
		t.Fatal("真实传送刷新了已有目标预算", before, after)
	}
	finish(t, task, 1)
	if task.State() != Ready {
		t.Fatal("合法起脚重规划失败", task.Progress())
	}
}
