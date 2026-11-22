// 本文件比对个人网格首步与旧算法结果，并验证共用工作区隔离房间状态。
package game

import (
	"fmt"
	"math/rand"
	"testing"
	"time"

	"minimir/internal/devconfig"
	"minimir/internal/persistence"
)

// TestPersonalNavigationPreservesFirstStep 核对「旧路径参考首步为」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestPersonalNavigationPreservesFirstStep(t *testing.T) {
	// start 初始化用作起点或开始时间的点位。
	start := devconfig.Point{X: 2, Y: 2}
	// diagonal 建立带初始值的点位，供本段继续处理。
	diagonal := devconfig.Point{X: 3, Y: 3}
	// farDiagonal 建立带初始值的点位，供本段继续处理。
	farDiagonal := devconfig.Point{X: 4, Y: 4}
	// orthogonal 建立带初始值的点位，供本段继续处理。
	orthogonal := devconfig.Point{X: 3, Y: 2}
	// tc 接收遍历临时结构列表时当前的元素。
	for _, tc := range []struct {
		// name 记录名称，使用文本表示。
		name string
		// goal 记录目标格，使用点位表示。
		goal devconfig.Point
		// blocked 记录阻挡，使用点位列表表示。
		blocked []devconfig.Point
		// setup 记录“setup”，使用供当前流程调用的函数表示。
		setup func(*room) *entity
		// want 记录期望值，使用点位表示。
		want devconfig.Point
	}{
		{name: "相邻空目标可进入", goal: diagonal, want: diagonal},
		{name: "离目标同样近时保留旧方向顺序", goal: devconfig.Point{X: 2, Y: 4}, want: diagonal},
		{name: "静态侧墙挡斜向", goal: farDiagonal, blocked: []devconfig.Point{orthogonal}, want: devconfig.Point{X: 2, Y: 3}},
		{name: "允许从受阻的起点离开", goal: diagonal, blocked: []devconfig.Point{start}, want: diagonal},
		// r 指向当前执行器拥有并更新的目标房间。
		{name: "动态侧格不挡斜向", goal: farDiagonal, want: diagonal, setup: func(r *room) *entity {
			r.Pet = &entity{HP: 10, Pos: orthogonal}
			r.Monsters = []*entity{{HP: 10, Pos: devconfig.Point{X: 2, Y: 3}}}
			r.MonsterCount = 1
			return &r.Player
		}},
		// r 指向当前执行器拥有并更新的目标房间。
		{name: "相邻活怪挡住终点", goal: diagonal, want: start, setup: func(r *room) *entity {
			r.Monsters = []*entity{{HP: 10, Pos: diagonal}}
			r.MonsterCount = 1
			return &r.Player
		}},
		// r 指向当前执行器拥有并更新的目标房间。
		{name: "在场活宠挡住相邻终点", goal: diagonal, want: start, setup: func(r *room) *entity {
			r.Pet = &entity{Kind: 2, HP: 10, Pos: diagonal}
			r.PetWorld = r.Ref
			return &r.Player
		}},
		// r 指向当前执行器拥有并更新的目标房间。
		{name: "死怪和未激活的活怪不挡终点", goal: diagonal, want: diagonal, setup: func(r *room) *entity {
			r.Monsters = []*entity{{HP: 0, Pos: diagonal}, {HP: 10, Pos: diagonal}}
			r.MonsterCount = 1
			return &r.Player
		}},
		// r 指向当前执行器拥有并更新的目标房间。
		{name: "同一角色在其他场景的宠物不挡终点", goal: diagonal, want: diagonal, setup: func(r *room) *entity {
			r.Pet = &entity{HP: 10, Pos: diagonal}
			r.PetWorld = worldRef{Epoch: r.Ref.Epoch, ID: 2}
			r.TownWorld = &worldState{Ref: r.PetWorld}
			return &r.Player
		}},
		// r 指向当前执行器拥有并更新的目标房间。
		{name: "宠物离开自己与角色重叠的起点", goal: diagonal, want: diagonal, setup: func(r *room) *entity {
			r.Pet = &entity{Kind: 2, HP: 10, Pos: r.Player.Pos}
			return r.Pet
		}},
		// r 指向当前执行器拥有并更新的目标房间。
		{name: "怪物接近目标而不进入当前角色格", goal: farDiagonal, want: diagonal, setup: func(r *room) *entity {
			r.Player.Pos = farDiagonal
			r.Monsters = []*entity{{Kind: 3, HP: 10, Pos: start}}
			r.MonsterCount = 1
			return r.Monsters[0]
		}},
	} {
		// t 提供断言失败报告、子用例和清理注册。
		t.Run(tc.name, func(t *testing.T) {
			// w 接收创建个人导航测试房间执行器供后续步骤使用。
			w := newPersonalNavigationTestWorker(13, tc.blocked)
			// r 接收创建个人导航测试房间供后续步骤使用。
			r := newPersonalNavigationTestRoom(start)
			// actor 保存当前处理所用的行动实体。
			actor := &r.Player
			if tc.setup != nil {
				actor = tc.setup(r)
			}
			// want 保存由引用个人导航取得的期望值，search 保存由引用个人导航取得的“search”。
			want, search := referencePersonalNavigation(&w.e.Store.Cfg.Fixture.Map, r, actor, tc.goal)
			if !search || want != tc.want {
				t.Fatalf("旧路径参考首步为%+v，预期%+v", want, tc.want)
			}
			checkPersonalNavigation(t, w, r, actor, tc.goal, want, search)
		})
	}
}

