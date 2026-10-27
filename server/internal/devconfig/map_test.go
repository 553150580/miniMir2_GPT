package devconfig

import "testing"

func mapFixture(size int) Fixture {
	f := Fixture{}
	f.Map.ID, f.Map.Width, f.Map.Height = 1, size, size
	f.Map.PlayerSpawn, f.Map.PetSpawn = Point{X: 8, Y: 8}, Point{X: 8, Y: 7}
	f.Map.MonsterSpawns = []Point{{X: 1, Y: 1}, {X: 1, Y: 14}, {X: 14, Y: 1}, {X: 14, Y: 14}, {X: 1, Y: 4}, {X: 3, Y: 3}, {X: 5, Y: 14}, {X: 2, Y: 12}}
	return f
}

func TestMapApprovedDimensionsAndDerivedSearchBudget(t *testing.T) {
	for _, size := range []int{16, 17} {
		f := mapFixture(size)
		if f.GridCellBudget() != size*size || f.ValidateMap() != nil {
			t.Fatalf("已批准的%d格地图或派生预算被拒绝", size)
		}
	}
	for _, size := range [][2]int{{0, 0}, {-1, -1}, {16, 17}, {17, 16}, {18, 18}, {int(^uint(0) >> 1), int(^uint(0) >> 1)}} {
		f := mapFixture(17)
		f.Map.Width, f.Map.Height = size[0], size[1]
		if f.GridCellBudget() != 0 || f.ValidateMap() == nil {
			t.Fatalf("未批准或溢出尺寸被接受：%v", size)
		}
	}
}

func TestMapRejectsInvalidCollisionSpawnsAndDisconnectedBirthCells(t *testing.T) {
	cases := map[string]func(*Fixture){
		"缺地图ID":    func(f *Fixture) { f.Map.ID = 0 },
		"缺怪物槽位":    func(f *Fixture) { f.Map.MonsterSpawns = f.Map.MonsterSpawns[:7] },
		"阻挡负坐标":    func(f *Fixture) { f.Map.Blocked = []Point{{X: -1, Y: 0}} },
		"阻挡越界":     func(f *Fixture) { f.Map.Blocked = []Point{{X: 17, Y: 16}} },
		"阻挡重复":     func(f *Fixture) { f.Map.Blocked = []Point{{X: 12, Y: 12}, {X: 12, Y: 12}} },
		"玩家出生越界":   func(f *Fixture) { f.Map.PlayerSpawn = Point{X: 17, Y: 8} },
		"宝宝出生越界":   func(f *Fixture) { f.Map.PetSpawn = Point{X: 8, Y: -1} },
		"怪物出生越界":   func(f *Fixture) { f.Map.MonsterSpawns[7] = Point{X: 16, Y: 17} },
		"玩家出生阻挡":   func(f *Fixture) { f.Map.Blocked = []Point{f.Map.PlayerSpawn} },
		"宝宝出生阻挡":   func(f *Fixture) { f.Map.Blocked = []Point{f.Map.PetSpawn} },
		"怪物出生阻挡":   func(f *Fixture) { f.Map.Blocked = []Point{f.Map.MonsterSpawns[7]} },
		"玩家宝宝出生重叠": func(f *Fixture) { f.Map.PetSpawn = f.Map.PlayerSpawn },
		"怪物出生重叠":   func(f *Fixture) { f.Map.MonsterSpawns[7] = f.Map.MonsterSpawns[0] },
		"怪物宝宝出生重叠": func(f *Fixture) { f.Map.MonsterSpawns[7] = f.Map.PetSpawn },
		"怪物出生静态孤岛": func(f *Fixture) {
			f.Map.Blocked = []Point{{X: 0, Y: 0}, {X: 1, Y: 0}, {X: 2, Y: 0}, {X: 0, Y: 1}, {X: 2, Y: 1}, {X: 0, Y: 2}, {X: 1, Y: 2}, {X: 2, Y: 2}}
		},
		"八向连通不得穿角": func(f *Fixture) {
			f.Map.PlayerSpawn = Point{X: 0, Y: 0}
			f.Map.Blocked = []Point{{X: 1, Y: 0}, {X: 0, Y: 1}}
		},
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			f := mapFixture(17)
			change(&f)
			if err := f.ValidateMap(); err == nil {
				t.Fatal("地图无效碰撞/出生/连通性配置被接受")
			}
		})
	}
}

func TestMapConnectivityChecksBirthCellsWithoutInventingWholeGroundConnectivity(t *testing.T) {
	f := mapFixture(17)
	// 右下空地可隔离；只要所有配置出生点可达，不新增‘每个可走格都连通’的产品限制。
	f.Map.Blocked = []Point{{X: 15, Y: 15}, {X: 15, Y: 16}, {X: 16, Y: 15}}
	if err := f.ValidateMap(); err != nil {
		t.Fatal(err)
	}
	// 超过旧256格的静态绕行也必须正确验证。
	f.Map.PlayerSpawn, f.Map.PetSpawn = Point{X: 0, Y: 16}, Point{X: 1, Y: 16}
	f.Map.MonsterSpawns = nil
	for y := 16; y >= 9; y-- {
		f.Map.MonsterSpawns = append(f.Map.MonsterSpawns, Point{X: 16, Y: y})
	}
	f.Map.Blocked = nil
	for y := 1; y < f.Map.Height; y++ {
		f.Map.Blocked = append(f.Map.Blocked, Point{X: 14, Y: y})
	}
	if err := f.ValidateMap(); err != nil {
		t.Fatal("17格绕行可达出生被旧预算截断：", err)
	}
}
