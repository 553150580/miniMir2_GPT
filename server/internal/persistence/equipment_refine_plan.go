// 本文件校验原装备、石堆、四钱包和锁槽，并从冻结的合法词条池生成右侧待选。
package persistence

import (
	"fmt"
	"math"
	"math/bits"

	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	p "minimir/internal/protocol"
)

const (
	refineGenerate     = 1  // 支付本次费用，生成或覆盖最新候选。
	refineAdopt        = 2  // 只采纳原已付候选。
	refineDiscard      = 3  // 只删除原已付候选，保留左侧和已经发生的绑定。
	maxRefineStoneRefs = 16 // 现有四个品质单次最多消耗十六枚石。
)

// equipmentRefinePlan 保存玩家确认前能够看到的原对象和精确投入。
type equipmentRefinePlan struct {
	Target      domain.Item             `bson:"target"`       // 原装备版本、已采纳词条及可能已有的待选。
	Equipped    bool                    `bson:"equipped"`     // 操作发生在原穿戴部位或背包。
	Stage       uint32                  `bson:"stage"`        // 原装备需求等级对应的目录阶段。
	SlotCount   uint32                  `bson:"slot_count"`   // 由原品质固定的槽数。
	LockedMask  uint32                  `bson:"locked_mask"`  // 玩家选择保留的左侧槽。
	AllowBound  bool                    `bson:"allow_bound"`  // 当前界面对绑定资源的明确授权。
	Stones      []enhancementStoneSpend `bson:"stones"`       // 原石堆的消费顺序和确切数量。
	Spend       domain.Wallet           `bson:"spend"`        // 原金币和锁槽元宝的四类分账。
	OutputBound bool                    `bson:"output_bound"` // 生成时在同一原装备立即生效的绑定。
}

// validRefineItemRef 验证原实例、版本和消息字段，实际拥有关系由角色库存核验。
func validRefineItemRef(ref *pb.EquipmentRefineItemRef) bool {
	return ref != nil && len(ref.InstanceId) == 16 && ref.Version > 0 && ref.Version < math.MaxInt64 && len(ref.ProtoReflect().GetUnknown()) == 0
}

// ValidEquipmentRefineQuoteRequest 将询价限定到合法原场景与原界面。
func ValidEquipmentRefineQuoteRequest(q *pb.EquipmentRefineQuoteRequest) bool {
	return q != nil && q.SceneEpoch > 0 && validRefineItemRef(q.Target) && len(q.UiSessionId) == 16 &&
		len(q.LockedSlots) < domain.EquipmentRefineSlots && len(q.Stones) > 0 && len(q.Stones) <= maxRefineStoneRefs && len(q.ProtoReflect().GetUnknown()) == 0
}

// ownedRefineTarget 查找当前角色原件，只认可与原引用相同的位置和确切版本。
func ownedRefineTarget(r domain.Role, f *devconfig.Fixture, ref *pb.EquipmentRefineItemRef) (domain.Item, devconfig.Item, bool) {
	if !validRefineItemRef(ref) {
		return domain.Item{}, devconfig.Item{}, false
	}
	var target domain.Item // 查询未命中时保持空；不会跨背包和穿戴位置回退找旧对象。
	id := domain.Hex(ref.InstanceId)
	if ref.Equipped {
		for _, equipped := range r.Equipped {
			if equipped.Item.ID == id && equipmentItemIntegrity(equipped.Item, equipped.Slot, *f) {
				target = equipped.Item
				break
			}
		}
	} else {
		for _, item := range r.Items {
			if item.ID == id {
				target = item
				break
			}
		}
	}
	definition, found := f.Item(target.ItemID)
	if !found || !equipmentItemIntegrity(target, definition.EquipmentSlot, *f) || !definition.EquipmentStatsKnown() ||
		definition.RequiredLevel == nil || !f.EquipmentRefine.Target(target.ItemID, target.Quality) ||
		uint64(target.Version) != ref.Version || target.Locked || target.ReservationID != "" ||
		!validOwnedRefineState(target, f.EquipmentRefine) {
		return domain.Item{}, devconfig.Item{}, false
	}
	return target, definition, true
}

