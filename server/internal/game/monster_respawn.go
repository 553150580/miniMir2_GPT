// 本文件按当前地图与实体占位挑选复活点，区分随机复活和旧规则固定出生点。
package game

import "minimir/internal/devconfig"

// randomMonsterRespawnPoint 处理随机值怪物复活点位。
// w 是调用该方法的房间执行器实例，r 指向当前执行器拥有并更新的目标房间，world 记录场景世界，使用场景世界状态表示，m 记录地图模板。
func (w *worker) randomMonsterRespawnPoint(r *room, world *worldState, m *devconfig.MapTemplate) (devconfig.Point, bool) {
	// cells 保存由地图格预算取得的地图格集合。
	cells := m.CellBudget()
	if cells == 0 {
		return devconfig.Point{}, false
	}
	// occupied 创建用于维护已占用的容器或通道。
	occupied := make(map[devconfig.Point]bool, len(world.Monsters)+2)
	if world == r.currentWorld() {
		occupied[r.Player.Pos] = true
	} else if world == r.PersonalWorld {
		// 旧地图受延迟事件推进时，仍给将返回原点的玩家留出格子。
		occupied[world.PlayerPosition] = true
	}
	// pet 保存由场景世界宠物取得的宠物。
	if pet := r.worldPet(world); pet != nil && pet.HP > 0 {
		occupied[pet.Pos] = true
	}
	// monster 接收遍历怪物集合时当前的元素。
	for _, monster := range world.Monsters {
		if monster != nil && monster.HP > 0 {
			// 保留尚未显示的活怪位置，增加怪物数量时也不会出现两个怪挤在同一格。
			occupied[monster.Pos] = true
		}
	}
	// free 创建用于维护免费或空闲的容器或通道。
	free := make([]devconfig.Point, 0, cells)
	// y 规定纵坐标的初始取值。
	for y := 0; y < m.Height; y++ {
		// x 规定横坐标的初始取值。
		for x := 0; x < m.Width; x++ {
			// point 组装点位所需的数据。
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
