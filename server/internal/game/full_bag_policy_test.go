// 本文件回归背包满时普通自动战斗、离线扣时、击杀事实和授权门禁。
package game

import (
	"encoding/json"
	"fmt"
	"google.golang.org/protobuf/proto"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/domain"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
	"minimir/internal/wire"
	"testing"
	"time"
)

// 独立战斗/容量资格；不代表自然养成速度或正式掉率验收。
// t 提供断言失败报告、子用例和清理注册。
func fullBagPolicyWorker(t *testing.T) (*worker, *room, *capturedConnection) {
	t.Helper()
	// w 接收阶段b房间执行器供后续步骤使用，r 接收阶段b房间执行器供后续步骤使用，c 接收阶段b房间执行器供后续步骤使用。
	w, r, c := stageBWorker(t, 0)
	r.Pet = nil
	r.Role.ID = domain.ID()
	r.Auth.RoleID = r.Role.ID
	r.Role.Revision, r.Role.SourceSeq, r.Role.EconomicSeq = 10, 4, 5
	r.Role.Capacity, r.Role.HostingSeconds, r.Role.PickupStopped = 1, 3, true
	r.Role.ProgressionFormatVersion = 1
	r.Role.ProgressionCurveVersion = w.e.Store.Cfg.Fixture.Progression.CurveVersion
	r.Role.AutoRecycle = domain.AutoRecyclePolicy{}
	r.Role.Items = []domain.Item{{ID: domain.ID(), ItemID: 218, Quantity: 1, Version: 1, Locked: true}}
	r.Role.Skills = nil
	r.Auto = true
	r.HostSession = domain.ID()
	w.rooms = map[string]*room{r.Role.ID: r}
	w.e.jobs = make(chan job, 8)
	// definition 保存从怪物集合取出的定义。
	definition := w.e.Store.Cfg.Fixture.Monsters[0]
	definition.AttackMin, definition.AttackMax, definition.Agility = 0, 0, 0
	r.Monsters[0].Monster = &definition
	w.roll = func(int64) int64 { return 0 }
	return w, r, c
}

// TestFullBagNormalAutoIntentRemainsAvailable 核对「拾取阻塞仍拒绝普通Auto，或自动清除容量事实」与「Auto响应不是当前真实hosting状态」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestFullBagNormalAutoIntentRemainsAvailable(t *testing.T) {
	// w 接收完整或满额背包策略房间执行器供后续步骤使用，r 接收完整或满额背包策略房间执行器供后续步骤使用，c 接收完整或满额背包策略房间执行器供后续步骤使用。
	w, r, c := fullBagPolicyWorker(t)
	// index 定位布尔状态列表中正在处理的成员，enabled 接收遍历布尔状态列表时当前的元素。
	for index, enabled := range []bool{false, true} {
		c.frames = nil
		// request 保存由无符号整数“32”取得的请求。
		request := uint32(index + 1)
		w.command(command{role: r.Role.ID, conn: c, frame: wire.Message(p.MsgC2SAutoBattle, request, uint64(r.Auth.SessionEpoch), &pb.AutoBattleIntent{SceneEpoch: r.Scene, Enabled: enabled, MonsterCount: 1})})
		if r.Auto != enabled || !r.Role.PickupStopped || len(c.frames) < 2 || c.frames[0].ID != p.MsgS2CHostingState || c.frames[0].Request != request || c.frames[0].Error != 0 {
			t.Fatal("拾取阻塞仍拒绝普通Auto，或自动清除容量事实", r.Auto, c.frames)
		}
		// state 记录状态，使用离线托管状态表示。
		var state pb.HostingState
		// err 接收反序列化过程中可能产生的错误，供后续失败分支处理。
		if err := proto.Unmarshal(c.frames[0].Payload, &state); err != nil || state.AutoBattleEnabled != enabled || state.RemainHostingSec != 3 {
			t.Fatal("Auto响应不是当前真实hosting状态", &state, err)
		}
	}
}

