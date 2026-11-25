// 本文件装配讨伐链 AI 的原职业、等级、战斗技能和固定金币报价。
package devconfig

import (
	"math"
	"slices"
	"strings"
)

const maxDungeonHireOptions = 16 // 单个模板可公示的能力档数量。

// DungeonHireOption 只描述从原职业成长和技能构建的临时战斗身体。
type DungeonHireOption struct {
	ID          uint32             `json:"id"`           // 模板内稳定选项编号。
	DisplayName string             `json:"display_name"` // 招募及场内显示名称。
	Job         uint32             `json:"job"`          // 战士、法师或道士。
	Level       uint32             `json:"level"`        // 原成长表计算属性的角色等级。
	FeeGold     int64              `json:"fee_gold"`     // 开战时由队长两类金币支付的总价。
	Skills      []DungeonHireSkill `json:"skills"`       // 已掌握的同职业实际可用技能。
}

// DungeonHireSkill 指定已掌握的技能编号和从原配置读取的等级。
type DungeonHireSkill struct {
	ID   uint32 `json:"id"`   // 技能目录编号。
	Rank uint32 `json:"rank"` // 允许零阶初学。
}

// HireOption 读取原模板中的确切报价，结果切片须作为装配快照只读使用。
func (c *DungeonConfig) HireOption(id uint32) (DungeonHireOption, bool) {
	for _, option := range c.Hires {
		if option.ID == id {
			return option, true
		}
	}
	return DungeonHireOption{}, false
}

// cloneDungeonHireOptions 校验有限费用及能力范围，再隔离原候选数组与运行态。
func (f *Fixture) cloneDungeonHireOptions(options []DungeonHireOption) ([]DungeonHireOption, bool) {
	if len(options) > maxDungeonHireOptions {
		return nil, false
	}
	result := slices.Clone(options)
	seen := make(map[uint32]bool, len(result))
	for index := range result {
		option := &result[index]
		fighter, hasProfession := f.PlayerAtLevel(option.Job, option.Level)
		if option.ID == 0 || seen[option.ID] || strings.TrimSpace(option.DisplayName) == "" || len(option.DisplayName) > 64 ||
			option.Job > 2 || option.Level == 0 || option.Level > 255 || !hasProfession || fighter.HP <= 0 ||
			option.FeeGold < 0 || option.FeeGold > math.MaxInt64/(DungeonMaxPlayers-1) ||
			!f.validDungeonHireSkills(*option) {
			return nil, false
		}
		seen[option.ID] = true
		option.Skills = slices.Clone(option.Skills)
	}
	return result, true
}

// validDungeonHireSkills 只允许同职业的被动或能在本场自动使用且不写永久材料的技能。
func (f *Fixture) validDungeonHireSkills(option DungeonHireOption) bool {
	if len(option.Skills) > len(f.Skills) {
		return false
	}
	seen := make(map[uint32]bool, len(option.Skills))
	slots := 0
	for _, entry := range option.Skills {
		skill, ok := f.Skill(entry.ID)
		if !ok || seen[entry.ID] || skill.JobID != option.Job || skill.LearnLevel > option.Level ||
			entry.Rank < skill.LearnedLevel || entry.Rank > skill.MaxLevel || !dungeonHireSkillUsable(skill) {
			return false
		}
		seen[entry.ID] = true
		if skill.AttackSlotEligible() {
			slots++
		}
	}
	return slots <= AttackSkillSlotCount
}

// dungeonHireSkillUsable 沿原副本技能范围，跳过需要手动场外操作的技能。
func dungeonHireSkillUsable(skill Skill) bool {
	if skill.MaterialQuantity != 0 || skill.Summon != nil || skill.Teleport != nil || skill.Displacement != nil {
		return false
	}
	switch skill.Behavior {
	case "fire_wall", "revelation", "electric_shock", "trap_hexagon":
		return false
	}
	activation := skill.Activation()
	return skill.AttackSlotEligible() || activation == "passive" || activation == "passive_proc"
}
