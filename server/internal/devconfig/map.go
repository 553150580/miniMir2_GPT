// 本文件定义个人地图、城镇地图目录、碰撞位图、安全区和出生点查询；验证地图的网格及可达边界。

package devconfig

import (
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"strings"
)

const (
	// SceneKindPersonal 规定场景类型个人的初始取值。
	SceneKindPersonal uint32 = 1
	// SceneKindTown 规定场景类型城镇的初始取值。
	SceneKindTown uint32 = 2
	// SceneKindPersonalBoss 表示独享首领挑战场景。
	SceneKindPersonalBoss uint32 = 3
	// SceneKindWorldBoss 表示同运行服共享的首领场次。
	SceneKindWorldBoss uint32 = 4

	// maxPersonalGridSide 规定最大个人网格“side”的初始取值。
	maxPersonalGridSide = 17
	// MaxPersonalGridCells 是兼容历史模板所需的个人地图最大格数。
	MaxPersonalGridCells = maxPersonalGridSide * maxPersonalGridSide
)

// 地图模板只有静态地形与来源信息，不保存实例、角色位置或战斗现场。
type MapTemplate struct {
	// ID 区分当前步骤指向的目标对象。
	ID uint64 `json:"id"`
	// DisplayName 记录显示名称，使用文本表示。
	DisplayName string `json:"display_name,omitempty"`
	// ResourceKey 记录“resource”键，使用文本表示。
	ResourceKey string `json:"resource_key,omitempty"`
	// Width 记录宽度，使用数值表示。
	Width int `json:"width"`
	// Height 记录高度，使用数值表示。
	Height int `json:"height"`
	// SceneKind 记录场景类型，使用数值表示。
	SceneKind uint32 `json:"scene_kind,omitempty"`
	// CompactTown 标记是否处于紧凑城镇状态。
	CompactTown bool `json:"compact_town,omitempty"`
	// Blocked 记录阻挡，使用点位列表表示。
	Blocked []Point `json:"blocked,omitempty"`
	// CollisionBits 记录“collision”二进制位，使用字节序列表示。
	CollisionBits []byte `json:"collision_bits,omitempty"`
	// SourceStaticBits 记录来源“static”二进制位，使用字节序列表示。
	SourceStaticBits []byte `json:"source_static_bits,omitempty"`
	// NoTeleport 标记是否处于“no”瞬移状态。
	NoTeleport *bool `json:"no_teleport,omitempty"`
	// NoPets 标记是否处于“no”宠物集合状态。
	NoPets *bool `json:"no_pets,omitempty"`
	// PlayerSpawn 记录玩家生成或出生点，使用点位表示。
	PlayerSpawn Point `json:"player_spawn"`
	// PetSpawn 记录宠物生成或出生点，使用点位表示。
	PetSpawn Point `json:"pet_spawn"`
	// MonsterSpawns 记录怪物生成或出生点集合，使用点位列表表示。
	MonsterSpawns []Point `json:"monster_spawns,omitempty"`
	// MonsterSpawnIDs 按出生槽顺序引用本图的普通怪模板。
	MonsterSpawnIDs []uint32 `json:"monster_spawn_ids,omitempty"`
	// InitialMonsterCount 是冷入本图时启用的普通怪槽位数。
	InitialMonsterCount int `json:"initial_monster_count,omitempty"`
	// SafeZones 记录安全区域集合，使用安全区域列表表示。
	SafeZones []SafeZone `json:"safe_zones,omitempty"`
	// LegacySafeZoneIDs 记录兼容旧数据安全区域i“ds”，使用数值列表表示。
	LegacySafeZoneIDs []uint64 `json:"legacy_safe_zone_ids,omitempty"`
	// NPCs 记录“np”“cs”，使用城镇NPC列表表示。
	NPCs []TownNPC `json:"npcs,omitempty"`
	// Source 记录来源，使用来源地图身份表示。
	Source *SourceMapIdentity `json:"source,omitempty"`
}

// 完整来源尺寸与运行目录尺寸相等；裁剪窗口不能借同一来源声明冒充完整地图。
type SourceMapIdentity struct {
	// Code 记录结果码，使用文本表示。
	Code string `json:"code"`
	// SHA256 记录“sha”“256”，使用文本表示。
	SHA256 string `json:"sha256"`
	// Width 记录宽度，使用数值表示。
	Width int `json:"width"`
	// Height 记录高度，使用数值表示。
	Height int `json:"height"`
}

