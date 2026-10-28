package game

import (
	"fmt"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	"minimir/internal/persistence"
	"minimir/internal/townnav"
	"testing"
	"time"
)

func navigationTownWorker(t *testing.T, width, height, quantum, total, workspaces, queue int, blocked ...devconfig.Point) (*worker, *room) {
	t.Helper()
	bits := make([]byte, (width*height+7)/8)
	for _, p := range blocked {
		i := p.Y*width + p.X
		bits[i/8] |= 1 << uint(i%8)
	}
	fixture := devconfig.Fixture{RuleVersion: 13, Map: devconfig.MapTemplate{ID: 1, Width: 17, Height: 17}, Maps: []devconfig.MapTemplate{{ID: 2, SceneKind: devconfig.SceneKindTown, Width: width, Height: height, CollisionBits: bits}}, TownTravel: &devconfig.TownTravelConfig{TownMapID: 2, PlanTTLMS: 1000, QuiesceTTLMS: 500, AckTTLMS: 1000, MaxPlansPerScene: 4}, TownNavigation: &devconfig.TownNavigationConfig{Enabled: true, WorkPerTask: quantum, WorkPerWorkerTick: total, WorkspacesPerWorker: workspaces, QueuePerWorker: queue, MaxExpanded: width * height, MaxFrontier: min(width*height, 65536), MaxRouteCells: min(width*height, 4096), MaxReplans: 3, MaxDynamicCells: 2, MaxMemoryBytesPerWorker: 33554432}}
	e := &Engine{Store: &persistence.Store{Cfg: devconfig.Config{Fixture: fixture}}}
	w := &worker{e: e, rooms: make(map[string]*room)}
	e.workers = []*worker{w}
	if err := e.initializeTownNavigation(); err != nil {
		t.Fatal(err)
	}
	r := addNavigationTownRoom(w, "a", devconfig.Point{X: 1, Y: 1})
	return w, r
}

func addNavigationTownRoom(w *worker, id string, pos devconfig.Point) *room {
	r := &room{Role: domain.Role{ID: id}, Conn: &capturedConnection{}, Ready: true, MapID: 2, Scene: 1, Player: entity{Handle: 1, Kind: 1, HP: 10, MaxHP: 10, Pos: pos}}
	w.rooms[id] = r
	return r
}

func requestTownGround(r *room, target devconfig.Point) {
	r.Destination = &target
	r.Navigation = navigationState{Revision: r.Navigation.Revision + 1, InputSequence: 1, Status: navigationActive, Destination: &target}
}

func navigationTownTick(w *worker, r *room, target devconfig.Point, now time.Time) {
	w.beginTownNavigationTick(now)
	w.moveTown(r, &r.Player, target, now, 1)
	w.endTownNavigationTick()
}

func TestTownNavigationSharesWorkerBudgetAndQueuesFairly(t *testing.T) {
	w, a := navigationTownWorker(t, 32, 32, 2, 4, 2, 3)
	b := addNavigationTownRoom(w, "b", devconfig.Point{X: 1, Y: 2})
	c := addNavigationTownRoom(w, "c", devconfig.Point{X: 1, Y: 3})
	now := time.Unix(1, 0)
	for _, r := range []*room{a, b, c} {
		target := devconfig.Point{X: 30, Y: r.Player.Pos.Y}
		requestTownGround(r, target)
		w.moveTown(r, &r.Player, target, now, 1)
	}
	w.beginTownNavigationTick(now)
	n := w.townNavigation
	if n.used != 4 || n.remaining != 0 || len(n.active) != 2 || len(n.queue) != 1 {
		t.Fatalf("不是worker共享总预算: used=%d remaining=%d active=%d queue=%d", n.used, n.remaining, len(n.active), len(n.queue))
	}
	for _, r := range []*room{a, b} {
		if got := r.TownNavigation[0].task.Progress().TotalExpanded; got != 2 {
			t.Fatalf("活跃任务未公平分量子: %d", got)
		}
	}
	w.endTownNavigationTick()
	if n.used != 4 {
		t.Fatal("tick后半重置了总预算")
	}
	w.cancelTownNavigation(a)
	w.endTownNavigationTick()
	if c.TownNavigation[0].task == nil || c.TownNavigation[0].task.State() != townnav.Searching {
		t.Fatal("FIFO等待者未借到归还工作区")
	}
	w.beginTownNavigationTick(now.Add(time.Second))
	if n.used != 4 || c.TownNavigation[0].task.Progress().TotalExpanded != 2 {
		t.Fatal("新获工作区的任务未公平获得预算")
	}
}

