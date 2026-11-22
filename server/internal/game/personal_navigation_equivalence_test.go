package game

import (
	"fmt"
	"math/rand"
	"testing"
	"time"

	"minimir/internal/devconfig"
	"minimir/internal/persistence"
)

func TestPersonalNavigationPreservesFirstStep(t *testing.T) {
	start := devconfig.Point{X: 2, Y: 2}
	diagonal := devconfig.Point{X: 3, Y: 3}
	farDiagonal := devconfig.Point{X: 4, Y: 4}
	orthogonal := devconfig.Point{X: 3, Y: 2}
	for _, tc := range []struct {
		name    string
		goal    devconfig.Point
		blocked []devconfig.Point
		setup   func(*room) *entity
		want    devconfig.Point
	}{
		{name: "相邻空目标可进入", goal: diagonal, want: diagonal},
		{name: "离目标同样近时保留旧方向顺序", goal: devconfig.Point{X: 2, Y: 4}, want: diagonal},
		{name: "静态侧墙挡斜向", goal: farDiagonal, blocked: []devconfig.Point{orthogonal}, want: devconfig.Point{X: 2, Y: 3}},
		{name: "允许从受阻的起点离开", goal: diagonal, blocked: []devconfig.Point{start}, want: diagonal},
		{name: "动态侧格不挡斜向", goal: farDiagonal, want: diagonal, setup: func(r *room) *entity {
			r.Pet = &entity{HP: 10, Pos: orthogonal}
			r.Monsters = []*entity{{HP: 10, Pos: devconfig.Point{X: 2, Y: 3}}}
			r.MonsterCount = 1
			return &r.Player
		}},
		{name: "相邻活怪挡住终点", goal: diagonal, want: start, setup: func(r *room) *entity {
			r.Monsters = []*entity{{HP: 10, Pos: diagonal}}
			r.MonsterCount = 1
			return &r.Player
		}},
		{name: "在场活宠挡住相邻终点", goal: diagonal, want: start, setup: func(r *room) *entity {
			r.Pet = &entity{Kind: 2, HP: 10, Pos: diagonal}
			r.PetWorld = r.Ref
			return &r.Player
		}},
		{name: "死怪和未激活的活怪不挡终点", goal: diagonal, want: diagonal, setup: func(r *room) *entity {
			r.Monsters = []*entity{{HP: 0, Pos: diagonal}, {HP: 10, Pos: diagonal}}
			r.MonsterCount = 1
			return &r.Player
		}},
		{name: "同一角色在其他场景的宠物不挡终点", goal: diagonal, want: diagonal, setup: func(r *room) *entity {
			r.Pet = &entity{HP: 10, Pos: diagonal}
			r.PetWorld = worldRef{Epoch: r.Ref.Epoch, ID: 2}
			r.TownWorld = &worldState{Ref: r.PetWorld}
			return &r.Player
		}},
		{name: "宠物离开自己与角色重叠的起点", goal: diagonal, want: diagonal, setup: func(r *room) *entity {
			r.Pet = &entity{Kind: 2, HP: 10, Pos: r.Player.Pos}
			return r.Pet
		}},
		{name: "怪物接近目标而不进入当前角色格", goal: farDiagonal, want: diagonal, setup: func(r *room) *entity {
			r.Player.Pos = farDiagonal
			r.Monsters = []*entity{{Kind: 3, HP: 10, Pos: start}}
			r.MonsterCount = 1
			return r.Monsters[0]
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := newPersonalNavigationTestWorker(13, tc.blocked)
			r := newPersonalNavigationTestRoom(start)
			actor := &r.Player
			if tc.setup != nil {
				actor = tc.setup(r)
			}
			want, search := referencePersonalNavigation(&w.e.Store.Cfg.Fixture.Map, r, actor, tc.goal)
			if !search || want != tc.want {
				t.Fatalf("旧路径参考首步为%+v，预期%+v", want, tc.want)
			}
			checkPersonalNavigation(t, w, r, actor, tc.goal, want, search)
		})
	}
}