// SafeZone 保存安全区域所需的状态。
type SafeZone struct {
	// ID 区分当前步骤指向的目标对象。
	ID uint64 `json:"id"`
	// Center 记录中心格，使用点位表示。
	Center Point `json:"center"`
	// Radius 记录“radius”，使用数值表示。
	Radius int `json:"radius"`
}

// 旧主个人图没有 scene_kind，额外地图须显式声明场景类型。
// m 是调用该方法的地图模板实例。
func (m *MapTemplate) SceneKindValue() uint32 {
	if m.SceneKind == 0 {
		return SceneKindPersonal
	}
	return m.SceneKind
}

// CellBudget 返回地图格预算供调用方使用。
// m 是调用该方法的地图模板实例。
func (m *MapTemplate) CellBudget() int {
	if m.Width <= 0 || m.Height <= 0 || m.Width > math.MaxInt32 || m.Height > math.MaxInt32 || m.Width > math.MaxInt/m.Height {
		return 0
	}
	return m.Width * m.Height
}

// Contains 判断“contains”是否满足要求。
// m 是调用该方法的地图模板实例，p 记录点位。
func (m *MapTemplate) Contains(p Point) bool {
	return m.CellBudget() != 0 && p.X >= 0 && p.Y >= 0 && p.X < m.Width && p.Y < m.Height
}

// collisionByteCount 返回“collision”字节数量供调用方使用。
// cells 记录地图格集合，使用数值表示。
func collisionByteCount(cells int) int {
	// count 保存根据地图格集合与当前数据计算的数量。
	count := cells / 8
	if cells%8 != 0 {
		count++
	}
	return count
}

// 城镇位图按y*width+x连续编号，低位在先；非法坐标或不完整位图关闭通行，避免越界。
// m 是调用该方法的地图模板实例，p 记录点位。
func (m *MapTemplate) Walkable(p Point) bool {
	if !m.Contains(p) {
		return false
	}
	if m.SceneKindValue() == SceneKindTown && !m.CompactTown {
		// cells 保存由地图格预算取得的地图格集合。
		cells := m.CellBudget()
		if len(m.Blocked) != 0 || len(m.CollisionBits) != collisionByteCount(cells) {
			return false
		}
		// cell 保存根据纵坐标与宽度的计算结果与横坐标计算的地图格。
		cell := p.Y*m.Width + p.X
		return m.CollisionBits[cell/8]&(1<<uint(cell%8)) == 0
	}
	if m.SceneKindValue() != SceneKindPersonal && m.SceneKindValue() != SceneKindPersonalBoss && m.SceneKindValue() != SceneKindWorldBoss && !(m.SceneKindValue() == SceneKindTown && m.CompactTown) || len(m.CollisionBits) != 0 {
		return false
	}
	// blocked 接收遍历阻挡时当前的元素。
	for _, blocked := range m.Blocked {
		if blocked == p {
			return false
		}
	}
	return true
}

// 瞬移只核原图静态Walk位；闭门及动态占格不能混入此来源判定。
// m 是调用该方法的地图模板实例，p 记录点位。
func (m *MapTemplate) TeleportWalkable(p Point) bool {
	if !m.Contains(p) {
		return false
	}
	if m.SceneKindValue() == SceneKindPersonal || m.SceneKindValue() == SceneKindTown && m.CompactTown {
		return m.Walkable(p)
	}
	if m.SceneKindValue() != SceneKindTown || m.Source == nil || len(m.SourceStaticBits) != collisionByteCount(m.CellBudget()) {
		return false
	}
	// cell 保存根据纵坐标与宽度的计算结果与横坐标计算的地图格。
	cell := p.Y*m.Width + p.X
	return m.SourceStaticBits[cell/8]&(1<<uint(cell%8)) == 0
}

