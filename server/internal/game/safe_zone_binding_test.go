package game

import (
	"context"
	"math"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
	"minimir/internal/wire"
	"reflect"
	"strconv"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
)

func safeBindingWorker(t *testing.T) (*worker, *room, *capturedConnection, devconfig.Point) {
	t.Helper()
	r, c := observedRoom()
	w := observedWorker(t, r)
	id := domain.ID()
	r.Role.ID, r.Auth.RoleID = id, id
	r.Role.Origin = 1
	r.Role.Revision, r.Role.SourceSeq = 15, 7
	w.rooms = map[string]*room{id: r}
	w.e.ctx = context.Background()
	w.e.jobs = make(chan job, 1)
	r.MapID = 2
	r.Monsters = nil
	r.MonsterCount = 0
	r.Auto = false
	r.Player.Life = 9
	fixture := &w.e.Store.Cfg.Fixture
	fixture.RuleVersion = 14
	if fixture.Binding == nil {
		fixture.Binding = &devconfig.BindingRules{}
	}
	on := true
	fixture.Binding.SafeZoneEnabled = &on
	m, ok := fixture.MapByID(2)
	if !ok {
		t.Fatal("缺完整来源Town目录")
	}
	var goal devconfig.Point
	for _, zone := range m.SafeZones {
		if zone.ID == 106 {
			goal = zone.Center
		}
	}
	if goal != (devconfig.Point{X: 650, Y: 631}) {
		t.Fatal("缺来源银杏安全区106")
	}
	found := false
	for _, d := range directions {
		candidate := devconfig.Point{X: goal.X + d.X, Y: goal.Y + d.Y}
		if m.Walkable(candidate) {
			r.Player.Pos = candidate
			found = true
			break
		}
	}
	if !found {
		t.Fatal("安全区中心没有合法相邻行走前态")
	}
	r.Pet.Pos = m.PlayerSpawn
	return w, r, c, goal
}

func safeBindingCommitted(r *room, fact *safeZoneBindingFact) (domain.Role, persistence.Operation) {
	role := r.Role
	center := fact.center
	role.Binding = &center
	role.BindingSourceSeq = fact.request.SourceSeq
	role.BindingSafeZoneID = fact.zoneID
	role.SourceSeq = fact.request.SourceSeq
	role.Revision = fact.request.ExpectedRevision + 1
	return role, persistence.Operation{ID: "binding:" + role.ID + ":" + fmtBindingSeq(fact.request.SourceSeq), RoleID: role.ID, Kind: "safe_zone_binding", Digest: fact.fingerprint, SourceSeq: fact.request.SourceSeq, Revision: role.Revision, Binding: &persistence.BindingReceipt{Point: fact.center, ZoneID: fact.zoneID, Action: "walk", EventID: fact.request.EventID, RuleVersion: fact.request.ExpectedRuleVersion, ConfigHash: fact.request.ExpectedConfigHash}}
}

func fmtBindingSeq(value int64) string { return strconv.FormatInt(value, 10) }

func TestSafeZoneBindingWalkCapturesOneFrozenFactAndLegacyNeedsReceipt(t *testing.T) {
	w, r, _, goal := safeBindingWorker(t)
	legacy := domain.RoleBinding{MapID: 2, X: int32(goal.X), Y: int32(goal.Y)}
	r.Role.Binding = &legacy
	now := time.Unix(1800000000, 0)
	if !w.commitWalk(r, &r.Player, goal, 4, now, 600) {
		t.Fatal("来源合法walk被拒绝")
	}
	fact := r.PendingBinding
	if !r.Pending || fact == nil || len(w.e.jobs) != 1 || fact.request.SourceSeq != 8 || fact.request.ExpectedRevision != r.Role.Revision || fact.request.Position != goal || fact.request.MapID != 2 || fact.request.Action != "walk" || len(fact.request.EventID) != 32 || fact.zoneID != 106 || fact.actor != (actorRef{r.Scene, r.Player.Handle, r.Player.Life}) || fact.movement != r.Player.MovementRevision || fact.cellEntry != r.Player.CellEntry {
		t.Fatal("walk后未捕获完整一次来源事实")
	}
	if r.Role.BindingSourceSeq != 0 || r.Role.SourceSeq != 7 || r.Role.Revision != fact.request.ExpectedRevision || r.Player.Pos != goal || !r.Player.NextMove.Equal(now.Add(600*time.Millisecond)) {
		t.Fatal("提交前伪造永久绑定，或Pending截断成功walk的动作时钟")
	}
	frozen := fact.request
	if w.commitWalk(r, &r.Player, devconfig.Point{X: goal.X + 1, Y: goal.Y}, 2, now.Add(time.Second), 600) || r.PendingBinding != fact || fact.request != frozen || len(w.e.jobs) != 1 {
		t.Fatal("Pending期间开始第二walk/第二SourceSeq")
	}
}

