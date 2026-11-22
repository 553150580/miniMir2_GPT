// 本文件在分片预算内调度城镇寻路，请求按角色和宠物身份绑定，复用规划工作区。
package game

import (
	"crypto/sha256"
	"fmt"
	"math"
	"minimir/internal/devconfig"
	"minimir/internal/townnav"
	"strconv"
	"time"
)

// 每个房间只保留玩家/宠物的小请求状态。全图工作区与路线缓冲属于worker的有界池。
type townNavigationRequest struct {
	// room 记录房间。
	room *room
	// actor 记录行动实体。
	actor *entity
	// key 记录键。
	key townnav.Key
	// inputRevision 记录输入修订号，使用数值表示。
	inputRevision uint64
	// target 记录目标，使用点位表示。
	target devconfig.Point
	// grid 记录网格。
	grid *townnav.Grid
	// task 记录任务。
	task *townnav.Task
	// state 记录状态。
	state townnav.State
	// dynamic 记录动态占位。
	dynamic townnav.Dynamic
	// plannedDynamic 记录“planned”动态占位，使用数值表示。
	plannedDynamic uint64
	// plannedPosition 记录“planned”位置，使用点位表示。
	plannedPosition devconfig.Point
	// queued 标记是否处于已排队状态。
	queued bool
	// adjacent 标记是否处于相邻状态。
	adjacent bool
	// storage 记录存储缓冲，使用点位列表表示。
	storage []townnav.Point
}

// townNavigationWorker 保存城镇导航房间执行器所需的状态。
type townNavigationWorker struct {
	// workspaces 记录工作区集合，使用搜索工作区列表表示。
	workspaces []*townnav.Workspace
	// routes 记录路由集合，使用点位列表列表表示。
	routes [][]townnav.Point
	// queue 记录队列，使用城镇导航请求列表表示。
	queue []*townNavigationRequest
	// active 记录“active”，使用城镇导航请求列表表示。
	active []*townNavigationRequest
	// holders 记录“holders”，使用城镇导航请求列表表示。
	holders []*townNavigationRequest
	// scratch 记录临时工作区，使用点位列表表示。
	scratch []townnav.Point
	// sequence 记录序号，使用数值表示。
	sequence uint64
	// cursor 记录游标，使用数值表示。
	cursor int
	// remaining 记录剩余额度，使用数值表示。
	remaining int
	// used 记录“used”，使用数值表示。
	used int
	// tick 记录模拟帧，使用时间点表示。
	tick time.Time
}

