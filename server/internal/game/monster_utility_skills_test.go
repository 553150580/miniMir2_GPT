// 本文件验证诱惑与生命启示在延迟、刷新、旧出生和宠物回收上的边界。
package game

import (
	"testing"
	"time"

	"minimir/internal/devconfig"
	"minimir/internal/domain"
	p "minimir/internal/protocol"
)

// utilitySkillRoom 复用真实施法和地图夹具，并显式固定被测技能、原怪物和练习层级。
func utilitySkillRoom(t *testing.T, job, skillID uint32, behavior string) (*worker, *room) {
	t.Helper()
	w, r, _ := stageAWorker(t, job)
	f := &w.e.Store.Cfg.Fixture
	skill := devconfig.Skill{ID: skillID, JobID: job, Behavior: behavior, TargetKind: "enemy_entity", RangeCells: 5,
		MaxLevel: 3, DelayBaseMS: 500, ActionMS: 400, GlobalSpellMS: 500, MPBaseCost: 8, PowerBase: 5}
	if behavior == "revelation" {
		skill.BuffID = skillID
		if _, exists := f.Buff(skill.BuffID); !exists {
			f.Buffs = append(f.Buffs, devconfig.BuffDefinition{ID: skillID, Kind: behavior, DurationKind: "sc_source_power", TickMS: 1000,
				StackPolicy: "ignore_present_on_impact", RemoveOnPlayerDeath: true})
		}
	}
	for i, configured := range f.Skills {
		if configured.ID == skillID {
			f.Skills = append(f.Skills[:i], f.Skills[i+1:]...)
			break
		}
	}
	f.Skills = append(f.Skills, skill)
	r.Role.Skills = append(r.Role.Skills, domain.LearnedSkill{SkillID: skillID, Level: 0})
	r.Auto, r.Pet, r.Player.MP = false, nil, 150
	monster := r.Monsters[0]
	definition := *monster.Monster
	definition.CanTame, definition.Level, definition.RespawnMS = true, 1, 10000
	monster.Monster, monster.HP, monster.MaxHP, monster.Life = &definition, 7, 15, 5
	return w, r
}

// TestElectricShockCapturesSingleLivingMonster 验证原怪物到活宠身份转换、补怪及到期释放。
func TestElectricShockCapturesSingleLivingMonster(t *testing.T) {
	w, r := utilitySkillRoom(t, 1, 20, "electric_shock")
	now := time.Unix(1800000000, 0)
	world, monster := r.currentWorld(), r.Monsters[0]
	original := referenceIn(world, monster)
	r.AutoTarget = original
	// 技能阶通过，走捕获分支，放行双方等级闸门，命中捕获掷点。
	w.roll = func(n int64) int64 {
		if n > 4 {
			return n - 1
		}
		return 0
	}
	if code := w.cast(r, 20, monster.Handle, 1, now); code != 0 || len(world.Casts) != 1 || r.Player.MP != 142 {
		t.Fatalf("施法未在指定目标扣一次蓝，结果码 %d", code)
	}
	cast := world.Casts[0]
	w.advanceEvents(r, cast.Due.Add(-time.Millisecond))
	if r.Pet != nil || monster.HP != 7 {
		t.Fatal("到期前提前改变了怪物归属或生命")
	}
	w.advanceEvents(r, cast.Due)
	pet, replacement := r.Pet, world.Monsters[0]
	if pet == nil || pet.HP != 7 || pet.Def != monster.Def || pet.Handle == monster.Handle || pet.Pos != monster.Pos || r.PetWorld != world.Ref ||
		!replacement.DeferredUntilRespawn || replacement.HP != 0 || w.resolveInWorld(r, world, original) != nil || len(r.KillFacts) != 0 || r.AutoTarget.Handle != 0 {
		t.Fatal("原出生身份没有与宠物彻底分离，或错误产生了击杀或继续选中")
	}
	w.worldDeadlines(r, world, replacement.RespawnAt)
	if replacement.DeferredUntilRespawn || replacement.HP != replacement.MaxHP || replacement.Life != original.Life+1 {
		t.Fatal("捕获后普通补怪机制未恢复原刷怪入口")
	}
	if w.applyElectricShock(r, world, replacement, 0, now.Add(time.Minute)) || r.Pet != pet || replacement.HP <= 0 {
		t.Fatal("已有活宠时仍然捕获并占用了第二个活宠槽")
	}
	expires := pet.PetRuntime.TameUntil
	if expires != cast.Due.Add(time.Hour) || w.expirePetCorpses(r, expires.Add(-time.Millisecond)) || r.Pet != pet {
		t.Fatal("诱惑宠物未保留一小时或提前离场")
	}
	if !w.expirePetCorpses(r, expires) || r.Pet != nil || !pet.IsPetCorpse || len(world.PetCorpses) != 1 || r.Isolated {
		t.Fatal("到期未通过正常宠物归档释放活宠槽")
	}
}

