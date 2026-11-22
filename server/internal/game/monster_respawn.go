package game

import "minimir/internal/devconfig"

func (w *worker) randomMonsterRespawnPoint(r *room, world *worldState, m *devconfig.MapTemplate) (devconfig.Point, bool) {
	cells := m.CellBudget()
	if cells == 0 {
		return devconfig.Point{}, false
	}
	occupied := make(map[devconfig.Point]bool, len(world.Monsters)+2)
	if world == r.currentWorld() {
		occupied[r.Player.Pos] = true
	} else if world == r.PersonalWorld {
		// 旧地图受延迟事件推进时，仍给将返回原点的玩家留出格子。
		occupied[world.PlayerPosition] = true
	}
	if pet := r.worldPet(world); pet != nil && pet.HP > 0 {
		occupied[pet.Pos] = true
	}
	for _, monster := range world.Monsters {
		if monster != nil && monster.HP > 0 {
			// 保留尚未显示的活怪位置，增加怪物数量时也不会出现两个怪挤在同一格。
			occupied[monster.Pos] = true
		}
	}
	free := make([]devconfig.Point, 0, cells)
	for y := 0; y < m.Height; y++ {
		for x := 0; x < m.Width; x++ {
			point := devconfig.Point{X: x, Y: y}
			if !occupied[point] && m.Walkable(point) {
				free = append(free, point)
			}
		}
	}
	if len(free) == 0 {
		return devconfig.Point{}, false
	}
	return free[w.draw(int64(len(free)))], true
}
