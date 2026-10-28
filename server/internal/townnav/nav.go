// Package townnav提供有界增量寻路，不读取运行配置、不创建goroutine，也不修改角色状态。
package townnav

import (
	"errors"
	"math"
	"slices"
)

type Point struct{ X, Y int }

// Key由owner提供；Life允许首次生命的0，其他身份不可补默认。
type Key struct {
	Scene, MapID, RequestRevision, Life uint64
	MapVersion                          string
}

// Limits全部由调用方显式提供。MaxReplans/MaxDynamicCells为0分别表示不允许重规划/无动态占位。
type Limits struct {
	MaxExpanded, MaxFrontier, MaxRouteCells int
	MaxReplans, MaxDynamicCells             int
}

// Dynamic是本次搜索固定的有界占格快照；Revision由owner在实际占格变化时推进。
type Dynamic struct {
	Revision uint64
	Blocked  []Point
}

type State string

const (
	Searching      State = "searching"
	Building       State = "building_route"
	Ready          State = "route_ready"
	Arrived        State = "arrived"
	NoPath         State = "static_unreachable"
	WaitingDynamic State = "waiting_dynamic_change"
	NeedsReplan    State = "needs_replan"
	Limited        State = "budget_exhausted"
	Cancelled      State = "cancelled"
)

type Progress struct {
	State                                                                    State
	Reason                                                                   string
	WorkUsed, Expanded, TotalExpanded, RouteRemaining, Replans, PeakFrontier int
}

// Grid及静态连通分量由配置装载者在owner准入前建立，共享给所有搜索任务。
type Grid struct {
	id            uint64
	version       string
	width, height int
	bits          []byte
	component     []int32
}

var directions = [...]Point{{0, -1}, {1, -1}, {1, 0}, {1, 1}, {0, 1}, {-1, 1}, {-1, 0}, {-1, -1}}

func NewGrid(id uint64, version string, width, height int, collisionBits []byte) (*Grid, error) {
	if id == 0 || version == "" || width <= 0 || height <= 0 || width > math.MaxInt32/height {
		return nil, errors.New("地图身份、版本或格子总数无效")
	}
	cells := width * height
	bytes := cells / 8
	if cells%8 != 0 {
		bytes++
	}
	if len(collisionBits) != bytes || cells%8 != 0 && collisionBits[bytes-1]>>uint(cells%8) != 0 {
		return nil, errors.New("碰撞位图长度或补位无效")
	}
	g := &Grid{id: id, version: version, width: width, height: height, bits: append([]byte(nil), collisionBits...), component: make([]int32, cells)}
	queue := make([]int32, 0, cells)
	var label int32
	for index := 0; index < cells; index++ {
		p := g.point(index)
		if g.component[index] != 0 || !g.Walkable(p) {
			continue
		}
		label++
		queue = queue[:0]
		queue = append(queue, int32(index))
		g.component[index] = label
		for head := 0; head < len(queue); head++ {
			from := g.point(int(queue[head]))
			for _, d := range directions {
				to := Point{from.X + d.X, from.Y + d.Y}
				if !g.StepAllowed(from, to) {
					continue
				}
				next := g.index(to)
				if g.component[next] != 0 {
					continue
				}
				g.component[next] = label
				queue = append(queue, int32(next))
			}
		}
	}
	return g, nil
}

func (g *Grid) Contains(p Point) bool { return p.X >= 0 && p.Y >= 0 && p.X < g.width && p.Y < g.height }
func (g *Grid) ID() uint64            { return g.id }
func (g *Grid) Version() string       { return g.version }
func (g *Grid) Cells() int            { return g.width * g.height }
func (g *Grid) Walkable(p Point) bool {
	if !g.Contains(p) {
		return false
	}
	i := g.index(p)
	return g.bits[i/8]&(1<<uint(i%8)) == 0
}
func (g *Grid) StepAllowed(from, to Point) bool {
	if !g.Walkable(from) || !g.Walkable(to) {
		return false
	}
	dx, dy := to.X-from.X, to.Y-from.Y
	if abs(dx) > 1 || abs(dy) > 1 || dx == 0 && dy == 0 {
		return false
	}
	return dx == 0 || dy == 0 || g.Walkable(Point{from.X + dx, from.Y}) && g.Walkable(Point{from.X, from.Y + dy})
}
func (g *Grid) index(p Point) int     { return p.Y*g.width + p.X }
func (g *Grid) point(index int) Point { return Point{index % g.width, index / g.width} }
func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
func distance(a, b Point) int {
	dx, dy := abs(a.X-b.X), abs(a.Y-b.Y)
	if dx > dy {
		return dx
	}
	return dy
}

