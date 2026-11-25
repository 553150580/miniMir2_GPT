// 本文件维护唯一活宠的出生、驻留、退役和迁移，使用原生命引用校验操作。
package game

import (
	"math"
	"minimir/internal/devconfig"
)

// petHandle 规定宠物实体句柄的初始取值。
const petHandle uint32 = 2

// 宠物归唯一owner所有，但居民World独立于主人的当前World。
// 零值仅兼容旧手造测试room；正式initialize与installPet总是写入明确身份。
// r 是调用该方法的房间实例。
func (r *room) petResidentWorld() *worldState {
	if r.Pet == nil {
		return nil
	}
	if r.PetWorld == (worldRef{}) {
		return r.currentWorld()
	}
	return r.world(r.PetWorld)
}

// worldPet 返回场景世界宠物供调用方使用。
// r 是调用该方法的房间实例，world 记录场景世界，使用场景世界状态表示。
func (r *room) worldPet(world *worldState) *entity {
	if world == nil || r.petResidentWorld() != world {
		return nil
	}
	return r.Pet
}

// petReference 返回宠物引用供调用方使用。
// r 是调用该方法的房间实例。
func (r *room) petReference() actorRef {
	if r.Pet == nil {
		return actorRef{}
	}
	// ref 保存宠物场景世界，用于当前步骤中的引用。
	ref := r.PetWorld
	if ref == (worldRef{}) {
		ref = r.currentWorld().Ref
	}
	return actorRef{World: ref, Handle: r.Pet.Handle, Life: r.Pet.Life}
}

// matchesActivePet 按同一房间执行代与生命匹配活宠，允许该宠已随主人迁入另一个地图库。
func (r *room) matchesActivePet(ref actorRef) bool {
	return ref.Handle != 0 && ref.World.Epoch == r.ExecutionEpoch && r.Pet != nil && r.Pet.HP > 0 &&
		!r.Pet.IsPetCorpse && r.Pet.Handle == ref.Handle && r.Pet.Life == ref.Life && r.petResidentWorld() != nil
}

// pendingPetBirthMatches 核对预约和当前宠物名额。预约后旧宠自然死亡仍允许新生，旧尸体独立保留。
func (r *room) pendingPetBirthMatches(token petBirthToken) bool {
	return r.PendingPetBirth != nil && r.PendingPetBirth.Token == token && token.ExecutionEpoch == r.ExecutionEpoch &&
		token.PlayerLife == r.Player.Life && (r.Pet == nil || r.matchesActivePet(token.Replacing))
}

// petAway 判断宠物“away”是否满足要求。
// r 是调用该方法的房间实例，e 记录实体。
func (r *room) petAway(e *entity) bool {
	return e != nil && e == r.Pet && r.worldPet(r.currentWorld()) != e
}

// recordPetPosition 处理记录宠物位置。
// r 是调用该方法的房间实例。
func (r *room) recordPetPosition() {
	// world 保存由宠物“resident”场景世界取得的场景世界。
	if world := r.petResidentWorld(); world != nil {
		world.HasPet, world.PetLife, world.PetPosition = true, r.Pet.Life, r.Pet.Pos
	}
}

// 新birth只在真实安装时分配；预建召唤计划不占居民槽，也不提前耗尽birth。
// 已有宠物（含尸体）不会被此API隐式覆盖，退役与尸体处理由来源业务显式决定。
// w 是调用该方法的房间执行器实例，r 指向当前执行器拥有并更新的目标房间，world 记录场景世界，使用场景世界状态表示，pet 记录宠物，使用实体表示。
func (w *worker) installPet(r *room, world *worldState, pet *entity) bool {
	return w.installPetWithReservation(r, world, pet, nil)
}

