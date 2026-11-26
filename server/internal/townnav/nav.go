// Package townnav提供有界增量寻路，不读取运行配置、不创建goroutine，也不修改角色状态。
// 本文件基于只读城镇碰撞图和可复用工作区，分预算推进可暂停、恢复和重规划的寻路任务。

package townnav

import (
	"errors"
	"math"
	"slices"
)

// Point 保存点位所需的状态。
// X 记录横坐标，使用数值表示。
// Y 记录纵坐标，使用数值表示。
type Point struct{ X, Y int }

// Key由owner提供；Life允许首次生命的0，其他身份不可补默认。
type Key struct {
	// Scene 记录场景，使用数值表示。
	// MapID 标识当前业务使用的地图。
	// RequestRevision 记录请求修订号，使用数值表示。
	// Life 记录生命代次，使用数值表示。
	Scene, MapID, RequestRevision, Life uint64
	// MapVersion 记录地图版本，使用文本表示。
	MapVersion string
}

// Limits全部由调用方显式提供。MaxReplans/MaxDynamicCells为0分别表示不允许重规划/无动态占位。
type Limits struct {
	// MaxExpanded 记录最大“expanded”，使用数值表示。
	// MaxFrontier 记录最大搜索边界，使用数值表示。
	// MaxRouteCells 记录最大路由地图格集合，使用数值表示。
	MaxExpanded, MaxFrontier, MaxRouteCells int
	// MaxReplans 记录最大“replans”，使用数值表示。
	// MaxDynamicCells 记录最大动态占位地图格集合，使用数值表示。
	MaxReplans, MaxDynamicCells int
}

// Dynamic是本次搜索固定的有界占格快照；Revision由owner在实际占格变化时推进。
type Dynamic struct {
	// Revision 记录修订号，使用数值表示。
	Revision uint64
	// Blocked 记录阻挡，使用点位列表表示。
	Blocked []Point
}

// State 描述状态。
type State string

const (
	// Searching 保存“searching”所使用的文本。
	Searching State = "searching"
	// Building 保存构建所使用的文本。
	Building State = "building_route"
	// Ready 保存就绪所使用的文本。
	Ready State = "route_ready"
	// Arrived 保存“arrived”所使用的文本。
	Arrived State = "arrived"
	// NoPath 保存“no”路径所使用的文本。
	NoPath State = "static_unreachable"
	// WaitingDynamic 保存等待动态占位所使用的文本。
	WaitingDynamic State = "waiting_dynamic_change"
	// NeedsReplan 保存所需量集合重新规划所使用的文本。
	NeedsReplan State = "needs_replan"
	// Limited 保存已上限所使用的文本。
	Limited State = "budget_exhausted"
	// Cancelled 保存已经取消所使用的文本。
	Cancelled State = "cancelled"
)

// Progress 保存进度所需的状态。
type Progress struct {
	// State 记录状态。
	State State
	// Reason 记录原因，使用文本表示。
	Reason string
	// WorkUsed 记录工作量“used”，使用数值表示。
	// Expanded 记录“expanded”，使用数值表示。
	// TotalExpanded 记录合计值“expanded”，使用数值表示。
	// RouteRemaining 记录路由剩余额度，使用数值表示。
	// Replans 记录“replans”，使用数值表示。
	// PeakFrontier 记录“peak”搜索边界，使用数值表示。
	WorkUsed, Expanded, TotalExpanded, RouteRemaining, Replans, PeakFrontier int
}

// Grid及静态连通分量由配置装载者在owner准入前建立，共享给所有搜索任务。
type Grid struct {
	// id 区分当前步骤指向的目标对象。
	id uint64
	// version 记录版本，使用文本表示。
	version string
	// width 记录宽度，使用数值表示。
	// height 记录高度，使用数值表示。
	width, height int
	// bits 记录二进制位，使用字节序列表示。
	bits []byte
	// component 记录组件，使用数值列表表示。
	component []int32
}

// directions 初始化用作朝向集合的点位列表。
var directions = [...]Point{{0, -1}, {1, -1}, {1, 0}, {1, 1}, {0, 1}, {-1, 1}, {-1, 0}, {-1, -1}}

