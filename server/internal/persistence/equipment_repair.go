package persistence

import (
	"math"
	"math/bits"
	"slices"

	"minimir/internal/devconfig"
	"minimir/internal/domain"
	p "minimir/internal/protocol"
)

type RepairPlan struct {
	Kind     uint32        `bson:"kind"`
	Equipped bool          `bson:"equipped"`
	Slot     uint32        `bson:"slot"`
	Before   domain.Item   `bson:"before"`
	After    domain.Item   `bson:"after"`
	Spend    domain.Wallet `bson:"spend"`
}

// 旧实例的三个值都缺失时初始化一次；原上限非零时，当前为零表示已损坏。
func InitializeItemDurability(item domain.Item, f *devconfig.Fixture) domain.Item {
	if f.EquipmentDurability == nil || item.OriginalDurabilityU != 0 || item.MaxDurabilityU != 0 || item.DurabilityU != 0 {
		return item
	}
	def, ok := f.Item(item.ItemID)
	if ok && def.IsEquipment && def.OriginalDurabilityU > 0 {
		item.OriginalDurabilityU = def.OriginalDurabilityU
		item.MaxDurabilityU = def.OriginalDurabilityU
		item.DurabilityU = def.OriginalDurabilityU
	}
	return item
}

func hydrateRoleDurability(r domain.Role, f *devconfig.Fixture) domain.Role {
	if f.EquipmentDurability == nil {
		return r
	}
	itemsCopied, equippedCopied := false, false
	for n, old := range r.Items {
		if item := InitializeItemDurability(old, f); item != old {
			if !itemsCopied {
				r.Items, itemsCopied = slices.Clone(r.Items), true
			}
			r.Items[n] = item
		}
	}
	for n, old := range r.Equipped {
		if item := InitializeItemDurability(old.Item, f); item != old.Item {
			if !equippedCopied {
				r.Equipped, equippedCopied = slices.Clone(r.Equipped), true
			}
			r.Equipped[n].Item = item
		}
	}
	return r
}

func validItemDurability(i domain.Item) bool {
	return i.HasValidDurability()
}

// 计算口径只依据本次缺额与该实例的永久原始上限，费用向上取整。
func repairCost(missing, original, fullCost int64) int64 {
	hi, lo := bits.Mul64(uint64(missing), uint64(fullCost))
	units, remainder := bits.Div64(hi, lo, uint64(original))
	if remainder != 0 {
		units++
	}
	return int64(units)
}

func repairItem(r domain.Role, id string) (domain.Item, int, uint32, bool) {
	for n, item := range r.Items {
		if item.ID == id {
			return item, n, 0, false
		}
	}
	for n, worn := range r.Equipped {
		if worn.Item.ID == id {
			return worn.Item, n, worn.Slot, true
		}
	}
	return domain.Item{}, -1, 0, false
}

func BuildRepairPlan(r domain.Role, f *devconfig.Fixture, option devconfig.RepairRule, id string, version uint64) (RepairPlan, uint32) {
	configured, allowed := f.EquipmentDurability.Repair(option.Kind)
	if !allowed || configured != option || !equipmentRoleUnique(r) {
		return RepairPlan{}, p.CodeEquipmentBlocked
	}
	item, index, slot, equipped := repairItem(r, id)
	def, known := f.Item(item.ItemID)
	if index < 0 || !known || !def.IsEquipment || equipped && def.EquipmentSlot != slot || item.Quantity != 1 || item.Version < 1 || item.Version == math.MaxInt64 || uint64(item.Version) != version {
		return RepairPlan{}, p.CodeEquipmentChanged
	}
	item = InitializeItemDurability(item, f)
	if equipmentOccupied(item) || !validItemDurability(item) || item.DurabilityU == item.MaxDurabilityU {
		return RepairPlan{}, p.CodeEquipmentBlocked
	}
	plan := RepairPlan{Kind: option.Kind, Equipped: equipped, Slot: slot, Before: item, After: item}
	missing := item.MaxDurabilityU - item.DurabilityU
	if option.Kind == devconfig.RepairOrdinary {
		if def.RepairBasePrice <= 0 {
			return RepairPlan{}, p.CodeEquipmentBlocked
		}
		minUpper := min(item.MaxDurabilityU, option.MinMaxDurabilityU)
		plan.After.MaxDurabilityU = max(minUpper, item.MaxDurabilityU-missing/option.UpperLossDivisor)
		fee := repairCost(missing, item.OriginalDurabilityU, def.RepairBasePrice)
		plan.Spend.BoundGold = min(max(r.Wallet.BoundGold, 0), fee)
		plan.Spend.Gold = fee - plan.Spend.BoundGold
	} else {
		fee := repairCost(missing, item.OriginalDurabilityU, option.FullRepairCostU)
		plan.Spend.BoundYuanbao = min(max(r.Wallet.BoundYuanbao, 0), fee)
		plan.Spend.Yuanbao = fee - plan.Spend.BoundYuanbao
	}
	plan.After.DurabilityU = plan.After.MaxDurabilityU
	plan.After.Version++
	return plan, 0
}

func ApplyRepairPlan(r domain.Role, plan RepairPlan) (domain.Role, uint32) {
	item, index, slot, equipped := repairItem(r, plan.Before.ID)
	if index < 0 || item != plan.Before || slot != plan.Slot || equipped != plan.Equipped {
		return r, p.CodeEquipmentChanged
	}
	if plan.Spend.Gold < 0 || plan.Spend.BoundGold < 0 || plan.Spend.Yuanbao < 0 || plan.Spend.BoundYuanbao < 0 ||
		plan.Spend.Gold > r.Wallet.Gold || plan.Spend.BoundGold > r.Wallet.BoundGold || plan.Spend.Yuanbao > r.Wallet.Yuanbao || plan.Spend.BoundYuanbao > r.Wallet.BoundYuanbao {
		return r, p.CodeEquipmentBlocked
	}
	if equipped {
		r.Equipped = slices.Clone(r.Equipped)
		r.Equipped[index].Item = plan.After
	} else {
		r.Items = slices.Clone(r.Items)
		r.Items[index] = plan.After
	}
	r.Wallet.Gold -= plan.Spend.Gold
	r.Wallet.BoundGold -= plan.Spend.BoundGold
	r.Wallet.Yuanbao -= plan.Spend.Yuanbao
	r.Wallet.BoundYuanbao -= plan.Spend.BoundYuanbao
	return r, 0
}