func TestSafeZoneBindingOnlyTrustedSameCenterSkipsAndOrdinarySnapshotsNeverWrite(t *testing.T) {
	for _, mode := range []string{"trusted", "outside", "disabled", "pet", "same_cell", "far_jump", "snapshot", "relocate", "revive"} {
		t.Run(mode, func(t *testing.T) {
			w, r, _, goal := safeBindingWorker(t)
			beforeRevision := r.Role.Revision
			now := time.Unix(1800000000, 0)
			switch mode {
			case "trusted":
				center := domain.RoleBinding{MapID: 2, X: int32(goal.X), Y: int32(goal.Y)}
				r.Role.Binding = &center
				r.Role.BindingSourceSeq = 7
				r.Role.BindingSafeZoneID = 106
				if !w.commitWalk(r, &r.Player, goal, 4, now, 600) {
					t.Fatal("可信同中心walk被拒")
				}
			case "outside":
				m, _ := w.currentMap(r)
				r.Player.Pos = devconfig.Point{X: m.PlayerSpawn.X, Y: m.PlayerSpawn.Y - 1}
				if !m.Walkable(r.Player.Pos) {
					r.Player.Pos = devconfig.Point{X: m.PlayerSpawn.X - 1, Y: m.PlayerSpawn.Y}
				}
				if !w.commitWalk(r, &r.Player, m.PlayerSpawn, 4, now, 600) {
					t.Fatal("区外walk被拒")
				}
			case "disabled":
				off := false
				w.e.Store.Cfg.Fixture.Binding.SafeZoneEnabled = &off
				w.commitWalk(r, &r.Player, goal, 4, now, 600)
			case "pet":
				r.Pet.Pos = r.Player.Pos
				w.commitWalk(r, r.Pet, goal, 4, now, 600)
			case "same_cell":
				r.Player.Pos = goal
				if w.commitWalk(r, &r.Player, goal, 4, now, 600) {
					t.Fatal("未移动伪造新walk来源")
				}
			case "far_jump":
				r.Player.Pos = devconfig.Point{X: 633, Y: 613}
				if w.commitWalk(r, &r.Player, goal, 4, now, 600) {
					t.Fatal("远跳伪造walk来源")
				}
			case "snapshot":
				r.Player.Pos = goal
				w.bindingState(r, 1)
				w.snapshot(r)
			case "relocate":
				w.relocate(r, &r.Player, goal)
			case "revive":
				m, _ := w.currentMap(r)
				m.PlayerSpawn = goal
				r.Player.HP = 0
				w.revivePlayer(r)
			}
			if r.Pending || r.PendingBinding != nil || len(w.e.jobs) != 0 || r.Role.SourceSeq != 7 || r.Role.Revision != beforeRevision {
				t.Fatal("非本阶段新合法walk写了绑定或可信同中心重复写")
			}
		})
	}
}

func TestSafeZoneBindingUnknownAndQueueFullRetainOriginalJobAndRequest(t *testing.T) {
	w, r, _, goal := safeBindingWorker(t)
	now := time.Unix(1800000000, 0)
	w.e.jobs <- job{}
	if !w.commitWalk(r, &r.Player, goal, 4, now, 600) {
		t.Fatal("队列满丢弃已成功walk")
	}
	fact, original := r.PendingBinding, r.PendingRetry
	if original == nil || !r.Pending {
		t.Fatal("队列满未保留原job")
	}
	frozen, op, fp := fact.request, r.PendingOperationID, r.PendingFingerprint
	<-w.e.jobs
	for _, code := range []uint32{p.CodeOperationPending, p.CodeServerBusy} {
		uncommitted := r.Role
		uncommitted.Revision = 999
		w.finishSafeZoneBinding(fact, uncommitted, persistence.Operation{}, persistence.Fail(code, "原提交尚未确定"), original)
		if r.PendingRetry != original || r.PendingBinding != fact || !r.Pending || fact.request != frozen || r.PendingOperationID != op || r.PendingFingerprint != fp || r.Role.Revision != frozen.ExpectedRevision {
			t.Fatal("unknown换请求/采用未确定out role/释放原Pending")
		}
		w.step(r, r.RetryAt.Add(time.Millisecond))
		if len(w.e.jobs) != 1 || r.PendingRetry != nil || !r.Pending || r.PendingBinding != fact {
			t.Fatal("未由既有tick重新投递同一原job")
		}
		queued := <-w.e.jobs
		if queued.target != original.target || reflect.ValueOf(queued.run).Pointer() != reflect.ValueOf(original.run).Pointer() {
			t.Fatal("重投递换了job")
		}
	}
}

