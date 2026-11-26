// 本文件核对装备部位、角色等级和此次强化确认的原石堆。
package persistence

import (
	"math"

	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	p "minimir/internal/protocol"
)

// enhancementStoneSpend 记录扣除前的原石堆和本次确切扣量。
type enhancementStoneSpend struct {
	Item     domain.Item `bson:"item"`     // 原堆身份、绑定、数量、来源及版本。
	Quantity int64       `bson:"quantity"` // 实际消费的整数石数。
}

// equipmentEnhancementPlan 冻结可向玩家公开的整笔条款。
type equipmentEnhancementPlan struct {
	Slot           uint32                  `bson:"slot"`             // 强化归角色的普通穿戴部位。
	CurrentLevel   uint32                  `bson:"current_level"`    // 扣料之前的部位等级。
	CurrentBonusBP uint32                  `bson:"current_bonus_bp"` // 原等级累计基础属性加成。
	TargetBonusBP  uint32                  `bson:"target_bonus_bp"`  // 成功升一级时累计加成。
	SuccessPPM     uint32                  `bson:"success_ppm"`      // 本次的成功百万分比。
	AllowBound     bool                    `bson:"allow_bound"`      // 玩家已确认允许选择绑定石。
	Stones         []enhancementStoneSpend `bson:"stones"`           // 依照玩家所选顺序保存原材料。
}

// ValidEquipmentEnhancementQuoteRequest 检查原附着、请求形状与请求投入数量上界。
func ValidEquipmentEnhancementQuoteRequest(q *pb.EquipmentEnhancementQuoteRequest) bool {
	return q != nil && q.SceneEpoch != 0 && q.Slot < domain.EquipmentEnhancementSlots && len(q.UiSessionId) == 16 &&
		len(q.Stones) > 0 && len(q.Stones) <= devconfig.MaxEquipmentEnhancementStoneRefs && len(q.ProtoReflect().GetUnknown()) == 0
}

// buildEquipmentEnhancementPlan 由权威角色和配置计算条款。空部位可购买成长，仍没有可放大的装备属性。
func buildEquipmentEnhancementPlan(role domain.Role, fixture *devconfig.Fixture, q *pb.EquipmentEnhancementQuoteRequest) (equipmentEnhancementPlan, uint32) {
	rules := fixture.EquipmentEnhancement
	if rules == nil {
		return equipmentEnhancementPlan{}, p.CodeEquipmentEnhancementUnavailable
	}
	if !ValidEquipmentEnhancementQuoteRequest(q) || len(q.Stones) > role.Capacity || !equipmentRoleUnique(role) {
		return equipmentEnhancementPlan{}, p.CodeInvalidArgument
	}
	current := role.EnhancementLevel(q.Slot)
	step, exists := rules.Level(current)
	if !exists || current >= role.Level {
		return equipmentEnhancementPlan{}, p.CodeEquipmentEnhancementInputs
	}
	definition, exists := fixture.Item(rules.StoneItemID)
	if !exists || definition.IsEquipment || !definition.Stackable || definition.MaxStack <= 0 {
		return equipmentEnhancementPlan{}, p.CodeEquipmentEnhancementUnavailable
	}
	plan := equipmentEnhancementPlan{Slot: q.Slot, CurrentLevel: current, CurrentBonusBP: rules.BonusBP(current),
		TargetBonusBP: rules.BonusBP(current + 1), SuccessPPM: step.SuccessPPM, AllowBound: q.AllowBound,
		Stones: make([]enhancementStoneSpend, 0, len(q.Stones))}
	owned := make(map[string]domain.Item, len(role.Items)) // 同一原石堆只能参与一次。
	for _, item := range role.Items {
		owned[item.ID] = item
	}
	remaining := step.StoneCost // 逐笔扣除剩余应付数量，避免伪造数量触发求和溢出。
	for _, ref := range q.Stones {
		if ref == nil || len(ref.InstanceId) != 16 || ref.Version == 0 || ref.Version >= math.MaxInt64 ||
			ref.Quantity <= 0 || ref.Quantity > remaining || len(ref.ProtoReflect().GetUnknown()) != 0 {
			return equipmentEnhancementPlan{}, p.CodeEquipmentEnhancementInputs
		}
		id := domain.Hex(ref.InstanceId)
		item, found := owned[id]
		if !found || !validGrowthMaterial(item, definition) || uint64(item.Version) != ref.Version ||
			ref.Quantity > item.Quantity || item.Bound && !q.AllowBound {
			return equipmentEnhancementPlan{}, p.CodeEquipmentEnhancementInputs
		}
		delete(owned, id)
		plan.Stones = append(plan.Stones, enhancementStoneSpend{Item: item, Quantity: ref.Quantity})
		remaining -= ref.Quantity
	}
	if remaining != 0 {
		return equipmentEnhancementPlan{}, p.CodeEquipmentEnhancementInputs
	}
	return plan, 0
}

// enhancementStonesProto 返回原石堆及确切扣量，不混入装备或钱包数据。
func enhancementStonesProto(spent []enhancementStoneSpend) []*pb.EquipmentEnhancementStoneSpend {
	result := make([]*pb.EquipmentEnhancementStoneSpend, 0, len(spent))
	for _, line := range spent {
		result = append(result, &pb.EquipmentEnhancementStoneSpend{Item: line.Item.Proto(), Quantity: line.Quantity})
	}
	return result
}
