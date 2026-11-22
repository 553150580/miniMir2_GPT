// 本文件回归怪物复活时遍历自身地图可用格、跳过占位和全满时延后，以及旧规则出生点。
package game

import (
	"testing"
	"time"

	"minimir/internal/devconfig"
	"minimir/internal/persistence"
)

// monsterRespawnRoom 处理怪物复活房间。
// m 记录地图模板，monsters 记录怪物集合，使用可变数量的实体表示。
func monsterRespawnRoom(m devconfig.MapTemplate, monsters ...*entity) (*worker, *room) {
	// r 保存当前处理所用的房间。
	r := &room{Ready: true, Player: entity{Handle: 1, Kind: 1, HP: 20}, worldState: &worldState{MapID: m.ID, Monsters: monsters, MonsterCount: len(monsters)}}
	// w 保存当前处理所用的房间执行器。
	w := &worker{e: &Engine{Store: &persistence.Store{Cfg: devconfig.Config{Fixture: devconfig.Fixture{RuleVersion: 26, Map: m}}}}}
	return w, r
}

// TestRandomMonsterRespawnSamplesEachFreeGridCell 核验原截止内应在指定空格一次复活，或旧生命仍可引用，用诊断「随机上界含阻挡或占格，或漏掉空格」定位对应断言。
// t 提供断言失败报告、子用例和清理注册。
func TestRandomMonsterRespawnSamplesEachFreeGridCell(t *testing.T) {
	// now 保存由Unix 时间取得的当前时间。
	now := time.Unix(1800000000, 0)
	// blocked 初始化用作阻挡的点位列表。
	blocked := []devconfig.Point{{X: 1, Y: 1}, {X: 2, Y: 2}}
	// candidates 建立带初始值的点位列表，供本段继续处理。
	candidates := []devconfig.Point{{X: 2, Y: 1}, {X: 0, Y: 2}, {X: 1, Y: 2}}
	// index 定位候选集合中正在处理的成员，want 接收遍历候选集合时当前的元素。
	for index, want := range candidates {
		// m 组装地图模板所需的数据。
		m := devconfig.MapTemplate{ID: 1, Width: 3, Height: 3, Blocked: blocked, MonsterSpawns: []devconfig.Point{{}, {X: 1}, {X: 2}}}
		// dead 保存当前处理所用的死亡状态。
		dead := &entity{Handle: 10, Kind: 3, MaxHP: 9, Life: 4, RespawnAt: now}
		// visible 保存当前处理所用的可见。
		visible := &entity{Handle: 11, Kind: 3, Pos: devconfig.Point{X: 2}, HP: 9}
		// parked 保存当前处理所用的“parked”。
		parked := &entity{Handle: 12, Kind: 3, Pos: devconfig.Point{Y: 1}, HP: 9}
		// w 接收怪物复活房间供后续步骤使用，r 接收怪物复活房间供后续步骤使用。
		w, r := monsterRespawnRoom(m, dead, visible, parked)
		r.MonsterCount = 2
		r.Pet = &entity{Handle: 2, Kind: 2, Pos: devconfig.Point{X: 1}, HP: 4}
		// world 保存由当前状态场景世界取得的场景世界。
		world := r.currentWorld()
		// old 保存由引用输入取得的原状态。
		old := referenceIn(world, dead)
		// calls 规定调用集合的初始取值。
		calls := 0
		// n 记录数值。
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

// TestRandomMonsterRespawnKeepsOriginalDeadlineUntilGridFree 核对「首次空位未立即复活，或两个同截止怪争用同一格」与「无空位提前复活、改动截止或发送了假观察」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestRandomMonsterRespawnKeepsOriginalDeadlineUntilGridFree(t *testing.T) {
	// now 保存由Unix 时间取得的当前时间。
	now := time.Unix(1800000000, 0)
	// m 组装地图模板所需的数据。
	m := devconfig.MapTemplate{ID: 1, Width: 2, Height: 2, Blocked: []devconfig.Point{{X: 1, Y: 1}}, MonsterSpawns: []devconfig.Point{{}, {X: 1}, {Y: 1}}}
	// first 保存当前处理所用的第一项。
	first := &entity{Handle: 10, Kind: 3, MaxHP: 9, Life: 4, RespawnAt: now}
	// second 保存当前处理所用的秒。
	second := &entity{Handle: 11, Kind: 3, MaxHP: 9, Life: 2, RespawnAt: now}
	// alive 保存当前处理所用的存活状态。
	alive := &entity{Handle: 12, Kind: 3, HP: 9, Pos: devconfig.Point{Y: 1}}
	// w 接收怪物复活房间供后续步骤使用，r 接收怪物复活房间供后续步骤使用。
	w, r := monsterRespawnRoom(m, first, second, alive)
	// c 保存当前处理所用的已捕获连接。
	c := &capturedConnection{}
	r.Conn, r.Pet = c, &entity{Handle: 2, Kind: 2, HP: 4, Pos: devconfig.Point{X: 1}}
	w.roll = func(int64) int64 {
		t.Fatal("没有格或唯一空格仍消耗随机抽样")
		return 0
	}
	// world 保存由当前状态场景世界取得的场景世界。
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

// TestRandomMonsterRespawnUsesOwnWorldResidents 核对「延迟旧图复活占用返程或驻留宠物格，或错误采纳现图玩家格」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestRandomMonsterRespawnUsesOwnWorldResidents(t *testing.T) {
	// now 保存由Unix 时间取得的当前时间。
	now := time.Unix(1800000000, 0)
	// m 组装地图模板所需的数据。
	m := devconfig.MapTemplate{ID: 1, Width: 2, Height: 2, Blocked: []devconfig.Point{{X: 1, Y: 1}}, MonsterSpawns: []devconfig.Point{{}}}
	// dead 保存当前处理所用的死亡状态。
	dead := &entity{Handle: 10, Kind: 3, MaxHP: 9, RespawnAt: now}
	// w 接收怪物复活房间供后续步骤使用，r 接收怪物复活房间供后续步骤使用。
	w, r := monsterRespawnRoom(m, dead)
	// personal 保存由当前状态场景世界取得的个人。
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

// TestLegacyMonsterRespawnRetainsConfiguredPoint 核验历史规则不应抽随机出生格，用诊断「历史fixture的出生规则发生变化」定位对应断言。
// t 提供断言失败报告、子用例和清理注册。
func TestLegacyMonsterRespawnRetainsConfiguredPoint(t *testing.T) {
	// now 保存由Unix 时间取得的当前时间。
	now := time.Unix(1800000000, 0)
	// seed 初始化用作预置数据的点位。
	seed := devconfig.Point{X: 1, Y: 1}
	// m 组装地图模板所需的数据。
	m := devconfig.MapTemplate{ID: 1, Width: 2, Height: 2, MonsterSpawns: []devconfig.Point{seed}}
	// dead 保存当前处理所用的死亡状态。
	dead := &entity{Handle: 10, Kind: 3, MaxHP: 9, RespawnAt: now}
	// w 接收怪物复活房间供后续步骤使用，r 接收怪物复活房间供后续步骤使用。
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
