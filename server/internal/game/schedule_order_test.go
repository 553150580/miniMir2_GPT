package game

import (
	"math"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/domain"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
	"reflect"
	"testing"
	"time"
)

func TestScheduleIDExhaustionRejectsSummonBeforeMaterialAndReservation(t *testing.T) {
	for _, material := range []bool{false, true} {
		t.Run(map[bool]string{false: "ordinary", true: "material"}[material], func(t *testing.T) {
			w, r, _, _, _, now := summonWorker(t, false)
			r.ScheduleID = math.MaxUint64
			mp, role := r.Player.MP, r.Role
			var code uint32
			if material {
				_, code = w.prepareMaterialCast(r, persistence.AssetRequest{SkillID: 17}, &pb.CastWithMaterialRequest{SkillId: 17}, now)
			} else {
				code = w.cast(r, 17, 0, 1, now)
			}
			if code != p.CodeSkillStateBlocked || r.Player.MP != mp || !reflect.DeepEqual(r.Role, role) || r.PendingPetBirth != nil || r.eventCount() != 0 || r.CastID != 0 {
				t.Fatal("共用order耗尽后17仍取材料/预留/扣MP", code)
			}
		})
	}
}

func TestScheduleIDPreparedCastCannotDebitAfterOrderExhausted(t *testing.T) {
	w, r, _ := stageBWorker(t, 1, 1)
	now := time.Now()
	plan, code := w.prepareCast(r, 1, 10, 0, now, false)
	if code != 0 {
		t.Fatal(code)
	}
	r.ScheduleID = math.MaxUint64
	mp, cast, action := r.Player.MP, r.CastID, r.Player.ActionReadyAt
	if code := w.acceptPreparedCast(r, plan, 1, now); code != p.CodeSkillStateBlocked || r.Player.MP != mp || r.CastID != cast || r.Player.ActionReadyAt != action || r.eventCount() != 0 {
		t.Fatal("prepared动作溢出后仍扣费/放行", code)
	}
}

func TestScheduleIDMaterialSubmitRejectsBeforeFreshReservation(t *testing.T) {
	w, r, _, _, _, now := summonWorker(t, false)
	plan, code := w.prepareCast(r, 17, 0, 0, now, true)
	if code != 0 {
		t.Fatal(code)
	}
	r.ScheduleID = math.MaxUint64
	if w.reserveSummonPet(r, plan, domain.ID(), now) || r.PendingPetBirth != nil || r.eventCount() != 0 || r.Player.MP != 1000 {
		t.Fatal("临时准备后order耗尽仍占预留")
	}
}

func TestScheduleIDMultiTargetAttackRejectsWholeAllocation(t *testing.T) {
	for _, maximum := range []uint64{math.MaxUint64, math.MaxUint64 - 1} {
		t.Run(map[uint64]string{math.MaxUint64: "full", math.MaxUint64 - 1: "one_slot"}[maximum], func(t *testing.T) {
			w, r, _ := stageBWorker(t, 0, 25)
			r.Melee.HalfMoon = true
			s, _ := w.e.Store.Cfg.Fixture.Skill(25)
			direction := uint32(2)
			front := r.Monsters[0]
			front.Pos = stepPoint(r.Player.Pos, direction, 1)
			if len(s.SecondaryDirectionOffsets) == 0 {
				t.Fatal("真实半月缺旁格")
			}
			sidePoint := stepPoint(r.Player.Pos, uint32((int32(direction)+s.SecondaryDirectionOffsets[0]+8)%8), s.SecondaryDistanceCells)
			side := &entity{Handle: 11, Kind: 3, HP: 100, MaxHP: 100, Pos: sidePoint, Monster: front.Monster}
			r.Monsters = append(r.Monsters, side)
			r.MonsterCount = len(r.Monsters)
			r.Player.MP = 1000
			r.ScheduleID = maximum
			before, mode := r.Player, r.Melee
			w.roll = func(int64) int64 { t.Fatal("序号预检不应进入攻击随机计算"); return 0 }
			if code := w.attackFacing(r, direction, 0, 1, time.Now()); code != p.CodeSkillStateBlocked || !reflect.DeepEqual(r.Player, before) || r.Melee != mode || r.eventCount() != 0 || r.ScheduleID != maximum {
				t.Fatal("多目标攻击分配部分order/扣资源", code)
			}
		})
	}
}

func TestScheduleIDPaidHellFireIsolatesAndKeepsOtherEvents(t *testing.T) {
	w, r, _ := stageBWorker(t, 1, 9)
	w.roll = func(int64) int64 { return 0 }
	now := time.Now()
	if code := w.castDirected(r, 9, 0, 1, 2, now); code != 0 {
		t.Fatal(code)
	}
	mp := r.Player.MP
	remaining := pendingAttack{Caster: reference(r, r.Monsters[0]), Target: reference(r, &r.Player), Due: r.Casts[0].Due, Order: 2, Amount: 1}
	r.Attacks = append(r.Attacks, remaining)
	r.ScheduleID = math.MaxUint64
	w.advanceEvents(r, r.Casts[0].Due)
	if !r.Isolated || r.ScheduleID != math.MaxUint64 || r.Player.MP != mp || len(r.Casts) != 0 || len(r.Attacks) != 1 || !reflect.DeepEqual(r.Attacks[0], remaining) || r.InflightEvents != 0 {
		t.Fatal("已付费地狱火溢出未隔离/发0order/丢队列后缀")
	}
}

func TestScheduleIDFinalAvailableOrderIsUsedExactlyOnce(t *testing.T) {
	w, r, _ := stageBWorker(t, 1, 1)
	w.roll = func(int64) int64 { return 0 }
	r.ScheduleID = math.MaxUint64 - 1
	now := time.Now()
	if code := w.cast(r, 1, 10, 1, now); code != 0 || r.ScheduleID != math.MaxUint64 || len(r.Casts) != 1 || r.Casts[0].Order != math.MaxUint64 {
		t.Fatal("最后合法order被提前拒绝/分配错误", code)
	}
	mp := r.Player.MP
	if code := w.cast(r, 1, 10, 2, now.Add(time.Hour)); code != p.CodeSkillStateBlocked || r.Player.MP != mp || len(r.Casts) != 1 || r.ScheduleID != math.MaxUint64 {
		t.Fatal("最后order后又扣费或回绕", code)
	}
}