// Workspace在有界池中预分配，不按room创建常驻协程或每轮重建哈希表。
// stamp避免每次请求/重规划清零全图；pos配合decrease-key使每格heap最多一项。
type Workspace struct {
	g, parent []int32
	stamp     []uint32
	pos, heap []int
	epoch     uint32
	busy      bool
}

func NewWorkspace(cells, frontierCapacity int) (*Workspace, error) {
	if cells <= 0 || cells > math.MaxInt32 || frontierCapacity <= 0 || frontierCapacity > cells {
		return nil, errors.New("工作区容量必须显式为有效正整数")
	}
	return &Workspace{g: make([]int32, cells), parent: make([]int32, cells), stamp: make([]uint32, cells), pos: make([]int, cells), heap: make([]int, 0, frontierCapacity)}, nil
}
func (w *Workspace) InUse() bool { return w.busy }
func (w *Workspace) begin() error {
	if w.busy {
		return errors.New("工作区已经被其它导航任务占用")
	}
	if w.epoch == math.MaxUint32 {
		return errors.New("工作区访问代次耗尽，须在owner外重建")
	}
	w.epoch++
	w.busy = true
	w.heap = w.heap[:0]
	return nil
}

type Task struct {
	grid                                                  *Grid
	workspace                                             *Workspace
	key                                                   Key
	limits                                                Limits
	state                                                 State
	reason                                                string
	start, goal, expected                                 Point
	goalRadius                                            int
	dynamic                                               Dynamic
	suspended                                             bool
	route                                                 []Point // 调用方独占缓冲，按目标到起点顺序保存，读取时不做全量反转。
	cursor, buildAt, totalExpanded, replans, peakFrontier int
}

func validateLimits(limits Limits, workspace *Workspace, grid *Grid, storage []Point) error {
	if limits.MaxExpanded <= 0 || limits.MaxExpanded > math.MaxInt32 || limits.MaxFrontier <= 0 || limits.MaxRouteCells <= 0 || limits.MaxRouteCells > grid.width*grid.height || limits.MaxReplans < 0 || limits.MaxReplans > math.MaxInt32 || limits.MaxDynamicCells < 0 || limits.MaxDynamicCells > grid.width*grid.height {
		return errors.New("导航预算必须显式提供，不使用代码默认值")
	}
	if workspace == nil || len(workspace.g) < grid.width*grid.height || limits.MaxFrontier > cap(workspace.heap) || limits.MaxRouteCells > len(storage) {
		return errors.New("导航预算超过调用方预分配工作区或路径缓冲")
	}
	return nil
}
func validKey(key Key, grid *Grid) bool {
	return key.Scene != 0 && key.MapID == grid.id && key.MapVersion == grid.version && key.RequestRevision != 0
}
func validateDynamic(grid *Grid, d Dynamic, maximum int) error {
	if d.Revision == 0 || len(d.Blocked) > maximum {
		return errors.New("动态占位快照缺代次或超过显式容量")
	}
	for _, p := range d.Blocked {
		if !grid.Contains(p) {
			return errors.New("动态占位格越界")
		}
	}
	return nil
}
func blocked(points []Point, p Point) bool {
	for _, q := range points {
		if q == p {
			return true
		}
	}
	return false
}

func NewTask(grid *Grid, workspace *Workspace, storage []Point, key Key, start, goal Point, dynamic Dynamic, limits Limits) (*Task, error) {
	return newTask(grid, workspace, storage, key, start, goal, dynamic, limits, 0)
}

// NewAdjacentTask只要求收敛到目标的八向邻格，用于不能进入主人占格的宠物跟随。
func NewAdjacentTask(grid *Grid, workspace *Workspace, storage []Point, key Key, start, goal Point, dynamic Dynamic, limits Limits) (*Task, error) {
	return newTask(grid, workspace, storage, key, start, goal, dynamic, limits, 1)
}