// TestPersonalNavigationWorkspacesPreserveRoomIsolation 在多个并行分片中交替重用寻路工作区，把不同房间的角色、宠物和怪物路径与独立实现逐一比较。
// t 提供断言失败报告、子用例和清理注册。
func TestPersonalNavigationWorkspacesPreserveRoomIsolation(t *testing.T) {
	// size 接收遍历数值列表时当前的元素。
	for _, size := range []int{13, 16, 17} {
		// t 提供断言失败报告、子用例和清理注册。
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			// blocked 记录阻挡，使用点位列表表示。
			var blocked []devconfig.Point
			// y 规定纵坐标的初始取值。
			for y := 1; y < size; y++ {
				blocked = append(blocked, devconfig.Point{X: size / 2, Y: y})
			}
			// engine 保存e，用于当前步骤中的游戏引擎。
			engine := newPersonalNavigationTestWorker(size, blocked).e
			// shard 接收遍历当前数据时当前的元素。
			for shard := range 4 {
				// t 提供断言失败报告、子用例和清理注册。
				t.Run(fmt.Sprint(shard), func(t *testing.T) {
					t.Parallel()
					// w 保存当前处理所用的房间执行器。
					w := &worker{e: engine}
					// m 保存当前处理所用的地图。
					m := &engine.Store.Cfg.Fixture.Map
					// random 保存由创建取得的随机值。
					random := rand.New(rand.NewSource(int64(size*10 + shard)))
					// point 封装点位对应的后续处理。
					point := func() devconfig.Point {
						return devconfig.Point{X: random.Intn(size), Y: random.Intn(size)}
					}
					// round 接收遍历当前数据时当前的元素。
					for round := range 128 {
						// r 接收创建个人导航测试房间供后续步骤使用。
						r := newPersonalNavigationTestRoom(point())
						// goal 保存由点位取得的目标格。
						goal := point()
						r.Pet = &entity{Kind: 2, HP: 10, Pos: point()}
						for range 8 {
							r.Monsters = append(r.Monsters, &entity{Kind: 3, HP: int64(random.Intn(2)), Pos: point()})
						}
						r.MonsterCount = 1 + random.Intn(len(r.Monsters))
						// 同一worker交替处理角色、宠物和怪物。每间房的活占格分布均不同。
						// actor 保存当前处理所用的行动实体。
						actor := &r.Player
						switch round % 3 {
						case 1:
							actor = r.Pet
						case 2:
							actor = r.Monsters[0]
							actor.HP = 10
						}
						// want 保存由引用个人导航取得的期望值，search 保存由引用个人导航取得的“search”。
						want, search := referencePersonalNavigation(m, r, actor, goal)
						checkPersonalNavigation(t, w, r, actor, goal, want, search)
					}
				})
			}
		})
	}
}

// newPersonalNavigationTestWorker 构造个人导航测试房间执行器。
// size 记录大小，使用数值表示，blocked 记录阻挡，使用点位列表表示。
func newPersonalNavigationTestWorker(size int, blocked []devconfig.Point) *worker {
	// m 组装地图模板所需的数据。
	m := devconfig.MapTemplate{ID: 1, Width: size, Height: size, Blocked: blocked}
	return &worker{e: &Engine{Store: &persistence.Store{Cfg: devconfig.Config{Fixture: devconfig.Fixture{RuleVersion: 26, Map: m}}}}}
}

// newPersonalNavigationTestRoom 构造个人导航测试房间。
// start 记录起点或开始时间，使用点位表示。
func newPersonalNavigationTestRoom(start devconfig.Point) *room {
	return &room{
		ExecutionEpoch: 1, WorldSequence: 1,
		worldState: &worldState{Ref: worldRef{Epoch: 1, ID: 1}, MapID: 1},
		Player:     entity{Kind: 1, HP: 10, Pos: start},
	}
}

