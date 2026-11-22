// 本文件回归托管计量在非计费时间、换图、复活、数据库在途、余额耗尽及回收时的边界。
package game

import (
	"encoding/json"
	"testing"
	"time"

	pb "minimir/api/generated/minimirv1"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
	"minimir/internal/wire"
)

// TestHostingFractionSurvivesNonBillablePeriods 核对「不计费期间丢失余量或发起扣时」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestHostingFractionSurvivesNonBillablePeriods(t *testing.T) {
	// gate 接收遍历文本列表时当前的元素。
	for _, gate := range []string{"online", "pending", "town", "dead", "zero_balance", "auto_off", "not_ready", "isolated", "stopped"} {
		// t 提供断言失败报告、子用例和清理注册。
		t.Run(gate, func(t *testing.T) {
			// w 接收完整或满额背包策略房间执行器供后续步骤使用，r 接收完整或满额背包策略房间执行器供后续步骤使用。
			w, r, _ := fullBagPolicyWorker(t)
			r.HostMS = 800
			r.Monsters[0].HP, r.Monsters[0].MaxHP = 1000, 1000
			switch gate {
			case "pending":
				r.Conn, r.Pending = nil, true
			case "town":
				r.Conn, r.MapID, r.MonsterCount, r.Monsters = nil, 2, 0, nil
				// m 接收地图“by”标识供后续步骤使用。
				m, _ := w.e.Store.Cfg.Fixture.MapByID(2)
				r.Player.Pos = m.PlayerSpawn
			case "dead":
				r.Conn, r.Player.HP = nil, 0
			case "zero_balance":
				r.Conn, r.Role.HostingSeconds = nil, 0
			case "auto_off":
				r.Conn, r.Auto = nil, false
			case "not_ready":
				r.Ready = false
			case "isolated":
				r.Isolated = true
			case "stopped":
				w.e.stopped.Store(true)
			}
			// now 保存由Unix 时间取得的当前时间。
			now := time.Unix(1800000000, 0)
			r.AwaitUntil = now.Add(time.Minute)
			r.LastTick = now.Add(-200 * time.Millisecond)
			// before 保存角色剩余托管秒数，用于当前步骤中的操作前的状态。
			before := r.Role.HostingSeconds
			w.step(r, now)
			if r.HostMS != 800 || r.Role.HostingSeconds != before || len(w.e.jobs) != 0 {
				t.Fatalf("不计费期间丢失余量或发起扣时: fraction=%d balance=%d jobs=%d", r.HostMS, r.Role.HostingSeconds, len(w.e.jobs))
			}
		})
	}
}

// TestHostingFractionSurvivesHealthyTravelAndRevive 核对「正常复活丢失已经模拟的托管余量」与「城镇清除了原个人托管余量」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestHostingFractionSurvivesHealthyTravelAndRevive(t *testing.T) {
	// w 接收地图切换房间执行器供后续步骤使用，r 接收地图切换房间执行器供后续步骤使用，c 接收地图切换房间执行器供后续步骤使用，now 保存由地图切换房间执行器取得的当前时间。
	w, r, c, now := travelWorker(t)
	r.HostMS = 800
	prepareTravelAt(w, r, 1, travelToTown, now)
	confirmTravelAt(w, r, 1, r.Scene, false, now)
	if r.MapID != 2 {
		t.Fatal("没有进入城镇")
	}
	w.command(command{role: r.Role.ID, conn: c, frame: wire.Message(p.MsgC2SEnterWorldAck, 1, uint64(r.Auth.SessionEpoch), &pb.EnterWorldAck{SceneEpoch: r.Scene, BaselineId: r.Baseline})})
	r.Conn = nil
	r.LastTick = now
	w.step(r, now.Add(100*time.Millisecond))
	if r.HostMS != 800 {
		t.Fatal("城镇清除了原个人托管余量")
	}
	r.Conn, r.Ready = c, true
	prepareTravelAt(w, r, 2, travelToPersonal, now.Add(time.Second))
	confirmTravelAt(w, r, 2, r.Scene, false, now.Add(time.Second))
	if r.MapID != w.e.Store.Cfg.Fixture.Map.ID || r.HostMS != 800 {
		t.Fatal("返回个人现场丢失余量")
	}
	r.Player.HP = 0
	w.revivePlayer(r)
	if r.Player.HP <= 0 || r.HostMS != 800 {
		t.Fatal("正常复活丢失已经模拟的托管余量")
	}
}

