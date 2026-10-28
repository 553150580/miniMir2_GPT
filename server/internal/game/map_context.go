package game

import "minimir/internal/devconfig"

// 地形属于当前房间的目录身份。MapID为0只兼容旧内存测试，显式未知ID绝不回退。
func (w *worker) currentMap(r *room) (*devconfig.MapTemplate, bool) {
	if r == nil {
		return nil, false
	}
	return w.worldMap(r.currentWorld())
}

func (w *worker) mapCellBudget(r *room) int {
	m, ok := w.currentMap(r)
	if !ok {
		return 0
	}
	return m.CellBudget()
}

func (w *worker) inBounds(r *room, point devconfig.Point) bool {
	m, ok := w.currentMap(r)
	return ok && m.Contains(point)
}

func (w *worker) walkable(r *room, point devconfig.Point) bool {
	m, ok := w.currentMap(r)
	return ok && m.Walkable(point)
}