func TestTownNavigationStaticNoPathMemoReturnsBuffersAndNeverRepeats(t *testing.T) {
	var blocked []devconfig.Point
	for y := 0; y < 8; y++ {
		blocked = append(blocked, devconfig.Point{X: 4, Y: y})
	}
	w, r := navigationTownWorker(t, 8, 8, 4, 8, 1, 2, blocked...)
	target := devconfig.Point{X: 6, Y: 1}
	requestTownGround(r, target)
	now := time.Unix(1, 0)
	navigationTownTick(w, r, target, now)
	q := r.TownNavigation[0]
	if q == nil || q.state != townnav.NoPath || q.storage != nil || len(w.townNavigation.routes) != 3 || w.townNavigation.workspaces[0].InUse() {
		t.Fatal("静态无路未保存小终态或归还全部大缓冲")
	}
	sequence := w.townNavigation.sequence
	for i := 0; i < 50; i++ {
		navigationTownTick(w, r, target, now.Add(time.Duration(i+1)*time.Second))
		if w.townNavigation.used != 0 || r.TownNavigation[0] != q || w.townNavigation.sequence != sequence {
			t.Fatal("未变目标的静态无路每tick重建")
		}
	}
}

func TestTownNavigationOccupiedGroundConvergesButRetainsOriginalRequest(t *testing.T) {
	w, r := navigationTownWorker(t, 16, 16, 128, 256, 1, 2)
	target := devconfig.Point{X: 6, Y: 1}
	r.Pet = &entity{Handle: 2, Kind: 2, HP: 10, Pos: target}
	requestTownGround(r, target)
	now := time.Unix(1, 0)
	for i := 0; i < 8; i++ {
		navigationTownTick(w, r, target, now.Add(time.Duration(i)*time.Second))
	}
	q := r.TownNavigation[0]
	if distance(r.Player.Pos, target) != 1 || r.Player.Pos == target || r.Navigation.Status != navigationActive || r.Destination == nil || *r.Destination != target || q.state != townnav.Arrived || !q.adjacent {
		t.Fatalf("占格收敛伪报到达或丢原目标: player=%v nav=%+v q=%+v", r.Player.Pos, r.Navigation, q)
	}
	before := q.task.Progress().TotalExpanded
	r.Pet.Pos = devconfig.Point{X: 8, Y: 3}
	navigationTownTick(w, r, target, now.Add(9*time.Second))
	if r.TownNavigation[0] != q || q.task == nil || q.task.Progress().Replans != 1 || q.task.Progress().TotalExpanded < before {
		t.Fatal("宠物移开后另起请求或重置累计预算")
	}
	navigationTownTick(w, r, target, now.Add(10*time.Second))
	if r.Player.Pos != target || r.Navigation.Destination == nil || *r.Navigation.Destination != target {
		t.Fatal("宠物移开后未走到原精确目标")
	}
}

func TestTownNavigationPetStopsBesideOwnerWithoutEnteringOwnerCell(t *testing.T) {
	w, r := navigationTownWorker(t, 16, 16, 128, 256, 1, 2)
	r.Player.Pos = devconfig.Point{X: 7, Y: 1}
	r.Pet = &entity{Handle: 2, Kind: 2, HP: 10, Pos: devconfig.Point{X: 1, Y: 1}}
	now := time.Unix(1, 0)
	for i := 0; i < 10; i++ {
		at := now.Add(time.Duration(i) * time.Second)
		w.beginTownNavigationTick(at)
		w.moveTown(r, r.Pet, r.Player.Pos, at, 1)
		w.endTownNavigationTick()
	}
	if distance(r.Pet.Pos, r.Player.Pos) != 1 || r.Pet.Pos == r.Player.Pos || r.TownNavigation[1].state != townnav.Arrived {
		t.Fatal("宠物未停在主人合法邻格")
	}
	for i := 0; i < 5; i++ {
		at := now.Add(time.Duration(i+11) * time.Second)
		w.beginTownNavigationTick(at)
		w.moveTown(r, r.Pet, r.Player.Pos, at, 1)
		w.endTownNavigationTick()
		if w.townNavigation.used != 0 {
			t.Fatal("宠物已邻接仍重复整图搜索")
		}
	}
}