func newTask(grid *Grid, workspace *Workspace, storage []Point, key Key, start, goal Point, dynamic Dynamic, limits Limits, radius int) (*Task, error) {
	if grid == nil || !validKey(key, grid) || !grid.Walkable(start) || !grid.Walkable(goal) {
		return nil, errors.New("导航地图/请求身份或起终点无效")
	}
	if err := validateLimits(limits, workspace, grid, storage); err != nil {
		return nil, err
	}
	if err := validateDynamic(grid, dynamic, limits.MaxDynamicCells); err != nil {
		return nil, err
	}
	t := &Task{grid: grid, key: key, limits: limits, start: start, goal: goal, expected: start, goalRadius: radius, route: storage[:0], cursor: -1}
	if err := t.begin(workspace, dynamic); err != nil {
		return nil, err
	}
	return t, nil
}

func (t *Task) begin(workspace *Workspace, d Dynamic) error {
	t.suspended = false
	t.dynamic = Dynamic{Revision: d.Revision, Blocked: append(t.dynamic.Blocked[:0], d.Blocked...)}
	t.route = t.route[:0]
	t.cursor = -1
	t.expected = t.start
	t.reason = ""
	if distance(t.start, t.goal) <= t.goalRadius {
		t.state = Arrived
		return nil
	}
	if !t.connectedGoal() {
		t.state = NoPath
		t.reason = "静态连通分量不同"
		return nil
	}
	if t.goalRadius == 0 && blocked(d.Blocked, t.goal) {
		t.state = WaitingDynamic
		t.reason = "目标格被动态对象占用"
		return nil
	}
	if err := workspace.begin(); err != nil {
		return err
	}
	t.workspace = workspace
	t.state = Searching
	start := t.grid.index(t.start)
	workspace.stamp[start] = workspace.epoch
	workspace.g[start] = 0
	workspace.parent[start] = -1
	t.push(start)
	return nil
}

func (t *Task) connectedGoal() bool {
	component := t.grid.component[t.grid.index(t.start)]
	for y := t.goal.Y - t.goalRadius; y <= t.goal.Y+t.goalRadius; y++ {
		for x := t.goal.X - t.goalRadius; x <= t.goal.X+t.goalRadius; x++ {
			p := Point{x, y}
			if t.grid.Walkable(p) && t.grid.component[t.grid.index(p)] == component {
				return true
			}
		}
	}
	return false
}

// ResumeExactGoal在占格对象离开后恢复原地面目标，不清累计展开/重规划预算。
func (t *Task) ResumeExactGoal(key Key) bool {
	if !t.sync(key) || t.goalRadius != 1 || t.state != Arrived {
		return false
	}
	t.goalRadius = 0
	t.state = NeedsReplan
	return true
}

// Suspend释放大缓冲并保留同请求预算。纯暂停恢复仍累计展开工作，但不消耗实际重规划次数。
func (t *Task) Suspend(key Key) bool {
	if !t.sync(key) || t.state != Searching && t.state != Building && t.state != Ready {
		return false
	}
	t.stop(NeedsReplan, "owner暂不可移动，归还共享寻路资源")
	t.suspended = true
	return true
}

// DetachRouteStorage只在不再读取路线时归还缓冲，等待动态变化不必占着大数组。
func (t *Task) DetachRouteStorage() []Point {
	if t.state == Searching || t.state == Building || t.state == Ready {
		return nil
	}
	storage := t.route[:cap(t.route)]
	t.route = nil
	return storage
}

func (t *Task) AttachRouteStorage(storage []Point) error {
	if t.state != NeedsReplan && t.state != WaitingDynamic && t.state != NoPath && t.state != Arrived || len(storage) < t.limits.MaxRouteCells {
		return errors.New("仅等待重规划任务可接收足够的独立路线缓冲")
	}
	t.route = storage[:0]
	return nil
}

