// 本文件校验手动分解的原奖励物品和精确概率分母。
package devconfig

import (
	"bytes"
	"encoding/json"
	"errors"
)

// EquipmentRecycleOilConfig 以一件装备的当前幸运为分子，读取工作簿的概率分母。
type EquipmentRecycleOilConfig struct {
	Denominator uint32 `json:"denominator"` // 分解抽样的完整整数区间。
}

// UnmarshalJSON 拒绝未知奖励参数，避免把未决渠道或价格当成有效设置。
func (c *EquipmentRecycleOilConfig) UnmarshalJSON(data []byte) error {
	type fields EquipmentRecycleOilConfig
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	return decoder.Decode((*fields)(c))
}

// validateEquipmentRecycleOil 要求分解使用与武器培养相同的原材料目录。
func (f *Fixture) validateEquipmentRecycleOil() error {
	if f.EquipmentRecycleOil == nil {
		return nil
	}
	if f.EquipmentRecycleOil.Denominator == 0 || f.LuckOil == nil || f.EquipmentRecycling == nil || f.RuleVersion < 19 {
		return errors.New("分解祝福油需要有效概率、祝福油目录和装备回收规则")
	}
	item, exists := f.Item(f.LuckOil.ItemID)
	if !exists || !item.EquipmentRecycleMaterial {
		return errors.New("分解产油必须引用已分配该来源的原祝福油物品")
	}
	return nil
}
