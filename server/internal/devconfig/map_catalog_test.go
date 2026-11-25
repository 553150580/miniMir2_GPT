// 本文件验证旧地图、城镇位图及安全区顺序的兼容性，拒绝污染目录的无效模板。

package devconfig

import (
	"encoding/json"
	"math"
	"strings"
	"testing"
)

// catalogTown 返回目录城镇供调用方使用。
func catalogTown() MapTemplate {
	return MapTemplate{
		ID: 2, SceneKind: SceneKindTown, Width: 700, Height: 700,
		PlayerSpawn:   Point{X: 633, Y: 613},
		CollisionBits: make([]byte, 700*700/8),
		SafeZones:     []SafeZone{{ID: 1, Center: Point{X: 650, Y: 631}, Radius: 10}},
		Source:        &SourceMapIdentity{Code: "0", SHA256: strings.Repeat("a", 64), Width: 700, Height: 700},
	}
}

// TestMapDirectoryPreservesLegacyPersonalMapAndAddsFullTown 核验城镇应保留700×700完整来源坐标，以及测试城镇不应有个人图出生配置。
// t 提供断言失败报告、子用例和清理注册。
func TestMapDirectoryPreservesLegacyPersonalMapAndAddsFullTown(t *testing.T) {
	// f 接收地图开发测试配置供后续步骤使用。
	f := mapFixture(17)
	f.Maps = []MapTemplate{catalogTown()}
	// err 接收校验地图过程中可能产生的错误，供后续失败分支处理。
	if err := f.ValidateMap(); err != nil {
		t.Fatal(err)
	}
	if f.GridCellBudget() != 289 || f.Map.SceneKindValue() != SceneKindPersonal {
		t.Fatal("新增城镇改变了旧个人地图尺寸或类型")
	}
	// id 接收遍历数值列表时当前的元素。
	for _, id := range []uint64{1, 2} {
		// m 接收地图“by”标识供后续步骤使用，ok 表明查询、校验或类型转换是否成功。
		m, ok := f.MapByID(id)
		if !ok || m.ID != id {
			t.Fatalf("地图%d没有保持独立目录身份", id)
		}
	}
	// id 接收遍历数值列表时当前的元素。
	for _, id := range []uint64{0, 3, math.MaxUint64} {
		// ok 表明查询、校验或类型转换是否成功。
		if _, ok := f.MapByID(id); ok {
			t.Fatalf("未知地图%d被补成个人地图", id)
		}
	}
	// town 保存由地图“by”标识取得的城镇。
	town, _ := f.MapByID(2)
	if town.CellBudget() != 490000 || !town.Contains(Point{X: 699, Y: 699}) || !town.Walkable(Point{X: 650, Y: 631}) {
		t.Fatal("城镇没有保留700×700完整来源坐标")
	}
	// 城镇没有静态宠物出生格，也不需要伪造怪物槽。
	if town.PetSpawn != (Point{}) || len(town.MonsterSpawns) != 0 {
		t.Fatal("测试城镇不应有个人图出生配置")
	}
	if !f.ValidBinding(BindingPoint{MapID: 2, X: 650, Y: 631}) || f.ValidBinding(BindingPoint{MapID: 1, X: 650, Y: 631}) {
		t.Fatal("绑定目录查询混淆了个人图与城镇坐标")
	}
	if f.ValidBinding(BindingPoint{MapID: 2, X: 700, Y: 631}) || f.ValidBinding(BindingPoint{MapID: 3, X: 650, Y: 631}) {
		t.Fatal("目录外地图或越界坐标获得了有效绑定")
	}
}

