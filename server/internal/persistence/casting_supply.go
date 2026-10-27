package persistence

import (
	"math"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	p "minimir/internal/protocol"
)

// 回执只记录永久消耗事实；随机持续时长、MP/CD与施法现场只留在原room的未决命令中。
type CastingReceipt struct {
	Scope      string `bson:"scope"`
	SkillID    uint32 `bson:"skill_id"`
	InstanceID string `bson:"instance_id"`
	Quantity   int64  `bson:"quantity"`
}

func CastingSupplyGate(r domain.Role, q AssetRequest, f devconfig.Fixture) uint32 {
	skill, exists := f.Skill(q.SkillID)
	if !exists || skill.MaterialItemID == 0 || skill.MaterialQuantity <= 0 || !f.CastingSupply.Enabled {
		return p.CodeSkillMaterialRequired
	}
	rank, learned := r.Skill(skill.ID)
	if !learned || r.Job != skill.JobID || rank < skill.LearnedLevel || rank > skill.MaxLevel {
		return p.CodeSkillNotLearned
	}
	for _, equipped := range r.Equipped {
		if equipped.Slot != devconfig.CastingSupplySlot {
			continue
		}
		item := equipped.Item
		if item.ID != q.ItemID || q.ItemVersion == 0 || q.ItemVersion > math.MaxInt64 || item.Version != int64(q.ItemVersion) {
			return p.CodeSkillMaterialChanged
		}
		if !f.IsCastingSupply(item.ItemID) || item.ItemID != skill.MaterialItemID || item.Quantity < skill.MaterialQuantity {
			return p.CodeSkillMaterialRequired
		}
		if item.Version == math.MaxInt64 {
			return p.CodeSkillMaterialChanged
		}
		return 0
	}
	return p.CodeSkillMaterialRequired
}

func consumeCastingMaterial(r domain.Role, q AssetRequest, f devconfig.Fixture) (domain.Role, *CastingReceipt, uint32) {
	if q.CastGateCode != 0 {
		return r, nil, q.CastGateCode
	}
	if q.SceneEpoch == 0 {
		return r, nil, p.CodeInvalidArgument
	}
	if code := CastingSupplyGate(r, q, f); code != 0 {
		return r, nil, code
	}
	skill, _ := f.Skill(q.SkillID)
	r.Equipped = append([]domain.Equipped(nil), r.Equipped...)
	for index := range r.Equipped {
		if r.Equipped[index].Slot != devconfig.CastingSupplySlot {
			continue
		}
		item := &r.Equipped[index].Item
		item.Quantity -= skill.MaterialQuantity
		item.Version++
		if item.Quantity == 0 {
			r.Equipped = append(r.Equipped[:index], r.Equipped[index+1:]...)
		}
		break
	}
	scope := "manual"
	if q.SourceSeq > 0 {
		scope = "auto"
	}
	return r, &CastingReceipt{Scope: scope, SkillID: skill.ID, InstanceID: q.ItemID, Quantity: skill.MaterialQuantity}, 0
}

func setCastingSupply(r domain.Role, q AssetRequest, f devconfig.Fixture) (domain.Role, uint32) {
	if !f.CastingSupply.Enabled {
		return r, p.CodeSkillMaterialRequired
	}
	if len(q.ItemID) != 32 || q.ItemVersion == 0 || q.ItemVersion > math.MaxInt64 {
		return r, p.CodeSkillMaterialChanged
	}
	equippedIndex := -1
	for i, equipped := range r.Equipped {
		if equipped.Slot == devconfig.CastingSupplySlot {
			equippedIndex = i
			break
		}
	}
	if q.Unequip {
		if equippedIndex < 0 {
			return r, p.CodeSkillMaterialRequired
		}
		item := r.Equipped[equippedIndex].Item
		if item.ID != q.ItemID || item.Version != int64(q.ItemVersion) || item.Version == math.MaxInt64 {
			return r, p.CodeSkillMaterialChanged
		}
		if len(r.Items) >= r.Capacity {
			return r, p.CodeOperationConflict
		}
		r.Equipped = append([]domain.Equipped(nil), r.Equipped...)
		r.Items = append([]domain.Item(nil), r.Items...)
		item.Version++
		r.Items = append(r.Items, item)
		r.Equipped = append(r.Equipped[:equippedIndex], r.Equipped[equippedIndex+1:]...)
		return r, 0
	}
	bagIndex := -1
	for i, item := range r.Items {
		if item.ID == q.ItemID {
			bagIndex = i
			break
		}
	}
	if bagIndex < 0 {
		return r, p.CodeSkillMaterialChanged
	}
	item := r.Items[bagIndex]
	definition, exists := f.Item(item.ItemID)
	if item.Version != int64(q.ItemVersion) || item.Version == math.MaxInt64 {
		return r, p.CodeSkillMaterialChanged
	}
	if !exists || !f.IsCastingSupply(item.ItemID) || definition.IsEquipment || definition.EquipmentSlot != devconfig.CastingSupplySlot || item.Quantity <= 0 {
		return r, p.CodeSkillMaterialRequired
	}
	if equippedIndex >= 0 && r.Equipped[equippedIndex].Item.Version == math.MaxInt64 {
		return r, p.CodeSkillMaterialChanged
	}
	r.Items = append([]domain.Item(nil), r.Items...)
	r.Equipped = append([]domain.Equipped(nil), r.Equipped...)
	r.Items = append(r.Items[:bagIndex], r.Items[bagIndex+1:]...)
	item.Version++
	if equippedIndex >= 0 {
		old := r.Equipped[equippedIndex].Item
		old.Version++
		r.Items = append(r.Items, old)
		r.Equipped[equippedIndex] = domain.Equipped{Slot: devconfig.CastingSupplySlot, Item: item}
	} else {
		r.Equipped = append(r.Equipped, domain.Equipped{Slot: devconfig.CastingSupplySlot, Item: item})
	}
	return r, 0
}
