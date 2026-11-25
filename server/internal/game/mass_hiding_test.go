// 本文件验证集体隐身的道术、施法时序、面积和同一实体的隐身规则。
package game

import (
	"slices"
	"testing"
	"time"

	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	"minimir/internal/persistence"
)

// TestMassHidingAppliesToLivingAlliesInArea 验证格点施法、存活友方和两种隐身互相刷新。
func TestMassHidingAppliesToLivingAlliesInArea(t *testing.T) {
	w, r, monster, q, request := soulFireBallFixture(t)
	f := &w.e.Store.Cfg.Fixture
	skill := devconfig.Skill{ID: 19, JobID: 2, Behavior: "mass_hiding", TargetKind: "friendly_grid", BuffID: 19, MaxLevel: 3,
		RangeCells: 9, AreaRadius: 1, MPBaseCost: 2, MPLevelCost: 2, ActionMS: 600, DelayBaseMS: 500, ProjectileMSPerCell: 50,
		GlobalSpellMS: 1800, MaterialItemID: 187, MaterialQuantity: 1}
	definition := devconfig.BuffDefinition{ID: 19, Kind: "mass_hiding", DurationKind: "sc_half_rank_linear", StatMultiplier: 1,
		DurationSecondsByLevel: []int64{2, 4, 6, 8}, TickMS: 1000, StackPolicy: "refresh_duration_keep_values", RemoveOnPlayerDeath: true}
	f.Skills = append(slices.DeleteFunc(f.Skills, func(entry devconfig.Skill) bool { return entry.ID == skill.ID }), skill)
	f.Buffs = append(slices.DeleteFunc(f.Buffs, func(entry devconfig.BuffDefinition) bool { return entry.ID == definition.ID }), definition)
	r.Role.Skills = append(r.Role.Skills, domain.LearnedSkill{SkillID: 19, Level: 1})
	r.Pet = &entity{Handle: 2, Kind: 2, HP: 20, Life: 1, Pos: devconfig.Point{X: 6, Y: 5},
		PetRuntime: &petRuntimeState{Fighter: devconfig.Fighter{Level: 12}}}
	monster.Pos = devconfig.Point{X: 5, Y: 6}
	now := time.Unix(1800000000, 0)
	// 点位离施法者一格，主人和宠物分居范围边缘。
	q.SkillID, request.SkillId, request.TargetHandle = 19, 19, 0
	request.TargetGrid = &pb.GridPoint{X: 5, Y: 5}
	plan, code := w.prepareMaterialCast(r, q, request, now)
	if code != 0 || plan == nil || plan.DelayMS != 550 || plan.BuffDurationMS != 14_000 || plan.Cost != 4 {
		t.Fatalf("集体隐身方案不符，结果 %d，方案 %+v", code, plan)
	}
	// 施法前主人已在单人隐身中，跨技能重施时只能保留一种隐身。
	r.Player.Statuses = map[uint32]*combatStatus{18: {ID: 18, Kind: "hiding", TargetLife: r.Player.Life, RemainingMS: 4000}}
	receipt := persistence.Operation{Kind: "cast_material", CommandSeq: 1, Casting: &persistence.CastingReceipt{SkillID: 19, Scope: "manual", InstanceID: q.ItemID, Quantity: 1}}
	if !w.applyMaterialReceipt(r, receipt, plan, 1, now) || w.applyMaterialReceipt(r, receipt, plan, 1, now) || r.Player.MP != 26 {
		t.Fatal("集体隐身未按同一持久回执受理一次")
	}
	world := r.currentWorld()
	due := r.Casts[0].Due
	w.advanceEvents(r, due.Add(-time.Millisecond))
	if old := hidingStatus(&r.Player); old == nil || old.ID != 18 || hidingStatus(r.Pet) != nil {
		t.Fatal("集体隐身早于规定时刻生效")
	}
	w.advanceEvents(r, due)
	playerStatus, petStatus := hidingStatus(&r.Player), hidingStatus(r.Pet)
	if playerStatus == nil || playerStatus.ID != 19 || len(r.Player.Statuses) != 1 || petStatus == nil || petStatus.ID != 19 || hidingStatus(monster) != nil {
		t.Fatal("集体隐身没有覆盖范围内的存活友方或错误影响怪物")
	}
	if visibleMonsterTarget(world, monster, &r.Player, r.Pet, r.Role.Level) != nil {
		t.Fatal("隐身期间的宠物或主人仍可被普通怪物直接选中")
	}
	monster.Monster.CoolEye, monster.Monster.Level = true, 11
	if visibleMonsterTarget(world, monster, &r.Player, r.Pet, r.Role.Level) != nil {
		t.Fatal("等级低于隐身宠物的冷眼怪不能识破")
	}
	monster.Monster.Level = 12
	if visibleMonsterTarget(world, monster, &r.Player, r.Pet, r.Role.Level) != r.Pet {
		t.Fatal("达到宠物战斗等级的冷眼怪应能识破宠物")
	}
	monster.Monster.CoolEye = false
	w.applyDamageInWorld(r, world, r.Pet, monster, 0, due)
	if visibleMonsterTarget(world, monster, &r.Player, r.Pet, r.Role.Level) != r.Pet {
		t.Fatal("受到隐身宠物攻击的怪物没有重新追踪原攻击者")
	}
	if !w.commitWalk(r, &r.Player, devconfig.Point{X: 4, Y: 6}, 2, due.Add(time.Second), 500) || hidingStatus(&r.Player) != nil || hidingStatus(r.Pet) == nil {
		t.Fatal("主动步行只应解除移动实体的隐身")
	}
	// 再按同样中心应用时，范围外的玩家和已阵亡的宠物都不能获得效果。
	r.Player.Pos, r.Pet.HP, r.Pet.Statuses = devconfig.Point{X: 3, Y: 5}, 0, nil
	cast := pendingCast{Skill: skill, TargetPoint: devconfig.Point{X: 5, Y: 5}}
	if targets := w.buffTargets(r, cast); len(targets) != 0 {
		t.Fatal("集体隐身选中了范围外或已阵亡的目标")
	}
}
