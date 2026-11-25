// 本文件校验施法请求，冻结技能数值与目标，安排命中事件并按职业处理自动释放。
package game

import (
	"math"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	p "minimir/internal/protocol"
	"sort"
	"strings"
	"time"
)

// skillHandler 把技能配置行为映射到范围、状态、被动、特殊移动和召唤等执行分支。
// Passive 标记是否处于“passive”状态。
// Healing 标记是否处于治疗状态。
// Area 标记是否处于区域状态。
// UndeadBonus 标记是否处于“undead”加成状态。
// Direction 标记是否处于朝向状态。
// SelfArea 标记是否处于“self”区域状态。
// Status 标记是否处于状态状态。
// Displacement 标记是否处于位移状态。
// Teleport 标记是否处于瞬移状态。
// Summon 标记是否处于召唤状态。
type skillHandler struct {
	Passive, Healing, Area, UndeadBonus, Direction, SelfArea, Status, Displacement, Teleport, Summon bool
	TurnUndead                                                                                       bool // 仅对亡灵怪物使用两段判定，成功后安排延迟处决。
}

// preparedCast 冻结施法目标、费用、效果和原身体身份，供后续安全提交。
type preparedCast struct {
	Scene                                 uint64                // 原附着身份独立于执行actorRef。
	Skill                                 devconfig.Skill       // 通过职业和已学等级门禁后的技能配置。
	Level                                 uint32                // 当前已受理技能的等级。
	Caster, Target                        actorRef              // 施法者与目标的原世界出生身份。
	Origin, Point                         devconfig.Point       // 原施法位置和选定地格，用于判定命中及观察。
	Direction                             uint32                // 接受计划时冻结的施法方向。
	Cost, Amount, DelayMS, BuffDurationMS int64                 // 待扣魔法值、效果量、命中延时与状态持续时间。
	Summon                                *summonPetPlan        // 需要新生或召回宠物时保留的专用计划。
	ExecuteTarget                         bool                  // 圣言术已通过等级和概率判定，可安排原目标的延迟处决。
	PoisonColor                           devconfig.PoisonColor // 由持久扣料同一原药粉实例判定的色别。
}

// skillHandlers 以配置行为名称标识服务端可执行的技能分支。
var skillHandlers = map[string]skillHandler{
	devconfig.TrapHexagonBehavior: {Area: true},
	"fencing":                     {Passive: true}, "spirit_sword": {Passive: true},
	"fireball": {}, "great_fireball": {}, "thunderbolt": {UndeadBonus: true},
	"firebang": {Area: true}, "fire_wall": {Area: true}, "healing": {Healing: true}, "mass_healing": {Healing: true, Area: true},
	"magic_shield": {Status: true}, "soul_shield": {Status: true, Area: true}, "blessed_armour": {Status: true, Area: true},
	"slaying": {Passive: true}, "thrusting": {Passive: true}, "half_moon": {Passive: true}, "flaming_sword": {Passive: true},
	"hell_fire": {Direction: true}, "lightning": {Direction: true}, "ice_storm": {Area: true}, "thunder_storm": {Area: true, SelfArea: true},
	"repulsion": {SelfArea: true, Displacement: true}, "shoulder_dash": {Direction: true, Displacement: true},
	"teleport":       {Teleport: true},
	"summon_pet":     {Summon: true},
	"turn_undead":    {TurnUndead: true},
	"electric_shock": {},
	"revelation":     {Status: true},
	"soul_fire_ball": {},
	"poisoning":      {},
	"hiding":         {Status: true},
	"mass_hiding":    {Status: true, Area: true},
}

// targetKind 根据技能配置决定默认目标类别。
// s 记录技能。
func targetKind(s devconfig.Skill) string {
	if s.TargetKind != "" {
		return s.TargetKind
	}
	if s.Behavior == "fencing" || s.Behavior == "spirit_sword" {
		return "passive"
	}
	if s.Behavior == "healing" {
		return "friendly_entity"
	}
	return "enemy_entity"
}

// eventCapacity 读取每个房间可同时保存的战斗事件预算。
// w 是调用该方法的房间执行器实例。
func (w *worker) eventCapacity() int {
	// n 保存当前处理所用的事件容量。
	n := w.e.Store.Cfg.Fixture.CombatTiming.EventCapacity
	if n <= 0 {
		return 64
	}
	return n
}