// TestTownCollisionBitmapUsesLowBitFirstAcrossRows 核对「不完整位图未关闭通行」与「越界点被当作可走」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestTownCollisionBitmapUsesLowBitFirstAcrossRows(t *testing.T) {
	// m 组装地图模板所需的数据。
	m := MapTemplate{SceneKind: SceneKindTown, Width: 9, Height: 2, CollisionBits: []byte{0x81, 0x03, 0x02}}
	// 独立列出位序对应格，覆盖字节首尾、非字节对齐换行与最后一格。
	// blocked 初始化用作阻挡的按点位索引的布尔状态映射。
	blocked := map[Point]bool{{X: 0, Y: 0}: true, {X: 7, Y: 0}: true, {X: 8, Y: 0}: true, {X: 0, Y: 1}: true, {X: 8, Y: 1}: true}
	// y 规定纵坐标的初始取值。
	for y := 0; y < 2; y++ {
		// x 规定横坐标的初始取值。
		for x := 0; x < 9; x++ {
			// point 组装点位所需的数据。
			point := Point{X: x, Y: y}
			if m.Walkable(point) == blocked[point] {
				t.Fatalf("碰撞位序错误：%+v", point)
			}
		}
	}
	// point 接收遍历点位列表时当前的元素。
	for _, point := range []Point{{X: -1}, {Y: -1}, {X: 9}, {Y: 2}, {X: math.MaxInt, Y: math.MaxInt}} {
		if m.Contains(point) || m.Walkable(point) {
			t.Fatalf("越界点被当作可走：%+v", point)
		}
	}
	m.CollisionBits = m.CollisionBits[:1]
	if m.Walkable(Point{X: 8, Y: 1}) {
		t.Fatal("不完整位图未关闭通行")
	}
}

// TestTownSafetyZonesUseInclusiveSquareAndSourceOrder 核对「明确的零坐标、零半径安全区应只包含中心」与「第二个安全区的独立范围未生效」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestTownSafetyZonesUseInclusiveSquareAndSourceOrder(t *testing.T) {
	// m 接收目录城镇供后续步骤使用。
	m := catalogTown()
	m.SafeZones = append(m.SafeZones, SafeZone{ID: 2, Center: Point{X: 655, Y: 631}, Radius: 10})
	// point 接收遍历点位列表时当前的元素。
	for _, point := range []Point{{X: 650, Y: 631}, {X: 640, Y: 621}, {X: 640, Y: 641}, {X: 660, Y: 621}, {X: 660, Y: 641}, {X: 655, Y: 631}} {
		// zone 保存由安全区域发生时间取得的区域，ok 表明查询、校验或类型转换是否成功。
		zone, ok := m.SafeZoneAt(point)
		if !ok || zone.ID != 1 || zone.Center != (Point{X: 650, Y: 631}) {
			t.Fatalf("安全区边界或来源顺序错误：%+v", point)
		}
	}
	// zone 保存由安全区域发生时间取得的区域，ok 表明查询、校验或类型转换是否成功。
	if zone, ok := m.SafeZoneAt(Point{X: 665, Y: 631}); !ok || zone.ID != 2 {
		t.Fatal("第二个安全区的独立范围未生效")
	}
	// point 接收遍历点位列表时当前的元素。
	for _, point := range []Point{{X: 639, Y: 631}, {X: 650, Y: 620}, {X: 666, Y: 631}, {X: -1, Y: 631}, {X: 700, Y: 631}} {
		// ok 表明查询、校验或类型转换是否成功。
		if _, ok := m.SafeZoneAt(point); ok {
			t.Fatalf("范围外格子落入安全区：%+v", point)
		}
	}
	m.SafeZones = []SafeZone{{ID: 3, Center: Point{}, Radius: 0}}
	// zone 保存由安全区域发生时间取得的区域，ok 表明查询、校验或类型转换是否成功。
	if zone, ok := m.SafeZoneAt(Point{}); !ok || zone.ID != 3 {
		t.Fatal("明确的零坐标、零半径安全区应只包含中心")
	}
}

