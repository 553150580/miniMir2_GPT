package game

import (
	"math"
	"minimir/internal/devconfig"
)

const petHandle uint32 = 2

// 宠物归唯一owner所有，但居民World独立于主人的当前World。
// 零值仅兼容旧手造测试room；正式initialize与installPet总是写入明确身份。
func (r *room) petResidentWorld() *worldState {
	if r.Pet == nil {
		return nil
	}
	if r.PetWorld == (worldRef{}) {
		return r.currentWorld()
	}
	return r.world(r.PetWorld)
}

func (r *room) worldPet(world *worldState) *entity {
	if world == nil || r.petResidentWorld() != world {
		return nil
	}
	return r.Pet
}

func (r *room) petReference() actorRef {
	if r.Pet == nil {
		return actorRef{}
	}
	ref := r.PetWorld
	if ref == (worldRef{}) {
		ref = r.currentWorld().Ref
	}
	return actorRef{World: ref, Handle: r.Pet.Handle, Life: r.Pet.Life}
}

func (r *room) petAway(e *entity) bool {
	return e != nil && e == r.Pet && r.worldPet(r.currentWorld()) != e
}

func (r *room) recordPetPosition() {
	if world := r.petResidentWorld(); world != nil {
		world.HasPet, world.PetLife, world.PetPosition = true, r.Pet.Life, r.Pet.Pos
	}
}

// 新birth只在真实安装时分配；预建召唤计划不占居民槽，也不提前耗尽birth。
// 已有宠物（含尸体）不会被此API隐式覆盖，退役与尸体处理由来源业务显式决定。
func (w *worker) installPet(r *room, world *worldState, pet *entity) bool {
	return w.installPetWithReservation(r, world, pet, nil)
}

func (w *worker) installReservedPet(r *room, world *worldState, pet *entity, token petBirthToken) bool {
	return w.installPetWithReservation(r, world, pet, &token)
}

func (w *worker) installPetWithReservation(r *room, world *worldState, pet *entity, token *petBirthToken) bool {
	if r == nil || world == nil || pet == nil || r.Pet != nil || r.Pending || r.Isolated || r.world(world.Ref) != world ||
		pet.Kind != 2 || pet.Life != 0 || pet.IsPetCorpse || pet.HP <= 0 || pet.MaxHP < pet.HP || pet.MP < 0 || pet.MaxMP < pet.MP ||
		r.PetLifeSequence == math.MaxUint64 || r.CellEntrySequence == math.MaxUint64 {
		return false
	}
	handle := petHandle
	if w.petRuntimeEnabled() {
		reserved := r.PendingPetBirth != nil
		if pet.Handle != 0 || pet.PetRuntime == nil || reserved && (token == nil || r.PendingPetBirth.Token != *token || token.ExecutionEpoch != r.ExecutionEpoch || token.PlayerLife != r.Player.Life) || !reserved && token != nil {
			return false
		}
		additional := 1
		if reserved {
			additional = 0
		}
		if r.retainedPetBodies()+additional > w.e.Store.Cfg.Fixture.PetRuntime.Budgets.RetainedBodyCapacityPerOwner {
			return false
		}
		var available bool
		handle, available = r.availablePetHandle()
		if !available {
			return false
		}
	} else if pet.Handle != petHandle || token != nil {
		return false
	}
	m, known := w.worldMap(world)
	if !known || !m.Contains(pet.Pos) {
		return false
	}
	w.cancelTownRequest(r.TownNavigation[1])
	r.PetLifeSequence++
	pet.Life, pet.Handle = r.PetLifeSequence, handle
	r.PetHandleSequence, r.PendingPetBirth = handle, nil
	r.Pet, r.PetWorld = pet, world.Ref
	w.commitRelocation(r, pet, pet.Pos)
	return true
}

// 返回原对象供调用方完成尸体/来源退役处理；不销毁、不复活、不重建其临时状态。
// 正常死亡不能自动调用本方法，否则会删除尚应可见的resident尸体。
func (w *worker) retirePet(r *room, expected actorRef) (*entity, bool) {
	if w.petRuntimeEnabled() {
		return nil, false // 启用源生命周期后必须死亡转交World，不能用清slot丢弃原身体。
	}
	if r == nil || r.Pet == nil || r.Pending || r.Isolated || r.petReference() != expected || r.petResidentWorld() == nil {
		return nil, false
	}
	pet := r.Pet
	if pet.Life > r.PetLifeSequence {
		r.PetLifeSequence = pet.Life
	}
	r.recordPetPosition()
	w.cancelTownRequest(r.TownNavigation[1])
	r.Pet, r.PetWorld = nil, worldRef{}
	return pet, true
}

// 仅执行已由来源业务选定地形与落点的宠物移交，不移动主人或更换attachment。
func (w *worker) relocatePet(r *room, expected actorRef, target *worldState, point devconfig.Point) bool {
	if r == nil || r.Pet == nil || r.Pending || r.Isolated || r.petReference() != expected || r.petResidentWorld() == nil ||
		target == nil || r.world(target.Ref) != target || r.Pet.MovementRevision == math.MaxUint64 || r.CellEntrySequence == math.MaxUint64 {
		return false
	}
	m, known := w.worldMap(target)
	if !known || !m.Contains(point) {
		return false
	}
	w.cancelTownRequest(r.TownNavigation[1])
	if !w.beginMovement(r, r.Pet, movementTeleport) {
		return false
	}
	r.recordPetPosition()
	r.PetWorld = target.Ref
	return w.commitRelocation(r, r.Pet, point)
}
