package game

import (
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	"minimir/internal/persistence"
	"testing"
	"time"
)

func TestGridEightDirectionsAndCornerBlocking(t *testing.T) {
	f := devconfig.Fixture{}
	f.Map.Width = 16
	f.Map.Height = 16
	e := &Engine{Store: &persistence.Store{Cfg: devconfig.Config{Fixture: f}}}
	w := &worker{e: e}
	for i, d := range directions {
		r := &room{Player: entity{Kind: 1, HP: 10, Pos: devconfig.Point{X: 8, Y: 8}}}
		to := devconfig.Point{X: 8 + d.X, Y: 8 + d.Y}
		if !w.neighbor(r, r.Player.Pos, to, &r.Player) {
			t.Errorf("方向%d被空地图错误阻挡", i)
		}
	}
	e.Store.Cfg.Fixture.Map.Blocked = []devconfig.Point{{X: 9, Y: 8}}
	r := &room{Role: domain.Role{}, Player: entity{Kind: 1, HP: 10, Pos: devconfig.Point{X: 8, Y: 8}}}
	if w.neighbor(r, r.Player.Pos, devconfig.Point{X: 9, Y: 9}, &r.Player) {
		t.Fatal("斜向移动穿过墙角")
	}
	if w.walkable(devconfig.Point{X: -1, Y: 8}) || w.walkable(devconfig.Point{X: 16, Y: 8}) {
		t.Fatal("可走范围越界")
	}
	target := &entity{Kind: 3, HP: 10, Pos: devconfig.Point{X: 9, Y: 9}}
	w.attack(r, &r.Player, target, time.Now())
	if target.HP != 10 || !r.Player.NextAttack.IsZero() {
		t.Fatal("斜角阻挡仍造成伤害或消耗攻击CD")
	}
}
