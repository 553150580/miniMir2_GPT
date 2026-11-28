// 本文件处理房间帧推进、移动与攻击时钟、实际伤害、托管扣时和收益事务交接。
package game

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"math/big"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
	"time"
)

// directions 初始化用作朝向集合的点位列表。
var directions = []devconfig.Point{{X: 0, Y: -1}, {X: 1, Y: -1}, {X: 1, Y: 0}, {X: 1, Y: 1}, {X: 0, Y: 1}, {X: -1, Y: 1}, {X: -1, Y: 0}, {X: -1, Y: -1}}

// random 使用安全随机源在给定上界内取值。
// n 记录数值。
func random(n int64) int64 {
	if n <= 1 {
		return 0
	}
	// x 保存随机源生成的区间内整数，e 表示生成过程是否失败。
	x, e := rand.Int(rand.Reader, big.NewInt(n))
	if e != nil {
		panic(e)
	}
	return x.Int64()
}

// abs 求网格坐标轴距离的非负值。
// x 记录数值。
func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// distance 使用两轴最大差值计算八方向网格距离。
// a 记录a，使用点位表示，b 记录b，使用点位表示。
func distance(a, b devconfig.Point) int {
	// x 接收绝对值供后续步骤使用，y 接收绝对值供后续步骤使用。
	x, y := abs(a.X-b.X), abs(a.Y-b.Y)
	if x > y {
		return x
	}
	return y
}

// free 校验格子地形和动态实体占位，忽略移动者自身。
// w 是调用该方法的房间执行器实例，r 指向当前执行器拥有并更新的目标房间，q 记录点位，self 记录“self”，使用实体表示。
func (w *worker) free(r *room, q devconfig.Point, self *entity) bool {
	if !w.walkable(r, q) || !w.townCellFree(r, self, q) {
		return false
	}
	if arena := r.worldBossCombat; arena != nil {
		return !arena.occupied(q, self)
	}
	if arena := r.dungeonCombat; arena != nil {
		return !arena.occupied(q, self)
	}
	if arena := r.currentWorld().mirrorCombat; arena != nil {
		return !arena.occupied(q, self)
	}
	// all 初始化用作全部的实体列表。
	all := []*entity{&r.Player}
	// pet 保存由场景世界宠物取得的宠物。
	if pet := r.worldPet(r.currentWorld()); pet != nil {
		all = append(all, pet)
	}
	all = append(all, r.Monsters[:r.MonsterCount]...)
	// e 接收遍历全部时当前的元素。
	for _, e := range all {
		if e != self && e.HP > 0 && e.Pos == q {
			return false
		}
	}
	return true
}

// neighbor 校验相邻格和对角移动约束。
// w 是调用该方法的房间执行器实例，r 指向当前执行器拥有并更新的目标房间，from 记录来源，使用点位表示，to 记录目标，使用点位表示，self 记录“self”，使用实体表示。
func (w *worker) neighbor(r *room, from, to devconfig.Point, self *entity) bool {
	if !w.free(r, to, self) {
		return false
	}
	// dx 保存根据横坐标与横坐标计算的横向差值，dy 保存根据纵坐标与纵坐标计算的纵向差值。
	dx, dy := to.X-from.X, to.Y-from.Y
	if dx != 0 && dy != 0 {
		if !w.walkable(r, devconfig.Point{X: from.X + dx, Y: from.Y}) || !w.walkable(r, devconfig.Point{X: from.X, Y: from.Y + dy}) {
			return false
		}
	}
	return true
}

// move 按动作时钟请求城镇规划或个人地图首步，再提交实际移动。
// w 是调用该方法的房间执行器实例，r 指向当前执行器拥有并更新的目标房间，e 记录实体，target 记录目标，使用点位表示，now 提供当前时点，作为状态有效期和计时判断的基准。
// interval 记录周期，使用数值表示。
func (w *worker) move(r *room, e *entity, target devconfig.Point, now time.Time, interval int64) {
	if r.petAway(e) {
		return
	}
	if w.moveTown(r, e, target, now, interval) {
		if e == &r.Player {
			w.finishFailedTownNavigation(r)
		}
		return
	}
	if now.Before(e.NextMove) || now.Before(e.ActionReadyAt) || e.HP <= 0 || e.Pos == target {
		return
	}
	// start 保存位置，用于当前步骤中的起点或开始时间。
	start := e.Pos
	// m 接收当前状态地图供后续步骤使用，ok 表明查询、校验或类型转换是否成功。
	m, ok := w.currentMap(r)
	if !ok || m.CellBudget() > devconfig.MaxPersonalGridCells || !m.Contains(start) || !m.Contains(target) {
		return
	}
	// next 是个人网格路径的首步，found 表示在当前占位和地形下找到了可走路径。
	next, found := w.personalNavigation.findFirstStep(m, r, e, target)
	if !found {
		if e == &r.Player && r.Destination != nil && *r.Destination == target && r.Navigation.Status == navigationActive {
			// 玩家点地的路径没有可继续的格子，保留目标观察并释放移动接管。
			r.Destination = nil
			r.Navigation.Status = navigationBlocked
			w.snapshot(r)
		}
		return
	}
	// direction 保存当前处理所用的朝向。
	direction := e.Direction
	// i 定位朝向集合中正在处理的成员，d 接收遍历朝向集合时当前的元素。
	for i, d := range directions {
		if next.X-start.X == d.X && next.Y-start.Y == d.Y {
			direction = uint32(i)
			break
		}
	}
	w.commitWalk(r, e, next, direction, now, interval)
}

