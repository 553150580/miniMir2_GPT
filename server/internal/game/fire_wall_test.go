// 本文件验证火墙的正交建场、入格伤害、时序、归属及离图清理。
package game

import (
	"testing"
	"time"

	"minimir/internal/devconfig"
	"minimir/internal/domain"
)

// fireWallTestRoom 为法师隔离选择火墙，使测试不依赖主试玩发布选择。
func fireWallTestRoom(t *testing.T) (*worker, *room, *entity) {
	t.Helper()
	// w 与 r 运行真实来源时间线，target 是原世界中的一只受控普通怪物。
	w, r, _ := stageAWorker(t, 1)
	// f 保存该测试独占的开发数据，skill 使用静态来源参数。
	f := &w.e.Store.Cfg.Fixture
	skill := devconfig.Skill{ID: 22, JobID: 1, Behavior: "fire_wall", TargetKind: "enemy_grid", RangeCells: 9, AreaRadius: 1,
		MPBaseCost: 30, MPLevelCost: 5, MaxLevel: 3, DelayBaseMS: 500, ActionMS: 600, GlobalSpellMS: 1800,
		PowerBase: 3, MPowerBase: 3, MultiplierBasePPM: 1000000}
	if _, exists := f.Skill(skill.ID); !exists {
		f.Skills = append(f.Skills, skill)
	}
	r.Role.Skills = append(r.Role.Skills, domain.LearnedSkill{SkillID: skill.ID})
	r.Player.Pos = devconfig.Point{X: 4, Y: 5}
	r.Player.MP = 100
	r.MonsterCount = 1
	target := r.Monsters[0]
	// monster 清除随机减免，使每次实际伤害能够直接与存下来的源威力核对。
	monster := *target.Monster
	monster.MagicDefenceMin, monster.MagicDefenceMax, monster.MagicResist = 0, 0, 0
	target.Monster = &monster
	w.roll = func(int64) int64 { return 0 }
	return w, r, target
}

// TestFireWallCrossTilesDamageAndWalkEntry 验证500毫秒前不生效、格形、入格、2秒周期和延迟后不追算。
func TestFireWallCrossTilesDamageAndWalkEntry(t *testing.T) {
	// now 是施法受理时刻，center 是指定中心，north 代表一个被地形剔除的邻格。
	w, r, target := fireWallTestRoom(t)
	now := time.Unix(1800000000, 0)
	center, north := devconfig.Point{X: 6, Y: 5}, devconfig.Point{X: 6, Y: 4}
	// source 保存可精确设置阻挡格的测试地图，其他地面均有效。
	source := &w.e.Store.Cfg.Fixture.Map
	source.CollisionBits = nil
	source.Blocked = []devconfig.Point{north}
	target.Pos = center
	if code := w.cast(r, 22, 0, 1, now, pointPB(center)); code != 0 || r.Player.MP != 70 || len(r.Casts) != 1 {
		t.Fatalf("火墙受理没有冻结单次魔法消耗和放置任务，结果 %d", code)
	}
	// world 是火墙唯一所属的原地图，cast 保留待到期的原伤害。
	world, cast := r.currentWorld(), r.Casts[0]
	w.advanceEvents(r, cast.Due.Add(-time.Millisecond))
	if len(world.FireWalls) != 0 || target.HP != 1000 {
		t.Fatal("火墙提前建场或扣血")
	}
	w.advanceEvents(r, cast.Due)
	if len(world.FireWalls) != 1 || len(r.Casts) != 1 || target.HP != 1000-cast.Amount {
		t.Fatal("首次地格伤害或单后继任务不符")
	}
	// field 保存实际地格，east 为正交邻格，diagonal 为不能命中的斜格。
	field := world.FireWalls[0]
	east, diagonal := devconfig.Point{X: 7, Y: 5}, devconfig.Point{X: 7, Y: 6}
	if field.Count != 4 || field.covers(north) || field.covers(diagonal) || !field.covers(east) || field.Expires.Sub(cast.Due) != fireWallLifetime(cast.Amount) {
		t.Fatalf("火墙正交、地形或持续时间不符，场 %+v", field)
	}
	target.Pos = diagonal
	// enter 是怪物真实行走进入东格的时刻，它不能推迟下一周期。
	enter := cast.Due.Add(time.Second)
	if !w.commitWalk(r, target, east, 0, enter, 500) || target.HP != 1000-2*cast.Amount {
		t.Fatal("移入火墙没有立即承受一次原威力伤害")
	}
	// next 是未因入格重置的周期边界。
	next := cast.Due.Add(2 * time.Second)
	w.advanceEvents(r, next.Add(-time.Millisecond))
	if target.HP != 1000-2*cast.Amount {
		t.Fatal("火墙周期提前命中")
	}
	w.advanceEvents(r, next)
	if target.HP != 1000-3*cast.Amount {
		t.Fatal("入格后原两秒周期应继续命中")
	}
	w.advanceEvents(r, next.Add(5*time.Second))
	if target.HP != 1000-4*cast.Amount || !r.Casts[0].Due.Equal(next.Add(7*time.Second)) {
		t.Fatal("迟到的火墙周期应只发生一次并以实际处理时间推进")
	}
	w.advanceEvents(r, field.Expires.Add(time.Nanosecond))
	if target.HP != 1000-4*cast.Amount || len(world.FireWalls) != 0 || r.eventCount() != 0 {
		t.Fatal("火墙逾期仍有伤害或遗留任务")
	}
}

