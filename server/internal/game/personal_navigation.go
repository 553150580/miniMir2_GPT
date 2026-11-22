package game

import "minimir/internal/devconfig"

type personalNavigationNode struct {
	cell      uint16
	firstCell uint16
}

// 同一 worker 串行处理它的房间，个人图寻路可复用这套定长空间。
type personalNavigationWorkspace struct {
	queue       [devconfig.MaxPersonalGridCells]personalNavigationNode
	unavailable [(devconfig.MaxPersonalGridCells + 63) / 64]uint64
}

func (b *personalNavigationWorkspace) markUnavailable(cell int) {
	b.unavailable[cell/64] |= uint64(1) << (cell % 64)
}

func (b *personalNavigationWorkspace) markActor(m *devconfig.MapTemplate, actor, self *entity) {
	if actor != nil && actor != self && actor.HP > 0 && m.Contains(actor.Pos) {
		b.markUnavailable(actor.Pos.Y*m.Width + actor.Pos.X)
	}
}

func (b *personalNavigationWorkspace) findFirstStep(m *devconfig.MapTemplate, r *room, self *entity, target devconfig.Point) (devconfig.Point, bool) {
	// 动态占格和已访问格都会禁止重复入队，可在本次搜索内共用一份位图。
	clear(b.unavailable[:])
	b.markActor(m, &r.Player, self)
	b.markActor(m, r.worldPet(r.currentWorld()), self)
	for _, monster := range r.Monsters[:r.MonsterCount] {
		b.markActor(m, monster, self)
	}

	start := self.Pos
	startCell := start.Y*m.Width + start.X
	b.markUnavailable(startCell)
	b.queue[0] = personalNavigationNode{cell: uint16(startCell)}
	tail := 1
	startDistance := distance(start, target)

	for head := 0; head < tail; head++ {
		node := b.queue[head]
		cell := int(node.cell)
		cur := devconfig.Point{X: cell % m.Width, Y: cell / m.Width}
		for _, d := range directions {
			q := devconfig.Point{X: cur.X + d.X, Y: cur.Y + d.Y}
			if !m.Contains(q) {
				continue
			}
			nextCell := q.Y*m.Width + q.X
			if b.unavailable[nextCell/64]&(uint64(1)<<(nextCell%64)) != 0 || !m.Walkable(q) {
				continue
			}
			// 斜向移动的两个侧格只检查静态地形。
			if d.X != 0 && d.Y != 0 &&
				(!m.Walkable(devconfig.Point{X: q.X, Y: cur.Y}) || !m.Walkable(devconfig.Point{X: cur.X, Y: q.Y})) {
				continue
			}

			firstCell := node.firstCell
			if head == 0 {
				firstCell = uint16(nextCell)
			}
			// 延续原规则，到达目标或首次发现更接近目标的相邻格时取首步。
			if q == target || startDistance > 1 && distance(q, target) == 1 {
				return devconfig.Point{X: int(firstCell) % m.Width, Y: int(firstCell) / m.Width}, true
			}
			b.markUnavailable(nextCell)
			b.queue[tail] = personalNavigationNode{cell: uint16(nextCell), firstCell: firstCell}
			tail++
		}
	}
	return devconfig.Point{}, false
}
