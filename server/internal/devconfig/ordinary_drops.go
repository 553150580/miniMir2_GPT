// 本文件定义普通怪物各掉落池的候选条目和跨表校验。
package devconfig

import (
	"errors"
	"fmt"
	"math"
)

// OrdinaryDropEntry 记录同一类别内的相对权重与单次掉落数量范围。
type OrdinaryDropEntry struct {
	ItemID      uint32 `json:"item_id"`      // 实际进入背包或参与回收的物品模板。
	Weight      int64  `json:"weight"`       // 相对于同类别其他条目的抽取权重。
	QuantityMin int64  `json:"quantity_min"` // 选中条目后的最小数量。
	QuantityMax int64  `json:"quantity_max"` // 选中条目后的最大数量。
}

// OrdinaryDropPool 将同组普通怪的道具、装备子池归入同一配置身份。
type OrdinaryDropPool struct {
	ID        uint32              `json:"id"`        // 怪物模板引用的稳定掉落池编号。
	Items     []OrdinaryDropEntry `json:"items"`     // 命中道具分支后抽取药品、材料或技能书。
	Equipment []OrdinaryDropEntry `json:"equipment"` // 命中装备分支或装备保底时抽取装备。
}

// OrdinaryDropPool 按怪物配置中的编号查找其普通掉落池。
func (f *Fixture) OrdinaryDropPool(id uint32) (OrdinaryDropPool, bool) {
	for _, pool := range f.OrdinaryDropPools {
		if pool.ID == id {
			return pool, true
		}
	}
	return OrdinaryDropPool{}, false
}

// validateOrdinaryDropPools 验证普通怪物到掉落池及物品定义的完整引用。
func (f *Fixture) validateOrdinaryDropPools() error {
	usedQuotas, err := f.validateOrdinaryEquipmentQuotas()
	if err != nil {
		return err
	}
	if len(f.OrdinaryDropPools) == 0 {
		if len(usedQuotas) != 0 {
			return errors.New("普通怪装备额度没有可关联的掉落池")
		}
		if f.RuleVersion >= 30 {
			return errors.New("规则30的普通怪物必须使用各模板绑定的掉落池")
		}
		for _, monster := range f.Monsters {
			if monster.DropPoolID != 0 {
				return fmt.Errorf("普通怪物%d引用了未定义的掉落池%d", monster.ID, monster.DropPoolID)
			}
		}
		return nil
	}
	if f.DamageFormula != "development_source_combat" || f.CombatTiming.PlayerAttackActionMS <= 0 {
		return errors.New("普通怪物专属掉落池需要统一战斗与击杀时间线")
	}
	used := make(map[uint32]bool, len(f.OrdinaryDropPools))
	for _, pool := range f.OrdinaryDropPools {
		if pool.ID == 0 {
			return errors.New("普通掉落池编号必须为正")
		}
		if _, duplicated := used[pool.ID]; duplicated {
			return fmt.Errorf("普通掉落池%d重复", pool.ID)
		}
		used[pool.ID] = false
		if err := f.validateOrdinaryDropEntries(pool.ID, pool.Items, false); err != nil {
			return err
		}
		if err := f.validateOrdinaryDropEntries(pool.ID, pool.Equipment, true); err != nil {
			return err
		}
		if err := f.validateOrdinaryPoolQuotaLinks(pool, usedQuotas); err != nil {
			return err
		}
	}
	for _, monster := range f.Monsters {
		if _, exists := used[monster.DropPoolID]; !exists {
			return fmt.Errorf("普通怪物%d缺少有效的掉落池", monster.ID)
		}
		used[monster.DropPoolID] = true
	}
	for id, selected := range used {
		if !selected {
			return fmt.Errorf("普通掉落池%d未被怪物引用", id)
		}
	}
	for key, selected := range usedQuotas {
		if !selected {
			return fmt.Errorf("阶段%d品质%d的普通怪装备额度未被任何掉落池使用", key.stage, key.quality)
		}
	}
	return nil
}

// validateOrdinaryDropEntries 校验分类、单次数量和当前放行的装备品质范围。
func (f *Fixture) validateOrdinaryDropEntries(poolID uint32, rows []OrdinaryDropEntry, equipment bool) error {
	if len(rows) == 0 {
		return fmt.Errorf("普通掉落池%d的道具和装备子池都须有条目", poolID)
	}
	seen := make(map[uint32]bool, len(rows))
	var total int64
	for _, row := range rows {
		item, exists := f.Item(row.ItemID)
		if !exists || item.IsEquipment != equipment || seen[row.ItemID] {
			return fmt.Errorf("普通掉落池%d的物品%d不存在、分类错误或重复", poolID, row.ItemID)
		}
		if row.Weight <= 0 || total > math.MaxInt64-row.Weight {
			return fmt.Errorf("普通掉落池%d的权重必须为有界正整数", poolID)
		}
		if row.QuantityMin <= 0 || row.QuantityMax < row.QuantityMin ||
			(!item.Stackable && row.QuantityMax != 1) || (item.Stackable && row.QuantityMax > item.MaxStack) {
			return fmt.Errorf("普通掉落池%d的物品%d数量与堆叠规则不符", poolID, row.ItemID)
		}
		if equipment && (row.QuantityMax != 1 || item.DropQuality > 3) {
			return fmt.Errorf("普通掉落池%d只允许单件非传说装备", poolID)
		}
		seen[row.ItemID] = true
		total += row.Weight
	}
	return nil
}