// 紧凑城镇只维护作者阻挡格，导航从同一份阻挡生成位图。
// m 是调用该方法的地图模板实例。
func (m *MapTemplate) TownCollisionBits() []byte {
	if !m.CompactTown {
		return m.CollisionBits
	}
	// bits 创建用于维护二进制位的容器或通道。
	bits := make([]byte, collisionByteCount(m.CellBudget()))
	// p 接收遍历阻挡时当前的元素。
	for _, p := range m.Blocked {
		if !m.Contains(p) {
			return nil
		}
		// cell 保存根据纵坐标与宽度的计算结果与横坐标计算的地图格。
		cell := p.Y*m.Width + p.X
		bits[cell/8] |= 1 << uint(cell%8)
	}
	return bits
}

// 来源按配置顺序取首个包含当前格子的安全区，方形范围包含边界。
// m 是调用该方法的地图模板实例，p 记录点位。
func (m *MapTemplate) SafeZoneAt(p Point) (*SafeZone, bool) {
	if !m.Contains(p) {
		return nil, false
	}
	// i 标记遍历安全区域集合时访问的下标。
	for i := range m.SafeZones {
		// zone 保存当前处理所用的区域。
		zone := &m.SafeZones[i]
		if !m.Contains(zone.Center) || zone.Radius < 0 || zone.Radius > zone.Center.X || zone.Radius > zone.Center.Y || zone.Radius >= m.Width-zone.Center.X || zone.Radius >= m.Height-zone.Center.Y {
			continue
		}
		if p.X >= zone.Center.X-zone.Radius && p.X <= zone.Center.X+zone.Radius && p.Y >= zone.Center.Y-zone.Radius && p.Y <= zone.Center.Y+zone.Radius {
			return zone, true
		}
	}
	return nil, false
}

// MapByID 处理地图“by”标识。
// f 是调用该方法的开发测试配置实例，id 区分当前步骤指向的目标对象。
func (f *Fixture) MapByID(id uint64) (*MapTemplate, bool) {
	if id == 0 {
		return nil, false
	}
	if f.Map.ID == id {
		return &f.Map, true
	}
	// i 标记遍历地图集合时访问的下标。
	for i := range f.Maps {
		if f.Maps[i].ID == id {
			return &f.Maps[i], true
		}
	}
	return nil, false
}

// 规则26接入13格挂机图，历史16格和17格模板仍可读取。
// f 是调用该方法的开发测试配置实例。
func (f *Fixture) GridCellBudget() int {
	return f.Map.personalGridCellBudget(f.RuleVersion)
}

// personalGridCellBudget 检查指定普通地图能否使用受限格子战斗工作区。
func (m *MapTemplate) personalGridCellBudget(ruleVersion uint64) int {
	if m.SceneKindValue() != SceneKindPersonal || m.Width != m.Height {
		return 0
	}
	switch m.Width {
	case 16, maxPersonalGridSide:
		return m.CellBudget()
	case 13:
		if ruleVersion >= 26 {
			return m.CellBudget()
		}
	}
	return 0
}

// ValidateMap 校验地图。
// f 是调用该方法的开发测试配置实例。
func (f *Fixture) ValidateMap() error {
	if err := f.validatePersonalMap(&f.Map); err != nil {
		return err
	}
	return f.validateMapDirectory()
}