// TestHostingFractionExcludesPendingDatabaseWait 核对「恢复后的实际200ms未累计整秒，或提前结算」与「DB等待混入余量或原余量丢失」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestHostingFractionExcludesPendingDatabaseWait(t *testing.T) {
	// w 接收完整或满额背包策略房间执行器供后续步骤使用，r 接收完整或满额背包策略房间执行器供后续步骤使用。
	w, r, _ := fullBagPolicyWorker(t)
	r.Conn, r.Pending, r.HostMS = nil, true, 800
	r.Monsters[0].HP, r.Monsters[0].MaxHP = 1000, 1000
	// now 保存由Unix 时间取得的当前时间。
	now := time.Unix(1800000000, 0)
	r.LastTick = now.Add(-5 * time.Second)
	w.step(r, now)
	r.Pending = false
	w.step(r, now.Add(100*time.Millisecond))
	if r.HostMS != 900 || len(w.e.jobs) != 0 || r.Role.HostingSeconds != 3 {
		t.Fatal("DB等待混入余量或原余量丢失")
	}
	w.step(r, now.Add(200*time.Millisecond))
	if r.HostMS != 0 || r.hostSecondsOpen != 1 || r.Pending || len(w.e.jobs) != 0 || r.Role.HostingSeconds != 3 {
		t.Fatal("恢复后的实际200ms未累计整秒，或提前结算")
	}
}

// TestHostingFractionIgnoresQueuedTickBeforeDisconnect 核验断线应设置托管时钟起点，用诊断「健康接管应在旧绑定上结清整秒后再连接」定位对应断言。
// t 提供断言失败报告、子用例和清理注册。
func TestHostingFractionIgnoresQueuedTickBeforeDisconnect(t *testing.T) {
	// w 接收完整或满额背包策略房间执行器供后续步骤使用，r 接收完整或满额背包策略房间执行器供后续步骤使用，c 接收完整或满额背包策略房间执行器供后续步骤使用。
	w, r, c := fullBagPolicyWorker(t)
	r.HostMS, r.MonsterCount = 900, 0
	w.command(command{role: r.Role.ID, conn: c, disconnect: true})
	// detached 保存上一次状态模拟帧，用于当前步骤中的脱离。
	detached := r.LastTick
	if r.Conn != nil || detached.IsZero() {
		t.Fatal("断线没有设置托管时钟起点")
	}
	w.step(r, detached.Add(-60*time.Millisecond))
	if !r.LastTick.Equal(detached) || r.HostMS != 900 {
		t.Fatal("排队的在线帧倒拨了托管时钟")
	}
	w.step(r, detached.Add(40*time.Millisecond))
	if r.HostMS != 940 || len(w.e.jobs) != 0 || r.Pending {
		t.Fatal("断线前的在线时长进入扣时事务")
	}
	w.step(r, detached.Add(100*time.Millisecond))
	if r.HostMS != 0 || r.hostSecondsOpen != 1 || len(w.e.jobs) != 0 || r.Pending {
		t.Fatal("真实托管余量满一秒时应累计整秒")
	}
	w.command(command{role: r.Role.ID, conn: &capturedConnection{}, ticket: "待接管"})
	if r.Conn != nil || r.hostSecondsOpen != 0 || len(r.Waiting) != 1 || len(w.e.jobs) != 1 || !r.Pending {
		t.Fatal("健康接管应在旧绑定上结清整秒后再连接")
	}
}

// TestHostingColdWorldDoesNotRestoreOldFraction 核对「故障新现场继承了旧临时计量」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestHostingColdWorldDoesNotRestoreOldFraction(t *testing.T) {
	// w 接收完整或满额背包策略房间执行器供后续步骤使用，r 接收完整或满额背包策略房间执行器供后续步骤使用。
	w, r, _ := fullBagPolicyWorker(t)
	r.HostMS = 800
	w.initialize(r, r.Role)
	if r.HostMS != 0 {
		t.Fatal("故障新现场继承了旧临时计量")
	}
}

// TestHostingBatchesThreeSecondsAndClipsBalanceBoundary 核对「最后一秒超额累计，或在耗尽后的帧产生新攻击」与「冻结检查点后仍重复持有整秒，或提前公布未确认余额」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestHostingBatchesThreeSecondsAndClipsBalanceBoundary(t *testing.T) {
	// w 接收完整或满额背包策略房间执行器供后续步骤使用，r 接收完整或满额背包策略房间执行器供后续步骤使用。
	w, r, _ := fullBagPolicyWorker(t)
	r.Conn, r.MonsterCount, r.Role.HostingSeconds = nil, 0, 10
	// now 保存由Unix 时间取得的当前时间。
	now := time.Unix(1800000000, 0)
	r.LastTick = now
	// i 标记循环或列表中的当前位置。
	for i := 1; i <= 29; i++ {
		w.step(r, now.Add(time.Duration(i)*100*time.Millisecond))
		if r.Pending || len(w.e.jobs) != 0 || r.Role.HostingSeconds != 10 {
			t.Fatalf("第%d帧提前提交整秒", i)
		}
	}
	if r.hostSecondsOpen != 2 || r.HostMS != 900 {
		t.Fatal("达到检查点前累计计量不正确")
	}
	// at 保存由新增取得的发生时间。
	at := now.Add(3 * time.Second)
	w.step(r, at)
	assertHostingJob(t, w, r, persistence.Settlement{SourceSeq: 5, HostSeconds: 3, HostingSession: r.HostSession, EventAt: at})
	if r.hostSecondsOpen != 0 || r.HostMS != 0 || r.Role.HostingSeconds != 10 {
		t.Fatal("冻结检查点后仍重复持有整秒，或提前公布未确认余额")
	}

	w, r, _ = fullBagPolicyWorker(t)
	r.Conn, r.MonsterCount, r.Role.HostingSeconds, r.HostMS = nil, 0, 1, 950
	r.LastTick = now.Add(-200 * time.Millisecond)
	w.step(r, now)
	assertHostingJob(t, w, r, persistence.Settlement{SourceSeq: 5, HostSeconds: 1, HostingSession: r.HostSession, EventAt: now})
	if r.HostMS != 0 || r.hostSecondsOpen != 0 || r.Player.AttackSequence != 0 {
		t.Fatal("最后一秒超额累计，或在耗尽后的帧产生新攻击")
	}
}

