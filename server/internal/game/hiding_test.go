// 本文件验证原护符隐身的时序、冷眼、重新仇恨和主动移动解除。
package game

import (
	"slices"
	"testing"
	"time"

	"minimir/internal/devconfig"
	"minimir/internal/domain"
	"minimir/internal/persistence"
)

// TestHidingUsesMaterialAndClearsTrackingUntilRevealed 验证同生命隐身，以及怪物主动索敌与受到攻击的区别。
func TestHidingUsesMaterialAndClearsTrackingUntilRevealed(t *testing.T) {
	w, r, monster, q, request := soulFireBallFixture(t)
	// f 中的临时定义只用于验收，不改变主配置的技能和供给入口。
	f := &w.e.Store.Cfg.Fixture
	skill := devconfig.Skill{ID: 18, JobID: 2, Behavior: "hiding", TargetKind: "friendly_self", BuffID: 18, MaxLevel: 3,
		MPBaseCost: 1, MPLevelCost: 1, ActionMS: 600, DelayBaseMS: 500, GlobalSpellMS: 1800, MaterialItemID: 187, MaterialQuantity: 1}
	definition := devconfig.BuffDefinition{ID: 18, Kind: "hiding", DurationKind: "sc_rank_linear", StatMultiplier: 1, DurationSecondsByLevel: []int64{5, 10, 15, 20},
		TickMS: 1000, StackPolicy: "refresh_duration_keep_values", RemoveOnPlayerDeath: true}
	f.Skills = append(slices.DeleteFunc(f.Skills, func(entry devconfig.Skill) bool { return entry.ID == skill.ID }), skill)
	f.Buffs = append(slices.DeleteFunc(f.Buffs, func(entry devconfig.BuffDefinition) bool { return entry.ID == definition.ID }), definition)
	r.Role.Skills = append(r.Role.Skills, domain.LearnedSkill{SkillID: 18})
	q.SkillID, request.SkillId, request.TargetHandle = 18, 18, 0
	// now 是技能受理时刻，plan 由原材料事务验证后保留25秒来源时长。
	now := time.Unix(1800000000, 0)
	plan, code := w.prepareMaterialCast(r, q, request, now)
	if code != 0 || plan == nil || plan.BuffDurationMS != 25_000 || r.Player.MP != 30 {
		t.Fatalf("原护符隐身时长或受理不符，结果 %d，计划 %+v", code, plan)
	}
	// receipt 是原消耗成功回执，仅首次提交现场效果。
	receipt := persistence.Operation{Kind: "cast_material", CommandSeq: 1, Casting: &persistence.CastingReceipt{SkillID: 18, Scope: "manual", InstanceID: q.ItemID, Quantity: 1}}
	if !w.applyMaterialReceipt(r, receipt, plan, 1, now) || w.applyMaterialReceipt(r, receipt, plan, 1, now) || r.Player.MP != 29 {
		t.Fatal("隐身未按原回执受理一次")
	}
	world := r.currentWorld()
	// cast 保存应用时间及原出生身份，测试之后还用同一状态刷新路径。
	cast := r.Casts[0]
	w.advanceEvents(r, cast.Due.Add(-time.Millisecond))
	if hidingStatus(&r.Player) != nil {
		t.Fatal("隐身提前生效")
	}
	w.advanceEvents(r, cast.Due)
	if hidingStatus(&r.Player) == nil || visibleMonsterTarget(world, monster, &r.Player, nil, r.Role.Level) != nil {
		t.Fatal("普通怪物没有因隐身失去原目标")
	}
	monster.Monster.CoolEye, monster.Monster.Level = true, r.Role.Level-1
	if canSeeFightTarget(world, monster, &r.Player, r.Role.Level) {
		t.Fatal("低等级冷眼不应识破隐身")
	}
	monster.Monster.Level++
	if !canSeeFightTarget(world, monster, &r.Player, r.Role.Level) {
		t.Fatal("同等级冷眼应识破隐身")
	}
	monster.Monster.CoolEye = false
	w.applyDamageInWorld(r, world, &r.Player, monster, 0, cast.Due)
	if !canSeeFightTarget(world, monster, &r.Player, r.Role.Level) {
		t.Fatal("被直接攻击的怪物即便防住伤害也能重新建立仇恨")
	}
	monster.Life++
	if canSeeFightTarget(world, monster, &r.Player, r.Role.Level) {
		t.Fatal("重生后错误继承上一生命的仇恨")
	}
	w.applyDamageInWorld(r, world, &r.Player, monster, 0, cast.Due)
	if !w.applyBuff(r, cast, &r.Player, cast.Due) || canSeeFightTarget(world, monster, &r.Player, r.Role.Level) {
		t.Fatal("重新隐身没有清除怪物刚建立的仇恨")
	}
	// next 是合法的主动行走格，实际走出后才清掉现有效果。
	next := devconfig.Point{X: 5, Y: 5}
	if !w.commitWalk(r, &r.Player, next, 2, now.Add(time.Second), 500) || hidingStatus(&r.Player) != nil {
		t.Fatal("完成主动步行后隐身没有解除")
	}
	if !w.applyBuff(r, cast, &r.Player, now) || !w.tickStatuses(r, now.Add(26*time.Second)) || hidingStatus(&r.Player) != nil {
		t.Fatal("静止隐身应在来源时长耗尽后自然解除")
	}
}
