// 本文件维护战士攻击模式、烈火蓄力和过期时钟，并在普通攻击入口选择技能和目标。
package game

import (
	"math"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	p "minimir/internal/protocol"
	"time"
)

// meleeState 保存近战状态所需的状态。
type meleeState struct {
	// Thrusting 标记是否处于“thrusting”状态。
	// HalfMoon 标记是否处于一半“moon”状态。
	// Slaying 标记是否处于“slaying”状态。
	// Flaming 标记是否处于“flaming”状态。
	Thrusting, HalfMoon, Slaying, Flaming bool
	// FlamingUntil 记录“flaming”有效截止时间，使用时间点表示。
	FlamingUntil time.Time
	// Revision 记录修订号，使用数值表示。
	Revision uint64
}

// physicalDefence 描述物理“defence”。
type physicalDefence uint8

const (
	// defenceACAgility 保存当前处理所用的防御“ac”“agility”。
	defenceACAgility physicalDefence = iota
	// defenceAC 记录防御“ac”。
	defenceAC
	// defenceAgility 记录防御“agility”。
	defenceAgility
)

// remainingMS 返回剩余额度毫秒供调用方使用。
// until 记录有效截止时间，使用时间点表示，now 提供当前时点，作为状态有效期和计时判断的基准。
func remainingMS(until, now time.Time) uint32 {
	if !until.After(now) {
		return 0
	}
	// ms 保存根据子项与毫秒的计算结果与当前数据的计算结果与毫秒计算的毫秒。
	ms := (until.Sub(now) + time.Millisecond - 1) / time.Millisecond
	if ms > math.MaxUint32 {
		return math.MaxUint32
	}
	return uint32(ms)
}

// learnedBehavior 处理已学习行为。
// w 是调用该方法的房间执行器实例，r 指向当前执行器拥有并更新的目标房间，behavior 记录行为，使用文本表示。
func (w *worker) learnedBehavior(r *room, behavior string) (devconfig.Skill, uint32, bool) {
	// s 接收遍历技能集合时当前的元素。
	for _, s := range w.e.Store.Cfg.Fixture.Skills {
		// rank 保存由技能取得的排序位次，learned 保存由技能取得的已学习。
		rank, learned := r.Role.Skill(s.ID)
		if s.Behavior == behavior && s.JobID == r.Role.Job && learned && rank >= s.LearnedLevel && rank <= s.MaxLevel {
			return s, rank, true
		}
	}
	return devconfig.Skill{}, 0, false
}

// modeState 返回模式状态供调用方使用。
// w 是调用该方法的房间执行器实例，r 指向当前执行器拥有并更新的目标房间，now 提供当前时点，作为状态有效期和计时判断的基准。
func (w *worker) modeState(r *room, now time.Time) *pb.SkillModeState {
	// out 保存当前处理所用的输出。
	out := &pb.SkillModeState{SceneEpoch: r.Scene, Revision: r.Melee.Revision, PlayerLife: r.Player.Life, ServerTimeUnixMs: uint64(now.UnixMilli())}
	// s 接收遍历技能集合时当前的元素。
	for _, s := range w.e.Store.Cfg.Fixture.Skills {
		if s.Activation() != "toggle" && s.Activation() != "charge" && s.Activation() != "passive_proc" {
			continue
		}
		// learned 保存由已学习行为取得的已学习。
		_, _, learned := w.learnedBehavior(r, s.Behavior)
		// entry 保存当前处理所用的入场。
		entry := &pb.SkillModeEntry{SkillId: s.ID}
		if learned {
			switch s.Behavior {
			case "thrusting":
				entry.Enabled = r.Melee.Thrusting
			case "half_moon":
				entry.Enabled = r.Melee.HalfMoon
			case "slaying":
				entry.Enabled, entry.Ready = true, r.Melee.Slaying
			case "flaming_sword":
				entry.Enabled = true
				entry.Ready = r.Melee.Flaming && now.Before(r.Melee.FlamingUntil)
				entry.CooldownRemainingMs = remainingMS(r.Melee.FlamingUntil, now)
				if entry.Ready {
					entry.RemainingMs = entry.CooldownRemainingMs
				}
			}
		}
		out.States = append(out.States, entry)
	}
	return out
}

