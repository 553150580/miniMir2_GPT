// 本文件回归城镇寻路的分片预算、队列公平、工作区归还、动态障碍与暂停恢复。
package game

import (
	"fmt"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	"minimir/internal/persistence"
	"minimir/internal/townnav"
	"slices"
	"testing"
	"time"
)

// navigationTownWorker 处理导航城镇房间执行器。
// t 提供断言失败报告、子用例和清理注册，width 记录宽度，使用数值表示，height 记录高度，使用数值表示，quantum 记录“quantum”，使用数值表示。
// total 记录合计，使用数值表示，workspaces 记录工作区集合，使用数值表示，queue 记录队列，使用数值表示，blocked 记录阻挡，使用可变数量的点位表示。
func navigationTownWorker(t *testing.T, width, height, quantum, total, workspaces, queue int, blocked ...devconfig.Point) (*worker, *room) {
	t.Helper()
	// bits 创建用于维护二进制位的容器或通道。
	bits := make([]byte, (width*height+7)/8)
	// p 接收遍历阻挡时当前的元素。
	for _, p := range blocked {
		// i 保存根据纵坐标与宽度的计算结果与横坐标计算的当前位置的下标。
		i := p.Y*width + p.X
		bits[i/8] |= 1 << uint(i%8)
	}
	// fixture 组装开发测试配置所需的数据。
	fixture := devconfig.Fixture{RuleVersion: 13, Map: devconfig.MapTemplate{ID: 1, Width: 17, Height: 17}, Maps: []devconfig.MapTemplate{{ID: 2, SceneKind: devconfig.SceneKindTown, Width: width, Height: height, CollisionBits: bits}}, TownTravel: &devconfig.TownTravelConfig{TownMapID: 2, PlanTTLMS: 1000, QuiesceTTLMS: 500, AckTTLMS: 1000, MaxPlansPerScene: 4}, TownNavigation: &devconfig.TownNavigationConfig{Enabled: true, WorkPerTask: quantum, WorkPerWorkerTick: total, WorkspacesPerWorker: workspaces, QueuePerWorker: queue, MaxExpanded: width * height, MaxFrontier: min(width*height, 65536), MaxRouteCells: min(width*height, 4096), MaxReplans: 3, MaxDynamicCells: 2, MaxMemoryBytesPerWorker: 33554432}}
	// e 保存当前处理所用的游戏引擎。
	e := &Engine{Store: &persistence.Store{Cfg: devconfig.Config{Fixture: fixture}}}
	// w 保存当前处理所用的房间执行器。
	w := &worker{e: e, rooms: make(map[string]*room)}
	e.workers = []*worker{w}
	// err 接收“initialize”城镇导航过程中可能产生的错误，供后续失败分支处理。
	if err := e.initializeTownNavigation(); err != nil {
		t.Fatal(err)
	}
	// r 接收新增导航城镇房间供后续步骤使用。
	r := addNavigationTownRoom(w, "a", devconfig.Point{X: 1, Y: 1})
	return w, r
}

// addNavigationTownRoom 增加导航城镇房间。
// w 记录房间执行器，id 区分当前步骤指向的目标对象，pos 记录位置，使用点位表示。
func addNavigationTownRoom(w *worker, id string, pos devconfig.Point) *room {
	// r 保存当前处理所用的房间。
	r := &room{Role: domain.Role{ID: id}, Conn: &capturedConnection{}, Ready: true, Scene: 1, Player: entity{Handle: 1, Kind: 1, HP: 10, MaxHP: 10, Pos: pos}, worldState: &worldState{MapID: 2}}
	w.rooms[id] = r
	return r
}

// requestTownGround 处理请求城镇地面。
// r 指向当前执行器拥有并更新的目标房间，target 记录目标，使用点位表示。
func requestTownGround(r *room, target devconfig.Point) {
	r.Destination = &target
	r.Navigation = navigationState{Revision: r.Navigation.Revision + 1, InputSequence: 1, Status: navigationActive, Destination: &target}
}

