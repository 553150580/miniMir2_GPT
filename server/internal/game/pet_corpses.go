// 本文件维护宠物实体各自的尸体保留周期、句柄分配和出生容量。
package game

import (
	"math"
	"time"
)

// 尸体仍是原对象，World只接管所有权与绝对截止，不更换其身份或重建临时现场。
type petCorpse struct {
	// Body 记录消息正文，使用实体表示。
	Body *entity
	// ExpiresAt 记录过期集合发生时间，使用时间点表示。
	ExpiresAt time.Time
}

// petBirthToken 保存宠物出生令牌所需的状态。
type petBirthToken struct {
	// ID 区分当前步骤指向的目标对象。
	ID string
	// ExecutionEpoch 记录“execution”代次，使用数值表示。
	ExecutionEpoch uint64
	// PlayerLife 记录玩家生命代次，使用数值表示。
	PlayerLife uint64
}

// petBirthReservation 保存宠物出生“reservation”所需的状态。
// Token 记录令牌，使用宠物出生令牌表示。
type petBirthReservation struct{ Token petBirthToken }

// petRuntimeEnabled 判断宠物运行时已开启是否满足要求。
// w 是调用该方法的房间执行器实例。
func (w *worker) petRuntimeEnabled() bool {
	// rules 保存宠物运行时，用于当前步骤中的规则集合。
	rules := w.e.Store.Cfg.Fixture.PetRuntime
	return rules != nil && rules.Enabled
}

// petBodies 返回宠物“bodies”供调用方使用。
// r 是调用该方法的房间实例，world 记录场景世界，使用场景世界状态表示。
func (r *room) petBodies(world *worldState) []*entity {
	// all 初始化用作全部的实体列表。
	all := []*entity{}
	// pet 保存由场景世界宠物取得的宠物。
	if pet := r.worldPet(world); pet != nil {
		all = append(all, pet)
	}
	if world != nil {
		// corpse 接收遍历宠物尸体集合时当前的元素。
		for _, corpse := range world.PetCorpses {
			all = append(all, corpse.Body)
		}
	}
	return all
}

// retainedPetBodies 返回“retained”宠物“bodies”供调用方使用。
// r 是调用该方法的房间实例。
func (r *room) retainedPetBodies() int {
	// count 规定数量的初始取值。
	count := 0
	if r.Pet != nil {
		count++
	}
	if r.PendingPetBirth != nil {
		count++
	}
	// world 接收遍历场景世界集合时当前的元素。
	for _, world := range r.worlds() {
		count += len(world.PetCorpses)
	}
	return count
}

// availablePetHandle 处理“available”宠物实体句柄。
// r 是调用该方法的房间实例。
func (r *room) availablePetHandle() (uint32, bool) {
	// high 保存宠物实体句柄序号，用于当前步骤中的“high”。
	high := r.PetHandleSequence
	if high < petHandle {
		high = petHandle
	}
	// used 封装“used”对应的后续处理，handle 记录实体句柄，使用数值表示。
	used := func(handle uint32) bool {
		if handle == 0 || handle == 1 || handle >= 10 && handle <= 17 || handle == 1000 || r.Player.Handle == handle || r.Pet != nil && r.Pet.Handle == handle {
			return true
		}
		// world 接收遍历场景世界集合时当前的元素。
		for _, world := range r.worlds() {
			// e 接收遍历“append”时当前的元素。
			for _, e := range append(append(world.Monsters[:len(world.Monsters):len(world.Monsters)], world.Loot...), r.petBodies(world)...) {
				if e != nil && e.Handle == handle {
					return true
				}
			}
		}
		return false
	}
	for high < math.MaxUint32 {
		high++
		if !used(high) {
			return high, true
		}
	}
	return 0, false
}

// canBeginPetBirth 判断能否执行“begin”宠物出生。
// w 是调用该方法的房间执行器实例，r 指向当前执行器拥有并更新的目标房间，now 提供当前时点，作为状态有效期和计时判断的基准。
func (w *worker) canBeginPetBirth(r *room, now time.Time) bool {
	if !w.petRuntimeEnabled() || r.Pending || r.Isolated || r.Pet != nil || r.PendingPetBirth != nil {
		return false
	}
	w.expirePetCorpses(r, now)
	// available 保存由“available”宠物实体句柄取得的“available”。
	_, available := r.availablePetHandle()
	return available && r.PetLifeSequence != math.MaxUint64 && r.CellEntrySequence != math.MaxUint64 && r.retainedPetBodies() < w.e.Store.Cfg.Fixture.PetRuntime.Budgets.RetainedBodyCapacityPerOwner
}

// reservePetBirth 判断保留宠物出生是否满足要求。
// w 是调用该方法的房间执行器实例，r 指向当前执行器拥有并更新的目标房间，token 记录令牌，使用宠物出生令牌表示，now 提供当前时点，作为状态有效期和计时判断的基准。
func (w *worker) reservePetBirth(r *room, token petBirthToken, now time.Time) bool {
	r.currentWorld()
	if token.ID == "" || token.ExecutionEpoch != r.ExecutionEpoch || token.PlayerLife != r.Player.Life {
		return false
	}
	// pending 保存待确认宠物出生，用于当前步骤中的待确认。
	if pending := r.PendingPetBirth; pending != nil {
		return pending.Token == token
	}
	if !w.canBeginPetBirth(r, now) {
		return false
	}
	r.PendingPetBirth = &petBirthReservation{Token: token}
	return true
}

