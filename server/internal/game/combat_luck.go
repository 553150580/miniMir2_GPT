// 本文件汇总实际装备幸运，并将攻击区间的上限取值与各目标的抵抗分开处理。
package game

import (
	"math"

	"minimir/internal/devconfig"
)

const luckMillion uint32 = 1_000_000 // 幸运表、抵抗和掷点统一使用百万分率。

// equipmentLuck 带上已生效的磨损，避免把破损件、耗材栏或重复部位作为贡献位。
func equipmentLuck(r *room, fixture *devconfig.Fixture, wingLuck uint32) uint32 {
	rules := fixture.CombatLuck
	if rules == nil {
		return 0
	}
	total := luckContribution(rules.WeaponWingPPM, wingLuck)
	var seen uint16
	for _, worn := range r.Role.Equipped {
		item := worn.Item
		if worn.Slot > 9 || item.InitialLuck == 0 || item.Quantity != 1 || item.Quality > 4 || item.Version <= 0 || item.Version == math.MaxInt64 ||
			fixture.EquipmentDurability != nil && item.OriginalDurabilityU > 0 && (!item.HasValidDurability() || item.DurabilityU <= r.Wear.loss(item, worn.Slot)) {
			continue
		}
		bit := uint16(1) << worn.Slot
		definition, known := fixture.Item(item.ItemID)
		if !known || !definition.IsEquipment || definition.EquipmentSlot != worn.Slot || seen&bit != 0 {
			continue
		}
		seen |= bit
		if worn.Slot == devconfig.WeaponSlot {
			total += luckContribution(rules.WeaponWingPPM, item.InitialLuck)
		} else {
			total += luckContribution(rules.OtherPPM, item.InitialLuck)
		}
	}
	return min(total, luckMillion)
}

// luckContribution 按所在部位的精确分档读取贡献，越界的实例值不参与战斗。
func luckContribution(rows []uint32, value uint32) uint32 {
	if uint64(value) < uint64(len(rows)) {
		return rows[value]
	}
	return 0
}

// effectiveAttackLuck 先封顶攻击方贡献总和，再扣目标抵抗，不能用溢出的贡献抵销抵抗。
func effectiveAttackLuck(attack, resistance uint32) uint32 {
	attack = min(attack, luckMillion)
	return attack - min(attack, resistance)
}

// upperAttackBound 按原区间采样的归一化规则取得上限。
func upperAttackBound(lower, upper int64) int64 {
	return max(lower, upper, 0)
}

// luckyDamage 冻结同一次施法可用的区间上限威力和掷点，目标仍按各自的抵抗结算。
type luckyDamage struct {
	attack uint32 // 攻击方已封顶的幸运贡献。
	roll   uint32 // 此次挥刀或施法共享的百万分率掷点。
	upper  int64  // 采用区间上限、沿原技能公式得到的威力。
}

// amount 只替换本次原攻击区间取值，不改变命中、防御和其他倍率。
func (luck luckyDamage) amount(normal int64, resistance uint32) int64 {
	if luck.roll < effectiveAttackLuck(luck.attack, resistance) {
		return luck.upper
	}
	return normal
}

// damageWithLuck 只在已冻结可能的幸运伤害时查询当前原目标的抵抗。
func (w *worker) damageWithLuck(r *room, target *entity, normal int64, luck luckyDamage) int64 {
	if luck.attack == 0 {
		return normal
	}
	return luck.amount(normal, w.fighter(r, target).LuckResistancePPM)
}

// prepareSpellDamage 抽取原职业区间，另外用相同技能随机项计算可能的上限威力。
func (w *worker) prepareSpellDamage(skill devconfig.Skill, level uint32, lower, upper int64, chance uint32) (int64, luckyDamage) {
	base := w.interval(lower, upper)
	limit := upperAttackBound(lower, upper)
	if chance == 0 || base == limit || w.e.Store.Cfg.Fixture.CombatLuck == nil {
		return w.magicAmount(skill, level, base), luckyDamage{}
	}
	normal, maximum := w.magicAmounts(skill, level, base, limit)
	return normal, luckyDamage{attack: chance, upper: maximum}
}

// acceptCastLuck 冻结整次伤害施法的一次掷点。显式目标抵抗已覆盖贡献时无需掷点。
func (w *worker) acceptCastLuck(r *room, plan *preparedCast) luckyDamage {
	luck := plan.Luck
	if luck.attack == 0 || luck.upper == plan.Amount ||
		plan.Skill.Behavior == "poisoning" && plan.PoisonColor != devconfig.PoisonGreen {
		return luckyDamage{}
	}
	if plan.Target.Handle != 0 {
		target := w.resolveInWorld(r, r.currentWorld(), plan.Target)
		if target == nil || target.HP <= 0 || effectiveAttackLuck(luck.attack, w.fighter(r, target).LuckResistancePPM) == 0 {
			return luckyDamage{}
		}
	}
	luck.roll = uint32(w.draw(int64(luckMillion)))
	return luck
}
