package game

import (
	"math"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	p "minimir/internal/protocol"
	"time"
)

// 新生属于原地图动作；Recall属于原宠物动作，不能借旧World身份抹掉迁移后的同一birth。
type summonPetPlan struct {
	Recall     bool
	Pet        actorRef
	Token      petBirthToken
	Definition devconfig.PetRuntimeDefinition
	Front      devconfig.Point
}

// Crystal BoneFamiliar构造使用DownLeft，属于固定八向枚举，不是玩法调参。
const skeletonSpawnDirection uint32 = 5

func (w *worker) sourcePetLifecycle() bool {
	return w.petRuntimeEnabled() && w.e.Store.Cfg.Fixture.RuleVersion >= 17
}

func mapForbidsPets(m *devconfig.MapTemplate) bool { return m != nil && m.NoPets != nil && *m.NoPets }

// 仅在初始化或真实跨图时调用；Fresh fallback和同图Teleport不是重新入图。
func (w *worker) applyPetMapEntryPolicy(r *room, m *devconfig.MapTemplate) {
	if r.Pet == nil || r.Pet.PetRuntime == nil {
		return
	}
	r.Pet.PetRuntime.FollowSuppressed = mapForbidsPets(m)
	if r.Pet.PetRuntime.FollowSuppressed {
		w.cancelTownRequest(r.TownNavigation[1])
	}
}

func (w *worker) prepareSummonPet(r *room, skill devconfig.Skill, direction uint32, now time.Time) (*summonPetPlan, uint32) {
	m, known := w.currentMap(r)
	if !w.sourcePetLifecycle() || !known || m.NoPets == nil || *m.NoPets || skill.Summon == nil {
		return nil, p.CodeSkillStateBlocked
	}
	d, ok := w.e.Store.Cfg.Fixture.PetRuntime.DefinitionByID(skill.Summon.DefinitionID)
	if !ok || d.RecallName == "" || d.OwnerJobID != r.Role.Job || d.SourceSkillID != skill.ID {
		return nil, p.CodeSkillStateBlocked
	}
	plan := &summonPetPlan{Definition: d, Front: stepPoint(r.Player.Pos, direction, 1)}
	if pet := r.Pet; pet != nil {
		if pet.HP <= 0 || pet.IsPetCorpse || r.petResidentWorld() == nil || pet.PetRuntime == nil || pet.PetRuntime.Definition.RecallName != d.RecallName {
			return nil, p.CodeSkillStateBlocked
		}
		plan.Recall, plan.Pet = true, r.petReference()
	} else if !w.canBeginPetBirth(r, now) {
		return nil, p.CodeSkillStateBlocked
	}
	return plan, 0
}

func (w *worker) reserveSummonPet(r *room, plan *preparedCast, operationID string, now time.Time) bool {
	if plan == nil || plan.Summon == nil || plan.Summon.Recall {
		return true
	}
	if r.ScheduleID == math.MaxUint64 || r.CastID == math.MaxUint64 {
		return false
	}
	plan.Summon.Token = petBirthToken{ID: operationID, ExecutionEpoch: r.ExecutionEpoch, PlayerLife: plan.Caster.Life}
	return w.reservePetBirth(r, plan.Summon.Token, now)
}

func (w *worker) cancelSummonPet(r *room, plan *preparedCast) {
	if plan != nil && plan.Summon != nil && !plan.Summon.Recall {
		w.cancelPetBirth(r, plan.Summon.Token)
	}
}

func (w *worker) summonPlanCurrent(r *room, plan *preparedCast) bool {
	if plan.Summon == nil {
		return true
	}
	if plan.Summon.Recall {
		return r.Pet != nil && r.Pet.HP > 0 && !r.Pet.IsPetCorpse && r.petResidentWorld() != nil &&
			r.Pet.Handle == plan.Summon.Pet.Handle && r.Pet.Life == plan.Summon.Pet.Life && plan.Summon.Pet.World.Epoch == r.ExecutionEpoch
	}
	return r.Pet == nil && r.PendingPetBirth != nil && r.PendingPetBirth.Token == plan.Summon.Token && plan.Summon.Token.ExecutionEpoch == r.ExecutionEpoch
}

func (w *worker) resolveSummonPet(r *room, origin *worldState, cast pendingCast, now time.Time) {
	plan := cast.Summon
	if plan == nil || r.Pending || r.Isolated {
		return
	}
	if !plan.Recall {
		defer w.cancelPetBirth(r, plan.Token)
	}
	if origin == nil || r.world(origin.Ref) != origin || cast.Caster.World.Epoch != r.ExecutionEpoch ||
		r.Player.Handle != cast.Caster.Handle || r.Player.Life != cast.Caster.Life || r.Player.HP <= 0 {
		return
	}
	if plan.Recall {
		if r.Pet == nil || r.Pet.HP <= 0 || r.Pet.IsPetCorpse || r.Pet.Handle != plan.Pet.Handle || r.Pet.Life != plan.Pet.Life || plan.Pet.World.Epoch != r.ExecutionEpoch {
			return
		}
		w.recallResidentPet(r)
		return
	}
	if r.Pet != nil || r.PendingPetBirth == nil || r.PendingPetBirth.Token != plan.Token {
		return
	}
	spawnWorld, point := origin, plan.Front
	m, known := w.worldMap(spawnWorld)
	if !known || !m.TeleportWalkable(point) {
		spawnWorld, point = r.currentWorld(), r.Player.Pos
		m, known = w.worldMap(spawnWorld)
	}
	// Fresh只在受理时检查NoPets；原Front失效时不添加来源没有的到期NoPets门禁。
	if !known || !m.TeleportWalkable(point) {
		return
	}
	pet, ok := w.newPetFromDefinition(r, plan.Definition, cast.SkillLevel, point, now)
	if !ok || !w.installReservedPet(r, spawnWorld, pet, plan.Token) {
		return
	}
	w.publishPetBorn(r, newPetBirthFact(r, spawnWorld, pet, cast, now))
}

