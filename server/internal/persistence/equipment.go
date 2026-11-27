// 装备换装、卸装和属性快照计算。按装备实例、槽位、角色职业等级与版本校验变更，并在回执中记录资产结果。

package persistence

import (
	"math"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	p "minimir/internal/protocol"
	"slices"
)

// EquipmentMovement 描述一次换装或卸装涉及的实例、去向与变更前后版本。
type EquipmentMovement struct {
	// From 记录来源，使用文本表示。
	From string `bson:"from"`
	// To 记录目标，使用文本表示。
	To string `bson:"to"`
	// Before 记录操作前的状态，使用道具表示。
	Before domain.Item `bson:"before"`
	// After 记录操作后的状态，使用道具表示。
	After domain.Item `bson:"after"`
}

// 原物品完整快照属于首次提交，重放不能按当前装备重新解释成反向动作。
// EquipmentReceipt 保存装备槽变更、实例移动与已确认的最终穿戴结果。
type EquipmentReceipt struct {
	// Slot 记录槽位，使用数值表示。
	Slot uint32 `bson:"slot"`
	// Unequip 标记是否处于“unequip”状态。
	Unequip bool `bson:"unequip"`
	// Moves 记录移动集合，使用装备移动列表表示。
	Moves []EquipmentMovement `bson:"moves"`
	// RuleVersion 记录规则版本，使用数值表示。
	RuleVersion uint64 `bson:"rule_version"`
	// ConfigHash 记录配置摘要，使用文本表示。
	ConfigHash string `bson:"config_hash"`
}

// cloneEquipmentReceipt 复制回执及内部可变切片，保留首次确认时的装备变化。
// in 记录输入，使用装备回执表示。
func cloneEquipmentReceipt(in *EquipmentReceipt) *EquipmentReceipt {
	if in == nil {
		return nil
	}
	// out 保存当前处理所用的输出。
	out := *in
	out.Moves = slices.Clone(in.Moves)
	return &out
}

// equipmentItemIntegrity 核验装备定义、实例身份和槽位一致性，供读写穿戴状态前检查。
// item 记录道具，slot 记录槽位，使用数值表示，f 记录开发测试配置。
func equipmentItemIntegrity(item domain.Item, slot uint32, f devconfig.Fixture) bool {
	// d 接收道具供后续步骤使用，ok 表明查询、校验或类型转换是否成功。
	d, ok := f.Item(item.ItemID)
	return ok && d.IsEquipment && d.EquipmentSlot == slot && slot <= 9 && item.Quantity == 1 && item.Version > 0 && item.Version < math.MaxInt64 && item.Quality <= 4
}

// 当前穿入资格不能反过来困住旧已穿戴物；旧合法品质仅原样返包，不因此获得重新穿入或回收资格。
// equipmentNewWearEligible 判断指定物品能否作为新装备穿到角色目标槽位。
// r 记录角色，item 记录道具，slot 记录槽位，使用数值表示，f 记录开发测试配置。
func equipmentNewWearEligible(r domain.Role, item domain.Item, slot uint32, f devconfig.Fixture) bool {
	// d 接收道具供后续步骤使用。
	d, _ := f.Item(item.ItemID)
	if !equipmentItemIntegrity(item, slot, f) || item.Quality != d.DropQuality || !d.EquipmentRecycleKnown() || !d.EquipmentStatsKnown() || r.Level < *d.RequiredLevel {
		return false
	}
	// allowed 保存根据“len”与当前数据计算的允许。
	allowed := len(d.AllowedJobs) == 0
	// job 接收遍历允许任务或职业编号集合时当前的元素。
	for _, job := range d.AllowedJobs {
		allowed = allowed || job == r.Job
	}
	return allowed
}

// equipmentOccupied 判断装备实例是否已有待确认结果或预留，暂不可参与其他资产操作。
// item 记录道具。
func equipmentOccupied(item domain.Item) bool {
	return item.PendingResultID != "" || item.ReservationID != ""
}

// equipmentRoleUnique 核查背包与穿戴之间不会出现同一个实例或多个物品占据同一槽。
// r 记录角色。
func equipmentRoleUnique(r domain.Role) bool {
	// ids 创建用于维护标识集合的容器或通道。
	ids := make(map[string]bool, len(r.Items)+len(r.Equipped))
	// item 接收遍历道具集合时当前的元素。
	for _, item := range r.Items {
		if len(item.ID) != 32 || len(domain.Bytes(item.ID)) != 16 || domain.Hex(domain.Bytes(item.ID)) != item.ID || ids[item.ID] {
			return false
		}
		ids[item.ID] = true
	}
	// slots 初始化用作槽位集合的按数值索引的布尔状态映射。
	slots := map[uint32]bool{}
	// equipped 接收遍历已穿戴装备时当前的元素。
	for _, equipped := range r.Equipped {
		// item 保存当前处理所用的道具。
		item := equipped.Item
		if len(item.ID) != 32 || len(domain.Bytes(item.ID)) != 16 || domain.Hex(domain.Bytes(item.ID)) != item.ID || ids[item.ID] || slots[equipped.Slot] || equipped.Slot > 9 && equipped.Slot != devconfig.CastingSupplySlot {
			return false
		}
		ids[item.ID], slots[equipped.Slot] = true, true
	}
	return true
}

