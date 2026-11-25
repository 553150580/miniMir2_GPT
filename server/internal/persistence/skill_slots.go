// 本文件核验并保存角色配置的四个有序攻击技能槽。
package persistence

import (
	"slices"

	"minimir/internal/devconfig"
	"minimir/internal/domain"
	p "minimir/internal/protocol"
)

const AssetSetSkillSlots = "set_skill_slots" // 复用原角色永久经济事务和回执。

// ResolvedSkillSlots 返回已保存的四槽，或按已学自动技能的优先级生成旧角色的缺省槽位。
func ResolvedSkillSlots(r domain.Role, f *devconfig.Fixture) [devconfig.AttackSkillSlotCount]uint32 {
	var ids [devconfig.AttackSkillSlotCount]uint32
	if len(r.SkillSlotIDs) == len(ids) {
		copy(ids[:], r.SkillSlotIDs)
		return ids
	}
	var priorities [devconfig.AttackSkillSlotCount]int
	for _, skill := range f.Skills {
		if skill.JobID != r.Job || !skill.DefaultAttackSlotEligible() {
			continue
		}
		rank, learned := r.Skill(skill.ID)
		if !learned || rank < skill.LearnedLevel || rank > skill.MaxLevel {
			continue
		}
		for index, id := range ids {
			if id == 0 || skill.AutoPriority > priorities[index] || skill.AutoPriority == priorities[index] && skill.ID < id {
				copy(ids[index+1:], ids[index:])
				copy(priorities[index+1:], priorities[index:])
				ids[index], priorities[index] = skill.ID, skill.AutoPriority
				break
			}
		}
	}
	return ids
}

// matchesAssetRevision 只有技能槽可用原槽位作比较，避免无关的挂机收益或磨损抢先推进角色修订。
func matchesAssetRevision(r domain.Role, f *devconfig.Fixture, q AssetRequest) bool {
	expected := q.Context.ExpectedRevision
	if q.Kind != AssetSetSkillSlots || len(q.ExpectedSkillSlotIDs) == 0 {
		return expected == uint64(r.Revision)
	}
	current := ResolvedSkillSlots(r, f)
	return expected != 0 && expected <= uint64(r.Revision) && slices.Equal(q.ExpectedSkillSlotIDs, current[:])
}

// setSkillSlots 在当前角色状态下重新验证每个待保存技能的资格。
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