// fighter 汇总当前实体的持久属性和战斗状态，供伤害和时钟计算。
// w 是调用该方法的房间执行器实例，r 指向当前执行器拥有并更新的目标房间，e 记录实体。
func (w *worker) fighter(r *room, e *entity) devconfig.Fighter {
	// f 保存当前处理所用的开发测试配置。
	f := &w.e.Store.Cfg.Fixture
	if e.Kind == 1 {
		if r.worldState != nil && r.worldState.mirrorCombat != nil && e != &r.Player {
			other := r.worldState.mirrorCombat.sides[1-r.worldState.mirrorSide].room
			return statusFighter(e, other.bossBattle.fighter)
		}
		if battle := r.bossBattle; battle != nil {
			return statusFighter(e, battle.fighter)
		}
		return statusFighter(e, w.basePlayerFighter(r))
	}
	if e.Kind == 2 {
		if e.PetRuntime != nil {
			return statusFighter(e, e.PetRuntime.Fighter)
		}
		return statusFighter(e, f.Pet)
	}
	monster := e.Monster.Fighter
	if rules := f.CombatLuck; rules != nil {
		monster.LuckResistancePPM = rules.MonsterResistancePPM[e.Monster.ID]
	}
	return statusFighter(e, monster)
}

// basePlayerFighter 计算可用于挑战入场快照的已确认装备和被动技能属性。
func (w *worker) basePlayerFighter(r *room) devconfig.Fighter {
	f := &w.e.Store.Cfg.Fixture
	// a 保存由玩家发生时间等级取得的a。
	a, _ := f.PlayerAtLevel(r.Role.Job, r.Role.Level)
	var equipment devconfig.EquipmentContributions // 确认的完整装备基础与各部位强化增量，只在最后取整。
	useGearGrowth := f.EquipmentEnhancement != nil || f.EquipmentRefine != nil
	// item 接收遍历已穿戴装备时当前的元素。
	for _, item := range r.Role.Equipped {
		if item.Slot == devconfig.CastingSupplySlot || f.EquipmentDurability != nil && item.Item.OriginalDurabilityU > 0 && item.Item.DurabilityU <= r.Wear.loss(item.Item, item.Slot) {
			continue
		}
		// def 保存由道具取得的“def”。
		def, _ := f.Item(item.Item.ItemID)
		if f.RuleVersion >= 22 {
			if useGearGrowth {
				if !equipment.Add(def, f.EquipmentEnhancement.BonusBP(r.Role.EnhancementLevel(item.Slot))) {
					w.isolateRoom(r)
					return devconfig.Fighter{}
				}
				if f.EquipmentRefine != nil {
					// 已付候选留在右侧。只有左侧已经采纳的槽进入当前战斗。
					for _, affix := range item.Item.Refine.Current {
						if !equipment.AddRefine(affix.AttributeID, affix.ValueUnits) {
							w.isolateRoom(r)
							return devconfig.Fighter{}
						}
					}
				}
				continue
			}
			// valid 标记是否处于有效状态。
			var valid bool
			a, valid = def.AddEquipmentBonuses(a)
			if !valid {
				w.isolateRoom(r)
				return devconfig.Fighter{}
			}
			continue
		}
		a.AttackMin += def.AttackBonus + def.AttackMinBonus
		a.AttackMax += def.AttackBonus + def.AttackMaxBonus
		a.AttackSpeed += def.AttackSpeedBonus
		a.Defence += def.DefenceBonus
		a.DefenceMin += def.DefenceMinBonus
		a.DefenceMax += def.DefenceMaxBonus
		a.MagicDefenceMin += def.MagicDefenceMinBonus
		a.MagicDefenceMax += def.MagicDefenceMaxBonus
	}
	if useGearGrowth {
		var valid bool
		a, valid = equipment.Apply(a)
		if !valid {
			w.isolateRoom(r)
			return devconfig.Fighter{}
		}
	}
	body := r.Role.BodyTrainingLevels()
	a, bodyValid := f.ApplyBodyTraining(a, body.Health, body.Mana)
	if !bodyValid {
		w.isolateRoom(r)
		return devconfig.Fighter{}
	}
	if f.ExpandedHitBudget != nil {
		a.HitBonuses = w.bodyHitBonuses(r)
	}
	a, medalValid := f.ApplyMedalBonuses(a, r.Role.MedalLevel())
	if !medalValid {
		w.isolateRoom(r)
		return devconfig.Fighter{}
	}
	// skill 接收遍历技能集合时当前的元素。
	for _, skill := range f.Skills {
		// level 是角色记录的技能等级，learned 表示技能已习得。
		if level, learned := r.Role.Skill(skill.ID); learned && (skill.Behavior == "fencing" || skill.Behavior == "spirit_sword" || skill.Behavior == "slaying") && skill.JobID == r.Role.Job && level >= skill.LearnedLevel && level <= skill.MaxLevel {
			a.Accuracy += skill.AccuracyBonus(level)
			a.AttackMax += skill.MaximumAttackBonus(level)
		}
	}
	if w.sourceTimeline() {
		if f.RuleVersion >= 22 && !f.EquipmentArithmeticSafe(a) {
			w.isolateRoom(r)
			return devconfig.Fighter{}
		}
		a.AttackMS = f.CombatTiming.PlayerAttackInterval(r.Role.Level, a.AttackSpeed)
	}
	if rules := f.CombatLuck; rules != nil {
		// 在线角色和挑战快照共同读取已经确认的装备及唯一翅膀的原幸运。
		a.LuckAttackPPM = equipmentLuck(r, f, roleWingLuck(r.Role, f))
		a.LuckResistancePPM = rules.RoleResistancePPM[r.Role.Job]
	}
	return a
}

