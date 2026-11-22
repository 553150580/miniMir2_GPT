// 把命中或死亡引起的耐久磨损应用到装备实例。按原穿戴槽位与实例版本校验扣减，输出可查证的磨损回执。

package persistence

import (
	"slices"

	"minimir/internal/devconfig"
	"minimir/internal/domain"
	p "minimir/internal/protocol"
)

// EquipmentWear 描述游戏房间生成的一笔耐久扣减事实，绑定目标装备实例和槽位。
type EquipmentWear struct {
	Slot       uint32 `bson:"slot" json:"slot"`                             // 磨损发生时占用的装备槽。
	InstanceID string `bson:"instance_id" json:"instance_id"`               // 磨损发生时实际穿戴的物品实例。
	Version    int64  `bson:"version" json:"version"`                       // 扣减前必须仍匹配的实例版本。
	AttackU    int64  `bson:"attack_u,omitempty" json:"attack_u,omitempty"` // 攻击造成的武器磨损整数单位。
	HitU       int64  `bson:"hit_u,omitempty" json:"hit_u,omitempty"`       // 被命中造成的非武器磨损整数单位。
	DeathU     int64  `bson:"death_u,omitempty" json:"death_u,omitempty"`   // 本笔死亡标识对应的磨损整数单位。
}

// EquipmentWearReceipt 记录扣减实际落到的装备状态及其成本，供结算后复核。
type EquipmentWearReceipt struct {
	// Cost 记录消耗，使用装备磨损表示。
	Cost EquipmentWear `bson:"cost"`
	// Before 记录操作前的状态，使用道具表示。
	Before domain.Item `bson:"before"`
	// After 记录操作后的状态，使用道具表示。
	After domain.Item `bson:"after"`
}

// applyEquipmentWear 核对每项磨损原实例和规则后扣减耐久，生成可永久查证的扣减结果。
// r 记录角色，costs 记录“costs”，使用装备磨损列表表示，deathID 标识当前业务使用的死亡，f 记录开发测试配置。
func applyEquipmentWear(r domain.Role, costs []EquipmentWear, deathID string, f *devconfig.Fixture) (domain.Role, []EquipmentWearReceipt, error) {
	if len(costs) == 0 && deathID == "" {
		return r, nil, nil
	}
	if f.EquipmentDurability == nil || len(costs) == 0 || len(costs) > 10 || !equipmentRoleUnique(r) {
		return r, nil, Fail(p.CodeOperationConflict, "装备磨损事实不完整")
	}
	// out 保存当前处理所用的输出。
	out := r
	out.Equipped = slices.Clone(r.Equipped)
	// used 建立带初始值的按数值索引的布尔状态映射，供本段继续处理。
	used := map[uint32]bool{}
	// receipts 创建用于维护回执集合的容器或通道。
	receipts := make([]EquipmentWearReceipt, 0, len(costs))
	// death 保存当前处理所用的死亡。
	death := false
	// 同份事实中每个槽只处理一次，同次死亡也只能记录一次死亡磨损。
	// cost 接收遍历“costs”时当前的元素。
	for _, cost := range costs {
		// matches 封装“matches”对应的后续处理，worn 记录“worn”，使用已穿戴装备表示。
		matches := func(worn domain.Equipped) bool { return worn.Slot == cost.Slot }
		// index 保存由下标“func”取得的下标。
		index := slices.IndexFunc(out.Equipped, matches)
		if index < 0 || used[cost.Slot] || cost.AttackU < 0 || cost.HitU < 0 || cost.DeathU < 0 ||
			cost.Slot == devconfig.WeaponSlot && cost.HitU != 0 || cost.Slot != devconfig.WeaponSlot && cost.AttackU != 0 ||
			cost.DeathU > f.EquipmentDurability.DeathLossU || death && cost.DeathU != 0 {
			return r, nil, Fail(p.CodeOperationConflict, "装备磨损类型或部位冲突")
		}
		// before 保存道具，用于当前步骤中的操作前的状态。
		before := out.Equipped[index].Item
		if before.ID != cost.InstanceID || before.Version != cost.Version || !equipmentItemIntegrity(before, cost.Slot, *f) || !validItemDurability(before) ||
			cost.AttackU > before.DurabilityU || cost.HitU > before.DurabilityU-cost.AttackU || cost.DeathU > before.DurabilityU-cost.AttackU-cost.HitU {
			return r, nil, Fail(p.CodeOperationConflict, "磨损实例或原耐久变化")
		}
		// loss 保存根据攻击u与命中u的计算结果与死亡u计算的损失。
		loss := cost.AttackU + cost.HitU + cost.DeathU
		if loss <= 0 {
			return r, nil, Fail(p.CodeOperationConflict, "磨损事实未扣除耐久")
		}
		used[cost.Slot], death = true, death || cost.DeathU != 0
		// after 保存当前处理所用的操作后的状态。
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
