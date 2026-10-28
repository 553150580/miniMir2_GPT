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
	room            *room
	actor           *entity
	key             townnav.Key
	inputRevision   uint64
	target          devconfig.Point
	grid            *townnav.Grid
	task            *townnav.Task
	state           townnav.State
	dynamic         townnav.Dynamic
	plannedDynamic  uint64
	plannedPosition devconfig.Point
	queued          bool
	adjacent        bool
	storage         []townnav.Point
}

type townNavigationWorker struct {
	workspaces []*townnav.Workspace
	routes     [][]townnav.Point
	queue      []*townNavigationRequest
	active     []*townNavigationRequest
	holders    []*townNavigationRequest
	scratch    []townnav.Point
	sequence   uint64
	cursor     int
	remaining  int
	used       int
	tick       time.Time
}

// initializeTownNavigation必须在全部worker构造完成、owner准入与goroutine启动之前调用。
func (e *Engine) initializeTownNavigation() error {
	cfg := e.Store.Cfg.Fixture.TownNavigation
	if cfg == nil || !cfg.Enabled {
		return nil
	}
	if err := e.Store.Cfg.Fixture.ValidateTownConfiguration(); err != nil {
		return err
	}
	if cfg.WorkPerTask <= 0 || cfg.WorkPerWorkerTick <= 0 || cfg.WorkspacesPerWorker <= 0 || cfg.QueuePerWorker <= 0 || cfg.MaxExpanded <= 0 || cfg.MaxExpanded > math.MaxInt32 || cfg.MaxFrontier <= 0 || cfg.MaxRouteCells <= 0 || cfg.MaxReplans < 0 || cfg.MaxDynamicCells < 0 || cfg.WorkspacesPerWorker > math.MaxInt-cfg.QueuePerWorker {
		return fmt.Errorf("城镇导航预算必须显式完整且不溢出")
	}
	e.TownGrids = make(map[uint64]*townnav.Grid)
	maxCells := 0
	f := &e.Store.Cfg.Fixture
	maps := append([]devconfig.MapTemplate{f.Map}, f.Maps...)
	for _, m := range maps {
		if m.SceneKindValue() != devconfig.SceneKindTown {
			continue
		}
		version := fmt.Sprintf("%x", sha256.Sum256(m.CollisionBits))
		grid, err := townnav.NewGrid(m.ID, version, m.Width, m.Height, m.CollisionBits)
		if err != nil {
			return fmt.Errorf("城镇地图%d导航准备失败: %w", m.ID, err)
		}
		if _, exists := e.TownGrids[m.ID]; exists {
			return fmt.Errorf("城镇导航地图ID重复%d", m.ID)
		}
		e.TownGrids[m.ID] = grid
		maxCells = max(maxCells, grid.Cells())
	}
	if maxCells == 0 {
		return fmt.Errorf("启用城镇导航但没有城镇地图")
	}
	routeCells := min(cfg.MaxRouteCells, maxCells)
	frontier := min(cfg.MaxFrontier, maxCells)
	word := int64(strconv.IntSize / 8)
	reserved := int64(cfg.WorkspacesPerWorker)*(int64(maxCells)*(12+word)+int64(frontier)*word) + int64(cfg.WorkspacesPerWorker+cfg.QueuePerWorker)*int64(routeCells)*2*word
	if reserved > cfg.MaxMemoryBytesPerWorker {
		return fmt.Errorf("全部城镇最大图的导航大缓冲预留超过显式每worker预算")
	}
	for _, w := range e.workers {
		if w.townNavigation != nil {
			return fmt.Errorf("城镇导航worker不可重复初始化")
		}
		n := &townNavigationWorker{queue: make([]*townNavigationRequest, 0, cfg.QueuePerWorker), active: make([]*townNavigationRequest, 0, cfg.WorkspacesPerWorker), holders: make([]*townNavigationRequest, 0, cfg.WorkspacesPerWorker+cfg.QueuePerWorker), scratch: make([]townnav.Point, 0, cfg.MaxDynamicCells)}
		for i := 0; i < cfg.WorkspacesPerWorker; i++ {
			workspace, err := townnav.NewWorkspace(maxCells, frontier)
			if err != nil {
				return err
			}
			n.workspaces = append(n.workspaces, workspace)
		}
		for i := 0; i < cfg.WorkspacesPerWorker+cfg.QueuePerWorker; i++ {
			n.routes = append(n.routes, make([]townnav.Point, routeCells))
		}
		w.townNavigation = n
	}
	return nil
}

