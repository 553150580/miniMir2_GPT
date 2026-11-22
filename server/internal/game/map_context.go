// 本文件按房间当前地图统一查询地形、格数预算、坐标范围和可走性。
package game

import "minimir/internal/devconfig"

// 地形属于当前房间的目录身份。MapID为0只兼容旧内存测试，显式未知ID绝不回退。
// w 是调用该方法的房间执行器实例，r 指向当前执行器拥有并更新的目标房间。
func (w *worker) currentMap(r *room) (*devconfig.MapTemplate, bool) {
	if r == nil {
		return nil, false
	}
	return w.worldMap(r.currentWorld())
}

// mapCellBudget 返回地图地图格预算供调用方使用。
// w 是调用该方法的房间执行器实例，r 指向当前执行器拥有并更新的目标房间。
func (w *worker) mapCellBudget(r *room) int {
	// m 接收当前状态地图供后续步骤使用，ok 表明查询、校验或类型转换是否成功。
	m, ok := w.currentMap(r)
	if !ok {
		return 0
	}
	return m.CellBudget()
}

// inBounds 判断输入“bounds”是否满足要求。
// w 是调用该方法的房间执行器实例，r 指向当前执行器拥有并更新的目标房间，point 记录点位。
func (w *worker) inBounds(r *room, point devconfig.Point) bool {
	// m 接收当前状态地图供后续步骤使用，ok 表明查询、校验或类型转换是否成功。
	m, ok := w.currentMap(r)
	return ok && m.Contains(point)
}

// walkable 判断可走格是否满足要求。
// w 是调用该方法的房间执行器实例，r 指向当前执行器拥有并更新的目标房间，point 记录点位。
func (w *worker) walkable(r *room, point devconfig.Point) bool {
	// m 接收当前状态地图供后续步骤使用，ok 表明查询、校验或类型转换是否成功。
	m, ok := w.currentMap(r)
	return ok && m.Walkable(point)
}