// validatePersonalMap 校验普通地图的出生槽、静态连通性及模板引用。
func (f *Fixture) validatePersonalMap(m *MapTemplate) error {
	budget := m.personalGridCellBudget(f.RuleVersion)
	if budget == 0 || m.ID == 0 || m.ID > math.MaxInt64 || m.CompactTown || len(m.LegacySafeZoneIDs) != 0 || len(m.NPCs) != 0 || len(m.CollisionBits) != 0 || len(m.MonsterSpawns) < 8 {
		return errors.New("开发地图须为16×16、17×17或规则26起的13×13，并提供至少8个怪物槽位")
	}
	if err := f.validateOrdinaryMonsterRoster(m); err != nil {
		return err
	}
	// inBounds 封装输入“bounds”对应的后续处理，p 记录点位。
	inBounds := func(p Point) bool {
		return p.X >= 0 && p.Y >= 0 && p.X < m.Width && p.Y < m.Height
	}
	// blocked 创建用于维护阻挡的容器或通道。
	blocked := make(map[Point]bool, len(m.Blocked))
	// p 接收遍历阻挡时当前的元素。
	for _, p := range m.Blocked {
		if !inBounds(p) || blocked[p] {
			return fmt.Errorf("地图阻挡格越界或重复：(%d,%d)", p.X, p.Y)
		}
		blocked[p] = true
	}
	// walkable 封装可走格对应的后续处理，p 记录点位。
	walkable := func(p Point) bool { return inBounds(p) && !blocked[p] }
	// spawns 保留加入新条目后的生成或出生点集合。
	spawns := append([]Point{m.PlayerSpawn, m.PetSpawn}, m.MonsterSpawns...)
	// occupied 创建用于维护已占用的容器或通道。
	occupied := make(map[Point]bool, len(spawns))
	// p 接收遍历生成或出生点集合时当前的元素。
	for _, p := range spawns {
		if !walkable(p) || occupied[p] {
			return fmt.Errorf("地图出生格越界、阻挡或重复：(%d,%d)", p.X, p.Y)
		}
		occupied[p] = true
	}
	// 只检查配置出生点的静态连通性；运行时由各自房间处理动态占位。
	// queue 初始化用作队列的点位列表。
	queue := []Point{m.PlayerSpawn}
	// seen 建立带初始值的按点位索引的布尔状态映射，供本段继续处理。
	seen := map[Point]bool{m.PlayerSpawn: true}
	// head 规定头部的初始取值。
	for head := 0; head < len(queue) && head < budget; head++ {
		// cur 保存从队列取出的“cur”。
		cur := queue[head]
		// dy 保存当前处理所用的纵向差值。
		for dy := -1; dy <= 1; dy++ {
			// dx 保存当前处理所用的横向差值。
			for dx := -1; dx <= 1; dx++ {
				if dx == 0 && dy == 0 {
					continue
				}
				// point 组装点位所需的数据。
				point := Point{X: cur.X + dx, Y: cur.Y + dy}
				if seen[point] || !walkable(point) {
					continue
				}
				if dx != 0 && dy != 0 && (!walkable(Point{X: cur.X + dx, Y: cur.Y}) || !walkable(Point{X: cur.X, Y: cur.Y + dy})) {
					continue
				}
				seen[point] = true
				queue = append(queue, point)
			}
		}
	}
	// p 接收遍历生成或出生点集合时当前的元素。
	for _, p := range spawns {
		if !seen[p] {
			return fmt.Errorf("地图出生格与玩家出生点静态不连通：(%d,%d)", p.X, p.Y)
		}
	}
	return m.validateSafeZones()
}

// validateMapDirectory 校验地图目录。
// f 是调用该方法的开发测试配置实例。
func (f *Fixture) validateMapDirectory() error {
	// seen 建立带初始值的按数值索引的布尔状态映射，供本段继续处理。
	seen := map[uint64]bool{f.Map.ID: true}
	// i 标记遍历地图集合时访问的下标。
	for i := range f.Maps {
		// m 保存当前处理所用的地图集合中的指定项。
		m := &f.Maps[i]
		if m.ID == 0 || m.ID > math.MaxInt64 || seen[m.ID] {
			return errors.New("地图目录ID须唯一且可持久化")
		}
		seen[m.ID] = true
		if m.SceneKind == SceneKindPersonal && f.OrdinaryMonsterRosterEnabled() {
			if m.Width != 13 || m.Height != 13 || m.Source != nil || len(m.SourceStaticBits) != 0 || strings.TrimSpace(m.DisplayName) == "" || strings.TrimSpace(m.ResourceKey) == "" {
				return errors.New("额外普通地图须显式提供13×13地形、名称和资源键")
			}
			if err := f.validatePersonalMap(m); err != nil {
				return fmt.Errorf("普通地图%d：%w", m.ID, err)
			}
			continue
		}
		if m.SceneKind != SceneKindTown {
			return errors.New("额外地图须显式为城镇或启用当前普通怪物目录的个人图")
		}
		if len(m.MonsterSpawnIDs) != 0 || m.InitialMonsterCount != 0 {
			return errors.New("城镇不能配置普通怪物目录")
		}
		if f.RuleVersion < 24 && m.CompactTown || !m.CompactTown && (len(m.LegacySafeZoneIDs) != 0 || len(m.NPCs) != 0) {
			return errors.New("紧凑城镇及历史安全区映射仅供rule24及后续紧凑地图")
		}
		// err 接收校验城镇过程中可能产生的错误，供后续失败分支处理。
		if err := m.validateTown(); err != nil {
			return fmt.Errorf("城镇地图%d：%w", m.ID, err)
		}
	}
	return nil
}