func townPoint(p devconfig.Point) townnav.Point { return townnav.Point{X: p.X, Y: p.Y} }
func gamePoint(p townnav.Point) devconfig.Point { return devconfig.Point{X: p.X, Y: p.Y} }

func townActorIndex(r *room, e *entity) int {
	if e == &r.Player {
		return 0
	}
	if e != nil && e == r.Pet {
		return 1
	}
	return -1
}

func (w *worker) validTownRequest(q *townNavigationRequest) bool {
	r, e := q.room, q.actor
	i := townActorIndex(r, e)
	if i < 0 || r.TownNavigation[i] != q || r.Scene != q.key.Scene || r.MapID != q.key.MapID || e.Life != q.key.Life || e.HP <= 0 || r.Isolated {
		return false
	}
	if r.Role.ID != "" && w.rooms[r.Role.ID] != r {
		return false
	}
	grid := w.e.TownGrids[r.MapID]
	if grid != q.grid || grid == nil || grid.Version() != q.key.MapVersion {
		return false
	}
	if i == 0 && (r.Manual || q.inputRevision != 0 && (r.Navigation.Revision != q.inputRevision || r.Destination == nil || *r.Destination != q.target)) {
		return false
	}
	return true
}

func removeTownRequest(items []*townNavigationRequest, q *townNavigationRequest) []*townNavigationRequest {
	for i, item := range items {
		if item == q {
			copy(items[i:], items[i+1:])
			items[len(items)-1] = nil
			return items[:len(items)-1]
		}
	}
	return items
}

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

func (w *worker) cancelTownRequest(q *townNavigationRequest) {
	if q == nil {
		return
	}
	if q.task != nil {
		q.task.Cancel()
	}
	q.state = townnav.Cancelled
	n := w.townNavigation
	if n != nil {
		n.queue = removeTownRequest(n.queue, q)
		n.active = removeTownRequest(n.active, q)
		w.releaseTownRoute(q)
	}
	q.queued = false
	for i, current := range q.room.TownNavigation {
		if current == q {
			q.room.TownNavigation[i] = nil
		}
	}
}

// cancelTownNavigation在切图、冷重建与房间移除前调用，归还所有旧scene资源。
func (w *worker) cancelTownNavigation(r *room) {
	if r == nil {
		return
	}
	for _, q := range r.TownNavigation {
		w.cancelTownRequest(q)
	}
	r.TownNavigation = [2]*townNavigationRequest{}
}

