// 本文件定义勋章的连续成长费用和战斗属性贡献。
package devconfig

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
)

// MedalConfig 指定原背包碎片及从激活开始连续开放的成长档位。
type MedalConfig struct {
	FragmentItemID uint32             `json:"fragment_item_id"` // 奖励、背包和消费共用的碎片模板。
	Levels         []MedalLevelConfig `json:"levels"`           // 从一级开始按目标级排序，缺少的档位不开放。
}

// MedalLevelConfig 保存到达目标级时的耗料和该级累计固定属性。
type MedalLevelConfig struct {
	Level        uint32 `json:"level"`         // 一级为初次激活。
	FragmentCost int64  `json:"fragment_cost"` // 单次实际从原背包消费的碎片数。
	MainAttack   int64  `json:"main_attack"`   // 战士攻击、法师魔法或道士道术的区间端点增量。
	Defense      int64  `json:"defense"`       // 物理防御的固定点数。
}

// UnmarshalJSON 拒绝运行时未识别的勋章配置字段。
func (c *MedalConfig) UnmarshalJSON(data []byte) error {
	type fields MedalConfig
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	return decoder.Decode((*fields)(c))
}

// Level 查询已开放的目标级，零级表示角色尚未激活。
func (c *MedalConfig) Level(level uint32) (MedalLevelConfig, bool) {
	if c == nil || level == 0 || uint64(level) > uint64(len(c.Levels)) {
		return MedalLevelConfig{}, false
	}
	row := c.Levels[level-1]
	return row, row.Level == level
}

// ApplyMedalBonuses 把累计整数属性接入原职业战斗字段，普通战斗和挑战快照共用。
func (f *Fixture) ApplyMedalBonuses(a Fighter, level uint32) (Fighter, bool) {
	if level == 0 || f.Medal == nil {
		return a, true
	}
	row, found := f.Medal.Level(level)
	if !found {
		return Fighter{}, false
	}
	values := equipmentStats{7: row.Defense, 8: row.Defense, 9: row.Defense}
	switch a.JobID {
	case 0: // 战士的物理攻击。
		values[0], values[1] = row.MainAttack, row.MainAttack
	case 1: // 法师的魔法攻击。
		values[2], values[3] = row.MainAttack, row.MainAttack
	case 2: // 道士的道术攻击。
		values[4], values[5] = row.MainAttack, row.MainAttack
	default:
		return Fighter{}, false
	}
	return applyEquipmentStats(a, values)
}

// validateMedal 校验新成长档位和真实碎片，属性总和由装备与技能管线复核。
func (f *Fixture) validateMedal() error {
	rules := f.Medal
	if rules == nil {
		return nil
	}
	if f.RuleVersion < 30 || !f.DevelopmentOnly || f.RuntimeReady || rules.FragmentItemID == 0 ||
		len(rules.Levels) == 0 || uint64(len(rules.Levels)) >= math.MaxUint32 {
		return errors.New("勋章需使用显式开发候选、独立碎片和连续等级")
	}
	if err := f.validateMedalFragment(rules.FragmentItemID); err != nil {
		return err
	}
	var previous MedalLevelConfig
	for index, row := range rules.Levels {
		if row.Level != uint32(index+1) || row.FragmentCost <= 0 || row.FragmentCost > math.MaxInt32 ||
			row.MainAttack < previous.MainAttack || row.Defense < previous.Defense {
			return fmt.Errorf("勋章第 %d 级的碎片费用、累计属性或编号不合法", index+1)
		}
		previous = row
	}
	return nil
}

// validateMedalFragment 确认碎片与其他专项材料身份独立，可由原库存流程分堆和流转。
func (f *Fixture) validateMedalFragment(id uint32) error {
	count := 0
	for _, item := range f.Items {
		if item.ID != id {
			continue
		}
		count++
		if strings.TrimSpace(item.Name) == "" || !item.Stackable || item.MaxStack <= 0 || item.IsEquipment ||
			item.EquipmentSlot != 0 || item.RequiredLevel != nil || item.BookSkillID != 0 || item.IsSkillPage ||
			item.DropQuality != 0 || item.RecycleGold != 0 || item.OriginalDurabilityU != 0 ||
			item.EquipmentRecycleMaterial || item.EquipmentRefineStone {
			return errors.New("勋章碎片须是独立的可堆叠材料")
		}
	}
	if count != 1 || f.IsCastingSupply(id) || (f.Wings != nil && f.Wings.FeatherItemID == id) ||
		(f.LuckOil != nil && f.LuckOil.ItemID == id) || (f.EquipmentEnhancement != nil && f.EquipmentEnhancement.StoneItemID == id) ||
		(f.EquipmentRefine != nil && f.EquipmentRefine.StoneItemID == id) {
		return errors.New("勋章碎片不能缺失、重复或复用其他成长材料")
	}
	if _, forgeBlueprint := f.ForgeRecipe(id); forgeBlueprint {
		return errors.New("勋章碎片不能作为打造图纸")
	}
	for _, potion := range f.Potions.Effects {
		if potion.ItemID == id {
			return errors.New("勋章碎片不能同时作为药品使用")
		}
	}
	return nil
}
