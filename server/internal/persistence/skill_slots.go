// 本文件核验并保存角色配置的四个有序攻击技能槽。
package persistence

import (
	"slices"

	"minimir/internal/devconfig"
	"minimir/internal/domain"
	p "minimir/internal/protocol"
)

const AssetSetSkillSlots = "set_skill_slots" // 复用原角色永久经济事务和回执。

func setSkillSlots(r domain.Role, f *devconfig.Fixture, ids []uint32) (domain.Role, uint32) {
	if len(ids) != devconfig.AttackSkillSlotCount {
		return r, p.CodeInvalidArgument
	}
	for index, id := range ids {
		if id == 0 {
			continue
		}
		skill, exists := f.Skill(id)
		level, learned := r.Skill(id)
		if !exists || skill.JobID != r.Job || !skill.AttackSlotEligible() || slices.Contains(ids[:index], id) {
			return r, p.CodeInvalidArgument
		}
		if !learned || level < skill.LearnedLevel || level > skill.MaxLevel {
			return r, p.CodeSkillNotLearned
		}
	}
	r.SkillSlotIDs = slices.Clone(ids)
	return r, 0
}