// nearest 在当前有效怪物中找到与施法者或攻击者最近的目标。
// w 是调用该方法的房间执行器实例，r 指向当前执行器拥有并更新的目标房间，from 记录来源，使用实体表示。
func (w *worker) nearest(r *room, from *entity) *entity {
	if r.petAway(from) {
		return nil
	}
	// best 保存当前最接近来源实体的存活怪物。
	var best *entity
	// m 接收遍历怪物集合中的范围时当前的元素。
	for _, m := range r.currentWorld().combatOpponents() {
		if m.HP > 0 && (best == nil || distance(from.Pos, m.Pos) < distance(from.Pos, best.Pos)) {
			best = m
		}
	}
	return best
}

// attack 校验行动、时钟和目标，按规则安排攻击命中或立即应用伤害。
// w 是调用该方法的房间执行器实例，r 指向当前执行器拥有并更新的目标房间，from 记录来源，使用实体表示，target 记录目标，使用实体表示。
// now 提供当前时点，作为状态有效期和计时判断的基准，input 记录输入，使用可变数量的数值表示。
func (w *worker) attack(r *room, from, target *entity, now time.Time, input ...uint32) {
	if w.mapCellBudget(r) == 0 || r.petAway(from) || r.petAway(target) {
		return
	}
	if from.Kind == 1 && w.sourceTimeline() {
		if target == nil || target.HP <= 0 || distance(from.Pos, target.Pos) > 1 {
			return
		}
		// sequence 保存由无符号整数“32”取得的序号。
		sequence := uint32(0)
		if len(input) > 0 {
			sequence = input[0]
		}
		w.attackFacing(r, facing(from.Pos, target.Pos, from.Direction), 0, sequence, now)
		return
	}
	// 攻击不依赖移动节流；保留本作>=截止边界，来源怪物严格>的调度差异不在本次改变。
	if r.Pending || from.HP <= 0 || target.HP <= 0 || now.Before(from.NextAttack) || now.Before(from.ActionReadyAt) || distance(from.Pos, target.Pos) > 1 {
		return
	}
	// dx 保存根据横坐标与横坐标计算的横向差值，dy 保存根据纵坐标与纵坐标计算的纵向差值。
	dx, dy := target.Pos.X-from.Pos.X, target.Pos.Y-from.Pos.Y
	if dx != 0 && dy != 0 && (!w.walkable(r, devconfig.Point{X: from.Pos.X + dx, Y: from.Pos.Y}) || !w.walkable(r, devconfig.Point{X: from.Pos.X, Y: from.Pos.Y + dy})) {
		return
	}
	// a 汇总攻击者战斗属性，b 汇总目标的属性和防御。
	a, b := w.fighter(r, from), w.fighter(r, target)
	// damage 保存由物理伤害取得的伤害。
	damage := w.physicalDamage(r, from, target)
	if w.e.Store.Cfg.Fixture.DamageFormula != "development_source_combat" {
		damage = w.interval(a.AttackMin, a.AttackMax) - b.Defence
		if damage < 1 {
			damage = 1
		}
	}
	from.AttackSequence++
	if from == &r.Player {
		w.weaponWear(r, now)
	}
	from.NextAttack = now.Add(time.Duration(a.AttackMS) * time.Millisecond)
	from.Action = 2
	from.ActionUntil = now.Add(200 * time.Millisecond)
	// i 定位朝向集合中正在处理的成员，d 接收遍历朝向集合时当前的元素。
	for i, d := range directions {
		if (dx == 0 && d.X == 0 || dx > 0 && d.X > 0 || dx < 0 && d.X < 0) && (dy == 0 && d.Y == 0 || dy > 0 && d.Y > 0 || dy < 0 && d.Y < 0) {
			from.Direction = uint32(i)
			break
		}
	}
	// world 保存由当前状态场景世界取得的场景世界，scene 保存当前处理所用的场景。
	world, scene := r.currentWorld(), r.Scene
	// targetLife 固定目标受击前的出生代次，hitPoint 是受击格，before 是扣血前生命值。
	targetLife, hitPoint, before := target.Life, target.Pos, target.HP
	w.applyDamageInWorld(r, world, from, target, damage, now)
	// actual 是目标实际减少的生命值，用于命中表现。
	if actual := before - target.HP; actual > 0 && w.e.Store.Cfg.Fixture.RuleVersion >= 24 && (from.Kind == 2 || from.Kind == 3) {
		// 宠物和怪物仍在本帧结算；致死时即使原实体已归档，事件仍引用受击生命和落点。
		w.impactInWorld(r, world, scene, 0, from.Handle, target.Handle, targetLife, hitPoint, actual, 1, false, 0, from.AttackSequence)
	}
}