// TestMapDirectoryRejectsInvalidTownDataWithoutRelaxingPersonalRules 核对「非法目录未被拒绝」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestMapDirectoryRejectsInvalidTownDataWithoutRelaxingPersonalRules(t *testing.T) {
	// name 定位按文本索引的供当前流程调用的函数映射中正在处理的成员，change 接收遍历按文本索引的供当前流程调用的函数映射时当前的元素。
	for name, change := range map[string]func(*Fixture){
		// f 记录开发测试配置。
		"重复主图ID": func(f *Fixture) { f.Maps[0].ID = f.Map.ID },
		// f 记录开发测试配置。
		"重复目录ID": func(f *Fixture) { f.Maps = append(f.Maps, catalogTown()) },
		// f 记录开发测试配置。
		"零ID": func(f *Fixture) { f.Maps[0].ID = 0 },
		// f 记录开发测试配置。
		"不可持久ID": func(f *Fixture) { f.Maps[0].ID = math.MaxUint64 },
		// f 记录开发测试配置。
		"缺明确类型": func(f *Fixture) { f.Maps[0].SceneKind = 0 },
		// f 记录开发测试配置。
		"额外个人图": func(f *Fixture) { f.Maps[0].SceneKind = SceneKindPersonal },
		// f 记录开发测试配置。
		"未知类型": func(f *Fixture) { f.Maps[0].SceneKind = 3 },
		// f 记录开发测试配置。
		"主图扩大": func(f *Fixture) { f.Map.Width, f.Map.Height = 700, 700 },
		// f 记录开发测试配置。
		"主图变城镇": func(f *Fixture) { f.Map.SceneKind = SceneKindTown },
		// f 记录开发测试配置。
		"主图偷偷换位图": func(f *Fixture) { f.Map.CollisionBits = make([]byte, 37) },
		// f 记录开发测试配置。
		"零宽": func(f *Fixture) { f.Maps[0].Width = 0 },
		// f 记录开发测试配置。
		"负高": func(f *Fixture) { f.Maps[0].Height = -1 },
		// f 记录开发测试配置。
		"尺寸溢出": func(f *Fixture) { f.Maps[0].Width, f.Maps[0].Height = math.MaxInt, math.MaxInt },
		// f 记录开发测试配置。
		"缺位图": func(f *Fixture) { f.Maps[0].CollisionBits = nil },
		// f 记录开发测试配置。
		"短位图": func(f *Fixture) { f.Maps[0].CollisionBits = f.Maps[0].CollisionBits[:61249] },
		// f 记录开发测试配置。
		"长位图": func(f *Fixture) { f.Maps[0].CollisionBits = append(f.Maps[0].CollisionBits, 0) },
		// f 记录开发测试配置。
		"并行阻挡源": func(f *Fixture) { f.Maps[0].Blocked = []Point{{X: 1, Y: 1}} },
		// f 记录开发测试配置。
		"伪造怪槽": func(f *Fixture) { f.Maps[0].MonsterSpawns = []Point{{X: 1, Y: 1}} },
		// f 记录开发测试配置。
		"入口越界": func(f *Fixture) { f.Maps[0].PlayerSpawn = Point{X: 700, Y: 631} },
		// f 记录开发测试配置。
		"入口阻挡": func(f *Fixture) { f.Maps[0].CollisionBits[(613*700+633)/8] |= 1 << uint((613*700+633)%8) },
		// f 记录开发测试配置。
		"无来源": func(f *Fixture) { f.Maps[0].Source = nil },
		// f 记录开发测试配置。
		"空来源代码": func(f *Fixture) { f.Maps[0].Source.Code = " " },
		// f 记录开发测试配置。
		"短来源摘要": func(f *Fixture) { f.Maps[0].Source.SHA256 = "abc" },
		// f 记录开发测试配置。
		"非法来源摘要": func(f *Fixture) { f.Maps[0].Source.SHA256 = strings.Repeat("g", 64) },
		// f 记录开发测试配置。
		"来源宽度不符": func(f *Fixture) { f.Maps[0].Source.Width = 17 },
		// f 记录开发测试配置。
		"来源高度不符": func(f *Fixture) { f.Maps[0].Source.Height = 17 },
		// f 记录开发测试配置。
		"安全区零ID": func(f *Fixture) { f.Maps[0].SafeZones[0].ID = 0 },
		// f 记录开发测试配置。
		"安全区重复ID": func(f *Fixture) { f.Maps[0].SafeZones = append(f.Maps[0].SafeZones, f.Maps[0].SafeZones[0]) },
		// f 记录开发测试配置。
		"安全区负半径": func(f *Fixture) { f.Maps[0].SafeZones[0].Radius = -1 },
		// f 记录开发测试配置。
		"安全区半径溢出": func(f *Fixture) { f.Maps[0].SafeZones[0].Radius = math.MaxInt },
		// f 记录开发测试配置。
		"安全区中心越界": func(f *Fixture) { f.Maps[0].SafeZones[0].Center.X = 700 },
		// f 记录开发测试配置。
		"安全区中心阻挡": func(f *Fixture) { f.Maps[0].CollisionBits[(631*700+650)/8] |= 1 << uint((631*700+650)%8) },
		// f 记录开发测试配置。
		"安全区左边越界": func(f *Fixture) { f.Maps[0].SafeZones[0].Center.X = 9 },
		// f 记录开发测试配置。
		"安全区上边越界": func(f *Fixture) { f.Maps[0].SafeZones[0].Center.Y = 9 },
		// f 记录开发测试配置。
		"安全区右边越界": func(f *Fixture) { f.Maps[0].SafeZones[0].Center.X = 690 },
		// f 记录开发测试配置。
		"安全区下边越界": func(f *Fixture) { f.Maps[0].SafeZones[0].Center.Y = 690 },
	} {
		// t 提供断言失败报告、子用例和清理注册。
		t.Run(name, func(t *testing.T) {
			// f 接收地图开发测试配置供后续步骤使用。
			f := mapFixture(17)
			f.Maps = []MapTemplate{catalogTown()}
			change(&f)
			// err 接收校验地图过程中可能产生的错误，供后续失败分支处理。
			if err := f.ValidateMap(); err == nil {
				t.Fatal("非法目录未被拒绝")
			}
		})
	}
}

