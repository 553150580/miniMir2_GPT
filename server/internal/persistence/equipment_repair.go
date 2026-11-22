// 耐久初始化、旧装备补齐和铁匠修理计划。按原始上限、当前上限和配置计算费用，复核实例版本后落地。

package persistence

import (
	"math"
	"math/bits"
	"slices"

	"minimir/internal/devconfig"
	"minimir/internal/domain"
	p "minimir/internal/protocol"
)

// RepairPlan 冻结原装备状态、修复后耐久和商店报价，供提交时重新核验。
type RepairPlan struct {
	// Kind 记录类型，使用数值表示。
	Kind uint32 `bson:"kind"`
	// Equipped 标记是否处于已穿戴装备状态。
	Equipped bool `bson:"equipped"`
	// Slot 记录槽位，使用数值表示。
	Slot uint32 `bson:"slot"`
	// Before 记录操作前的状态，使用道具表示。
	Before domain.Item `bson:"before"`
	// After 记录操作后的状态，使用道具表示。
	After domain.Item `bson:"after"`
	// Spend 记录“spend”，使用钱包表示。
	Spend domain.Wallet `bson:"spend"`
}

// 旧实例的三个值都缺失时初始化一次；原上限非零时，当前为零表示已损坏。
// InitializeItemDurability 按装备定义为首次生成或旧格式未初始化的实例补齐耐久。
// item 记录道具，f 记录开发测试配置。
func InitializeItemDurability(item domain.Item, f *devconfig.Fixture) domain.Item {
	if f.EquipmentDurability == nil || item.OriginalDurabilityU != 0 || item.MaxDurabilityU != 0 || item.DurabilityU != 0 {
		return item
	}
	// def 保存由道具取得的“def”，ok 表明查询、校验或类型转换是否成功。
	def, ok := f.Item(item.ItemID)
	if ok && def.IsEquipment && def.OriginalDurabilityU > 0 {
		item.OriginalDurabilityU = def.OriginalDurabilityU
		item.MaxDurabilityU = def.OriginalDurabilityU
		item.DurabilityU = def.OriginalDurabilityU
	}
	return item
}

// hydrateRoleDurability 补齐背包与穿戴中的旧格式耐久字段，保留已初始过的损坏状态。
// r 记录角色，f 记录开发测试配置。
func hydrateRoleDurability(r domain.Role, f *devconfig.Fixture) domain.Role {
	if f.EquipmentDurability == nil {
		return r
	}
	// itemsCopied 保存当前处理所用的道具集合“copied”，equippedCopied 保存当前处理所用的已穿戴装备“copied”。
	itemsCopied, equippedCopied := false, false
	// n 定位道具集合中正在处理的成员，old 接收遍历道具集合时当前的元素。
	for n, old := range r.Items {
		// item 保存由“initialize”道具耐久取得的道具。
		if item := InitializeItemDurability(old, f); item != old {
			if !itemsCopied {
				r.Items, itemsCopied = slices.Clone(r.Items), true
			}
			r.Items[n] = item
		}
	}
	// n 定位已穿戴装备中正在处理的成员，old 接收遍历已穿戴装备时当前的元素。
	for n, old := range r.Equipped {
		// item 保存由“initialize”道具耐久取得的道具。
		if item := InitializeItemDurability(old.Item, f); item != old.Item {
			if !equippedCopied {
				r.Equipped, equippedCopied = slices.Clone(r.Equipped), true
			}
			r.Equipped[n].Item = item
		}
	}
	return r
}

// validItemDurability 核对当前、现有上限和原始上限三者是否处在允许范围。
// i 记录当前位置的下标，使用道具表示。
func validItemDurability(i domain.Item) bool {
	return i.HasValidDurability()
}

// 计算口径只依据本次缺额与该实例的永久原始上限，费用向上取整。
// repairCost 按实际损失与原上限折算满修报价，保留整数货币计算。
// missing 记录缺失条目，使用数值表示，original 记录原始，使用数值表示，fullCost 记录完整或满额消耗，使用数值表示。
func repairCost(missing, original, fullCost int64) int64 {
	// hi 保存由“mul”“64”取得的“hi”，lo 保存由“mul”“64”取得的“lo”。
	hi, lo := bits.Mul64(uint64(missing), uint64(fullCost))
	// units 保存由“div”“64”取得的单位集合，remainder 保存由“div”“64”取得的“remainder”。
	units, remainder := bits.Div64(hi, lo, uint64(original))
	if remainder != 0 {
		units++
	}
	return int64(units)
}