func (w *worker) recallResidentPet(r *room) bool {
	pet := r.Pet
	if pet == nil || pet.HP <= 0 || pet.IsPetCorpse || pet.PetRuntime == nil || r.petResidentWorld() == nil || r.Pending || r.Isolated {
		return false
	}
	m, known := w.currentMap(r)
	if !known || m.NoPets == nil {
		return false
	}
	if *m.NoPets {
		pet.PetRuntime.FollowSuppressed = true
		w.cancelTownRequest(r.TownNavigation[1])
		return false
	}
	pet.PetRuntime.FollowSuppressed = false
	if r.worldPet(r.currentWorld()) == pet {
		return true
	}
	back := stepPoint(r.Player.Pos, (r.Player.Direction+4)%8, 1)
	if !m.TeleportWalkable(back) {
		back = r.Player.Pos
	}
	if !m.TeleportWalkable(back) {
		return false
	}
	return w.relocatePet(r, r.petReference(), r.currentWorld(), back)
}

// 自动跟随只调度唯一宠物，不激活旧地图怪物，不在ACK前发起新AI。
func (w *worker) tickResidentPetFollow(r *room, now time.Time) bool {
	pet := r.Pet
	if !w.sourcePetLifecycle() || pet == nil || pet.HP <= 0 || pet.PetRuntime == nil {
		return false
	}
	m, known := w.currentMap(r)
	// Crystal ProcessAI1686：Frozen且异图、主人允许时Recall后立即结束本次宠AI。
	skipPetAI := pet.PetRuntime.FollowSuppressed && r.petResidentWorld() != r.currentWorld() && known && m.NoPets != nil && !*m.NoPets
	if r.petResidentWorld() != r.currentWorld() || pet.PetRuntime.FollowSuppressed || distance(pet.Pos, r.Player.Pos) > pet.PetRuntime.Definition.SourceDataRangeCells() {
		w.recallResidentPet(r)
	}
	return skipPetAI
}

func (w *worker) killOwnedPet(r *room, now time.Time) {
	if !w.sourcePetLifecycle() || r.Pet == nil || r.Pet.HP <= 0 {
		return
	}
	world, pet := r.petResidentWorld(), r.Pet
	if world == nil {
		w.isolateRoom(r)
		return
	}
	// Player.Die直接调用宠物Die，不是再次Attacked；由外层一次推送角色与宠物完整终态。
	pet.HP, pet.Action, pet.ActionUntil = 0, 4, now.Add(200*time.Millisecond)
	removeDeathStatuses(pet)
	if !w.archivePetDeath(r, world, r.petReference(), now) {
		w.isolateRoom(r)
	}
}

// owner观察不把异图宠物加入当前场景实体列表；尸体与预生都没有活动Master。
func (w *worker) ownedPetPB(r *room) *pb.OwnedPetState {
	pet := r.Pet
	world := r.petResidentWorld()
	if pet == nil || pet.HP <= 0 || pet.IsPetCorpse || world == nil {
		return nil
	}
	m, known := w.worldMap(world)
	if !known {
		return nil
	}
	out := &pb.OwnedPetState{EntityHandle: pet.Handle, Life: pet.Life, DefinitionId: pet.Def, MapId: m.ID, Hp: pet.HP, HpMax: pet.MaxHP, PetTraining: petTrainingPB(pet)}
	if pet.PetRuntime != nil {
		out.FollowSuppressed = pet.PetRuntime.FollowSuppressed
		for _, skill := range w.e.Store.Cfg.Fixture.Skills {
			if skill.Summon == nil {
				continue
			}
			d, ok := w.e.Store.Cfg.Fixture.PetRuntime.DefinitionByID(skill.Summon.DefinitionID)
			if _, learned := r.Role.Skill(skill.ID); ok && learned && skill.JobID == r.Role.Job && d.RecallName != "" && d.RecallName == pet.PetRuntime.Definition.RecallName {
				out.RecallSkillIds = append(out.RecallSkillIds, skill.ID)
			}
		}
	}
	return out
}

// 不使用客户端给的材料/名称推断Recall；只读当前正式技能合同。
func (w *worker) isSummonSkill(id uint32) bool {
	s, ok := w.e.Store.Cfg.Fixture.Skill(id)
	return ok && s.Behavior == "summon_pet" && s.Summon != nil
}
