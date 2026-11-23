// 本文件区分施毒使用的两类原道具，并验证隔离规则中材料身份不会重叠。
package devconfig

import (
	"errors"
	"slices"
)

// PoisonColor 标记原药粉在命中后产生的毒色。
type PoisonColor uint8

const (
	PoisonNone  PoisonColor = iota // 道具不是这个施毒技能认可的药粉。
	PoisonGreen                    // 原灰色粉，产生持续扣血。
	PoisonRed                      // 原黄色粉，降低实际防御。
)

// PoisonMaterials 保存隔离配置明确许可且互斥的两种药粉定义号。
type PoisonMaterials struct {
	GreenItemIDs []uint32 `json:"green_item_ids"` // 可产生绿毒的原药粉定义号。
	RedItemIDs   []uint32 `json:"red_item_ids"`   // 可产生红毒的原药粉定义号。
}

// PoisonColorForMaterial 按定义号读取已确认色别，不从物品展示名或品质推断。
func (s Skill) PoisonColorForMaterial(itemID uint32) PoisonColor {
	if s.Behavior != "poisoning" || s.Poison == nil || itemID == 0 {
		return PoisonNone
	}
	if slices.Contains(s.Poison.GreenItemIDs, itemID) {
		return PoisonGreen
	}
	if slices.Contains(s.Poison.RedItemIDs, itemID) {
		return PoisonRed
	}
	return PoisonNone
}

// AcceptsMaterial 同时保留普通耗材的一对一门禁，并支持施毒自己的两色药粉白名单。
func (s Skill) AcceptsMaterial(itemID uint32) bool {
	if s.Behavior == "poisoning" {
		return s.PoisonColorForMaterial(itemID) != PoisonNone
	}
	return itemID != 0 && itemID == s.MaterialItemID
}

// validatePoisonMaterials 确认已选耗材均有唯一色别，且和持久层使用同一个专槽白名单。
func validatePoisonMaterials(s Skill, supplies map[uint32]bool) error {
	if s.Poison == nil || s.MaterialItemID != 0 || s.MaterialQuantity != 1 || len(s.Poison.GreenItemIDs) == 0 || len(s.Poison.RedItemIDs) == 0 {
		return errors.New("施毒需明确两色原药粉并每次消耗原实例的一个单位")
	}
	// seen 汇总两组色别，避免同一物品定义被重复或同时判为两色。
	seen := map[uint32]bool{}
	for _, ids := range [][]uint32{s.Poison.GreenItemIDs, s.Poison.RedItemIDs} {
		for _, id := range ids {
			if id == 0 || !supplies[id] || seen[id] {
				return errors.New("施毒药粉必须在专槽白名单中且各色不重复")
			}
			seen[id] = true
		}
	}
	return nil
}