// navigationTownTick 处理导航城镇模拟帧。
// w 记录房间执行器，r 指向当前执行器拥有并更新的目标房间，target 记录目标，使用点位表示，now 提供当前时点，作为状态有效期和计时判断的基准。
func navigationTownTick(w *worker, r *room, target devconfig.Point, now time.Time) {
	w.beginTownNavigationTick(now)
	w.moveTown(r, &r.Player, target, now, 1)
	w.endTownNavigationTick()
}

// TestTownNavigationSharesWorkerBudgetAndQueuesFairly 核对「FIFO等待者未借到归还工作区」与「新获工作区的任务未公平获得预算」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestTownNavigationSharesWorkerBudgetAndQueuesFairly(t *testing.T) {
	// w 接收导航城镇房间执行器供后续步骤使用，a 保存由导航城镇房间执行器取得的a。
	w, a := navigationTownWorker(t, 32, 32, 2, 4, 2, 3)
	// b 保存新增导航城镇房间返回的结果。
	b := addNavigationTownRoom(w, "b", devconfig.Point{X: 1, Y: 2})
	// c 接收新增导航城镇房间供后续步骤使用。
	c := addNavigationTownRoom(w, "c", devconfig.Point{X: 1, Y: 3})
	// now 保存由Unix 时间取得的当前时间。
	now := time.Unix(1, 0)
	// r 接收遍历房间列表时当前的元素。
	for _, r := range []*room{a, b, c} {
		// target 初始化用作目标的点位。
		target := devconfig.Point{X: 30, Y: r.Player.Pos.Y}
		requestTownGround(r, target)
		w.moveTown(r, &r.Player, target, now, 1)
	}
	w.beginTownNavigationTick(now)
	// n 保存当前处理所用的城镇导航。
	n := w.townNavigation
	if n.used != 4 || n.remaining != 0 || len(n.active) != 2 || len(n.queue) != 1 {
		t.Fatalf("不是worker共享总预算: used=%d remaining=%d active=%d queue=%d", n.used, n.remaining, len(n.active), len(n.queue))
	}
	// r 接收遍历房间列表时当前的元素。
	for _, r := range []*room{a, b} {
		// got 保存合计值“expanded”，用于当前步骤中的实际结果。
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

// TestTownNavigationStaticNoPathMemoReturnsBuffersAndNeverRepeats 核对「静态无路未保存小终态或归还全部大缓冲」与「未变目标的静态无路每tick重建」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestTownNavigationStaticNoPathMemoReturnsBuffersAndNeverRepeats(t *testing.T) {
	// blocked 记录阻挡，使用点位列表表示。
	var blocked []devconfig.Point
	// y 规定纵坐标的初始取值。
	for y := 0; y < 8; y++ {
		blocked = append(blocked, devconfig.Point{X: 4, Y: y})
	}
	// w 接收导航城镇房间执行器供后续步骤使用，r 接收导航城镇房间执行器供后续步骤使用。
	w, r := navigationTownWorker(t, 8, 8, 4, 8, 1, 2, blocked...)
	// target 初始化用作目标的点位。
	target := devconfig.Point{X: 6, Y: 1}
	requestTownGround(r, target)
	// now 保存由Unix 时间取得的当前时间。
	now := time.Unix(1, 0)
	navigationTownTick(w, r, target, now)
	// q 保存从城镇导航取出的城镇导航中的指定项。
	q := r.TownNavigation[0]
	if q == nil || q.state != townnav.NoPath || q.storage != nil || len(w.townNavigation.routes) != 3 || w.townNavigation.workspaces[0].InUse() {
		t.Fatal("静态无路未保存小终态或归还全部大缓冲")
	}
	// sequence 保存当前处理所用的序号。
	sequence := w.townNavigation.sequence
	// i 标记循环或列表中的当前位置。
	for i := 0; i < 50; i++ {
		navigationTownTick(w, r, target, now.Add(time.Duration(i+1)*time.Second))
		if w.townNavigation.used != 0 || r.TownNavigation[0] != q || w.townNavigation.sequence != sequence {
			t.Fatal("未变目标的静态无路每tick重建")
		}
	}
}

