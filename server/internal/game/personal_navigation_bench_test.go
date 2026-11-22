package game

import (
	"testing"
	"time"

	"minimir/internal/devconfig"
)

func BenchmarkPersonalNavigation(b *testing.B) {
	for _, tc := range []struct {
		name   string
		target devconfig.Point
	}{
		{name: "Near", target: devconfig.Point{X: 2, Y: 2}},
		{name: "AcrossMap", target: devconfig.Point{X: 12, Y: 12}},
	} {
		b.Run(tc.name, func(b *testing.B) {
			w := newPersonalNavigationTestWorker(13, nil)
			start := devconfig.Point{}
			r := newPersonalNavigationTestRoom(start)
			for x := 2; x < 10; x++ {
				r.Monsters = append(r.Monsters, &entity{Kind: 3, HP: 10, Pos: devconfig.Point{X: x, Y: 6}})
			}
			r.MonsterCount = len(r.Monsters)
			player := r.Player
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