// applyDamage 把实际伤害交给当前世界处理。
// w 是调用该方法的房间执行器实例，r 指向当前执行器拥有并更新的目标房间，from 记录来源，使用实体表示，target 记录目标，使用实体表示，damage 记录伤害，使用数值表示。
// now 提供当前时点，作为状态有效期和计时判断的基准，context 记录上下文，使用可变数量的命中上下文表示。
func (w *worker) applyDamage(r *room, from, target *entity, damage int64, now time.Time, context ...hitContext) {
	w.applyDamageInWorld(r, r.currentWorld(), from, target, damage, now, context...)
}

// applyDamageInWorld 更新原世界目标生命，记录死亡、磨损、收益和可见伤害。
// w 是调用该方法的房间执行器实例，r 指向当前执行器拥有并更新的目标房间，world 记录场景世界，使用场景世界状态表示，from 记录来源，使用实体表示。
// target 记录目标，使用实体表示，damage 记录伤害，使用数值表示，now 提供当前时点，作为状态有效期和计时判断的基准。
// context 记录上下文，使用可变数量的命中上下文表示。
func (w *worker) applyDamageInWorld(r *room, world *worldState, from, target *entity, damage int64, now time.Time, context ...hitContext) {
	damage = w.expandedHitDamage(r, from, target, damage, context)
	if world != nil && world.mirrorCombat != nil {
		w.applyMirrorDamage(r, world, from, target, damage, now, context)
		return
	}
	if r.dungeonCombat != nil {
		if damage > 0 && target.HP > 0 && world == r.currentWorld() {
			w.applyDungeonDamage(r, from, target, damage, now, context)
		}
		return
	}
	if from != nil && len(from.Statuses) != 0 && target.HP > 0 && world == r.currentWorld() {
		exposeHiddenAttacker(world, from, target)
	}
	if damage <= 0 || target.HP <= 0 {
		return
	}
	if r.bossBattle != nil {
		if world == r.currentWorld() {
			w.applyPersonalBossDamage(r, from, target, damage, now, context)
		}
		return
	}
	if from != nil && (len(context) == 0 || !context[0].PeriodicPoison) {
		world.clearTrapHexagon(target)
		target.ShockUntilMS = 0
	}
	wearStatuses(target, damage)
	target.HP -= damage
	if target.HP < 0 {
		target.HP = 0
	}
	if target == &r.Player {
		w.receivedWear(r, false, now)
	}
	target.Action = 3
	target.ActionUntil = now.Add(200 * time.Millisecond)
	if target.HP == 0 {
		world.clearTrapHexagon(target)
		removeDeathStatuses(target)
		target.Action = 4
		if target.Kind == 3 {
			if r.AutoTarget == referenceIn(world, target) {
				r.AutoTarget = actorRef{}
			}
			target.RespawnAt = now.Add(normalMonsterRespawnDelay(target.Monster))
			if len(context) > 0 && w.sourceTimeline() {
				// until 是作用于该命中时段的特权有效截止时刻。
				until := r.Role.PrivilegeUntil
				if context[0].PrivilegeCaptured {
					until = context[0].PrivilegeUntil
				}
				w.enqueueKillFactInWorld(r, world, target, context[0].At, from, context[0].HostSession, until)
			} else {
				w.kill(r, target, now, from)
			}
		} else if target.Kind == 2 && target == r.Pet && w.petRuntimeEnabled() {
			if !w.archivePetDeath(r, world, referenceIn(world, target), now) {
				w.isolateRoom(r)
			}
		} else if target.Kind == 1 {
			r.AutoTarget = actorRef{}
			w.killOwnedPet(r, now)
			r.DeathID = domain.ID()
			w.receivedWear(r, true, now)
			w.deathNotice(r)
			// sourceTimeline 表示规则启用统一时间线，死亡相关事实按原命中时间归属。
			sourceTimeline := w.sourceTimeline()
			// at 保存实际命中时刻，hostSession 保存该时刻所属的托管会话。
			at, hostSession := now, ""
			if sourceTimeline && len(context) > 0 {
				// 已到期的致死攻击归原事件区段，迟处理不借当前连接重判。
				at, hostSession = context[0].At, context[0].HostSession
			} else if r.Conn == nil {
				hostSession = r.HostSession
			}
			if hostSession != "" {
				if sourceTimeline {
					r.FactOrder++
					r.KillFacts = append(r.KillFacts, killFact{World: world.Ref, Order: r.FactOrder, At: at, HostSession: hostSession, StopReason: 2})
				} else {
					w.settle(r, persistence.Settlement{SourceSeq: r.Role.SourceSeq + 1, HostingSession: hostSession, StopReason: 2, EventAt: at})
				}
			}
		}
		// 致死HP/动作属于临时现场，立即同步；奖励仍只在资产事务提交后推送。
		r.LastBroadcast = now
		if world == r.currentWorld() {
			w.snapshot(r)
		}
	}
}