// installReservedPet 判断“install”已保留宠物是否满足要求。
// w 是调用该方法的房间执行器实例，r 指向当前执行器拥有并更新的目标房间，world 记录场景世界，使用场景世界状态表示，pet 记录宠物，使用实体表示。
// token 记录令牌，使用宠物出生令牌表示。
func (w *worker) installReservedPet(r *room, world *worldState, pet *entity, token petBirthToken) bool {
	return w.installPetWithReservation(r, world, pet, &token)
}

// installPetWithReservation 判断“install”宠物带有“reservation”是否满足要求。
// w 是调用该方法的房间执行器实例，r 指向当前执行器拥有并更新的目标房间，world 记录场景世界，使用场景世界状态表示，pet 记录宠物，使用实体表示。
// token 记录令牌，使用宠物出生令牌表示。
func (w *worker) installPetWithReservation(r *room, world *worldState, pet *entity, token *petBirthToken) bool {
	if r == nil || world == nil || pet == nil || r.Pending || r.Isolated || r.world(world.Ref) != world ||
		pet.Kind != 2 || pet.Life != 0 || pet.IsPetCorpse || pet.HP <= 0 || pet.MaxHP < pet.HP || pet.MP < 0 || pet.MaxMP < pet.MP ||
		r.PetLifeSequence == math.MaxUint64 || r.CellEntrySequence == math.MaxUint64 {
		return false
	}
	if r.Pet != nil && token == nil {
		return false
	}
	// handle 保存当前处理所用的实体句柄。
	handle := petHandle
	if w.petRuntimeEnabled() {
		// reserved 保存根据待确认宠物出生与“nil”计算的已保留。
		reserved := r.PendingPetBirth != nil
		if pet.Handle != 0 || pet.PetRuntime == nil || reserved != (token != nil) || token != nil && !r.pendingPetBirthMatches(*token) {
			return false
		}
		// additional 规定“additional”的初始取值。
		additional := 1
		if reserved {
			additional = 0
		}
		if r.retainedPetBodies()+additional > w.e.Store.Cfg.Fixture.PetRuntime.Budgets.RetainedBodyCapacityPerOwner {
			return false
		}
		// available 标记是否处于“available”状态。
		var available bool
		handle, available = r.availablePetHandle()
		if !available {
			return false
		}
	} else if pet.Handle != petHandle || token != nil {
		return false
	}
	// m 接收场景世界地图供后续步骤使用，known 保存由场景世界地图取得的已知状态。
	m, known := w.worldMap(world)
	if !known || !m.Contains(pet.Pos) {
		return false
	}
	// 验证均已通过。直接接替名额，如果旧宠已自然死亡，其尸体仍由原地图持有。
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
// w 是调用该方法的房间执行器实例，r 指向当前执行器拥有并更新的目标房间，expected 记录预期结果，使用行动实体引用表示。
func (w *worker) retirePet(r *room, expected actorRef) (*entity, bool) {
	if w.petRuntimeEnabled() {
		return nil, false // 启用源生命周期后必须死亡转交World，不能用清slot丢弃原身体。
	}
	if r == nil || r.Pet == nil || r.Pending || r.Isolated || r.petReference() != expected || r.petResidentWorld() == nil {
		return nil, false
	}
	// pet 保存当前处理所用的宠物。
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
// w 是调用该方法的房间执行器实例，r 指向当前执行器拥有并更新的目标房间，expected 记录预期结果，使用行动实体引用表示，target 记录目标，使用场景世界状态表示。
// point 记录点位。
func (w *worker) relocatePet(r *room, expected actorRef, target *worldState, point devconfig.Point) bool {
	if r == nil || r.Pet == nil || r.Pending || r.Isolated || r.petReference() != expected || r.petResidentWorld() == nil ||
		target == nil || r.world(target.Ref) != target || r.Pet.MovementRevision == math.MaxUint64 || r.CellEntrySequence == math.MaxUint64 {
		return false
	}
	// m 接收场景世界地图供后续步骤使用，known 保存由场景世界地图取得的已知状态。
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
	moved := w.commitRelocation(r, r.Pet, point)
	r.releasePetHoldingWorld()
	return moved
}
