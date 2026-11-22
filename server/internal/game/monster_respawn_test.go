package game

import (
	"testing"
	"time"

	"minimir/internal/devconfig"
	"minimir/internal/persistence"
)

func monsterRespawnRoom(m devconfig.MapTemplate, monsters ...*entity) (*worker, *room) {
	r := &room{Ready: true, Player: entity{Handle: 1, Kind: 1, HP: 20}, worldState: &worldState{MapID: m.ID, Monsters: monsters, MonsterCount: len(monsters)}}
	w := &worker{e: &Engine{Store: &persistence.Store{Cfg: devconfig.Config{Fixture: devconfig.Fixture{RuleVersion: 26, Map: m}}}}}
	return w, r
}

func TestRandomMonsterRespawnSamplesEachFreeGridCell(t *testing.T) {
	now := time.Unix(1800000000, 0)
	blocked := []devconfig.Point{{X: 1, Y: 1}, {X: 2, Y: 2}}
	candidates := []devconfig.Point{{X: 2, Y: 1}, {X: 0, Y: 2}, {X: 1, Y: 2}}
	for index, want := range candidates {
		m := devconfig.MapTemplate{ID: 1, Width: 3, Height: 3, Blocked: blocked, MonsterSpawns: []devconfig.Point{{}, {X: 1}, {X: 2}}}
		dead := &entity{Handle: 10, Kind: 3, MaxHP: 9, Life: 4, RespawnAt: now}
		visible := &entity{Handle: 11, Kind: 3, Pos: devconfig.Point{X: 2}, HP: 9}
		parked := &entity{Handle: 12, Kind: 3, Pos: devconfig.Point{Y: 1}, HP: 9}
		w, r := monsterRespawnRoom(m, dead, visible, parked)
		r.MonsterCount = 2
		r.Pet = &entity{Handle: 2, Kind: 2, Pos: devconfig.Point{X: 1}, HP: 4}
		world := r.currentWorld()
		old := referenceIn(world, dead)
		calls := 0
		w.roll = func(n int64) int64 {
			t.Helper()
			if n != int64(len(candidates)) {
				t.Fatalf("随机上界含阻挡或占格，或漏掉空格: %d", n)
			}
			calls++
			return int64(index)
		}
		w.worldDeadlines(r, world, now.Add(-time.Nanosecond))
		if calls != 0 || dead.HP != 0 || dead.Life != old.Life {
			t.Fatal("十秒原截止前就抽取或改变怪物生命")
		}
		w.worldDeadlines(r, world, now)
		if calls != 1 || dead.Pos != want || dead.HP != dead.MaxHP || dead.Life != old.Life+1 || w.resolveInWorld(r, world, old) != nil || !dead.RespawnAt.IsZero() || r.Isolated {
			t.Fatal("原截止内没有在指定空格一次复活，或旧生命仍可引用", dead)
		}
	}
}

func TestRandomMonsterRespawnKeepsOriginalDeadlineUntilGridFree(t *testing.T) {
	now := time.Unix(1800000000, 0)
	m := devconfig.MapTemplate{ID: 1, Width: 2, Height: 2, Blocked: []devconfig.Point{{X: 1, Y: 1}}, MonsterSpawns: []devconfig.Point{{}, {X: 1}, {Y: 1}}}
	first := &entity{Handle: 10, Kind: 3, MaxHP: 9, Life: 4, RespawnAt: now}
	second := &entity{Handle: 11, Kind: 3, MaxHP: 9, Life: 2, RespawnAt: now}
	alive := &entity{Handle: 12, Kind: 3, HP: 9, Pos: devconfig.Point{Y: 1}}
	w, r := monsterRespawnRoom(m, first, second, alive)
	c := &capturedConnection{}
	r.Conn, r.Pet = c, &entity{Handle: 2, Kind: 2, HP: 4, Pos: devconfig.Point{X: 1}}
	w.roll = func(int64) int64 {
		t.Fatal("没有格或唯一空格仍消耗随机抽样")
		return 0
	}
	world := r.currentWorld()
	w.worldDeadlines(r, world, now)
	if first.HP != 0 || first.Life != 4 || first.RespawnAt != now || second.HP != 0 || second.Life != 2 || second.RespawnAt != now || r.CellEntrySequence != 0 || r.Isolated || len(c.frames) != 0 {
		t.Fatal("无空位提前复活、改动截止或发送了假观察")
	}
	r.Pet.Pos = r.Player.Pos
	w.worldDeadlines(r, world, now.Add(100*time.Millisecond))
	if first.HP != 9 || first.Pos != (devconfig.Point{X: 1}) || second.HP != 0 || second.RespawnAt != now || r.CellEntrySequence != 1 {
		t.Fatal("首次空位未立即复活，或两个同截止怪争用同一格")
	}
	alive.Pos = r.Player.Pos
	w.worldDeadlines(r, world, now.Add(200*time.Millisecond))
	if second.HP != 9 || second.Pos != (devconfig.Point{Y: 1}) || second.Life != 3 || r.CellEntrySequence != 2 {
		t.Fatal("第二个格释放后未用原截止继续复活")
	}
}

func TestRandomMonsterRespawnUsesOwnWorldResidents(t *testing.T) {
	now := time.Unix(1800000000, 0)
	m := devconfig.MapTemplate{ID: 1, Width: 2, Height: 2, Blocked: []devconfig.Point{{X: 1, Y: 1}}, MonsterSpawns: []devconfig.Point{{}}}
	dead := &entity{Handle: 10, Kind: 3, MaxHP: 9, RespawnAt: now}
	w, r := monsterRespawnRoom(m, dead)
	personal := r.currentWorld()
	r.PersonalWorld, r.SavedPersonal = personal, personal
	personal.PlayerPosition = devconfig.Point{Y: 1}
	r.TownWorld = r.newWorld(2)
	r.worldState, r.Player.Pos = r.TownWorld, devconfig.Point{X: 1}
	r.Pet, r.PetWorld = &entity{Handle: 2, Kind: 2, HP: 4}, personal.Ref
	w.worldDeadlines(r, personal, now)
	if dead.Pos != r.Player.Pos || dead.HP != dead.MaxHP || dead.Pos == personal.PlayerPosition || dead.Pos == r.Pet.Pos {
		t.Fatal("延迟旧图复活占用返程或驻留宠物格，或错误采纳现图玩家格", dead)
	}
}

func TestLegacyMonsterRespawnRetainsConfiguredPoint(t *testing.T) {
	now := time.Unix(1800000000, 0)
	seed := devconfig.Point{X: 1, Y: 1}
	m := devconfig.MapTemplate{ID: 1, Width: 2, Height: 2, MonsterSpawns: []devconfig.Point{seed}}
	dead := &entity{Handle: 10, Kind: 3, MaxHP: 9, RespawnAt: now}
	w, r := monsterRespawnRoom(m, dead)
	w.e.Store.Cfg.Fixture.RuleVersion = 25
	w.roll = func(int64) int64 {
		t.Fatal("历史规则不应抽随机出生格")
		return 0
	}
	w.worldDeadlines(r, r.currentWorld(), now)
	if dead.Pos != seed || dead.HP != 9 {
		t.Fatal("历史fixture的出生规则发生变化")
	}
}