// validOwnedRefineState 区分历史空词条、归本系统的已付候选和其他业务的占用。
func validOwnedRefineState(item domain.Item, rules *devconfig.EquipmentRefineConfig) bool {
	quality, found := rules.Quality(item.Quality)
	if !found || quality.SlotCount < 1 || quality.SlotCount > domain.EquipmentRefineSlots ||
		!validRefineAffixes(item.Refine.Current, rules, quality, false) {
		return false
	}
	if item.PendingResultID == "" {
		return item.Refine.Candidate == (domain.EquipmentAffixes{}) && item.Refine.LockedMask == 0
	}
	if !lowerHex(item.PendingResultID, 16) || !validRefineAffixes(item.Refine.Candidate, rules, quality, true) ||
		item.Refine.LockedMask >= 1<<quality.SlotCount || bits.OnesCount32(item.Refine.LockedMask) >= int(quality.SlotCount) {
		return false
	}
	for _, slot := range domain.RefineLockedSlots(item.Refine.LockedMask) {
		if item.Refine.Current[slot] != item.Refine.Candidate[slot] {
			return false
		}
	}
	return true
}

// validRefineAffixes 校验冻结的持久词条。旧原值保留，报价才采用现行曲线抽新值。
func validRefineAffixes(lines domain.EquipmentAffixes, rules *devconfig.EquipmentRefineConfig, quality devconfig.EquipmentRefineQuality, populated bool) bool {
	var seen [domain.EquipmentRefineSlots]uint32 // 固定槽的有效编号，无需为普通装备分配映射。
	for slot, line := range lines {
		if line == (domain.EquipmentAffix{}) {
			if populated && uint32(slot) < quality.SlotCount {
				return false
			}
			continue
		}
		definition, found := rules.Affix(line.AttributeID)
		if uint32(slot) >= quality.SlotCount || !found || definition.MinQuality > quality.Quality ||
			line.Tier < 1 || line.Tier > 5 || line.ValueUnits <= 0 || line.ValueUnits > math.MaxInt32 {
			return false
		}
		if !quality.AllowDuplicateAttr {
			for _, id := range seen[:slot] {
				if id == line.AttributeID {
					return false
				}
			}
		}
		seen[slot] = line.AttributeID
	}
	return true
}

// buildEquipmentRefinePlan 冻结真正会消费的石和钱包。生成仍可覆盖同一原件的旧候选。
func buildEquipmentRefinePlan(role domain.Role, f *devconfig.Fixture, q *pb.EquipmentRefineQuoteRequest) (equipmentRefinePlan, uint32) {
	rules := f.EquipmentRefine
	if rules == nil {
		return equipmentRefinePlan{}, p.CodeEquipmentRefineUnavailable
	}
	if !ValidEquipmentRefineQuoteRequest(q) || len(q.Stones) > role.Capacity || !equipmentRoleUnique(role) {
		return equipmentRefinePlan{}, p.CodeInvalidArgument
	}
	target, definition, found := ownedRefineTarget(role, f, q.Target)
	if !found {
		return equipmentRefinePlan{}, p.CodeEquipmentRefineChanged
	}
	quality, _ := rules.Quality(target.Quality)
	if len(q.LockedSlots) >= int(quality.SlotCount) {
		return equipmentRefinePlan{}, p.CodeEquipmentRefineInputs
	}
	plan := equipmentRefinePlan{Target: target, Equipped: q.Target.Equipped, Stage: definition.EquipmentStage,
		SlotCount: quality.SlotCount, AllowBound: q.AllowBound, OutputBound: target.Bound}
	for _, slot := range q.LockedSlots {
		if slot >= quality.SlotCount || plan.LockedMask&(1<<slot) != 0 || target.Refine.Current[slot].AttributeID == 0 {
			return equipmentRefinePlan{}, p.CodeEquipmentRefineInputs
		}
		plan.LockedMask |= 1 << slot
	}
	stone, found := f.Item(rules.StoneItemID)
	if !found || !stone.EquipmentRefineStone || !stone.Stackable || stone.IsEquipment {
		return equipmentRefinePlan{}, p.CodeEquipmentRefineUnavailable
	}
	remaining := quality.StoneCost
	owned := make(map[string]domain.Item, len(role.Items))
	for _, item := range role.Items {
		owned[item.ID] = item
	}
	for _, ref := range q.Stones {
		if ref == nil || len(ref.InstanceId) != 16 || ref.Version == 0 || ref.Version >= math.MaxInt64 ||
			ref.Quantity <= 0 || ref.Quantity > remaining || len(ref.ProtoReflect().GetUnknown()) != 0 {
			return equipmentRefinePlan{}, p.CodeEquipmentRefineInputs
		}
		id := domain.Hex(ref.InstanceId)
		item, found := owned[id]
		if !found || !validGrowthMaterial(item, stone) || !item.Refine.IsZero() || uint64(item.Version) != ref.Version ||
			ref.Quantity > item.Quantity || item.Bound && !q.AllowBound {
			return equipmentRefinePlan{}, p.CodeEquipmentRefineInputs
		}
		delete(owned, id)
		plan.Stones = append(plan.Stones, enhancementStoneSpend{Item: item, Quantity: ref.Quantity})
		plan.OutputBound = plan.OutputBound || item.Bound
		remaining -= ref.Quantity
	}
	goldCost, hasGold := rules.GoldCost(*definition.RequiredLevel)
	yuanbaoCost, hasLockCost := rules.LockCost(uint32(len(q.LockedSlots)))
	if remaining != 0 || !hasGold || !hasLockCost {
		return equipmentRefinePlan{}, p.CodeEquipmentRefineInputs
	}
	if q.AllowBound {
		plan.Spend.BoundGold = min(max(role.Wallet.BoundGold, 0), goldCost)
		plan.Spend.BoundYuanbao = min(max(role.Wallet.BoundYuanbao, 0), yuanbaoCost)
	}
	plan.Spend.Gold, plan.Spend.Yuanbao = goldCost-plan.Spend.BoundGold, yuanbaoCost-plan.Spend.BoundYuanbao
	plan.OutputBound = plan.OutputBound || plan.Spend.BoundGold > 0 || plan.Spend.BoundYuanbao > 0
	if !vendorCanPay(role.Wallet, plan.Spend) {
		return equipmentRefinePlan{}, p.CodeEquipmentRefineInputs
	}
	return plan, 0
}