// TestFireWallOverlapAndWorldCleanup 验证重复格不覆盖旧场，真实离图会立即清理活场和计时任务。
func TestFireWallOverlapAndWorldCleanup(t *testing.T) {
	// now 是首场落地时刻，world 是两个火墙的原世界。
	w, r, target := fireWallTestRoom(t)
	now := time.Unix(1800000000, 0)
	world := r.currentWorld()
	// center 是首场中心，shared 在首场东侧且为次场中心。
	center, shared := devconfig.Point{X: 6, Y: 5}, devconfig.Point{X: 7, Y: 5}
	target.Pos = shared
	// first 模拟首个已受理并达到放置时间的原事件，second 尝试对旧场重复格覆盖。
	skill, _ := w.e.Store.Cfg.Fixture.Skill(22)
	first := pendingCast{Skill: skill, Scene: r.Scene, Caster: reference(r, &r.Player), ID: 1, Amount: 8, TargetPoint: center, Due: now}
	second := first
	second.ID, second.Amount, second.TargetPoint = 2, 30, shared
	w.resolveCast(r, first, now)
	w.resolveCast(r, second, now)
	if len(world.FireWalls) != 2 || !world.FireWalls[0].covers(shared) || world.FireWalls[1].covers(shared) || target.HP != 992 {
		t.Fatal("同格重复建立、覆盖原场或重复伤害")
	}
	// other 使用同一地图模板但属于不同执行世界，避免将同图瞬移误判为离图。
	other := r.allocateWorld(world.MapID, 9)
	r.PersonalWorld, r.TownWorld = world, other
	// petPoint 在存在默认宠物时为本次真实迁移提供安全落点。
	petPoint := devconfig.Point{X: 3, Y: 5}
	// stale 保留曾被摘入调度批次的旧场副本，用于模拟快速回原图后仍存在的批次引用。
	stale := world.Casts[0]
	if !w.activateWorld(r, other, devconfig.Point{X: 4, Y: 5}, &petPoint) || len(world.FireWalls) != 0 || len(world.Casts) != 0 {
		t.Fatal("真正跨世界没有移除两个旧火墙及其后继")
	}
	if !w.activateWorld(r, world, devconfig.Point{X: 4, Y: 5}, &petPoint) {
		t.Fatal("测试无法回到原执行世界")
	}
	w.resolveCast(r, stale, now.Add(2*time.Second))
	if len(world.FireWalls) != 0 || target.HP != 992 {
		t.Fatal("离图前已摘取的旧火墙批次在返回时复活")
	}
}

// TestFireWallTickUsesOriginalDueAttribution 验证已到期事件延迟处理时保留原会话和绑定条件。
func TestFireWallTickUsesOriginalDueAttribution(t *testing.T) {
	// now 是首场受理后的实际放置点，center 保持空闲，避免首次处理提前击杀。
	w, r, target := fireWallTestRoom(t)
	now := time.Unix(1800000000, 0)
	center := devconfig.Point{X: 6, Y: 5}
	target.Pos = devconfig.Point{X: 8, Y: 5}
	r.Conn, r.HostSession, r.Role.PrivilegeUntil = nil, "原离线会话", now.Add(30*time.Second)
	// skill 和 first 组成第一个已受理事件，只服务定向场景。
	skill, _ := w.e.Store.Cfg.Fixture.Skill(22)
	first := pendingCast{Skill: skill, Scene: r.Scene, Caster: reference(r, &r.Player), ID: 1, Amount: 8, TargetPoint: center, Due: now}
	w.resolveCast(r, first, now)
	// due 是已发生但随后晚处理的命中归属点。两项更改都发生在这个归属点之后。
	due := r.Casts[0].Due
	transitionEventHosting(r, due.Add(time.Millisecond), "另一会话")
	transitionEventPrivilege(r, due.Add(time.Millisecond), time.Time{})
	r.HostSession, r.Role.PrivilegeUntil = "另一会话", time.Time{}
	target.Pos, target.HP = center, 4
	w.advanceEvents(r, due.Add(time.Second))
	if len(r.KillFacts) != 1 || !r.KillFacts[0].At.Equal(due) || r.KillFacts[0].HostSession != "原离线会话" || r.KillFacts[0].Bound {
		t.Fatalf("火墙击杀未沿用原到期时刻的会话和特权：%+v", r.KillFacts)
	}
}

// TestFireWallPushOnlyTouchesFinalCell 验证受推跨过火墙和最终落入火墙走不同的来源分支。
func TestFireWallPushOnlyTouchesFinalCell(t *testing.T) {
	// now 是独立结算时钟，center 不占用移动起点和最终非火墙格。
	w, r, target := fireWallTestRoom(t)
	now := time.Unix(1800000000, 0)
	center := devconfig.Point{X: 6, Y: 8}
	// 本用例只验证目标自身的入格时钟，移除默认宠物造成的路线占格。
	r.Pet = nil
	target.Pos, target.Monster.CanPush = devconfig.Point{X: 4, Y: 8}, true
	// skill 是已配置的火墙行为，cast 以定值建立一处待测试的火墙。
	skill, _ := w.e.Store.Cfg.Fixture.Skill(22)
	cast := pendingCast{Skill: skill, Scene: r.Scene, Caster: reference(r, &r.Player), ID: 1, Amount: 8, TargetPoint: center, Due: now}
	w.resolveCast(r, cast, now)
	// out 记录推过三格火墙并抵达无火的格，应该保留全部生命。
	out := w.sourcePushed(r, reference(r, target), 2, 4, now.Add(time.Millisecond), displacementSource{})
	if out.Steps != 4 || target.HP != 1000 {
		t.Fatalf("沿途火墙不应扣血：位移 %+v，生命 %d", out, target.HP)
	}
	out = w.sourcePushed(r, reference(r, target), 6, 1, now.Add(time.Second), displacementSource{})
	if out.Steps != 1 || target.HP != 992 {
		t.Fatalf("最终落入火墙应立即触发一次：位移 %+v，生命 %d", out, target.HP)
	}
}