// TestTownNavigationOccupiedGroundConvergesButRetainsOriginalRequest 核对「宠物移开后另起请求或重置累计预算」与「占格收敛伪报到达或丢原目标」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestTownNavigationOccupiedGroundConvergesButRetainsOriginalRequest(t *testing.T) {
	// w 接收导航城镇房间执行器供后续步骤使用，r 接收导航城镇房间执行器供后续步骤使用。
	w, r := navigationTownWorker(t, 16, 16, 128, 256, 1, 2)
	// target 初始化用作目标的点位。
	target := devconfig.Point{X: 6, Y: 1}
	r.Pet = &entity{Handle: 2, Kind: 2, HP: 10, Pos: target}
	requestTownGround(r, target)
	// now 保存由Unix 时间取得的当前时间。
	now := time.Unix(1, 0)
	// i 标记循环或列表中的当前位置。
	for i := 0; i < 8; i++ {
		navigationTownTick(w, r, target, now.Add(time.Duration(i)*time.Second))
	}
	// q 保存从城镇导航取出的城镇导航中的指定项。
	q := r.TownNavigation[0]
	if distance(r.Player.Pos, target) != 1 || r.Player.Pos == target || r.Navigation.Status != navigationActive || r.Destination == nil || *r.Destination != target || q.state != townnav.Arrived || !q.adjacent {
		t.Fatalf("占格收敛伪报到达或丢原目标: player=%v nav=%+v q=%+v", r.Player.Pos, r.Navigation, q)
	}
	// before 保存合计值“expanded”，用于当前步骤中的操作前的状态。
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

// TestTownNavigationReplansWhenTargetOpensBeforeArrival 回归银杏山谷出生点附近的 NPC 拐角。
func TestTownNavigationReplansWhenTargetOpensBeforeArrival(t *testing.T) {
	walls := []devconfig.Point{{X: 4, Y: 5}, {X: 8, Y: 4}, {X: 12, Y: 5}, {X: 4, Y: 9}, {X: 12, Y: 9}, {X: 6, Y: 9}}
	want := []devconfig.Point{{X: 7, Y: 12}, {X: 6, Y: 11}, {X: 5, Y: 10}, {X: 5, Y: 9}}
	target := want[len(want)-1]
	for _, phase := range []townnav.State{townnav.Searching, townnav.Building, townnav.Ready} {
		t.Run(string(phase), func(t *testing.T) {
			w, r := navigationTownWorker(t, 17, 17, 1, 1, 1, 2, walls...)
			r.Player.Pos = devconfig.Point{X: 8, Y: 13}
			r.Pet = &entity{Handle: 2, Kind: 2, HP: 10, Pos: target}
			requestTownGround(r, target)
			now := time.Unix(1, 0)
			navigationTownTick(w, r, target, now)
			request := r.TownNavigation[0]
			for frame := 0; frame < 20 && request.state != phase; frame++ {
				now = now.Add(time.Second)
				w.beginTownNavigationTick(now)
			}
			if request.state != phase {
				t.Fatalf("未准备好指定的旧邻格路线阶段：%s", request.state)
			}
			priorExpanded := request.task.Progress().TotalExpanded
			r.Pet.Pos = devconfig.Point{X: 1, Y: 1}
			var got []devconfig.Point
			var retainedBudget bool
			for frame := 0; frame < 36 && r.Player.Pos != target; frame++ {
				now = now.Add(time.Second)
				from := r.Player.Pos
				navigationTownTick(w, r, target, now)
				if r.Player.Pos != from {
					got = append(got, r.Player.Pos)
				}
				if r.TownNavigation[0] != request || r.Navigation.Revision != request.inputRevision || w.townNavigation.used > 1 {
					t.Fatal("恢复原目标时更换了导航身份或超出该帧预算")
				}
				if request.task != nil && request.task.Progress().Replans == 1 && request.task.Progress().TotalExpanded >= priorExpanded {
					retainedBudget = true
				}
			}
			if !slices.Equal(got, want) || !retainedBudget {
				t.Fatalf("目标腾空后未及时走原目标的四步最短路或未保留预算：实际 %v，预期 %v，预算保留 %v", got, want, retainedBudget)
			}
		})
	}
}