// rollEquipmentRefine 用报价专属种子精确抽样，先保留锁槽，再在剩余同品质池逐位置抽词与档位。
func rollEquipmentRefine(plan equipmentRefinePlan, rules *devconfig.EquipmentRefineConfig, seed string) (domain.EquipmentAffixes, bool) {
	var result domain.EquipmentAffixes
	quality, found := rules.Quality(plan.Target.Quality)
	if !found || plan.SlotCount != quality.SlotCount || plan.SlotCount > domain.EquipmentRefineSlots || seed == "" {
		return result, false
	}
	used := make(map[uint32]bool, plan.SlotCount)
	for _, slot := range domain.RefineLockedSlots(plan.LockedMask) {
		result[slot] = plan.Target.Refine.Current[slot]
		used[result[slot].AttributeID] = true
	}
	for slot := uint32(0); slot < plan.SlotCount; slot++ {
		if plan.LockedMask&(1<<slot) != 0 {
			continue
		}
		var eligible []devconfig.EquipmentRefineAffix
		var sum uint64 // 保证传入原百分比抽样器的分母可用。
		for _, affix := range rules.Affixes {
			if affix.MinQuality <= quality.Quality && affix.BaseValuesU[plan.Stage] > 0 && (quality.AllowDuplicateAttr || !used[affix.ID]) {
				eligible = append(eligible, affix)
				sum += uint64(affix.Weight)
			}
		}
		if sum == 0 || sum > math.MaxUint32 {
			return domain.EquipmentAffixes{}, false
		}
		draw := sourceDraw(fmt.Sprintf("refine-attribute:%s:%d", seed, slot), uint32(sum))
		var chosen uint32
		for _, affix := range eligible {
			if draw < affix.Weight {
				chosen = affix.ID
				break
			}
			draw -= affix.Weight
		}
		grade := sourceDraw(fmt.Sprintf("refine-tier:%s:%d", seed, slot), 1_000_000)
		var tier uint32
		for _, candidate := range rules.Tiers {
			if grade < candidate.WeightPPM {
				tier = candidate.Tier
				break
			}
			grade -= candidate.WeightPPM
		}
		value, valid := rules.ValueUnits(chosen, plan.Stage, tier)
		if !valid || value <= 0 {
			return domain.EquipmentAffixes{}, false
		}
		used[chosen] = true
		result[slot] = domain.EquipmentAffix{AttributeID: chosen, Tier: tier, ValueUnits: value}
	}
	return result, true
}

// refineStonesProto 公开实际原石堆与扣量，决策事务没有材料时返回空。
func refineStonesProto(stones []enhancementStoneSpend) []*pb.EquipmentRefineSpend {
	var out []*pb.EquipmentRefineSpend
	for _, stone := range stones {
		out = append(out, &pb.EquipmentRefineSpend{Item: stone.Item.Proto(), Quantity: stone.Quantity})
	}
	return out
}
