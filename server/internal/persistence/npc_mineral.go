// 本文件按当前商店的矿石目录和原实例纯度计算回收单价。
package persistence

import (
	"minimir/internal/devconfig"
	"minimir/internal/domain"
)

// vendorSaleUnitPrice 计算原实例的单件收购价。价格为零时拒绝出售并保留原物品。
func vendorSaleUnitPrice(fixture *devconfig.Fixture, shop *devconfig.NPCShop, item domain.Item) int64 {
	// price 是此店对此矿种的收购规则。未收购的矿石不能走通用固定价。
	price, wanted := shop.MineralBuybackFor(item.ItemID)
	if !wanted {
		if item.SourceKind == "mining" || item.OrePurity != 0 {
			return 0
		}
		return shop.SellPrice(item.ItemID)
	}
	// definition 用于区分可保存原纯度的单件矿石与其他物品。
	definition, exists := fixture.Item(item.ItemID)
	if !exists || !validSingleMiningOre(item, definition) || item.OriginalDurabilityU != 0 ||
		item.MaxDurabilityU != 0 || item.DurabilityU != 0 || price.Numerator == 0 || price.Denominator == 0 {
		return 0
	}
	// 纯度最多为 100，分子为无符号 32 位。先乘后除，逐块取整，不经过浮点或权益倍率。
	product := int64(item.OrePurity) * int64(price.Numerator)
	divisor := int64(price.Denominator)
	switch price.Rounding {
	case "floor_per_ore":
		return product / divisor
	case "exact":
		if product%divisor == 0 {
			return product / divisor
		}
	}
	return 0
}
