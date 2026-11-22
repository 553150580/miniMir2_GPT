// 施法消耗品和自动补充选择的持久规则。验证技能、职业、耗材实例及数量，扣除后输出可幂等重放的消耗回执。

package persistence

import (
	"math"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	p "minimir/internal/protocol"
)

// 回执只记录永久消耗事实；随机持续时长、MP/CD与施法现场只留在原room的未决命令中。
// CastingReceipt 记录本次施法耗材消耗与触发它的原技能、物品和生命代次。
type CastingReceipt struct {
	// Scope 记录作用域，使用文本表示。
	Scope string `bson:"scope"`
	// SkillID 标识当前业务使用的技能。
	SkillID uint32 `bson:"skill_id"`
	// InstanceID 标识当前业务使用的进程或道具实例。
	InstanceID string `bson:"instance_id"`
	// Quantity 记录数量，使用数值表示。
	Quantity int64 `bson:"quantity"`
}

// CastingSupplyGate 复核施法所需耗材已装备且有库存，并确定当前规则是否允许消费。
// r 记录角色，q 记录资产请求，f 记录开发测试配置。
func CastingSupplyGate(r domain.Role, q AssetRequest, f devconfig.Fixture) uint32 {
	// skill 接收技能供后续步骤使用，exists 保存由技能取得的存在。
	skill, exists := f.Skill(q.SkillID)
	if !exists || skill.MaterialItemID == 0 || skill.MaterialQuantity <= 0 || !f.CastingSupply.Enabled {
		return p.CodeSkillMaterialRequired
	}
	// rank 保存由技能取得的排序位次，learned 保存由技能取得的已学习。
	rank, learned := r.Skill(skill.ID)
	if !learned || r.Job != skill.JobID || rank < skill.LearnedLevel || rank > skill.MaxLevel {
		return p.CodeSkillNotLearned
	}
	// equipped 接收遍历已穿戴装备时当前的元素。
	for _, equipped := range r.Equipped {
		if equipped.Slot != devconfig.CastingSupplySlot {
			continue
		}
		// item 保存当前处理所用的道具。
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

// consumeCastingMaterial 依据原库存来源和施法资格扣除一个耗材，返回保留原身份的回执。
// r 记录角色，q 记录资产请求，f 记录开发测试配置。
func consumeCastingMaterial(r domain.Role, q AssetRequest, f devconfig.Fixture) (domain.Role, *CastingReceipt, uint32) {
	if q.CastGateCode != 0 {
		return r, nil, q.CastGateCode
	}
	if q.SceneEpoch == 0 {
		return r, nil, p.CodeInvalidArgument
	}
	// code 保存由施法耗材门禁取得的结果码。
	if code := CastingSupplyGate(r, q, f); code != 0 {
		return r, nil, code
	}
	// skill 接收技能供后续步骤使用。
	skill, _ := f.Skill(q.SkillID)
	r.Equipped = append([]domain.Equipped(nil), r.Equipped...)
	// index 标记遍历已穿戴装备时访问的下标。
	for index := range r.Equipped {
		if r.Equipped[index].Slot != devconfig.CastingSupplySlot {
			continue
		}
		// item 保存当前处理所用的道具。
		item := &r.Equipped[index].Item
		item.Quantity -= skill.MaterialQuantity
		item.Version++
		if item.Quantity == 0 {
			r.Equipped = append(r.Equipped[:index], r.Equipped[index+1:]...)
		}
		break
	}
	// scope 保存作用域所使用的文本。
	scope := "manual"
	if q.SourceSeq > 0 {
		scope = "auto"
	}
	return r, &CastingReceipt{Scope: scope, SkillID: skill.ID, InstanceID: q.ItemID, Quantity: skill.MaterialQuantity}, 0
}

// setCastingSupply 切换或卸下角色当前装备的施法耗材，并核查背包容量及实例归属。
// r 记录角色，q 记录资产请求，f 记录开发测试配置。
func setCastingSupply(r domain.Role, q AssetRequest, f devconfig.Fixture) (domain.Role, uint32) {
	if !f.CastingSupply.Enabled {
		return r, p.CodeSkillMaterialRequired
	}
	if len(q.ItemID) != 32 || q.ItemVersion == 0 || q.ItemVersion > math.MaxInt64 {
		return r, p.CodeSkillMaterialChanged
	}
	// equippedIndex 保存当前处理所用的已穿戴装备下标。
	equippedIndex := -1
	// i 定位已穿戴装备中正在处理的成员，equipped 接收遍历已穿戴装备时当前的元素。
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
		// item 保存当前处理所用的道具。
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
	// bagIndex 保存当前处理所用的背包下标。
	bagIndex := -1
	// i 定位道具集合中正在处理的成员，item 接收遍历道具集合时当前的元素。
	for i, item := range r.Items {
		if item.ID == q.ItemID {
			bagIndex = i
			break
		}
	}
	if bagIndex < 0 {
		return r, p.CodeSkillMaterialChanged
	}
	// item 保存从道具集合取出的道具。
	item := r.Items[bagIndex]
	// definition 保存由道具取得的定义，exists 保存由道具取得的存在。
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
		// old 保存道具，用于当前步骤中的原状态。
		old := r.Equipped[equippedIndex].Item
		old.Version++
		r.Items = append(r.Items, old)
		r.Equipped[equippedIndex] = domain.Equipped{Slot: devconfig.CastingSupplySlot, Item: item}
	} else {
		r.Equipped = append(r.Equipped, domain.Equipped{Slot: devconfig.CastingSupplySlot, Item: item})
	}
	return r, 0
}
