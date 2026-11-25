// 本文件为复活查找空闲落点，避免角色新生命与活动实体重叠。
package game

import (
	"math"

	"minimir/internal/devconfig"
)

// nearestOpenLanding 优先选择配置出生格，再按地格距离和稳定行序寻找空格。
func nearestOpenLanding(m *devconfig.MapTemplate, preferred devconfig.Point, available func(devconfig.Point) bool) (devconfig.Point, bool) {
	if available(preferred) {
		return preferred, true
	}
	best, bestDistance := devconfig.Point{}, math.MaxInt
	for y := 0; y < m.Height; y++ {
		for x := 0; x < m.Width; x++ {
			point := devconfig.Point{X: x, Y: y}
			if score := distance(preferred, point); score < bestDistance && available(point) {
				best, bestDistance = point, score
			}
		}
	}
	return best, bestDistance != math.MaxInt
}

// revivalLanding 在共享场景原子占格。在独享场景只观察活体，角色和宠物尸体不占格。
func (w *worker) revivalLanding(r *room, m *devconfig.MapTemplate) (devconfig.Point, bool) {
	world := r.currentWorld()
	pet := r.worldPet(world)
	var petPosition *devconfig.Point
	if pet != nil && pet.HP > 0 {
		petPosition = &pet.Pos
	}
	if r.townMembership.owner != 0 {
		return w.reserveSharedTownRevival(r, m, petPosition)
	}
	occupied := make(map[devconfig.Point]bool)
	for _, monster := range world.Monsters {
		if monster != nil && monster.HP > 0 {
			occupied[monster.Pos] = true
		}
	}
	if petPosition != nil {
		occupied[*petPosition] = true
	}
	return nearestOpenLanding(m, m.PlayerSpawn, func(point devconfig.Point) bool {
		return !occupied[point] && m.Walkable(point)
	})
}