func TestSafeZoneBindingPendingStopsPetAndPreservesReleasedEventsUntilReceipt(t *testing.T) {
	w, r, _, goal := safeBindingWorker(t)
	now := time.Unix(1800000000, 0)
	r.Manual = true
	r.ManualDirection = facing(r.Player.Pos, goal, 0)
	r.LastTick = now.Add(-100 * time.Millisecond)
	petBefore := *r.Pet
	event := pendingAttack{Caster: reference(r, &r.Player), Target: reference(r, r.Pet), Due: now.Add(time.Second), Amount: 1000, Defence: defenceAC, Sequence: 4, Order: 11}
	r.Attacks = []pendingAttack{event}
	w.step(r, now)
	if !r.Pending || r.PendingBinding == nil || r.Pet.Pos != petBefore.Pos || r.Pet.MovementRevision != petBefore.MovementRevision || r.Pet.AttackSequence != petBefore.AttackSequence || r.Pet.CastSequence != petBefore.CastSequence || r.Pet.NextMove != petBefore.NextMove || r.Pet.NextAttack != petBefore.NextAttack {
		t.Fatal("玩家绑定Pending后同tick宠物仍开始新行走/攻击；不把此前已处理的恢复时钟误判为新AI")
	}
	w.step(r, now.Add(2*time.Second))
	if len(r.Attacks) != 1 || !reflect.DeepEqual(r.Attacks[0], event) || r.Pet.HP != petBefore.HP {
		t.Fatal("Pending丢弃/重写/提前结算已释放事件")
	}
	fact := r.PendingBinding
	role, receipt := safeBindingCommitted(r, fact)
	w.finishSafeZoneBinding(fact, role, receipt, nil, nil)
	w.advanceEvents(r, now.Add(2*time.Second))
	if len(r.Attacks) != 0 || r.Pet.HP != 0 || r.Role.SourceSeq != 8 {
		t.Fatal("绑定终态后原事件没有按原身份结算一次")
	}
}

func TestSafeZoneBindingSuccessPublishesInventoryBeforeFactWithoutChangingCombatOrEconomicSequence(t *testing.T) {
	w, r, c, goal := safeBindingWorker(t)
	now := time.Unix(1800000000, 0)
	r.Role.EconomicSeq = 3
	r.SkillCooldowns = map[uint32]time.Time{1: now.Add(9 * time.Second)}
	r.Player.MP = 12
	r.Potions = potionState{HP: 3, MP: 4, NextTick: now.Add(time.Second)}
	w.commitWalk(r, &r.Player, goal, 4, now, 600)
	fact := r.PendingBinding
	player, pet, potions, cooldowns := r.Player, *r.Pet, r.Potions, r.SkillCooldowns
	role, receipt := safeBindingCommitted(r, fact)
	c.frames = nil
	w.finishSafeZoneBinding(fact, role, receipt, nil, nil)
	if r.Pending || r.PendingBinding != nil || r.PendingRetry != nil || r.Role.EconomicSeq != 3 || r.Role.BindingSourceSeq != 8 || r.Role.BindingSafeZoneID != 106 || !reflect.DeepEqual(player, r.Player) || !reflect.DeepEqual(pet, *r.Pet) || r.Potions != potions || !reflect.DeepEqual(cooldowns, r.SkillCooldowns) {
		t.Fatal("绑定成功改了临时战斗/客户端economic序号或未释放本Pending")
	}
	inventory, binding := -1, -1
	var state pb.BindingState
	for i, frame := range c.frames {
		if frame.ID == p.MsgS2CInventory {
			inventory = i
		}
		if frame.ID == p.MsgS2CBindingState {
			binding = i
			mustBindingProto(t, frame, &state)
		}
	}
	if inventory < 0 || binding <= inventory || state.Cause != 2 || !state.Trusted || state.SourceSeq != 8 || state.SafeZoneId != 106 || state.Binding.GetCenter().X != 650 || state.Binding.GetCenter().Y != 631 || state.RoleRevision != uint64(fact.request.ExpectedRevision+1) {
		t.Fatal("未先同步完整Inventory再发布新绑定事实")
	}
	c.frames = nil
	w.finishSafeZoneBinding(fact, role, receipt, nil, nil)
	if len(c.frames) != 0 {
		t.Fatal("重复回调再次发布cause2")
	}
}

func mustBindingProto(t *testing.T, frame wire.Frame, out proto.Message) {
	t.Helper()
	if err := proto.Unmarshal(frame.Payload, out); err != nil {
		t.Fatal(err)
	}
}