// modes 处理模式集合。
// w 是调用该方法的房间执行器实例，r 指向当前执行器拥有并更新的目标房间，now 提供当前时点，作为状态有效期和计时判断的基准，request 记录请求，使用可变数量的数值表示。
func (w *worker) modes(r *room, now time.Time, request ...uint32) {
	// id 区分当前步骤指向的目标对象。
	var id uint32
	if len(request) != 0 {
		id = request[0]
	}
	w.send(r, p.MsgS2CSkillModeState, id, w.modeState(r, now))
}

// expireModes 清理过期的模式集合。
// w 是调用该方法的房间执行器实例，r 指向当前执行器拥有并更新的目标房间，now 提供当前时点，作为状态有效期和计时判断的基准。
func (w *worker) expireModes(r *room, now time.Time) {
	if r.Melee.Flaming && !now.Before(r.Melee.FlamingUntil) {
		r.Melee.Flaming = false
		r.Melee.Revision++
		w.modes(r, now)
	}
}

// setSkillMode 设置技能模式。
// w 是调用该方法的房间执行器实例，r 指向当前执行器拥有并更新的目标房间，id 区分当前步骤指向的目标对象，enabled 标记是否处于已开启状态。
// now 提供当前时点，作为状态有效期和计时判断的基准，request 记录请求，使用可变数量的数值表示。
func (w *worker) setSkillMode(r *room, id uint32, enabled bool, now time.Time, request ...uint32) uint32 {
	// s 接收技能供后续步骤使用，exists 保存由技能取得的存在。
	s, exists := w.e.Store.Cfg.Fixture.Skill(id)
	// rank 保存由技能取得的排序位次，learned 保存由技能取得的已学习。
	rank, learned := r.Role.Skill(id)
	if !exists || !learned || s.JobID != r.Role.Job || rank < s.LearnedLevel || rank > s.MaxLevel {
		return p.CodeSkillNotLearned
	}
	if r.Isolated || r.Player.HP <= 0 || w.mapCellBudget(r) == 0 || r.Melee.Revision == math.MaxUint64 {
		return p.CodeSkillStateBlocked
	}
	switch s.Behavior {
	case "thrusting":
		if r.Melee.Thrusting == enabled {
			w.modes(r, now, request...)
			return 0
		}
		r.Melee.Thrusting = enabled
	case "half_moon":
		if r.Melee.HalfMoon == enabled {
			w.modes(r, now, request...)
			return 0
		}
		r.Melee.HalfMoon = enabled
	case "flaming_sword":
		// 刺杀和半月只改临时开关；烈火还要扣MP并生成可用窗口，继续等待永久事务。
		if !enabled || r.Pending {
			return p.CodeSkillStateBlocked
		}
		if now.Before(r.Melee.FlamingUntil) {
			return p.CodeSkillCooldown
		}
		// cost 保存根据魔法值基础值消耗与整数“64”与魔法值等级消耗的计算结果计算的消耗。
		cost := s.MPBaseCost + int64(rank)*s.MPLevelCost
		// Crystal SpellToggle：充能保留至少1点MP，与普通施法/半月的>=门槛不同。
		if r.Player.MP <= cost {
			return p.CodeSkillResource
		}
		r.Player.MP -= cost
		r.Melee.Flaming = true
		r.Melee.FlamingUntil = now.Add(time.Duration(s.ChargeWindowMS) * time.Millisecond)
	default:
		return p.CodeSkillStateBlocked
	}
	r.Melee.Revision++
	w.modes(r, now, request...)
	w.snapshot(r)
	return 0
}

// stepPoint 返回步骤点位供调用方使用。
// origin 记录大区或起点，使用点位表示，direction 记录朝向，使用数值表示，distance 记录距离，使用数值表示。
func stepPoint(origin devconfig.Point, direction uint32, distance int) devconfig.Point {
	// d 保存从朝向集合取出的朝向集合中的指定项。
	d := directions[direction%uint32(len(directions))]
	return devconfig.Point{X: origin.X + d.X*distance, Y: origin.Y + d.Y*distance}
}

// enemyAt 只从当前世界的活跃敌方中按稳定句柄取首个目标。
// w 是调用该方法的房间执行器实例，r 指向当前执行器拥有并更新的目标房间，point 记录点位。
func (w *worker) enemyAt(r *room, point devconfig.Point) *entity {
	return w.enemyAtInWorld(r.currentWorld(), point)
}