// areaTargets 在当前世界选取满足范围和阵营规则的目标。
// w 是调用该方法的房间执行器实例，r 指向当前执行器拥有并更新的目标房间，s 记录技能，grid 记录网格，使用点位表示。
func (w *worker) areaTargets(r *room, s devconfig.Skill, grid devconfig.Point) []*entity {
	return w.areaTargetsInWorld(r, r.currentWorld(), s, grid)
}

// areaTargetsInWorld 在指定世界按实际地格和目标生命确定范围技能命中对象。
// w 是调用该方法的房间执行器实例，r 指向当前执行器拥有并更新的目标房间，world 记录场景世界，使用场景世界状态表示，s 记录技能，grid 记录网格，使用点位表示。
func (w *worker) areaTargetsInWorld(r *room, world *worldState, s devconfig.Skill, grid devconfig.Point) []*entity {
	// friendly 区分来源方的临时友方身体与当前地图的合法敌方。
	friendly := strings.HasPrefix(targetKind(s), "friendly")
	if friendly && r.dungeonCombat != nil {
		if world != r.currentWorld() {
			return nil
		}
		return w.dungeonAreaAllies(r, s, grid, false)
	}
	// all 保存当前处理所用的全部。
	all := world.combatOpponents()
	if world.mirrorCombat == nil || friendly {
		if world.mirrorCombat != nil {
			all = nil // 镜像增益只给本方角色和本方原宠物。
		}
		if world == r.currentWorld() {
			all = append([]*entity{&r.Player}, all...)
		}
		all = append(all[:len(all):len(all)], r.worldPet(world))
	}
	// out 初始化用作输出的实体列表。
	out := []*entity{}
	// e 接收遍历全部时当前的元素。
	for _, e := range all {
		if e == nil || e.HP <= 0 || distance(e.Pos, grid) > s.AreaRadius || !w.worldWalkable(world, e.Pos) {
			continue
		}
		if (friendly && (e.Kind == 1 || e.Kind == 2)) || (!friendly && (e.Kind == 3 || world.mirrorCombat != nil)) {
			out = append(out, e)
		}
	}
	// i 记录当前位置的下标，使用数值表示，j 记录当前位置的下标，使用数值表示。
	sort.Slice(out, func(i, j int) bool { return out[i].Handle < out[j].Handle })
	return out
}

// lineTargets 在当前世界收集直线技能的合法目标。
// w 是调用该方法的房间执行器实例，r 指向当前执行器拥有并更新的目标房间，c 记录待确认施法。
func (w *worker) lineTargets(r *room, c pendingCast) []*entity {
	return w.lineTargetsInWorld(r, r.currentWorld(), c)
}

// lineTargetsInWorld 按冻结方向和有效格数选取原世界直线目标。
// w 是调用该方法的房间执行器实例，r 指向当前执行器拥有并更新的目标房间，world 记录场景世界，使用场景世界状态表示，c 记录待确认施法。
func (w *worker) lineTargetsInWorld(r *room, world *worldState, c pendingCast) []*entity {
	if c.Skill.Behavior == "hell_fire" && c.LineRemaining > 0 {
		// point 保存由步骤点位取得的点位。
		point := stepPoint(c.TargetPoint, c.Direction, 1)
		if !w.worldWalkable(world, point) {
			return nil
		}
		// out 记录输出，使用实体列表表示。
		var out []*entity
		// e 接收遍历怪物集合中的范围时当前的元素。
		for _, e := range world.combatOpponents() {
			if e.HP > 0 && e.Pos == point {
				out = append(out, e)
			}
		}
		// i 记录当前位置的下标，使用数值表示，j 记录当前位置的下标，使用数值表示。
		sort.Slice(out, func(i, j int) bool { return out[i].Handle < out[j].Handle })
		return out
	}
	// out 记录输出，使用实体列表表示。
	var out []*entity
	// i 标记循环或列表中的当前位置。
	for i := 1; i <= c.Skill.LineCells; i++ {
		// point 保存由步骤点位取得的点位。
		point := stepPoint(c.TargetPoint, c.Direction, i)
		if !w.worldWalkable(world, point) {
			if c.Skill.LineStopOnBlocked {
				break
			}
			continue
		}
		// target 保存由敌人发生时间输入场景世界取得的目标。
		if target := w.enemyAtInWorld(world, point); target != nil {
			out = append(out, target)
		}
	}
	return out
}

