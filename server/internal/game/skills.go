// 本文件提供技能共享的输入水位、随机掷点、属性计算、选择目标、视线和持续治疗。
package game

import (
	"minimir/internal/devconfig"
	"time"
)

// pendingCast 保存待确认施法所需的状态。
type pendingCast struct {
	// Scene 记录场景，使用数值表示。
	Scene uint64
	// Skill 记录技能。
	Skill devconfig.Skill
	// Target 记录目标，使用数值表示。
	Target uint32
	// TargetLife 记录目标生命代次，使用数值表示。
	TargetLife uint64
	// TargetPoint 记录目标点位。
	TargetPoint devconfig.Point
	// Due 记录已到期，使用时间点表示。
	Due time.Time
	// Amount 记录数量，使用数值表示。
	Amount int64
	// Caster 记录施法者，使用行动实体引用表示。
	Caster actorRef
	// ID 区分当前步骤指向的目标对象。
	ID uint64
	// Order 记录顺序，使用数值表示。
	Order uint64
	// HostSession 记录托管会话标识，使用文本表示。
	HostSession string
	// PrivilegeUntil 记录“privilege”有效截止时间，使用时间点表示。
	PrivilegeUntil time.Time
	// PrivilegeCaptured 标记是否处于“privilege”已捕获状态。
	PrivilegeCaptured bool
	// Direction 记录朝向，使用数值表示。
	Direction uint32
	// LineRemaining 记录线路剩余额度，使用数值表示。
	LineRemaining int
	// SkillLevel 记录技能等级，使用数值表示。
	SkillLevel uint32
	// BuffDurationMS 记录状态效果持续时间毫秒，使用数值表示。
	BuffDurationMS int64
	// Summon 记录召唤，使用召唤宠物计划表示。
	Summon *summonPetPlan
	// FireWall 保存已建立的火墙。字段为空时仍是第一次放置。
	FireWall *fireWallField
	// PoisonColor 来自事务核对过的原药粉；Poison 非空时为已经应用的状态后继。
	PoisonColor devconfig.PoisonColor
	Poison      *poisonEffect
}

// acceptInput 受理输入。
// r 指向当前执行器拥有并更新的目标房间，sequence 记录序号，使用数值表示。
func acceptInput(r *room, sequence uint32) bool {
	if sequence == 0 || sequence <= r.InputSequence {
		return false
	}
	r.InputSequence = sequence
	return true
}

// draw 返回“draw”供调用方使用。
// w 是调用该方法的房间执行器实例，n 记录数值。
func (w *worker) draw(n int64) int64 {
	if n <= 1 {
		return 0
	}
	if w.roll != nil {
		return w.roll(n)
	}
	return random(n)
}

// interval 返回周期供调用方使用。
// w 是调用该方法的房间执行器实例，min 记录最小，使用数值表示，max 记录最大，使用数值表示。
func (w *worker) interval(min, max int64) int64 {
	if min < 0 {
		min = 0
	}
	if max < min {
		max = min
	}
	return min + w.draw(max-min+1)
}

// refreshPlayerStats 刷新玩家统计或属性。
// w 是调用该方法的房间执行器实例，r 指向当前执行器拥有并更新的目标房间。
func (w *worker) refreshPlayerStats(r *room) {
	if r.Player.Kind != 1 {
		return
	}
	// a 保存战斗单位返回的结果。
	a := w.fighter(r, &r.Player)
	r.Player.MaxHP, r.Player.MaxMP = a.HP, a.MP
	if r.Player.HP > a.HP {
		r.Player.HP = a.HP
	}
	if r.Player.MP > a.MP {
		r.Player.MP = a.MP
	}
}

// physicalDamage 返回物理伤害供调用方使用。
// w 是调用该方法的房间执行器实例，r 指向当前执行器拥有并更新的目标房间，from 记录来源，使用实体表示，target 记录目标，使用实体表示。
func (w *worker) physicalDamage(r *room, from, target *entity) int64 {
	// a 保存战斗单位返回的结果，b 保存战斗单位返回的结果。
	a, b := w.fighter(r, from), w.fighter(r, target)
	if w.draw(b.Agility+1) > a.Accuracy {
		return 0
	}
	return statusDamage(target, w.interval(a.AttackMin, a.AttackMax), poisonArmour(target, w.interval(b.DefenceMin, b.DefenceMax)))
}

// magicDamage 返回魔法伤害供调用方使用。
// w 是调用该方法的房间执行器实例，r 指向当前执行器拥有并更新的目标房间，target 记录目标，使用实体表示，amount 记录数量，使用数值表示。
func (w *worker) magicDamage(r *room, target *entity, amount int64) int64 {
	// b 保存战斗单位返回的结果。
	b := w.fighter(r, target)
	if b.MagicResist > 0 && w.draw(b.MagicResistWeight) < b.MagicResist {
		return 0
	}
	return statusDamage(target, amount, poisonArmour(target, w.interval(b.MagicDefenceMin, b.MagicDefenceMax)))
}

