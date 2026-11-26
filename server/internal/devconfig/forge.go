// 本文件定义图纸打造目录，并校验黑铁、图纸和实际装备模板的身份。
package devconfig

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
)

// ForgeConfig 保存打造配方、黑铁矿及成功后初始幸运的来源配置。
type ForgeConfig struct {
	OreItemID        uint32        `json:"ore_item_id"`       // 必须是当前挖矿目录的黑铁矿物品编号。
	GuaranteedPurity uint32        `json:"guaranteed_purity"` // 平均纯度达到此值时，装备打造必成。
	LuckPPM          uint32        `json:"luck_ppm"`          // 成功装备附带初始幸运的百万分率。
	LuckWeights      []uint32      `json:"luck_weights"`      // 依次为初始幸运 1、2、3 的条件权重。
	Recipes          []ForgeRecipe `json:"recipes"`           // 可开启的原图纸与指定产出。
}

// ForgeRecipe 按原图纸确定装备模板、固定品质及一次打造的材料和金币。
type ForgeRecipe struct {
	BlueprintItemID uint32 `json:"blueprint_item_id"` // 提交时必须持有的原图纸模板。
	OutputItemID    uint32 `json:"output_item_id"`    // 成功后生成的实际装备模板。
	OreCount        uint32 `json:"ore_count"`         // 成功和失败都会消耗的单块黑铁矿数量。
	Quality         uint32 `json:"quality"`           // 配方固定品质，应与图纸和成品目录一致。
	Gold            int64  `json:"gold"`              // 本配方的单次整数金币费用。
}

// UnmarshalJSON 拒绝拼写错误的配置键进入打造规则。
func (c *ForgeConfig) UnmarshalJSON(data []byte) error {
	type fields ForgeConfig
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	return decoder.Decode((*fields)(c))
}

// ForgeRecipe 按原图纸编号查找已配置的配方。缺省规则不可打造。
func (f *Fixture) ForgeRecipe(blueprintItemID uint32) (ForgeRecipe, bool) {
	if f != nil && f.Forge != nil {
		for _, recipe := range f.Forge.Recipes {
			if recipe.BlueprintItemID == blueprintItemID {
				return recipe, true
			}
		}
	}
	return ForgeRecipe{}, false
}

// validateForge 约束固定品质、装备等级与部位，避免误用常规装备或其他矿种。
func (f *Fixture) validateForge() error {
	forge := f.Forge
	if forge == nil {
		return nil
	}
	if forge.OreItemID == 0 || forge.GuaranteedPurity == 0 || forge.GuaranteedPurity > 100 ||
		forge.LuckPPM > 1_000_000 || len(forge.LuckWeights) != 3 || len(forge.Recipes) == 0 || f.Mining == nil {
		return errors.New("图纸打造缺少有效的黑铁矿、必成阈值、初始幸运或配方")
	}
	var totalWeight uint64
	for _, weight := range forge.LuckWeights {
		if weight == 0 {
			return errors.New("图纸打造的三个初始幸运权重必须为正")
		}
		totalWeight += uint64(weight)
	}
	if totalWeight > math.MaxUint32 {
		return errors.New("图纸打造初始幸运总权重超出抽样范围")
	}
	blackIron := false
	for _, ore := range f.Mining.Ores {
		if ore.Key == "black_iron" && ore.ItemID == forge.OreItemID {
			blackIron = true
			break
		}
	}
	if !blackIron {
		return errors.New("图纸打造材料必须引用当前矿区的黑铁矿")
	}
	return f.validateForgeRecipes()
}

// validateForgeRecipes 要求每张图纸与成品通过唯一合法目录定义。
func (f *Fixture) validateForgeRecipes() error {
	items := make(map[uint32]Item, len(f.Items))
	for _, item := range f.Items {
		if _, exists := items[item.ID]; exists {
			items[item.ID] = Item{} // 重复物品编号不能进入打造配方。
		} else {
			items[item.ID] = item
		}
	}
	used := map[uint32]bool{f.Forge.OreItemID: true}
	for _, tool := range f.Mining.Tools {
		used[tool.ItemID] = true
	}
	for _, ore := range f.Mining.Ores {
		used[ore.ItemID] = true
	}
	for _, potion := range f.Potions.Effects {
		used[potion.ItemID] = true
	}
	for _, recipe := range f.Forge.Recipes {
		if recipe.OreCount == 0 || recipe.OreCount > 8 || recipe.Quality > 4 || recipe.Gold <= 0 || recipe.BlueprintItemID == recipe.OutputItemID ||
			used[recipe.BlueprintItemID] || used[recipe.OutputItemID] || !validForgeBlueprint(items[recipe.BlueprintItemID], recipe.Quality) ||
			!validForgeOutput(items[recipe.OutputItemID], recipe.Quality) {
			return fmt.Errorf("图纸打造配方 %d 的图纸、成品、耗材或金币无效", recipe.BlueprintItemID)
		}
		used[recipe.BlueprintItemID], used[recipe.OutputItemID] = true, true
	}
	return nil
}

// validForgeBlueprint 只允许单张非装备图纸，不套用药品和技能书的定义。
func validForgeBlueprint(item Item, quality uint32) bool {
	return item.ID != 0 && strings.TrimSpace(item.Name) != "" && !item.IsEquipment && item.EquipmentSlot == 0 && !item.Stackable &&
		item.MaxStack == 1 && item.BookSkillID == 0 && !item.IsSkillPage && item.RecycleGold == 0 && item.OriginalDurabilityU == 0 && item.DropQuality == quality
}

// validForgeOutput 检查开放的装备等级、标准部位和可穿戴职业。
func validForgeOutput(item Item, quality uint32) bool {
	if item.ID == 0 || strings.TrimSpace(item.Name) == "" || !item.IsEquipment || item.EquipmentSlot > 9 || item.Stackable ||
		item.BookSkillID != 0 || item.IsSkillPage || item.OriginalDurabilityU <= 0 || item.DropQuality != quality || item.RequiredLevel == nil {
		return false
	}
	switch *item.RequiredLevel {
	case 20, 30, 40, 50:
	default:
		return false
	}
	if item.EquipmentStage != (*item.RequiredLevel-1)/10 || len(item.AllowedJobs) == 0 || len(item.AllowedJobs) > 3 {
		return false
	}
	var jobs [3]bool
	for _, job := range item.AllowedJobs {
		if job > 2 || jobs[job] {
			return false
		}
		jobs[job] = true
	}
	return true
}
