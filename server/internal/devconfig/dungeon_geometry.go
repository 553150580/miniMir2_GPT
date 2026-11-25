// 本文件校验讨伐链相邻小区的静态连通性，不包含运行时格战和房间状态。
package devconfig

// dungeonAreaDirections 用四方向边判断小区连通，防止斜角接触充当通道。
var dungeonAreaDirections = [...]Point{{X: 1}, {X: -1}, {Y: 1}, {Y: -1}}

// overlapDungeonAreas 判定两个含端点的小区是否复用同一格。
func overlapDungeonAreas(a, b DungeonArea) bool {
	return a.Min.X <= b.Max.X && b.Min.X <= a.Max.X && a.Min.Y <= b.Max.Y && b.Min.Y <= a.Max.Y
}

// dungeonAreaReachable 在本段范围内求出与第一个玩家落点连通的走格。
func dungeonAreaReachable(m *MapTemplate, area DungeonArea, spawn Point) map[Point]struct{} {
	if !area.Contains(spawn) || !m.Walkable(spawn) {
		return nil
	}
	queue := []Point{spawn}                    // 玩家可到达的待遍历坐标。
	reachable := map[Point]struct{}{spawn: {}} // 已遍历的小区格。
	for index := 0; index < len(queue); index++ {
		point := queue[index]
		for _, direction := range dungeonAreaDirections {
			next := Point{X: point.X + direction.X, Y: point.Y + direction.Y}
			if _, exists := reachable[next]; exists || !area.Contains(next) || !m.Walkable(next) {
				continue
			}
			reachable[next] = struct{}{}
			queue = append(queue, next)
		}
	}
	return reachable
}

// adjacentDungeonAreas 要求两段都有可从各自落点走到且共边的两格。
func adjacentDungeonAreas(previous, current map[Point]struct{}) bool {
	for point := range current {
		for _, direction := range dungeonAreaDirections {
			if _, exists := previous[Point{X: point.X + direction.X, Y: point.Y + direction.Y}]; exists {
				return true
			}
		}
	}
	return false
}
