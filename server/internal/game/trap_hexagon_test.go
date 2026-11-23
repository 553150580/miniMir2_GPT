// 本文件核对困魔咒的隔离施法、原地格门禁、原出生约束和解除时钟。
package game

import (
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	p "minimir/internal/protocol"
	"testing"
	"time"
)

// trapHexagonTestRoom 在独占测试夹具中引入未开放的技能，不改变默认运行目录。
func trapHexagonTestRoom(t *testing.T) (*worker, *room) {
	t.Helper()
	w, r, _ := stageAWorker(t, 2)
	f := &w.e.Store.Cfg.Fixture
	if _, enabled := f.Skill(16); enabled {
		t.Fatal("困魔咒不应混入默认技能目录")
	}
	f.Skills = append(f.Skills, devconfig.Skill{ID: 16, JobID: 2, Behavior: devconfig.TrapHexagonBehavior,
		TargetKind: "enemy_grid", RangeCells: 9, AreaRadius: 1, MaterialItemID: 187, MaterialQuantity: 1,
		MaxLevel: 3, MPBaseCost: 7, MPLevelCost: 3, DelayBaseMS: devconfig.TrapHexagonDelayMS, ActionMS: 600, GlobalSpellMS: 1800})
	r.Role.Skills = append(r.Role.Skills, domain.LearnedSkill{SkillID: 16, Level: 2})
	r.Player.MP, r.Player.Pos = 100, devconfig.Point{X: 6, Y: 5}
	r.Auto, r.Pet, r.Ready = false, nil, true
	f.Map.Blocked = nil
	return w, r
}

// TestTrapHexagonUsesOriginalGridAndDeadline 验证受理和到期不同的扫描范围及精确暂停。
func TestTrapHexagonUsesOriginalGridAndDeadline(t *testing.T) {
	w, r := trapHexagonTestRoom(t)
	now := time.Unix(1800000000, 0)
	center := devconfig.Point{X: 7, Y: 6}
	world := r.currentWorld()
	// 目标只位于南排时不能受理，但在别的合法活怪触发后应计入到期扫描。
	newMonster := func(handle uint32, x, y int, level uint32) *entity {
		return &entity{Handle: handle, Kind: 3, Life: 1, Pos: devconfig.Point{X: x, Y: y}, HP: 10, MaxHP: 10,
			Monster: &devconfig.Monster{Fighter: devconfig.Fighter{Level: level, MoveMS: 350, AttackMS: 500}}}
	}
	south := newMonster(10, 7, 7, r.Role.Level+2)
	north := newMonster(11, 7, 5, r.Role.Level)
	tooStrong := newMonster(12, 8, 6, r.Role.Level+3)
	outside := newMonster(13, 9, 5, r.Role.Level)
	world.Monsters, world.MonsterCount = []*entity{south, tooStrong, outside}, 3
	if _, code := w.prepareCast(r, 16, 0, 2, now, true, pointPB(center)); code != p.CodeSkillTarget {
		t.Fatalf("只框到南排或超等级怪时仍可扣材料，错误 %d", code)
	}
	world.Monsters, world.MonsterCount = append(world.Monsters, north), 4
	if code := w.cast(r, 16, 0, 1, now, pointPB(center)); code != p.CodeSkillMaterialRequired || r.Player.MP != 100 {
		t.Fatalf("普通施法绕过了护符确认：%d", code)
	}
	plan, code := w.prepareCast(r, 16, 0, 2, now, true, pointPB(center))
	if code != 0 || w.acceptPreparedCast(r, plan, 2, now) != 0 || len(world.Casts) != 1 || r.Player.MP != 87 {
		t.Fatalf("已批准围困没有扣除原生阶魔法或安排唯一到期：%d", code)
	}
	cast := world.Casts[0]
	w.advanceEvents(r, now.Add(499*time.Millisecond))
	if len(world.TrapHexagon) != 0 {
		t.Fatal("围困在原500毫秒前提前生效")
	}
	w.advanceEvents(r, cast.Due)
	until := cast.Due.Add(20 * time.Second)
	if len(world.TrapHexagon) != 2 || world.TrapHexagon[referenceIn(world, north)] != until || world.TrapHexagon[referenceIn(world, south)] != until ||
		world.trapHexagonHeld(tooStrong, cast.Due) || world.trapHexagonHeld(outside, cast.Due) || north.HP != 10 || south.HP != 10 {
		t.Fatal("到期3×3、等级边界、原生阶时钟或无伤害行为不符")
	}
	// w 走真实普通怪行为入口，停在目标相邻格的怪也不能安排新攻击。
	before := north.Pos
	attackSeq := north.AttackSequence
	w.stepGridCombat(r, &w.e.Store.Cfg.Fixture.Map, cast.Due.Add(time.Millisecond), false)
	if north.Pos != before || north.AttackSequence != attackSeq {
		t.Fatal("普通怪仍在围困有效时产生主动行为")
	}
	w.applyDamageInWorld(r, world, &r.Player, north, 0, cast.Due)
	w.applyDamageInWorld(r, world, &r.Player, north, 1, cast.Due, hitContext{PeriodicPoison: true})
	if !world.trapHexagonHeld(north, cast.Due) {
		t.Fatal("未命中或毒的节拍解除了围困")
	}
	w.applyDamageInWorld(r, world, &r.Player, north, 1, cast.Due)
	if world.trapHexagonHeld(north, cast.Due) || !world.trapHexagonHeld(south, until.Add(-time.Millisecond)) || world.trapHexagonHeld(south, until) {
		t.Fatal("有效直接攻击或精确到期未解除围困")
	}
	south.Life++
	if world.trapHexagonHeld(south, cast.Due) {
		t.Fatal("怪物新出生代次继承了原围困")
	}
	w.worldDeadlines(r, world, until)
	if world.TrapHexagon != nil {
		t.Fatal("围困到期没有回收临时状态")
	}
}

// TestTrapHexagonRejectsStaleWorldAndBothBossKinds 防止迟到事件或特殊场景获得围困能力。
func TestTrapHexagonRejectsStaleWorldAndBothBossKinds(t *testing.T) {
	w, r := trapHexagonTestRoom(t)
	now := time.Unix(1800000000, 0)
	point := r.Monsters[0].Pos
	plan, code := w.prepareCast(r, 16, 0, 2, now, true, pointPB(point))
	if code != 0 || w.acceptPreparedCast(r, plan, 1, now) != 0 {
		t.Fatalf("隔离原场景未受理围困：%d", code)
	}
	original, cast := r.currentWorld(), r.Casts[0]
	for _, context := range []func(){
		func() { r.bossBattle = &personalBossBattle{} },
		func() { r.bossBattle = nil; r.worldBossObservation = &worldBossObservation{} },
		func() { r.worldBossObservation = nil; r.worldState = &worldState{} },
	} {
		context()
		w.resolveTrapHexagon(r, original, &r.Player, cast, cast.Due)
		if len(original.TrapHexagon) != 0 {
			t.Fatal("已付材料事件越过了场景归属或首领门禁")
		}
	}
}
