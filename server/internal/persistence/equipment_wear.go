package persistence

import (
	"slices"

	"minimir/internal/devconfig"
	"minimir/internal/domain"
	p "minimir/internal/protocol"
)

type EquipmentWear struct {
	Slot       uint32 `bson:"slot" json:"slot"`
	InstanceID string `bson:"instance_id" json:"instance_id"`
	Version    int64  `bson:"version" json:"version"`
	AttackU    int64  `bson:"attack_u,omitempty" json:"attack_u,omitempty"`
	HitU       int64  `bson:"hit_u,omitempty" json:"hit_u,omitempty"`
	DeathU     int64  `bson:"death_u,omitempty" json:"death_u,omitempty"`
}

type EquipmentWearReceipt struct {
	Cost   EquipmentWear `bson:"cost"`
	Before domain.Item   `bson:"before"`
	After  domain.Item   `bson:"after"`
}

func applyEquipmentWear(r domain.Role, costs []EquipmentWear, deathID string, f *devconfig.Fixture) (domain.Role, []EquipmentWearReceipt, error) {
	if len(costs) == 0 && deathID == "" {
		return r, nil, nil
	}
	if f.EquipmentDurability == nil || len(costs) == 0 || len(costs) > 10 || !equipmentRoleUnique(r) {
		return r, nil, Fail(p.CodeOperationConflict, "装备磨损事实不完整")
	}
	out := r
	out.Equipped = slices.Clone(r.Equipped)
	used := map[uint32]bool{}
	receipts := make([]EquipmentWearReceipt, 0, len(costs))
	death := false
	for _, cost := range costs {
		matches := func(worn domain.Equipped) bool { return worn.Slot == cost.Slot }
		index := slices.IndexFunc(out.Equipped, matches)
		if index < 0 || used[cost.Slot] || cost.AttackU < 0 || cost.HitU < 0 || cost.DeathU < 0 ||
			cost.Slot == devconfig.WeaponSlot && cost.HitU != 0 || cost.Slot != devconfig.WeaponSlot && cost.AttackU != 0 ||
			cost.DeathU > f.EquipmentDurability.DeathLossU || death && cost.DeathU != 0 {
			return r, nil, Fail(p.CodeOperationConflict, "装备磨损类型或部位冲突")
		}
		before := out.Equipped[index].Item
		if before.ID != cost.InstanceID || before.Version != cost.Version || !equipmentItemIntegrity(before, cost.Slot, *f) || !validItemDurability(before) ||
			cost.AttackU > before.DurabilityU || cost.HitU > before.DurabilityU-cost.AttackU || cost.DeathU > before.DurabilityU-cost.AttackU-cost.HitU {
			return r, nil, Fail(p.CodeOperationConflict, "磨损实例或原耐久变化")
		}
		loss := cost.AttackU + cost.HitU + cost.DeathU
		if loss <= 0 {
			return r, nil, Fail(p.CodeOperationConflict, "磨损事实未扣除耐久")
		}
		used[cost.Slot], death = true, death || cost.DeathU != 0
		after := before
		after.DurabilityU -= loss
		after.Version++
		out.Equipped[index].Item = after
		receipts = append(receipts, EquipmentWearReceipt{Cost: cost, Before: before, After: after})
	}
	if death && (len(deathID) != 32 || len(domain.Bytes(deathID)) != 16) || !death && deathID != "" {
		return r, nil, Fail(p.CodeOperationConflict, "死亡磨损缺少对应的原死亡身份")
	}
	return out, receipts, nil
}
