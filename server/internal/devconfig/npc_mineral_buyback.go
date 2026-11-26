// 本文件定义矿石逐块收购的配置和矿种引用检查。
package devconfig

import (
	"fmt"
	"slices"
)

// NPCMineralBuyback 指定一家商店接收的矿种和每点纯度的整数比价。
type NPCMineralBuyback struct {
	ItemID      uint32 `json:"item_id"`     // 矿种对应的独立物品模板编号。
	Numerator   uint32 `json:"numerator"`   // 每点纯度参考金币价格的分子。
	Denominator uint32 `json:"denominator"` // 每点纯度参考金币价格的分母。
	Rounding    string `json:"rounding"`    // exact 使用整数单价，floor_per_ore 逐块向下取整。
}

// MineralBuybackFor 按物品模板查询本商店明确开放的矿石收购价。
func (s *NPCShop) MineralBuybackFor(itemID uint32) (NPCMineralBuyback, bool) {
	for _, price := range s.MineralBuyback {
		if price.ItemID == itemID {
			return price, true
		}
	}
	return NPCMineralBuyback{}, false
}

// validateMineralBuyback 要求矿种来自本规则的矿区配置，防止重叠报价绕开纯度计价。
func (f *Fixture) validateMineralBuyback(shop NPCShop) error {
	if len(shop.MineralBuyback) == 0 {
		return nil
	}
	if f.Mining == nil {
		return fmt.Errorf("城镇商店%s的矿石收购缺少矿种配置", shop.Key)
	}
	seen := make(map[uint32]bool, len(shop.MineralBuyback))
	for _, price := range shop.MineralBuyback {
		knownOre := slices.ContainsFunc(f.Mining.Ores, func(ore MiningOre) bool { return ore.ItemID == price.ItemID })
		if !knownOre || price.ItemID == 0 || seen[price.ItemID] || shop.SellPrice(price.ItemID) != 0 {
			return fmt.Errorf("城镇商店%s的矿石收购物品%d须对应唯一矿种且不能配置通用收购价", shop.Key, price.ItemID)
		}
		if price.Numerator == 0 || price.Denominator == 0 ||
			(price.Rounding != "floor_per_ore" && (price.Rounding != "exact" || price.Denominator != 1)) {
			return fmt.Errorf("城镇商店%s的矿石%d收购比价或取整方式无效", shop.Key, price.ItemID)
		}
		seen[price.ItemID] = true
	}
	return nil
}