// cast 从普通技能入口受理目标或地格请求。
// w 是调用该方法的房间执行器实例，r 指向当前执行器拥有并更新的目标房间，id 区分当前步骤指向的目标对象，targetHandle 记录目标实体句柄，使用数值表示。
// input 记录输入，使用数值表示，now 提供当前时点，作为状态有效期和计时判断的基准，ground 记录地面，使用可变数量的网格点位表示。
func (w *worker) cast(r *room, id, targetHandle, input uint32, now time.Time, ground ...*pb.GridPoint) uint32 {
	return w.castDirected(r, id, targetHandle, input, r.Player.Direction, now, ground...)
}

// castDirected 先准备本次施法，再在同一房间检查并提交动作。
// w 是调用该方法的房间执行器实例，r 指向当前执行器拥有并更新的目标房间，id 区分当前步骤指向的目标对象，targetHandle 记录目标实体句柄，使用数值表示。
// input 记录输入，使用数值表示，direction 记录朝向，使用数值表示，now 提供当前时点，作为状态有效期和计时判断的基准。
// ground 记录地面，使用可变数量的网格点位表示。
func (w *worker) castDirected(r *room, id, targetHandle, input, direction uint32, now time.Time, ground ...*pb.GridPoint) uint32 {
	// plan 保存由准备施法取得的计划，code 保存由准备施法取得的结果码。
	plan, code := w.prepareCast(r, id, targetHandle, direction, now, false, ground...)
	if code != 0 {
		return code
	}
	return w.acceptPreparedCast(r, plan, input, now)
}

// prepareCast 校验一般施法并生成无副作用的执行计划。
// w 是调用该方法的房间执行器实例，r 指向当前执行器拥有并更新的目标房间，id 区分当前步骤指向的目标对象，targetHandle 记录目标实体句柄，使用数值表示。
// direction 记录朝向，使用数值表示，now 提供当前时点，作为状态有效期和计时判断的基准，materialAllowed 标记是否处于材料允许状态。
// ground 记录地面，使用可变数量的网格点位表示。
func (w *worker) prepareCast(r *room, id, targetHandle, direction uint32, now time.Time, materialAllowed bool, ground ...*pb.GridPoint) (*preparedCast, uint32) {
	return w.prepareCastWithMana(r, id, targetHandle, direction, now, materialAllowed, false, ground...)
}

// prepareCastRecordingMana 为自动用药记录唯一剩余的魔法值门槛。
// w 是调用该方法的房间执行器实例，r 指向当前执行器拥有并更新的目标房间，id 区分当前步骤指向的目标对象，targetHandle 记录目标实体句柄，使用数值表示。
// direction 记录朝向，使用数值表示，now 提供当前时点，作为状态有效期和计时判断的基准，materialAllowed 标记是否处于材料允许状态。
// ground 记录地面，使用网格点位表示。
func (w *worker) prepareCastRecordingMana(r *room, id, targetHandle, direction uint32, now time.Time, materialAllowed bool, ground *pb.GridPoint) (*preparedCast, uint32) {
	return w.prepareCastWithMana(r, id, targetHandle, direction, now, materialAllowed, true, ground)
}

