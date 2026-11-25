// 本文件执行诱惑之光的震慑和捕获，以及心灵启示的目标生命观察。
package game

import (
	"time"

	"minimir/internal/devconfig"
)

// revelationDuration 在消耗魔法前冻结本次道术与技能威力掷点，返回持续秒数。
func (w *worker) revelationDuration(skill devconfig.Skill, rank uint32, stats devconfig.Fighter) int64 {
	skillPower := skill.SourcePower(rank, skill.MPowerBase+w.draw(skill.MPowerBonus), skill.PowerBase+w.draw(skill.PowerBonus))
	return w.interval(stats.TaoMin, stats.TaoMax) + skillPower
}

// electricShockAllowed 确认地图允许捕获宠物，且目标世界是施法者当前的普通个人地图。
func (w *worker) electricShockAllowed(r *room, world *worldState) bool {
	if !w.sourcePetLifecycle() || r.bossBattle != nil || r.worldBossObservation != nil || r.worldBossCombat != nil || world == nil || world != r.currentWorld() {
		return false
	}
	m, known := w.worldMap(world)
	return known && m.SceneKindValue() == devconfig.SceneKindPersonal && !mapForbidsPets(m)
}

// resolveMonsterUtility 只对同场的原怪物生命生效，结果不造成伤害或产生击杀。
func (w *worker) resolveMonsterUtility(r *room, world *worldState, caster *entity, cast pendingCast, now time.Time) {
	target := w.skillTargetInWorld(r, world, cast.Skill, cast.Target)
	point, outcome := cast.TargetPoint, uint32(3)
	if caster == &r.Player && caster.HP > 0 && world == r.currentWorld() && target != nil && target.Kind == 3 && target.Life == cast.TargetLife {
		point, outcome = target.Pos, 2
		if cast.Skill.Behavior == "electric_shock" && w.applyElectricShock(r, world, target, cast.SkillLevel, now) ||
			cast.Skill.Behavior == "revelation" && w.draw(4) <= int64(cast.SkillLevel) && w.applyBuff(r, cast, target, now) {
			outcome = 1
		}
	}
	w.impactInWorld(r, world, cast.Scene, cast.Skill.ID, cast.Caster.Handle, cast.Target, cast.TargetLife, point, 0, outcome, false, cast.ID, 0)
}

// applyElectricShock 按技能阶决定是否先震慑，符合等级与怪物种类的目标才进入捕获分支。
func (w *worker) applyElectricShock(r *room, world *worldState, target *entity, rank uint32, now time.Time) bool {
	if !w.electricShockAllowed(r, world) || target.Monster == nil || w.draw(4-int64(rank)) != 0 {
		return false
	}
	if w.draw(2) == 1 {
		target.ShockUntilMS = now.Add(time.Duration(10+5*rank) * time.Second).UnixMilli()
		return true
	}
	level, targetLevel := int64(r.Role.Level), int64(target.Monster.Level)
	if !target.Monster.CanTame || targetLevel > level+2 || w.draw(level+20+int64(rank)*5) <= targetLevel+10 || !w.canBeginPetBirth(r, now, actorRef{}) {
		return false
	}
	chanceDenominator := max(2, 2*(target.MaxHP/100))
	if w.draw(chanceDenominator) != 0 {
		return false
	}
	return w.captureMonster(r, world, target, now)
}

// captureMonster 以新宠物身份接手怪物的当前位置和生命，原出生位独立进入普通补怪计时。
func (w *worker) captureMonster(r *room, world *worldState, target *entity, now time.Time) bool {
	// 项目已开放的职业宠物规则提供统一的新生动作时钟及尸体回收期限。
	profiles := w.e.Store.Cfg.Fixture.PetRuntime.Definitions
	if len(profiles) == 0 {
		return false
	}
	for index, monster := range world.Monsters[:world.MonsterCount] {
		if monster != target {
			continue
		}
		base := target.Monster.Fighter
		pet := &entity{Kind: 2, Def: target.Def, Pos: target.Pos, Direction: target.Direction, HP: target.HP, MaxHP: target.MaxHP, MP: target.MP, MaxMP: target.MaxMP,
			ActionReadyAt: now.Add(time.Duration(profiles[0].Lifecycle.SpawnReadyMS) * time.Millisecond),
			PetRuntime: &petRuntimeState{Definition: devconfig.PetRuntimeDefinition{
				SourceSkillID: 20, MonsterTemplateID: target.Monster.ID, OwnerJobID: r.Role.Job, Base: base, Undead: target.Monster.Undead, Lifecycle: profiles[0].Lifecycle,
				Training: devconfig.PetTrainingRules{OwnerRangeCells: profiles[0].SourceDataRangeCells()},
			}, Fighter: base, TameUntil: now.Add(time.Hour)},
		}
		if !w.installPet(r, world, pet) {
			return false
		}
		world.clearTrapHexagon(target)
		if r.AutoTarget == referenceIn(world, target) {
			r.AutoTarget = actorRef{}
		}
		world.Monsters[index] = &entity{Handle: target.Handle, Kind: 3, Def: target.Def, Pos: target.Pos, MaxHP: target.MaxHP, MaxMP: target.MaxMP,
			Life: target.Life, Monster: target.Monster, DeferredUntilRespawn: true, RespawnAt: now.Add(normalMonsterRespawnDelay(target.Monster))}
		return true
	}
	return false
}

// expireTamedPet 在有效期到达时按既有职业宠物死亡路径交还现场，释放所属角色的单宠槽。
func (w *worker) expireTamedPet(r *room, now time.Time) bool {
	pet := r.Pet
	if pet == nil || pet.HP <= 0 || pet.PetRuntime == nil || pet.PetRuntime.TameUntil.IsZero() || now.Before(pet.PetRuntime.TameUntil) {
		return false
	}
	w.killOwnedPet(r, now)
	return r.Pet != pet
}