// initializeTownNavigation必须在全部worker构造完成、owner准入与goroutine启动之前调用。
// e 是调用该方法的游戏引擎实例。
func (e *Engine) initializeTownNavigation() error {
	// cfg 保存城镇导航，用于当前步骤中的服务配置。
	cfg := e.Store.Cfg.Fixture.TownNavigation
	if cfg == nil || !cfg.Enabled {
		return nil
	}
	// err 接收校验城镇“configuration”过程中可能产生的错误，供后续失败分支处理。
	if err := e.Store.Cfg.Fixture.ValidateTownConfiguration(); err != nil {
		return err
	}
	if cfg.WorkPerTask <= 0 || cfg.WorkPerWorkerTick <= 0 || cfg.WorkspacesPerWorker <= 0 || cfg.QueuePerWorker <= 0 || cfg.MaxExpanded <= 0 || cfg.MaxExpanded > math.MaxInt32 || cfg.MaxFrontier <= 0 || cfg.MaxRouteCells <= 0 || cfg.MaxReplans < 0 || cfg.MaxDynamicCells < 0 || cfg.WorkspacesPerWorker > math.MaxInt-cfg.QueuePerWorker {
		return fmt.Errorf("城镇导航预算必须显式完整且不溢出")
	}
	e.TownGrids = make(map[uint64]*townnav.Grid)
	// maxCells 规定最大地图格集合的初始取值。
	maxCells := 0
	// f 保存当前处理所用的开发测试配置。
	f := &e.Store.Cfg.Fixture
	// maps 保留加入新条目后的地图集合。
	maps := append([]devconfig.MapTemplate{f.Map}, f.Maps...)
	// m 接收遍历地图集合时当前的元素。
	for _, m := range maps {
		if m.SceneKindValue() != devconfig.SceneKindTown || f.RuleVersion >= 24 && !f.TownDestination(m.ID) {
			continue
		}
		// bits 保存由城镇“collision”二进制位取得的二进制位。
		bits := m.TownCollisionBits()
		// version 保存由“sprintf”取得的版本。
		version := fmt.Sprintf("%x", sha256.Sum256(bits))
		// grid 保存由创建网格取得的网格，err 接收创建网格过程中可能产生的错误，供后续失败分支处理。
		grid, err := townnav.NewGrid(m.ID, version, m.Width, m.Height, bits)
		if err != nil {
			return fmt.Errorf("城镇地图%d导航准备失败: %w", m.ID, err)
		}
		// exists 保存从城镇网格集合取出的存在。
		if _, exists := e.TownGrids[m.ID]; exists {
			return fmt.Errorf("城镇导航地图ID重复%d", m.ID)
		}
		e.TownGrids[m.ID] = grid
		maxCells = max(maxCells, grid.Cells())
	}
	if maxCells == 0 {
		return fmt.Errorf("启用城镇导航但没有城镇地图")
	}
	// routeCells 保存由最小取得的路由地图格集合。
	routeCells := min(cfg.MaxRouteCells, maxCells)
	// frontier 保存由最小取得的搜索边界。
	frontier := min(cfg.MaxFrontier, maxCells)
	// word 保存由整数“64”取得的“word”。
	word := int64(strconv.IntSize / 8)
	// reserved 保存根据整数“64”与整数“64”与当前数据与“word”的计算结果的计算结果与整数“64”与“word”的计算结果的计算结果的计算结果与整数“64”与整数“64”的计算结果与当前数据的计算结果与“word”的计算结果计算的已保留。
	reserved := int64(cfg.WorkspacesPerWorker)*(int64(maxCells)*(12+word)+int64(frontier)*word) + int64(cfg.WorkspacesPerWorker+cfg.QueuePerWorker)*int64(routeCells)*2*word
	if reserved > cfg.MaxMemoryBytesPerWorker {
		return fmt.Errorf("全部城镇最大图的导航大缓冲预留超过显式每worker预算")
	}
	// w 接收遍历房间执行器集合时当前的元素。
	for _, w := range e.workers {
		if w.townNavigation != nil {
			return fmt.Errorf("城镇导航worker不可重复初始化")
		}
		// n 保存当前处理所用的城镇导航房间执行器。
		n := &townNavigationWorker{queue: make([]*townNavigationRequest, 0, cfg.QueuePerWorker), active: make([]*townNavigationRequest, 0, cfg.WorkspacesPerWorker), holders: make([]*townNavigationRequest, 0, cfg.WorkspacesPerWorker+cfg.QueuePerWorker), scratch: make([]townnav.Point, 0, cfg.MaxDynamicCells)}
		// i 标记循环或列表中的当前位置。
		for i := 0; i < cfg.WorkspacesPerWorker; i++ {
			// workspace 保存由创建搜索工作区取得的搜索工作区，err 接收创建搜索工作区过程中可能产生的错误，供后续失败分支处理。
			workspace, err := townnav.NewWorkspace(maxCells, frontier)
			if err != nil {
				return err
			}
			n.workspaces = append(n.workspaces, workspace)
		}
		// i 标记循环或列表中的当前位置。
		for i := 0; i < cfg.WorkspacesPerWorker+cfg.QueuePerWorker; i++ {
			n.routes = append(n.routes, make([]townnav.Point, routeCells))
		}
		w.townNavigation = n
	}
	return nil
}

// townPoint 返回城镇点位供调用方使用。
// p 记录点位。
func townPoint(p devconfig.Point) townnav.Point { return townnav.Point{X: p.X, Y: p.Y} }

// gamePoint 返回游戏点位供调用方使用。
// p 记录点位。
func gamePoint(p townnav.Point) devconfig.Point { return devconfig.Point{X: p.X, Y: p.Y} }

