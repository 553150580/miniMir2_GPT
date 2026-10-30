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
func fullBagPolicyWorker(t *testing.T) (*worker, *room, *capturedConnection) {
	t.Helper()
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
	definition := w.e.Store.Cfg.Fixture.Monsters[0]
	definition.AttackMin, definition.AttackMax, definition.Agility = 0, 0, 0
	r.Monsters[0].Monster = &definition
	w.roll = func(int64) int64 { return 0 }
	return w, r, c
}

func TestFullBagNormalAutoIntentRemainsAvailable(t *testing.T) {
	w, r, c := fullBagPolicyWorker(t)
	for index, enabled := range []bool{false, true} {
		c.frames = nil
		request := uint32(index + 1)
		w.command(command{role: r.Role.ID, conn: c, frame: wire.Message(p.MsgC2SAutoBattle, request, uint64(r.Auth.SessionEpoch), &pb.AutoBattleIntent{SceneEpoch: r.Scene, Enabled: enabled, MonsterCount: 1})})
		if r.Auto != enabled || !r.Role.PickupStopped || len(c.frames) < 2 || c.frames[0].ID != p.MsgS2CHostingState || c.frames[0].Request != request || c.frames[0].Error != 0 {
			t.Fatal("拾取阻塞仍拒绝普通Auto，或自动清除容量事实", r.Auto, c.frames)
		}
		var state pb.HostingState
		if err := proto.Unmarshal(c.frames[0].Payload, &state); err != nil || state.AutoBattleEnabled != enabled || state.RemainHostingSec != 3 {
			t.Fatal("Auto响应不是当前真实hosting状态", &state, err)
		}
	}
}

func TestFullBagConfirmedSettlementKeepsEveryWorldAutoIntent(t *testing.T) {
	w, r, _ := fullBagPolicyWorker(t)
	original := r.currentWorld()
	r.PersonalWorld = original
	r.TownWorld = r.newWorld(2)
	r.TownWorld.Auto = false
	r.Pending = true
	r.SettlementGeneration = 1
	r.PendingOperationID, r.PendingFingerprint = "original-source", "original-digest"
	b := settlementBinding{room: r, auth: r.Auth, world: original.Ref, generation: 1, id: r.PendingOperationID, fingerprint: r.PendingFingerprint}
	q := persistence.Settlement{SourceSeq: r.Role.SourceSeq + 1}
	next := r.Role
	next.Revision++
	next.SourceSeq = q.SourceSeq
	w.finishSettlement(b, q, next, nil, &job{})
	if r.Pending || !original.Auto || r.TownWorld.Auto || !r.Role.PickupStopped {
		t.Fatal("满包结果关闭了当前或休眠World的Auto")
	}
}

func TestFullBagActualAttackStillReleasesKillFactOnlineAndOffline(t *testing.T) {
	for _, offline := range []bool{false, true} {
		t.Run(fmt.Sprint(offline), func(t *testing.T) {
			w, r, _ := fullBagPolicyWorker(t)
			if offline {
				r.Conn = nil
			}
			m := r.Monsters[0]
			m.HP, m.MaxHP = 1, 1
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
			q := w.killSettlement(r, r.KillFacts[0])
			if q.Kills != 1 || q.Experience != m.Monster.Exp || q.SourceSeq != 5 || r.Role.SourceSeq != 4 || offline && (q.HostingSession != r.HostSession || r.HostMS <= 0) {
				t.Fatal("满包kill的EXP/原来源/离线时钟被截断", q)
			}
		})
	}
}

func TestFullBagOfflineChargesOriginalSlotAndStopsNewAIAtZero(t *testing.T) {
	w, r, _ := fullBagPolicyWorker(t)
	r.Conn = nil
	r.Role.HostingSeconds, r.HostMS = 1, 900
	now := time.Unix(1800000000, 0)
	r.LastTick = now.Add(-100 * time.Millisecond)
	w.step(r, now)
	q := persistence.Settlement{SourceSeq: 5, HostSeconds: 1, HostingSession: r.HostSession}
	raw, err := json.Marshal(q)
	if err != nil {
		t.Fatal(err)
	}
	if !r.Pending || len(w.e.jobs) != 1 || r.HostMS != 0 || r.Role.HostingSeconds != 1 || r.PendingFingerprint != persistence.Hash(raw) {
		t.Fatal("满包未进入原扣秒事务，或提交前先减余额")
	}
	retry := <-w.e.jobs // 只取原job身份，不执行DB；下方模拟既有完成回调。
	b := settlementBinding{room: r, auth: r.Auth, world: r.currentWorld().Ref, generation: r.SettlementGeneration, id: r.PendingOperationID, fingerprint: r.PendingFingerprint}
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

func TestFullBagDoesNotBypassOwnerAndLifeSafetyGates(t *testing.T) {
	for _, reason := range []string{"pending", "dead", "not_ready", "stopped", "isolated", "unknown_map", "offline_zero", "offline_auto_off", "town"} {
		t.Run(reason, func(t *testing.T) {
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
				m, known := w.e.Store.Cfg.Fixture.MapByID(2)
				if !known {
					t.Fatal("当前目录缺少Town")
				}
				r.Player.Pos = m.PlayerSpawn
			}
			now := time.Unix(1800000000, 0)
			r.AwaitUntil = now.Add(time.Minute)
			r.LastTick = now.Add(-100 * time.Millisecond)
			w.step(r, now)
			if r.Player.AttackSequence != 0 || len(r.Attacks) != 0 || len(r.Casts) != 0 || len(w.e.jobs) != 0 || r.HostMS != 0 || r.Role.SourceSeq != 4 {
				t.Fatal("容量修复绕过了原安全屏障", reason)
			}
			if reason == "dead" && r.Player.HP != 0 || reason == "pending" && !r.Pending {
				t.Fatal("容量修复复活玩家或清除了原Pending", reason)
			}
		})
	}
}
