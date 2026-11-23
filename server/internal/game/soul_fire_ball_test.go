// 本文件验证灵魂火符的真实材料门禁、按道术计算的弹道及原生命结算。
package game

import (
	"testing"
	"time"

	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
)

// soulFireBallFixture 只为测试选择技能和显式护符单位，保留产品护符入口状态。
func soulFireBallFixture(t *testing.T) (*worker, *room, *entity, persistence.AssetRequest, *pb.CastWithMaterialRequest) {
	t.Helper()
	w, r, _ := stageAWorker(t, 2)
	// f 由该测试独占。把道术和魔法置为不同定值来证明专属伤害路径。
	f := &w.e.Store.Cfg.Fixture
	f.PlayerGrowth = nil
	for i := range f.Players {
		if f.Players[i].JobID == 2 {
			f.Players[i].TaoMin, f.Players[i].TaoMax = 20, 20
			f.Players[i].MagicMin, f.Players[i].MagicMax = 1, 1
		}
	}
	// skill 保留 Crystal 给出的道术弹道、等级零消耗和技能基本威力。
	skill := devconfig.Skill{ID: 13, JobID: 2, Behavior: "soul_fire_ball", TargetKind: "enemy_entity", RangeCells: 9,
		MPBaseCost: 3, MPLevelCost: 1, MaxLevel: 3, DelayBaseMS: 500, ProjectileMSPerCell: 50, TargetMoveToleranceCells: 2,
		RequireLineOfSight: true, ActionMS: 600, GlobalSpellMS: 1800, PowerBase: 3, MPowerBase: 8, MultiplierBasePPM: 1000000,
		MaterialItemID: 187, MaterialQuantity: 1}
	if _, found := f.Skill(skill.ID); !found {
		f.Skills = append(f.Skills, skill)
	}
	// item 是隔离测试下已装备且有版本的原护符实例。
	item := domain.Item{ID: domain.ID(), ItemID: 187, Quantity: 2, Version: 4}
	r.Role.Equipped = []domain.Equipped{{Slot: devconfig.CastingSupplySlot, Item: item}}
	r.Role.Skills = append(r.Role.Skills, domain.LearnedSkill{SkillID: skill.ID})
	f.CastingSupply.Enabled = true
	r.Player.Pos, r.Player.MP = devconfig.Point{X: 4, Y: 5}, 30
	// target 是原场景中用于验证弹道的怪物，其魔防固定为5。
	target := r.Monsters[0]
	target.Pos = devconfig.Point{X: 8, Y: 5}
	target.Monster.MagicDefenceMin, target.Monster.MagicDefenceMax, target.Monster.MagicResist = 5, 5, 0
	w.roll = func(int64) int64 { return 0 }
	// q 和 request 通常由正式材料请求解码器填写。
	q := persistence.AssetRequest{SkillID: skill.ID, SceneEpoch: r.Scene, CasterLife: r.Player.Life, ItemID: item.ID, ItemVersion: uint64(item.Version)}
	request := &pb.CastWithMaterialRequest{SkillId: skill.ID, TargetHandle: target.Handle, Direction: 2}
	return w, r, target, q, request
}

// TestSoulFireBallMaterialFlightAndSameLife 验证受理前不扣蓝、原回执只排一次、两格内同生命由道术和魔防结算。
func TestSoulFireBallMaterialFlightAndSameLife(t *testing.T) {
	w, r, target, q, request := soulFireBallFixture(t)
	// now 是测试的受理时钟。
	now := time.Unix(1800000000, 0)
	if code := w.cast(r, 13, target.Handle, 1, now); code != p.CodeSkillMaterialRequired {
		t.Fatalf("无材料入口返回 %d", code)
	}
	// plan 在未收到持久回执时只计算道术、弹道和费用，禁止改变房间状态。
	plan, code := w.prepareMaterialCast(r, q, request, now)
	if code != 0 || plan == nil || plan.Amount != 25 || plan.DelayMS != 700 || r.Player.MP != 30 || len(r.Casts) != 0 {
		t.Fatalf("灵魂火符计划不符，结果 %d，计划 %+v", code, plan)
	}
	// receipt 代表持久层返回的原护符消费，用于验证现场回执水位。
	receipt := persistence.Operation{Kind: "cast_material", CommandSeq: 1,
		Casting: &persistence.CastingReceipt{Scope: "manual", InstanceID: q.ItemID, SkillID: 13, Quantity: 1}}
	if !w.applyMaterialReceipt(r, receipt, plan, 1, now) || w.applyMaterialReceipt(r, receipt, plan, 1, now) || r.Player.MP != 27 || len(r.Casts) != 1 {
		t.Fatal("原材料回执没有恰好受理一次")
	}
	target.Pos.X += 2
	w.advanceEvents(r, now.Add(699*time.Millisecond))
	if target.HP != 1000 {
		t.Fatal("灵魂火符在弹道到期前扣血")
	}
	w.advanceEvents(r, now.Add(700*time.Millisecond))
	if target.HP != 980 {
		t.Fatalf("两格内同生命应承受25点原伤害减5点魔防，剩余 %d", target.HP)
	}
}

// TestSoulFireBallRejectsChangedMaterialAndObstruction 验证旧实例、旧版本和遮挡路径无法受理带材请求。
func TestSoulFireBallRejectsChangedMaterialAndObstruction(t *testing.T) {
	w, r, _, q, request := soulFireBallFixture(t)
	now := time.Unix(1800000000, 0)
	q.ItemVersion--
	if _, code := w.prepareMaterialCast(r, q, request, now); code != p.CodeSkillMaterialChanged {
		t.Fatalf("旧护符版本返回 %d", code)
	}
	q.ItemVersion++
	// m 在施法者和目标之间设置确认过的视线阻挡，角色状态保持原值。
	m := &w.e.Store.Cfg.Fixture.Map
	m.CollisionBits = nil
	m.Blocked = []devconfig.Point{{X: 6, Y: 5}}
	if _, code := w.prepareMaterialCast(r, q, request, now); code != p.CodeSkillTarget || r.Player.MP != 30 || len(r.Casts) != 0 {
		t.Fatalf("被地形遮挡的灵魂火符不应受理，结果 %d", code)
	}
}