// clearFinishedAction 清除活体已过期的动作表现，不改变攻击或移动门禁。
// e 记录实体，now 提供当前时点，作为状态有效期和计时判断的基准。
func clearFinishedAction(e *entity, now time.Time) {
	if e != nil && e.HP > 0 && !now.Before(e.ActionUntil) {
		e.Action = 0
	}
}

// step 推进一个房间的逻辑帧，处理在途状态、已到期事实、恢复、托管和战斗行动。
// w 是调用该方法的房间执行器实例，r 指向当前执行器拥有并更新的目标房间，now 提供当前时点，作为状态有效期和计时判断的基准。
func (w *worker) step(r *room, now time.Time) {
	if w.e.stopped.Load() || r.Isolated {
		return
	}
	if r.Scene == 0 {
		return
	}
	if r.townMembership.owner != 0 && (r.Conn == nil || !r.Conn.Alive()) {
		w.leaveSharedTown(r)
	}
	// 连接控制可能越过排队的旧帧，旧帧不得倒拨托管计时或推进现场。
	if now.Before(r.LastTick) {
		return
	}
	// mapTemplate 保存由当前状态地图取得的地图模板，ok 表明查询、校验或类型转换是否成功。
	mapTemplate, ok := w.currentMap(r)
	if !ok {
		w.isolateRoom(r)
		return
	}
	if r.Conn != nil && !r.Ready {
		if now.After(r.AwaitUntil) {
			w.leaveSharedTown(r)
			r.Conn.Close("baseline acknowledgement timeout")
		}
	}
	if r.LastTick.IsZero() {
		r.LastTick = now
	}
	// delta 保存由子项取得的增量。
	delta := now.Sub(r.LastTick)
	r.LastTick = now
	if delta > 200*time.Millisecond {
		delta = 200 * time.Millisecond
	}
	if r.Pending {
		w.progressMapTravel(r, now)
		w.retryPendingRoom(r, now)
		w.snapshotSharedTownWhenDue(r, now)
		return
	}
	if w.expirePetCorpses(r, now) {
		w.snapshot(r)
	}
	w.expireModes(r, now)
	// A28：本作统一在本帧事件前处理全部实体状态；按实际处理时钟离散扣时，不回造Due历史。
	if w.tickStatuses(r, now) {
		w.snapshot(r)
	}
	w.flushKillFacts(r)
	if r.Pending || len(r.KillFacts) > 0 {
		return
	}
	// 已经释放的法术与恢复队列不属于后续离线战斗授权；未知事务仍在上方冻结。
	// activeBeforeEvents 保存到期事件处理前的活动世界，sceneBeforeEvents 保存同一时点的客户端场景编号。
	activeBeforeEvents, sceneBeforeEvents := r.currentWorld(), r.Scene
	// reactive 收集本帧需处理复活、战利品等期限的世界。
	reactive := w.reactiveWorlds(r)
	w.advanceEvents(r, now)
	// world 接收遍历“reactive”场景世界集合时当前的元素。
	for _, world := range w.reactiveWorlds(r) {
		// found 保存当前处理所用的已找到标记。
		found := false
		// earlier 接收遍历“reactive”时当前的元素。
		for _, earlier := range reactive {
			if earlier == world {
				found = true
			}
		}
		if !found {
			reactive = append(reactive, world)
		}
	}
	// world 接收遍历“reactive”时当前的元素。
	for _, world := range reactive {
		if !r.Pending && !r.Isolated {
			w.worldDeadlines(r, world, now)
		}
	}
	w.flushKillFacts(r)
	if r.Pending || r.Isolated {
		return
	}
	if w.flushWear(r, now, false) {
		return
	}
	if r.currentWorld() != activeBeforeEvents || r.Scene != sceneBeforeEvents {
		return
	}
	if w.tickHealing(r, now) {
		w.snapshot(r)
	}
	// offline 表示房间当前没有附着连接，可按地图、存活和余额判断托管。
	offline := r.Conn == nil
	if r.Player.HP <= 0 {
		if w.flushHostingSeconds(r, now) {
			return
		}
		w.progressMapTravel(r, now)
		w.snapshotSharedTownWhenDue(r, now)
		return
	}
	if w.tickPotions(r, now) {
		// 恢复量已经由权威场景应用，立即同步；不等待下一次战斗或奖励事务。
		w.snapshot(r)
	}
	if r.Player.HP >= r.Player.MaxHP {
		// Crystal ProcessRegen统一结算满值后清除所有HP恢复余额，不能供同帧受击后复用。
		r.Player.HealAmount = 0
		r.Potions.HP = 0
	}
	if w.progressMapTravel(r, now) {
		return
	}
	if mapTemplate.SceneKindValue() == devconfig.SceneKindMining {
		w.stepMining(r, now)
		return
	}
	if mapTemplate.SceneKindValue() == devconfig.SceneKindTown {
		// 来源城镇不运行离线战斗或扣托管；既有状态/恢复已在上方按真实时钟推进。
		if offline {
			w.flushHostingSeconds(r, now)
			return
		}
	}
	if offline && (r.Role.HostingSeconds <= 0 || !r.Auto) {
		w.flushHostingSeconds(r, now)
		return
	}
	if offline && w.meterHosting(r, delta, now) {
		return
	}
	w.stepGridCombat(r, mapTemplate, now, offline)
	w.flushKillFacts(r)
}

