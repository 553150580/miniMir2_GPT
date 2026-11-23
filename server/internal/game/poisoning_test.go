// 本文件验证施毒的原药粉颜色、离散时钟、伤害、覆盖及原生命归属。
package game

import (
	"testing"
	"time"

	"minimir/internal/devconfig"
	"minimir/internal/domain"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
)

// TestPoisoningPreparedPowderIsImmutable 验证灰粉及黄粉在事务闸门按原实例选择，后续换粉不改变本次施法。
func TestPoisoningPreparedPowderIsImmutable(t *testing.T) {
	w, r, target, q, request := soulFireBallFixture(t)
	// f 是隔离候选，只加入源灰粉181和黄粉182，不开启主试玩供应。
	f := &w.e.Store.Cfg.Fixture
	f.CastingSupply.ItemIDs = append(f.CastingSupply.ItemIDs, 181, 182)
	// skill 定义受理时使用的原药粉和时序，颜色不来自客户端字段。
	skill := devconfig.Skill{ID: 6, JobID: 2, Behavior: "poisoning", TargetKind: "enemy_entity", RangeCells: 9, MaxLevel: 3,
		MPBaseCost: 2, MPLevelCost: 1, DelayBaseMS: 500, ActionMS: 600, GlobalSpellMS: 1800, MultiplierBasePPM: 1000000,
		MaterialQuantity: 1, Poison: &devconfig.PoisonMaterials{GreenItemIDs: []uint32{181}, RedItemIDs: []uint32{182}}}
	f.Skills = append(f.Skills, skill)
	r.Role.Skills = append(r.Role.Skills, domain.LearnedSkill{SkillID: 6})
	q.SkillID, request.SkillId = 6, 6
	// now 是隔离时间。装备着护符时不能冒充毒粉。
	now := time.Unix(1800000000, 0)
	if _, code := w.prepareMaterialCast(r, q, request, now); code != p.CodeSkillMaterialRequired {
		t.Fatalf("护符冒充药粉的结果 %d", code)
	}
	r.Role.Equipped[0].Item.ItemID = 181
	// plan 冻结已经过校验的原灰粉和道术威力。
	plan, code := w.prepareMaterialCast(r, q, request, now)
	if code != 0 || plan == nil || plan.PoisonColor != devconfig.PoisonGreen || plan.Amount != 20 || plan.DelayMS != 500 {
		t.Fatalf("灰粉冻结失败，结果 %d，计划 %+v", code, plan)
	}
	r.Role.Equipped[0].Item.ItemID = 182
	// receipt 代表原灰粉消费已确认；后续装备不能改写已经冻结的计划。
	receipt := persistence.Operation{Kind: "cast_material", CommandSeq: 1, Casting: &persistence.CastingReceipt{SkillID: 6, Scope: "manual", InstanceID: q.ItemID, Quantity: 1}}
	if !w.applyMaterialReceipt(r, receipt, plan, 1, now) || r.Player.MP != 28 {
		t.Fatal("原施毒回执没有受理")
	}
	target.Pos.X += 4
	w.advanceEvents(r, now.Add(500*time.Millisecond))
	if target.HP != 998 || target.Poisons == nil || target.Poisons.Green == nil || target.Poisons.Red != nil {
		t.Fatal("原灰粉应在500毫秒后对仍活着的原目标施加绿毒，与换粉或目标位移无关")
	}
}