// TestTownBitmapRejectsNonzeroPaddingAndAcceptsOptionalLegacyPetSpawn 核验城镇不应验证legacy宠物出生点，用诊断「有效的最后一格阻挡被误认为补位」定位对应断言。
// t 提供断言失败报告、子用例和清理注册。
func TestTownBitmapRejectsNonzeroPaddingAndAcceptsOptionalLegacyPetSpawn(t *testing.T) {
	// f 接收地图开发测试配置供后续步骤使用。
	f := mapFixture(17)
	// m 接收目录城镇供后续步骤使用。
	m := catalogTown()
	m.Width, m.Height = 3, 3
	m.Source.Width, m.Source.Height = 3, 3
	m.PlayerSpawn, m.PetSpawn = Point{}, Point{X: math.MaxInt, Y: math.MaxInt}
	m.SafeZones = []SafeZone{{ID: 1, Center: Point{X: 1, Y: 1}, Radius: 1}}
	m.CollisionBits = []byte{0, 0}
	f.Maps = []MapTemplate{m}
	// err 接收校验地图过程中可能产生的错误，供后续失败分支处理。
	if err := f.ValidateMap(); err != nil {
		t.Fatal("城镇不应验证legacy宠物出生点：", err)
	}
	f.Maps[0].CollisionBits[1] = 0x02
	if f.ValidateMap() == nil {
		t.Fatal("位图未拒绝尾部补位")
	}
	f.Maps[0].CollisionBits[1] = 0x01
	// err 接收校验地图过程中可能产生的错误，供后续失败分支处理。
	if err := f.ValidateMap(); err != nil {
		t.Fatal("有效的最后一格阻挡被误认为补位：", err)
	}
}

