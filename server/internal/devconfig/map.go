package devconfig

import (
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"strings"
)

const (
	SceneKindPersonal uint32 = 1
	SceneKindTown     uint32 = 2
)

// 地图模板只有静态地形与来源信息，不保存实例、角色位置或战斗现场。
type MapTemplate struct {
	ID            uint64             `json:"id"`
	DisplayName   string             `json:"display_name,omitempty"`
	ResourceKey   string             `json:"resource_key,omitempty"`
	Width         int                `json:"width"`
	Height        int                `json:"height"`
	SceneKind     uint32             `json:"scene_kind,omitempty"`
	Blocked       []Point            `json:"blocked,omitempty"`
	CollisionBits []byte             `json:"collision_bits,omitempty"`
	PlayerSpawn   Point              `json:"player_spawn"`
	PetSpawn      Point              `json:"pet_spawn"`
	MonsterSpawns []Point            `json:"monster_spawns,omitempty"`
	SafeZones     []SafeZone         `json:"safe_zones,omitempty"`
	Source        *SourceMapIdentity `json:"source,omitempty"`
}

// 完整来源尺寸与运行目录尺寸相等；裁剪窗口不能借同一来源声明冒充完整地图。
type SourceMapIdentity struct {
	Code   string `json:"code"`
	SHA256 string `json:"sha256"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
}

type SafeZone struct {
	ID     uint64 `json:"id"`
	Center Point  `json:"center"`
	Radius int    `json:"radius"`
}

// 旧个人图没有scene_kind；目录中的额外地图仍必须显式声明城镇类型。
func (m *MapTemplate) SceneKindValue() uint32 {
	if m.SceneKind == 0 {
		return SceneKindPersonal
	}
	return m.SceneKind
}

func (m *MapTemplate) CellBudget() int {
	if m.Width <= 0 || m.Height <= 0 || m.Width > math.MaxInt32 || m.Height > math.MaxInt32 || m.Width > math.MaxInt/m.Height {
		return 0
	}
	return m.Width * m.Height
}

func (m *MapTemplate) Contains(p Point) bool {
	return m.CellBudget() != 0 && p.X >= 0 && p.Y >= 0 && p.X < m.Width && p.Y < m.Height
}

func collisionByteCount(cells int) int {
	count := cells / 8
	if cells%8 != 0 {
		count++
	}
	return count
}

// 城镇位图按y*width+x连续编号，低位在先；非法坐标或不完整位图关闭通行，避免越界。
func (m *MapTemplate) Walkable(p Point) bool {
	if !m.Contains(p) {
		return false
	}
	if m.SceneKindValue() == SceneKindTown {
		cells := m.CellBudget()
		if len(m.Blocked) != 0 || len(m.CollisionBits) != collisionByteCount(cells) {
			return false
		}
		cell := p.Y*m.Width + p.X
		return m.CollisionBits[cell/8]&(1<<uint(cell%8)) == 0
	}
	if m.SceneKindValue() != SceneKindPersonal || len(m.CollisionBits) != 0 {
		return false
	}
	for _, blocked := range m.Blocked {
		if blocked == p {
			return false
		}
	}
	return true
}

// 来源按配置顺序取首个包含当前格子的安全区，方形范围包含边界。
func (m *MapTemplate) SafeZoneAt(p Point) (*SafeZone, bool) {
	if !m.Contains(p) {
		return nil, false
	}
	for i := range m.SafeZones {
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

func (f *Fixture) MapByID(id uint64) (*MapTemplate, bool) {
	if id == 0 {
		return nil, false
	}
	if f.Map.ID == id {
		return &f.Map, true
	}
	for i := range f.Maps {
		if f.Maps[i].ID == id {
			return &f.Maps[i], true
		}
	}
	return nil, false
}

// 个人地图继续只接受已获批准的开发尺寸；不因增加城镇而扩大其规则。
func (f *Fixture) GridCellBudget() int {
	if f.Map.SceneKindValue() != SceneKindPersonal || f.Map.Width != f.Map.Height || f.Map.Width != 16 && f.Map.Width != 17 {
		return 0
	}
	return f.Map.CellBudget()
}

func (f *Fixture) ValidateMap() error {
	budget := f.GridCellBudget()
	if budget == 0 || f.Map.ID == 0 || f.Map.ID > math.MaxInt64 || len(f.Map.CollisionBits) != 0 || len(f.Map.MonsterSpawns) < 8 {
		return errors.New("开发地图须为已批准的16×16或17×17，并提供至少8个怪物槽位")
	}
	inBounds := func(p Point) bool {
		return p.X >= 0 && p.Y >= 0 && p.X < f.Map.Width && p.Y < f.Map.Height
	}
	blocked := make(map[Point]bool, len(f.Map.Blocked))
	for _, p := range f.Map.Blocked {
		if !inBounds(p) || blocked[p] {
			return fmt.Errorf("地图阻挡格越界或重复：(%d,%d)", p.X, p.Y)
		}
		blocked[p] = true
	}
	walkable := func(p Point) bool { return inBounds(p) && !blocked[p] }
	spawns := append([]Point{f.Map.PlayerSpawn, f.Map.PetSpawn}, f.Map.MonsterSpawns...)
	occupied := make(map[Point]bool, len(spawns))
	for _, p := range spawns {
		if !walkable(p) || occupied[p] {
			return fmt.Errorf("地图出生格越界、阻挡或重复：(%d,%d)", p.X, p.Y)
		}
		occupied[p] = true
	}
	// 只检查静态地面连通性；不把动态角色/怪物占位当永久障碍，也不限制运行时重生叠格。
	queue := []Point{f.Map.PlayerSpawn}
	seen := map[Point]bool{f.Map.PlayerSpawn: true}
	for head := 0; head < len(queue) && head < budget; head++ {
		cur := queue[head]
		for dy := -1; dy <= 1; dy++ {
			for dx := -1; dx <= 1; dx++ {
				if dx == 0 && dy == 0 {
					continue
				}
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
	for _, p := range spawns {
		if !seen[p] {
			return fmt.Errorf("地图出生格与玩家出生点静态不连通：(%d,%d)", p.X, p.Y)
		}
	}
	if err := f.Map.validateSafeZones(); err != nil {
		return err
	}
	return f.validateMapDirectory()
}

func (f *Fixture) validateMapDirectory() error {
	seen := map[uint64]bool{f.Map.ID: true}
	for i := range f.Maps {
		m := &f.Maps[i]
		if m.ID == 0 || m.ID > math.MaxInt64 || seen[m.ID] || m.SceneKind != SceneKindTown {
			return errors.New("额外地图须显式为城镇，地图ID须唯一且可持久化")
		}
		seen[m.ID] = true
		if err := m.validateTown(); err != nil {
			return fmt.Errorf("城镇地图%d：%w", m.ID, err)
		}
	}
	return nil
}

func (m *MapTemplate) validateTown() error {
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
	if _, err := hex.DecodeString(m.Source.SHA256); err != nil {
		return errors.New("城镇来源SHA256须为64位十六进制")
	}
	return m.validateSafeZones()
}

const sha256HexLength = 64

func (m *MapTemplate) validateSafeZones() error {
	seen := map[uint64]bool{}
	for _, zone := range m.SafeZones {
		if zone.ID == 0 || seen[zone.ID] || !m.Walkable(zone.Center) || zone.Radius < 0 || zone.Radius > zone.Center.X || zone.Radius > zone.Center.Y || zone.Radius >= m.Width-zone.Center.X || zone.Radius >= m.Height-zone.Center.Y {
			return errors.New("安全区ID须唯一，中心可走且含边界的方形范围不得越出地图")
		}
		seen[zone.ID] = true
	}
	return nil
}
