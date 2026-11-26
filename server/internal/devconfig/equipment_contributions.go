// 本文件汇总普通装备基础值与部位强化贡献，统一处理战斗定点取整。
package devconfig

import "math"

const equipmentBasis = 10_000 // 单个属性点对应的基础属性基点数。

// equipmentStats 的位置顺序与原战斗单位的装备属性目标一一对应。
type equipmentStats [12]int64

// equipmentStats 读取装备模板中的可强化基础字段，实例词条及独立养成无入口。
func (i Item) equipmentStats() equipmentStats {
	return equipmentStats{
		i.AttackMinBonus, i.AttackMaxBonus, i.MagicMinBonus, i.MagicMaxBonus, i.TaoMinBonus, i.TaoMaxBonus,
		i.AttackSpeedBonus, i.DefenceBonus, i.DefenceMinBonus, i.DefenceMaxBonus, i.MagicDefenceMinBonus, i.MagicDefenceMaxBonus,
	}
}

// applyEquipmentStats 将已完成同类汇总的属性放入原战斗字段，并检查随机区间。
func applyEquipmentStats(a Fighter, values equipmentStats) (Fighter, bool) {
	targets := [...]*int64{
		&a.AttackMin, &a.AttackMax, &a.MagicMin, &a.MagicMax, &a.TaoMin, &a.TaoMax,
		&a.AttackSpeed, &a.Defence, &a.DefenceMin, &a.DefenceMax, &a.MagicDefenceMin, &a.MagicDefenceMax,
	}
	for index, target := range targets {
		value, valid := equipmentAdd(*target, values[index])
		if !valid {
			return Fighter{}, false
		}
		*target = value
	}
	for _, pair := range [...][2]int64{
		{a.AttackMin, a.AttackMax}, {a.MagicMin, a.MagicMax}, {a.TaoMin, a.TaoMax},
		{a.DefenceMin, a.DefenceMax}, {a.MagicDefenceMin, a.MagicDefenceMax},
	} {
		if pair[0] < 0 || pair[1] < pair[0] || pair[1]-pair[0] == math.MaxInt64 {
			return Fighter{}, false // 随机闭区间的长度还需要加一。
		}
	}
	return a, true
}

// EquipmentContributions 按战斗部位汇总原整数值和所有强化的定点分量。
type EquipmentContributions struct {
	base   equipmentStats // 各部位在强化前的整数属性。
	bonusU equipmentStats // 各部位强化后新增的属性基点，保留到应用阶段。
}

// Add 接纳一个有效穿戴模板；空部位、耐久耗尽及独立养成应在调用处过滤。
func (c *EquipmentContributions) Add(item Item, bonusBP uint32) bool {
	if bonusBP > maxEnhancementBonusBP {
		return false
	}
	values := item.equipmentStats()
	minimum, minimumOK := equipmentAdd(values[0], item.AttackBonus)
	maximum, maximumOK := equipmentAdd(values[1], item.AttackBonus)
	if !minimumOK || !maximumOK {
		return false
	}
	values[0], values[1] = minimum, maximum
	next := *c
	factor := int64(bonusBP)
	for index, value := range values {
		var valid bool
		next.base[index], valid = equipmentAdd(next.base[index], value)
		if !valid || (factor != 0 && (value > math.MaxInt64/factor || value < math.MinInt64/factor)) {
			return false
		}
		next.bonusU[index], valid = equipmentAdd(next.bonusU[index], value*factor)
		if !valid {
			return false
		}
	}
	*c = next
	return true
}

// Apply 只在同一属性所有强化来源累加完毕后截断一次，原基础值继续按整数入账。
func (c *EquipmentContributions) Apply(a Fighter) (Fighter, bool) {
	values := c.base
	for index, bonus := range c.bonusU {
		value, valid := equipmentAdd(values[index], bonus/equipmentBasis)
		if !valid {
			return Fighter{}, false
		}
		values[index] = value
	}
	return applyEquipmentStats(a, values)
}
