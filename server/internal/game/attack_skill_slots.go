// 本文件按角色四个攻击槽的实际就绪时间，选择当前现场可执行的动作。
package game

import (
	"slices"
	"time"

	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
)

// attackSlotsForRole 返回发给客户端的四个槽，缺省按原自动技能优先级生成。
func attackSlotsForRole(role domain.Role, fixture devconfig.Fixture) []uint32 {
	ids := persistence.ResolvedSkillSlots(role, &fixture)
	return ids[:]
}

// readyAttackSlot 持有原配置、当前技能等级和排序使用的最早可用时刻。
type readyAttackSlot struct {
	skill devconfig.Skill
	rank  uint32
	at    time.Time
}

// attackFromConfiguredSlots 校验手动攻击的实体代次，并沿用原输入号确认受理动作。
func (w *worker) attackFromConfiguredSlots(r *room, intent *pb.AttackIntent, now time.Time) uint32 {
	// 普通怪首次出生代次为零；是否点选目标使用实体句柄判断。
	if intent.SceneEpoch != r.Scene || intent.Direction > 7 || intent.TargetHandle == 0 && intent.TargetLife != 0 {
		return p.CodeInvalidArgument
	}
	direction := intent.Direction
	var focus *entity
	if intent.TargetHandle != 0 {
		focus = r.activeMonster(intent.TargetHandle, intent.TargetLife)
		if focus == nil {
			return p.CodeSkillTarget
		}
		direction = facing(r.Player.Pos, focus.Pos, direction)
	}
	if intent.SkillId != 0 {
		return w.attackFacingTarget(r, direction, intent.SkillId, intent.InputSequence, now, focus)
	}
	// 老客户端只传方向，沿该方向取首个前格敌人供有射程的技能使用。
	spellTarget := focus
	if spellTarget == nil {
		spellTarget = w.enemyAt(r, stepPoint(r.Player.Pos, direction, 1))
	}
	if accepted, _ := w.tryAttackSkillSlots(r, spellTarget, direction, intent.InputSequence, now, true); accepted {
		return 0
	}
	return w.attackSlotFallback(r, direction, intent.InputSequence, now, focus)
}

// tryAttackSkillSlots 先尝试最早可用的技能，同一时刻按配置槽位次序。
func (w *worker) tryAttackSkillSlots(r *room, focus *entity, direction, input uint32, now time.Time, manual bool) (bool, automaticManaNeed) {
	var need automaticManaNeed
	if r.Isolated || !manual && r.AutoSkills != nil && !r.AutoSkills.enabled {
		return false, need
	}
	fixture := &w.e.Store.Cfg.Fixture
	var candidates [devconfig.AttackSkillSlotCount]readyAttackSlot
	count := 0
	for _, id := range persistence.ResolvedSkillSlots(r.Role, fixture) {
		if id == 0 || !manual && !r.AutoSkills.permits(id) {
			continue
		}
		skill, exists := fixture.Skill(id)
		rank, learned := r.Role.Skill(id)
		if !exists || !learned || skill.JobID != r.Role.Job || rank < skill.LearnedLevel || rank > skill.MaxLevel || !skill.AttackSlotEligible() {
			continue
		}
		ready := r.SkillCooldowns[id]
		if skill.Behavior == "flaming_sword" {
			flamingReady := r.Melee.FlamingUntil
			if r.Melee.Flaming && now.Before(flamingReady) {
				flamingReady = flamingReady.Add(-time.Duration(skill.ChargeWindowMS) * time.Millisecond)
			}
			if flamingReady.After(ready) {
				ready = flamingReady
			}
		}
		if !now.Before(ready) {
			candidates[count] = readyAttackSlot{skill: skill, rank: rank, at: ready}
			count++
		}
	}
	slices.SortStableFunc(candidates[:count], func(a, b readyAttackSlot) int { return a.at.Compare(b.at) })
	for _, candidate := range candidates[:count] {
		if candidate.skill.Activation() == "cast" {
			if w.castQueuedAttackSkill(r, candidate.skill, focus, direction, input, now, manual, &need) {
				return true, need
			}
		} else if w.tryQueuedMelee(r, candidate, focus, direction, input, now, manual) {
			return true, need
		}
	}
	return false, need
}

// attackSlotFallback 兼容历史职业模式，并让显式配置失败后的普攻始终保持纯普攻。
func (w *worker) attackSlotFallback(r *room, direction, input uint32, now time.Time, focus *entity) uint32 {
	if len(r.Role.SkillSlotIDs) != devconfig.AttackSkillSlotCount {
		return w.attackFacingTarget(r, direction, 0, input, now, focus)
	}
	if focus != nil && w.enemyAt(r, stepPoint(r.Player.Pos, direction, 1)) != focus {
		return p.CodeSkillTarget
	}
	return w.attackWithSkill(r, direction, input, now, devconfig.Skill{}, 0)
}

// tryQueuedMelee 只从锁定怪选方向；尚未锁怪的自动战斗复用近格优先的次序。
func (w *worker) tryQueuedMelee(r *room, candidate readyAttackSlot, focus *entity, direction, input uint32, now time.Time, manual bool) bool {
	if !w.sourceTimeline() {
		return false
	}
	if focus != nil || manual {
		if focus != nil {
			direction = facing(r.Player.Pos, focus.Pos, direction)
		}
		return w.acceptQueuedMelee(r, candidate, focus, direction, input, now, manual)
	}
	for _, cells := range []int{1, 2} {
		for _, target := range r.Monsters[:r.MonsterCount] {
			if target == nil || target.HP <= 0 || distance(r.Player.Pos, target.Pos) != cells {
				continue
			}
			direction = facing(r.Player.Pos, target.Pos, direction)
			if w.acceptQueuedMelee(r, candidate, target, direction, input, now, false) {
				return true
			}
		}
	}
	return false
}

// acceptQueuedMelee 校验刀法形状和资源后交给原攻击事件入口，失败不会消耗一段烈火。
func (w *worker) acceptQueuedMelee(r *room, candidate readyAttackSlot, focus *entity, direction, input uint32, now time.Time, manual bool) bool {
	skill, rank := candidate.skill, candidate.rank
	front := w.enemyAt(r, stepPoint(r.Player.Pos, direction, 1))
	cost := skill.MPBaseCost + int64(rank)*skill.MPLevelCost
	switch skill.Behavior {
	case "half_moon":
		secondary := w.secondaryTargets(r, skill, direction)
		if r.Player.MP < cost || front == nil && len(secondary) == 0 || focus != nil && front != focus && !slices.Contains(secondary, focus) {
			return false
		}
	case "thrusting":
		second := w.enemyAt(r, stepPoint(r.Player.Pos, direction, skill.SecondaryDistanceCells))
		if second == nil || focus != nil && focus != front && focus != second {
			return false
		}
	case "flaming_sword":
		charged := r.Melee.Flaming && now.Before(r.Melee.FlamingUntil)
		if front == nil || focus != nil && focus != front || !charged && r.Player.MP <= cost {
			return false
		}
	default:
		return false
	}
	var ref actorRef
	if !manual {
		ref = reference(r, focus)
	}
	if w.attackWithSkill(r, direction, input, now, skill, rank) != 0 {
		return false
	}
	if r.SkillCooldowns == nil {
		r.SkillCooldowns = make(map[uint32]time.Time)
	}
	r.SkillCooldowns[skill.ID] = now.Add(time.Duration(skill.CooldownBaseMS-int64(rank)*skill.CooldownReductionMS) * time.Millisecond)
	if !manual {
		r.lockAutoTarget(ref)
	}
	return true
}