// Crystal MagicInfo.cs:180–211：基础区间、技能随机项、四分之一取偶舍入及倍率分开。
// w 是调用该方法的房间执行器实例，s 记录技能，level 记录等级，使用数值表示，base 记录基础值，使用数值表示。
func (w *worker) magicAmount(s devconfig.Skill, level uint32, base int64) int64 {
	// mpower 保存根据m能力值基础值与“draw”计算的“mpower”。
	mpower := s.MPowerBase + w.draw(s.MPowerBonus)
	// power 保存根据能力值基础值与“draw”计算的能力值。
	power := s.PowerBase + w.draw(s.PowerBonus)
	return s.SourceDamage(level, base, s.SourcePower(level, mpower, power))
}

// skillTarget 返回技能目标供调用方使用。
// w 是调用该方法的房间执行器实例，r 指向当前执行器拥有并更新的目标房间，s 记录技能，handle 记录实体句柄，使用数值表示。
func (w *worker) skillTarget(r *room, s devconfig.Skill, handle uint32) *entity {
	return w.skillTargetInWorld(r, r.currentWorld(), s, handle)
}

// skillTargetInWorld 返回技能目标输入场景世界供调用方使用。
// w 是调用该方法的房间执行器实例，r 指向当前执行器拥有并更新的目标房间，world 记录场景世界，使用场景世界状态表示，s 记录技能，handle 记录实体句柄，使用数值表示。
func (w *worker) skillTargetInWorld(r *room, world *worldState, s devconfig.Skill, handle uint32) *entity {
	if s.Behavior == "healing" {
		if r.dungeonCombat != nil {
			if world != r.currentWorld() {
				return nil
			}
			if handle == 0 {
				handle = r.Player.Handle
			}
			for _, ally := range r.dungeonAllies() {
				if ally.body.Handle == handle && ally.body.HP > 0 {
					return ally.body
				}
			}
			return nil
		}
		if handle == 0 || handle == r.Player.Handle {
			if world == r.currentWorld() && r.Player.HP > 0 {
				return &r.Player
			}
			return nil
		}
		// pet 保存由场景世界宠物取得的宠物。
		if pet := r.worldPet(world); pet != nil && pet.Handle == handle && pet.HP > 0 {
			return pet
		}
		return nil
	}
	// m 接收遍历怪物集合中的范围时当前的元素。
	for _, m := range world.combatOpponents() {
		if m.Handle == handle && m.HP > 0 {
			return m
		}
	}
	return nil
}

// lineOfSight 判断线路“of”“sight”是否满足要求。
// w 是调用该方法的房间执行器实例，r 指向当前执行器拥有并更新的目标房间，from 记录来源，使用点位表示，to 记录目标，使用点位表示。
func (w *worker) lineOfSight(r *room, from, to devconfig.Point) bool {
	if !w.inBounds(r, from) || !w.walkable(r, to) {
		return false
	}
	for from != to {
		// dx 保存根据横坐标与横坐标计算的横向差值，dy 保存根据纵坐标与纵坐标计算的纵向差值。
		dx, dy := to.X-from.X, to.Y-from.Y
		if dx > 0 {
			from.X++
		} else if dx < 0 {
			from.X--
		}
		if dy > 0 {
			from.Y++
		} else if dy < 0 {
			from.Y--
		}
		if !w.walkable(r, from) {
			return false
		}
	}
	return true
}

// tickHealing 判断模拟帧治疗是否满足要求。
// w 是调用该方法的房间执行器实例，r 指向当前执行器拥有并更新的目标房间，now 提供当前时点，作为状态有效期和计时判断的基准。
func (w *worker) tickHealing(r *room, now time.Time) bool {
	// h 保存治疗，用于当前步骤中的h。
	h := w.e.Store.Cfg.Fixture.Healing
	if h.TickMS <= 0 || h.LevelsPerStep <= 0 || h.QueueFractionDivisor <= 0 {
		return false
	}
	// changed 保存当前处理所用的已变化。
	changed := false
	// e 接收遍历实体列表时当前的元素。
	for _, e := range []*entity{&r.Player, r.Pet} {
		if e == nil || e.HP <= 0 {
			continue
		}
		// before 保存生命值，用于当前步骤中的操作前的状态。
		before := e.HP
		if now.After(e.NextHeal) {
			e.NextHeal = now.Add(time.Duration(h.TickMS) * time.Millisecond)
			// rate 保存宠物基础值“per”模拟帧，用于当前步骤中的比例或变化速率。
			rate := h.PetBasePerTick
			if e.Kind == 1 {
				rate = h.BasePerTick + int64(r.Role.Level)/h.LevelsPerStep + e.HealAmount/h.QueueFractionDivisor
			}
			e.HP, e.HealAmount = restorePool(e.HP, e.MaxHP, e.HealAmount, rate)
		}
		if e.HP >= e.MaxHP {
			e.HealAmount = 0
		}
		changed = changed || before != e.HP
	}
	return changed
}