func (t *Task) releaseWorkspace() {
	if t.workspace != nil {
		t.workspace.heap = t.workspace.heap[:0]
		t.workspace.busy = false
		t.workspace = nil
	}
}
func (t *Task) Cancel() {
	t.suspended = false
	t.releaseWorkspace()
	t.route = t.route[:0]
	t.cursor = -1
	t.state = Cancelled
	t.reason = "导航身份失效或owner取消"
}
func (t *Task) sync(key Key) bool {
	if key != t.key {
		t.Cancel()
		return false
	}
	return t.state != Cancelled
}
func (t *Task) State() State { return t.state }
func (t *Task) Progress() Progress {
	return Progress{State: t.state, Reason: t.reason, TotalExpanded: t.totalExpanded, Replans: t.replans, RouteRemaining: t.cursor + 1, PeakFrontier: t.peakFrontier}
}
func (t *Task) stop(state State, reason string) {
	t.suspended = false
	t.releaseWorkspace()
	t.state = state
	t.reason = reason
	t.route = t.route[:0]
	t.cursor = -1
}

// Advance的每个工作单位是一次唯一节点展开或一格路径回溯；最多8个邻格检查。
// 调用方应从worker本轮总预算扣WorkUsed，而不是给每个room各一份完整预算。
func (t *Task) Advance(key Key, budget int) (Progress, error) {
	if budget <= 0 {
		return t.Progress(), errors.New("本轮展开上限必须显式为正整数")
	}
	if !t.sync(key) {
		return t.Progress(), nil
	}
	used, expanded := 0, 0
	for used < budget && (t.state == Searching || t.state == Building) {
		w := t.workspace
		if t.state == Building {
			if t.buildAt == t.grid.index(t.start) {
				t.state = Ready
				t.cursor = len(t.route) - 1
				t.releaseWorkspace()
				break
			}
			if len(t.route) >= t.limits.MaxRouteCells {
				t.stop(Limited, "路径长度预算耗尽")
				break
			}
			t.route = append(t.route, t.grid.point(t.buildAt))
			t.buildAt = int(w.parent[t.buildAt])
			used++
			continue
		}
		if len(w.heap) == 0 {
			if len(t.dynamic.Blocked) > 0 {
				t.stop(WaitingDynamic, "当前动态占位下无路，等待占位变化")
			} else {
				t.stop(NoPath, "静态搜索已完成且无路")
			}
			break
		}
		if t.totalExpanded >= t.limits.MaxExpanded {
			t.stop(Limited, "累计展开预算耗尽")
			break
		}
		cur := t.pop()
		used++
		expanded++
		t.totalExpanded++
		if distance(t.grid.point(cur), t.goal) <= t.goalRadius {
			t.state = Building
			t.buildAt = cur
			continue
		}
		from := t.grid.point(cur)
		for _, d := range directions {
			to := Point{from.X + d.X, from.Y + d.Y}
			if !t.grid.StepAllowed(from, to) || blocked(t.dynamic.Blocked, to) {
				continue
			}
			next := t.grid.index(to)
			cost := w.g[cur] + 1
			if w.stamp[next] == w.epoch {
				if w.pos[next] < 0 || cost >= w.g[next] {
					continue
				}
				w.g[next] = cost
				w.parent[next] = int32(cur)
				t.up(w.pos[next])
				continue
			}
			if len(w.heap) >= t.limits.MaxFrontier {
				t.stop(Limited, "frontier容量预算耗尽")
				break
			}
			w.stamp[next] = w.epoch
			w.g[next] = cost
			w.parent[next] = int32(cur)
			t.push(next)
		}
	}
	p := t.Progress()
	p.WorkUsed = used
	p.Expanded = expanded
	return p, nil
}

// Next只给候选下一步，不修改实体；owner须在同一串行回合核验占位并成功移动后CommitStep。
func (t *Task) Next(key Key, position Point, dynamic Dynamic) (Point, bool, error) {
	if !t.sync(key) {
		return Point{}, false, nil
	}
	if err := validateDynamic(t.grid, dynamic, t.limits.MaxDynamicCells); err != nil {
		return Point{}, false, err
	}
	if dynamic.Revision < t.dynamic.Revision {
		return Point{}, false, errors.New("动态占位快照代次过期")
	}
	if t.state != Ready {
		return Point{}, false, nil
	}
	if position != t.expected {
		t.state = NeedsReplan
		t.reason = "实际起脚位置已改变"
		return Point{}, false, nil
	}
	if t.cursor < 0 {
		t.state = Arrived
		return Point{}, false, nil
	}
	next := t.route[t.cursor]
	if !t.grid.StepAllowed(position, next) || blocked(dynamic.Blocked, next) {
		t.state = NeedsReplan
		t.reason = "路径下一格已被占用或失效"
		return Point{}, false, nil
	}
	return next, true, nil
}