// equipmentRoleArithmeticSafe 核对换装后属性汇总是否会越界或引入非法数值。
// r 记录角色，f 记录开发测试配置。
func equipmentRoleArithmeticSafe(r domain.Role, f devconfig.Fixture) bool {
	// a 保存由玩家发生时间等级取得的a，ok 表明查询、校验或类型转换是否成功。
	a, ok := f.PlayerAtLevel(r.Job, r.Level)
	if !ok {
		return false
	}
	var gear devconfig.EquipmentContributions // 与当前场景和挑战战斗共用强化前后的定点聚合。
	useGearGrowth := f.EquipmentEnhancement != nil || f.EquipmentRefine != nil
	// eq 接收遍历已穿戴装备时当前的元素。
	for _, eq := range r.Equipped {
		if eq.Slot == devconfig.CastingSupplySlot {
			continue
		}
		// d 接收道具供后续步骤使用，exists 保存由道具取得的存在。
		d, exists := f.Item(eq.Item.ItemID)
		if !exists {
			return false
		}
		if useGearGrowth {
			if !gear.Add(d, f.EquipmentEnhancement.BonusBP(r.EnhancementLevel(eq.Slot))) {
				return false
			}
			if f.EquipmentRefine != nil {
				for _, affix := range eq.Item.Refine.Current {
					if !gear.AddRefine(affix.AttributeID, affix.ValueUnits) {
						return false
					}
				}
			}
			continue
		}
		a, ok = d.AddEquipmentBonuses(a)
		if !ok {
			return false
		}
	}
	if useGearGrowth {
		a, ok = gear.Apply(a)
		if !ok {
			return false
		}
	}
	a, ok = f.ApplyMedalBonuses(a, r.MedalLevel())
	return ok && f.EquipmentArithmeticSafe(a)
}

// equipEquipment 执行换装或卸装，按实例版本、保护状态和背包容量生成新角色与回执。
// r 记录角色，q 记录资产请求，f 记录开发测试配置。
func equipEquipment(r domain.Role, q AssetRequest, f devconfig.Fixture) (domain.Role, *EquipmentReceipt, uint32) {
	if f.RuleVersion < 22 {
		return equipEquipmentLegacy(r, q, f)
	}
	if q.Slot > 9 || len(q.ItemID) != 32 || len(domain.Bytes(q.ItemID)) != 16 || q.ItemVersion == 0 || q.ItemVersion > math.MaxInt64 || r.Capacity < 1 || !equipmentRoleUnique(r) {
		return r, nil, p.CodeEquipmentChanged
	}
	// worn 保存当前处理所用的“worn”。
	worn := -1
	// n 定位已穿戴装备中正在处理的成员，eq 接收遍历已穿戴装备时当前的元素。
	for n, eq := range r.Equipped {
		if eq.Slot == q.Slot {
			worn = n
		}
	}
	// incoming 记录新到达项，使用道具表示。
	var incoming domain.Item
	// bag 保存当前处理所用的背包。
	bag := -1
	if q.Unequip {
		if worn < 0 {
			return r, nil, p.CodeEquipmentChanged
		}
		incoming = r.Equipped[worn].Item
	} else {
		// n 定位道具集合中正在处理的成员，item 接收遍历道具集合时当前的元素。
		for n, item := range r.Items {
			if item.ID == q.ItemID {
				bag, incoming = n, item
				break
			}
		}
		if bag < 0 {
			return r, nil, p.CodeEquipmentChanged
		}
	}
	if incoming.ID != q.ItemID || incoming.Version != int64(q.ItemVersion) || !equipmentItemIntegrity(incoming, q.Slot, f) || !q.Unequip && !equipmentNewWearEligible(r, incoming, q.Slot, f) {
		return r, nil, p.CodeEquipmentChanged
	}
	if equipmentOccupied(incoming) {
		return r, nil, p.CodeEquipmentBlocked
	}
	if !q.Unequip && worn >= 0 {
		// old 保存道具，用于当前步骤中的原状态。
		old := r.Equipped[worn].Item
		if !equipmentItemIntegrity(old, q.Slot, f) {
			return r, nil, p.CodeEquipmentChanged
		}
		if equipmentOccupied(old) {
			return r, nil, p.CodeEquipmentBlocked
		}
	}
	// bagAfter 记录道具集合中的条目数。
	bagAfter := len(r.Items)
	if q.Unequip {
		bagAfter++
	} else {
		bagAfter--
		if worn >= 0 {
			bagAfter++
		}
	}
	if bagAfter > r.Capacity {
		return r, nil, p.CodeEquipmentBagFull
	}
	// out 保存当前处理所用的输出。
	out := r
	out.Items, out.Equipped = slices.Clone(r.Items), slices.Clone(r.Equipped)
	// receipt 保存当前处理所用的回执。
	receipt := &EquipmentReceipt{Slot: q.Slot, Unequip: q.Unequip, RuleVersion: f.RuleVersion, ConfigHash: f.Hash}
	// moved 保存当前处理所用的“moved”。
	moved := incoming
	moved.Version++
	if q.Unequip {
		out.Items = append(out.Items, moved)
		out.Equipped = append(out.Equipped[:worn], out.Equipped[worn+1:]...)
		receipt.Moves = append(receipt.Moves, EquipmentMovement{From: "equipped", To: "backpack", Before: incoming, After: moved})
	} else {
		out.Items = append(out.Items[:bag], out.Items[bag+1:]...)
		receipt.Moves = append(receipt.Moves, EquipmentMovement{From: "backpack", To: "equipped", Before: incoming, After: moved})
		if worn >= 0 {
			// old 保存道具，用于当前步骤中的原状态。
			old := out.Equipped[worn].Item
			// returned 保存当前处理所用的已返回值。
			returned := old
			returned.Version++
			out.Items = append(out.Items, returned)
			receipt.Moves = append(receipt.Moves, EquipmentMovement{From: "equipped", To: "backpack", Before: old, After: returned})
			out.Equipped[worn] = domain.Equipped{Slot: q.Slot, Item: moved}
		} else {
			out.Equipped = append(out.Equipped, domain.Equipped{Slot: q.Slot, Item: moved})
		}
	}
	if !equipmentRoleArithmeticSafe(out, f) {
		return r, nil, p.CodeEquipmentChanged
	}
	return out, receipt, 0
}

