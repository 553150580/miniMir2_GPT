// 本文件在普通怪死亡时冻结对应掉落池内的候选条目和数量。
package game

import (
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	"minimir/internal/persistence"
)

// captureOrdinaryDrop 在保底仍待按来源水位结算时，同时保存互斥两类物品候选。
func (w *worker) captureOrdinaryDrop(fact *killFact) bool {
	f := &w.e.Store.Cfg.Fixture
	if len(f.OrdinaryDropPools) == 0 {
		// 旧规则持续使用原全局等权配置，每次拾取一个单位。
		if ids := f.Drops.EquipmentItemIDs; len(ids) > 0 {
			fact.EquipmentID = ids[w.draw(int64(len(ids)))]
		}
		if ids := f.Drops.ItemIDs; len(ids) > 0 {
			fact.ItemID = ids[w.draw(int64(len(ids)))]
			fact.ItemQuantity = 1
		}
		return true
	}
	pool, exists := f.OrdinaryDropPool(fact.Monster.DropPoolID)
	if !exists {
		return false
	}
	equipment := w.weightedOrdinaryDrop(pool.Equipment)
	item := w.weightedOrdinaryDrop(pool.Items)
	if equipment.ItemID == 0 || item.ItemID == 0 {
		return false
	}
	fact.EquipmentID, fact.ItemID = equipment.ItemID, item.ItemID
	fact.ItemQuantity = w.interval(item.QuantityMin, item.QuantityMax)
	if len(f.OrdinaryEquipmentQuotas) == 0 {
		return true
	}
	return w.captureEquipmentDowngrades(f, fact, pool)
}

// captureEquipmentDowngrades 按每档的显式权重逐级冻结装备候选，数据库重试不重新抽取。
func (w *worker) captureEquipmentDowngrades(f *devconfig.Fixture, fact *killFact, pool devconfig.OrdinaryDropPool) bool {
	item, found := f.Item(fact.EquipmentID)
	if !found || item.DropQuality > 3 {
		return false
	}
	stage := item.EquipmentStage
	for quality := item.DropQuality; quality > 1; {
		quota, valid := f.OrdinaryEquipmentQuota(stage, quality)
		if !valid {
			return false
		}
		lower := w.weightedDowngradeQuality(quality, quota.ExhaustedDowngradeWeights)
		if lower >= quality {
			return false
		}
		quality = lower
		next := w.weightedOrdinaryDrop(f.OrdinaryEquipmentCandidates(pool, stage, quality))
		if next.ItemID == 0 {
			return false
		}
		fact.EquipmentDowngrades = append(fact.EquipmentDowngrades, frozenEquipmentDrop{ItemID: next.ItemID, Quality: quality})
	}
	return true
}

// weightedDowngradeQuality 以稳定的低到高顺序抽选低于当前品质的显式档位。
func (w *worker) weightedDowngradeQuality(current uint32, weights map[uint32]int64) uint32 {
	var total int64
	for _, weight := range weights {
		total += weight
	}
	roll := w.draw(total)
	for quality := uint32(0); quality < current; quality++ {
		if roll < weights[quality] {
			return quality
		}
		roll -= weights[quality]
	}
	return current
}

// frozenEquipmentDrop 记录较低品质的实际装备定义，所有分支共享同一个原掉落实例。
type frozenEquipmentDrop struct {
	ItemID  uint32 // 所选装备定义编号。
	Quality uint32 // 由原装备定义确认的品质。
}

// ordinaryEquipmentPlan 把已冻结的降档路径和原掉落的来源、绑定合并。
func ordinaryEquipmentPlan(item domain.Item, lower []frozenEquipmentDrop) *persistence.OrdinaryEquipmentPlan {
	plan := &persistence.OrdinaryEquipmentPlan{Fallbacks: make([]domain.Item, 0, len(lower))}
	for _, row := range lower {
		alternative := item
		alternative.ItemID, alternative.Quality = row.ItemID, row.Quality
		plan.Fallbacks = append(plan.Fallbacks, alternative)
	}
	return plan
}

// confirmedSettlementLoot 只把已入包的物品映射成原场景战利品，额度降级时使用实际确认的装备。
func confirmedSettlementLoot(q persistence.Settlement, op persistence.Operation) (*domain.Item, bool) {
	if q.OrdinaryEquipment == nil {
		if op.Settlement != nil && !op.Settlement.GrantedItem {
			return nil, true
		}
		if op.Settlement != nil && op.Settlement.LuckyDrop != nil {
			actual := op.Settlement.LuckyDrop
			if q.Item == nil || op.Kind != "settlement" || op.SourceSeq != q.SourceSeq ||
				actual.InitialLuck < 1 || actual.InitialLuck > 3 || actual.ID != q.Item.ID || actual.ItemID != q.Item.ItemID ||
				actual.Quality != q.Item.Quality || actual.Bound != q.Item.Bound || actual.Quantity != q.Item.Quantity ||
				actual.SourceKind != q.Item.SourceKind || actual.SourceID != q.Item.SourceID {
				return nil, false
			}
			return actual, true
		}
		return q.Item, true
	}
	if q.Item == nil || op.Kind != "settlement" || op.SourceSeq != q.SourceSeq || op.Settlement == nil || op.Settlement.OrdinaryEquipment == nil {
		return nil, false
	}
	// actual 来自事务中真正生效的装备候选，包含已确认为该实例的完整物品属性。
	actual := &op.Settlement.OrdinaryEquipment.Item
	if actual.ID != q.Item.ID || actual.Bound != q.Item.Bound || actual.SourceKind != q.Item.SourceKind || actual.SourceID != q.Item.SourceID || actual.Quantity != q.Item.Quantity {
		return nil, false
	}
	if actual.ItemID != q.Item.ItemID || actual.Quality != q.Item.Quality {
		matches := false
		for _, candidate := range q.OrdinaryEquipment.Fallbacks {
			matches = matches || actual.ItemID == candidate.ItemID && actual.Quality == candidate.Quality
		}
		if !matches {
			return nil, false
		}
	}
	if !op.Settlement.GrantedItem {
		return nil, true
	}
	return actual, true
}

// weightedOrdinaryDrop 在已通过权重上界校验的条目中进行一次等比例抽选。
func (w *worker) weightedOrdinaryDrop(entries []devconfig.OrdinaryDropEntry) devconfig.OrdinaryDropEntry {
	var total int64
	for _, entry := range entries {
		total += entry.Weight
	}
	roll := w.draw(total)
	for _, entry := range entries {
		if roll < entry.Weight {
			return entry
		}
		roll -= entry.Weight
	}
	return devconfig.OrdinaryDropEntry{}
}
