// 本文件执行圣言术的亡灵识别、等级与技能阶判定，以及原生命的延迟结算。
package game

import (
	"time"

	"minimir/internal/devconfig"
)

// turnUndeadTarget 判断 target 是否是当前仍存活的敌对亡灵怪物。
// 目标只能从当前世界的敌方实体列表取得；宠物、玩家和尸体均不在范围内。
func turnUndeadTarget(target *entity) bool {
	return target != nil && target.Kind == 3 && target.HP > 0 && target.Monster != nil && target.Monster.Undead
}

// turnUndeadSucceeds 依次执行来源定义的等级闸门与成功率判定。
// w 提供确定性掷点，casterLevel 和 targetLevel 是本次受理时的双方等级，rank 是技能的原生阶。
func (w *worker) turnUndeadSucceeds(casterLevel, targetLevel, rank uint32) bool {
	// difference 保存施法者比怪物高的实际等级差，用有符号类型计算，避免低等级下溢。
	difference := int64(casterLevel) - int64(targetLevel)
	if difference-1+w.draw(2) <= 0 {
		return false
	}
	// chance 是来源公式中原生阶加一后乘八、等级差与固定偏移的和。
	chance := devconfig.TurnUndeadBaseThreshold(rank) + difference
	return w.draw(100) < chance
}

// resolveTurnUndead 结算已通过成功率判定的圣言术。
// w 持有房间 r，world 是原执行世界，caster 是仍匹配原出生代次的施法者，c 保留原事件归属，now 是实际推进时刻。
func (w *worker) resolveTurnUndead(r *room, world *worldState, caster *entity, c pendingCast, now time.Time) {
	// target 按当前仍归原世界的句柄解析，再验证是同一出生代次的亡灵。
	target := w.skillTargetInWorld(r, world, c.Skill, c.Target)
	if !turnUndeadTarget(target) || target.Life != c.TargetLife {
		w.impactInWorld(r, world, c.Scene, c.Skill.ID, c.Caster.Handle, c.Target, c.TargetLife, c.TargetPoint, 0, 3, false, c.ID, 0)
		return
	}
	// before 保存当时剩余生命。成功效果沿普通击杀路径记录原事件来源，使归属、掉落和托管保持一致。
	before := target.HP
	w.applyDamageInWorld(r, world, caster, target, before, now, hitContext{
		At: c.Due, HostSession: c.HostSession, PrivilegeUntil: c.PrivilegeUntil, PrivilegeCaptured: c.PrivilegeCaptured,
	})
	w.impactInWorld(r, world, c.Scene, c.Skill.ID, c.Caster.Handle, target.Handle, target.Life, target.Pos, before-target.HP, 1, false, c.ID, 0)
}
