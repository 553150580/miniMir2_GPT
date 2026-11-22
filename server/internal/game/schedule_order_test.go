// 本文件回归统一事件序号不足时拒绝整批动作，避免多扣资源或只安排部分事件。
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

// TestScheduleIDExhaustionRejectsSummonBeforeMaterialAndReservation 核对「共用order耗尽后17仍取材料/预留/扣MP」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestScheduleIDExhaustionRejectsSummonBeforeMaterialAndReservation(t *testing.T) {
	// material 接收遍历布尔状态列表时当前的元素。
	for _, material := range []bool{false, true} {
		// t 提供断言失败报告、子用例和清理注册。
		t.Run(map[bool]string{false: "ordinary", true: "material"}[material], func(t *testing.T) {
			// w 接收召唤房间执行器供后续步骤使用，r 接收召唤房间执行器供后续步骤使用，now 保存由召唤房间执行器取得的当前时间。
			w, r, _, _, _, now := summonWorker(t, false)
			r.ScheduleID = math.MaxUint64
			// mp 保存当前处理所用的魔法值，role 保存当前处理所用的角色。
			mp, role := r.Player.MP, r.Role
			// code 记录结果码，使用数值表示。
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

// TestScheduleIDPreparedCastCannotDebitAfterOrderExhausted 核对「prepared动作溢出后仍扣费/放行」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestScheduleIDPreparedCastCannotDebitAfterOrderExhausted(t *testing.T) {
	// w 接收阶段b房间执行器供后续步骤使用，r 接收阶段b房间执行器供后续步骤使用。
	w, r, _ := stageBWorker(t, 1, 1)
	// now 记录本次处理开始时的时间。
	now := time.Now()
	// plan 保存由准备施法取得的计划，code 保存由准备施法取得的结果码。
	plan, code := w.prepareCast(r, 1, 10, 0, now, false)
	if code != 0 {
		t.Fatal(code)
	}
	r.ScheduleID = math.MaxUint64
	// mp 保存当前处理所用的魔法值，cast 保存施法标识，用于当前步骤中的施法，action 保存动作就绪发生时间，用于当前步骤中的动作。
	mp, cast, action := r.Player.MP, r.CastID, r.Player.ActionReadyAt
	// code 保存由“accept”“prepared”施法取得的结果码。
	if code := w.acceptPreparedCast(r, plan, 1, now); code != p.CodeSkillStateBlocked || r.Player.MP != mp || r.CastID != cast || r.Player.ActionReadyAt != action || r.eventCount() != 0 {
		t.Fatal("prepared动作溢出后仍扣费/放行", code)
	}
}

// TestScheduleIDMaterialSubmitRejectsBeforeFreshReservation 核对「临时准备后order耗尽仍占预留」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestScheduleIDMaterialSubmitRejectsBeforeFreshReservation(t *testing.T) {
	// w 接收召唤房间执行器供后续步骤使用，r 接收召唤房间执行器供后续步骤使用，now 保存由召唤房间执行器取得的当前时间。
	w, r, _, _, _, now := summonWorker(t, false)
	// plan 保存由准备施法取得的计划，code 保存由准备施法取得的结果码。
	plan, code := w.prepareCast(r, 17, 0, 0, now, true)
	if code != 0 {
		t.Fatal(code)
	}
	r.ScheduleID = math.MaxUint64
	if w.reserveSummonPet(r, plan, domain.ID(), now) || r.PendingPetBirth != nil || r.eventCount() != 0 || r.Player.MP != 1000 {
		t.Fatal("临时准备后order耗尽仍占预留")
	}
}

// TestScheduleIDMultiTargetAttackRejectsWholeAllocation 核验序号预检不应进入攻击随机计算，用诊断「多目标攻击分配部分order/扣资源」定位对应断言。
// t 提供断言失败报告、子用例和清理注册。
func TestScheduleIDMultiTargetAttackRejectsWholeAllocation(t *testing.T) {
	// maximum 接收遍历数值列表时当前的元素。
	for _, maximum := range []uint64{math.MaxUint64, math.MaxUint64 - 1} {
		// t 提供断言失败报告、子用例和清理注册。
		t.Run(map[uint64]string{math.MaxUint64: "full", math.MaxUint64 - 1: "one_slot"}[maximum], func(t *testing.T) {
			// w 接收阶段b房间执行器供后续步骤使用，r 接收阶段b房间执行器供后续步骤使用。
			w, r, _ := stageBWorker(t, 0, 25)
			r.Melee.HalfMoon = true
			// s 接收技能供后续步骤使用。
			s, _ := w.e.Store.Cfg.Fixture.Skill(25)
			// direction 保存由无符号整数“32”取得的朝向。
			direction := uint32(2)
			// front 保存从怪物集合取出的前端。
			front := r.Monsters[0]
			front.Pos = stepPoint(r.Player.Pos, direction, 1)
			if len(s.SecondaryDirectionOffsets) == 0 {
				t.Fatal("真实半月缺旁格")
			}
			// sidePoint 保存由步骤点位取得的“side”点位。
			sidePoint := stepPoint(r.Player.Pos, uint32((int32(direction)+s.SecondaryDirectionOffsets[0]+8)%8), s.SecondaryDistanceCells)
			// side 保存当前处理所用的“side”。
			side := &entity{Handle: 11, Kind: 3, HP: 100, MaxHP: 100, Pos: sidePoint, Monster: front.Monster}
			r.Monsters = append(r.Monsters, side)
			r.MonsterCount = len(r.Monsters)
			r.Player.MP = 1000
			r.ScheduleID = maximum
			// before 保存玩家，用于当前步骤中的操作前的状态，mode 保存近战，用于当前步骤中的模式。
			before, mode := r.Player, r.Melee
			w.roll = func(int64) int64 { t.Fatal("序号预检不应进入攻击随机计算"); return 0 }
			// code 保存由攻击“facing”取得的结果码。
			if code := w.attackFacing(r, direction, 0, 1, time.Now()); code != p.CodeSkillStateBlocked || !reflect.DeepEqual(r.Player, before) || r.Melee != mode || r.eventCount() != 0 || r.ScheduleID != maximum {
				t.Fatal("多目标攻击分配部分order/扣资源", code)
			}
		})
	}
}

// TestScheduleIDPaidHellFireIsolatesAndKeepsOtherEvents 核对「已付费地狱火溢出未隔离/发0order/丢队列后缀」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestScheduleIDPaidHellFireIsolatesAndKeepsOtherEvents(t *testing.T) {
	// w 接收阶段b房间执行器供后续步骤使用，r 接收阶段b房间执行器供后续步骤使用。
	w, r, _ := stageBWorker(t, 1, 9)
	w.roll = func(int64) int64 { return 0 }
	// now 记录本次处理开始时的时间。
	now := time.Now()
	// code 保存由施法“directed”取得的结果码。
	if code := w.castDirected(r, 9, 0, 1, 2, now); code != 0 {
		t.Fatal(code)
	}
	// mp 保存当前处理所用的魔法值。
	mp := r.Player.MP
	// remaining 初始化用作剩余额度的待确认攻击。
	remaining := pendingAttack{Caster: reference(r, r.Monsters[0]), Target: reference(r, &r.Player), Due: r.Casts[0].Due, Order: 2, Amount: 1}
	r.Attacks = append(r.Attacks, remaining)
	r.ScheduleID = math.MaxUint64
	w.advanceEvents(r, r.Casts[0].Due)
	if !r.Isolated || r.ScheduleID != math.MaxUint64 || r.Player.MP != mp || len(r.Casts) != 0 || len(r.Attacks) != 1 || !reflect.DeepEqual(r.Attacks[0], remaining) || r.InflightEvents != 0 {
		t.Fatal("已付费地狱火溢出未隔离/发0order/丢队列后缀")
	}
}