// 旧搜索作为独立参照，避免新旧实现共享可变工作区或占格判断。
// m 记录地图模板，r 指向当前执行器拥有并更新的目标房间，actor 记录行动实体，goal 记录目标格，使用点位表示。
func referencePersonalNavigation(m *devconfig.MapTemplate, r *room, actor *entity, goal devconfig.Point) (devconfig.Point, bool) {
	// start 保存位置，用于当前步骤中的起点或开始时间。
	start := actor.Pos
	// inside 封装“inside”对应的后续处理，p 记录点位。
	inside := func(p devconfig.Point) bool {
		return p.X >= 0 && p.Y >= 0 && p.X < m.Width && p.Y < m.Height
	}
	if start == goal || !inside(start) || !inside(goal) {
		return start, false
	}
	// static 创建用于维护“static”的容器或通道。
	static := make(map[devconfig.Point]bool)
	// p 接收遍历阻挡时当前的元素。
	for _, p := range m.Blocked {
		static[p] = true
	}
	// walkable 封装可走格对应的后续处理，p 记录点位。
	walkable := func(p devconfig.Point) bool { return inside(p) && !static[p] }
	// occupied 创建用于维护已占用的容器或通道。
	occupied := make(map[devconfig.Point]bool)
	// mark 封装“mark”对应的后续处理，e 记录实体。
	mark := func(e *entity) {
		if e != nil && e != actor && e.HP > 0 {
			occupied[e.Pos] = true
		}
	}
	mark(&r.Player)
	if r.PetWorld == (worldRef{}) || r.PetWorld == r.Ref {
		mark(r.Pet)
	}
	// monster 接收遍历怪物集合中的范围时当前的元素。
	for _, monster := range r.Monsters[:r.MonsterCount] {
		mark(monster)
	}
	// visit 保存“visit”所需的状态。
	// point 记录点位。
	// first 记录第一项，使用点位表示。
	type visit struct{ point, first devconfig.Point }
	// queue 初始化用作队列的“visit”列表。
	queue := []visit{{point: start}}
	// seen 建立带初始值的按点位索引的布尔状态映射，供本段继续处理。
	seen := map[devconfig.Point]bool{start: true}
	// offsets 初始化用作偏移集合的数值列表列表。
	offsets := [8][2]int{{0, -1}, {1, -1}, {1, 0}, {1, 1}, {0, 1}, {-1, 1}, {-1, 0}, {-1, -1}}
	// head 规定头部的初始取值。
	for head := 0; head < len(queue); head++ {
		// current 保存从队列取出的当前状态。
		current := queue[head]
		// offset 接收遍历偏移集合时当前的元素。
		for _, offset := range offsets {
			// dx 保存从偏移取出的横向差值，dy 保存从偏移取出的纵向差值。
			dx, dy := offset[0], offset[1]
			// p 组装点位所需的数据。
			p := devconfig.Point{X: current.point.X + dx, Y: current.point.Y + dy}
			if seen[p] || !walkable(p) || occupied[p] {
				continue
			}
			if dx != 0 && dy != 0 && (!walkable(devconfig.Point{X: p.X, Y: current.point.Y}) || !walkable(devconfig.Point{X: current.point.X, Y: p.Y})) {
				continue
			}
			seen[p] = true
			// first 保存当前处理所用的第一项。
			first := current.first
			if head == 0 {
				first = p
			}
			if p == goal || referencePersonalAdjacent(p, goal) && !referencePersonalAdjacent(start, goal) {
				return first, true
			}
			queue = append(queue, visit{point: p, first: first})
		}
	}
	return start, true
}

// referencePersonalAdjacent 判断引用个人相邻是否满足要求。
// a 记录a，使用点位表示，b 记录b，使用点位表示。
func referencePersonalAdjacent(a, b devconfig.Point) bool {
	// dx 保存根据横坐标与横坐标计算的横向差值，dy 保存根据纵坐标与纵坐标计算的纵向差值。
	dx, dy := a.X-b.X, a.Y-b.Y
	return dx >= -1 && dx <= 1 && dy >= -1 && dy <= 1 && (dx != 0 || dy != 0)
}

// checkPersonalNavigation 检查个人导航。
// t 提供断言失败报告、子用例和清理注册，w 记录房间执行器，r 指向当前执行器拥有并更新的目标房间，actor 记录行动实体，goal 记录目标格，使用点位表示。
// want 记录期望值，使用点位表示，search 标记是否处于“search”状态。
func checkPersonalNavigation(t *testing.T, w *worker, r *room, actor *entity, goal, want devconfig.Point, search bool) {
	t.Helper()
	// start 保存位置，用于当前步骤中的起点或开始时间。
	start := actor.Pos
	if actor == &r.Player {
		r.Destination = &goal
		r.Navigation = navigationState{Destination: &goal, Status: navigationActive, InputSequence: 1, Revision: 1}
	}
	w.move(r, actor, goal, time.Unix(1, 0), 100)
	if actor.Pos != want {
		t.Fatalf("地图%d格，实体%d从%+v走向%+v，首步为%+v，旧路径为%+v", w.e.Store.Cfg.Fixture.Map.Width, actor.Kind, start, goal, actor.Pos, want)
	}
	if want != start {
		// delta 初始化用作增量的点位。
		delta := devconfig.Point{X: want.X - start.X, Y: want.Y - start.Y}
		if actor.Direction >= uint32(len(directions)) || directions[actor.Direction] != delta || actor.MovementKind != movementWalk || actor.MovementRevision != 1 {
			t.Fatalf("首步%+v的方向或移动提交状态不正确：方向%d，类型%d，版本%d", delta, actor.Direction, actor.MovementKind, actor.MovementRevision)
		}
	}
	if actor == &r.Player && search && want == start && (r.Destination != nil || r.Navigation.Status != navigationBlocked || r.Navigation.Destination == nil || *r.Navigation.Destination != goal) {
		t.Fatalf("没有合法首步时，移动接管和原目标状态不正确：%+v", r.Navigation)
	}
}
