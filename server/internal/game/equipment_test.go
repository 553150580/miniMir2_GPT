package game

import (
	"math"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
	"minimir/internal/wire"
	"reflect"
	"testing"
	"time"
)

func equipmentWorker(t *testing.T, job uint32) (*worker, *room, *capturedConnection) {
	t.Helper()
	w, r, c := skillWorker(t, job)
	if w.e.Store.Cfg.Fixture.RuleVersion != 23 {
		t.Fatal("装备管线测试必须从真实rule22目录派生")
	}
	// 仅隔离非零MC/SC参数；主目录木剑/布衣仍是来源零，未新增装备。
	for n := range w.e.Store.Cfg.Fixture.Items {
		i := &w.e.Store.Cfg.Fixture.Items[n]
		if i.ID == 218 {
			i.MagicMinBonus, i.MagicMaxBonus, i.TaoMinBonus, i.TaoMaxBonus = 1, 3, 2, 4
		}
	}
	r.Player.Life = 4
	return w, r, c
}

func TestEquipmentFourEndpointsAndSupplySlotDoNotChangePet(t *testing.T) {
	for _, job := range []uint32{0, 1, 2} {
		t.Run(string(rune('0'+job)), func(t *testing.T) {
			w, r, _ := equipmentWorker(t, job)
			base := w.fighter(r, &r.Player)
			petBefore := w.fighter(r, r.Pet)
			r.Role.Equipped = []domain.Equipped{{Slot: 0, Item: domain.Item{ID: domain.ID(), ItemID: 218, Quantity: 1, Version: 1}}, {Slot: 100, Item: domain.Item{ID: domain.ID(), ItemID: 218, Quantity: 1, Version: 1}}}
			a := w.fighter(r, &r.Player)
			if a.MagicMin != base.MagicMin+1 || a.MagicMax != base.MagicMax+3 || a.TaoMin != base.TaoMin+2 || a.TaoMax != base.TaoMax+4 || a.AttackMin != base.AttackMin+2 || a.AttackMax != base.AttackMax+5 {
				t.Fatalf("装备端点未唯一叠加或护符槽重复加成: %+v %+v", base, a)
			}
			if !reflect.DeepEqual(petBefore, w.fighter(r, r.Pet)) {
				t.Fatal("主人装备改写原宠物属性")
			}
			attrs := w.combatAttributesPB(r, &r.Player)
			if attrs.MagicAttackMin != a.MagicMin || attrs.SpiritAttackMax != a.TaoMax {
				t.Fatal("权威属性PB未投影MC/SC")
			}
			r.Role.Equipped = nil
			if !reflect.DeepEqual(w.fighter(r, &r.Player), base) {
				t.Fatal("卸下后加成未消除")
			}
		})
	}
}

func TestEquipmentMagicCastFreezesAcceptedAmountAndSourceFormula(t *testing.T) {
	w, r, _ := equipmentWorker(t, 1)
	now := time.Now()
	s, _ := w.e.Store.Cfg.Fixture.Skill(1)
	r.Role.Equipped = []domain.Equipped{{Slot: 0, Item: domain.Item{ItemID: 218}}}
	w.roll = func(n int64) int64 { return n - 1 }
	stats := w.fighter(r, &r.Player)
	if code := w.cast(r, 1, r.Monsters[0].Handle, 1, now); code != 0 {
		t.Fatal(code)
	}
	if len(r.Casts) != 1 {
		t.Fatal("未生成真实到期事件")
	}
	expected := s.SourceDamage(s.LearnedLevel, stats.MagicMax, s.SourcePower(s.LearnedLevel, s.MPowerBase+max(int64(0), s.MPowerBonus-1), s.PowerBase+max(int64(0), s.PowerBonus-1)))
	accepted := r.Casts[0]
	if accepted.Amount != expected {
		t.Fatalf("装备MC没有走原Single公式: got=%d expected=%d", accepted.Amount, expected)
	}
	r.Role.Equipped = nil
	w.refreshPlayerStats(r)
	if r.Casts[0].Amount != expected {
		t.Fatal("卸装重算已受理施法")
	}
	r.Monsters[0].HP, r.Monsters[0].MaxHP = 1000, 1000
	r.Monsters[0].Monster = &devconfig.Monster{Fighter: devconfig.Fighter{MagicResistWeight: 10}}
	w.advanceEvents(r, accepted.Due)
	if r.Monsters[0].HP != 1000-expected {
		t.Fatal("到期没有使用原冻结量")
	}
}

