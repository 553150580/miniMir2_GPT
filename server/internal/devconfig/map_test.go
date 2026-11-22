// 本文件验证挂机图尺寸、碰撞、出生连通性及其导出的有限搜索预算。

package devconfig

import "testing"

// mapFixture 返回地图开发测试配置供调用方使用。
// size 记录大小，使用数值表示。
func mapFixture(size int) Fixture {
	// f 组装开发测试配置所需的数据。
	f := Fixture{}
	f.Map.ID, f.Map.Width, f.Map.Height = 1, size, size
	f.Map.PlayerSpawn, f.Map.PetSpawn = Point{X: 8, Y: 8}, Point{X: 8, Y: 7}
	f.Map.MonsterSpawns = []Point{{X: 1, Y: 1}, {X: 1, Y: 14}, {X: 14, Y: 1}, {X: 14, Y: 14}, {X: 1, Y: 4}, {X: 3, Y: 3}, {X: 5, Y: 14}, {X: 2, Y: 12}}
	return f
}

// TestMapApprovedDimensionsAndDerivedSearchBudget 核对「规则26未接入13格地图及169格搜索预算」与「已批准的格地图或派生预算被拒绝」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestMapApprovedDimensionsAndDerivedSearchBudget(t *testing.T) {
	// size 接收遍历数值列表时当前的元素。
	for _, size := range []int{16, 17} {
		// f 接收地图开发测试配置供后续步骤使用。
		f := mapFixture(size)
		if f.GridCellBudget() != size*size || f.ValidateMap() != nil {
			t.Fatalf("已批准的%d格地图或派生预算被拒绝", size)
		}
	}
	// size 接收遍历数值列表列表时当前的元素。
	for _, size := range [][2]int{{0, 0}, {-1, -1}, {16, 17}, {17, 16}, {18, 18}, {int(^uint(0) >> 1), int(^uint(0) >> 1)}} {
		// f 接收地图开发测试配置供后续步骤使用。
		f := mapFixture(17)
		f.Map.Width, f.Map.Height = size[0], size[1]
		if f.GridCellBudget() != 0 || f.ValidateMap() == nil {
			t.Fatalf("未批准或溢出尺寸被接受：%v", size)
		}
	}
	// f 接收地图开发测试配置供后续步骤使用。
	f := mapFixture(13)
	f.Map.MonsterSpawns = []Point{{X: 1, Y: 1}, {X: 1, Y: 11}, {X: 11, Y: 1}, {X: 11, Y: 11}, {X: 1, Y: 4}, {X: 3, Y: 3}, {X: 5, Y: 11}, {X: 2, Y: 12}}
	f.RuleVersion = 25
	if f.GridCellBudget() != 0 || f.ValidateMap() == nil {
		t.Fatal("历史规则错误接入13格地图")
	}
	f.RuleVersion = 26
	if f.GridCellBudget() != 169 || f.ValidateMap() != nil {
		t.Fatal("规则26未接入13格地图及169格搜索预算")
	}
}

