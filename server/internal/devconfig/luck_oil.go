// 本文件校验祝福油的物品身份、逐级失败回退及两段保底结构。
package devconfig

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
)

// LuckOilConfig 选择实际消耗的祝福油物品及唯一工作簿的用油档位。
type LuckOilConfig struct {
	ItemID uint32        `json:"item_id"` // 每次消耗一瓶的统一物品编号。
	Steps  []LuckOilStep `json:"steps"`   // 按当前幸运 0 至 8 顺序保存。
}

// LuckOilStep 确定一瓶祝福油对应的概率、失败去向和本段累计条件。
type LuckOilStep struct {
	SuccessPPM  uint32 `json:"success_ppm"`  // 本档未达保底时的成功百万分率。
	FailureLuck uint32 `json:"failure_luck"` // 失败后返回的幸运等级。
	PityAfter   uint32 `json:"pity_after"`   // 先前累计达到此值后的下一瓶必成，0 表示该档无保底。
}

// UnmarshalJSON 拒绝未定义的字段，避免用油条款因拼写错误静默生效。
func (c *LuckOilConfig) UnmarshalJSON(data []byte) error {
	type fields LuckOilConfig
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	return decoder.Decode((*fields)(c))
}

// validateLuckOil 要求每一瓶均沿当前幸运逐档前进或回到原阶段底部。
func (f *Fixture) validateLuckOil() error {
	oil := f.LuckOil
	if oil == nil {
		return nil
	}
	if f.CombatLuck == nil || len(oil.Steps) != 9 || oil.Steps[0].SuccessPPM != maxLuckPPM {
		return errors.New("用油规则需要已开放战斗幸运及九个逐级档位，首档必成")
	}
	if err := f.validateLuckOilItem(oil.ItemID); err != nil {
		return err
	}
	for index, step := range oil.Steps {
		failure := index
		var pity uint32
		if index >= 3 {
			failure = (index / 3) * 3
			pity = oil.Steps[failure].PityAfter
		}
		if step.FailureLuck != uint32(failure) || step.PityAfter != pity || step.SuccessPPM == 0 ||
			(index > 0 && step.SuccessPPM > oil.Steps[index-1].SuccessPPM) {
			return fmt.Errorf("当前幸运 %d 的成功率、失败回退或保底条款无效", index)
		}
	}
	first, second := oil.Steps[3].PityAfter, oil.Steps[6].PityAfter
	if first == 0 || second <= first || second > math.MaxUint32-3 {
		return errors.New("两段用油保底必须为递增正值，并预留逐瓶收尾的计数")
	}
	return nil
}

// validateLuckOilItem 防止把既有技能书、药水或可回收道具当成祝福油。
func (f *Fixture) validateLuckOilItem(id uint32) error {
	count := 0
	for _, item := range f.Items {
		if item.ID != id {
			continue
		}
		count++
		if id == 0 || strings.TrimSpace(item.Name) == "" || !item.Stackable || item.MaxStack < 1 ||
			item.IsEquipment || item.EquipmentSlot != 0 || item.OriginalDurabilityU != 0 ||
			item.BookSkillID != 0 || item.IsSkillPage || item.RecycleGold != 0 {
			return errors.New("祝福油必须引用独立、可堆叠的非装备材料")
		}
	}
	if count != 1 {
		return errors.New("祝福油必须引用唯一物品模板")
	}
	for _, potion := range f.Potions.Effects {
		if potion.ItemID == id {
			return errors.New("祝福油不能用普通药水入口消耗")
		}
	}
	return nil
}
