// 本文件定义角色专属翅膀的阶段、羽毛和成长前置条件。
package devconfig

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// WingConfig 保存已确认的阶数结构，以及有实际费用条目的可开放阶段。
type WingConfig struct {
	FeatherItemID   uint32           `json:"feather_item_id"`          // 实际由背包消费的原羽毛。
	MaxRank         uint32           `json:"max_rank"`                 // 玩法规定的最高阶。
	AppearanceTiers []uint32         `json:"appearance_tiers"`         // 到达对应阶时自动解锁的外观。
	Ranks           []WingRankConfig `json:"ranks"`                    // 从激活开始连续开放，缺失阶段不能报价。
	AllowLuckOil    bool             `json:"allow_luck_oil,omitempty"` // 显式开放独立翅膀用油。
}

// WingRankConfig 表示到达某个目标阶段所需的羽毛数量。
type WingRankConfig struct {
	Rank     uint32 `json:"rank"`     // 激活的目标为一阶，后续逐阶递增。
	Feathers int64  `json:"feathers"` // 实际扣除的原羽毛数量。
}

// UnmarshalJSON 拒绝未知配置，数值与玩法表的投影一一对应。
func (c *WingConfig) UnmarshalJSON(data []byte) error {
	type fields WingConfig
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	return decoder.Decode((*fields)(c))
}

// validateWings 校验翅膀的羽毛物品和连续费用，无完整配置的后续阶段保持关闭。
func (f *Fixture) validateWings() error {
	wings := f.Wings
	if wings == nil {
		return nil
	}
	if f.CombatLuck == nil || wings.MaxRank == 0 || len(wings.Ranks) == 0 || uint64(len(wings.Ranks)) > uint64(wings.MaxRank) ||
		len(wings.AppearanceTiers) == 0 || wings.AppearanceTiers[0] != 1 {
		return errors.New("翅膀需接入幸运规则，并从一阶起提供连续费用与初始外观")
	}
	if wings.AllowLuckOil && f.LuckOil == nil {
		return errors.New("翅膀用油须与武器共享已校验的祝福油规则")
	}
	for index, tier := range wings.AppearanceTiers {
		if tier > wings.MaxRank || (index > 0 && tier <= wings.AppearanceTiers[index-1]) {
			return errors.New("翅膀外观解锁阶段须唯一且递增")
		}
	}
	for index, entry := range wings.Ranks {
		if entry.Rank != uint32(index+1) || entry.Feathers <= 0 {
			return fmt.Errorf("翅膀 %d 阶未提供连续的正羽毛费用", index+1)
		}
	}
	count := 0
	for _, item := range f.Items {
		if item.ID != wings.FeatherItemID {
			continue
		}
		count++
		if item.ID == 0 || strings.TrimSpace(item.Name) == "" || item.IsEquipment || item.EquipmentSlot != 0 ||
			!item.Stackable || item.MaxStack < 1 || item.BookSkillID != 0 || item.IsSkillPage || item.RecycleGold != 0 ||
			item.OriginalDurabilityU != 0 || item.EquipmentRecycleMaterial || (f.LuckOil != nil && item.ID == f.LuckOil.ItemID) {
			return errors.New("翅膀羽毛必须是原目录中独立的可堆叠材料")
		}
	}
	if count != 1 {
		return errors.New("翅膀须引用唯一的原羽毛模板")
	}
	return nil
}
