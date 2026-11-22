// 本文件解析耐久存储尺度、磨损类型和修理计算所需参数。

package devconfig

import (
	"errors"
	"math"
)

const (
	// RepairOrdinary 规定修理“ordinary”的初始取值。
	RepairOrdinary uint32 = 1
	// RepairSpecial 规定修理特殊的初始取值。
	RepairSpecial uint32 = 2
	// WeaponSlot 规定“weapon”槽位的初始取值。
	WeaponSlot uint32 = 0
)

// EquipmentDurabilityRules 保存装备耐久规则集合所需的状态。
type EquipmentDurabilityRules struct {
	// WeaponLossU 记录“weapon”损失u，使用数值表示。
	WeaponLossU int64 `json:"weapon_loss_u"`
	// ArmorHitLossU 记录防具命中损失u，使用数值表示。
	ArmorHitLossU int64 `json:"armor_hit_loss_u"`
	// DeathLossU 记录死亡损失u，使用数值表示。
	DeathLossU int64 `json:"death_loss_u"`
	// RepairRangeCells 记录修理范围地图格集合，使用数值表示。
	RepairRangeCells int `json:"repair_range_cells"`
	// Ordinary 记录“ordinary”，使用修理规则表示。
	Ordinary RepairRule `json:"ordinary"`
	// Special 记录特殊，使用修理规则表示。
	Special *RepairRule `json:"special,omitempty"`
}

// RepairRule 保存修理规则所需的状态。
type RepairRule struct {
	// Kind 记录类型，使用数值表示。
	Kind uint32 `json:"kind"`
	// UpperLossDivisor 记录较高值损失“divisor”，使用数值表示。
	UpperLossDivisor int64 `json:"upper_loss_divisor,omitempty"`
	// MinMaxDurabilityU 记录最小最大耐久u，使用数值表示。
	MinMaxDurabilityU int64 `json:"min_max_durability_u,omitempty"`
	// FullRepairCostU 记录完整或满额修理消耗u，使用数值表示。
	FullRepairCostU int64 `json:"full_repair_cost_u,omitempty"`
}

// Repair 处理修理。
// r 是调用该方法的装备耐久规则集合实例，kind 记录类型，使用数值表示。
func (r *EquipmentDurabilityRules) Repair(kind uint32) (RepairRule, bool) {
	if r == nil {
		return RepairRule{}, false
	}
	switch kind {
	case RepairOrdinary:
		return r.Ordinary, validRepairRule(r.Ordinary, RepairOrdinary)
	case RepairSpecial:
		if r.Special != nil {
			return *r.Special, validRepairRule(*r.Special, RepairSpecial)
		}
	}
	return RepairRule{}, false
}

// validRepairRule 判断有效修理规则是否满足要求。
// rule 记录规则，使用修理规则表示，kind 记录类型，使用数值表示。
func validRepairRule(rule RepairRule, kind uint32) bool {
	if rule.Kind != kind {
		return false
	}
	if kind == RepairOrdinary {
		return rule.UpperLossDivisor > 1 && rule.MinMaxDurabilityU > 0 && rule.FullRepairCostU == 0
	}
	return kind == RepairSpecial && rule.UpperLossDivisor == 0 && rule.MinMaxDurabilityU == 0 && rule.FullRepairCostU > 0
}

// ValidateEquipmentDurability 校验装备耐久。
// f 是调用该方法的开发测试配置实例。
func (f *Fixture) ValidateEquipmentDurability() error {
	// rules 保存装备耐久，用于当前步骤中的规则集合。
	rules := f.EquipmentDurability
	if rules == nil {
		return nil
	}
	if rules.WeaponLossU <= 0 || rules.ArmorHitLossU <= 0 || rules.DeathLossU <= 0 || rules.RepairRangeCells <= 0 || rules.RepairRangeCells > 8 ||
		!validRepairRule(rules.Ordinary, RepairOrdinary) || rules.Special != nil && !validRepairRule(*rules.Special, RepairSpecial) {
		return errors.New("装备磨损和修理配置无效")
	}
	// item 接收遍历道具集合时当前的元素。
	for _, item := range f.Items {
		if item.IsEquipment {
			if item.Stackable || item.OriginalDurabilityU < rules.Ordinary.MinMaxDurabilityU || item.OriginalDurabilityU > math.MaxInt32 || item.RepairBasePrice <= 0 {
				return errors.New("耐久装备须配置可修理的原始上限和满损基础费用")
			}
		} else if item.OriginalDurabilityU != 0 || item.RepairBasePrice != 0 {
			return errors.New("普通道具不能配置装备耐久或修理费")
		}
	}
	return nil
}