// TestPoisoningDifferentColorsCoexistAndExpire 验证红绿两色可共存、最后一跳只移除，重施和晚处理不叠伤。
func TestPoisoningDifferentColorsCoexistAndExpire(t *testing.T) {
	w, r, target, _, _ := soulFireBallFixture(t)
	// now 是首个伤害处理时点，world 保存原怪物和对其作用的事件。
	now := time.Unix(1800000000, 0)
	world := r.currentWorld()
	// green 与 red 都是已经扣料后即将应用的原事件，使用原生阶0和来源威力0。
	green := pendingCast{Skill: devconfig.Skill{ID: 6, Behavior: "poisoning"}, Caster: reference(r, &r.Player), Target: target.Handle,
		TargetLife: target.Life, Scene: r.Scene, ID: 1, Due: now, PoisonColor: devconfig.PoisonGreen}
	red := green
	red.ID, red.PoisonColor = 2, devconfig.PoisonRed
	w.resolveCast(r, green, now)
	w.resolveCast(r, red, now)
	if target.Poisons == nil || target.Poisons.Green == nil || target.Poisons.Red == nil || target.HP != 999 || w.magicDamage(r, target, 10) != 8 {
		t.Fatal("红绿毒没有共存，或实际抽取的5点魔防没有按来源减半为2点")
	}
	// next 在已错过多个周期后才推进；两色各消费一个周期，绿毒只扣一次。
	next := now.Add(5 * time.Second)
	w.advanceEvents(r, next)
	if target.HP != 998 || target.Poisons.Green.Remaining != 5 || len(world.Casts) != 2 || !world.Casts[0].Due.Equal(next.Add(2*time.Second)) {
		t.Fatal("迟处理毒伤不应补扣历史跳数")
	}
	for i := 1; i <= 5; i++ {
		w.advanceEvents(r, next.Add(time.Duration(i)*2*time.Second))
	}
	if target.HP != 994 || target.Poisons != nil || w.magicDamage(r, target, 10) != 5 || len(world.Casts) != 0 {
		t.Fatalf("原生阶0威力0应有效扣血六次，最后仅移除毒。生命 %d", target.HP)
	}
}

// TestPoisoningPreservesStrongerApplicationAndOriginalKill 验证弱毒不覆盖强毒且旧生命及已到期归属不能漂移。
func TestPoisoningPreservesStrongerApplicationAndOriginalKill(t *testing.T) {
	w, r, target, _, _ := soulFireBallFixture(t)
	// now 是开始测试的逻辑时钟，strong 使用强毒威力。
	now := time.Unix(1800000000, 0)
	strong := pendingCast{Skill: devconfig.Skill{ID: 6, Behavior: "poisoning"}, Caster: reference(r, &r.Player), Target: target.Handle,
		TargetLife: target.Life, Scene: r.Scene, ID: 1, Due: now, PoisonColor: devconfig.PoisonGreen, Amount: 45}
	r.Conn, r.HostSession, r.Role.PrivilegeUntil = nil, "原会话", now.Add(time.Minute)
	w.resolveCast(r, strong, now)
	// original 是当前强毒的真实引用，weak 尝试用低伤害覆盖。
	original := target.Poisons.Green
	weak := strong
	weak.Amount, weak.ID = 0, 2
	w.resolveCast(r, weak, now.Add(time.Millisecond))
	if target.Poisons.Green != original || len(r.Casts) != 1 || target.HP != 996 {
		t.Fatal("弱绿毒覆盖强毒，或不受理的刷新仍扣血")
	}
	// due 指向击杀真实逻辑时间。此后的连接和特权变化不能改写已经发生的账。
	due := r.Casts[0].Due
	transitionEventHosting(r, due.Add(time.Millisecond), "新会话")
	transitionEventPrivilege(r, due.Add(time.Millisecond), time.Time{})
	target.HP = 2
	w.advanceEvents(r, due.Add(time.Second))
	if target.HP != 0 || target.Poisons != nil || len(r.KillFacts) != 1 || !r.KillFacts[0].At.Equal(due) || r.KillFacts[0].Bound || r.KillFacts[0].HostSession != "原会话" {
		t.Fatalf("毒伤击杀没有经统一归属，事实 %+v", r.KillFacts)
	}
	// target 模拟在同句柄上获得新的出生代次，旧施法不得再次影响它。
	target.HP, target.Life = 1000, target.Life+1
	w.resolveCast(r, strong, due.Add(2*time.Second))
	if target.HP != 1000 || target.Poisons != nil || len(r.KillFacts) != 1 {
		t.Fatal("旧事件命中重生后的新生命")
	}
}
