// 本文件处理召唤宠物技能、原召唤计划预约、已驻留宠物召回、地图禁宠和主人死亡事件。
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
	// Recall 标记是否处于召回状态。
	Recall bool
	// Pet 锁定本次召回或切换所涉及的旧宠物出生身份。
	Pet actorRef
	// Token 记录令牌，使用宠物出生令牌表示。
	Token petBirthToken
	// Definition 记录定义，使用宠物运行时定义表示。
	Definition devconfig.PetRuntimeDefinition
	// Front 保存受理时玩家前方的出生格。
	Front devconfig.Point
	// Direction 保存受理时的施法朝向，供继承主人朝向的宠物出生使用。
	Direction uint32
}

// Crystal BoneFamiliar构造使用DownLeft，属于固定八向枚举，不是玩法调参。
const skeletonSpawnDirection uint32 = 5

// sourcePetLifecycle 判断来源宠物“lifecycle”是否满足要求。
// w 是调用该方法的房间执行器实例。
func (w *worker) sourcePetLifecycle() bool {
	return w.petRuntimeEnabled() && w.e.Store.Cfg.Fixture.RuleVersion >= 17
}

// mapForbidsPets 判断地图“forbids”宠物集合是否满足要求。
// m 记录地图模板。
func mapForbidsPets(m *devconfig.MapTemplate) bool { return m != nil && m.NoPets != nil && *m.NoPets }

// 仅在初始化或真实跨图时调用；Fresh fallback和同图Teleport不是重新入图。
// w 是调用该方法的房间执行器实例，r 指向当前执行器拥有并更新的目标房间，m 记录地图模板。
func (w *worker) applyPetMapEntryPolicy(r *room, m *devconfig.MapTemplate) {
	if r.Pet == nil || r.Pet.PetRuntime == nil {
		return
	}
	r.Pet.PetRuntime.FollowSuppressed = mapForbidsPets(m)
	if r.Pet.PetRuntime.FollowSuppressed {
		w.cancelTownRequest(r.TownNavigation[1])
	}
}

// prepareSummonPet 处理准备召唤宠物。
// w 是调用该方法的房间执行器实例，r 指向当前执行器拥有并更新的目标房间，skill 记录技能，direction 记录朝向，使用数值表示。
// now 提供当前时点，作为状态有效期和计时判断的基准。
func (w *worker) prepareSummonPet(r *room, skill devconfig.Skill, direction uint32, now time.Time) (*summonPetPlan, uint32) {
	// m 接收当前状态地图供后续步骤使用，known 保存由当前状态地图取得的已知状态。
	m, known := w.currentMap(r)
	if !w.sourcePetLifecycle() || !known || m.NoPets == nil || *m.NoPets || skill.Summon == nil {
		return nil, p.CodeSkillStateBlocked
	}
	// d 接收定义“by”标识供后续步骤使用，ok 表明查询、校验或类型转换是否成功。
	d, ok := w.e.Store.Cfg.Fixture.PetRuntime.DefinitionByID(skill.Summon.DefinitionID)
	if !ok || d.RecallName == "" || d.OwnerJobID != r.Role.Job || d.SourceSkillID != skill.ID {
		return nil, p.CodeSkillStateBlocked
	}
	// plan 保存当前处理所用的计划。
	plan := &summonPetPlan{Definition: d, Front: stepPoint(r.Player.Pos, direction, 1), Direction: direction}
	// pet 保存当前处理所用的宠物。
	if pet := r.Pet; pet != nil {
		if pet.HP <= 0 || pet.IsPetCorpse || r.petResidentWorld() == nil || pet.PetRuntime == nil {
			return nil, p.CodeSkillStateBlocked
		}
		plan.Pet = r.petReference()
		if pet.PetRuntime.Definition.RecallName == d.RecallName {
			plan.Recall = true
			return plan, 0
		}
		// 只有同职业的另一种正式召唤宠可切换，来源不明或诱惑获得的活宠保留原名额。
		old := pet.PetRuntime.Definition
		if old.SourceSkillID == skill.ID || old.OwnerJobID != r.Role.Job || !w.isSummonSkill(old.SourceSkillID) {
			return nil, p.CodeSkillStateBlocked
		}
	}
	if !w.canBeginPetBirth(r, now, plan.Pet) {
		return nil, p.CodeSkillStateBlocked
	}
	return plan, 0
}