// stepGridCombat 用同一套地格、移动、攻击及宠物行为推进一个活动场景。
func (w *worker) stepGridCombat(r *room, mapTemplate *devconfig.MapTemplate, now time.Time, offline bool) {
	if r.Player.HP <= 0 {
		return
	}
	clearFinishedAction(&r.Player, now)
	// monster 接收遍历怪物集合时当前的元素。
	if r.worldBossCombat == nil && r.dungeonCombat == nil {
		for _, monster := range r.Monsters {
			clearFinishedAction(monster, now)
		}
	}
	clearFinishedAction(r.Pet, now)
	if r.bossBattle == nil {
		w.worldDeadlines(r, r.currentWorld(), now)
	}
	if r.Isolated {
		return
	}
	// 已确认过的挑战在健康换票加载期间可继续原自动规则。在线输入仍须匹配当前地图 ACK。
	if r.Conn != nil && !r.Ready && !offline || r.Travel != nil && r.Travel.Phase == travelCommitted && !r.Travel.Acked {
		return
	}
	// skipPetAI 表示驻留宠物的跟随动作占用了本帧宠物自动行动机会。
	skipPetAI := w.tickResidentPetFollow(r, now)
	// a 保存战斗单位返回的结果。
	a := w.fighter(r, &r.Player)
	// 掉线期间暂停玩家的手动换点，健康重连后仍可继续原地面目标。
	if !offline && r.Manual {
		// d 保存从朝向集合取出的朝向集合中的指定项。
		d := directions[r.ManualDirection]
		// target 初始化用作目标的点位。
		target := devconfig.Point{X: r.Player.Pos.X + d.X, Y: r.Player.Pos.Y + d.Y}
		if w.neighbor(r, r.Player.Pos, target, &r.Player) && !now.Before(r.Player.NextMove) && !now.Before(r.Player.ActionReadyAt) {
			w.commitWalk(r, &r.Player, target, r.ManualDirection, now, a.MoveMS)
		}
	} else if !offline && r.Destination != nil {
		if r.Player.Pos == *r.Destination {
			if r.Navigation.InputSequence != 0 && r.Navigation.Status == navigationActive {
				r.Navigation.Status = navigationArrived
				w.snapshot(r)
			}
			r.Destination = nil
		} else {
			w.move(r, &r.Player, *r.Destination, now, a.MoveMS)
		}
	} else if r.Auto && !w.autoSkill(r, now) {
		w.autoAttack(r, now)
	}
	if r.Pending || r.Isolated {
		return
	}
	// pet 保存由场景世界宠物取得的宠物。
	pet := r.worldPet(r.currentWorld())
	if !skipPetAI && pet != nil && pet.HP > 0 && (pet.PetRuntime == nil || !pet.PetRuntime.FollowSuppressed) {
		if (r.Auto || r.bossBattle != nil) && mapTemplate.SceneKindValue() != devconfig.SceneKindTown {
			// target 保存由最近取得的目标。
			if target := w.nearest(r, pet); target != nil {
				if !w.tryPetAttack(r, pet, target, now) {
					w.move(r, pet, target.Pos, now, w.fighter(r, pet).MoveMS)
				}
			}
		} else if distance(pet.Pos, r.Player.Pos) > 2 {
			w.move(r, pet, r.Player.Pos, now, w.fighter(r, pet).MoveMS)
		}
	}
	if r.Pending || r.Isolated {
		return
	}
	if r.worldBossCombat != nil || r.dungeonCombat != nil {
		if now.Sub(r.LastBroadcast) >= 200*time.Millisecond {
			r.LastBroadcast = now
			w.snapshot(r)
		}
		return // 共享场次随后统一运行敌人的移动和攻击 AI。
	}
	// world 固定本帧怪物索敌和隐身暴露所使用的原执行地图。
	world := r.currentWorld()
	// m 接收遍历怪物集合中的范围时当前的元素。
	for _, m := range r.Monsters[:r.MonsterCount] {
		if m.HP <= 0 || m.ShockUntilMS != 0 && now.UnixMilli() < m.ShockUntilMS || world.trapHexagonHeld(m, now) {
			continue
		}
		// target 是本轮能被原怪物察觉的玩家或宠物；没有目标时留在当前格。
		target := visibleMonsterTarget(world, m, &r.Player, pet, r.Role.Level)
		if target == nil {
			continue
		}
		if distance(m.Pos, target.Pos) <= 1 {
			w.attack(r, m, target, now)
		} else {
			w.move(r, m, target.Pos, now, m.Monster.MoveMS)
		}
		if r.Pending || r.Isolated {
			break
		}
	}
	if now.Sub(r.LastBroadcast) >= 200*time.Millisecond {
		r.LastBroadcast = now
		w.snapshot(r)
	}
}

