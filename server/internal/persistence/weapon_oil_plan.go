// 本文件选择原武器、原材料和用油条款，并把已确认的一瓶用油计划应用到角色资产。
package persistence

import (
	"math"
	"slices"

	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	p "minimir/internal/protocol"
)

const (
	weaponOilMaxLuck = 9         // 武器用油的最高幸运档位。
	weaponOilPPM     = 1_000_000 // 一个完整概率区间的百万分单位。
)

// weaponOilPlan 冻结确认窗显示的全部资产和条款。随机值不属于公开条款。
type weaponOilPlan struct {
	Weapon      domain.Item `bson:"weapon"`       // 原武器完整实例，确认时必须逐字段相符。
	Oil         domain.Item `bson:"oil"`          // 游戏服选出的原油堆，确认时不能更换。
	Equipped    bool        `bson:"equipped"`     // true 表示原武器来自穿戴槽，否则来自背包。
	AllowBound  bool        `bson:"allow_bound"`  // 本次报价显式采用的绑定材料授权。
	BeforeLuck  uint32      `bson:"before_luck"`  // 报价时的当前幸运，兼容初始幸运物品。
	SuccessLuck uint32      `bson:"success_luck"` // 本瓶成功后的幸运。
	FailureLuck uint32      `bson:"failure_luck"` // 本瓶失败时保持或退回的幸运。
	SuccessPPM  uint32      `bson:"success_ppm"`  // 已考虑原保底进度的本瓶实际成功率。
	PityUsed    uint32      `bson:"pity_used"`    // 本瓶投入前所属段的累计瓶数，无硬保底时为零。
	PityAfter   uint32      `bson:"pity_after"`   // 本段达到后，下瓶起必成的门槛。
	Guaranteed  bool        `bson:"guaranteed"`   // 是否由历史累计触发本瓶硬保底。
	OutputBound bool        `bson:"output_bound"` // 当次实际原油或原武器为绑定时，结果绑定。
}

// validWeaponOilRef 过滤缺失、溢出和未来版本的原装备引用。
func validWeaponOilRef(ref *pb.WeaponOilItemRef) bool {
	return ref != nil && len(ref.InstanceId) == 16 && ref.Version > 0 && ref.Version < math.MaxInt64 &&
		len(ref.ProtoReflect().GetUnknown()) == 0
}

// validWeaponOilQuoteRequest 核对原场景、原武器引用和当前功能界面的身份。
func validWeaponOilQuoteRequest(q *pb.WeaponOilQuoteRequest) bool {
	return q != nil && q.SceneEpoch != 0 && len(q.UiSessionId) == 16 && validWeaponOilRef(q.Weapon) &&
		len(q.ProtoReflect().GetUnknown()) == 0
}

// weaponOilTarget 要求武器仍属于当前角色、仍在原位置，且能够培养。
func weaponOilTarget(role domain.Role, fixture *devconfig.Fixture, ref *pb.WeaponOilItemRef) (domain.Item, bool) {
	var weapon domain.Item
	id := domain.Hex(ref.InstanceId)
	if ref.Equipped {
		for _, entry := range role.Equipped {
			if entry.Slot == 0 && entry.Item.ID == id {
				weapon = entry.Item
				break
			}
		}
	} else {
		for _, entry := range role.Items {
			if entry.ID == id {
				weapon = entry
				break
			}
		}
	}
	if weapon.Version != int64(ref.Version) || !equipmentItemIntegrity(weapon, 0, *fixture) || weapon.Locked || equipmentOccupied(weapon) {
		return domain.Item{}, false
	}
	luck := weapon.EffectiveLuck()
	if weapon.InitialLuck > 3 || luck >= weaponOilMaxLuck || luck < weapon.InitialLuck ||
		luck < 3 && weapon.LuckOilToSix != 0 || luck < 6 && weapon.LuckOilToNine != 0 {
		return domain.Item{}, false
	}
	return weapon, true
}

// selectWeaponOil 关闭授权时只选非绑定原堆，打开时先选绑定原堆。
// 同绑定档用原实例编号固定顺序，背包排序变化不会偷偷更换投入。
func selectWeaponOil(items []domain.Item, definition devconfig.Item, allowBound bool) (domain.Item, bool) {
	var chosen domain.Item
	for _, item := range items {
		if item.ItemID != definition.ID || item.Quantity <= 0 || item.Quantity > definition.MaxStack ||
			item.Version < 1 || item.Version == math.MaxInt64 || item.Locked || equipmentOccupied(item) ||
			item.Bound && !allowBound || item.OrePurity != 0 || item.EffectiveLuck() != 0 ||
			item.LuckOilToSix != 0 || item.LuckOilToNine != 0 || item.DurabilityU != 0 ||
			item.MaxDurabilityU != 0 || item.OriginalDurabilityU != 0 {
			continue
		}
		if chosen.ID == "" || item.Bound && !chosen.Bound || item.Bound == chosen.Bound && item.ID < chosen.ID {
			chosen = item
		}
	}
	return chosen, chosen.ID != ""
}

