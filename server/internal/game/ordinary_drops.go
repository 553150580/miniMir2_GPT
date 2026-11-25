// 本文件在普通怪死亡时冻结对应掉落池内的候选条目和数量。
package game

import "minimir/internal/devconfig"

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
	return true
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