// TestTownNavigationPetStopsBesideOwnerWithoutEnteringOwnerCell 核对「宠物未停在主人合法邻格」与「宠物已邻接仍重复整图搜索」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestTownNavigationPetStopsBesideOwnerWithoutEnteringOwnerCell(t *testing.T) {
	// w 接收导航城镇房间执行器供后续步骤使用，r 接收导航城镇房间执行器供后续步骤使用。
	w, r := navigationTownWorker(t, 16, 16, 128, 256, 1, 2)
	r.Player.Pos = devconfig.Point{X: 7, Y: 1}
	r.Pet = &entity{Handle: 2, Kind: 2, HP: 10, Pos: devconfig.Point{X: 1, Y: 1}}
	// now 保存由Unix 时间取得的当前时间。
	now := time.Unix(1, 0)
	// i 标记循环或列表中的当前位置。
	for i := 0; i < 10; i++ {
		// at 保存由新增取得的发生时间。
		at := now.Add(time.Duration(i) * time.Second)
		w.beginTownNavigationTick(at)
		w.moveTown(r, r.Pet, r.Player.Pos, at, 1)
		w.endTownNavigationTick()
	}
	if distance(r.Pet.Pos, r.Player.Pos) != 1 || r.Pet.Pos == r.Player.Pos || r.TownNavigation[1].state != townnav.Arrived {
		t.Fatal("宠物未停在主人合法邻格")
	}
	// i 标记循环或列表中的当前位置。
	for i := 0; i < 5; i++ {
		// at 保存由新增取得的发生时间。
		at := now.Add(time.Duration(i+11) * time.Second)
		w.beginTownNavigationTick(at)
		w.moveTown(r, r.Pet, r.Player.Pos, at, 1)
		w.endTownNavigationTick()
		if w.townNavigation.used != 0 {
			t.Fatal("宠物已邻接仍重复整图搜索")
		}
	}
}