// prepareCastWithMana 统一校验资源、场景、目标、动作时钟、配置与事件预算。
// w 是调用该方法的房间执行器实例，r 指向当前执行器拥有并更新的目标房间，id 区分当前步骤指向的目标对象，targetHandle 记录目标实体句柄，使用数值表示。
// direction 记录朝向，使用数值表示，now 提供当前时点，作为状态有效期和计时判断的基准，materialAllowed 标记是否处于材料允许状态。
// recordMana 标记是否处于记录魔法值状态，ground 记录地面，使用可变数量的网格点位表示。
func (w *worker) prepareCastWithMana(r *room, id, targetHandle, direction uint32, now time.Time, materialAllowed, recordMana bool, ground ...*pb.GridPoint) (*preparedCast, uint32) {
	if arena := r.dungeonCombat; arena != nil && !arena.acceptsActions(now) {
		return nil, p.CodeSkillStateBlocked
	}
	if w.mapCellBudget(r) == 0 {
		return nil, p.CodeSkillStateBlocked
	}
	if direction > 7 {
		return nil, p.CodeSkillTarget
	}
	// s 接收技能供后续步骤使用，exists 保存由技能取得的存在。
	s, exists := w.e.Store.Cfg.Fixture.Skill(id)
	// level 保存由技能取得的等级，learned 保存由技能取得的已学习。
	level, learned := r.Role.Skill(id)
	// handler 保存从技能处理器集合取出的处理器，registered 保存从技能处理器集合取出的“registered”。
	handler, registered := skillHandlers[s.Behavior]
	if !exists || !learned || !registered || s.JobID != r.Role.Job || level < s.LearnedLevel || level > s.MaxLevel {
		return nil, p.CodeSkillNotLearned
	}
	// 未接入角色 owner 的永久消耗前，私有挑战只运行本场的可独立结算动作。
	if r.bossBattle != nil && (handler.Teleport || handler.Summon || s.MaterialQuantity > 0 && r.currentWorld().mirrorCombat == nil) ||
		r.worldBossObservation != nil && handler.Teleport {
		return nil, p.CodeSkillStateBlocked
	}
	// 共享地格持续技能还需统一落格、冲突和拥有者归属，候选阶段只受理已具备场次语义的技能。
	if (r.worldBossCombat != nil || r.dungeonCombat != nil) && s.Behavior == "fire_wall" {
		return nil, p.CodeSkillStateBlocked
	}
	if s.Behavior == devconfig.TrapHexagonBehavior && !w.trapHexagonScope(r, r.currentWorld()) {
		return nil, p.CodeSkillStateBlocked
	}
	if s.Behavior == "electric_shock" && !w.electricShockAllowed(r, r.currentWorld()) ||
		s.Behavior == "revelation" && (r.worldBossCombat != nil || r.worldBossObservation != nil || r.dungeonCombat != nil) {
		return nil, p.CodeSkillStateBlocked
	}
	if handler.Passive || r.Pending || r.ScheduleID == math.MaxUint64 || r.castSequenceFull() || r.Player.HP <= 0 || now.Before(r.Player.NextMove) || now.Before(r.Player.ActionReadyAt) || r.eventCount() >= w.eventCapacity() || !w.killBudgetAvailable(r) {
		return nil, p.CodeSkillStateBlocked
	}
	if now.Before(r.NextSpell) || now.Before(r.SkillCooldowns[id]) {
		return nil, p.CodeSkillCooldown
	}
	if handler.Teleport {
		if s.Teleport == nil || !w.trustedTeleportBinding(r) {
			return nil, p.CodeSkillStateBlocked
		}
		if targetHandle != 0 || len(ground) > 0 && ground[0] != nil {
			return nil, p.CodeSkillTarget
		}
	}
	// target 记录目标，使用实体表示。
	var target *entity
	// point 记录点位。
	var point devconfig.Point
	// summon 记录召唤，使用召唤宠物计划表示。
	var summon *summonPetPlan
	if handler.Summon {
		if targetHandle != 0 || len(ground) > 0 && ground[0] != nil {
			return nil, p.CodeSkillTarget
		}
		// code 记录结果码，使用数值表示。
		var code uint32
		summon, code = w.prepareSummonPet(r, s, direction, now)
		if code != 0 {
			return nil, code
		}
		point = r.Player.Pos
	} else if handler.Direction || handler.SelfArea || s.TargetKind == "friendly_self" {
		if targetHandle != 0 || direction > 7 {
			return nil, p.CodeSkillTarget
		}
		if len(ground) > 0 && ground[0] != nil && !w.inBounds(r, devconfig.Point{X: int(ground[0].X), Y: int(ground[0].Y)}) {
			return nil, p.CodeSkillTarget
		}
		point = r.Player.Pos
	} else if handler.Area {
		if targetHandle != 0 || len(ground) == 0 || ground[0] == nil {
			return nil, p.CodeSkillTarget
		}
		point = devconfig.Point{X: int(ground[0].X), Y: int(ground[0].Y)}
		if !w.inBounds(r, point) {
			return nil, p.CodeSkillTarget
		}
	} else {
		target = w.skillTarget(r, s, targetHandle)
		if target == nil || handler.TurnUndead && !turnUndeadTarget(target) {
			return nil, p.CodeSkillTarget
		}
		point = target.Pos
	}
	if distance(r.Player.Pos, point) > s.RangeCells {
		return nil, p.CodeSkillTarget
	}
	if skillRequiresFlight(s) && !w.lineOfSight(r, r.Player.Pos, point) {
		return nil, p.CodeSkillTarget
	}
	if s.Behavior == devconfig.TrapHexagonBehavior && !hasTrapHexagonTarget(r.currentWorld(), point, r.Role.Level) {
		return nil, p.CodeSkillTarget
	}
	// cost 保存由瞬移魔法值消耗取得的消耗，validCost 保存由瞬移魔法值消耗取得的有效消耗。
	cost, validCost := teleportManaCost(&r.Player, s, level)
	if !validCost {
		return nil, p.CodeSkillStateBlocked
	}
	if !recordMana && r.Player.MP < cost {
		return nil, p.CodeSkillResource
	}
	if r.eventCount()+1 > w.eventCapacity() {
		return nil, p.CodeSkillStateBlocked
	}
	if s.MaterialQuantity > 0 && !materialAllowed && (summon == nil || !summon.Recall) {
		return nil, p.CodeSkillMaterialRequired
	}
	// stats 保存由战斗单位取得的统计或属性。
	stats := w.fighter(r, &r.Player)
	if r.Isolated || w.e.Store.Cfg.Fixture.RuleVersion >= 22 && !w.e.Store.Cfg.Fixture.EquipmentMagicArithmeticSafe(stats) {
		return nil, p.CodeSkillStateBlocked
	}
	// definition 记录定义，使用状态效果定义表示。
	var definition devconfig.BuffDefinition
	if handler.Status {
		// exists 标记是否处于存在状态。
		var exists bool
		definition, exists = w.e.Store.Cfg.Fixture.Buff(s.BuffID)
		if !exists {
			return nil, p.CodeSkillNotLearned
		}
	}
	if handler.Displacement && s.Displacement == nil {
		return nil, p.CodeSkillStateBlocked
	}
	// delay 保存根据“delay”基础值毫秒与整数“64”与“projectile”毫秒“per”地图格的计算结果计算的“delay”。
	delay := s.DelayBaseMS + int64(distance(r.Player.Pos, point))*s.ProjectileMSPerCell
	if delay > math.MaxUint32 {
		return nil, p.CodeSkillStateBlocked
	}
	if r.Player.MP < cost {
		return &preparedCast{Cost: cost, Summon: summon}, p.CodeSkillResource
	}
	// amount 保存由整数“64”取得的数量。
	amount := int64(0)
	// duration 保存由整数“64”取得的持续时间。
	duration := int64(0)
	if handler.Teleport || handler.Summon || handler.TurnUndead || s.Behavior == devconfig.TrapHexagonBehavior || s.Behavior == "electric_shock" {
		// 这些行为使用各自的迁移、召唤、处决或围困规则。
	} else if handler.Status {
		switch definition.DurationKind {
		case "mc_source_power":
			// stat 保存由周期取得的属性。
			stat := w.interval(stats.MagicMin, stats.MagicMax)
			// power 保存根据能力值基础值与“draw”计算的能力值。
			power := s.PowerBase + w.draw(s.PowerBonus)
			duration = buffDurationSeconds(s, definition, level, stat, power)
		case "sc_rank_linear", "sc_half_rank_linear":
			duration = buffDurationSeconds(s, definition, level, w.interval(stats.TaoMin, stats.TaoMax), 0)
		case "sc_source_power":
			duration = w.revelationDuration(s, level, stats)
		}
		if duration <= 0 || duration > math.MaxInt32/int64(time.Second/time.Millisecond) {
			return nil, p.CodeSkillStateBlocked
		}
		duration *= int64(time.Second / time.Millisecond)
	} else if handler.Displacement {
		// 野蛮只以0进入源技能公式，不抽职业DC/MC；抗拒怪物分支没有伤害计算。
		if s.Behavior == "shoulder_dash" {
			amount = w.magicAmount(s, level, 0)
		}
	} else if handler.Healing {
		// multiplier 保存治疗属性倍率，用于当前步骤中的倍率。
		multiplier := s.HealingStatMultiplier
		// addLevel 保存新增施法者等级，用于当前步骤中的新增等级。
		addLevel := s.AddCasterLevel
		if s.TargetKind == "" && s.Behavior == "healing" {
			multiplier = 2
			addLevel = true
		}
		amount = w.magicAmount(s, level, w.interval(stats.TaoMin, stats.TaoMax)*multiplier)
		if addLevel {
			amount += int64(r.Role.Level)
		}
	} else if s.Behavior == "soul_fire_ball" || s.Behavior == "poisoning" {
		// 两种道士攻击技能的原始威力来自道术，后续各按弹道与药粉结算。
		amount = w.magicAmount(s, level, w.interval(stats.TaoMin, stats.TaoMax))
	} else {
		amount = w.magicAmount(s, level, w.interval(stats.MagicMin, stats.MagicMax))
	}
	if s.Behavior == "fire_wall" && fireWallLifetime(amount) == 0 {
		return nil, p.CodeSkillStateBlocked
	}
	if handler.UndeadBonus && target != nil && target.Monster != nil && target.Monster.Undead {
		amount = amount * s.UndeadMultiplierPPM / 1000000
	}
	// plan 保存当前处理所用的计划。
	plan := &preparedCast{Scene: r.Scene, Skill: s, Level: level, Caster: reference(r, &r.Player), Origin: r.Player.Pos, Point: point, Cost: cost, Amount: amount, DelayMS: delay, BuffDurationMS: duration, Direction: r.Player.Direction}
	plan.Summon = summon
	if target != nil {
		plan.Target = reference(r, target)
		if handler.TurnUndead {
			plan.ExecuteTarget = w.turnUndeadSucceeds(r.Role.Level, target.Monster.Level, level)
		}
	}
	if handler.Direction || handler.Summon {
		plan.Direction = direction
	} else if !handler.SelfArea && !handler.Summon && s.TargetKind != "friendly_self" {
		plan.Direction = facing(r.Player.Pos, point, r.Player.Direction)
	}
	return plan, 0
}