// townActorIndex 返回城镇行动实体下标供调用方使用。
// r 指向当前执行器拥有并更新的目标房间，e 记录实体。
func townActorIndex(r *room, e *entity) int {
	if e == &r.Player {
		return 0
	}
	if e != nil && e == r.worldPet(r.currentWorld()) {
		return 1
	}
	return -1
}

// validTownRequest 判断有效城镇请求是否满足要求。
// w 是调用该方法的房间执行器实例，q 记录城镇导航请求。
func (w *worker) validTownRequest(q *townNavigationRequest) bool {
	// r 保存当前处理所用的房间，e 保存当前处理所用的行动实体。
	r, e := q.room, q.actor
	// i 保存由城镇行动实体下标取得的当前位置的下标。
	i := townActorIndex(r, e)
	if i < 0 || r.TownNavigation[i] != q || r.Scene != q.key.Scene || r.MapID != q.key.MapID || e.Life != q.key.Life || e.HP <= 0 || r.Isolated {
		return false
	}
	if r.Role.ID != "" && w.rooms[r.Role.ID] != r {
		return false
	}
	// grid 保存从城镇网格集合取出的网格。
	grid := w.e.TownGrids[r.MapID]
	if grid != q.grid || grid == nil || grid.Version() != q.key.MapVersion {
		return false
	}
	if i == 0 && (r.Manual || q.inputRevision != 0 && (r.Navigation.Revision != q.inputRevision || r.Destination == nil || *r.Destination != q.target)) {
		return false
	}
	return true
}

// removeTownRequest 移除城镇请求。
// items 记录道具集合，使用城镇导航请求列表表示，q 记录城镇导航请求。
func removeTownRequest(items []*townNavigationRequest, q *townNavigationRequest) []*townNavigationRequest {
	// i 定位道具集合中正在处理的成员，item 接收遍历道具集合时当前的元素。
	for i, item := range items {
		if item == q {
			copy(items[i:], items[i+1:])
			items[len(items)-1] = nil
			return items[:len(items)-1]
		}
	}
	return items
}

// releaseTownRoute 释放城镇路由。
// w 是调用该方法的房间执行器实例，q 记录城镇导航请求。
func (w *worker) releaseTownRoute(q *townNavigationRequest) {
	if q.storage != nil {
		if q.task != nil {
			q.task.DetachRouteStorage()
		}
		w.townNavigation.routes = append(w.townNavigation.routes, q.storage)
		w.townNavigation.holders = removeTownRequest(w.townNavigation.holders, q)
		q.storage = nil
	}
}

// cancelTownRequest 处理取消函数城镇请求。
// w 是调用该方法的房间执行器实例，q 记录城镇导航请求。
func (w *worker) cancelTownRequest(q *townNavigationRequest) {
	if q == nil {
		return
	}
	if q.task != nil {
		q.task.Cancel()
	}
	q.state = townnav.Cancelled
	// n 保存当前处理所用的城镇导航。
	n := w.townNavigation
	if n != nil {
		n.queue = removeTownRequest(n.queue, q)
		n.active = removeTownRequest(n.active, q)
		w.releaseTownRoute(q)
	}
	q.queued = false
	// i 定位城镇导航中正在处理的成员，current 接收遍历城镇导航时当前的元素。
	for i, current := range q.room.TownNavigation {
		if current == q {
			q.room.TownNavigation[i] = nil
		}
	}
}

// cancelTownNavigation在切图、冷重建与房间移除前调用，归还所有旧scene资源。
// w 是调用该方法的房间执行器实例，r 指向当前执行器拥有并更新的目标房间。
func (w *worker) cancelTownNavigation(r *room) {
	if r == nil {
		return
	}
	// q 接收遍历城镇导航时当前的元素。
	for _, q := range r.TownNavigation {
		w.cancelTownRequest(q)
	}
	r.TownNavigation = [2]*townNavigationRequest{}
}