// buildWeaponOilPlan 同时供报价和提交复核，客户端不提供扣款或概率。
func buildWeaponOilPlan(role domain.Role, fixture *devconfig.Fixture, q *pb.WeaponOilQuoteRequest) (weaponOilPlan, uint32) {
	rules := fixture.LuckOil
	if rules == nil || len(rules.Steps) != weaponOilMaxLuck {
		return weaponOilPlan{}, p.CodeWeaponOilUnavailable
	}
	if !validWeaponOilQuoteRequest(q) || !equipmentRoleUnique(role) {
		return weaponOilPlan{}, p.CodeInvalidArgument
	}
	definition, exists := fixture.Item(rules.ItemID)
	if !exists || definition.IsEquipment || !definition.Stackable || definition.MaxStack < 1 {
		return weaponOilPlan{}, p.CodeWeaponOilUnavailable
	}
	weapon, ownsWeapon := weaponOilTarget(role, fixture, q.Weapon)
	oil, ownsOil := selectWeaponOil(role.Items, definition, q.AllowBound)
	if !ownsWeapon || !ownsOil {
		return weaponOilPlan{}, p.CodeWeaponOilInputs
	}
	luck := weapon.EffectiveLuck()
	step := rules.Steps[luck]
	if step.SuccessPPM > weaponOilPPM || step.FailureLuck > luck || step.FailureLuck < weapon.InitialLuck {
		return weaponOilPlan{}, p.CodeWeaponOilUnavailable
	}
	plan := weaponOilPlan{Weapon: weapon, Oil: oil, Equipped: q.Weapon.Equipped, AllowBound: q.AllowBound,
		BeforeLuck: luck, SuccessLuck: luck + 1, FailureLuck: step.FailureLuck, SuccessPPM: step.SuccessPPM,
		PityAfter: step.PityAfter, OutputBound: weapon.Bound || oil.Bound}
	if step.PityAfter != 0 {
		plan.PityUsed = weapon.LuckOilToSix
		if luck >= 6 {
			plan.PityUsed = weapon.LuckOilToNine
		}
		if plan.PityUsed == math.MaxUint32 {
			return weaponOilPlan{}, p.CodeWeaponOilInputs
		}
		// 第 N 瓶开判前只有 N-1 瓶。第 N 瓶照常抽样，下一瓶起才强制成功。
		plan.Guaranteed = plan.PityUsed >= plan.PityAfter
		if plan.Guaranteed {
			plan.SuccessPPM = weaponOilPPM
		}
	}
	return plan, 0
}

// applyWeaponOilPlan 仅应用已经逐字段确认的计划，保留同一武器和剩余油堆的永久身份。
func applyWeaponOilPlan(role domain.Role, quote weaponOilStoredQuote) (domain.Role, WeaponOilResultReceipt) {
	plan := quote.Plan
	weapon, oil := plan.Weapon, plan.Oil
	success := quote.Roll < plan.SuccessPPM
	weapon.CurrentLuck = plan.FailureLuck
	if success {
		weapon.CurrentLuck = plan.SuccessLuck
	}
	weapon.Bound = plan.OutputBound
	weapon.Version++
	used := plan.PityUsed
	if plan.PityAfter != 0 {
		used++
		if plan.BeforeLuck < 6 {
			weapon.LuckOilToSix = used
		} else {
			weapon.LuckOilToNine = used
		}
	}
	result := WeaponOilResultReceipt{QuoteID: quote.ID, Scene: quote.Scene, UISession: quote.UISession, Rule: quote.Rule,
		Config: quote.Config, Plan: plan, Weapon: weapon, Success: success, PityUsed: used, OilConsumed: 1}
	if plan.Equipped {
		role.Equipped = slices.Clone(role.Equipped)
		for index, entry := range role.Equipped {
			if entry.Item.ID == weapon.ID {
				role.Equipped[index].Item = weapon
				break
			}
		}
	}
	role.Items = slices.Clone(role.Items)
	for index, item := range role.Items {
		if item.ID == weapon.ID {
			role.Items[index] = weapon
			break
		}
	}
	for index, item := range role.Items {
		if item.ID != oil.ID {
			continue
		}
		if oil.Quantity == 1 {
			role.Items = slices.Delete(role.Items, index, index+1)
		} else {
			oil.Quantity--
			oil.Version++
			role.Items[index] = oil
			result.OilRemainder = &oil
		}
		break
	}
	return role, result
}
