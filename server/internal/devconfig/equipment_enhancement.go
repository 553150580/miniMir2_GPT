// 本文件装载并校验装备部位强化的物品身份和逐级规则。
package devconfig

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
)

const (
	equipmentSlotCount               = 10    // 普通角色装备部位数量，独立养成及施法耗材另计。
	MaxEquipmentEnhancementStoneRefs = 50    // 单笔报价最多可列出的真实材料堆数。
	maxEnhancementLevel              = 50    // 当前工作簿提供的完整等级目录。
	maxEnhancementBonusBP            = 1_500 // 当前阶段的累计候选基点上限。
)

// EquipmentEnhancementConfig 保存独立消耗材料和连续的升级档位。
type EquipmentEnhancementConfig struct {
	StoneItemID  uint32                           `json:"stone_item_id"` // 原强化石物品模板。
	Levels       []EquipmentEnhancementLevel      `json:"levels"`        // 按当前级别依次排列的升档规则。
	bonusByLevel *[maxEnhancementLevel + 1]uint32 // 经校验的累计加成，战斗汇总直接查表。
}

// EquipmentEnhancementLevel 表示从一个实际槽位等级升到下一级的代价与效果。
type EquipmentEnhancementLevel struct {
	CurrentLevel uint32 `json:"current_level"` // 本次尝试之前的等级，初始为零。
	StoneCost    int64  `json:"stone_cost"`    // 成败都实际消耗的强化石数量。
	SuccessPPM   uint32 `json:"success_ppm"`   // 成功的百万分率。
	BonusBP      uint32 `json:"bonus_bp"`      // 本级成功才增加的装备基础属性基点。
}

// UnmarshalJSON 拒绝未知规则字段，确保工作簿投影和服务器使用相同的数据结构。
func (c *EquipmentEnhancementConfig) UnmarshalJSON(data []byte) error {
	type fields EquipmentEnhancementConfig
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	return decoder.Decode((*fields)(c))
}

// Level 返回某级尚可尝试的下一级规则，目录终点与功能关闭时无下一档。
func (c *EquipmentEnhancementConfig) Level(current uint32) (EquipmentEnhancementLevel, bool) {
	if c == nil || uint64(current) >= uint64(len(c.Levels)) || c.Levels[current].CurrentLevel != current {
		return EquipmentEnhancementLevel{}, false
	}
	return c.Levels[current], true
}

// BonusBP 汇总已经取得的等级对应的全部成功增量。
func (c *EquipmentEnhancementConfig) BonusBP(level uint32) uint32 {
	if c == nil || uint64(level) > uint64(len(c.Levels)) {
		return 0
	}
	if c.bonusByLevel != nil {
		return c.bonusByLevel[level]
	}
	total := uint32(0)
	for _, entry := range c.Levels[:level] {
		total += entry.BonusBP
	}
	return total
}

// validateEquipmentEnhancement 检查材料独占、连续规则和总预算，再由装备管线核验属性算术。
func (f *Fixture) validateEquipmentEnhancement() error {
	rules := f.EquipmentEnhancement
	if rules == nil {
		return nil
	}
	if f.RuleVersion < 22 || len(rules.Levels) != maxEnhancementLevel || rules.StoneItemID == 0 {
		return errors.New("部位强化需使用完整装备基础属性及 0 至 50 级连续规则")
	}
	bonus := new([maxEnhancementLevel + 1]uint32)
	for index, level := range rules.Levels {
		if level.CurrentLevel != uint32(index) || level.StoneCost <= 0 || level.StoneCost > math.MaxInt32 ||
			level.SuccessPPM == 0 || level.SuccessPPM > 1_000_000 ||
			level.BonusBP == 0 || level.BonusBP > maxEnhancementBonusBP-bonus[index] {
			return fmt.Errorf("部位强化 %d 级的费用、概率或基础属性预算不合法", index)
		}
		bonus[index+1] = bonus[index] + level.BonusBP
	}
	count := 0
	for _, item := range f.Items {
		if item.ID != rules.StoneItemID {
			continue
		}
		count++
		if strings.TrimSpace(item.Name) == "" || item.IsEquipment || item.EquipmentSlot != 0 || !item.Stackable ||
			item.MaxStack < 1 || item.BookSkillID != 0 || item.IsSkillPage || item.RecycleGold != 0 ||
			item.OriginalDurabilityU != 0 || item.EquipmentRecycleMaterial || f.IsCastingSupply(item.ID) ||
			(f.Wings != nil && item.ID == f.Wings.FeatherItemID) || (f.LuckOil != nil && item.ID == f.LuckOil.ItemID) {
			return errors.New("强化石须是原目录中独立的可堆叠材料")
		}
		for _, level := range rules.Levels {
			if (level.StoneCost-1)/item.MaxStack+1 > MaxEquipmentEnhancementStoneRefs {
				return errors.New("强化石费用超过单次可引用的原材料堆数")
			}
		}
	}
	if count != 1 {
		return errors.New("部位强化须引用唯一的原强化石模板")
	}
	for _, effect := range f.Potions.Effects {
		if effect.ItemID == rules.StoneItemID {
			return errors.New("强化石不能同时作为药品使用")
		}
	}
	rules.bonusByLevel = bonus
	return nil
}
