package game

import (
	"math"
	"time"
)

// 尸体仍是原对象，World只接管所有权与绝对截止，不更换其身份或重建临时现场。
type petCorpse struct {
	Body      *entity
	ExpiresAt time.Time
}

type petBirthToken struct {
	ID             string
	ExecutionEpoch uint64
	PlayerLife     uint64
}

type petBirthReservation struct{ Token petBirthToken }

func (w *worker) petRuntimeEnabled() bool {
	rules := w.e.Store.Cfg.Fixture.PetRuntime
	return rules != nil && rules.Enabled
}

func (r *room) petBodies(world *worldState) []*entity {
	all := []*entity{}
	if pet := r.worldPet(world); pet != nil {
		all = append(all, pet)
	}
	if world != nil {
		for _, corpse := range world.PetCorpses {
			all = append(all, corpse.Body)
		}
	}
	return all
}

func (r *room) retainedPetBodies() int {
	count := 0
	if r.Pet != nil {
		count++
	}
	if r.PendingPetBirth != nil {
		count++
	}
	for _, world := range r.worlds() {
		count += len(world.PetCorpses)
	}
	return count
}

func (r *room) availablePetHandle() (uint32, bool) {
	high := r.PetHandleSequence
	if high < petHandle {
		high = petHandle
	}
	used := func(handle uint32) bool {
		if handle == 0 || handle == 1 || handle >= 10 && handle <= 17 || handle == 1000 || r.Player.Handle == handle || r.Pet != nil && r.Pet.Handle == handle {
			return true
		}
		for _, world := range r.worlds() {
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

func (w *worker) canBeginPetBirth(r *room, now time.Time) bool {
	if !w.petRuntimeEnabled() || r.Pending || r.Isolated || r.Pet != nil || r.PendingPetBirth != nil {
		return false
	}
	w.expirePetCorpses(r, now)
	_, available := r.availablePetHandle()
	return available && r.PetLifeSequence != math.MaxUint64 && r.CellEntrySequence != math.MaxUint64 && r.retainedPetBodies() < w.e.Store.Cfg.Fixture.PetRuntime.Budgets.RetainedBodyCapacityPerOwner
}

func (w *worker) reservePetBirth(r *room, token petBirthToken, now time.Time) bool {
	r.currentWorld()
	if token.ID == "" || token.ExecutionEpoch != r.ExecutionEpoch || token.PlayerLife != r.Player.Life {
		return false
	}
	if pending := r.PendingPetBirth; pending != nil {
		return pending.Token == token
	}
	if !w.canBeginPetBirth(r, now) {
		return false
	}
	r.PendingPetBirth = &petBirthReservation{Token: token}
	return true
}

func (w *worker) cancelPetBirth(r *room, token petBirthToken) bool {
	if r.PendingPetBirth == nil || r.PendingPetBirth.Token != token || token.ExecutionEpoch != r.ExecutionEpoch {
		return false
	}
	r.PendingPetBirth = nil
	return true
}

func (w *worker) archivePetDeath(r *room, world *worldState, expected actorRef, now time.Time) bool {
	if !w.petRuntimeEnabled() || world == nil || r.worldPet(world) == nil || r.petReference() != expected || r.Pet.HP != 0 ||
		r.retainedPetBodies() > w.e.Store.Cfg.Fixture.PetRuntime.Budgets.RetainedBodyCapacityPerOwner || !w.attachLegacyPetState(r.Pet) {
		return false
	}
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

func (w *worker) expirePetCorpses(r *room, now time.Time) bool {
	if !w.petRuntimeEnabled() || r.Pending || r.Isolated {
		return false
	}
	changed := false
	for _, world := range r.worlds() {
		before := world.PetCorpses
		keep := before[:0]
		for _, corpse := range before {
			if now.Before(corpse.ExpiresAt) {
				keep = append(keep, corpse)
				continue
			}
			changed = true
			ref := referenceIn(world, corpse.Body)
			// Recall排在主人受理图的共享队列，但动作归原宠物；Despawn须清掉所有World里的该birth动作。
			for _, queuedWorld := range r.worlds() {
				keepCasts := queuedWorld.Casts[:0]
				for _, cast := range queuedWorld.Casts {
					ownedRecall := cast.Summon != nil && cast.Summon.Recall && cast.Summon.Pet.World.Epoch == ref.World.Epoch && cast.Summon.Pet.Handle == ref.Handle && cast.Summon.Pet.Life == ref.Life
					if !ownedRecall {
						keepCasts = append(keepCasts, cast)
					}
				}
				queuedWorld.Casts = keepCasts
			}
			attacks := world.Attacks[:0]
			for _, attack := range world.Attacks {
				if attack.Caster != ref {
					attacks = append(attacks, attack)
				}
			}
			world.Attacks = attacks
			casts := world.Casts[:0]
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
