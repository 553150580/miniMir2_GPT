// 本文件限定困魔咒隔离候选采用的原生阶位、施法延时和材料规则。
package devconfig

import (
	"errors"
	"time"
)

// TrapHexagonBehavior 是独立地格围困行为在技能目录中的配置键。
const TrapHexagonBehavior = "trap_hexagon"

// TrapHexagonDelayMS 是来源中受理到第一次围困之间的毫秒数。
const TrapHexagonDelayMS = 500

// TrapHexagonDuration 用原生技能等级计算怪物停止主动行为的期限。
func TrapHexagonDuration(rank uint32) time.Duration {
	if rank > 3 {
		return 0
	}
	return time.Duration(10+5*rank) * time.Second
}

// validateTrapHexagonSkill 拒绝把围困当作自动、伤害、正式护符包或未知职业技能开放。
func validateTrapHexagonSkill(s Skill) error {
	if s.ID != 16 || s.JobID != 2 || s.LearnedLevel != 0 || s.MaxLevel != 3 || s.TargetKind != "enemy_grid" ||
		s.AreaRadius != 1 || s.RangeCells != 9 || s.AutoEnabled || s.BuffID != 0 || s.DelayBaseMS != TrapHexagonDelayMS ||
		s.ProjectileMSPerCell != 0 || s.RequireLineOfSight || s.MaterialItemID == 0 || s.MaterialQuantity != 1 {
		return errors.New("困魔咒须为道士手动地格候选，持正式材料回执后延迟500毫秒建立围困")
	}
	return nil
}