// occupancy只在实际占格内容改变时推进revision。不能用tick次数制造自动重搜。
// w 是调用该方法的房间执行器实例，q 记录城镇导航请求。
func (w *worker) updateTownOccupancy(q *townNavigationRequest) bool {
	// n 保存当前处理所用的城镇导航。
	n := w.townNavigation
	n.scratch = n.scratch[:0]
	// r 保存当前处理所用的房间。
	r := q.room
	if r.MonsterCount < 0 || r.MonsterCount > len(r.Monsters) {
		return false
	}
	// add 封装新增对应的后续处理，e 记录实体。
	add := func(e *entity) bool {
		if e == nil || e == q.actor || e.HP <= 0 {
			return true
		}
		if len(n.scratch) == cap(n.scratch) || !q.grid.Contains(townPoint(e.Pos)) {
			return false
		}
		n.scratch = append(n.scratch, townPoint(e.Pos))
		return true
	}
	if !add(&r.Player) || !add(r.worldPet(r.currentWorld())) {
		return false
	}
	// e 接收遍历怪物集合中的范围时当前的元素。
	for _, e := range r.Monsters[:r.MonsterCount] {
		if !add(e) {
			return false
		}
	}
	// changed 保存根据修订号与当前数据的计算结果与“len”与“len”的计算结果计算的已变化。
	changed := q.dynamic.Revision == 0 || len(n.scratch) != len(q.dynamic.Blocked)
	if !changed {
		// i 定位临时工作区中正在处理的成员，p 接收遍历临时工作区时当前的元素。
		for i, p := range n.scratch {
			if p != q.dynamic.Blocked[i] {
				changed = true
				break
			}
		}
	}
	if changed {
		if q.dynamic.Revision == math.MaxUint64 {
			return false
		}
		q.dynamic.Revision++
		q.dynamic.Blocked = append(q.dynamic.Blocked[:0], n.scratch...)
	}
	return true
}

// townBlocked 判断城镇阻挡是否满足要求。
// q 记录城镇导航请求，p 记录点位。
func townBlocked(q *townNavigationRequest, p devconfig.Point) bool {
	// blocked 接收遍历阻挡时当前的元素。
	for _, blocked := range q.dynamic.Blocked {
		if blocked == townPoint(p) {
			return true
		}
	}
	return false
}

// queueTownRequest 判断队列城镇请求是否满足要求。
// w 是调用该方法的房间执行器实例，q 记录城镇导航请求。
func (w *worker) queueTownRequest(q *townNavigationRequest) bool {
	if q.queued {
		return true
	}
	// n 保存当前处理所用的城镇导航。
	n := w.townNavigation
	if len(n.queue) >= cap(n.queue) {
		return false
	}
	q.queued = true
	n.queue = append(n.queue, q)
	return true
}

// finishTownPlanning 完成城镇“planning”。
// w 是调用该方法的房间执行器实例，q 记录城镇导航请求。
func (w *worker) finishTownPlanning(q *townNavigationRequest) {
	q.state = q.task.State()
	if q.state == townnav.Searching || q.state == townnav.Building {
		return
	}
	// n 保存当前处理所用的城镇导航。
	n := w.townNavigation
	n.active = removeTownRequest(n.active, q)
	if q.state != townnav.Ready {
		w.releaseTownRoute(q)
		if q.state != townnav.WaitingDynamic && q.state != townnav.NeedsReplan && q.state != townnav.NoPath && !(q.state == townnav.Arrived && q.adjacent) {
			q.task = nil
		}
	}
}

// townNavigationPaused 判断城镇导航“paused”是否满足要求。
// r 指向当前执行器拥有并更新的目标房间。
func townNavigationPaused(r *room) bool {
	return r.Pending || !r.Ready || r.Conn == nil || mapTravelQuiescing(r)
}

// 暂挂只归还共享搜索/路线，不改变原经济job、地面目标或请求累计预算。
// w 是调用该方法的房间执行器实例，q 记录城镇导航请求。
func (w *worker) suspendTownRequest(q *townNavigationRequest) bool {
	if q != nil && q.task != nil && q.task.Suspend(q.key) {
		q.state = townnav.NeedsReplan
		w.townNavigation.active = removeTownRequest(w.townNavigation.active, q)
		w.releaseTownRoute(q)
		return true
	}
	return false
}

