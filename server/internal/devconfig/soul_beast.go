// 本文件校验魂兽名录、三池词条、逐星价值及首领掉落合同。
package devconfig

import "errors"

// SoulRules 是从唯一玩法表及明确隔离装配生成的作用范围。
type SoulRules struct {
	Types    []SoulType          `json:"types"`    // 全部角色独有类型。
	Affixes  []SoulAffix         `json:"affixes"`  // 已接入战斗的候选词条。
	Defaults [3]uint32           `json:"defaults"` // 一星初始的三池选项。
	Personal map[uint32]SoulDrop `json:"personal"` // 按个人首领模板配置。
	World    map[uint32]SoulDrop `json:"world"`    // 按世界首领模板配置。
}

// SoulType 包含独立于市场和背包的逐星成本。
type SoulType struct {
	ID        uint32      `json:"id"`        // 玩法表原名录编号。
	Name      string      `json:"name"`      // 客户端和同名首领使用的名字。
	Duplicate int64       `json:"duplicate"` // 重复本体换取的固定碎片。
	Synthesis int64       `json:"synthesis"` // 未拥有时一星合成费用。
	Stars     [7]SoulStar `json:"stars"`     // 数组下标从一星开始对应。
}

// SoulStar 明确当前分解价值和进入该星级的费用。
type SoulStar struct {
	Salvage int64 `json:"salvage"` // 该星级分解回收。
	Upgrade int64 `json:"upgrade"` // 从前一星升级到此星的费用。
}

// SoulAffix 以显式单位保存逐星贡献，编号对应固定的属性目标。
type SoulAffix struct {
	ID     uint32   `json:"id"`     // 原词条编号。
	Name   string   `json:"name"`   // 原目录可读名称。
	Unit   string   `json:"unit"`   // points 或 bp。
	Values [7]int64 `json:"values"` // 七个星级的实际效果。
}

// SoulDrop 在同名首领的原胜利或原有效参与奖记录上确认一次。
type SoulDrop struct {
	TypeID    uint32 `json:"type_id"`    // 与对应首领同名的角色收藏。
	ChancePPM uint32 `json:"chance_ppm"` // 每一条原获奖资格的概率。
}

// Type 返回已声明的唯一名录数据。
func (s *SoulRules) Type(id uint32) (SoulType, bool) {
	if s != nil && id > 0 && int(id) <= len(s.Types) && s.Types[id-1].ID == id {
		return s.Types[id-1], true
	}
	return SoulType{}, false
}

// Affix 反查已经接入的三池选项。
func (s *SoulRules) Affix(id uint32) (SoulAffix, bool) {
	if s != nil {
		for _, option := range s.Affixes {
			if option.ID == id {
				return option, true
			}
		}
	}
	return SoulAffix{}, false
}

// SoulAffixPool 按工作簿固定分池，返回 0 至 2，非法编号返回 -1。
func SoulAffixPool(id uint32) int {
	if id == 0 || id > 21 {
		return -1
	}
	return int((id - 1) / 7)
}