func (t *Task) CommitStep(key Key, from, to Point) error {
	if !t.sync(key) {
		return errors.New("导航身份已经失效")
	}
	if t.state != Ready || from != t.expected || t.cursor < 0 || to != t.route[t.cursor] {
		return errors.New("只能确认原路径上真正成功的一步")
	}
	t.expected = to
	t.cursor--
	if t.cursor < 0 {
		t.state = Arrived
	}
	return nil
}

// Replan不会隐式分配工作区；纯暂停且起脚/占格未变时只恢复共享资源。
// 任何实际变化仍消耗重规划次数，两条路径均保留同一请求的累计展开预算。
func (t *Task) Replan(workspace *Workspace, key Key, position Point, dynamic Dynamic) error {
	if !t.sync(key) {
		return errors.New("导航身份已经失效")
	}
	if t.state != NeedsReplan && t.state != WaitingDynamic && t.state != NoPath && t.state != Arrived {
		return errors.New("当前导航状态不允许自动重新搜索")
	}
	if err := validateLimits(t.limits, workspace, t.grid, t.route[:cap(t.route)]); err != nil {
		return err
	}
	if err := validateDynamic(t.grid, dynamic, t.limits.MaxDynamicCells); err != nil {
		return err
	}
	if !t.grid.Walkable(position) {
		return errors.New("重新规划起点不可走")
	}
	if workspace.busy {
		return errors.New("重规划工作区尚未归还，不能抢占其它任务")
	}
	if dynamic.Revision < t.dynamic.Revision || (t.state == NoPath || t.state == Arrived) && position == t.expected || t.state == WaitingDynamic && dynamic.Revision == t.dynamic.Revision && position == t.expected {
		return errors.New("占位和位置未变化，不重复搜索")
	}
	resume := t.suspended && position == t.expected && dynamic.Revision == t.dynamic.Revision && slices.Equal(dynamic.Blocked, t.dynamic.Blocked)
	if !resume && t.replans >= t.limits.MaxReplans {
		t.stop(Limited, "重规划次数预算耗尽")
		return nil
	}
	t.releaseWorkspace()
	if !resume {
		t.replans++
	}
	t.start = position
	return t.begin(workspace, dynamic)
}

func (t *Task) less(a, b int) bool {
	w := t.workspace
	ha, hb := max(0, distance(t.grid.point(a), t.goal)-t.goalRadius), max(0, distance(t.grid.point(b), t.goal)-t.goalRadius)
	fa, fb := int64(w.g[a])+int64(ha), int64(w.g[b])+int64(hb)
	if fa != fb {
		return fa < fb
	}
	if ha != hb {
		return ha < hb
	}
	return a < b
}
func (t *Task) swap(i, j int) {
	w := t.workspace
	w.heap[i], w.heap[j] = w.heap[j], w.heap[i]
	w.pos[w.heap[i]] = i
	w.pos[w.heap[j]] = j
}
func (t *Task) up(index int) {
	w := t.workspace
	for index > 0 {
		parent := (index - 1) / 2
		if !t.less(w.heap[index], w.heap[parent]) {
			break
		}
		t.swap(index, parent)
		index = parent
	}
}
func (t *Task) push(index int) {
	w := t.workspace
	at := len(w.heap)
	w.heap = w.heap[:at+1]
	w.heap[at] = index
	if len(w.heap) > t.peakFrontier {
		t.peakFrontier = len(w.heap)
	}
	w.pos[index] = at
	t.up(at)
}
func (t *Task) pop() int {
	w := t.workspace
	out := w.heap[0]
	last := len(w.heap) - 1
	t.swap(0, last)
	w.heap = w.heap[:last]
	w.pos[out] = -1
	for at := 0; ; {
		left := at*2 + 1
		if left >= len(w.heap) {
			break
		}
		best := left
		right := left + 1
		if right < len(w.heap) && t.less(w.heap[right], w.heap[left]) {
			best = right
		}
		if !t.less(w.heap[best], w.heap[at]) {
			break
		}
		t.swap(at, best)
		at = best
	}
	return out
}