// acceptPreparedCast 重新核对身份及水位后一次性扣魔法值，设置冷却并安排命中。
// w 是调用该方法的房间执行器实例，r 指向当前执行器拥有并更新的目标房间，plan 记录计划，使用“prepared”施法表示，input 记录输入，使用数值表示。
// now 提供当前时点，作为状态有效期和计时判断的基准。
func (w *worker) acceptPreparedCast(r *room, plan *preparedCast, input uint32, now time.Time) uint32 {
	if plan == nil || r.ScheduleID == math.MaxUint64 || r.castSequenceFull() || plan.Scene != r.Scene || plan.Caster.World != r.currentWorld().Ref || plan.Caster.Life != r.Player.Life || r.Player.HP <= 0 || !w.summonPlanCurrent(r, plan) {
		return p.CodeSkillStateBlocked
	}
	// s 保存当前处理所用的技能，level 保存当前处理所用的等级。
	s, level := plan.Skill, plan.Level
	if s.Behavior == "poisoning" && plan.PoisonColor == devconfig.PoisonNone {
		return p.CodeSkillMaterialRequired
	}
	// id 区分当前步骤指向的目标对象。
	id := s.ID
	// point 保存当前处理所用的点位，amount 保存当前处理所用的数量，delay 保存“delay”毫秒，用于当前步骤中的“delay”，cost 保存当前处理所用的消耗。
	point, amount, delay, cost := plan.Point, plan.Amount, plan.DelayMS, plan.Cost

	// 所有身份和序号门禁先通过，才一次性扣魔法值并更新各行动时钟。
	r.Player.MP -= cost
	r.Player.Action = 5
	r.Player.ActionUntil = now.Add(time.Duration(s.ActionMS) * time.Millisecond)
	r.Player.ActionReadyAt = r.Player.ActionUntil
	r.Player.NextAttack = r.Player.ActionReadyAt
	r.Player.CastSequence++
	r.Player.CastSkillID = id
	r.advanceCastID()
	r.ScheduleID++
	r.Player.Direction = plan.Direction
	r.NextSpell = now.Add(time.Duration(s.GlobalSpellMS) * time.Millisecond)
	if r.SkillCooldowns == nil {
		r.SkillCooldowns = map[uint32]time.Time{}
	}
	r.SkillCooldowns[id] = now.Add(time.Duration(s.CooldownBaseMS-int64(level)*s.CooldownReductionMS) * time.Millisecond)
	// handle 保存当前处理所用的实体句柄，life 保存当前处理所用的生命代次。
	handle, life := plan.Target.Handle, plan.Target.Life
	// 先将同一动作编号向共享观察流登记，再用该编号安排实际命中与宠物新生来源。
	start := &pb.CastAccepted{SceneEpoch: r.Scene, CastId: r.CastID, SkillId: id, CasterHandle: r.Player.Handle, TargetHandle: handle,
		Origin: pointPB(plan.Origin), TargetGrid: pointPB(point), Direction: r.Player.Direction, DelayMs: uint32(delay),
		InputSequence: input, CasterLife: r.Player.Life, TargetLife: life, SkillLevel: level}
	w.sendCombatObservation(r, p.MsgS2CCastAccepted, start)
	// event 初始化用作事件的待确认施法。
	event := pendingCast{Scene: r.Scene, Skill: s, Target: handle, TargetLife: life, TargetPoint: point, Due: now.Add(time.Duration(delay) * time.Millisecond), Amount: amount, Caster: plan.Caster, ID: start.CastId, Order: r.ScheduleID, Direction: r.Player.Direction, PrivilegeUntil: r.Role.PrivilegeUntil, PrivilegeCaptured: true, SkillLevel: level, BuffDurationMS: plan.BuffDurationMS}
	event.Summon = plan.Summon
	event.PoisonColor = plan.PoisonColor
	if s.Behavior == "hell_fire" {
		event.LineRemaining = s.LineCells
	}
	if r.Conn == nil {
		event.HostSession = r.HostSession
	}
	// handler 决定当前受理动作立即处理、延迟推进或只发送圣言未命中。
	handler := skillHandlers[s.Behavior]
	if !handler.Displacement && (!handler.TurnUndead || plan.ExecuteTarget) {
		r.Casts = append(r.Casts, event)
	}
	if handler.Displacement {
		w.resolveDisplacementSkill(r, event, now)
		return 0
	}
	if handler.TurnUndead && !plan.ExecuteTarget {
		w.impact(r, id, r.Player.Handle, handle, life, point, 0, 2, false, event.ID, 0)
	}
	w.snapshot(r)
	return 0
}