// TestFullBagConfirmedSettlementKeepsEveryWorldAutoIntent 核对「满包结果关闭了当前或休眠World的Auto」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestFullBagConfirmedSettlementKeepsEveryWorldAutoIntent(t *testing.T) {
	// w 接收完整或满额背包策略房间执行器供后续步骤使用，r 接收完整或满额背包策略房间执行器供后续步骤使用。
	w, r, _ := fullBagPolicyWorker(t)
	// original 保存由当前状态场景世界取得的原始。
	original := r.currentWorld()
	r.PersonalWorld = original
	r.TownWorld = r.newWorld(2)
	r.TownWorld.Auto = false
	r.Pending = true
	r.SettlementGeneration = 1
	r.PendingOperationID, r.PendingFingerprint = "original-source", "original-digest"
	// b 建立带初始值的结算会话绑定，供本段继续处理。
	b := settlementBinding{room: r, auth: r.Auth, world: original.Ref, generation: 1, id: r.PendingOperationID, fingerprint: r.PendingFingerprint}
	// q 组装结算所需的数据。
	q := persistence.Settlement{SourceSeq: r.Role.SourceSeq + 1}
	// next 保存角色，用于当前步骤中的下一状态。
	next := r.Role
	next.Revision++
	next.SourceSeq = q.SourceSeq
	w.finishSettlement(b, q, next, nil, &job{})
	if r.Pending || !original.Auto || r.TownWorld.Auto || !r.Role.PickupStopped {
		t.Fatal("满包结果关闭了当前或休眠World的Auto")
	}
}

// TestFullBagActualAttackStillReleasesKillFactOnlineAndOffline 核对「满包Auto未真正释放普通攻击，或伤害提前发生」与「满包kill的EXP/原来源/离线时钟被截断」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestFullBagActualAttackStillReleasesKillFactOnlineAndOffline(t *testing.T) {
	// offline 接收遍历布尔状态列表时当前的元素。
	for _, offline := range []bool{false, true} {
		// t 提供断言失败报告、子用例和清理注册。
		t.Run(fmt.Sprint(offline), func(t *testing.T) {
			// w 接收完整或满额背包策略房间执行器供后续步骤使用，r 接收完整或满额背包策略房间执行器供后续步骤使用。
			w, r, _ := fullBagPolicyWorker(t)
			if offline {
				r.Conn = nil
			}
			// m 保存从怪物集合取出的怪物集合中的指定项。
			m := r.Monsters[0]
			m.HP, m.MaxHP = 1, 1
			// now 保存由Unix 时间取得的当前时间。
			now := time.Unix(1800000000, 0)
			r.LastTick = now.Add(-100 * time.Millisecond)
			w.step(r, now)
			if r.Player.AttackSequence != 1 || len(r.Attacks) != 1 || m.HP != 1 {
				t.Fatal("满包Auto未真正释放普通攻击，或伤害提前发生")
			}
			w.step(r, r.Attacks[0].Due)
			if m.HP != 0 || len(r.KillFacts) != 1 || !r.Pending || len(w.e.jobs) != 1 {
				t.Fatal("满包后的真实命中未进入原永久事实单槽")
			}
			// q 接收击杀结算供后续步骤使用。
			q := w.killSettlement(r, r.KillFacts[0])
			if q.Kills != 1 || q.Experience != m.Monster.Exp || q.SourceSeq != 5 || r.Role.SourceSeq != 4 || offline && (q.HostingSession != r.HostSession || r.hostRemainder <= 0) {
				t.Fatal("满包kill的EXP/原来源/离线时钟被截断", q)
			}
		})
	}
}