// 旧fixture保留原穿入语义；普通卸下不能被新字段反向解释。
// equipEquipmentLegacy 沿用旧规则版本的穿戴操作语义，兼容开发快照中的早期装备数据。
// r 记录角色，q 记录资产请求，f 记录开发测试配置。
func equipEquipmentLegacy(r domain.Role, q AssetRequest, f devconfig.Fixture) (domain.Role, *EquipmentReceipt, uint32) {
	if q.Unequip {
		return r, nil, p.CodeInvalidArgument
	}
	// index 保存当前处理所用的下标。
	index := -1
	// n 定位道具集合中正在处理的成员，item 接收遍历道具集合时当前的元素。
	for n, item := range r.Items {
		if item.ID == q.ItemID {
			index = n
			break
		}
	}
	if index < 0 {
		return r, nil, p.CodeInvalidArgument
	}
	// item 保存从道具集合取出的道具。
	item := r.Items[index]
	// d 接收道具供后续步骤使用，ok 表明查询、校验或类型转换是否成功。
	d, ok := f.Item(item.ItemID)
	// allowed 保存根据“len”与当前数据计算的允许。
	allowed := len(d.AllowedJobs) == 0
	// job 接收遍历允许任务或职业编号集合时当前的元素。
	for _, job := range d.AllowedJobs {
		allowed = allowed || job == r.Job
	}
	if !ok || !d.IsEquipment || d.EquipmentSlot != q.Slot || q.Slot > 9 || !allowed || f.RuleVersion >= 20 && (d.RequiredLevel == nil || *d.RequiredLevel < 1 || r.Level < *d.RequiredLevel) {
		return r, nil, p.CodeInvalidArgument
	}
	// out 保存当前处理所用的输出。
	out := r
	out.Items, out.Equipped = slices.Clone(r.Items), slices.Clone(r.Equipped)
	out.Items = append(out.Items[:index], out.Items[index+1:]...)
	item.Version++
	// n 定位已穿戴装备中正在处理的成员，eq 接收遍历已穿戴装备时当前的元素。
	for n, eq := range out.Equipped {
		if eq.Slot == q.Slot {
			out.Items = append(out.Items, eq.Item)
			out.Equipped[n] = domain.Equipped{Slot: q.Slot, Item: item}
			return out, nil, 0
		}
	}
	out.Equipped = append(out.Equipped, domain.Equipped{Slot: q.Slot, Item: item})
	return out, nil, 0
}