// admitTownRequests 处理“admit”城镇请求集合。
// w 是调用该方法的房间执行器实例。
func (w *worker) admitTownRequests() {
	// n 保存当前处理所用的城镇导航。
	n := w.townNavigation
	// attempts 记录队列中的条目数。
	attempts := len(n.queue)
	for attempts > 0 && len(n.queue) > 0 && len(n.routes) > 0 {
		attempts--
		// workspace 记录搜索工作区。
		var workspace *townnav.Workspace
		// candidate 接收遍历工作区集合时当前的元素。
		for _, candidate := range n.workspaces {
			if !candidate.InUse() {
				workspace = candidate
				break
			}
		}
		if workspace == nil {
			return
		}
		// q 保存从队列取出的队列中的指定项。
		q := n.queue[0]
		n.queue = removeTownRequest(n.queue, q)
		q.queued = false
		if !w.validTownRequest(q) {
			w.cancelTownRequest(q)
			continue
		}
		if townNavigationPaused(q.room) {
			w.queueTownRequest(q)
			continue
		}
		if !w.updateTownOccupancy(q) {
			q.state = townnav.Limited
			continue
		}
		q.storage = n.routes[len(n.routes)-1]
		n.routes = n.routes[:len(n.routes)-1]
		n.holders = append(n.holders, q)
		// cfg 保存城镇导航，用于当前步骤中的服务配置。
		cfg := w.e.Store.Cfg.Fixture.TownNavigation
		// limits 组装上限集合所需的数据。
		limits := townnav.Limits{MaxExpanded: cfg.MaxExpanded, MaxFrontier: min(cfg.MaxFrontier, q.grid.Cells()), MaxRouteCells: min(cfg.MaxRouteCells, q.grid.Cells()), MaxReplans: cfg.MaxReplans, MaxDynamicCells: min(cfg.MaxDynamicCells, q.grid.Cells())}
		// err 保存当前操作的失败原因，供调用方检查或返回。
		var err error
		// m 接收当前状态地图供后续步骤使用，known 保存由当前状态地图取得的已知状态。
		m, known := w.currentMap(q.room)
		// blockedStart 保存根据已知状态与可走格的计算结果与瞬移可走格计算的阻挡起点或开始时间。
		blockedStart := known && !m.Walkable(q.actor.Pos) && m.TeleportWalkable(q.actor.Pos)
		if q.task == nil {
			q.adjacent = townBlocked(q, q.target)
			// create 保存创建任务，用于当前步骤中的创建。
			create := townnav.NewTask
			if q.adjacent {
				create = townnav.NewAdjacentTask
			}
			if blockedStart {
				create = townnav.NewTaskFromBlockedStart
				if q.adjacent {
					create = townnav.NewAdjacentTaskFromBlockedStart
				}
			}
			q.task, err = create(q.grid, workspace, q.storage, q.key, townPoint(q.actor.Pos), townPoint(q.target), q.dynamic, limits)
		} else {
			err = q.task.AttachRouteStorage(q.storage)
			if err == nil {
				if blockedStart {
					err = q.task.ReplanFromBlockedStart(workspace, q.key, townPoint(q.actor.Pos), q.dynamic)
				} else {
					err = q.task.Replan(workspace, q.key, townPoint(q.actor.Pos), q.dynamic)
				}
			}
		}
		if err != nil {
			if q.task != nil {
				q.task.Cancel()
			}
			q.task = nil
			q.state = townnav.Limited
			w.releaseTownRoute(q)
			continue
		}
		q.plannedDynamic = q.dynamic.Revision
		q.plannedPosition = q.actor.Pos
		q.state = q.task.State()
		if q.state == townnav.Searching || q.state == townnav.Building {
			n.active = append(n.active, q)
		} else {
			w.finishTownPlanning(q)
		}
	}
}