// repairItem 从角色背包或装备中定位要修理的唯一实例以及所在位置。
// r 记录角色，id 区分当前步骤指向的目标对象。
func repairItem(r domain.Role, id string) (domain.Item, int, uint32, bool) {
	// n 定位道具集合中正在处理的成员，item 接收遍历道具集合时当前的元素。
	for n, item := range r.Items {
		if item.ID == id {
			return item, n, 0, false
		}
	}
	// n 定位已穿戴装备中正在处理的成员，worn 接收遍历已穿戴装备时当前的元素。
	for n, worn := range r.Equipped {
		if worn.Item.ID == id {
			return worn.Item, n, worn.Slot, true
		}
	}
	return domain.Item{}, -1, 0, false
}

// BuildRepairPlan 根据商人选项与原实例版本计算修复效果和所需费用。
// r 记录角色，f 记录开发测试配置，option 记录选项，使用修理规则表示，id 区分当前步骤指向的目标对象，version 记录版本，使用数值表示。
func BuildRepairPlan(r domain.Role, f *devconfig.Fixture, option devconfig.RepairRule, id string, version uint64) (RepairPlan, uint32) {
	// configured 保存由修理取得的“configured”，allowed 保存由修理取得的允许。
	configured, allowed := f.EquipmentDurability.Repair(option.Kind)
	if !allowed || configured != option || !equipmentRoleUnique(r) {
		return RepairPlan{}, p.CodeEquipmentBlocked
	}
	// item 保存由修理道具取得的道具，index 保存由修理道具取得的下标，slot 保存由修理道具取得的槽位，equipped 保存由修理道具取得的已穿戴装备。
	item, index, slot, equipped := repairItem(r, id)
	// def 保存由道具取得的“def”，known 保存由道具取得的已知状态。
	def, known := f.Item(item.ItemID)
	if index < 0 || !known || !def.IsEquipment || equipped && def.EquipmentSlot != slot || item.Quantity != 1 || item.Version < 1 || item.Version == math.MaxInt64 || uint64(item.Version) != version {
		return RepairPlan{}, p.CodeEquipmentChanged
	}
	item = InitializeItemDurability(item, f)
	if equipmentOccupied(item) || !validItemDurability(item) || item.DurabilityU == item.MaxDurabilityU {
		return RepairPlan{}, p.CodeEquipmentBlocked
	}
	// plan 初始化用作计划的修理计划。
	plan := RepairPlan{Kind: option.Kind, Equipped: equipped, Slot: slot, Before: item, After: item}
	// missing 保存根据最大耐久u与耐久u计算的缺失条目。
	missing := item.MaxDurabilityU - item.DurabilityU
	if option.Kind == devconfig.RepairOrdinary {
		if def.RepairBasePrice <= 0 {
			return RepairPlan{}, p.CodeEquipmentBlocked
		}
		// minUpper 保存由最小取得的最小较高值。
		minUpper := min(item.MaxDurabilityU, option.MinMaxDurabilityU)
		plan.After.MaxDurabilityU = max(minUpper, item.MaxDurabilityU-missing/option.UpperLossDivisor)
		// fee 保存由修理消耗取得的“fee”。
		fee := repairCost(missing, item.OriginalDurabilityU, def.RepairBasePrice)
		plan.Spend.BoundGold = min(max(r.Wallet.BoundGold, 0), fee)
		plan.Spend.Gold = fee - plan.Spend.BoundGold
	} else {
		// fee 保存由修理消耗取得的“fee”。
		fee := repairCost(missing, item.OriginalDurabilityU, option.FullRepairCostU)
		plan.Spend.BoundYuanbao = min(max(r.Wallet.BoundYuanbao, 0), fee)
		plan.Spend.Yuanbao = fee - plan.Spend.BoundYuanbao
	}
	plan.After.DurabilityU = plan.After.MaxDurabilityU
	plan.After.Version++
	return plan, 0
}

// ApplyRepairPlan 再次校验报价对应的实例和支付条件，应用钱包与耐久变更。
// r 记录角色，plan 记录计划，使用修理计划表示。
func ApplyRepairPlan(r domain.Role, plan RepairPlan) (domain.Role, uint32) {
	// item 保存由修理道具取得的道具，index 保存由修理道具取得的下标，slot 保存由修理道具取得的槽位，equipped 保存由修理道具取得的已穿戴装备。
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
