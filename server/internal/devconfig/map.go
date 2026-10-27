package devconfig

import (
	"errors"
	"fmt"
)

// 地图只接受已获批准的开发尺寸；预算由实际格数导出，不维护第二份寻路节点数。
func (f *Fixture) GridCellBudget() int {
	if f.Map.Width != f.Map.Height || f.Map.Width != 16 && f.Map.Width != 17 {
		return 0
	}
	return f.Map.Width * f.Map.Height
}

func (f *Fixture) ValidateMap() error {
	budget := f.GridCellBudget()
	if budget == 0 || f.Map.ID == 0 || len(f.Map.MonsterSpawns) < 8 {
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
	return nil
}
