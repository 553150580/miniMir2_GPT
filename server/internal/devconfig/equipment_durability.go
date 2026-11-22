package devconfig

import (
	"errors"
	"math"
)

const (
	RepairOrdinary uint32 = 1
	RepairSpecial  uint32 = 2
	WeaponSlot     uint32 = 0
)

type EquipmentDurabilityRules struct {
	WeaponLossU      int64       `json:"weapon_loss_u"`
	ArmorHitLossU    int64       `json:"armor_hit_loss_u"`
	DeathLossU       int64       `json:"death_loss_u"`
	RepairRangeCells int         `json:"repair_range_cells"`
	Ordinary         RepairRule  `json:"ordinary"`
	Special          *RepairRule `json:"special,omitempty"`
}

type RepairRule struct {
	Kind              uint32 `json:"kind"`
	UpperLossDivisor  int64  `json:"upper_loss_divisor,omitempty"`
	MinMaxDurabilityU int64  `json:"min_max_durability_u,omitempty"`
	FullRepairCostU   int64  `json:"full_repair_cost_u,omitempty"`
}

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

func validRepairRule(rule RepairRule, kind uint32) bool {
	if rule.Kind != kind {
		return false
	}
	if kind == RepairOrdinary {
		return rule.UpperLossDivisor > 1 && rule.MinMaxDurabilityU > 0 && rule.FullRepairCostU == 0
	}
	return kind == RepairSpecial && rule.UpperLossDivisor == 0 && rule.MinMaxDurabilityU == 0 && rule.FullRepairCostU > 0
}

func (f *Fixture) ValidateEquipmentDurability() error {
	rules := f.EquipmentDurability
	if rules == nil {
		return nil
	}
	if rules.WeaponLossU <= 0 || rules.ArmorHitLossU <= 0 || rules.DeathLossU <= 0 || rules.RepairRangeCells <= 0 || rules.RepairRangeCells > 8 ||
		!validRepairRule(rules.Ordinary, RepairOrdinary) || rules.Special != nil && !validRepairRule(*rules.Special, RepairSpecial) {
		return errors.New("装备磨损和修理配置无效")
	}
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
