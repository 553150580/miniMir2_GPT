// 验证复活落点避开独享图中的活怪、活宠和地形，尸体仍可通行。
package game

import (
	"testing"

	"minimir/internal/devconfig"
)

func TestOrdinaryRevivalUsesNearestFreeCell(t *testing.T) {
	w, r, _ := stageBWorker(t, 0)
	m := &w.e.Store.Cfg.Fixture.Map
	spawn := devconfig.Point{X: 6, Y: 6}
	blocked := devconfig.Point{X: 5, Y: 5}
	petCell := devconfig.Point{X: 6, Y: 5}
	corpseCell := devconfig.Point{X: 7, Y: 5}
	m.PlayerSpawn = spawn
	m.Blocked = []devconfig.Point{blocked}

	// 活怪占用原出生格，活宠和阻挡占据最先检查的两个相邻格。
	world := r.currentWorld()
	r.Monsters = []*entity{{Pos: spawn, HP: 10}, {Pos: corpseCell, HP: 0}}
	r.MonsterCount = len(r.Monsters)
	r.Pet = &entity{Kind: 2, Pos: petCell, HP: 10}
	r.PetWorld = world.Ref
	r.Player.Pos, r.Player.HP, r.DeathID = devconfig.Point{X: 8, Y: 8}, 0, "复活验证"
	beforeLife := r.Player.Life

	w.revivePlayer(r)

	if r.Isolated || r.Player.Pos != corpseCell || r.Player.HP != r.Player.MaxHP || r.Player.MP != r.Player.MaxMP ||
		r.Player.Life != beforeLife+1 || r.DeathID != "" {
		t.Fatalf("复活未在最近合规空格恢复原角色：%+v", r.Player)
	}
}

func TestOrdinaryRevivalKeepsFreeConfiguredSpawn(t *testing.T) {
	w, r, _ := stageBWorker(t, 0)
	spawn := w.e.Store.Cfg.Fixture.Map.PlayerSpawn
	r.Monsters = []*entity{{Pos: spawn, HP: 0}}
	r.MonsterCount = len(r.Monsters)
	r.Player.HP = 0

	w.revivePlayer(r)

	if r.Isolated || r.Player.Pos != spawn || r.Player.HP <= 0 {
		t.Fatalf("原出生格只有尸体，应保留配置落点：%+v", r.Player)
	}
}