// TestMapRejectsInvalidCollisionSpawnsAndDisconnectedBirthCells 核验拒绝地图无效碰撞/出生/连通性配置。
// t 提供断言失败报告、子用例和清理注册。
func TestMapRejectsInvalidCollisionSpawnsAndDisconnectedBirthCells(t *testing.T) {
	// cases 建立带初始值的按文本索引的供当前流程调用的函数映射，供本段继续处理。
	cases := map[string]func(*Fixture){
		// f 记录开发测试配置。
		"缺地图ID": func(f *Fixture) { f.Map.ID = 0 },
		// f 记录开发测试配置。
		"缺怪物槽位": func(f *Fixture) { f.Map.MonsterSpawns = f.Map.MonsterSpawns[:7] },
		// f 记录开发测试配置。
		"阻挡负坐标": func(f *Fixture) { f.Map.Blocked = []Point{{X: -1, Y: 0}} },
		// f 记录开发测试配置。
		"阻挡越界": func(f *Fixture) { f.Map.Blocked = []Point{{X: 17, Y: 16}} },
		// f 记录开发测试配置。
		"阻挡重复": func(f *Fixture) { f.Map.Blocked = []Point{{X: 12, Y: 12}, {X: 12, Y: 12}} },
		// f 记录开发测试配置。
		"玩家出生越界": func(f *Fixture) { f.Map.PlayerSpawn = Point{X: 17, Y: 8} },
		// f 记录开发测试配置。
		"宝宝出生越界": func(f *Fixture) { f.Map.PetSpawn = Point{X: 8, Y: -1} },
		// f 记录开发测试配置。
		"怪物出生越界": func(f *Fixture) { f.Map.MonsterSpawns[7] = Point{X: 16, Y: 17} },
		// f 记录开发测试配置。
		"玩家出生阻挡": func(f *Fixture) { f.Map.Blocked = []Point{f.Map.PlayerSpawn} },
		// f 记录开发测试配置。
		"宝宝出生阻挡": func(f *Fixture) { f.Map.Blocked = []Point{f.Map.PetSpawn} },
		// f 记录开发测试配置。
		"怪物出生阻挡": func(f *Fixture) { f.Map.Blocked = []Point{f.Map.MonsterSpawns[7]} },
		// f 记录开发测试配置。
		"玩家宝宝出生重叠": func(f *Fixture) { f.Map.PetSpawn = f.Map.PlayerSpawn },
		// f 记录开发测试配置。
		"怪物出生重叠": func(f *Fixture) { f.Map.MonsterSpawns[7] = f.Map.MonsterSpawns[0] },
		// f 记录开发测试配置。
		"怪物宝宝出生重叠": func(f *Fixture) { f.Map.MonsterSpawns[7] = f.Map.PetSpawn },
		// f 记录开发测试配置。
		"怪物出生静态孤岛": func(f *Fixture) {
			f.Map.Blocked = []Point{{X: 0, Y: 0}, {X: 1, Y: 0}, {X: 2, Y: 0}, {X: 0, Y: 1}, {X: 2, Y: 1}, {X: 0, Y: 2}, {X: 1, Y: 2}, {X: 2, Y: 2}}
		},
		// f 记录开发测试配置。
		"八向连通不得穿角": func(f *Fixture) {
			f.Map.PlayerSpawn = Point{X: 0, Y: 0}
			f.Map.Blocked = []Point{{X: 1, Y: 0}, {X: 0, Y: 1}}
		},
	}
	// name 定位“cases”中正在处理的成员，change 接收遍历“cases”时当前的元素。
	for name, change := range cases {
		// t 提供断言失败报告、子用例和清理注册。
		t.Run(name, func(t *testing.T) {
			// f 接收地图开发测试配置供后续步骤使用。
			f := mapFixture(17)
			change(&f)
			// err 接收校验地图过程中可能产生的错误，供后续失败分支处理。
			if err := f.ValidateMap(); err == nil {
				t.Fatal("地图无效碰撞/出生/连通性配置被接受")
			}
		})
	}
}

// TestMapConnectivityChecksBirthCellsWithoutInventingWholeGroundConnectivity 核对「17格绕行可达出生被旧预算截断」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestMapConnectivityChecksBirthCellsWithoutInventingWholeGroundConnectivity(t *testing.T) {
	// f 接收地图开发测试配置供后续步骤使用。
	f := mapFixture(17)
	// 右下空地可隔离；只要所有配置出生点可达，不新增‘每个可走格都连通’的产品限制。
	f.Map.Blocked = []Point{{X: 15, Y: 15}, {X: 15, Y: 16}, {X: 16, Y: 15}}
	// err 接收校验地图过程中可能产生的错误，供后续失败分支处理。
	if err := f.ValidateMap(); err != nil {
		t.Fatal(err)
	}
	// 超过旧256格的静态绕行也必须正确验证。
	f.Map.PlayerSpawn, f.Map.PetSpawn = Point{X: 0, Y: 16}, Point{X: 1, Y: 16}
	f.Map.MonsterSpawns = nil
	// y 规定纵坐标的初始取值。
	for y := 16; y >= 9; y-- {
		f.Map.MonsterSpawns = append(f.Map.MonsterSpawns, Point{X: 16, Y: y})
	}
	f.Map.Blocked = nil
	// y 规定纵坐标的初始取值。
	for y := 1; y < f.Map.Height; y++ {
		f.Map.Blocked = append(f.Map.Blocked, Point{X: 14, Y: y})
	}
	// err 接收校验地图过程中可能产生的错误，供后续失败分支处理。
	if err := f.ValidateMap(); err != nil {
		t.Fatal("17格绕行可达出生被旧预算截断：", err)
	}
}
