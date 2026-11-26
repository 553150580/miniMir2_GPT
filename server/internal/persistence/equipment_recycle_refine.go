// 本文件在原手动分解中判定洗炼石来源，保存原件、产物与单次随机事实。
package persistence

import (
	"strconv"

	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	p "minimir/internal/protocol"
)

const (
	equipmentRecycleRefineSource       = "equipment_recycle_refine" // 与单瓶祝福油分开保存及校验原来源。
	equipmentRecycleRefineRandomDomain = "equipment-recycle-refine" // 本渠道的私有抽样及产物身份前缀。
	equipmentRecycleRefineDenominator  = 1_000_000                  // 原概率采用整数百万分比。
)

// RecycleRefineRoll 冻结原装备、实际概率和已经入包的唯一产物。
type RecycleRefineRoll struct {
	EquipmentInstanceID string       `bson:"equipment_instance_id"` // 已实际分解的原装备编号。
	Numerator           uint32       `bson:"numerator"`             // 本次使用的概率分子。
	Denominator         uint32       `bson:"denominator"`           // 私有抽样的排他范围。
	Draw                uint32       `bson:"draw"`                  // 仅在永久账本和审计保留的整型抽样。
	Stone               *domain.Item `bson:"stone,omitempty"`       // 概率命中就保存完整原石，失手为空。
}

// validEquipmentRecycleRefineStone 验证整个原石的来源和目录身份，禁止用于伪装其他分解产物。
func validEquipmentRecycleRefineStone(item domain.Item, definition devconfig.Item) bool {
	return item.SourceKind == equipmentRecycleRefineSource && definition.EquipmentRefineStone &&
		!definition.EquipmentRecycleMaterial && validEquipmentRecycleMaterialInstance(item, definition)
}

// eligibleEquipmentRecycleRefine 仅接入当前确认的普通击杀原装备，隔离破损、锻造、商店循环与幸运油产出。
func eligibleEquipmentRecycleRefine(item domain.Item, rule devconfig.EquipmentRefineRecycleRule, definition devconfig.Item) bool {
	if item.ItemID != rule.ItemID || item.SourceKind != rule.SourceKind || item.SourceKind != ordinaryEquipmentItemSource ||
		item.Quality != rule.Quality || !definition.IsEquipment || definition.EquipmentStage != rule.Stage ||
		item.DurabilityU <= 0 || !item.HasValidDurability() || item.InitialLuck != 0 || item.CurrentLuck != 0 ||
		item.LuckOilToSix != 0 || item.LuckOilToNine != 0 || !lowerHex(item.ID, 16) {
		return false
	}
	// 普通战斗资产以原收益来源序号标明来源；仅记录非空字符串不足以识别该事实。
	sequence, err := strconv.ParseInt(item.SourceID, 10, 64)
	return err == nil && sequence > 0 && strconv.FormatInt(sequence, 10) == item.SourceID
}

// applyManualRecycleRefine 逐件确认原石并随原分解事务发放，原装备的绑定归属完全继承。
func applyManualRecycleRefine(before domain.Role, out *domain.Role, receipt *EquipmentRecyclingReceipt, f *devconfig.Fixture, attempt *equipmentRecycleAttempt) error {
	rules := f.EquipmentRefine
	if rules == nil {
		return nil
	}
	rule := rules.Recycle
	material, found := f.Item(rules.StoneItemID)
	equipment, equipmentFound := f.Item(rule.ItemID)
	if !found || !material.EquipmentRefineStone || !equipmentFound || rule.Quantity != 1 ||
		rule.ProbabilityPPM == 0 || rule.ProbabilityPPM > equipmentRecycleRefineDenominator || attempt == nil || attempt.operationID == "" {
		return Fail(p.CodeOperationConflict, "分解洗炼石物品目录或来源规则不可用")
	}
	for _, item := range receipt.Recycled {
		if !eligibleEquipmentRecycleRefine(item, rule, equipment) {
			continue
		}
		draw, err := attempt.draw(equipmentRecycleRefineRandomDomain, item.ID, equipmentRecycleRefineDenominator)
		if err != nil {
			return err
		}
		fact := RecycleRefineRoll{EquipmentInstanceID: item.ID, Numerator: rule.ProbabilityPPM, Denominator: equipmentRecycleRefineDenominator, Draw: draw}
		if draw < fact.Numerator {
			id := Hash([]byte(equipmentRecycleRefineRandomDomain + ":item:" + attempt.operationID + ":" + item.ID))[:32]
			stone := domain.Item{ID: id, ItemID: material.ID, Quantity: rule.Quantity, Version: 1, Bound: item.Bound,
				SourceKind: equipmentRecycleRefineSource, SourceID: item.ID}
			if !validEquipmentRecycleRefineStone(stone, material) || before.HasItemInstance(id) || out.HasItemInstance(id) {
				return Fail(p.CodeOperationConflict, "分解洗炼石身份与原库存不符")
			}
			if len(out.Items) >= out.Capacity {
				return Fail(p.CodeEquipmentBagFull, "分解后的背包没有洗炼石位置")
			}
			out.Items = append(out.Items, stone)
			fact.Stone = &stone
		}
		receipt.RefineRolls = append(receipt.RefineRolls, fact)
	}
	return nil
}

// refineProto 发布已经确认的原件和产物，不公开私有抽样数字。
func (r *EquipmentRecyclingReceipt) refineProto() *pb.RecycleRefineReceipt {
	if r == nil || len(r.RefineRolls) == 0 {
		return nil
	}
	result := &pb.RecycleRefineReceipt{Rolls: make([]*pb.RecycleRefineRoll, 0, len(r.RefineRolls))}
	for _, fact := range r.RefineRolls {
		view := &pb.RecycleRefineRoll{EquipmentInstanceId: domain.Bytes(fact.EquipmentInstanceID)}
		if fact.Stone != nil {
			view.Stone = fact.Stone.Proto()
		}
		result.Rolls = append(result.Rolls, view)
	}
	return result
}