func TestSafeZoneBindingLateScopeAndNewOwnerDoNotLoseFactsOrClearAnotherPending(t *testing.T) {
	for _, change := range []string{"disconnect", "life_scene", "new_slot", "new_auth", "new_room", "later_binding", "bad_receipt"} {
		t.Run(change, func(t *testing.T) {
			w, r, c, goal := safeBindingWorker(t)
			w.commitWalk(r, &r.Player, goal, 4, time.Unix(1800000000, 0), 600)
			fact := r.PendingBinding
			role, receipt := safeBindingCommitted(r, fact)
			switch change {
			case "disconnect":
				r.Conn = nil
			case "life_scene":
				r.Player.Life++
				r.Scene++
			case "new_slot":
				r.PendingBinding = &safeZoneBindingFact{}
				r.PendingOperationID = "another"
				r.PendingFingerprint = "another"
			case "new_auth":
				r.Auth.SessionEpoch++
			case "new_room":
				fresh := &room{Role: r.Role, Auth: r.Auth, Pending: true}
				w.rooms[r.Role.ID] = fresh
			case "later_binding":
				later := domain.RoleBinding{MapID: 2, X: 331, Y: 267}
				role.Binding = &later
				role.BindingSourceSeq = 9
				role.SourceSeq = 9
				role.BindingSafeZoneID = 105
				role.Revision = fact.request.ExpectedRevision + 2
			case "bad_receipt":
				receipt.Binding.EventID = domain.ID()
			}
			c.frames = nil
			w.finishSafeZoneBinding(fact, role, receipt, nil, nil)
			switch change {
			case "disconnect", "life_scene":
				if r.Pending || r.Role.BindingSourceSeq != 8 {
					t.Fatal("连接/临时scene/life改变丢弃已提交永久事实")
				}
				for _, frame := range c.frames {
					if frame.ID == p.MsgS2CBindingState {
						state := &pb.BindingState{}
						mustBindingProto(t, frame, state)
						if state.Cause == 2 {
							t.Fatal("旧动作向新临时身份重播提示")
						}
					}
				}
			case "new_slot", "new_auth", "new_room":
				if !r.Pending || r.Role.BindingSourceSeq != 0 || len(c.frames) != 0 {
					t.Fatal("旧回调改新owner或释放新Pending")
				}
			case "later_binding":
				if r.Role.BindingSourceSeq != 9 || r.Role.Binding.X != 331 {
					t.Fatal("旧receipt覆盖较新绑定")
				}
				for _, frame := range c.frames {
					if frame.ID == p.MsgS2CBindingState {
						state := &pb.BindingState{}
						mustBindingProto(t, frame, state)
						if state.Cause != 1 || state.SourceSeq != 9 {
							t.Fatal("把原op错标为当前绑定cause2")
						}
					}
				}
			case "bad_receipt":
				if !r.Isolated || r.Role.BindingSourceSeq != 0 || r.PendingBinding != fact {
					t.Fatal("损坏回执被当可信或丢失诊断fact")
				}
			}
		})
	}
}

func TestSafeZoneBindingDoesNotBorrowSourceSequenceAheadOfExistingKills(t *testing.T) {
	w, r, _, goal := safeBindingWorker(t)
	r.KillFacts = []killFact{{ID: "prior-kill"}}
	before := r.Player.Pos
	if w.commitWalk(r, &r.Player, goal, 4, time.Now(), 600) || r.Player.Pos != before || r.PendingBinding != nil || len(w.e.jobs) != 0 {
		t.Fatal("绑定抢在旧kill来源前保留同一个SourceSeq")
	}
	r.KillFacts = nil
	w.commitWalk(r, &r.Player, goal, 4, time.Now(), 600)
	fact := r.PendingBinding
	role, receipt := safeBindingCommitted(r, fact)
	<-w.e.jobs
	r.KillFacts = []killFact{{ID: "later-kill", Prepared: false}}
	w.finishSafeZoneBinding(fact, role, receipt, nil, nil)
	if r.Role.SourceSeq != 8 || !r.Pending || r.PendingBinding != nil || len(w.e.jobs) != 1 || !r.KillFacts[0].Prepared || w.killSettlement(r, r.KillFacts[0]).SourceSeq != 9 {
		t.Fatal("后续kill未在已完成binding水位后串行提交")
	}
}

func TestSafeZoneBindingCounterExhaustionCannotCreateUnreceiptedWalk(t *testing.T) {
	for _, field := range []string{"source", "revision"} {
		t.Run(field, func(t *testing.T) {
			w, r, _, goal := safeBindingWorker(t)
			if field == "source" {
				r.Role.SourceSeq = math.MaxInt64
			} else {
				r.Role.Revision = math.MaxInt64
			}
			before := r.Player.Pos
			if w.commitWalk(r, &r.Player, goal, 4, time.Now(), 600) || r.Player.Pos != before || r.Pending || len(w.e.jobs) != 0 {
				t.Fatal("计数溢出仍移动或创造不可对账来源")
			}
		})
	}
}