// TestScheduleIDFinalAvailableOrderIsUsedExactlyOnce 核对「最后合法order被提前拒绝/分配错误」与「最后order后又扣费或回绕」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestScheduleIDFinalAvailableOrderIsUsedExactlyOnce(t *testing.T) {
	// w 接收阶段b房间执行器供后续步骤使用，r 接收阶段b房间执行器供后续步骤使用。
	w, r, _ := stageBWorker(t, 1, 1)
	w.roll = func(int64) int64 { return 0 }
	r.ScheduleID = math.MaxUint64 - 1
	// now 记录本次处理开始时的时间。
	now := time.Now()
	// code 保存由施法取得的结果码。
	if code := w.cast(r, 1, 10, 1, now); code != 0 || r.ScheduleID != math.MaxUint64 || len(r.Casts) != 1 || r.Casts[0].Order != math.MaxUint64 {
		t.Fatal("最后合法order被提前拒绝/分配错误", code)
	}
	// mp 保存当前处理所用的魔法值。
	mp := r.Player.MP
	// code 保存由施法取得的结果码。
	if code := w.cast(r, 1, 10, 2, now.Add(time.Hour)); code != p.CodeSkillStateBlocked || r.Player.MP != mp || len(r.Casts) != 1 || r.ScheduleID != math.MaxUint64 {
		t.Fatal("最后order后又扣费或回绕", code)
	}
}