// beginTownNavigationTick先公平推进既有任务，再执行逐room战斗/移动；本轮总额度只重置一次。
// w 是调用该方法的房间执行器实例，now 提供当前时点，作为状态有效期和计时判断的基准。
func (w *worker) beginTownNavigationTick(now time.Time) {
	// n 保存当前处理所用的城镇导航。
	n := w.townNavigation
	if n == nil {
		return
	}
	n.tick = now
	n.used = 0
	n.remaining = w.e.Store.Cfg.Fixture.TownNavigation.WorkPerWorkerTick
	// i 标记循环或列表中的当前位置。
	for i := 0; i < len(n.holders); {
		// q 保存从“holders”取出的“holders”中的指定项。
		q := n.holders[i]
		if !w.validTownRequest(q) || q.room.Conn == nil {
			w.cancelTownRequest(q)
			continue
		}
		if townNavigationPaused(q.room) && w.suspendTownRequest(q) {
			continue
		}
		i++
	}
	// i 标记循环或列表中的当前位置。
	for i := 0; i < len(n.active); {
		// q 保存从“active”取出的“active”中的指定项。
		q := n.active[i]
		if !w.validTownRequest(q) {
			w.cancelTownRequest(q)
		} else {
			i++
		}
	}
	// i 标记循环或列表中的当前位置。
	for i := 0; i < len(n.queue); {
		// q 保存从队列取出的队列中的指定项。
		q := n.queue[i]
		if !w.validTownRequest(q) {
			w.cancelTownRequest(q)
		} else {
			i++
		}
	}
	w.advanceTownNavigation()
}

// advanceTownNavigation 推进城镇导航。
// w 是调用该方法的房间执行器实例。
func (w *worker) advanceTownNavigation() {
	// n 保存当前处理所用的城镇导航。
	n := w.townNavigation
	if n == nil {
		return
	}
	// 即使本tick额度已用完，新进入收束/冻结也立即让出共享池。
	// i 标记循环或列表中的当前位置。
	for i := 0; i < len(n.holders); {
		// q 保存从“holders”取出的“holders”中的指定项。
		q := n.holders[i]
		if townNavigationPaused(q.room) && w.suspendTownRequest(q) {
			continue
		}
		i++
	}
	w.admitTownRequests()
	for n.remaining > 0 && len(n.active) > 0 {
		// progressed 保存当前处理所用的已进度。
		progressed := false
		// count 记录“active”中的条目数。
		count := len(n.active)
		// visited 规定“visited”的初始取值。
		for visited := 0; visited < count && n.remaining > 0 && len(n.active) > 0; visited++ {
			if n.cursor >= len(n.active) {
				n.cursor = 0
			}
			// q 保存从“active”取出的“active”中的指定项。
			q := n.active[n.cursor]
			n.cursor = (n.cursor + 1) % len(n.active)
			if !w.validTownRequest(q) {
				w.cancelTownRequest(q)
				continue
			}
			if townNavigationPaused(q.room) {
				w.suspendTownRequest(q)
				continue
			}
			// budget 保存由最小取得的预算。
			budget := min(w.e.Store.Cfg.Fixture.TownNavigation.WorkPerTask, n.remaining)
			// p 接收推进供后续步骤使用，err 接收推进过程中可能产生的错误，供后续失败分支处理。
			p, err := q.task.Advance(q.key, budget)
			if err != nil {
				w.cancelTownRequest(q)
				continue
			}
			n.used += p.WorkUsed
			n.remaining -= p.WorkUsed
			progressed = progressed || p.WorkUsed > 0
			w.finishTownPlanning(q)
			w.admitTownRequests()
		}
		if !progressed {
			break
		}
	}
}

// endTownNavigationTick利用本轮剩余额度推进刚入队目标，不重置worker总预算。
// w 是调用该方法的房间执行器实例。
func (w *worker) endTownNavigationTick() { w.advanceTownNavigation() }