// TestMapTemplateJSONRetainsLegacyFormatAndDecodesBase64Collision 核对「城镇base64碰撞位图未按约定解码」与「旧地图JSON改变了地形含义」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestMapTemplateJSONRetainsLegacyFormatAndDecodesBase64Collision(t *testing.T) {
	// f 记录开发测试配置。
	var f Fixture
	// err 接收反序列化过程中可能产生的错误，供后续失败分支处理。
	if err := json.Unmarshal([]byte(`{"map":{"id":1,"width":17,"height":17,"blocked":[{"x":1,"y":1}],"player_spawn":{"x":8,"y":8},"pet_spawn":{"x":8,"y":7},"monster_spawns":[]},"maps":[{"id":2,"scene_kind":2,"width":3,"height":3,"player_spawn":{"x":0,"y":0},"collision_bits":"AQA=","source":{"code":"0","sha256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","width":3,"height":3}}]}`), &f); err != nil {
		t.Fatal(err)
	}
	if f.Map.SceneKindValue() != SceneKindPersonal || !f.Map.Walkable(Point{}) || f.Map.Walkable(Point{X: 1, Y: 1}) {
		t.Fatal("旧地图JSON改变了地形含义")
	}
	// m 接收地图“by”标识供后续步骤使用，ok 表明查询、校验或类型转换是否成功。
	m, ok := f.MapByID(2)
	if !ok || len(m.CollisionBits) != 2 || m.CollisionBits[0] != 1 || m.CollisionBits[1] != 0 || m.Walkable(Point{}) || !m.Walkable(Point{X: 1}) {
		t.Fatal("城镇base64碰撞位图未按约定解码")
	}
}

// TestMapQueriesFailClosedForInvalidDimensions 核对「非法尺寸产生了可用格子预算」与「非法尺寸产生了安全区」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestMapQueriesFailClosedForInvalidDimensions(t *testing.T) {
	// m 接收遍历地图模板列表时当前的元素。
	for _, m := range []MapTemplate{{}, {Width: -1, Height: 1}, {Width: 1, Height: 0}, {Width: math.MaxInt, Height: math.MaxInt}} {
		if m.CellBudget() != 0 || m.Contains(Point{}) || m.Walkable(Point{}) {
			t.Fatal("非法尺寸产生了可用格子预算")
		}
		// ok 表明查询、校验或类型转换是否成功。
		if _, ok := m.SafeZoneAt(Point{}); ok {
			t.Fatal("非法尺寸产生了安全区")
		}
	}
}

// TestCurrentFixtureCatalogMetadataDoesNotChangeInitialPersonalMap 核对「当前地图缺少唯一配置导出的展示名或资源键」与「当前城镇目录未保留完整来源尺寸或开发入口」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestCurrentFixtureCatalogMetadataDoesNotChangeInitialPersonalMap(t *testing.T) {
	// cfg 保存由加载取得的服务配置，err 接收加载过程中可能产生的错误，供后续失败分支处理。
	cfg, err := Load("../../../conf/dev/server.json")
	if err != nil {
		t.Fatal(err)
	}
	// f 保存当前处理所用的开发测试配置。
	f := &cfg.Fixture
	if f.Map.ID != 1 || f.Map.SceneKindValue() != SceneKindPersonal || f.RuleVersion < 24 || f.Map.NoPets == nil || *f.Map.NoPets || f.Binding == nil || f.Binding.Enabled || !f.SafeZoneBindingEnabled() || f.PetRuntime == nil || !f.PetRuntime.Enabled {
		t.Fatal("生命周期合同改变了个人初始地图、宠物运行状态或随机绑定池")
	}
	// id 接收遍历数值列表时当前的元素。
	for _, id := range []uint64{1, 2, 3, 4, 5} {
		// m 接收地图“by”标识供后续步骤使用，ok 表明查询、校验或类型转换是否成功。
		m, ok := f.MapByID(id)
		if !ok || m.DisplayName == "" || m.ResourceKey == "" {
			t.Fatalf("当前地图%d缺少唯一配置导出的展示名或资源键", id)
		}
	}
	// town 保存由地图“by”标识取得的城镇。
	town, _ := f.MapByID(2)
	if town.SceneKindValue() != SceneKindTown || town.Width != 700 || town.Height != 700 || town.PlayerSpawn != (Point{X: 633, Y: 613}) {
		t.Fatal("当前城镇目录未保留完整来源尺寸或开发入口")
	}
}