// enemyAtInWorld 返回敌人发生时间输入场景世界供调用方使用。
// w 是调用该方法的房间执行器实例，world 记录场景世界，使用场景世界状态表示，point 记录点位。
func (w *worker) enemyAtInWorld(world *worldState, point devconfig.Point) *entity {
	if !w.worldWalkable(world, point) {
		return nil
	}
	// found 记录已找到标记，使用实体表示。
	var found *entity
	// e 接收遍历怪物集合中的范围时当前的元素。
	for _, e := range world.combatOpponents() {
		if e.HP > 0 && e.Pos == point && (found == nil || e.Handle < found.Handle) {
			found = e
		}
	}
	return found
}

// secondaryTargets 返回次优先目标集合供调用方使用。
// w 是调用该方法的房间执行器实例，r 指向当前执行器拥有并更新的目标房间，s 记录技能，direction 记录朝向，使用数值表示。
func (w *worker) secondaryTargets(r *room, s devconfig.Skill, direction uint32) []*entity {
	// targets 记录目标集合，使用实体列表表示。
	var targets []*entity
	// offset 接收遍历次优先朝向偏移集合时当前的元素。
	for _, offset := range s.SecondaryDirectionOffsets {
		// d 接收无符号整数“32”供后续步骤使用。
		d := uint32((int32(direction) + offset + int32(len(directions))) % int32(len(directions)))
		// e 接收敌人发生时间供后续步骤使用。
		if e := w.enemyAt(r, stepPoint(r.Player.Pos, d, s.SecondaryDistanceCells)); e != nil {
			targets = append(targets, e)
		}
	}
	return targets
}

// Crystal客户端的本子集覆盖顺序：烈火、半月、刺杀、攻杀，模式开关互不覆盖。
// w 是调用该方法的房间执行器实例，r 指向当前执行器拥有并更新的目标房间，direction 记录朝向，使用数值表示，now 提供当前时点，作为状态有效期和计时判断的基准。
// focus 记录“focus”，使用实体表示。
func (w *worker) selectedAttack(r *room, direction uint32, now time.Time, focus *entity) (devconfig.Skill, uint32, bool) {
	// front 保存由敌人发生时间取得的前端。
	front := w.enemyAt(r, stepPoint(r.Player.Pos, direction, 1))
	if focus != nil && focus != front {
		// 定点锁位于第二格时，只能选择确实会穿刺该怪的模式。
		// s 接收已学习行为供后续步骤使用，rank 保存由已学习行为取得的排序位次。
		// learned 保存由已学习行为取得的已学习。
		if s, rank, learned := w.learnedBehavior(r, "thrusting"); learned && r.Melee.Thrusting &&
			focus == w.enemyAt(r, stepPoint(r.Player.Pos, direction, s.SecondaryDistanceCells)) {
			return s, rank, true
		}
		return devconfig.Skill{}, 0, false
	}
	// s 接收已学习行为供后续步骤使用，rank 保存由已学习行为取得的排序位次。
	// learned 保存由已学习行为取得的已学习。
	if s, rank, learned := w.learnedBehavior(r, "flaming_sword"); learned && r.Melee.Flaming && now.Before(r.Melee.FlamingUntil) && front != nil {
		return s, rank, true
	}
	// s 接收已学习行为供后续步骤使用，rank 保存由已学习行为取得的排序位次。
	// learned 保存由已学习行为取得的已学习。
	if s, rank, learned := w.learnedBehavior(r, "half_moon"); learned && r.Melee.HalfMoon && r.Player.MP >= s.MPBaseCost+int64(rank)*s.MPLevelCost && (front != nil || len(w.secondaryTargets(r, s, direction)) > 0) {
		return s, rank, true
	}
	// s 接收已学习行为供后续步骤使用，rank 保存由已学习行为取得的排序位次。
	// learned 保存由已学习行为取得的已学习。
	if s, rank, learned := w.learnedBehavior(r, "thrusting"); learned && r.Melee.Thrusting && len(w.secondaryTargets(r, s, direction)) > 0 {
		return s, rank, true
	}
	// s 接收已学习行为供后续步骤使用，rank 保存由已学习行为取得的排序位次。
	// learned 保存由已学习行为取得的已学习。
	if s, rank, learned := w.learnedBehavior(r, "slaying"); learned && r.Melee.Slaying && front != nil {
		return s, rank, true
	}
	return devconfig.Skill{}, 0, true
}