func TestTownNavigationSceneLifeAndInputChangesCancelBeforeAdvancing(t *testing.T) {
	for _, change := range []string{"scene", "map", "life", "manual", "revision", "removed", "dead", "explicit"} {
		t.Run(change, func(t *testing.T) {
			w, r := navigationTownWorker(t, 32, 32, 1, 1, 1, 2)
			target := devconfig.Point{X: 30, Y: 30}
			requestTownGround(r, target)
			now := time.Unix(1, 0)
			navigationTownTick(w, r, target, now)
			if !w.townNavigation.workspaces[0].InUse() {
				t.Fatal("未建立跨tick活跃搜索")
			}
			switch change {
			case "scene":
				r.Scene++
			case "map":
				r.MapID = 1
			case "life":
				r.Player.Life++
			case "manual":
				r.Manual = true
			case "revision":
				r.Navigation.Revision++
			case "removed":
				delete(w.rooms, r.Role.ID)
			case "dead":
				r.Player.HP = 0
			case "explicit":
				w.cancelTownNavigation(r)
			}
			w.beginTownNavigationTick(now.Add(time.Second))
			if r.TownNavigation[0] != nil || w.townNavigation.used != 0 || w.townNavigation.workspaces[0].InUse() || len(w.townNavigation.routes) != 3 {
				t.Fatal("旧身份继续展开或未归还共享资源")
			}
		})
	}
}

func TestTownNavigationReadyRouteReleasesWorkspaceForNextRoom(t *testing.T) {
	w, a := navigationTownWorker(t, 16, 16, 128, 256, 1, 2)
	b := addNavigationTownRoom(w, "b", devconfig.Point{X: 1, Y: 2})
	now := time.Unix(1, 0)
	for _, r := range []*room{a, b} {
		target := devconfig.Point{X: 10, Y: r.Player.Pos.Y}
		requestTownGround(r, target)
		w.moveTown(r, &r.Player, target, now, 1)
	}
	w.beginTownNavigationTick(now)
	if a.TownNavigation[0].state != townnav.Ready || b.TownNavigation[0].state != townnav.Ready || w.townNavigation.workspaces[0].InUse() {
		t.Fatal("Ready路线仍绑定全图workspace，阻止下一房间")
	}
	if len(w.townNavigation.holders) != 2 || len(w.townNavigation.routes) != 1 {
		t.Fatal("独立路线池租约数量不实")
	}
	for _, r := range []*room{a, b} {
		before := r.Player.Pos
		w.moveTown(r, &r.Player, *r.Destination, now, 100)
		if r.Player.Pos == before {
			t.Fatal("工作区复用后旧路线被覆盖")
		}
		pos := r.Player.Pos
		w.moveTown(r, &r.Player, *r.Destination, now.Add(time.Millisecond), 100)
		if r.Player.Pos != pos {
			t.Fatal("路线绕过NextMove/动作截止")
		}
	}
	w.cancelTownNavigation(a)
	w.cancelTownNavigation(b)
	if len(w.townNavigation.routes) != 3 || len(w.townNavigation.holders) != 0 {
		t.Fatal("路线取消后未归还有界池")
	}
}

func TestTownNavigationUnknownPendingPausesWithoutBlockingOtherOwners(t *testing.T) {
	w, a := navigationTownWorker(t, 32, 32, 1, 1, 1, 2)
	b := addNavigationTownRoom(w, "b", devconfig.Point{X: 1, Y: 2})
	now := time.Unix(1, 0)
	for _, r := range []*room{a, b} {
		target := devconfig.Point{X: 30, Y: r.Player.Pos.Y}
		requestTownGround(r, target)
		w.moveTown(r, &r.Player, target, now, 1)
	}
	w.beginTownNavigationTick(now)
	q := a.TownNavigation[0]
	prior := q.task.Progress().TotalExpanded
	a.Pending = true
	w.beginTownNavigationTick(now.Add(time.Second))
	if q.state != townnav.NeedsReplan || q.storage != nil || b.TownNavigation[0].task == nil || b.TownNavigation[0].task.Progress().TotalExpanded != 1 {
		t.Fatal("未知事务冻结占住全worker唯一搜索区")
	}
	w.cancelTownNavigation(b)
	a.Pending = false
	w.moveTown(a, &a.Player, *a.Destination, now.Add(2*time.Second), 1)
	w.beginTownNavigationTick(now.Add(2 * time.Second))
	if q.task.Progress().Replans != 0 || q.task.Progress().TotalExpanded != prior+1 {
		t.Fatal("事务恢复重置同请求累计预算")
	}
}

