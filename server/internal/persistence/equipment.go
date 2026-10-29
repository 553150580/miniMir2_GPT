package persistence

import (
	"math"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	p "minimir/internal/protocol"
	"slices"
)

type EquipmentMovement struct {
	From   string      `bson:"from"`
	To     string      `bson:"to"`
	Before domain.Item `bson:"before"`
	After  domain.Item `bson:"after"`
}

// 原物品完整快照属于首次提交，重放不能按当前装备重新解释成反向动作。
type EquipmentReceipt struct {
	Slot        uint32              `bson:"slot"`
	Unequip     bool                `bson:"unequip"`
	Moves       []EquipmentMovement `bson:"moves"`
	RuleVersion uint64              `bson:"rule_version"`
	ConfigHash  string              `bson:"config_hash"`
}

func cloneEquipmentReceipt(in *EquipmentReceipt) *EquipmentReceipt {
	if in == nil {
		return nil
	}
	out := *in
	out.Moves = slices.Clone(in.Moves)
	return &out
}

func equipmentItemIntegrity(item domain.Item, slot uint32, f devconfig.Fixture) bool {
	d, ok := f.Item(item.ItemID)
	return ok && d.IsEquipment && d.EquipmentSlot == slot && slot <= 9 && item.Quantity == 1 && item.Version > 0 && item.Version < math.MaxInt64 && item.Quality <= 4
}

// 当前穿入资格不能反过来困住旧已穿戴物；旧合法品质仅原样返包，不因此获得重新穿入或回收资格。
func equipmentNewWearEligible(r domain.Role, item domain.Item, slot uint32, f devconfig.Fixture) bool {
	d, _ := f.Item(item.ItemID)
	if !equipmentItemIntegrity(item, slot, f) || item.Quality != d.DropQuality || !d.EquipmentRecycleKnown() || !d.EquipmentStatsKnown() || r.Level < *d.RequiredLevel {
		return false
	}
	allowed := len(d.AllowedJobs) == 0
	for _, job := range d.AllowedJobs {
		allowed = allowed || job == r.Job
	}
	return allowed
}

func equipmentOccupied(item domain.Item) bool {
	return item.PendingResultID != "" || item.ReservationID != ""
}

func equipmentRoleUnique(r domain.Role) bool {
	ids := make(map[string]bool, len(r.Items)+len(r.Equipped))
	for _, item := range r.Items {
		if len(item.ID) != 32 || len(domain.Bytes(item.ID)) != 16 || domain.Hex(domain.Bytes(item.ID)) != item.ID || ids[item.ID] {
			return false
		}
		ids[item.ID] = true
	}
	slots := map[uint32]bool{}
	for _, equipped := range r.Equipped {
		item := equipped.Item
		if len(item.ID) != 32 || len(domain.Bytes(item.ID)) != 16 || domain.Hex(domain.Bytes(item.ID)) != item.ID || ids[item.ID] || slots[equipped.Slot] || equipped.Slot > 9 && equipped.Slot != devconfig.CastingSupplySlot {
			return false
		}
		ids[item.ID], slots[equipped.Slot] = true, true
	}
	return true
}

func equipmentRoleArithmeticSafe(r domain.Role, f devconfig.Fixture) bool {
	a, ok := f.PlayerAtLevel(r.Job, r.Level)
	if !ok {
		return false
	}
	for _, eq := range r.Equipped {
		if eq.Slot == devconfig.CastingSupplySlot {
			continue
		}
		d, exists := f.Item(eq.Item.ItemID)
		if !exists {
			return false
		}
		a, ok = d.AddEquipmentBonuses(a)
		if !ok {
			return false
		}
	}
	return f.EquipmentArithmeticSafe(a)
}

func equipEquipment(r domain.Role, q AssetRequest, f devconfig.Fixture) (domain.Role, *EquipmentReceipt, uint32) {
	if f.RuleVersion < 22 {
		return equipEquipmentLegacy(r, q, f)
	}
	if q.Slot > 9 || len(q.ItemID) != 32 || len(domain.Bytes(q.ItemID)) != 16 || q.ItemVersion == 0 || q.ItemVersion > math.MaxInt64 || r.Capacity < 1 || !equipmentRoleUnique(r) {
		return r, nil, p.CodeEquipmentChanged
	}
	worn := -1
	for n, eq := range r.Equipped {
		if eq.Slot == q.Slot {
			worn = n
		}
	}
	var incoming domain.Item
	bag := -1
	if q.Unequip {
		if worn < 0 {
			return r, nil, p.CodeEquipmentChanged
		}
		incoming = r.Equipped[worn].Item
	} else {
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
		old := r.Equipped[worn].Item
		if !equipmentItemIntegrity(old, q.Slot, f) {
			return r, nil, p.CodeEquipmentChanged
		}
		if equipmentOccupied(old) {
			return r, nil, p.CodeEquipmentBlocked
		}
	}
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
	out := r
	out.Items, out.Equipped = slices.Clone(r.Items), slices.Clone(r.Equipped)
	receipt := &EquipmentReceipt{Slot: q.Slot, Unequip: q.Unequip, RuleVersion: f.RuleVersion, ConfigHash: f.Hash}
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
			old := out.Equipped[worn].Item
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
func equipEquipmentLegacy(r domain.Role, q AssetRequest, f devconfig.Fixture) (domain.Role, *EquipmentReceipt, uint32) {
	if q.Unequip {
		return r, nil, p.CodeInvalidArgument
	}
	index := -1
	for n, item := range r.Items {
		if item.ID == q.ItemID {
			index = n
			break
		}
	}
	if index < 0 {
		return r, nil, p.CodeInvalidArgument
	}
	item := r.Items[index]
	d, ok := f.Item(item.ItemID)
	allowed := len(d.AllowedJobs) == 0
	for _, job := range d.AllowedJobs {
		allowed = allowed || job == r.Job
	}
	if !ok || !d.IsEquipment || d.EquipmentSlot != q.Slot || q.Slot > 9 || !allowed || f.RuleVersion >= 20 && (d.RequiredLevel == nil || *d.RequiredLevel < 1 || r.Level < *d.RequiredLevel) {
		return r, nil, p.CodeInvalidArgument
	}
	out := r
	out.Items, out.Equipped = slices.Clone(r.Items), slices.Clone(r.Equipped)
	out.Items = append(out.Items[:index], out.Items[index+1:]...)
	item.Version++
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