// legacyKill 处理未启用统一时间线规则时的怪物死亡与收益。
// w 是调用该方法的房间执行器实例，r 指向当前执行器拥有并更新的目标房间，m 记录实体，now 提供当前时点，作为状态有效期和计时判断的基准。
func (w *worker) legacyKill(r *room, m *entity, now time.Time) {
	// f 保存当前处理所用的开发测试配置。
	f := &w.e.Store.Cfg.Fixture
	// bound 表示击杀时没有有效特权，随后按绑定资产发放。
	bound := !r.Role.PrivilegeUntil.After(now)
	// q 组装结算所需的数据。
	q := persistence.Settlement{SourceSeq: r.Role.SourceSeq + 1, Kills: 1, MonsterID: m.Monster.ID, MapID: r.currentWorld().MapID,
		Experience: m.Monster.Exp, Bound: bound, NoEquipmentKills: r.Role.NoEquipmentKills + 1, EventAt: now}
	// roll 接收随机值供后续步骤使用。
	roll := random(1000000)
	// equipment 表示已达到保底次数或旧规则掷点落在装备掉落区间。
	equipment := q.NoEquipmentKills >= 20 || roll >= 950000
	if equipment {
		// ids 是可供旧规则抽取的装备定义编号集合。
		ids := f.Drops.EquipmentItemIDs
		// item 组装道具所需的数据。
		item := domain.Item{ID: domain.ID(), ItemID: ids[random(int64(len(ids)))], Quantity: 1, Bound: bound, Version: 1}
		item.SourceKind, item.SourceID = "monster_drop", fmt.Sprint(q.SourceSeq)
		// definition 提供配置中的物品定义，用来读取生成掉落时的品质。
		definition, _ := f.Item(item.ItemID)
		item.Quality = definition.DropQuality
		q.Item = &item
		q.NoEquipmentKills = 0
	} else if roll >= 650000 && roll < 850000 {
		q.Gold = m.Monster.GoldMin + random(m.Monster.GoldMax-m.Monster.GoldMin+1)
	} else if roll >= 850000 && roll < 950000 && len(f.Drops.ItemIDs) > 0 {
		// ids 是可供旧规则抽取的普通物品定义编号集合。
		ids := f.Drops.ItemIDs
		// item 组装道具所需的数据。
		item := domain.Item{ID: domain.ID(), ItemID: ids[random(int64(len(ids)))], Quantity: 1, Bound: bound, Version: 1}
		item.SourceKind, item.SourceID = "monster_drop", fmt.Sprint(q.SourceSeq)
		// definition 提供配置中的物品定义，用来读取生成掉落时的品质。
		definition, _ := f.Item(item.ItemID)
		item.Quality = definition.DropQuality
		q.Item = &item
	}
	if r.Conn == nil {
		q.HostingSession = r.HostSession
	}
	w.settle(r, q)
}

// settle 用当前世界和角色位置提交来源收益。
// w 是调用该方法的房间执行器实例，r 指向当前执行器拥有并更新的目标房间，q 记录结算，finished 记录“finished”，使用可变数量的供当前流程调用的函数表示。
func (w *worker) settle(r *room, q persistence.Settlement, finished ...func()) {
	w.settleInWorld(r, r.currentWorld(), r.Player.Pos, q, finished...)
}

// settlementBinding 冻结收益与房间、授权、原世界和操作指纹的绑定。
type settlementBinding struct {
	room             *room            // 最初接受收益的房间实例。
	auth             persistence.Auth // 发往数据库的原账号、角色和会话授权。
	world            worldRef         // 归属战斗和可见战利品的原世界身份。
	point            devconfig.Point  // 原世界中的战利品显示坐标。
	generation       uint64           // 房间内结算代次，防止旧回调覆盖下一笔事务。
	id, fingerprint  string           // 操作编号与内容指纹共同匹配待确认收益。
	progressionActor actorRef         // 能接收升级恢复效果的原角色出生身份。
	sourceBefore     int64            // 提交前的来源水位，用于区分首次确认与已经应用的重放。
}

// settleInWorld 将来源收益送入原世界结算，并按需执行无回执参数的完成动作。
// w 是调用该方法的房间执行器实例，r 指向当前执行器拥有并更新的目标房间，world 记录场景世界，使用场景世界状态表示，point 记录点位，q 记录结算。
// finished 记录“finished”，使用可变数量的供当前流程调用的函数表示。
func (w *worker) settleInWorld(r *room, world *worldState, point devconfig.Point, q persistence.Settlement, finished ...func()) {
	// actor 记录行动实体，使用行动实体引用表示。
	var actor actorRef
	if world == r.currentWorld() {
		actor = referenceIn(world, &r.Player)
	}
	w.settleInWorldReceipt(r, world, point, q, actor, func(_ persistence.Operation) {
		if len(finished) > 0 {
			finished[0]()
		}
	})
}