func TestTownNavigationRepeatedUnchangedPausesKeepGroundTargetAndBudgets(t *testing.T) {
	for _, reason := range []string{"pending", "not_ready", "quiescing"} {
		t.Run(reason, func(t *testing.T) {
			w, r := navigationTownWorker(t, 32, 32, 128, 256, 1, 2)
			now := time.Unix(1, 0)
			target := devconfig.Point{X: 10, Y: 1}
			requestTownGround(r, target)
			r.Player.NextMove = now.Add(24 * time.Hour)
			w.move(r, &r.Player, target, now, 1)
			w.beginTownNavigationTick(now)
			q := r.TownNavigation[0]
			if q == nil || q.state != townnav.Ready {
				t.Fatal("未建立原地面路线")
			}
			revision, start := r.Navigation.Revision, r.Player.Pos
			prior := q.task.Progress().TotalExpanded
			job := &job{}
			r.PendingRetry = job
			for i := 0; i < 4; i++ {
				at := now.Add(time.Duration(i+1) * time.Second)
				switch reason {
				case "pending":
					r.Pending = true
				case "not_ready":
					r.Ready = false
				case "quiescing":
					r.Travel = &mapTravelPlan{Phase: travelQuiescing}
				}
				w.beginTownNavigationTick(at)
				w.move(r, &r.Player, target, at, 1)
				w.endTownNavigationTick()
				if q.storage != nil || w.townNavigation.workspaces[0].InUse() || w.townNavigation.used != 0 || r.PendingRetry != job {
					t.Fatal("暂停未归还共享池、继续展开或改变原经济job")
				}
				r.Pending, r.Ready, r.Travel = false, true, nil
				w.move(r, &r.Player, target, at, 1)
				w.endTownNavigationTick()
				w.move(r, &r.Player, target, at, 1)
				if r.TownNavigation[0] != q || r.Destination == nil || *r.Destination != target || r.Navigation.Status != navigationActive || r.Navigation.Revision != revision || r.Player.Pos != start || q.state != townnav.Ready {
					t.Fatalf("第%d次合法暂停恢复清除/替换原地面目标: nav=%+v state=%s", i+1, r.Navigation, q.state)
				}
				progress := q.task.Progress()
				if progress.Replans != 0 || progress.TotalExpanded <= prior || w.townNavigation.used <= 0 || w.townNavigation.used > 256 {
					t.Fatal("纯暂停计入实际重规划或重置了累计/worker工作额度", progress)
				}
				prior = progress.TotalExpanded
			}
		})
	}
}

func TestTownNavigationQuiescingReturnsPoolWithoutAdmittingOrAdvancing(t *testing.T) {
	w, a := navigationTownWorker(t, 32, 32, 2, 4, 1, 2)
	b := addNavigationTownRoom(w, "b", devconfig.Point{X: 1, Y: 2})
	now := time.Unix(1, 0)
	for _, r := range []*room{a, b} {
		target := devconfig.Point{X: 30, Y: r.Player.Pos.Y}
		requestTownGround(r, target)
		w.move(r, &r.Player, target, now, 1)
	}
	w.beginTownNavigationTick(now)
	if w.townNavigation.remaining != 0 || !w.townNavigation.workspaces[0].InUse() {
		t.Fatal("未建立耗尽tick额度的活跃搜索")
	}
	prior := a.TownNavigation[0].task.Progress().TotalExpanded
	a.Travel, b.Travel = &mapTravelPlan{Phase: travelQuiescing}, &mapTravelPlan{Phase: travelQuiescing}
	w.endTownNavigationTick()
	if a.TownNavigation[0].storage != nil || b.TownNavigation[0].task != nil || w.townNavigation.workspaces[0].InUse() || len(w.townNavigation.routes) != 3 || a.TownNavigation[0].task.Progress().TotalExpanded != prior {
		t.Fatal("收束仍持有大缓冲、启动排队搜索或继续展开")
	}
	w.beginTownNavigationTick(now.Add(time.Second))
	w.endTownNavigationTick()
	if w.townNavigation.used != 0 || a.Destination == nil || b.Destination == nil {
		t.Fatal("收束期间规划了新工作或清掉原目标")
	}
}

func TestTownNavigationQueueAndMemoryCapsAreExplicit(t *testing.T) {
	w, r := navigationTownWorker(t, 16, 16, 1, 1, 1, 1)
	for i := 0; i < 5; i++ {
		actor := r
		if i > 0 {
			actor = addNavigationTownRoom(w, fmt.Sprint(i), devconfig.Point{X: 1, Y: 1})
		}
		target := devconfig.Point{X: 15, Y: 15}
		requestTownGround(actor, target)
		w.moveTown(actor, &actor.Player, target, time.Unix(1, 0), 1)
	}
	if len(w.townNavigation.queue) != 1 || cap(w.townNavigation.queue) != 1 || len(w.townNavigation.workspaces) != 1 || len(w.townNavigation.routes) != 2 {
		t.Fatal("队列压力扩大了预分配池")
	}
	w.e.Store.Cfg.Fixture.TownNavigation.MaxMemoryBytesPerWorker = 1
	w.townNavigation = nil
	if err := w.e.initializeTownNavigation(); err == nil {
		t.Fatal("缺少显式内存预算仍启动")
	}
}

