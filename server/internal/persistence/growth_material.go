// 本文件校验和扣除已明确选中的角色养成材料原堆。
package persistence

import (
	"math"

	"minimir/internal/devconfig"
	"minimir/internal/domain"
)

// validGrowthMaterial 检查原材料的身份、来源、可扣版本和占用，允许已登记的不同来源。
func validGrowthMaterial(item domain.Item, definition devconfig.Item) bool {
	return item.ItemID == definition.ID && item.Quantity > 0 && item.Quantity <= definition.MaxStack &&
		item.Version > 0 && item.Version < math.MaxInt64 && !item.Locked && !equipmentOccupied(item) &&
		item.SourceKind != "" && item.SourceID != "" && item.OrePurity == 0 && item.InitialLuck == 0 && item.CurrentLuck == 0 &&
		item.LuckOilToSix == 0 && item.LuckOilToNine == 0 && item.DurabilityU == 0 && item.MaxDurabilityU == 0 && item.OriginalDurabilityU == 0
}

// spendGrowthMaterials 应用已验证的原实例扣量。其他背包格保持原顺序、数量和版本。
func spendGrowthMaterials(before []domain.Item, uses map[string]int64) []domain.Item {
	after := make([]domain.Item, 0, len(before))
	for _, item := range before {
		if amount, used := uses[item.ID]; used {
			item.Quantity -= amount
			item.Version++
			if item.Quantity == 0 {
				continue
			}
		}
		after = append(after, item)
	}
	return after
}
