// 本文件衡量个人网格导航工作区的首步搜索分配与吞吐。
package game

import (
	"testing"
	"time"

	"minimir/internal/devconfig"
)

// BenchmarkPersonalNavigation 核验寻路应迈出第一步。
// b 控制基准循环并收集性能指标。
func BenchmarkPersonalNavigation(b *testing.B) {
	// tc 接收遍历临时结构列表时当前的元素。
	for _, tc := range []struct {
		// name 记录名称，使用文本表示。
		name string
		// target 记录目标，使用点位表示。
		target devconfig.Point
	}{
		{name: "Near", target: devconfig.Point{X: 2, Y: 2}},
		{name: "AcrossMap", target: devconfig.Point{X: 12, Y: 12}},
	} {
		// b 控制基准循环并收集性能指标。
		b.Run(tc.name, func(b *testing.B) {
			// w 接收创建个人导航测试房间执行器供后续步骤使用。
			w := newPersonalNavigationTestWorker(13, nil)
			// start 初始化用作起点或开始时间的点位。
			start := devconfig.Point{}
			// r 接收创建个人导航测试房间供后续步骤使用。
			r := newPersonalNavigationTestRoom(start)
			// x 规定横坐标的初始取值。
			for x := 2; x < 10; x++ {
				r.Monsters = append(r.Monsters, &entity{Kind: 3, HP: 10, Pos: devconfig.Point{X: x, Y: 6}})
			}
			r.MonsterCount = len(r.Monsters)
			// player 保存当前处理所用的玩家。
			player := r.Player
			// now 保存由Unix 时间取得的当前时间。
			now := time.Unix(100, 0)
			w.move(r, &r.Player, tc.target, now, 100)
			if r.Player.Pos == start {
				b.Fatal("寻路没有迈出第一步")
			}
			b.ReportAllocs()
			for b.Loop() {
				r.Player = player
				w.move(r, &r.Player, tc.target, now, 100)
			}
		})
	}
}