// moveTown返回true表示城镇已接管，包括禁用或无工作区时；不得退回全图同步BFS。
// w 是调用该方法的房间执行器实例，r 指向当前执行器拥有并更新的目标房间，e 记录实体，target 记录目标，使用点位表示，now 提供当前时点，作为状态有效期和计时判断的基准。
// interval 记录周期，使用数值表示。
func (w *worker) moveTown(r *room, e *entity, target devconfig.Point, now time.Time, interval int64) bool {
	// m 接收当前状态地图供后续步骤使用，ok 表明查询、校验或类型转换是否成功。
	m, ok := w.currentMap(r)
	if !ok || m.SceneKindValue() != devconfig.SceneKindTown {
		return false
	}
	// i 保存由城镇行动实体下标取得的当前位置的下标。
	i := townActorIndex(r, e)
	if i < 0 {
		return true
	}
	// cfg 保存城镇导航，用于当前步骤中的服务配置。
	cfg := w.e.Store.Cfg.Fixture.TownNavigation
	// n 保存当前处理所用的城镇导航。
	n := w.townNavigation
	if cfg == nil || !cfg.Enabled || n == nil {
		w.cancelTownRequest(r.TownNavigation[i])
		return true
	}
	// q 保存从城镇导航取出的城镇导航中的指定项。
	q := r.TownNavigation[i]
	if q != nil && (!w.validTownRequest(q) || q.target != target) {
		w.cancelTownRequest(q)
		q = nil
	}
	if e.Pos == target {
		w.cancelTownRequest(q)
		return true
	}
	if townNavigationPaused(r) {
		w.suspendTownRequest(q)
		return true
	}
	if r.Isolated || e.HP <= 0 || !m.Walkable(e.Pos) && !m.TeleportWalkable(e.Pos) || !m.Walkable(target) {
		return true
	}
	if q == nil {
		// grid 保存从城镇网格集合取出的网格。
		grid := w.e.TownGrids[m.ID]
		if grid == nil || n.sequence == math.MaxUint64 {
			return true
		}
		n.sequence++
		q = &townNavigationRequest{room: r, actor: e, target: target, grid: grid, key: townnav.Key{Scene: r.Scene, MapID: r.MapID, MapVersion: grid.Version(), RequestRevision: n.sequence, Life: e.Life}, dynamic: townnav.Dynamic{Blocked: make([]townnav.Point, 0, cfg.MaxDynamicCells)}}
		if i == 0 && r.Destination != nil && *r.Destination == target {
			q.inputRevision = r.Navigation.Revision
		}
		r.TownNavigation[i] = q
	}
	if !w.updateTownOccupancy(q) {
		w.cancelTownRequest(q)
		return true
	}
	if q.task == nil && q.state == "" {
		w.queueTownRequest(q)
		return true
	}
	if q.task == nil {
		return true
	}
	if q.state == townnav.Arrived && q.adjacent && !townBlocked(q, target) && q.task.ResumeExactGoal(q.key) {
		q.adjacent = false
		q.state = townnav.NeedsReplan
	}
	if q.state == townnav.WaitingDynamic && (q.dynamic.Revision != q.plannedDynamic || e.Pos != q.plannedPosition) || (q.state == townnav.NoPath || q.state == townnav.Arrived) && e.Pos != q.plannedPosition || q.state == townnav.NeedsReplan {
		w.queueTownRequest(q)
		return true
	}
	if q.state != townnav.Ready || now.Before(e.NextMove) || now.Before(e.ActionReadyAt) {
		return true
	}
	// next 接收下一状态供后续步骤使用，hasNext 保存由下一状态取得的“has”下一状态，err 接收下一状态过程中可能产生的错误，供后续失败分支处理。
	next, hasNext, err := q.task.Next(q.key, townPoint(e.Pos), q.dynamic)
	q.state = q.task.State()
	if err != nil {
		w.cancelTownRequest(q)
		return true
	}
	if !hasNext {
		if q.state == townnav.NeedsReplan {
			w.releaseTownRoute(q)
			w.queueTownRequest(q)
		}
		return true
	}
	// point 保存由游戏点位取得的点位。
	point := gamePoint(next)
	if !w.neighbor(r, e.Pos, point, e) {
		return true
	}
	// from 保存位置，用于当前步骤中的来源。
	from := e.Pos
	// direction 保存当前处理所用的朝向。
	direction := e.Direction
	// i 定位朝向集合中正在处理的成员，d 接收遍历朝向集合时当前的元素。
	for i, d := range directions {
		if point.X-from.X == d.X && point.Y-from.Y == d.Y {
			direction = uint32(i)
			break
		}
	}
	if !w.commitWalk(r, e, point, direction, now, interval) {
		return true
	}
	// err 接收提交步骤过程中可能产生的错误，供后续失败分支处理。
	if err := q.task.CommitStep(q.key, townPoint(from), next); err != nil {
		w.cancelTownRequest(q)
		return true
	}
	q.state = q.task.State()
	q.plannedPosition = e.Pos
	if q.state == townnav.Arrived {
		w.releaseTownRoute(q)
		if !q.adjacent {
			q.task = nil
		}
	}
	return true
}