// settleInWorldReceipt 固化耐久、托管整秒和来源内容，投递永久事务并保存原任务重试。
// w 是调用该方法的房间执行器实例，r 指向当前执行器拥有并更新的目标房间，world 记录场景世界，使用场景世界状态表示，point 记录点位，q 记录结算。
// actor 记录行动实体，使用行动实体引用表示，finished 记录“finished”，使用供当前流程调用的函数表示。
func (w *worker) settleInWorldReceipt(r *room, world *worldState, point devconfig.Point, q persistence.Settlement, actor actorRef, finished func(persistence.Operation)) {
	if r.bossBattle != nil {
		return
	}
	r.SettlementGeneration++
	r.Wear.attach(&q)
	// hostSeconds 是与原事务兼容且已冻结到该事务中的托管整秒。
	hostSeconds := attachHostingSeconds(r, &q)
	// raw 保存由序列化取得的原始值，err 接收序列化过程中可能产生的错误，供后续失败分支处理。
	raw, err := json.Marshal(q)
	if err != nil {
		w.isolateRoom(r)
		return
	}
	// 从仍在计量的整秒中移走已冻结量，后续未知确认由同一数据库任务继续承接。
	r.hostSecondsOpen -= hostSeconds
	if len(q.EquipmentWear) > 0 {
		r.Wear.inflight = nil
	}
	if q.WearDeathID != "" {
		r.Wear.deathID = ""
	}
	// binding 初始化用作会话绑定的结算会话绑定。
	binding := settlementBinding{room: r, auth: r.Auth, world: world.Ref, point: point, generation: r.SettlementGeneration, id: fmt.Sprintf("settle:%d", q.SourceSeq), fingerprint: persistence.Hash(raw)}
	binding.progressionActor, binding.sourceBefore = actor, r.Role.SourceSeq
	r.Pending = true
	r.PendingOperationID, r.PendingFingerprint = binding.id, binding.fingerprint
	// auth 保存当前处理所用的授权。
	auth := r.Auth
	// store 提供原房间的运行服围栏，托管、收益和重试均使用同一视图。
	store := w.roomStore(r)
	// j 记录当前位置的下标，使用任务或职业编号表示。
	var j job
	j = job{target: w, settlement: true, run: func() func(*worker) {
		// role 是事务返回的永久角色视图，receipt 是原来源回执，err 指出写入或确认有无失败。
		role, receipt, err := store.SettleWithReceipt(context.Background(), auth, q)
		// w 记录房间执行器。
		return func(w *worker) {
			w.finishSettlementReceipt(binding, q, role, receipt, err, &j, finished)
		}
	}}
	if !w.submit(j) {
		r.PendingRetry = &j
		r.RetryAt = time.Now().Add(100 * time.Millisecond)
	}
}

// finishSettlement 核对回调归属，原样重试未定结果，确认后才更新资产与原场景观察。
// w 是调用该方法的房间执行器实例，binding 记录会话绑定，使用结算会话绑定表示，q 记录结算，role 记录角色，err 保存当前操作的失败原因，供调用方检查或返回。
// retry 记录重试，使用任务或职业编号表示，finished 记录“finished”，使用可变数量的供当前流程调用的函数表示。
func (w *worker) finishSettlement(binding settlementBinding, q persistence.Settlement, role domain.Role, err error, retry *job, finished ...func()) {
	// r 保存当前处理所用的房间。
	r := binding.room
	if w.rooms[binding.auth.RoleID] != r || r.Auth != binding.auth || r.Isolated || !r.Pending || r.SettlementGeneration != binding.generation || r.PendingOperationID != binding.id || r.PendingFingerprint != binding.fingerprint {
		return
	}
	if err != nil && (persistence.Code(err) == p.CodeOperationPending || persistence.Code(err) == p.CodeServerBusy) {
		r.PendingRetry = retry
		r.RetryAt = time.Now().Add(500 * time.Millisecond)
		return
	}
	r.resumeAfterPending(time.Now())
	r.PendingRetry = nil
	r.PendingOperationID, r.PendingFingerprint = "", ""
	if err != nil {
		w.isolateRoom(r)
		return
	}
	// 只采纳不落后于内存资产视图的持久版本，避免回执重放倒退状态。
	if role.Revision >= r.Role.Revision {
		previousPrivilege := r.Role.PrivilegeUntil
		r.Role = role
		w.acceptRolePrivilege(r, time.Now(), previousPrivilege)
		w.refreshPlayerStats(r)
	}
	if len(finished) > 0 {
		finished[0]()
	}
	if q.Item != nil {
		// world 接收场景世界供后续步骤使用。
		world := r.world(binding.world)
		if world == nil {
			w.isolateRoom(r)
			return
		}
		world.Loot = []*entity{{Handle: 1000, Kind: 4, Def: q.Item.ItemID, Pos: binding.point, HP: 1, MaxHP: 1}}
		world.LootUntil = time.Now().Add(500 * time.Millisecond)
	}
	w.assets(r)
	if q.WearDeathID != "" && q.WearDeathID == r.DeathID && r.Player.HP <= 0 {
		w.deathNotice(r)
	}
	if q.HostingSession != "" && r.Conn != nil && r.Ready {
		w.send(r, p.MsgS2CHostingReport, 0, r.Role.HostingReport())
	}
	w.snapshot(r)
	w.flushKillFacts(r)
	w.after(r)
}

// deathNotice 在必要磨损已经结算时，通知附着连接当前死亡选项。
// w 是调用该方法的房间执行器实例，r 指向当前执行器拥有并更新的目标房间。
func (w *worker) deathNotice(r *room) {
	if r.Conn == nil || r.Wear.deathID != "" && r.Wear.deathID == r.DeathID {
		return
	}
	// allowed 记录当前规则允许的死亡后操作选项。
	allowed := uint32(0)
	if w.e.Store.Cfg.Fixture.AllowFreeDevRevive {
		allowed = 1
	}
	w.send(r, p.MsgS2CDeathNotice, 0, &pb.DeathNotice{DeathId: domain.Bytes(r.DeathID), CauseCode: 1, AllowedActions: allowed, RuleVersion: w.e.Store.Cfg.Fixture.RuleVersion})
}