// resolveCast 根据施法者引用找到原执行世界。
// w 是调用该方法的房间执行器实例，r 指向当前执行器拥有并更新的目标房间，c 记录待确认施法，now 提供当前时点，作为状态有效期和计时判断的基准。
func (w *worker) resolveCast(r *room, c pendingCast, now time.Time) {
	w.resolveCastInWorld(r, r.world(c.Caster.World), c, now)
}

// resolveCastInWorld 执行到期技能效果，观察仍按原世界和当前附着过滤。
// w 是调用该方法的房间执行器实例，r 指向当前执行器拥有并更新的目标房间，world 记录场景世界，使用场景世界状态表示，c 记录待确认施法。
// now 提供当前时点，作为状态有效期和计时判断的基准。
func (w *worker) resolveCastInWorld(r *room, world *worldState, c pendingCast, now time.Time) {
	if world == nil || r.world(c.Caster.World) != world {
		return
	}
	// known 保存由场景世界地图取得的已知状态。
	if _, known := w.worldMap(world); !known {
		return
	}
	// handler 保存从技能处理器集合取出的处理器，registered 保存从技能处理器集合取出的“registered”。
	handler, registered := skillHandlers[c.Skill.Behavior]
	// caster 保存由“resolve”输入场景世界取得的施法者。
	caster := w.resolveInWorld(r, world, c.Caster)
	if c.Skill.Behavior == "hell_fire" {
		caster = w.resolveCredit(r, c.Caster)
	}
	if c.Skill.Behavior == "fire_wall" {
		w.resolveFireWall(r, world, caster, c, now)
		return
	}
	if c.Skill.Behavior == "poisoning" {
		w.resolvePoisoning(r, world, caster, c, now)
		return
	}
	if c.Skill.Behavior == devconfig.TrapHexagonBehavior {
		w.resolveTrapHexagon(r, world, caster, c, now)
		return
	}
	if !registered || caster == nil {
		w.impactInWorld(r, world, c.Scene, c.Skill.ID, c.Caster.Handle, c.Target, c.TargetLife, c.TargetPoint, 0, 3, handler.Healing, c.ID, 0)
		return
	}
	if handler.TurnUndead {
		w.resolveTurnUndead(r, world, caster, c, now)
		return
	}
	if c.Skill.Behavior == "electric_shock" || c.Skill.Behavior == "revelation" {
		w.resolveMonsterUtility(r, world, caster, c, now)
		return
	}
	// targets 初始化用作目标集合的实体列表。
	targets := []*entity{}
	if handler.Status {
		if world == r.currentWorld() {
			targets = w.buffTargets(r, c)
		}
	} else if handler.Direction {
		targets = w.lineTargetsInWorld(r, world, c)
		if c.Skill.Behavior == "hell_fire" {
			// point 保存由步骤点位取得的点位。
			point := stepPoint(c.TargetPoint, c.Direction, 1)
			if !w.worldWalkable(world, point) {
				return
			}
			if c.LineRemaining > 1 && r.ScheduleID == math.MaxUint64 {
				// 已付费地图动作不能回绕为0次序；隔离保留其它原事件，不制造无序后继。
				w.isolateRoom(r)
				return
			}
			if r.observesWorld(world, c.Scene) {
				w.sendCombatObservation(r, p.MsgS2CCastStep, &pb.CastStep{SceneEpoch: r.Scene, CastId: c.ID, SkillId: c.Skill.ID, CasterHandle: c.Caster.Handle, CasterLife: c.Caster.Life, Grid: pointPB(point), Direction: c.Direction, StepIndex: uint32(c.Skill.LineCells - c.LineRemaining + 1), SkillLevel: c.SkillLevel})
			}
			if c.LineRemaining > 1 {
				// Crystal Map.CompleteMagic按本次实际处理时间创建下一格，不追赶卡帧。
				// child 保存当前处理所用的子进程。
				child := c
				child.TargetPoint, child.LineRemaining = point, c.LineRemaining-1
				child.Due = now.Add(time.Duration(c.Skill.LineStepMS) * time.Millisecond)
				r.ScheduleID++
				child.Order = r.ScheduleID
				child.PrivilegeUntil, child.PrivilegeCaptured = r.Role.PrivilegeUntil, true
				child.HostSession = ""
				if r.Conn == nil {
					child.HostSession = r.HostSession
				}
				world.Casts = append(world.Casts, child)
			}
		}
	} else if handler.Area {
		targets = w.areaTargetsInWorld(r, world, c.Skill, c.TargetPoint)
	} else {
		// target 保存由技能目标输入场景世界取得的目标。
		target := w.skillTargetInWorld(r, world, c.Skill, c.Target)
		if target != nil && target.Life == c.TargetLife && (!handler.Healing && distance(target.Pos, c.TargetPoint) <= c.Skill.TargetMoveToleranceCells || handler.Healing) {
			targets = append(targets, target)
		}
	}
	if len(targets) == 0 {
		// outcome 保存由无符号整数“32”取得的“outcome”。
		outcome := uint32(3)
		if handler.Area || handler.Direction {
			outcome = 2
		}
		// point 保存目标点位，用于当前步骤中的点位。
		point := c.TargetPoint
		if c.Skill.Behavior == "hell_fire" {
			point = stepPoint(point, c.Direction, 1)
		}
		w.impactInWorld(r, world, c.Scene, c.Skill.ID, c.Caster.Handle, c.Target, c.TargetLife, point, 0, outcome, handler.Healing, c.ID, 0)
		return
	}
	// buffChanged 记录群体状态是否真正落地，整次施法完成后统一发布原身体状态。
	buffChanged := false
	// target 接收遍历目标集合时当前的元素。
	for _, target := range targets {
		if handler.Status {
			buffChanged = w.applyBuff(r, c, target, now) || buffChanged
			continue
		}
		// amount 保存由整数“64”取得的数量。
		amount := int64(0)
		// outcome 保存由无符号整数“32”取得的“outcome”。
		outcome := uint32(2)
		if handler.Healing {
			if target.HP < target.MaxHP {
				// before 保存治疗数量，用于当前步骤中的操作前的状态。
				before := target.HealAmount
				target.HealAmount = queueAdd(target.HealAmount, c.Amount, w.e.Store.Cfg.Fixture.Healing.QueueCap)
				amount = target.HealAmount - before
				if amount > 0 {
					outcome = 4
				}
			}
		} else {
			// value 保存数量，用于当前步骤中的值。
			value := c.Amount
			if c.Skill.Behavior == "thunder_storm" && (target.Monster != nil && !target.Monster.Undead || world.mirrorCombat != nil) {
				value /= c.Skill.NonUndeadDivisor
			}
			// damage 保存由魔法伤害取得的伤害。
			damage := w.magicDamage(r, target, value)
			// before 保存生命值，用于当前步骤中的操作前的状态。
			before := target.HP
			w.applyDamageInWorld(r, world, caster, target, damage, now, hitContext{At: c.Due, HostSession: c.HostSession, PrivilegeUntil: c.PrivilegeUntil, PrivilegeCaptured: c.PrivilegeCaptured})
			amount = before - target.HP
			if amount > 0 {
				outcome = 1
			}
		}
		w.impactInWorld(r, world, c.Scene, c.Skill.ID, c.Caster.Handle, target.Handle, target.Life, target.Pos, amount, outcome, handler.Healing, c.ID, 0)
	}
	if buffChanged && r.dungeonCombat != nil {
		w.snapshotDungeonSeats(r.dungeonCombat, now)
	}
}