// cancelPetBirth 判断取消函数宠物出生是否满足要求。
// w 是调用该方法的房间执行器实例，r 指向当前执行器拥有并更新的目标房间，token 记录令牌，使用宠物出生令牌表示。
func (w *worker) cancelPetBirth(r *room, token petBirthToken) bool {
	if r.PendingPetBirth == nil || r.PendingPetBirth.Token != token || token.ExecutionEpoch != r.ExecutionEpoch {
		return false
	}
	r.PendingPetBirth = nil
	return true
}

// archivePetDeath 判断“archive”宠物死亡是否满足要求。
// w 是调用该方法的房间执行器实例，r 指向当前执行器拥有并更新的目标房间，world 记录场景世界，使用场景世界状态表示，expected 记录预期结果，使用行动实体引用表示。
// now 提供当前时点，作为状态有效期和计时判断的基准。
func (w *worker) archivePetDeath(r *room, world *worldState, expected actorRef, now time.Time) bool {
	if !w.petRuntimeEnabled() || world == nil || r.worldPet(world) == nil || r.petReference() != expected || r.Pet.HP != 0 ||
		r.retainedPetBodies() > w.e.Store.Cfg.Fixture.PetRuntime.Budgets.RetainedBodyCapacityPerOwner || !w.attachLegacyPetState(r.Pet) {
		return false
	}
	// pet 保存当前处理所用的宠物。
	pet := r.Pet
	w.cancelTownRequest(r.TownNavigation[1])
	pet.IsPetCorpse = true
	world.PetCorpses = append(world.PetCorpses, &petCorpse{Body: pet, ExpiresAt: now.Add(time.Duration(pet.PetRuntime.Definition.Lifecycle.CorpseTTLMS) * time.Millisecond)})
	if pet.Life > r.PetLifeSequence {
		r.PetLifeSequence = pet.Life
	}
	if pet.Handle > r.PetHandleSequence {
		r.PetHandleSequence = pet.Handle
	}
	r.Pet, r.PetWorld = nil, worldRef{}
	return true
}

// expirePetCorpses 清理过期的宠物尸体集合。
// w 是调用该方法的房间执行器实例，r 指向当前执行器拥有并更新的目标房间，now 提供当前时点，作为状态有效期和计时判断的基准。
func (w *worker) expirePetCorpses(r *room, now time.Time) bool {
	if !w.petRuntimeEnabled() || r.Pending || r.Isolated {
		return false
	}
	// changed 保存当前处理所用的已变化。
	changed := false
	// world 接收遍历场景世界集合时当前的元素。
	for _, world := range r.worlds() {
		// before 保存宠物尸体集合，用于当前步骤中的操作前的状态。
		before := world.PetCorpses
		// keep 保存当前处理所用的“keep”。
		keep := before[:0]
		// corpse 接收遍历操作前的状态时当前的元素。
		for _, corpse := range before {
			if now.Before(corpse.ExpiresAt) {
				keep = append(keep, corpse)
				continue
			}
			changed = true
			// ref 保存由引用输入取得的引用。
			ref := referenceIn(world, corpse.Body)
			// Recall排在主人受理图的共享队列，但动作归原宠物；Despawn须清掉所有World里的该birth动作。
			// queuedWorld 接收遍历场景世界集合时当前的元素。
			for _, queuedWorld := range r.worlds() {
				// keepCasts 保存当前处理所用的“keep”施法集合。
				keepCasts := queuedWorld.Casts[:0]
				// cast 接收遍历施法集合时当前的元素。
				for _, cast := range queuedWorld.Casts {
					// ownedRecall 保存根据召唤与“nil”的计算结果与召回的计算结果与代次与代次的计算结果的计算结果与实体句柄与实体句柄的计算结果的计算结果与生命代次与生命代次的计算结果计算的“owned”召回。
					ownedRecall := cast.Summon != nil && cast.Summon.Recall && cast.Summon.Pet.World.Epoch == ref.World.Epoch && cast.Summon.Pet.Handle == ref.Handle && cast.Summon.Pet.Life == ref.Life
					if !ownedRecall {
						keepCasts = append(keepCasts, cast)
					}
				}
				queuedWorld.Casts = keepCasts
			}
			// attacks 保存当前处理所用的攻击集合。
			attacks := world.Attacks[:0]
			// attack 接收遍历攻击集合时当前的元素。
			for _, attack := range world.Attacks {
				if attack.Caster != ref {
					attacks = append(attacks, attack)
				}
			}
			world.Attacks = attacks
			// casts 保存当前处理所用的施法集合。
			casts := world.Casts[:0]
			// cast 接收遍历施法集合时当前的元素。
			for _, cast := range world.Casts {
				if cast.Caster != ref || cast.Skill.Behavior == "hell_fire" {
					casts = append(casts, cast)
				}
			}
			world.Casts = casts
		}
		clear(before[len(keep):])
		world.PetCorpses = keep
	}
	return changed
}
