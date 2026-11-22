// 本文件按当前地图与实体占位挑选复活点，区分随机复活和旧规则固定出生点。
package game

import (
	"math"

	"minimir/internal/devconfig"
)

func monsterResidentCells(r *room, world *worldState) map[devconfig.Point]bool {
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
	return occupied
}

// 随机复活也保留隐藏活怪的位置。
func (w *worker) randomMonsterRespawnPoint(r *room, world *worldState, m *devconfig.MapTemplate) (devconfig.Point, bool) {
	occupied := monsterResidentCells(r, world)
	for _, monster := range world.Monsters {
		if monster != nil && monster.HP > 0 {
			occupied[monster.Pos] = true
		}
	}
	return w.freeMonsterPoint(m, occupied)
}

func (w *worker) freeMonsterPoint(m *devconfig.MapTemplate, occupied map[devconfig.Point]bool) (devconfig.Point, bool) {
	cells := m.CellBudget()
	if cells == 0 {
		return devconfig.Point{}, false
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

// 恢复怪物槽位时先为所有冲突活怪预留格子，再一起生效。死亡槽位仍按原截止复活。
func (w *worker) setVisibleMonsterCount(r *room, m *devconfig.MapTemplate, count int) bool {
	if count > len(r.Monsters) {
		return false
	}
	if count <= r.MonsterCount {
		r.MonsterCount = count
		return true
	}
	occupied := monsterResidentCells(r, r.currentWorld())
	for _, monster := range r.Monsters[:r.MonsterCount] {
		if monster.HP > 0 {
			occupied[monster.Pos] = true
		}
	}
	type relocation struct {
		monster *entity
		point   devconfig.Point
	}
	var moves []relocation
	for _, monster := range r.Monsters[r.MonsterCount:count] {
		if monster.HP <= 0 {
			continue
		}
		if occupied[monster.Pos] || !m.Walkable(monster.Pos) {
			moves = append(moves, relocation{monster: monster})
		} else {
			occupied[monster.Pos] = true
		}
	}
	for index := range moves {
		move := &moves[index]
		point, available := w.freeMonsterPoint(m, occupied)
		if !available || move.monster.MovementRevision == math.MaxUint64 {
			return false
		}
		move.point = point
		occupied[point] = true
	}
	for _, move := range moves {
		move.monster.Pos = move.point
		move.monster.MovementRevision++
		move.monster.MovementKind = movementTeleport
	}
	r.MonsterCount = count
	return true
}