func TestEquipmentSpiritHealingAndDurationUseOnlySC(t *testing.T) {
	w, r, _ := equipmentWorker(t, 2)
	now := time.Now()
	r.Player.HP = 1
	r.Role.Equipped = []domain.Equipped{{Slot: 0, Item: domain.Item{ItemID: 218}}}
	w.roll = func(n int64) int64 { return n - 1 }
	stats := w.fighter(r, &r.Player)
	s, _ := w.e.Store.Cfg.Fixture.Skill(2)
	plan, code := w.prepareCast(r, 2, r.Player.Handle, 0, now, false)
	if code != 0 {
		t.Fatal(code)
	}
	expected := s.SourceDamage(s.LearnedLevel, stats.TaoMax*s.HealingStatMultiplier, s.SourcePower(s.LearnedLevel, s.MPowerBase+max(int64(0), s.MPowerBonus-1), s.PowerBase+max(int64(0), s.PowerBonus-1)))
	if s.AddCasterLevel {
		expected += int64(r.Role.Level)
	}
	if plan.Amount != expected {
		t.Fatal("SC治疗不符合源公式")
	}
	for n := range w.e.Store.Cfg.Fixture.Items {
		if w.e.Store.Cfg.Fixture.Items[n].ID == 218 {
			w.e.Store.Cfg.Fixture.Items[n].MagicMinBonus = 100
			w.e.Store.Cfg.Fixture.Items[n].MagicMaxBonus = 100
		}
	}
	other, code := w.prepareCast(r, 2, r.Player.Handle, 0, now, false)
	if code != 0 || other.Amount != expected {
		t.Fatal("MC错误加入道士治疗")
	}
	statusSkill, _ := w.e.Store.Cfg.Fixture.Skill(15)
	definition, _ := w.e.Store.Cfg.Fixture.Buff(statusSkill.BuffID)
	if seconds := buffDurationSeconds(statusSkill, definition, statusSkill.LearnedLevel, stats.TaoMax, 0); seconds != stats.TaoMax*definition.StatMultiplier+definition.DurationSecondsByLevel[statusSkill.LearnedLevel] {
		t.Fatal("SC时长未按原公式")
	}
}

func TestEquipmentAssetRefreshDoesNotHealReviveOrResetLifeState(t *testing.T) {
	for _, dead := range []bool{false, true} {
		t.Run(map[bool]string{false: "受伤", true: "死亡"}[dead], func(t *testing.T) {
			w, r, _ := equipmentWorker(t, 1)
			r.Role.ID, r.Auth.RoleID = "role", "role"
			r.Role.Revision, r.Role.EconomicSeq = 5, 2
			r.Player.HP, r.Player.MP = 1, 1
			if dead {
				r.Player.HP = 0
				r.DeathID = "original-death"
			}
			destination := devconfig.Point{X: 6, Y: 8}
			r.Destination = &destination
			deadline := time.Now().Add(time.Minute)
			r.NextSpell = deadline
			r.SkillCooldowns = map[uint32]time.Time{1: deadline}
			r.Player.Statuses = map[uint32]*combatStatus{31: {ID: 31, TargetLife: r.Player.Life, RemainingMS: 10000, DamageReductionPercent: 20}}
			status := r.Player.Statuses[31]
			petBefore := w.fighter(r, r.Pet)
			id := domain.ID()
			r.Pending = true
			r.AssetGeneration = 8
			r.PendingOperationID = id
			r.PendingFingerprint = "equip-original"
			binding := assetBinding{room: r, auth: r.Auth, generation: 8, id: id, fingerprint: r.PendingFingerprint}
			next := r.Role
			next.Revision++
			next.EconomicSeq++
			next.Equipped = []domain.Equipped{{Slot: 0, Item: domain.Item{ID: domain.ID(), ItemID: 218, Quantity: 1, Version: 2}}}
			w.finishAsset(binding, persistence.AssetRequest{Kind: "equip"}, wire.Frame{ID: p.MsgC2SEquip}, "", next, &pb.OperationResult{NewRevision: uint64(next.Revision)}, persistence.Operation{}, nil, nil)
			if r.Player.HP != map[bool]int64{false: 1, true: 0}[dead] || r.Player.MP != 1 || r.Player.Life != 4 || r.Destination != &destination || r.NextSpell != deadline || r.SkillCooldowns[1] != deadline || r.Player.Statuses[31] != status || !reflect.DeepEqual(petBefore, w.fighter(r, r.Pet)) {
				t.Fatal("装备刷新改变资源/生命/状态/导航/宠物")
			}
			if dead && r.DeathID != "original-death" {
				t.Fatal("装备伪造复活")
			}
			// 重放旧callback不能采用新Role或清理新的Pending。
			r.Pending = true
			r.AssetGeneration++
			r.PendingOperationID = domain.ID()
			newID := r.PendingOperationID
			w.finishAsset(binding, persistence.AssetRequest{Kind: "equip"}, wire.Frame{}, "", next, &pb.OperationResult{}, persistence.Operation{}, nil, nil)
			if !r.Pending || r.PendingOperationID != newID {
				t.Fatal("旧装备回调清除了新任务")
			}
		})
	}
}

func TestEquipmentOverflowCannotAcceptCastOrChargeMana(t *testing.T) {
	w, r, _ := equipmentWorker(t, 2)
	for n := range w.e.Store.Cfg.Fixture.Items {
		if w.e.Store.Cfg.Fixture.Items[n].ID == 218 {
			w.e.Store.Cfg.Fixture.Items[n].TaoMaxBonus = math.MaxInt64 / 2
		}
	}
	r.Role.Equipped = []domain.Equipped{{Slot: 0, Item: domain.Item{ItemID: 218}}}
	r.Player.HP = 1
	mp := r.Player.MP
	if _, code := w.prepareCast(r, 2, r.Player.Handle, 0, time.Now(), false); code != p.CodeSkillStateBlocked || r.Player.MP != mp || len(r.Casts) != 0 {
		t.Fatal("SC中间值溢出仍受理或扣费")
	}
}
