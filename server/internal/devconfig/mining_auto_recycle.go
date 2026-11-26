// 本文件将矿石自动回收范围关联到已有 NPC 矿石收购价。
package devconfig

import (
	"errors"
	"fmt"
	"slices"
)

// MiningAutoRecycleConfig 指定矿区允许自动回收的矿石及统一计价来源。
type MiningAutoRecycleConfig struct {
	ShopKey string   `json:"shop_key"` // 提供矿种纯度价格的原商店。
	ItemIDs []uint32 `json:"item_ids"` // 该商店已定价且明确允许自动回收的矿种。
}

// MiningAutoRecyclePrice 仅为矿区规则明确开放的矿种返回纯度价格。
func (f *Fixture) MiningAutoRecyclePrice(itemID uint32) (NPCMineralBuyback, bool) {
	if f == nil || f.Mining == nil || f.Mining.AutoRecycle == nil || !slices.Contains(f.Mining.AutoRecycle.ItemIDs, itemID) {
		return NPCMineralBuyback{}, false
	}
	shop := f.NPCShopByKey(f.Mining.AutoRecycle.ShopKey)
	if shop == nil {
		return NPCMineralBuyback{}, false
	}
	return shop.MineralBuybackFor(itemID)
}

// validateMiningAutoRecycle 核对矿种和商店的引用，防止通用收购价或工具误入自动回收。
func (f *Fixture) validateMiningAutoRecycle() error {
	rules := f.Mining.AutoRecycle
	if rules == nil {
		return nil
	}
	shop := f.NPCShopByKey(rules.ShopKey)
	if shop == nil || len(rules.ItemIDs) == 0 || len(rules.ItemIDs) > len(f.Mining.Ores) {
		return errors.New("矿石自动回收须指定收购商店和有效矿种")
	}
	seen := make(map[uint32]bool, len(rules.ItemIDs))
	for _, itemID := range rules.ItemIDs {
		ore := slices.ContainsFunc(f.Mining.Ores, func(ore MiningOre) bool { return ore.ItemID == itemID })
		price, priced := shop.MineralBuybackFor(itemID)
		if !ore || seen[itemID] || !priced || price.Numerator == 0 || price.Denominator == 0 ||
			uint64(price.Numerator)*100 < uint64(price.Denominator) {
			return fmt.Errorf("自动回收矿种%d须唯一并有可结算的纯度收购价", itemID)
		}
		seen[itemID] = true
	}
	return nil
}