// TestTownNavigationSceneLifeAndInputChangesCancelBeforeAdvancing 核对「旧身份继续展开或未归还共享资源」与「未建立跨tick活跃搜索」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestTownNavigationSceneLifeAndInputChangesCancelBeforeAdvancing(t *testing.T) {
	// change 接收遍历文本列表时当前的元素。
	for _, change := range []string{"scene", "map", "life", "manual", "revision", "removed", "dead", "explicit"} {
		// t 提供断言失败报告、子用例和清理注册。
		t.Run(change, func(t *testing.T) {
			// w 接收导航城镇房间执行器供后续步骤使用，r 接收导航城镇房间执行器供后续步骤使用。
			w, r := navigationTownWorker(t, 32, 32, 1, 1, 1, 2)
			// target 初始化用作目标的点位。
			target := devconfig.Point{X: 30, Y: 30}
			requestTownGround(r, target)
			// now 保存由Unix 时间取得的当前时间。
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

// TestTownNavigationReadyRouteReleasesWorkspaceForNextRoom 核验工作区复用后旧路线应保留，用诊断「路线绕过NextMove/动作截止」定位对应断言。
// t 提供断言失败报告、子用例和清理注册。
func TestTownNavigationReadyRouteReleasesWorkspaceForNextRoom(t *testing.T) {
	// w 接收导航城镇房间执行器供后续步骤使用，a 保存由导航城镇房间执行器取得的a。
	w, a := navigationTownWorker(t, 16, 16, 128, 256, 1, 2)
	// b 保存新增导航城镇房间返回的结果。
	b := addNavigationTownRoom(w, "b", devconfig.Point{X: 1, Y: 2})
	// now 保存由Unix 时间取得的当前时间。
	now := time.Unix(1, 0)
	// r 接收遍历房间列表时当前的元素。
	for _, r := range []*room{a, b} {
		// target 初始化用作目标的点位。
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
	// r 接收遍历房间列表时当前的元素。
	for _, r := range []*room{a, b} {
		// before 保存位置，用于当前步骤中的操作前的状态。
		before := r.Player.Pos
		w.moveTown(r, &r.Player, *r.Destination, now, 100)
		if r.Player.Pos == before {
			t.Fatal("工作区复用后旧路线被覆盖")
		}
		// pos 保存当前处理所用的位置。
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

// TestTownNavigationUnknownPendingPausesWithoutBlockingOtherOwners 核对「未知事务冻结占住全worker唯一搜索区」与「事务恢复重置同请求累计预算」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestTownNavigationUnknownPendingPausesWithoutBlockingOtherOwners(t *testing.T) {
	// w 接收导航城镇房间执行器供后续步骤使用，a 保存由导航城镇房间执行器取得的a。
	w, a := navigationTownWorker(t, 32, 32, 1, 1, 1, 2)
	// b 保存新增导航城镇房间返回的结果。
	b := addNavigationTownRoom(w, "b", devconfig.Point{X: 1, Y: 2})
	// now 保存由Unix 时间取得的当前时间。
	now := time.Unix(1, 0)
	// r 接收遍历房间列表时当前的元素。
	for _, r := range []*room{a, b} {
		// target 初始化用作目标的点位。
		target := devconfig.Point{X: 30, Y: r.Player.Pos.Y}
		requestTownGround(r, target)
		w.moveTown(r, &r.Player, target, now, 1)
	}
	w.beginTownNavigationTick(now)
	// q 保存从城镇导航取出的城镇导航中的指定项。
	q := a.TownNavigation[0]
	// prior 保存合计值“expanded”，用于当前步骤中的“prior”。
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

// TestTownNavigationRepeatedUnchangedPausesKeepGroundTargetAndBudgets 核对「暂停未归还共享池、继续展开或改变原经济job」与「第次合法暂停恢复清除/替换原地面目标」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestTownNavigationRepeatedUnchangedPausesKeepGroundTargetAndBudgets(t *testing.T) {
	// reason 接收遍历文本列表时当前的元素。
	for _, reason := range []string{"pending", "not_ready", "quiescing"} {
		// t 提供断言失败报告、子用例和清理注册。
		t.Run(reason, func(t *testing.T) {
			// w 接收导航城镇房间执行器供后续步骤使用，r 接收导航城镇房间执行器供后续步骤使用。
			w, r := navigationTownWorker(t, 32, 32, 128, 256, 1, 2)
			// now 保存由Unix 时间取得的当前时间。
			now := time.Unix(1, 0)
			// target 初始化用作目标的点位。
			target := devconfig.Point{X: 10, Y: 1}
			requestTownGround(r, target)
			r.Player.NextMove = now.Add(24 * time.Hour)
			w.move(r, &r.Player, target, now, 1)
			w.beginTownNavigationTick(now)
			// q 保存从城镇导航取出的城镇导航中的指定项。
			q := r.TownNavigation[0]
			if q == nil || q.state != townnav.Ready {
				t.Fatal("未建立原地面路线")
			}
			// revision 保存当前处理所用的修订号，start 保存位置，用于当前步骤中的起点或开始时间。
			revision, start := r.Navigation.Revision, r.Player.Pos
			// prior 保存合计值“expanded”，用于当前步骤中的“prior”。
			prior := q.task.Progress().TotalExpanded
			// job 保存当前处理所用的任务或职业编号。
			job := &job{}
			r.PendingRetry = job
			// i 标记循环或列表中的当前位置。
			for i := 0; i < 4; i++ {
				// at 保存由新增取得的发生时间。
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
				// progress 接收进度供后续步骤使用。
				progress := q.task.Progress()
				if progress.Replans != 0 || progress.TotalExpanded <= prior || w.townNavigation.used <= 0 || w.townNavigation.used > 256 {
					t.Fatal("纯暂停计入实际重规划或重置了累计/worker工作额度", progress)
				}
				prior = progress.TotalExpanded
			}
		})
	}
}

// TestTownNavigationQuiescingReturnsPoolWithoutAdmittingOrAdvancing 核对「收束仍持有大缓冲、启动排队搜索或继续展开」与「未建立耗尽tick额度的活跃搜索」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestTownNavigationQuiescingReturnsPoolWithoutAdmittingOrAdvancing(t *testing.T) {
	// w 接收导航城镇房间执行器供后续步骤使用，a 保存由导航城镇房间执行器取得的a。
	w, a := navigationTownWorker(t, 32, 32, 2, 4, 1, 2)
	// b 保存新增导航城镇房间返回的结果。
	b := addNavigationTownRoom(w, "b", devconfig.Point{X: 1, Y: 2})
	// now 保存由Unix 时间取得的当前时间。
	now := time.Unix(1, 0)
	// r 接收遍历房间列表时当前的元素。
	for _, r := range []*room{a, b} {
		// target 初始化用作目标的点位。
		target := devconfig.Point{X: 30, Y: r.Player.Pos.Y}
		requestTownGround(r, target)
		w.move(r, &r.Player, target, now, 1)
	}
	w.beginTownNavigationTick(now)
	if w.townNavigation.remaining != 0 || !w.townNavigation.workspaces[0].InUse() {
		t.Fatal("未建立耗尽tick额度的活跃搜索")
	}
	// prior 保存合计值“expanded”，用于当前步骤中的“prior”。
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

// TestTownNavigationQueueAndMemoryCapsAreExplicit 核对「队列压力扩大了预分配池」与「缺少显式内存预算仍启动」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestTownNavigationQueueAndMemoryCapsAreExplicit(t *testing.T) {
	// w 接收导航城镇房间执行器供后续步骤使用，r 接收导航城镇房间执行器供后续步骤使用。
	w, r := navigationTownWorker(t, 16, 16, 1, 1, 1, 1)
	// i 标记循环或列表中的当前位置。
	for i := 0; i < 5; i++ {
		// actor 保存当前处理所用的行动实体。
		actor := r
		if i > 0 {
			actor = addNavigationTownRoom(w, fmt.Sprint(i), devconfig.Point{X: 1, Y: 1})
		}
		// target 初始化用作目标的点位。
		target := devconfig.Point{X: 15, Y: 15}
		requestTownGround(actor, target)
		w.moveTown(actor, &actor.Player, target, time.Unix(1, 0), 1)
	}
	if len(w.townNavigation.queue) != 1 || cap(w.townNavigation.queue) != 1 || len(w.townNavigation.workspaces) != 1 || len(w.townNavigation.routes) != 2 {
		t.Fatal("队列压力扩大了预分配池")
	}
	w.e.Store.Cfg.Fixture.TownNavigation.MaxMemoryBytesPerWorker = 1
	w.townNavigation = nil
	// err 接收“initialize”城镇导航过程中可能产生的错误，供后续失败分支处理。
	if err := w.e.initializeTownNavigation(); err == nil {
		t.Fatal("缺少显式内存预算仍启动")
	}
}

// TestTownNavigationDisabledNeverFallsBackToWholeTownBFS 核对「未知地图静默回到primary」与「城镇停用时退回同步寻路」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestTownNavigationDisabledNeverFallsBackToWholeTownBFS(t *testing.T) {
	// w 接收导航城镇房间执行器供后续步骤使用，r 接收导航城镇房间执行器供后续步骤使用。
	w, r := navigationTownWorker(t, 16, 16, 1, 1, 1, 1)
	w.e.Store.Cfg.Fixture.TownNavigation.Enabled = false
	// before 保存位置，用于当前步骤中的操作前的状态。
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

// TestTownNavigationForcedRelocationReconsidersStaticTerminalStart 核对「强制位移后保留失效无路结论，或换请求重置预算」与「换到可达分量后未继续原地面目标」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestTownNavigationForcedRelocationReconsidersStaticTerminalStart(t *testing.T) {
	// walls 记录“walls”，使用点位列表表示。
	var walls []devconfig.Point
	// y 规定纵坐标的初始取值。
	for y := 0; y < 8; y++ {
		walls = append(walls, devconfig.Point{X: 4, Y: y})
	}
	// w 接收导航城镇房间执行器供后续步骤使用，r 接收导航城镇房间执行器供后续步骤使用。
	w, r := navigationTownWorker(t, 8, 8, 32, 64, 1, 2, walls...)
	// target 初始化用作目标的点位。
	target := devconfig.Point{X: 6, Y: 1}
	requestTownGround(r, target)
	// now 保存由Unix 时间取得的当前时间。
	now := time.Unix(1, 0)
	navigationTownTick(w, r, target, now)
	// q 保存从城镇导航取出的城镇导航中的指定项。
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

// TestTownNavigationDynamicNextStepCannotBypassReplanCap 核对「动态阻挡进入占格、绕过零重规划上限或泄漏路线」与「限额终态每tick另起搜索」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestTownNavigationDynamicNextStepCannotBypassReplanCap(t *testing.T) {
	// w 接收导航城镇房间执行器供后续步骤使用，r 接收导航城镇房间执行器供后续步骤使用。
	w, r := navigationTownWorker(t, 16, 16, 128, 256, 1, 2)
	w.e.Store.Cfg.Fixture.TownNavigation.MaxReplans = 0
	// target 初始化用作目标的点位。
	target := devconfig.Point{X: 10, Y: 1}
	requestTownGround(r, target)
	// now 保存由Unix 时间取得的当前时间。
	now := time.Unix(1, 0)
	navigationTownTick(w, r, target, now)
	// q 保存从城镇导航取出的城镇导航中的指定项。
	q := r.TownNavigation[0]
	// next 接收下一状态供后续步骤使用，ok 表明查询、校验或类型转换是否成功，err 接收下一状态过程中可能产生的错误，供后续失败分支处理。
	next, ok, err := q.task.Next(q.key, townPoint(r.Player.Pos), q.dynamic)
	if err != nil || !ok {
		t.Fatal("未准备就绪路线", err)
	}
	r.Pet = &entity{Handle: 2, Kind: 2, HP: 10, Pos: gamePoint(next)}
	// before 保存位置，用于当前步骤中的操作前的状态。
	before := r.Player.Pos
	navigationTownTick(w, r, target, now.Add(time.Second))
	if r.Player.Pos != before || q.state != townnav.Limited || q.task != nil || len(w.townNavigation.routes) != 3 {
		t.Fatal("动态阻挡进入占格、绕过零重规划上限或泄漏路线")
	}
	// i 标记循环或列表中的当前位置。
	for i := 0; i < 3; i++ {
		navigationTownTick(w, r, target, now.Add(time.Duration(i+2)*time.Second))
		if w.townNavigation.used != 0 || r.TownNavigation[0] != q {
			t.Fatal("限额终态每tick另起搜索")
		}
	}
}

// TestTownNavigationFullTownCoordinatesRemainWithinPerTickBudget 核对「源700图坐标触发越过worker本轮展开预算」与「来源Town坐标被个人图裁剪或未能增量完成」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestTownNavigationFullTownCoordinatesRemainWithinPerTickBudget(t *testing.T) {
	// w 接收导航城镇房间执行器供后续步骤使用，r 接收导航城镇房间执行器供后续步骤使用。
	w, r := navigationTownWorker(t, 700, 700, 1, 1, 1, 2)
	r.Player.Pos = devconfig.Point{X: 633, Y: 613}
	// target 初始化用作目标的点位。
	target := devconfig.Point{X: 633, Y: 614}
	requestTownGround(r, target)
	// now 保存由Unix 时间取得的当前时间。
	now := time.Unix(1, 0)
	// i 标记循环或列表中的当前位置。
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