// reserveSummonPet 判断保留召唤宠物是否满足要求。
// w 是调用该方法的房间执行器实例，r 指向当前执行器拥有并更新的目标房间，plan 记录计划，使用“prepared”施法表示，operationID 标识当前业务使用的操作。
// now 提供当前时点，作为状态有效期和计时判断的基准。
func (w *worker) reserveSummonPet(r *room, plan *preparedCast, operationID string, now time.Time) bool {
	if plan == nil || plan.Summon == nil || plan.Summon.Recall {
		return true
	}
	if r.ScheduleID == math.MaxUint64 || r.CastID == math.MaxUint64 {
		return false
	}
	plan.Summon.Token = petBirthToken{ID: operationID, ExecutionEpoch: r.ExecutionEpoch, PlayerLife: plan.Caster.Life, Replacing: plan.Summon.Pet}
	return w.reservePetBirth(r, plan.Summon.Token, now)
}

// cancelSummonPet 处理取消函数召唤宠物。
// w 是调用该方法的房间执行器实例，r 指向当前执行器拥有并更新的目标房间，plan 记录计划，使用“prepared”施法表示。
func (w *worker) cancelSummonPet(r *room, plan *preparedCast) {
	if plan != nil && plan.Summon != nil && !plan.Summon.Recall {
		w.cancelPetBirth(r, plan.Summon.Token)
	}
}

// summonPlanCurrent 判断召唤计划当前状态是否满足要求。
// w 是调用该方法的房间执行器实例，r 指向当前执行器拥有并更新的目标房间，plan 记录计划，使用“prepared”施法表示。
func (w *worker) summonPlanCurrent(r *room, plan *preparedCast) bool {
	if plan.Summon == nil {
		return true
	}
	if plan.Summon.Recall {
		return r.matchesActivePet(plan.Summon.Pet)
	}
	return r.pendingPetBirthMatches(plan.Summon.Token)
}

// resolveSummonPet 确定“summon”宠物。
// w 是调用该方法的房间执行器实例，r 指向当前执行器拥有并更新的目标房间，origin 记录大区或起点，使用场景世界状态表示，cast 记录施法，使用待确认施法表示。
// now 提供当前时点，作为状态有效期和计时判断的基准。
func (w *worker) resolveSummonPet(r *room, origin *worldState, cast pendingCast, now time.Time) {
	// plan 保存召唤，用于当前步骤中的计划。
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
		if !r.matchesActivePet(plan.Pet) {
			return
		}
		w.recallResidentPet(r)
		return
	}
	if !r.pendingPetBirthMatches(plan.Token) {
		return
	}
	// spawnWorld 保存当前处理所用的生成或出生点场景世界，point 保存前端，用于当前步骤中的点位。
	spawnWorld, point := origin, plan.Front
	// m 接收场景世界地图供后续步骤使用，known 保存由场景世界地图取得的已知状态。
	m, known := w.worldMap(spawnWorld)
	if !known || !m.TeleportWalkable(point) {
		spawnWorld, point = r.currentWorld(), r.Player.Pos
		m, known = w.worldMap(spawnWorld)
	}
	// Fresh只在受理时检查NoPets；原Front失效时不添加来源没有的到期NoPets门禁。
	if !known || !m.TeleportWalkable(point) {
		return
	}
	// pet 保存由创建宠物来源定义取得的宠物，ok 表明查询、校验或类型转换是否成功。
	pet, ok := w.newPetFromDefinition(r, plan.Definition, cast.SkillLevel, point, now)
	if !ok {
		return
	}
	if plan.Definition.SourceSkillID == devconfig.SummonShinsuSkillID {
		pet.Direction = plan.Direction
	}
	if !w.installReservedPet(r, spawnWorld, pet, plan.Token) {
		return
	}
	w.publishPetBorn(r, newPetBirthFact(r, spawnWorld, pet, cast, now))
}