// validateTown 校验城镇。
// m 是调用该方法的地图模板实例。
func (m *MapTemplate) validateTown() error {
	if m.CompactTown {
		return m.validateCompactTown()
	}
	// cells 保存由地图格预算取得的地图格集合。
	cells := m.CellBudget()
	if cells == 0 || len(m.Blocked) != 0 || len(m.MonsterSpawns) != 0 {
		return errors.New("城镇须提供有效完整尺寸与独立碰撞位图，不沿用个人阻挡表或怪物槽")
	}
	if len(m.CollisionBits) != collisionByteCount(cells) {
		return errors.New("城镇碰撞位图字节数与地图格数不一致")
	}
	if cells%8 != 0 && m.CollisionBits[len(m.CollisionBits)-1]>>uint(cells%8) != 0 {
		return errors.New("城镇碰撞位图尾部补位必须为0")
	}
	if !m.Walkable(m.PlayerSpawn) {
		return errors.New("城镇开发入口须为可走格")
	}
	if m.Source == nil || strings.TrimSpace(m.Source.Code) == "" || len(m.Source.SHA256) != sha256HexLength || m.Source.Width != m.Width || m.Source.Height != m.Height {
		return errors.New("城镇须提供来源代码、SHA256与匹配完整地图的来源尺寸")
	}
	// err 接收解码文本过程中可能产生的错误，供后续失败分支处理。
	if _, err := hex.DecodeString(m.Source.SHA256); err != nil {
		return errors.New("城镇来源SHA256须为64位十六进制")
	}
	return m.validateSafeZones()
}

// validateCompactTown 校验紧凑城镇。
// m 是调用该方法的地图模板实例。
func (m *MapTemplate) validateCompactTown() error {
	if m.Width != 17 || m.Height != 17 || m.Source != nil || len(m.CollisionBits) != 0 || len(m.SourceStaticBits) != 0 || len(m.MonsterSpawns) != 0 || len(m.SafeZones) != 1 || strings.TrimSpace(m.DisplayName) == "" || strings.TrimSpace(m.ResourceKey) == "" {
		return errors.New("紧凑城镇须为独立17×17布局、具有唯一安全区，不沿用原图位图或个人怪物槽")
	}
	// seen 创建用于维护“seen”的容器或通道。
	seen := make(map[Point]bool, len(m.Blocked))
	// p 接收遍历阻挡时当前的元素。
	for _, p := range m.Blocked {
		if !m.Contains(p) || seen[p] {
			return fmt.Errorf("紧凑城镇阻挡格越界或重复：(%d,%d)", p.X, p.Y)
		}
		seen[p] = true
	}
	if !m.Walkable(m.PlayerSpawn) {
		return errors.New("紧凑城镇入口须为可走格")
	}
	// err 接收校验城镇“np”“cs”过程中可能产生的错误，供后续失败分支处理。
	if err := m.validateTownNPCs(seen); err != nil {
		return err
	}
	return m.validateSafeZones()
}

// sha256HexLength 规定“sha”“256”十六进制长度的初始取值。
const sha256HexLength = 64

// validateSafeZones 校验安全区域集合。
// m 是调用该方法的地图模板实例。
func (m *MapTemplate) validateSafeZones() error {
	// seen 建立带初始值的按数值索引的布尔状态映射，供本段继续处理。
	seen := map[uint64]bool{}
	// zone 接收遍历安全区域集合时当前的元素。
	for _, zone := range m.SafeZones {
		if zone.ID == 0 || seen[zone.ID] || !m.Walkable(zone.Center) || zone.Radius < 0 || zone.Radius > zone.Center.X || zone.Radius > zone.Center.Y || zone.Radius >= m.Width-zone.Center.X || zone.Radius >= m.Height-zone.Center.Y {
			return errors.New("安全区ID须唯一，中心可走且含边界的方形范围不得越出地图")
		}
		seen[zone.ID] = true
	}
	return nil
}