// TestFullBagOfflineChargesOriginalSlotAndStopsNewAIAtZero 核对「满包未进入原扣秒事务，或提交前先减余额」与「余额0仍新增离线AI/重复扣时，或篡改用户Auto意图」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestFullBagOfflineChargesOriginalSlotAndStopsNewAIAtZero(t *testing.T) {
	// w 接收完整或满额背包策略房间执行器供后续步骤使用，r 接收完整或满额背包策略房间执行器供后续步骤使用。
	w, r, _ := fullBagPolicyWorker(t)
	r.Conn = nil
	r.Role.HostingSeconds, r.hostRemainder = 1, 900*time.Millisecond
	// now 保存由Unix 时间取得的当前时间。
	now := time.Unix(1800000000, 0)
	r.LastTick = now.Add(-100 * time.Millisecond)
	w.step(r, now)
	// q 组装结算所需的数据。
	q := persistence.Settlement{SourceSeq: 5, HostSeconds: 1, HostingSession: r.HostSession, EventAt: now}
	// raw 保存由序列化取得的原始值，err 接收序列化过程中可能产生的错误，供后续失败分支处理。
	raw, err := json.Marshal(q)
	if err != nil {
		t.Fatal(err)
	}
	if !r.Pending || len(w.e.jobs) != 1 || r.hostRemainder != 0 || r.Role.HostingSeconds != 1 || r.PendingFingerprint != persistence.Hash(raw) {
		t.Fatal("满包未进入原扣秒事务，或提交前先减余额")
	}
	// retry 接收任务或职业编号集合通道送来的重试。
	retry := <-w.e.jobs // 只取原job身份，不执行DB；下方模拟既有完成回调。
	// b 建立带初始值的结算会话绑定，供本段继续处理。
	b := settlementBinding{room: r, auth: r.Auth, world: r.currentWorld().Ref, generation: r.SettlementGeneration, id: r.PendingOperationID, fingerprint: r.PendingFingerprint}
	// next 保存角色，用于当前步骤中的下一状态。
	next := r.Role
	next.HostingSeconds, next.SourceSeq = 0, q.SourceSeq
	next.Revision++
	w.finishSettlement(b, q, next, persistence.Fail(p.CodeOperationPending, "unknown"), &retry)
	w.step(r, r.RetryAt.Add(-time.Nanosecond))
	if !r.Pending || r.Role.HostingSeconds != 1 || r.Role.SourceSeq != 4 || len(w.e.jobs) != 0 || r.Player.AttackSequence != 0 {
		t.Fatal("未知扣秒重复分配或提前放开战斗")
	}
	w.finishSettlement(b, q, next, nil, &retry)
	w.step(r, r.LastTick.Add(100*time.Millisecond))
	if r.Pending || r.Role.HostingSeconds != 0 || !r.Auto || r.Player.AttackSequence != 0 || len(w.e.jobs) != 0 {
		t.Fatal("余额0仍新增离线AI/重复扣时，或篡改用户Auto意图")
	}
}

// TestFullBagDoesNotBypassOwnerAndLifeSafetyGates 核对「容量修复复活玩家或清除了原Pending」与「容量修复绕过了原安全屏障」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestFullBagDoesNotBypassOwnerAndLifeSafetyGates(t *testing.T) {
	// reason 接收遍历文本列表时当前的元素。
	for _, reason := range []string{"pending", "dead", "not_ready", "stopped", "isolated", "unknown_map", "offline_zero", "offline_auto_off", "town"} {
		// t 提供断言失败报告、子用例和清理注册。
		t.Run(reason, func(t *testing.T) {
			// w 接收完整或满额背包策略房间执行器供后续步骤使用，r 接收完整或满额背包策略房间执行器供后续步骤使用。
			w, r, _ := fullBagPolicyWorker(t)
			switch reason {
			case "pending":
				r.Pending = true
			case "dead":
				r.Player.HP = 0
			case "not_ready":
				r.Ready = false
			case "stopped":
				w.e.stopped.Store(true)
			case "isolated":
				r.Isolated = true
			case "unknown_map":
				r.MapID = ^uint64(0)
			case "offline_zero":
				r.Conn, r.Role.HostingSeconds = nil, 0
			case "offline_auto_off":
				r.Conn, r.Auto = nil, false
			case "town":
				r.Conn, r.MapID, r.MonsterCount, r.Monsters = nil, 2, 0, nil
				// m 接收地图“by”标识供后续步骤使用，known 保存由地图“by”标识取得的已知状态。
				m, known := w.e.Store.Cfg.Fixture.MapByID(2)
				if !known {
					t.Fatal("当前目录缺少Town")
				}
				r.Player.Pos = m.PlayerSpawn
			}
			// now 保存由Unix 时间取得的当前时间。
			now := time.Unix(1800000000, 0)
			r.AwaitUntil = now.Add(time.Minute)
			r.LastTick = now.Add(-100 * time.Millisecond)
			w.step(r, now)
			if r.Player.AttackSequence != 0 || len(r.Attacks) != 0 || len(r.Casts) != 0 || len(w.e.jobs) != 0 || r.hostRemainder != 0 || r.Role.SourceSeq != 4 {
				t.Fatal("容量修复绕过了原安全屏障", reason)
			}
			if reason == "dead" && r.Player.HP != 0 || reason == "pending" && !r.Pending {
				t.Fatal("容量修复复活玩家或清除了原Pending", reason)
			}
		})
	}
}