// TestElectricShockRespectsMonsterAndMapConditions 验证震慑效果、打断及禁宠地图拒绝施法。
func TestElectricShockRespectsMonsterAndMapConditions(t *testing.T) {
	w, r := utilitySkillRoom(t, 1, 20, "electric_shock")
	now := time.Unix(1800000000, 0)
	world, monster := r.currentWorld(), r.Monsters[0]
	w.roll = func(n int64) int64 {
		if n == 2 {
			return 1
		}
		return 0
	}
	if !w.applyElectricShock(r, world, monster, 1, now) || monster.ShockUntilMS != now.Add(15*time.Second).UnixMilli() || monster.HP != 7 || r.Pet != nil {
		t.Fatal("震慑没有保留原怪物和正确的持续时间")
	}
	w.applyDamageInWorld(r, world, &r.Player, monster, 1, now, hitContext{PeriodicPoison: true})
	if monster.ShockUntilMS == 0 {
		t.Fatal("持续中毒错误打断了震慑")
	}
	w.applyDamageInWorld(r, world, &r.Player, monster, 1, now)
	if monster.ShockUntilMS != 0 {
		t.Fatal("直接伤害后震慑应当解除")
	}
	forbidden := true
	w.e.Store.Cfg.Fixture.Map.NoPets = &forbidden
	if _, code := w.prepareCast(r, 20, monster.Handle, 1, now, false); code != p.CodeSkillStateBlocked || r.Player.MP != 150 {
		t.Fatalf("禁宠地图未在扣蓝前拒绝，错误码 %d", code)
	}
}

// TestRevelationShowsOnlyOriginalLifeUntilDeadline 验证延迟概率、无伤害、定时显效、不刷新与出生隔离。
func TestRevelationShowsOnlyOriginalLifeUntilDeadline(t *testing.T) {
	w, r := utilitySkillRoom(t, 2, 28, "revelation")
	now := time.Unix(1800000000, 0)
	world, monster := r.currentWorld(), r.Monsters[0]
	if code := w.cast(r, 28, monster.Handle, 1, now); code != 0 || len(world.Casts) != 1 {
		t.Fatalf("心灵启示未安排延迟命中，结果码 %d", code)
	}
	cast := world.Casts[0]
	w.advanceEvents(r, cast.Due.Add(-time.Millisecond))
	if len(monster.Statuses) != 0 || cast.BuffDurationMS < 5000 {
		t.Fatal("启示提前显示，或遗漏道术与威力持续时间")
	}
	w.advanceEvents(r, cast.Due)
	status := monster.Statuses[28]
	if status == nil || status.TargetLife != monster.Life || monster.HP != 7 || len(statusesPB(monster, cast.Due)) != 1 {
		t.Fatal("原怪物未生成真实启示状态，或启示造成了伤害")
	}
	w.resolveCastInWorld(r, world, cast, cast.Due.Add(time.Second))
	if monster.Statuses[28] != status || status.LastProcess != cast.Due {
		t.Fatal("有效期内重施改写了状态")
	}
	deadline := cast.Due.Add(time.Duration(cast.BuffDurationMS) * time.Millisecond)
	tickEntityStatuses([]*entity{monster}, deadline)
	if len(monster.Statuses) != 0 {
		t.Fatal("启示达到实际截止后仍可见")
	}
	w.roll = func(n int64) int64 { return n - 1 }
	w.resolveCastInWorld(r, world, cast, deadline)
	if len(monster.Statuses) != 0 {
		t.Fatal("技能未通过本阶成功率仍显示生命")
	}
	monster.Life++
	w.roll = func(int64) int64 { return 0 }
	w.resolveCastInWorld(r, world, cast, deadline)
	if len(monster.Statuses) != 0 {
		t.Fatal("旧施法命中了怪物的新生命")
	}
}