// NewGrid 构造网格。
// id 区分当前步骤指向的目标对象，version 记录版本，使用文本表示，width 记录宽度，使用数值表示，height 记录高度，使用数值表示。
// collisionBits 记录“collision”二进制位，使用字节序列表示。
func NewGrid(id uint64, version string, width, height int, collisionBits []byte) (*Grid, error) {
	if id == 0 || version == "" || width <= 0 || height <= 0 || width > math.MaxInt32/height {
		return nil, errors.New("地图身份、版本或格子总数无效")
	}
	// cells 保存根据宽度与高度计算的地图格集合。
	cells := width * height
	// bytes 保存根据地图格集合与当前数据计算的字节。
	bytes := cells / 8
	if cells%8 != 0 {
		bytes++
	}
	if len(collisionBits) != bytes || cells%8 != 0 && collisionBits[bytes-1]>>uint(cells%8) != 0 {
		return nil, errors.New("碰撞位图长度或补位无效")
	}
	// g 保存当前处理所用的g。
	g := &Grid{id: id, version: version, width: width, height: height, bits: append([]byte(nil), collisionBits...), component: make([]int32, cells)}
	// queue 创建用于维护队列的容器或通道。
	queue := make([]int32, 0, cells)
	// label 记录标签，使用数值表示。
	var label int32
	// index 规定下标的初始取值。
	for index := 0; index < cells; index++ {
		// p 接收点位供后续步骤使用。
		p := g.point(index)
		if g.component[index] != 0 || !g.Walkable(p) {
			continue
		}
		label++
		queue = queue[:0]
		queue = append(queue, int32(index))
		g.component[index] = label
		// head 规定头部的初始取值。
		for head := 0; head < len(queue); head++ {
			// from 保存由点位取得的来源。
			from := g.point(int(queue[head]))
			// d 接收遍历朝向集合时当前的元素。
			for _, d := range directions {
				// to 建立带初始值的点位，供本段继续处理。
				to := Point{from.X + d.X, from.Y + d.Y}
				if !g.StepAllowed(from, to) {
					continue
				}
				// next 保存由下标取得的下一状态。
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

// Contains 判断格点是否落在地图宽高范围内。
// g 是调用该方法的网格实例，p 记录点位。
func (g *Grid) Contains(p Point) bool { return p.X >= 0 && p.Y >= 0 && p.X < g.width && p.Y < g.height }

// ID 返回该碰撞网格所属的地图编号。
// g 是调用该方法的网格实例。
func (g *Grid) ID() uint64 { return g.id }

// Version 返回用于匹配寻路请求的地图数据版本。
// g 是调用该方法的网格实例。
func (g *Grid) Version() string { return g.version }

// Cells 返回地图的格点总数，用于分配索引工作区。
// g 是调用该方法的网格实例。
func (g *Grid) Cells() int { return g.width * g.height }

// Walkable 判断坐标是否在图内且未被静态碰撞位阻挡。
// g 是调用该方法的网格实例，p 记录点位。
func (g *Grid) Walkable(p Point) bool {
	if !g.Contains(p) {
		return false
	}
	// i 保存由下标取得的当前位置的下标。
	i := g.index(p)
	return g.bits[i/8]&(1<<uint(i%8)) == 0
}

// StepAllowed 检查两格是否相邻且可走，并限制斜走穿过阻挡拐角。
// g 是调用该方法的网格实例，from 记录来源，使用点位表示，to 记录目标，使用点位表示。
func (g *Grid) StepAllowed(from, to Point) bool {
	if !g.Walkable(from) || !g.Walkable(to) {
		return false
	}
	return g.exitStepAllowed(from, to)
}

// 仅供已获调用方授权的被占起点离开；目的点及斜向两侧仍按普通阻挡检查。
// g 是调用该方法的网格实例，from 记录来源，使用点位表示，to 记录目标，使用点位表示。
func (g *Grid) exitStepAllowed(from, to Point) bool {
	if !g.Contains(from) || !g.Walkable(to) {
		return false
	}
	// dx 保存根据横坐标与横坐标计算的横向差值，dy 保存根据纵坐标与纵坐标计算的纵向差值。
	dx, dy := to.X-from.X, to.Y-from.Y
	if abs(dx) > 1 || abs(dy) > 1 || dx == 0 && dy == 0 {
		return false
	}
	return dx == 0 || dy == 0 || g.Walkable(Point{from.X + dx, from.Y}) && g.Walkable(Point{from.X, from.Y + dy})
}

// index 把二维格点换算成按行排列的一维索引。
// g 是调用该方法的网格实例，p 记录点位。
func (g *Grid) index(p Point) int { return p.Y*g.width + p.X }

// point 把按行排列的格索引还原为横纵坐标。
// g 是调用该方法的网格实例，index 记录下标，使用数值表示。
func (g *Grid) point(index int) Point { return Point{index % g.width, index / g.width} }

// abs 返回绝对值供调用方使用。
// x 记录数值。
func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// distance 返回八方向网格中的切比雪夫距离。
// a、b 是需要计算横纵坐标差的两个格点。
func distance(a, b Point) int {
	// dx 保存由绝对值取得的横向差值，dy 保存由绝对值取得的纵向差值。
	dx, dy := abs(a.X-b.X), abs(a.Y-b.Y)
	if dx > dy {
		return dx
	}
	return dy
}

// Workspace在有界池中预分配，不按room创建常驻协程或每轮重建哈希表。
// stamp避免每次请求/重规划清零全图；pos配合decrease-key使每格heap最多一项。
type Workspace struct {
	// g 记录g，使用数值列表表示。
	// parent 记录上级，使用数值列表表示。
	g, parent []int32
	// stamp 记录标记时间，使用数值列表表示。
	stamp []uint32
	// pos 记录位置，使用数值列表表示。
	// heap 记录“heap”，使用数值列表表示。
	pos, heap []int
	// epoch 记录代次，使用数值表示。
	epoch uint32
	// busy 标记是否处于“busy”状态。
	busy bool
}

// NewWorkspace 为搜索轮次、累计代价、父节点与候选堆分配有界数组。
// cells 记录地图格集合，使用数值表示，frontierCapacity 记录搜索边界容量，使用数值表示。
func NewWorkspace(cells, frontierCapacity int) (*Workspace, error) {
	if cells <= 0 || cells > math.MaxInt32 || frontierCapacity <= 0 || frontierCapacity > cells {
		return nil, errors.New("工作区容量必须显式为有效正整数")
	}
	return &Workspace{g: make([]int32, cells), parent: make([]int32, cells), stamp: make([]uint32, cells), pos: make([]int, cells), heap: make([]int, 0, frontierCapacity)}, nil
}

// InUse 判断复用工作区当前是否被某个寻路任务占用。
// w 是供任务独占复用的搜索工作区。
func (w *Workspace) InUse() bool { return w.busy }

// begin 取得工作区并推进存储轮次，复用上一轮搜索留下的内存。
// w 是供任务独占复用的搜索工作区。
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

// Task 保存一个可按预算暂停和推进的寻路请求及路径游标。
type Task struct {
	// grid 记录网格。
	grid *Grid
	// workspace 提供搜索工作区。
	workspace *Workspace
	// key 记录键。
	key Key
	// limits 记录上限集合。
	limits Limits
	// state 记录状态。
	state State
	// reason 记录原因，使用文本表示。
	reason string
	// start 标识本次搜索的起脚格。
	// goal 标识目标格。
	// expected 记录预期结果，使用点位表示。
	start, goal, expected Point
	// goalRadius 记录目标格“radius”，使用数值表示。
	goalRadius int
	// stepWeight 将精确点地的步数排在累计直线偏差之前，邻格及大地图仍按单位步数。
	stepWeight int32
	// dynamic 保存当前动态占位快照。
	dynamic Dynamic
	// suspended 标记是否处于“suspended”状态。
	suspended bool
	// allowBlockedStart 标记是否处于“allow”阻挡起点或开始时间状态。
	allowBlockedStart bool
	route             []Point // 调用方独占缓冲，按目标到起点顺序保存，读取时不做全量反转。
	// cursor 记录游标，使用数值表示。
	// buildAt 记录构建发生时间，使用数值表示。
	// totalExpanded 记录合计值“expanded”，使用数值表示。
	// replans 记录“replans”，使用数值表示。
	// peakFrontier 记录“peak”搜索边界，使用数值表示。
	cursor, buildAt, totalExpanded, replans, peakFrontier int
}

// validateLimits 核对调用者的搜索上限与工作区、网格和路径存储容量。
// limits 记录上限集合，workspace 提供搜索工作区，grid 记录网格，storage 提供可复用的路径点缓冲。
func validateLimits(limits Limits, workspace *Workspace, grid *Grid, storage []Point) error {
	if limits.MaxExpanded <= 0 || limits.MaxExpanded > math.MaxInt32 || limits.MaxFrontier <= 0 || limits.MaxRouteCells <= 0 || limits.MaxRouteCells > grid.width*grid.height || limits.MaxReplans < 0 || limits.MaxReplans > math.MaxInt32 || limits.MaxDynamicCells < 0 || limits.MaxDynamicCells > grid.width*grid.height {
		return errors.New("导航预算必须显式提供，不使用代码默认值")
	}
	if workspace == nil || len(workspace.g) < grid.width*grid.height || limits.MaxFrontier > cap(workspace.heap) || limits.MaxRouteCells > len(storage) {
		return errors.New("导航预算超过调用方预分配工作区或路径缓冲")
	}
	return nil
}

// validKey 检查调用者提供的场景、地图版本、移动请求和实体身份。
// key 记录键，grid 记录网格。
func validKey(key Key, grid *Grid) bool {
	return key.Scene != 0 && key.MapID == grid.id && key.MapVersion == grid.version && key.RequestRevision != 0
}

// validateDynamic 验证动态占位数量与坐标是否落在预算和网格边界内。
// grid 记录网格，d 保存当前动态占位快照，maximum 记录最大值，使用数值表示。
func validateDynamic(grid *Grid, d Dynamic, maximum int) error {
	if d.Revision == 0 || len(d.Blocked) > maximum {
		return errors.New("动态占位快照缺代次或超过显式容量")
	}
	// p 接收遍历阻挡时当前的元素。
	for _, p := range d.Blocked {
		if !grid.Contains(p) {
			return errors.New("动态占位格越界")
		}
	}
	return nil
}

// blocked 查询目标格点是否出现在快照的阻挡列表。
// points 记录点位集合，使用点位列表表示，p 记录点位。
func blocked(points []Point, p Point) bool {
	// q 接收遍历点位集合时当前的元素。
	for _, q := range points {
		if q == p {
			return true
		}
	}
	return false
}

// NewTask 建立必须到达精确目标格的寻路任务。
// grid 记录网格，workspace 提供搜索工作区，storage 提供可复用的路径点缓冲，key 记录键，start 标识本次搜索的起脚格。
// goal 标识目标格，dynamic 保存当前动态占位快照，limits 记录上限集合。
func NewTask(grid *Grid, workspace *Workspace, storage []Point, key Key, start, goal Point, dynamic Dynamic, limits Limits) (*Task, error) {
	return newTask(grid, workspace, storage, key, start, goal, dynamic, limits, 0, false)
}

// NewAdjacentTask只要求收敛到目标的八向邻格，用于不能进入主人占格的宠物跟随。
// grid 记录网格，workspace 提供搜索工作区，storage 提供可复用的路径点缓冲，key 记录键，start 标识本次搜索的起脚格。
// goal 标识目标格，dynamic 保存当前动态占位快照，limits 记录上限集合。
func NewAdjacentTask(grid *Grid, workspace *Workspace, storage []Point, key Key, start, goal Point, dynamic Dynamic, limits Limits) (*Task, error) {
	return newTask(grid, workspace, storage, key, start, goal, dynamic, limits, 1, false)
}

// 调用方必须先核对实体真实站在来源静态可走格；只豁免起脚，不开放中途阻挡或目标格。
// grid 记录网格，workspace 提供搜索工作区，storage 提供可复用的路径点缓冲，key 记录键，start 标识本次搜索的起脚格。
// goal 标识目标格，dynamic 保存当前动态占位快照，limits 记录上限集合。
func NewTaskFromBlockedStart(grid *Grid, workspace *Workspace, storage []Point, key Key, start, goal Point, dynamic Dynamic, limits Limits) (*Task, error) {
	return newTask(grid, workspace, storage, key, start, goal, dynamic, limits, 0, true)
}

// NewAdjacentTaskFromBlockedStart 允许从已授权的阻挡起脚格离开，目标为指定格的相邻可走格。
// grid 记录网格，workspace 提供搜索工作区，storage 提供可复用的路径点缓冲，key 记录键，start 标识本次搜索的起脚格。
// goal 标识目标格，dynamic 保存当前动态占位快照，limits 记录上限集合。
func NewAdjacentTaskFromBlockedStart(grid *Grid, workspace *Workspace, storage []Point, key Key, start, goal Point, dynamic Dynamic, limits Limits) (*Task, error) {
	return newTask(grid, workspace, storage, key, start, goal, dynamic, limits, 1, true)
}

// newTask 构造任务。
// grid 记录网格，workspace 提供搜索工作区，storage 提供可复用的路径点缓冲，key 记录键，start 标识本次搜索的起脚格。
// goal 标识目标格，dynamic 保存当前动态占位快照，limits 记录上限集合，radius 记录“radius”，使用数值表示。
// allowBlockedStart 标记是否处于“allow”阻挡起点或开始时间状态。
func newTask(grid *Grid, workspace *Workspace, storage []Point, key Key, start, goal Point, dynamic Dynamic, limits Limits, radius int, allowBlockedStart bool) (*Task, error) {
	if grid == nil || !validKey(key, grid) || !grid.Contains(start) || !grid.Walkable(start) && !allowBlockedStart || !grid.Walkable(goal) {
		return nil, errors.New("导航地图/请求身份或起终点无效")
	}
	// err 接收校验上限集合过程中可能产生的错误，供后续失败分支处理。
	if err := validateLimits(limits, workspace, grid, storage); err != nil {
		return nil, err
	}
	// err 接收校验动态占位过程中可能产生的错误，供后续失败分支处理。
	if err := validateDynamic(grid, dynamic, limits.MaxDynamicCells); err != nil {
		return nil, err
	}
	// t 保存当前处理所用的t。
	t := &Task{grid: grid, key: key, limits: limits, start: start, goal: goal, expected: start, goalRadius: radius, route: storage[:0], cursor: -1, allowBlockedStart: allowBlockedStart}
	// err 接收“begin”过程中可能产生的错误，供后续失败分支处理。
	if err := t.begin(workspace, dynamic); err != nil {
		return nil, err
	}
	return t, nil
}

// begin 取得工作区并推进存储轮次，复用上一轮搜索留下的内存。
// t 是调用该方法的任务实例，workspace 提供搜索工作区，d 保存当前动态占位快照。
func (t *Task) begin(workspace *Workspace, d Dynamic) error {
	t.suspended = false
	t.dynamic = Dynamic{Revision: d.Revision, Blocked: append(t.dynamic.Blocked[:0], d.Blocked...)}
	t.route = t.route[:0]
	t.cursor = -1
	t.expected = t.start
	t.reason = ""
	t.stepWeight = 1
	const compactTownSide = 17
	if t.goalRadius == 0 && t.grid.width <= compactTownSide && t.grid.height <= compactTownSide {
		// 最短路线没有重复格。一个格的叉积不超过横纵范围乘积的两倍。
		// 步权比整条路线的偏差上界大，保证先少走格，再比较整条路线偏差。
		// 最大 289 格时累计代价低于 4300 万，沿用原 int32 代价工作区。
		t.stepWeight = int32(1 + (t.grid.Cells()-1)*2*(t.grid.width-1)*(t.grid.height-1))
	}
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
	// err 接收“begin”过程中可能产生的错误，供后续失败分支处理。
	if err := workspace.begin(); err != nil {
		return err
	}
	t.workspace = workspace
	t.state = Searching
	// start 保存由下标取得的起点或开始时间。
	start := t.grid.index(t.start)
	workspace.stamp[start] = workspace.epoch
	workspace.g[start] = 0
	workspace.parent[start] = -1
	t.push(start)
	return nil
}

// connectedGoal 判断“connected”目标格是否满足要求。
// t 是调用该方法的任务实例。
func (t *Task) connectedGoal() bool {
	// component 保存从组件取出的组件。
	component := t.grid.component[t.grid.index(t.start)]
	// y 保存根据纵坐标与目标格“radius”计算的纵坐标与目标格“radius”的计算结果。
	for y := t.goal.Y - t.goalRadius; y <= t.goal.Y+t.goalRadius; y++ {
		// x 保存根据横坐标与目标格“radius”计算的横坐标与目标格“radius”的计算结果。
		for x := t.goal.X - t.goalRadius; x <= t.goal.X+t.goalRadius; x++ {
			// p 组装点位所需的数据。
			p := Point{x, y}
			if t.grid.Walkable(p) {
				// goalComponent 保存从组件取出的目标格组件。
				goalComponent := t.grid.component[t.grid.index(p)]
				if component != 0 && goalComponent == component {
					return true
				}
				if component == 0 && t.allowBlockedStart {
					// d 接收遍历朝向集合时当前的元素。
					for _, d := range directions {
						// exit 初始化用作退出的点位。
						exit := Point{t.start.X + d.X, t.start.Y + d.Y}
						if t.stepAllowed(t.start, exit) && t.grid.component[t.grid.index(exit)] == goalComponent {
							return true
						}
					}
				}
			}
		}
	}
	return false
}

// stepAllowed 判断步骤允许是否满足要求。
// t 是调用该方法的任务实例，from 记录来源，使用点位表示，to 记录目标，使用点位表示。
func (t *Task) stepAllowed(from, to Point) bool {
	if t.allowBlockedStart && from == t.start && !t.grid.Walkable(from) {
		return t.grid.exitStepAllowed(from, to)
	}
	return t.grid.StepAllowed(from, to)
}

// SetAdjacentGoal 按目标的占位切换精确落格和邻格模式，保留原任务及累计预算。
// key 校验本次导航身份，adjacent 表示终点被占时先靠近可达邻格。
func (t *Task) SetAdjacentGoal(key Key, adjacent bool) bool {
	if !t.sync(key) || adjacent == (t.goalRadius == 1) || t.state == NoPath || t.state == Limited {
		return false
	}
	if adjacent {
		t.goalRadius = 1
		t.stop(NeedsReplan, "原目标格被占用，规划可达邻格")
	} else {
		t.goalRadius = 0
		t.stop(NeedsReplan, "原目标格已腾空，恢复精确寻路")
	}
	return true
}

// Suspend释放大缓冲并保留同请求预算。纯暂停恢复仍累计展开工作，但不消耗实际重规划次数。
// t 是调用该方法的任务实例，key 记录键。
func (t *Task) Suspend(key Key) bool {
	if !t.sync(key) || t.state != Searching && t.state != Building && t.state != Ready {
		return false
	}
	t.stop(NeedsReplan, "owner暂不可移动，归还共享寻路资源")
	t.suspended = true
	return true
}

// DetachRouteStorage只在不再读取路线时归还缓冲，等待动态变化不必占着大数组。
// t 是调用该方法的任务实例。
func (t *Task) DetachRouteStorage() []Point {
	if t.state == Searching || t.state == Building || t.state == Ready {
		return nil
	}
	// storage 保存当前处理所用的存储缓冲。
	storage := t.route[:cap(t.route)]
	t.route = nil
	return storage
}

// AttachRouteStorage 关联路由“storage”。
// t 是调用该方法的任务实例，storage 提供可复用的路径点缓冲。
func (t *Task) AttachRouteStorage(storage []Point) error {
	if t.state != NeedsReplan && t.state != WaitingDynamic && t.state != NoPath && t.state != Arrived || len(storage) < t.limits.MaxRouteCells {
		return errors.New("仅等待重规划任务可接收足够的独立路线缓冲")
	}
	t.route = storage[:0]
	return nil
}

// releaseWorkspace 释放“workspace”。
// t 是调用该方法的任务实例。
func (t *Task) releaseWorkspace() {
	if t.workspace != nil {
		t.workspace.heap = t.workspace.heap[:0]
		t.workspace.busy = false
		t.workspace = nil
	}
}

// Cancel 处理取消函数。
// t 是调用该方法的任务实例。
func (t *Task) Cancel() {
	t.suspended = false
	t.releaseWorkspace()
	t.route = t.route[:0]
	t.cursor = -1
	t.state = Cancelled
	t.reason = "导航身份失效或owner取消"
}

// sync 判断同步是否满足要求。
// t 是调用该方法的任务实例，key 记录键。
func (t *Task) sync(key Key) bool {
	if key != t.key {
		t.Cancel()
		return false
	}
	return t.state != Cancelled
}

// State 返回状态供调用方使用。
// t 是调用该方法的任务实例。
func (t *Task) State() State { return t.state }

// Progress 推进当前请求或状态，并把结果交给调用方。
// t 是调用该方法的任务实例。
func (t *Task) Progress() Progress {
	return Progress{State: t.state, Reason: t.reason, TotalExpanded: t.totalExpanded, Replans: t.replans, RouteRemaining: t.cursor + 1, PeakFrontier: t.peakFrontier}
}

// stop 停止当前请求或状态，并把结果交给调用方。
// t 是调用该方法的任务实例，state 记录状态，reason 记录原因，使用文本表示。
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
// t 是调用该方法的任务实例，key 记录键，budget 记录预算，使用数值表示。
func (t *Task) Advance(key Key, budget int) (Progress, error) {
	if budget <= 0 {
		return t.Progress(), errors.New("本轮展开上限必须显式为正整数")
	}
	if !t.sync(key) {
		return t.Progress(), nil
	}
	// used 累计展开和构造路径共同消耗的本轮工作量，expanded 统计本轮实际从候选堆取出并展开的格子数。
	used, expanded := 0, 0
	for used < budget && (t.state == Searching || t.state == Building) {
		// w 保存当前处理所用的搜索工作区。
		w := t.workspace
		if t.state == Building {
			// 路径从目标沿父指针逆序收集。构造过程也计入本轮展开预算。
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
		// cur 保存当前从候选堆取出的格索引。
		cur := t.pop()
		used++
		expanded++
		t.totalExpanded++
		if distance(t.grid.point(cur), t.goal) <= t.goalRadius {
			t.state = Building
			t.buildAt = cur
			continue
		}
		// from 保存由点位取得的来源。
		from := t.grid.point(cur)
		// d 接收遍历朝向集合时当前的元素。
		for _, d := range directions {
			// to 建立带初始值的点位，供本段继续处理。
			to := Point{from.X + d.X, from.Y + d.Y}
			if !t.stepAllowed(from, to) || blocked(t.dynamic.Blocked, to) {
				continue
			}
			// next 保存由下标取得的下一状态。
			next := t.grid.index(to)
			// 累计代价先比较实际步数，再比较各格到点击线的偏差。
			cost := w.g[cur] + t.stepWeight
			if t.stepWeight > 1 {
				cost += int32(t.lineDeviation(to))
			}
			// 使用轮次标记复用旧数组。已在堆中的点只在找到更小累计代价时更新。
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
	// p 接收进度供后续步骤使用。
	p := t.Progress()
	p.WorkUsed = used
	p.Expanded = expanded
	return p, nil
}

// Next只给候选下一步，不修改实体；owner须在同一串行回合核验占位并成功移动后CommitStep。
// t 是调用该方法的任务实例，key 记录键，position 记录位置，使用点位表示，dynamic 保存当前动态占位快照。
func (t *Task) Next(key Key, position Point, dynamic Dynamic) (Point, bool, error) {
	if !t.sync(key) {
		return Point{}, false, nil
	}
	// err 接收校验动态占位过程中可能产生的错误，供后续失败分支处理。
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
	// next 保存从路由取出的下一状态。
	next := t.route[t.cursor]
	if !t.stepAllowed(position, next) || blocked(dynamic.Blocked, next) {
		t.state = NeedsReplan
		t.reason = "路径下一格已被占用或失效"
		return Point{}, false, nil
	}
	return next, true, nil
}

// CommitStep 执行提交步骤，并把失败交给调用方处理。
// t 是调用该方法的任务实例，key 记录键，from 记录来源，使用点位表示，to 记录目标，使用点位表示。
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
// t 是调用该方法的任务实例，workspace 提供搜索工作区，key 记录键，position 记录位置，使用点位表示，dynamic 保存当前动态占位快照。
func (t *Task) Replan(workspace *Workspace, key Key, position Point, dynamic Dynamic) error {
	return t.replan(workspace, key, position, dynamic, false)
}

// ReplanFromBlockedStart 执行重新规划来源阻挡起点或开始时间，并把失败交给调用方处理。
// t 是调用该方法的任务实例，workspace 提供搜索工作区，key 记录键，position 记录位置，使用点位表示，dynamic 保存当前动态占位快照。
func (t *Task) ReplanFromBlockedStart(workspace *Workspace, key Key, position Point, dynamic Dynamic) error {
	return t.replan(workspace, key, position, dynamic, true)
}

// replan 执行重新规划，并把失败交给调用方处理。
// t 是调用该方法的任务实例，workspace 提供搜索工作区，key 记录键，position 记录位置，使用点位表示，dynamic 保存当前动态占位快照。
// allowBlockedStart 标记是否处于“allow”阻挡起点或开始时间状态。
func (t *Task) replan(workspace *Workspace, key Key, position Point, dynamic Dynamic, allowBlockedStart bool) error {
	if !t.sync(key) {
		return errors.New("导航身份已经失效")
	}
	if t.state != NeedsReplan && t.state != WaitingDynamic && t.state != NoPath && t.state != Arrived {
		return errors.New("当前导航状态不允许自动重新搜索")
	}
	// err 接收校验上限集合过程中可能产生的错误，供后续失败分支处理。
	if err := validateLimits(t.limits, workspace, t.grid, t.route[:cap(t.route)]); err != nil {
		return err
	}
	// err 接收校验动态占位过程中可能产生的错误，供后续失败分支处理。
	if err := validateDynamic(t.grid, dynamic, t.limits.MaxDynamicCells); err != nil {
		return err
	}
	if !t.grid.Contains(position) || !t.grid.Walkable(position) && !allowBlockedStart {
		return errors.New("重新规划起点不可走")
	}
	if workspace.busy {
		return errors.New("重规划工作区尚未归还，不能抢占其它任务")
	}
	if dynamic.Revision < t.dynamic.Revision || (t.state == NoPath || t.state == Arrived) && position == t.expected || t.state == WaitingDynamic && dynamic.Revision == t.dynamic.Revision && position == t.expected {
		return errors.New("占位和位置未变化，不重复搜索")
	}
	// resume 保存根据“suspended”与位置与预期结果的计算结果的计算结果与修订号与修订号的计算结果的计算结果与相等计算的恢复。
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
	t.allowBlockedStart = allowBlockedStart
	return t.begin(workspace, dynamic)
}

// less 判断“less”是否满足要求。
// t 是调用该方法的任务实例，a 记录a，使用数值表示，b 记录b，使用数值表示。
func (t *Task) less(a, b int) bool {
	// w 保存当前处理所用的搜索工作区。
	w := t.workspace
	// ha 保存由最大取得的“ha”，hb 保存由最大取得的“hb”。
	ha, hb := max(0, distance(t.grid.point(a), t.goal)-t.goalRadius), max(0, distance(t.grid.point(b), t.goal)-t.goalRadius)
	// 剩余切比雪夫距离只包含最少步数，保守估计未来路线的代价。
	fa, fb := int64(w.g[a])+int64(ha)*int64(t.stepWeight), int64(w.g[b])+int64(hb)*int64(t.stepWeight)
	if fa != fb {
		return fa < fb
	}
	if ha != hb {
		return ha < hb
	}
	// 步数和剩余距离相同，优先贴近原点击方向，避免按网格编号绕向屏幕一侧。
	da, db := t.lineDeviation(t.grid.point(a)), t.lineDeviation(t.grid.point(b))
	if da != db {
		return da < db
	}
	return a < b
}

// lineDeviation 用二维叉积比较格点偏离起点到点击目标的程度，只影响等长候选的排序。
func (t *Task) lineDeviation(point Point) int64 {
	dx, dy := int64(t.goal.X-t.start.X), int64(t.goal.Y-t.start.Y)
	deviation := int64(point.X-t.start.X)*dy - int64(point.Y-t.start.Y)*dx
	return max(deviation, -deviation)
}

// swap 处理交换。
// t 是调用该方法的任务实例，i 记录当前位置的下标，使用数值表示，j 记录当前位置的下标，使用数值表示。
func (t *Task) swap(i, j int) {
	// w 保存当前处理所用的搜索工作区。
	w := t.workspace
	w.heap[i], w.heap[j] = w.heap[j], w.heap[i]
	w.pos[w.heap[i]] = i
	w.pos[w.heap[j]] = j
}

// up 处理“up”。
// t 是调用该方法的任务实例，index 记录下标，使用数值表示。
func (t *Task) up(index int) {
	// w 保存当前处理所用的搜索工作区。
	w := t.workspace
	for index > 0 {
		// parent 保存根据下标与当前数据的计算结果与当前数据计算的上级。
		parent := (index - 1) / 2
		if !t.less(w.heap[index], w.heap[parent]) {
			break
		}
		t.swap(index, parent)
		index = parent
	}
}

// push 处理推入。
// t 是调用该方法的任务实例，index 记录下标，使用数值表示。
func (t *Task) push(index int) {
	// w 保存当前处理所用的搜索工作区。
	w := t.workspace
	// at 记录“heap”中的条目数。
	at := len(w.heap)
	w.heap = w.heap[:at+1]
	w.heap[at] = index
	if len(w.heap) > t.peakFrontier {
		t.peakFrontier = len(w.heap)
	}
	w.pos[index] = at
	t.up(at)
}

// pop 返回“pop”供调用方使用。
// t 是调用该方法的任务实例。
func (t *Task) pop() int {
	// w 保存当前处理所用的搜索工作区。
	w := t.workspace
	// out 保存从“heap”取出的输出。
	out := w.heap[0]
	// last 保存根据“len”与当前数据计算的上一次状态。
	last := len(w.heap) - 1
	t.swap(0, last)
	w.heap = w.heap[:last]
	w.pos[out] = -1
	// at 规定发生时间的初始取值。
	for at := 0; ; {
		// left 保存根据发生时间与当前数据的计算结果与当前数据计算的剩余量。
		left := at*2 + 1
		if left >= len(w.heap) {
			break
		}
		// best 保存当前处理所用的“best”。
		best := left
		// right 保存根据剩余量与当前数据计算的右侧。
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