func TestPersonalNavigationWorkspacesPreserveRoomIsolation(t *testing.T) {
	for _, size := range []int{13, 16, 17} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			var blocked []devconfig.Point
			for y := 1; y < size; y++ {
				blocked = append(blocked, devconfig.Point{X: size / 2, Y: y})
			}
			engine := newPersonalNavigationTestWorker(size, blocked).e
			for shard := range 4 {
				t.Run(fmt.Sprint(shard), func(t *testing.T) {
					t.Parallel()
					w := &worker{e: engine}
					m := &engine.Store.Cfg.Fixture.Map
					random := rand.New(rand.NewSource(int64(size*10 + shard)))
					point := func() devconfig.Point {
						return devconfig.Point{X: random.Intn(size), Y: random.Intn(size)}
					}
					for round := range 128 {
						r := newPersonalNavigationTestRoom(point())
						goal := point()
						r.Pet = &entity{Kind: 2, HP: 10, Pos: point()}
						for range 8 {
							r.Monsters = append(r.Monsters, &entity{Kind: 3, HP: int64(random.Intn(2)), Pos: point()})
						}
						r.MonsterCount = 1 + random.Intn(len(r.Monsters))
						// 同一worker交替处理角色、宠物和怪物。每间房的活占格分布均不同。
						actor := &r.Player
						switch round % 3 {
						case 1:
							actor = r.Pet
						case 2:
							actor = r.Monsters[0]
							actor.HP = 10
						}
						want, search := referencePersonalNavigation(m, r, actor, goal)
						checkPersonalNavigation(t, w, r, actor, goal, want, search)
					}
				})
			}
		})
	}
}

func newPersonalNavigationTestWorker(size int, blocked []devconfig.Point) *worker {
	m := devconfig.MapTemplate{ID: 1, Width: size, Height: size, Blocked: blocked}
	return &worker{e: &Engine{Store: &persistence.Store{Cfg: devconfig.Config{Fixture: devconfig.Fixture{RuleVersion: 26, Map: m}}}}}
}

func newPersonalNavigationTestRoom(start devconfig.Point) *room {
	return &room{
		ExecutionEpoch: 1, WorldSequence: 1,
		worldState: &worldState{Ref: worldRef{Epoch: 1, ID: 1}, MapID: 1},
		Player:     entity{Kind: 1, HP: 10, Pos: start},
	}
}

// 旧搜索作为独立参照，避免新旧实现共享可变工作区或占格判断。
func referencePersonalNavigation(m *devconfig.MapTemplate, r *room, actor *entity, goal devconfig.Point) (devconfig.Point, bool) {
	start := actor.Pos
	inside := func(p devconfig.Point) bool {
		return p.X >= 0 && p.Y >= 0 && p.X < m.Width && p.Y < m.Height
	}
	if start == goal || !inside(start) || !inside(goal) {
		return start, false
	}
	static := make(map[devconfig.Point]bool)
	for _, p := range m.Blocked {
		static[p] = true
	}
	walkable := func(p devconfig.Point) bool { return inside(p) && !static[p] }
	occupied := make(map[devconfig.Point]bool)
	mark := func(e *entity) {
		if e != nil && e != actor && e.HP > 0 {
			occupied[e.Pos] = true
		}
	}
	mark(&r.Player)
	if r.PetWorld == (worldRef{}) || r.PetWorld == r.Ref {
		mark(r.Pet)
	}
	for _, monster := range r.Monsters[:r.MonsterCount] {
		mark(monster)
	}
	type visit struct{ point, first devconfig.Point }
	queue := []visit{{point: start}}
	seen := map[devconfig.Point]bool{start: true}
	offsets := [8][2]int{{0, -1}, {1, -1}, {1, 0}, {1, 1}, {0, 1}, {-1, 1}, {-1, 0}, {-1, -1}}
	for head := 0; head < len(queue); head++ {
		current := queue[head]
		for _, offset := range offsets {
			dx, dy := offset[0], offset[1]
			p := devconfig.Point{X: current.point.X + dx, Y: current.point.Y + dy}
			if seen[p] || !walkable(p) || occupied[p] {
				continue
			}
			if dx != 0 && dy != 0 && (!walkable(devconfig.Point{X: p.X, Y: current.point.Y}) || !walkable(devconfig.Point{X: current.point.X, Y: p.Y})) {
				continue
			}
			seen[p] = true
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

func referencePersonalAdjacent(a, b devconfig.Point) bool {
	dx, dy := a.X-b.X, a.Y-b.Y
	return dx >= -1 && dx <= 1 && dy >= -1 && dy <= 1 && (dx != 0 || dy != 0)
}

func checkPersonalNavigation(t *testing.T, w *worker, r *room, actor *entity, goal, want devconfig.Point, search bool) {
	t.Helper()
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
		delta := devconfig.Point{X: want.X - start.X, Y: want.Y - start.Y}
		if actor.Direction >= uint32(len(directions)) || directions[actor.Direction] != delta || actor.MovementKind != movementWalk || actor.MovementRevision != 1 {
			t.Fatalf("首步%+v的方向或移动提交状态不正确：方向%d，类型%d，版本%d", delta, actor.Direction, actor.MovementKind, actor.MovementRevision)
		}
	}
	if actor == &r.Player && search && want == start && (r.Destination != nil || r.Navigation.Status != navigationBlocked || r.Navigation.Destination == nil || *r.Navigation.Destination != goal) {
		t.Fatalf("没有合法首步时，移动接管和原目标状态不正确：%+v", r.Navigation)
	}
}