// attackFacing 返回攻击“facing”供调用方使用。
// w 是调用该方法的房间执行器实例，r 指向当前执行器拥有并更新的目标房间，direction 记录朝向，使用数值表示，expected 记录预期结果，使用数值表示。
// input 记录输入，使用数值表示，now 提供当前时点，作为状态有效期和计时判断的基准。
func (w *worker) attackFacing(r *room, direction, expected, input uint32, now time.Time) uint32 {
	return w.attackFacingTarget(r, direction, expected, input, now, nil)
}

// attackFacingTarget 返回攻击“facing”目标供调用方使用。
// w 是调用该方法的房间执行器实例，r 指向当前执行器拥有并更新的目标房间，direction 记录朝向，使用数值表示，expected 记录预期结果，使用数值表示。
// input 记录输入，使用数值表示，now 提供当前时点，作为状态有效期和计时判断的基准，focus 记录“focus”，使用实体表示。
func (w *worker) attackFacingTarget(r *room, direction, expected, input uint32, now time.Time, focus *entity) uint32 {
	if direction >= uint32(len(directions)) {
		return p.CodeInvalidArgument
	}
	// s 接收已选择攻击供后续步骤使用，rank 保存由已选择攻击取得的排序位次，reachable 保存由已选择攻击取得的“reachable”。
	s, rank, reachable := w.selectedAttack(r, direction, now, focus)
	if !reachable || expected != 0 && expected != s.ID {
		return p.CodeSkillStateBlocked
	}
	return w.attackWithSkill(r, direction, input, now, s, rank)
}