// recallResidentPet 判断召回“resident”宠物是否满足要求。
// w 是调用该方法的房间执行器实例，r 指向当前执行器拥有并更新的目标房间。
func (w *worker) recallResidentPet(r *room) bool {
	// pet 保存当前处理所用的宠物。
	pet := r.Pet
	if pet == nil || pet.HP <= 0 || pet.IsPetCorpse || pet.PetRuntime == nil || r.petResidentWorld() == nil || r.Pending || r.Isolated {
		return false
	}
	// m 接收当前状态地图供后续步骤使用，known 保存由当前状态地图取得的已知状态。
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
	// back 保存由步骤点位取得的回退。
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
// w 是调用该方法的房间执行器实例，r 指向当前执行器拥有并更新的目标房间，now 提供当前时点，作为状态有效期和计时判断的基准。
func (w *worker) tickResidentPetFollow(r *room, now time.Time) bool {
	// pet 保存当前处理所用的宠物。
	pet := r.Pet
	if !w.sourcePetLifecycle() || pet == nil || pet.HP <= 0 || pet.PetRuntime == nil {
		return false
	}
	// m 接收当前状态地图供后续步骤使用，known 保存由当前状态地图取得的已知状态。
	m, known := w.currentMap(r)
	// Crystal ProcessAI1686：Frozen且异图、主人允许时Recall后立即结束本次宠AI。
	// skipPetAI 保存根据“follow”“suppressed”与宠物“resident”场景世界与当前状态场景世界的计算结果的计算结果与已知状态的计算结果与“no”宠物集合与“nil”的计算结果的计算结果与“no”宠物集合计算的“skip”宠物“ai”。
	skipPetAI := pet.PetRuntime.FollowSuppressed && r.petResidentWorld() != r.currentWorld() && known && m.NoPets != nil && !*m.NoPets
	if r.petResidentWorld() != r.currentWorld() || pet.PetRuntime.FollowSuppressed || distance(pet.Pos, r.Player.Pos) > pet.PetRuntime.Definition.SourceDataRangeCells() {
		w.recallResidentPet(r)
	}
	return skipPetAI
}

// killOwnedPet 结算击杀“owned”宠物。
// w 是调用该方法的房间执行器实例，r 指向当前执行器拥有并更新的目标房间，now 提供当前时点，作为状态有效期和计时判断的基准。
func (w *worker) killOwnedPet(r *room, now time.Time) {
	if !w.sourcePetLifecycle() || r.Pet == nil || r.Pet.HP <= 0 {
		return
	}
	// world 保存由宠物“resident”场景世界取得的场景世界，pet 保存当前处理所用的宠物。
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
// w 是调用该方法的房间执行器实例，r 指向当前执行器拥有并更新的目标房间。
func (w *worker) ownedPetPB(r *room) *pb.OwnedPetState {
	// pet 保存当前处理所用的宠物。
	pet := r.Pet
	// world 保存由宠物“resident”场景世界取得的场景世界。
	world := r.petResidentWorld()
	if pet == nil || pet.HP <= 0 || pet.IsPetCorpse || world == nil {
		return nil
	}
	// m 接收场景世界地图供后续步骤使用，known 保存由场景世界地图取得的已知状态。
	m, known := w.worldMap(world)
	if !known {
		return nil
	}
	// out 保存当前处理所用的输出。
	out := &pb.OwnedPetState{EntityHandle: pet.Handle, Life: pet.Life, DefinitionId: pet.Def, MapId: m.ID, Hp: pet.HP, HpMax: pet.MaxHP, PetTraining: petTrainingPB(pet)}
	if pet.PetRuntime != nil {
		out.FollowSuppressed = pet.PetRuntime.FollowSuppressed
		// skill 接收遍历技能集合时当前的元素。
		for _, skill := range w.e.Store.Cfg.Fixture.Skills {
			if skill.Summon == nil {
				continue
			}
			// d 接收定义“by”标识供后续步骤使用，ok 表明查询、校验或类型转换是否成功。
			d, ok := w.e.Store.Cfg.Fixture.PetRuntime.DefinitionByID(skill.Summon.DefinitionID)
			// learned 保存由技能取得的已学习。
			if _, learned := r.Role.Skill(skill.ID); ok && learned && skill.JobID == r.Role.Job && d.RecallName != "" && d.RecallName == pet.PetRuntime.Definition.RecallName {
				out.RecallSkillIds = append(out.RecallSkillIds, skill.ID)
			}
		}
	}
	return out
}

// 不使用客户端给的材料/名称推断Recall；只读当前正式技能合同。
// w 是调用该方法的房间执行器实例，id 区分当前步骤指向的目标对象。
func (w *worker) isSummonSkill(id uint32) bool {
	// s 接收技能供后续步骤使用，ok 表明查询、校验或类型转换是否成功。
	s, ok := w.e.Store.Cfg.Fixture.Skill(id)
	return ok && s.Behavior == "summon_pet" && s.Summon != nil
}
