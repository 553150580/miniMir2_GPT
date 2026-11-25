// 本文件校验诱惑之光和心灵启示的职业、目标、时钟及状态引用。
package devconfig

import "errors"

// validateMonsterUtilitySkill 校验 s 是否符合针对单个怪物的手动施法合同。
func validateMonsterUtilitySkill(s Skill) error {
	switch s.Behavior {
	case "electric_shock":
		if s.ID != 20 || s.JobID != 1 || s.BuffID != 0 {
			return errors.New("诱惑之光的技能编号、职业或状态引用无效")
		}
	case "revelation":
		if s.ID != 28 || s.JobID != 2 || s.BuffID != s.ID {
			return errors.New("心灵启示的技能编号、职业或状态引用无效")
		}
	default:
		return nil
	}
	if s.Activation() != "cast" || s.MaxLevel > 3 || s.TargetKind != "enemy_entity" || s.RangeCells <= 0 || s.AreaRadius != 0 || s.DelayBaseMS != 500 ||
		s.ProjectileMSPerCell != 0 || s.MaterialItemID != 0 || s.MaterialQuantity != 0 || s.RequireLineOfSight || s.AutoEnabled {
		return errors.New("诱惑之光和心灵启示只允许手动点选单个怪物施法")
	}
	return nil
}