// attackWithSkill 让传统攻击模式和技能槽共用同一个扣蓝、时钟、命中和耐久入口。
func (w *worker) attackWithSkill(r *room, direction, input uint32, now time.Time, s devconfig.Skill, rank uint32) uint32 {
	if arena := r.dungeonCombat; arena != nil && !arena.acceptsActions(now) {
		return p.CodeSkillStateBlocked
	}
	from := &r.Player
	// Human.CanAttack只依赖动作与攻击时钟；保留本作截止时间相等即可受理的边界。
	if r.Pending || r.Isolated || r.ScheduleID == math.MaxUint64 || from.HP <= 0 || now.Before(from.NextAttack) || now.Before(from.ActionReadyAt) || !w.killBudgetAvailable(r) || w.mapCellBudget(r) == 0 {
		return p.CodeSkillStateBlocked
	}
	// front 保存由敌人发生时间取得的前端。
	front := w.enemyAt(r, stepPoint(from.Pos, direction, 1))
	// secondary 保存由次优先目标集合取得的次优先。
	secondary := w.secondaryTargets(r, s, direction)
	// needed 记录次优先中的条目数。
	needed := len(secondary)
	if front != nil {
		needed++
	}
	if r.eventCount()+needed > w.eventCapacity() || uint64(needed) > math.MaxUint64-r.ScheduleID || from.AttackSequence == math.MaxUint32 {
		return p.CodeSkillStateBlocked
	}
	// stats 保存由战斗单位取得的统计或属性。
	stats := w.fighter(r, from)
	// changed 保存当前处理所用的已变化。
	changed := false
	if s.Behavior == "slaying" {
		r.Melee.Slaying, changed = false, true
	}
	// 每次已受理挥刀都可准备下一次攻杀；准备状态保留到真正选中该次攻击。
	// proc 保存由已学习行为取得的“proc”，level 保存由已学习行为取得的等级。
	// learned 保存由已学习行为取得的已学习。
	if proc, level, learned := w.learnedBehavior(r, "slaying"); learned && !r.Melee.Slaying && w.draw(int64(proc.ReadyProcDenominator)) < int64(proc.ReadyProcNumerators[level]) {
		r.Melee.Slaying, changed = true, true
	}
	if s.Behavior == "half_moon" {
		from.MP -= s.MPBaseCost + int64(rank)*s.MPLevelCost
	}
	if s.Behavior == "flaming_sword" && front != nil {
		// 传统蓄力已扣过蓝，槽位未蓄力时原时间窗口和扣蓝与攻击同次确认。
		if !r.Melee.Flaming || !now.Before(r.Melee.FlamingUntil) {
			from.MP -= s.MPBaseCost + int64(rank)*s.MPLevelCost
			r.Melee.FlamingUntil = now.Add(time.Duration(s.ChargeWindowMS) * time.Millisecond)
		}
		r.Melee.Flaming, changed = false, true
	}
	// base 保存由周期取得的基础值。
	base := w.interval(stats.AttackMin, stats.AttackMax)
	// frontAmount 保存当前处理所用的前端数量，secondaryAmount 保存由整数“64”取得的次优先数量。
	frontAmount, secondaryAmount := base, int64(0)
	// frontDefence 保存当前处理所用的前端防御。
	frontDefence := defenceACAgility
	if s.Behavior == "slaying" || s.Behavior == "flaming_sword" {
		frontAmount = w.magicAmount(s, rank, base)
	}
	if s.Behavior == "flaming_sword" {
		frontDefence = defenceAC
	}
	if len(secondary) > 0 {
		secondaryAmount = w.magicAmount(s, rank, base)
	}
	from.AttackSequence++
	w.weaponWear(r, now)
	from.AttackSkillID, from.AttackSkillLevel = s.ID, rank
	from.NextAttack = now.Add(time.Duration(stats.AttackMS) * time.Millisecond)
	from.ActionReadyAt = now.Add(time.Duration(w.e.Store.Cfg.Fixture.CombatTiming.PlayerAttackActionMS) * time.Millisecond)
	from.Action, from.ActionUntil, from.Direction = 2, now.Add(200*time.Millisecond), direction
	// queue 封装队列对应的后续处理，target 记录目标，使用实体表示，amount 记录数量，使用数值表示，defence 记录防御，使用物理防御表示。
	// delay 记录“delay”，使用数值表示。
	queue := func(target *entity, amount int64, defence physicalDefence, delay int64) {
		r.ScheduleID++
		// a 建立带初始值的待确认攻击，供本段继续处理。
		a := pendingAttack{Scene: r.Scene, Caster: reference(r, from), Target: reference(r, target), Due: now.Add(time.Duration(delay) * time.Millisecond), Amount: amount, Defence: defence, SkillID: s.ID, SkillLevel: rank, Sequence: from.AttackSequence, Order: r.ScheduleID, PrivilegeUntil: r.Role.PrivilegeUntil, PrivilegeCaptured: true}
		if r.Conn == nil {
			a.HostSession = r.HostSession
		}
		r.Attacks = append(r.Attacks, a)
	}
	if front != nil {
		queue(front, frontAmount, frontDefence, w.e.Store.Cfg.Fixture.CombatTiming.PlayerAttackImpactMS)
	}
	// target 接收遍历次优先时当前的元素。
	for _, target := range secondary {
		queue(target, secondaryAmount, defenceAgility, s.SecondaryImpactMS)
	}
	// handle 保存由无符号整数“32”取得的实体句柄。
	handle := uint32(0)
	if front != nil {
		handle = front.Handle
	}
	observation := &pb.AttackAccepted{SceneEpoch: r.Scene, EventId: w.nextEvent(r), CasterHandle: from.Handle, TargetHandle: handle, Origin: pointPB(from.Pos), TargetGrid: pointPB(stepPoint(from.Pos, direction, 1)), Direction: direction, DelayMs: uint32(w.e.Store.Cfg.Fixture.CombatTiming.PlayerAttackImpactMS), AttackSequence: from.AttackSequence, InputSequence: input, SkillId: s.ID, SkillLevel: rank, CasterLife: from.Life}
	if arena := r.currentWorld().mirrorCombat; arena != nil {
		arena.addObservation(p.MsgS2CAttackAccepted, observation)
	} else if r.dungeonCombat != nil {
		w.sendDungeonObservation(r, p.MsgS2CAttackAccepted, observation)
	} else {
		w.send(r, p.MsgS2CAttackAccepted, 0, observation)
	}
	if changed {
		r.Melee.Revision++
		w.modes(r, now)
	}
	// 旁格立即命中仍进入统一事件顺序，不能越过同一worker中已经到期的旧事件。
	w.advanceEvents(r, now)
	w.snapshot(r)
	return 0
}