// validateSouls 同时校验经济守恒、战斗额度和首领奖励的真实模板。
func (f *Fixture) validateSouls() error {
	s := f.Souls
	if s == nil {
		return nil
	}
	if f.RuleVersion < 30 || f.ExpandedHitBudget == nil || f.MasteryCombat == nil || len(s.Types) != 13 {
		return errors.New("魂兽需完整名录及已经验证的独立战斗属性组")
	}
	for i, beast := range s.Types {
		if beast.ID != uint32(i+1) || beast.Name == "" || beast.Duplicate <= 0 || beast.Duplicate > 1_000_000_000 ||
			beast.Synthesis < beast.Stars[0].Salvage || beast.Synthesis > 1_000_000_000 {
			return errors.New("魂兽类型、折片或合成费用不合法")
		}
		for star, value := range beast.Stars {
			if value.Salvage <= 0 || value.Salvage > 1_000_000_000 || value.Upgrade > 1_000_000_000 ||
				star == 0 && value.Upgrade != 0 ||
				star > 0 && (value.Upgrade <= 0 || value.Salvage < beast.Stars[star-1].Salvage ||
					value.Salvage > beast.Stars[star-1].Salvage+value.Upgrade) {
				return errors.New("魂兽星级费用或分解价值违反碎片守恒")
			}
		}
	}
	seen := make(map[uint32]bool, len(s.Affixes))
	for _, option := range s.Affixes {
		unit, cap := soulAffixContract(option.ID, f)
		if seen[option.ID] || option.Name == "" || cap == 0 || option.Unit != unit {
			return errors.New("魂兽词条范围或单位未核验")
		}
		var previous int64
		for _, value := range option.Values {
			if value <= 0 || value < previous || value > cap {
				return errors.New("魂兽词条七星贡献必须完整并受同类上限约束")
			}
			previous = value
		}
		seen[option.ID] = true
	}
	for i, id := range s.Defaults {
		if !seen[id] || SoulAffixPool(id) != i {
			return errors.New("魂兽初始配置须三池各一项")
		}
	}
	for template, drop := range s.Personal {
		boss, beastOK := f.PersonalBossByTemplate(template), s.validDrop(drop)
		if boss == nil || !beastOK || boss.DisplayName != s.Types[drop.TypeID-1].Name {
			return errors.New("个人首领必须引用实际同名魂兽")
		}
	}
	for template, drop := range s.World {
		var boss *WorldBossConfig
		if f.WorldBoss != nil && f.WorldBoss.TemplateID == template {
			boss = f.WorldBoss
		}
		for i := range f.WorldBossVariants {
			if f.WorldBossVariants[i].TemplateID == template {
				boss = &f.WorldBossVariants[i]
				break
			}
		}
		if !s.validDrop(drop) || boss == nil || boss.Rewards == nil || boss.DisplayName != s.Types[drop.TypeID-1].Name {
			return errors.New("世界首领魂兽必须沿同名模板的实际参与资格发放")
		}
	}
	return nil
}

// validDrop 防止零概率、未配置对象和溢出概率进入原结算。
func (s *SoulRules) validDrop(d SoulDrop) bool {
	_, ok := s.Type(d.TypeID)
	return ok && d.ChancePPM > 0 && d.ChancePPM <= BodyChancePPM
}

// soulAffixContract 只让已经接入的原属性进入候选，未知效果保留工作簿待核验状态。
func soulAffixContract(id uint32, f *Fixture) (string, int64) {
	switch id {
	case 1, 2, 3, 8, 9, 18:
		return "points", 2
	case 7:
		return "bp", int64(min(200, f.ExpandedHitBudget.DamageCapBP))
	case 15:
		return "bp", int64(min(150, f.MasteryCombat.AttackSpeedCapBP))
	case 19:
		return "bp", int64(min(150, f.MasteryCombat.CriticalCapBP))
	}
	return "", 0
}

// SoulBonuses 保存已附体的且限定作用阶段的贡献。
type SoulBonuses struct {
	stats    equipmentStats // 独立整数属性，在装备强化之后加入。
	accuracy int64          // 原命中准确整数点数。
	Hit      HitBonuses     // 进入原扩展伤害组。
	Mastery  MasteryBonuses // 进入原攻速或暴击组。
}

// Bonuses 从唯一附体对象和已保存的三池选项计算各自效果。
func (s *SoulRules) Bonuses(star uint32, selected [3]uint32) SoulBonuses {
	var result SoulBonuses
	if s == nil || star == 0 || star > 7 {
		return result
	}
	for index, id := range selected {
		affix, ok := s.Affix(id)
		if !ok || SoulAffixPool(id) != index {
			continue
		}
		value := affix.Values[star-1]
		switch id {
		case 1:
			result.stats[0], result.stats[1] = value, value
		case 2:
			result.stats[2], result.stats[3] = value, value
		case 3:
			result.stats[4], result.stats[5] = value, value
		case 7:
			result.Hit.DamageBP = uint32(value)
		case 8:
			result.stats[7], result.stats[8], result.stats[9] = value, value, value
		case 9:
			result.stats[10], result.stats[11] = value, value
		case 15:
			result.Mastery.AttackSpeedBP = uint32(value)
		case 18:
			result.accuracy = value
		case 19:
			result.Mastery.CriticalBP = uint32(value)
		}
	}
	return result
}

// ApplyBase 投影整数选择并保留原战斗算术边界。
func (s SoulBonuses) ApplyBase(base Fighter) (Fighter, bool) {
	accuracy, ok := equipmentAdd(base.Accuracy, s.accuracy)
	if !ok {
		return Fighter{}, false
	}
	base.Accuracy = accuracy
	return applyEquipmentStats(base, s.stats)
}
