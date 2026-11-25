// 本文件为镜像两侧共用的临时召唤分配出生身份、合法地格及原尸体期限。
package game

import (
	"math"
	"slices"
	"time"

	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	p "minimir/internal/protocol"
)

// prepareMirrorSummon 核对空槽来源和本侧限额。实际扣料与挂入事件仍由镜像施法共用入口完成。
func (w *worker) prepareMirrorSummon(r *room, skill devconfig.Skill, direction uint32, now time.Time) (*summonPetPlan, uint32) {
	world := r.currentWorld()
	b := world.mirrorCombat
	side := &b.sides[world.mirrorSide]
	rule := b.config.Summoning
	if rule == nil || r.Pet != nil || side.pendingSummon != nil || !w.sourcePetLifecycle() || mapForbidsPets(&b.config.Map) ||
		side.births >= rule.MaxBirthsPerSide || side.births >= devconfig.MirrorArenaMaxBirthsPerSide ||
		side.entryPet.Handle == 0 && !rule.AllowWithoutEntryPet || r.CellEntrySequence == math.MaxUint64 ||
		!skill.AttackSlotSummon() || skill.ID != devconfig.SummonSkeletonSkillID && skill.ID != devconfig.SummonShinsuSkillID ||
		!b.acceptsActions(now.Add(time.Duration(skill.DelayBaseMS)*time.Millisecond)) {
		return nil, p.CodeSkillStateBlocked
	}
	definition, ok := w.e.Store.Cfg.Fixture.PetRuntime.DefinitionByID(skill.Summon.DefinitionID)
	if !ok || definition.OwnerJobID != r.Role.Job || definition.SourceSkillID != skill.ID || definition.RecallName == "" {
		return nil, p.CodeSkillStateBlocked
	}
	front, ok := w.mirrorSummonCell(r, direction)
	if !ok {
		return nil, p.CodeSkillTarget
	}
	return &summonPetPlan{Definition: definition, Front: front, Direction: direction, MirrorBirth: side.births + 1}, 0
}

// mirrorSummonCell 优先尝试施法面向，沿相邻方向找合法空格并保留对角墙角限制。
func (w *worker) mirrorSummonCell(r *room, direction uint32) (devconfig.Point, bool) {
	for _, turn := range [...]int{0, -1, 1, -2, 2, -3, 3, 4} {
		angle := uint32((int(direction) + turn + len(directions)) % len(directions))
		point := stepPoint(r.Player.Pos, angle, 1)
		if w.neighbor(r, r.Player.Pos, point, &r.Player) {
			return point, true
		}
	}
	return devconfig.Point{}, false
}

// mirrorSummonReserved 确认事件仍持有本场本侧唯一待出生名额。
func mirrorSummonReserved(r *room, plan *summonPetPlan) bool {
	world := r.currentWorld()
	if world.mirrorCombat == nil {
		return false
	}
	side := &world.mirrorCombat.sides[world.mirrorSide]
	return r.Pet == nil && side.pendingSummon == plan && plan.MirrorBirth == side.births+1
}

// resolveMirrorSummon 按原施法的到期时钟安装出生。生效前重新验证名额、施法者以及实际落格。
func (w *worker) resolveMirrorSummon(r *room, world *worldState, cast pendingCast, now time.Time) {
	plan := cast.Summon
	if world != r.currentWorld() || !mirrorSummonReserved(r, plan) {
		return
	}
	b := world.mirrorCombat
	side := &b.sides[world.mirrorSide]
	side.pendingSummon = nil
	if cast.Caster != referenceIn(world, &r.Player) || r.Player.HP <= 0 || !b.acceptsActions(cast.Due) {
		return
	}
	point := plan.Front
	if distance(r.Player.Pos, point) != 1 || !w.neighbor(r, r.Player.Pos, point, &r.Player) {
		var ok bool
		point, ok = w.mirrorSummonCell(r, plan.Direction)
		if !ok {
			return
		}
	}
	pet, ok := w.newPetFromDefinition(r, plan.Definition, cast.SkillLevel, point, now)
	if !ok || !w.commitRelocation(r, pet, point) {
		return
	}
	if cast.Skill.ID == devconfig.SummonShinsuSkillID {
		pet.Direction = plan.Direction
	}
	side.births = plan.MirrorBirth
	r.PetLifeSequence++
	pet.Life = r.PetLifeSequence
	pet.Handle = 103 + 2*(side.births-1) + uint32(world.mirrorSide)
	r.Pet, r.PetWorld = pet, world.Ref
	r.recordPetPosition()
	// 镜像出生身份从外层观战帧获取附着与序号，不携原角色的持久角色键。
	b.addObservation(p.MsgS2CPetBorn, &pb.PetBorn{SceneEpoch: r.Scene, EntityHandle: pet.Handle, EntityLife: pet.Life,
		DefinitionId: pet.Def, MapId: world.MapID, SourceCastId: cast.ID, SourceSkillId: cast.Skill.ID,
		BornAtUnixMs: uint64(now.UnixMilli()), MovementRevision: pet.MovementRevision, Direction: pet.Direction})
	b.observations[len(b.observations)-1].bornAt = now
}

// archiveMirrorPet 让原尸体退出参战槽，并保留该宠原定义规定的绝对期限。
func archiveMirrorPet(r *room, at time.Time) {
	pet := r.Pet
	world := r.currentWorld()
	if pet == nil || pet.HP != 0 || world.mirrorCombat.config.Summoning == nil {
		return
	}
	r.Pet, r.PetWorld, world.HasPet = nil, worldRef{}, false
	if pet.PetRuntime == nil || pet.PetRuntime.Definition.Lifecycle.CorpseTTLMS <= 0 {
		return
	}
	pet.IsPetCorpse = true
	world.PetCorpses = append(world.PetCorpses, &petCorpse{Body: pet,
		ExpiresAt: at.Add(time.Duration(pet.PetRuntime.Definition.Lifecycle.CorpseTTLMS) * time.Millisecond)})
}

// expireMirrorPetCorpses 只清除已到期的非参战尸体。后续事件按原句柄和生命查不到旧实体。
func expireMirrorPetCorpses(r *room, at time.Time) {
	world := r.currentWorld()
	world.PetCorpses = slices.DeleteFunc(world.PetCorpses, func(corpse *petCorpse) bool { return !at.Before(corpse.ExpiresAt) })
}