// TestHostingMergesWithOnlyMatchingSettlement 核验同会话事务已绑定原整秒，不能重复累计，以及旧会话击杀不能扣除当前区间的时长。
// t 提供断言失败报告、子用例和清理注册。
func TestHostingMergesWithOnlyMatchingSettlement(t *testing.T) {
	// reason 接收遍历文本列表时当前的元素。
	for _, reason := range []string{"击杀", "磨损", "旧会话击杀", "死亡"} {
		// t 提供断言失败报告、子用例和清理注册。
		t.Run(reason, func(t *testing.T) {
			// w 接收完整或满额背包策略房间执行器供后续步骤使用，r 接收完整或满额背包策略房间执行器供后续步骤使用。
			w, r, _ := fullBagPolicyWorker(t)
			r.Conn, r.hostSecondsOpen, r.HostMS = nil, 2, 400
			// q 组装结算所需的数据。
			q := persistence.Settlement{SourceSeq: 5, Kills: 1, HostingSession: r.HostSession}
			switch reason {
			case "磨损":
				q.Kills, q.HostingSession = 0, ""
			case "旧会话击杀":
				q.HostingSession = "旧会话"
			case "死亡":
				r.Player.HP, q.Kills, q.StopReason = 0, 0, 2
			}
			w.settle(r, q)
			if reason != "旧会话击杀" {
				q.HostingSession, q.HostSeconds = r.HostSession, 2
				if r.hostSecondsOpen != 0 {
					t.Fatal("同会话事务已绑定原整秒，不能重复累计")
				}
			} else if r.hostSecondsOpen != 2 {
				t.Fatal("旧会话击杀不能扣除当前区间的时长")
			}
			assertHostingJob(t, w, r, q)
			if r.HostMS != 400 {
				t.Fatal("原未满秒余量应保留")
			}
		})
	}
}

// TestHostingClosesBeforeIdleReleaseAndLeavesIsolatedRoomFrozen 核验死亡房间仍有未确认时长，不能回收，以及隔离房间不应以健康身份另行结算。
// t 提供断言失败报告、子用例和清理注册。
func TestHostingClosesBeforeIdleReleaseAndLeavesIsolatedRoomFrozen(t *testing.T) {
	// w 接收完整或满额背包策略房间执行器供后续步骤使用，r 接收完整或满额背包策略房间执行器供后续步骤使用。
	w, r, _ := fullBagPolicyWorker(t)
	r.Conn, r.Player.HP, r.hostSecondsOpen, r.HostMS = nil, 0, 1, 350
	// now 保存由Unix 时间取得的当前时间。
	now := time.Unix(1800000000, 0)
	r.LastTick, r.idleSince = now.Add(-100*time.Millisecond), now.Add(-offlineRoomRetention)
	if w.expireIdleRoom(r.Role.ID, r, now) {
		t.Fatal("死亡房间仍有未确认时长，不能回收")
	}
	r.Isolated = true
	if w.drainRoomSettlement(r, now) || len(w.e.jobs) != 0 || r.hostSecondsOpen != 1 {
		t.Fatal("隔离房间不应以健康身份另行结算")
	}
	r.Isolated = false
	w.step(r, now)
	assertHostingJob(t, w, r, persistence.Settlement{SourceSeq: 5, HostSeconds: 1, HostingSession: r.HostSession, EventAt: now})
	if r.HostMS != 350 || w.expireIdleRoom(r.Role.ID, r, now) {
		t.Fatal("终态误计时或在确认前删除房间")
	}
}

// assertHostingJob 断言离线托管任务或职业编号。
// t 提供断言失败报告、子用例和清理注册，w 记录房间执行器，r 指向当前执行器拥有并更新的目标房间，expected 记录预期结果，使用结算表示。
func assertHostingJob(t *testing.T, w *worker, r *room, expected persistence.Settlement) {
	t.Helper()
	// raw 保存由序列化取得的原始值，err 接收序列化过程中可能产生的错误，供后续失败分支处理。
	raw, err := json.Marshal(expected)
	if err != nil {
		t.Fatal(err)
	}
	if !r.Pending || len(w.e.jobs) != 1 || r.PendingFingerprint != persistence.Hash(raw) {
		t.Fatalf("冻结的结算请求与原身份或计量不符：%+v", expected)
	}
}