func TestTownNavigationDisabledNeverFallsBackToWholeTownBFS(t *testing.T) {
	w, r := navigationTownWorker(t, 16, 16, 1, 1, 1, 1)
	w.e.Store.Cfg.Fixture.TownNavigation.Enabled = false
	before := r.Player.Pos
	if !w.moveTown(r, &r.Player, devconfig.Point{X: 2, Y: 1}, time.Unix(1, 0), 1) || r.Player.Pos != before {
		t.Fatal("城镇停用时退回同步寻路")
	}
	r.MapID = 1
	if w.moveTown(r, &r.Player, devconfig.Point{X: 2, Y: 1}, time.Unix(1, 0), 1) {
		t.Fatal("新算法接管个人图原路径")
	}
	r.MapID = 999
	w.move(r, &r.Player, devconfig.Point{X: 2, Y: 1}, time.Unix(1, 0), 1)
	if r.Player.Pos != before {
		t.Fatal("未知地图静默回到primary")
	}
}

func TestTownNavigationForcedRelocationReconsidersStaticTerminalStart(t *testing.T) {
	var walls []devconfig.Point
	for y := 0; y < 8; y++ {
		walls = append(walls, devconfig.Point{X: 4, Y: y})
	}
	w, r := navigationTownWorker(t, 8, 8, 32, 64, 1, 2, walls...)
	target := devconfig.Point{X: 6, Y: 1}
	requestTownGround(r, target)
	now := time.Unix(1, 0)
	navigationTownTick(w, r, target, now)
	q := r.TownNavigation[0]
	if q.state != townnav.NoPath {
		t.Fatal("未建立静态无路终态")
	}
	r.Player.Pos = devconfig.Point{X: 5, Y: 1}
	navigationTownTick(w, r, target, now.Add(time.Second))
	if r.TownNavigation[0] != q || q.task == nil || q.task.Progress().Replans != 1 || q.state != townnav.Ready {
		t.Fatal("强制位移后保留失效无路结论，或换请求重置预算")
	}
	navigationTownTick(w, r, target, now.Add(2*time.Second))
	if r.Player.Pos != target {
		t.Fatal("换到可达分量后未继续原地面目标")
	}
}

func TestTownNavigationDynamicNextStepCannotBypassReplanCap(t *testing.T) {
	w, r := navigationTownWorker(t, 16, 16, 128, 256, 1, 2)
	w.e.Store.Cfg.Fixture.TownNavigation.MaxReplans = 0
	target := devconfig.Point{X: 10, Y: 1}
	requestTownGround(r, target)
	now := time.Unix(1, 0)
	navigationTownTick(w, r, target, now)
	q := r.TownNavigation[0]
	next, ok, err := q.task.Next(q.key, townPoint(r.Player.Pos), q.dynamic)
	if err != nil || !ok {
		t.Fatal("未准备就绪路线", err)
	}
	r.Pet = &entity{Handle: 2, Kind: 2, HP: 10, Pos: gamePoint(next)}
	before := r.Player.Pos
	navigationTownTick(w, r, target, now.Add(time.Second))
	if r.Player.Pos != before || q.state != townnav.Limited || q.task != nil || len(w.townNavigation.routes) != 3 {
		t.Fatal("动态阻挡进入占格、绕过零重规划上限或泄漏路线")
	}
	for i := 0; i < 3; i++ {
		navigationTownTick(w, r, target, now.Add(time.Duration(i+2)*time.Second))
		if w.townNavigation.used != 0 || r.TownNavigation[0] != q {
			t.Fatal("限额终态每tick另起搜索")
		}
	}
}

func TestTownNavigationFullTownCoordinatesRemainWithinPerTickBudget(t *testing.T) {
	w, r := navigationTownWorker(t, 700, 700, 1, 1, 1, 2)
	r.Player.Pos = devconfig.Point{X: 633, Y: 613}
	target := devconfig.Point{X: 633, Y: 614}
	requestTownGround(r, target)
	now := time.Unix(1, 0)
	for i := 0; i < 8 && r.Player.Pos != target; i++ {
		navigationTownTick(w, r, target, now.Add(time.Duration(i)*time.Second))
		if w.townNavigation.used > 1 {
			t.Fatal("源700图坐标触发越过worker本轮展开预算")
		}
	}
	if r.Player.Pos != target || w.e.TownGrids[2].Cells() != 490000 {
		t.Fatal("来源Town坐标被个人图裁剪或未能增量完成")
	}
}
