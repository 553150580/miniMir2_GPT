// 本文件为个人地图复用分片寻路工作区，按当前地形和实体构建临时占位并计算首步。
package game

import "minimir/internal/devconfig"

// personalNavigationNode 记录一个待展开网格和通向它的第一步。
type personalNavigationNode struct {
	cell      uint16 // 当前待展开格的一维坐标。
	firstCell uint16 // 从起点通往当前格的那一步，用于无需重建整条路径地返回首步。
}

// 同一 worker 串行处理它的房间，个人图寻路可复用这套定长空间。
type personalNavigationWorkspace struct {
	queue       [devconfig.MaxPersonalGridCells]personalNavigationNode // 一次广度优先搜索使用的定长队列。
	unavailable [(devconfig.MaxPersonalGridCells + 63) / 64]uint64     // 本次搜索的动态占用与已访问位图，每次重置。
}

// markUnavailable 将已占用格或已访问格设为不可再次入队。
// b 是调用该方法的个人导航搜索工作区实例，cell 记录地图格，使用数值表示。
func (b *personalNavigationWorkspace) markUnavailable(cell int) {
	b.unavailable[cell/64] |= uint64(1) << (cell % 64)
}

// markActor 将地图内除搜索者外的存活身体写入临时占位。
// b 是调用该方法的个人导航搜索工作区实例，m 记录地图模板，actor 记录行动实体，self 记录“self”，使用实体表示。
func (b *personalNavigationWorkspace) markActor(m *devconfig.MapTemplate, actor, self *entity) {
	if actor != nil && actor != self && actor.HP > 0 && m.Contains(actor.Pos) {
		b.markUnavailable(actor.Pos.Y*m.Width + actor.Pos.X)
	}
}

// findFirstStep 从当前实体位置执行广度优先搜索，返回原规则允许的第一步。
// b 是调用该方法的个人导航搜索工作区实例，m 记录地图模板，r 指向当前执行器拥有并更新的目标房间，self 记录“self”，使用实体表示。
// target 记录目标，使用点位表示。
func (b *personalNavigationWorkspace) findFirstStep(m *devconfig.MapTemplate, r *room, self *entity, target devconfig.Point) (devconfig.Point, bool) {
	// 动态占格和已访问格都会禁止重复入队，可在本次搜索内共用一份位图。
	clear(b.unavailable[:])
	if arena := r.worldBossCombat; arena != nil {
		arena.eachLiving(func(actor *entity) { b.markActor(m, actor, self) })
	} else if arena := r.dungeonCombat; arena != nil {
		arena.eachLiving(func(actor *entity) { b.markActor(m, actor, self) })
	} else if arena := r.currentWorld().mirrorCombat; arena != nil {
		arena.eachBody(func(actor *entity) { b.markActor(m, actor, self) })
	} else {
		b.markActor(m, &r.Player, self)
		b.markActor(m, r.worldPet(r.currentWorld()), self)
		for _, monster := range r.Monsters[:r.MonsterCount] {
			b.markActor(m, monster, self)
		}
	}

	// start 保存位置，用于当前步骤中的起点或开始时间。
	start := self.Pos
	// 玩家明确点击的空闲地格必须真正可达。追击与临时占用目标仍沿用靠近原目标的规则。
	exact := false
	if self == &r.Player && r.Destination != nil && *r.Destination == target &&
		r.Navigation.Status == navigationActive && m.Walkable(target) {
		goal := target.Y*m.Width + target.X
		exact = b.unavailable[goal/64]&(uint64(1)<<(goal%64)) == 0
	}
	// preferred 优先沿点击目标的朝向，等长路径以同向和最小偏转为先。
	preferred := 0
	if exact {
		preferred = int(facing(start, target, 0))
	}
	offsets := [...]int{0, 1, 7, 2, 6, 3, 5, 4}
	// startCell 保存根据纵坐标与宽度的计算结果与横坐标计算的起点或开始时间地图格。
	startCell := start.Y*m.Width + start.X
	b.markUnavailable(startCell)
	b.queue[0] = personalNavigationNode{cell: uint16(startCell)}
	// tail 规定尾部的初始取值。
	tail := 1
	// startDistance 保存由距离取得的起点或开始时间距离。
	startDistance := distance(start, target)

	// head 规定头部的初始取值。
	for head := 0; head < tail; head++ {
		// node 保存从队列取出的节点。
		node := b.queue[head]
		// cell 保存由整数取得的地图格。
		cell := int(node.cell)
		// cur 建立带初始值的点位，供本段继续处理。
		cur := devconfig.Point{X: cell % m.Width, Y: cell / m.Width}
		// d 接收遍历朝向集合时当前的元素。
		for direction, d := range directions {
			if exact && head == 0 {
				d = directions[(preferred+offsets[direction])%len(directions)]
			}
			// q 组装点位所需的数据。
			q := devconfig.Point{X: cur.X + d.X, Y: cur.Y + d.Y}
			if !m.Contains(q) {
				continue
			}
			// nextCell 保存根据纵坐标与宽度的计算结果与横坐标计算的下一状态地图格。
			nextCell := q.Y*m.Width + q.X
			if b.unavailable[nextCell/64]&(uint64(1)<<(nextCell%64)) != 0 || !m.Walkable(q) {
				continue
			}
			// 斜向移动的两个侧格只检查静态地形。
			if d.X != 0 && d.Y != 0 &&
				(!m.Walkable(devconfig.Point{X: q.X, Y: cur.Y}) || !m.Walkable(devconfig.Point{X: cur.X, Y: q.Y})) {
				continue
			}

			// firstCell 保存当前处理所用的第一项地图格。
			firstCell := node.firstCell
			if head == 0 {
				firstCell = uint16(nextCell)
			}
			// 确切目标以抵达为终点，避免错误邻格造成往返。追击仍可选择邻格。
			if q == target || !exact && startDistance > 1 && distance(q, target) == 1 {
				return devconfig.Point{X: int(firstCell) % m.Width, Y: int(firstCell) / m.Width}, true
			}
			b.markUnavailable(nextCell)
			b.queue[tail] = personalNavigationNode{cell: uint16(nextCell), firstCell: firstCell}
			tail++
		}
	}
	return devconfig.Point{}, false
}