// occupancy只在实际占格内容改变时推进revision。不能用tick次数制造自动重搜。
func (w *worker) updateTownOccupancy(q *townNavigationRequest) bool {
	n := w.townNavigation
	n.scratch = n.scratch[:0]
	r := q.room
	if r.MonsterCount < 0 || r.MonsterCount > len(r.Monsters) {
		return false
	}
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
	if !add(&r.Player) || !add(r.Pet) {
		return false
	}
	for _, e := range r.Monsters[:r.MonsterCount] {
		if !add(e) {
			return false
		}
	}
	changed := q.dynamic.Revision == 0 || len(n.scratch) != len(q.dynamic.Blocked)
	if !changed {
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

func townBlocked(q *townNavigationRequest, p devconfig.Point) bool {
	for _, blocked := range q.dynamic.Blocked {
		if blocked == townPoint(p) {
			return true
		}
	}
	return false
}

func (w *worker) queueTownRequest(q *townNavigationRequest) bool {
	if q.queued {
		return true
	}
	n := w.townNavigation
	if len(n.queue) >= cap(n.queue) {
		return false
	}
	q.queued = true
	n.queue = append(n.queue, q)
	return true
}

func (w *worker) finishTownPlanning(q *townNavigationRequest) {
	q.state = q.task.State()
	if q.state == townnav.Searching || q.state == townnav.Building {
		return
	}
	n := w.townNavigation
	n.active = removeTownRequest(n.active, q)
	if q.state != townnav.Ready {
		w.releaseTownRoute(q)
		if q.state != townnav.WaitingDynamic && q.state != townnav.NeedsReplan && q.state != townnav.NoPath && !(q.state == townnav.Arrived && q.adjacent) {
			q.task = nil
		}
	}
}

func townNavigationPaused(r *room) bool {
	return r.Pending || !r.Ready || r.Conn == nil || mapTravelQuiescing(r)
}

// 暂挂只归还共享搜索/路线，不改变原经济job、地面目标或请求累计预算。
func (w *worker) suspendTownRequest(q *townNavigationRequest) bool {
	if q != nil && q.task != nil && q.task.Suspend(q.key) {
		q.state = townnav.NeedsReplan
		w.townNavigation.active = removeTownRequest(w.townNavigation.active, q)
		w.releaseTownRoute(q)
		return true
	}
	return false
}

func (w *worker) admitTownRequests() {
	n := w.townNavigation
	attempts := len(n.queue)
	for attempts > 0 && len(n.queue) > 0 && len(n.routes) > 0 {
		attempts--
		var workspace *townnav.Workspace
		for _, candidate := range n.workspaces {
			if !candidate.InUse() {
				workspace = candidate
				break
			}
		}
		if workspace == nil {
			return
		}
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
		cfg := w.e.Store.Cfg.Fixture.TownNavigation
		limits := townnav.Limits{MaxExpanded: cfg.MaxExpanded, MaxFrontier: min(cfg.MaxFrontier, q.grid.Cells()), MaxRouteCells: min(cfg.MaxRouteCells, q.grid.Cells()), MaxReplans: cfg.MaxReplans, MaxDynamicCells: min(cfg.MaxDynamicCells, q.grid.Cells())}
		var err error
		m, known := w.currentMap(q.room)
		blockedStart := known && !m.Walkable(q.actor.Pos) && m.TeleportWalkable(q.actor.Pos)
		if q.task == nil {
			q.adjacent = townBlocked(q, q.target)
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
func (w *worker) beginTownNavigationTick(now time.Time) {
	n := w.townNavigation
	if n == nil {
		return
	}
	n.tick = now
	n.used = 0
	n.remaining = w.e.Store.Cfg.Fixture.TownNavigation.WorkPerWorkerTick
	for i := 0; i < len(n.holders); {
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
	for i := 0; i < len(n.active); {
		q := n.active[i]
		if !w.validTownRequest(q) {
			w.cancelTownRequest(q)
		} else {
			i++
		}
	}
	for i := 0; i < len(n.queue); {
		q := n.queue[i]
		if !w.validTownRequest(q) {
			w.cancelTownRequest(q)
		} else {
			i++
		}
	}
	w.advanceTownNavigation()
}

func (w *worker) advanceTownNavigation() {
	n := w.townNavigation
	if n == nil {
		return
	}
	// 即使本tick额度已用完，新进入收束/冻结也立即让出共享池。
	for i := 0; i < len(n.holders); {
		q := n.holders[i]
		if townNavigationPaused(q.room) && w.suspendTownRequest(q) {
			continue
		}
		i++
	}
	w.admitTownRequests()
	for n.remaining > 0 && len(n.active) > 0 {
		progressed := false
		count := len(n.active)
		for visited := 0; visited < count && n.remaining > 0 && len(n.active) > 0; visited++ {
			if n.cursor >= len(n.active) {
				n.cursor = 0
			}
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
			budget := min(w.e.Store.Cfg.Fixture.TownNavigation.WorkPerTask, n.remaining)
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
func (w *worker) endTownNavigationTick() { w.advanceTownNavigation() }

// moveTown返回true表示城镇已接管，包括禁用或无工作区时；不得退回全图同步BFS。
func (w *worker) moveTown(r *room, e *entity, target devconfig.Point, now time.Time, interval int64) bool {
	m, ok := w.currentMap(r)
	if !ok || m.SceneKindValue() != devconfig.SceneKindTown {
		return false
	}
	i := townActorIndex(r, e)
	if i < 0 {
		return true
	}
	cfg := w.e.Store.Cfg.Fixture.TownNavigation
	n := w.townNavigation
	if cfg == nil || !cfg.Enabled || n == nil {
		w.cancelTownRequest(r.TownNavigation[i])
		return true
	}
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
	point := gamePoint(next)
	if !w.neighbor(r, e.Pos, point, e) {
		return true
	}
	from := e.Pos
	direction := e.Direction
	for i, d := range directions {
		if point.X-from.X == d.X && point.Y-from.Y == d.Y {
			direction = uint32(i)
			break
		}
	}
	if !w.commitWalk(r, e, point, direction, now, interval) {
		return true
	}
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
