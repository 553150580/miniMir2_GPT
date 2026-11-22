// 本文件回归安全区绑定的原始事实、来源顺序、幂等重试与持久回执推送。
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

// safeBindingWorker 处理安全会话绑定房间执行器。
// t 提供断言失败报告、子用例和清理注册。
func safeBindingWorker(t *testing.T) (*worker, *room, *capturedConnection, devconfig.Point) {
	t.Helper()
	// r 接收观察到的房间供后续步骤使用，c 接收观察到的房间供后续步骤使用。
	r, c := observedRoom()
	// w 接收观察到的房间执行器供后续步骤使用。
	w := observedWorker(t, r)
	// id 区分当前步骤指向的目标对象。
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
	// fixture 保存当前处理所用的开发测试配置。
	fixture := &w.e.Store.Cfg.Fixture
	fixture.RuleVersion = 14
	if fixture.Binding == nil {
		fixture.Binding = &devconfig.BindingRules{}
	}
	// on 保存当前处理所用的启用状态。
	on := true
	fixture.Binding.SafeZoneEnabled = &on
	// m 接收地图“by”标识供后续步骤使用，ok 表明查询、校验或类型转换是否成功。
	m, ok := fixture.MapByID(2)
	if !ok {
		t.Fatal("缺完整来源Town目录")
	}
	// goal 记录目标格，使用点位表示。
	var goal devconfig.Point
	// zone 接收遍历安全区域集合时当前的元素。
	for _, zone := range m.SafeZones {
		if zone.ID == 106 {
			goal = zone.Center
		}
	}
	if goal != (devconfig.Point{X: 650, Y: 631}) {
		t.Fatal("缺来源银杏安全区106")
	}
	// found 保存当前处理所用的已找到标记。
	found := false
	// d 接收遍历朝向集合时当前的元素。
	for _, d := range directions {
		// candidate 建立带初始值的点位，供本段继续处理。
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

// safeBindingCommitted 处理安全会话绑定已提交。
// r 指向当前执行器拥有并更新的目标房间，fact 记录已发生的事实，使用安全区域会话绑定已发生的事实表示。
func safeBindingCommitted(r *room, fact *safeZoneBindingFact) (domain.Role, persistence.Operation) {
	// role 保存当前处理所用的角色。
	role := r.Role
	// center 保存当前处理所用的中心格。
	center := fact.center
	role.Binding = &center
	role.BindingSourceSeq = fact.request.SourceSeq
	role.BindingSafeZoneID = fact.zoneID
	role.SourceSeq = fact.request.SourceSeq
	role.Revision = fact.request.ExpectedRevision + 1
	return role, persistence.Operation{ID: "binding:" + role.ID + ":" + fmtBindingSeq(fact.request.SourceSeq), RoleID: role.ID, Kind: "safe_zone_binding", Digest: fact.fingerprint, SourceSeq: fact.request.SourceSeq, Revision: role.Revision, Binding: &persistence.BindingReceipt{Point: fact.center, ZoneID: fact.zoneID, Action: "walk", EventID: fact.request.EventID, RuleVersion: fact.request.ExpectedRuleVersion, ConfigHash: fact.request.ExpectedConfigHash}}
}

// fmtBindingSeq 返回“fmt”会话绑定序号供调用方使用。
// value 记录值，使用数值表示。
func fmtBindingSeq(value int64) string { return strconv.FormatInt(value, 10) }

// TestSafeZoneBindingWalkCapturesOneFrozenFactAndLegacyNeedsReceipt 核对「walk后未捕获完整一次来源事实」与「Pending期间开始第二walk/第二SourceSeq」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestSafeZoneBindingWalkCapturesOneFrozenFactAndLegacyNeedsReceipt(t *testing.T) {
	// w 接收安全会话绑定房间执行器供后续步骤使用，r 接收安全会话绑定房间执行器供后续步骤使用，goal 保存由安全会话绑定房间执行器取得的目标格。
	w, r, _, goal := safeBindingWorker(t)
	// legacy 初始化用作兼容旧数据的角色会话绑定。
	legacy := domain.RoleBinding{MapID: 2, X: int32(goal.X), Y: int32(goal.Y)}
	r.Role.Binding = &legacy
	// now 保存由Unix 时间取得的当前时间。
	now := time.Unix(1800000000, 0)
	if !w.commitWalk(r, &r.Player, goal, 4, now, 600) {
		t.Fatal("来源合法walk被拒绝")
	}
	// fact 保存待确认会话绑定，用于当前步骤中的已发生的事实。
	fact := r.PendingBinding
	if !r.Pending || fact == nil || len(w.e.jobs) != 1 || fact.request.SourceSeq != 8 || fact.request.ExpectedRevision != r.Role.Revision || fact.request.Position != goal || fact.request.MapID != 2 || fact.request.Action != "walk" || len(fact.request.EventID) != 32 || fact.zoneID != 106 || fact.actor != reference(r, &r.Player) || fact.movement != r.Player.MovementRevision || fact.cellEntry != r.Player.CellEntry {
		t.Fatal("walk后未捕获完整一次来源事实")
	}
	if r.Role.BindingSourceSeq != 0 || r.Role.SourceSeq != 7 || r.Role.Revision != fact.request.ExpectedRevision || r.Player.Pos != goal || !r.Player.NextMove.Equal(now.Add(600*time.Millisecond)) {
		t.Fatal("提交前伪造永久绑定，或Pending截断成功walk的动作时钟")
	}
	// frozen 保存请求，用于当前步骤中的已冻结状态。
	frozen := fact.request
	if w.commitWalk(r, &r.Player, devconfig.Point{X: goal.X + 1, Y: goal.Y}, 2, now.Add(time.Second), 600) || r.PendingBinding != fact || fact.request != frozen || len(w.e.jobs) != 1 {
		t.Fatal("Pending期间开始第二walk/第二SourceSeq")
	}
}

// TestSafeZoneBindingOnlyTrustedSameCenterSkipsAndOrdinarySnapshotsNeverWrite 核对「非本阶段新合法walk写了绑定或可信同中心重复写」与「可信同中心walk被拒」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestSafeZoneBindingOnlyTrustedSameCenterSkipsAndOrdinarySnapshotsNeverWrite(t *testing.T) {
	// mode 接收遍历文本列表时当前的元素。
	for _, mode := range []string{"trusted", "outside", "disabled", "pet", "same_cell", "far_jump", "snapshot", "relocate", "revive"} {
		// t 提供断言失败报告、子用例和清理注册。
		t.Run(mode, func(t *testing.T) {
			// w 接收安全会话绑定房间执行器供后续步骤使用，r 接收安全会话绑定房间执行器供后续步骤使用，goal 保存由安全会话绑定房间执行器取得的目标格。
			w, r, _, goal := safeBindingWorker(t)
			// beforeRevision 保存修订号，用于当前步骤中的操作前的状态修订号。
			beforeRevision := r.Role.Revision
			// now 保存由Unix 时间取得的当前时间。
			now := time.Unix(1800000000, 0)
			switch mode {
			case "trusted":
				// center 建立带初始值的角色会话绑定，供本段继续处理。
				center := domain.RoleBinding{MapID: 2, X: int32(goal.X), Y: int32(goal.Y)}
				r.Role.Binding = &center
				r.Role.BindingSourceSeq = 7
				r.Role.BindingSafeZoneID = 106
				if !w.commitWalk(r, &r.Player, goal, 4, now, 600) {
					t.Fatal("可信同中心walk被拒")
				}
			case "outside":
				// m 接收当前状态地图供后续步骤使用。
				m, _ := w.currentMap(r)
				r.Player.Pos = devconfig.Point{X: m.PlayerSpawn.X, Y: m.PlayerSpawn.Y - 1}
				if !m.Walkable(r.Player.Pos) {
					r.Player.Pos = devconfig.Point{X: m.PlayerSpawn.X - 1, Y: m.PlayerSpawn.Y}
				}
				if !w.commitWalk(r, &r.Player, m.PlayerSpawn, 4, now, 600) {
					t.Fatal("区外walk被拒")
				}
			case "disabled":
				// off 保存当前处理所用的关闭状态。
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
				// m 接收当前状态地图供后续步骤使用。
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

// TestSafeZoneBindingUnknownAndQueueFullRetainOriginalJobAndRequest 核对「未由既有tick重新投递同一原job」与「队列满丢弃已成功walk」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestSafeZoneBindingUnknownAndQueueFullRetainOriginalJobAndRequest(t *testing.T) {
	// w 接收安全会话绑定房间执行器供后续步骤使用，r 接收安全会话绑定房间执行器供后续步骤使用，goal 保存由安全会话绑定房间执行器取得的目标格。
	w, r, _, goal := safeBindingWorker(t)
	// now 保存由Unix 时间取得的当前时间。
	now := time.Unix(1800000000, 0)
	w.e.jobs <- job{}
	if !w.commitWalk(r, &r.Player, goal, 4, now, 600) {
		t.Fatal("队列满丢弃已成功walk")
	}
	// fact 保存待确认会话绑定，用于当前步骤中的已发生的事实，original 保存待按原身份重试的操作，用于当前步骤中的原始。
	fact, original := r.PendingBinding, r.PendingRetry
	if original == nil || !r.Pending {
		t.Fatal("队列满未保留原job")
	}
	// frozen 保存请求，用于当前步骤中的已冻结状态，op 保存待确认操作标识，用于当前步骤中的操作，fp 保存待确认“fingerprint”，用于当前步骤中的“fp”。
	frozen, op, fp := fact.request, r.PendingOperationID, r.PendingFingerprint
	<-w.e.jobs
	// code 接收遍历数值列表时当前的元素。
	for _, code := range []uint32{p.CodeOperationPending, p.CodeServerBusy} {
		// uncommitted 保存角色，用于当前步骤中的“uncommitted”。
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
		// queued 接收任务或职业编号集合通道送来的已排队。
		queued := <-w.e.jobs
		if queued.target != original.target || reflect.ValueOf(queued.run).Pointer() != reflect.ValueOf(original.run).Pointer() {
			t.Fatal("重投递换了job")
		}
	}
}

// TestSafeZoneBindingPendingStopsPetAndPreservesReleasedEventsUntilReceipt 核验绑定终态后原事件应按原身份结算一次，用诊断「Pending丢弃/重写/提前结算已释放事件」定位对应断言。
// t 提供断言失败报告、子用例和清理注册。
func TestSafeZoneBindingPendingStopsPetAndPreservesReleasedEventsUntilReceipt(t *testing.T) {
	// w 接收安全会话绑定房间执行器供后续步骤使用，r 接收安全会话绑定房间执行器供后续步骤使用，goal 保存由安全会话绑定房间执行器取得的目标格。
	w, r, _, goal := safeBindingWorker(t)
	// now 保存由Unix 时间取得的当前时间。
	now := time.Unix(1800000000, 0)
	r.Manual = true
	r.ManualDirection = facing(r.Player.Pos, goal, 0)
	r.LastTick = now.Add(-100 * time.Millisecond)
	// pet 保存当前处理所用的宠物。
	pet := r.Pet
	// petBefore 保存当前处理所用的宠物操作前的状态。
	petBefore := *r.Pet
	// event 初始化用作事件的待确认攻击。
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
	// fact 保存待确认会话绑定，用于当前步骤中的已发生的事实。
	fact := r.PendingBinding
	// role 保存由安全会话绑定已提交取得的角色，receipt 保存由安全会话绑定已提交取得的回执。
	role, receipt := safeBindingCommitted(r, fact)
	w.finishSafeZoneBinding(fact, role, receipt, nil, nil)
	w.advanceEvents(r, now.Add(2*time.Second))
	if len(r.Attacks) != 0 || pet.HP != 0 || r.Pet != nil || !pet.IsPetCorpse || w.resolve(r, event.Target) != pet || r.Role.SourceSeq != 8 {
		t.Fatal("绑定终态后原事件没有按原身份结算一次")
	}
}

// TestSafeZoneBindingSuccessPublishesInventoryBeforeFactWithoutChangingCombatOrEconomicSequence 核对「未先同步完整Inventory再发布新绑定事实」与「重复回调再次发布cause2」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestSafeZoneBindingSuccessPublishesInventoryBeforeFactWithoutChangingCombatOrEconomicSequence(t *testing.T) {
	// w 接收安全会话绑定房间执行器供后续步骤使用，r 接收安全会话绑定房间执行器供后续步骤使用，c 接收安全会话绑定房间执行器供后续步骤使用。
	// goal 保存由安全会话绑定房间执行器取得的目标格。
	w, r, c, goal := safeBindingWorker(t)
	// now 保存由Unix 时间取得的当前时间。
	now := time.Unix(1800000000, 0)
	r.Role.EconomicSeq = 3
	r.SkillCooldowns = map[uint32]time.Time{1: now.Add(9 * time.Second)}
	r.Player.MP = 12
	r.Potions = potionState{HP: 3, MP: 4, NextTick: now.Add(time.Second)}
	w.commitWalk(r, &r.Player, goal, 4, now, 600)
	// fact 保存待确认会话绑定，用于当前步骤中的已发生的事实。
	fact := r.PendingBinding
	// player 保存当前处理所用的玩家，pet 保存当前处理所用的宠物，potions 保存当前处理所用的药水集合，cooldowns 保存技能冷却集合，用于当前步骤中的冷却集合。
	player, pet, potions, cooldowns := r.Player, *r.Pet, r.Potions, r.SkillCooldowns
	// role 保存由安全会话绑定已提交取得的角色，receipt 保存由安全会话绑定已提交取得的回执。
	role, receipt := safeBindingCommitted(r, fact)
	c.frames = nil
	w.finishSafeZoneBinding(fact, role, receipt, nil, nil)
	if r.Pending || r.PendingBinding != nil || r.PendingRetry != nil || r.Role.EconomicSeq != 3 || r.Role.BindingSourceSeq != 8 || r.Role.BindingSafeZoneID != 106 || !reflect.DeepEqual(player, r.Player) || !reflect.DeepEqual(pet, *r.Pet) || r.Potions != potions || !reflect.DeepEqual(cooldowns, r.SkillCooldowns) {
		t.Fatal("绑定成功改了临时战斗/客户端economic序号或未释放本Pending")
	}
	// inventory 保存当前处理所用的背包资产，binding 保存当前处理所用的会话绑定。
	inventory, binding := -1, -1
	// state 记录状态，使用会话绑定状态表示。
	var state pb.BindingState
	// i 定位协议帧集合中正在处理的成员，frame 接收遍历协议帧集合时当前的元素。
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

// mustBindingProto 取得测试所需的会话绑定协议。
// t 提供断言失败报告、子用例和清理注册，frame 记录协议帧，out 记录输出，使用消息表示。
func mustBindingProto(t *testing.T, frame wire.Frame, out proto.Message) {
	t.Helper()
	// err 接收反序列化过程中可能产生的错误，供后续失败分支处理。
	if err := proto.Unmarshal(frame.Payload, out); err != nil {
		t.Fatal(err)
	}
}

// TestSafeZoneBindingLateScopeAndNewOwnerDoNotLoseFactsOrClearAnotherPending 核对「旧回调改新owner或释放新Pending」与「连接/临时scene/life改变丢弃已提交永久事实」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestSafeZoneBindingLateScopeAndNewOwnerDoNotLoseFactsOrClearAnotherPending(t *testing.T) {
	// change 接收遍历文本列表时当前的元素。
	for _, change := range []string{"disconnect", "life_scene", "new_slot", "new_auth", "new_room", "later_binding", "bad_receipt"} {
		// t 提供断言失败报告、子用例和清理注册。
		t.Run(change, func(t *testing.T) {
			// w 接收安全会话绑定房间执行器供后续步骤使用，r 接收安全会话绑定房间执行器供后续步骤使用，c 接收安全会话绑定房间执行器供后续步骤使用。
			// goal 保存由安全会话绑定房间执行器取得的目标格。
			w, r, c, goal := safeBindingWorker(t)
			w.commitWalk(r, &r.Player, goal, 4, time.Unix(1800000000, 0), 600)
			// fact 保存待确认会话绑定，用于当前步骤中的已发生的事实。
			fact := r.PendingBinding
			// role 保存由安全会话绑定已提交取得的角色，receipt 保存由安全会话绑定已提交取得的回执。
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
				// fresh 保存当前处理所用的首次装载。
				fresh := &room{Role: r.Role, Auth: r.Auth, Pending: true, worldState: &worldState{}}
				w.rooms[r.Role.ID] = fresh
			case "later_binding":
				// later 建立带初始值的角色会话绑定，供本段继续处理。
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
				// frame 接收遍历协议帧集合时当前的元素。
				for _, frame := range c.frames {
					if frame.ID == p.MsgS2CBindingState {
						// state 保存当前处理所用的状态。
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
				// frame 接收遍历协议帧集合时当前的元素。
				for _, frame := range c.frames {
					if frame.ID == p.MsgS2CBindingState {
						// state 保存当前处理所用的状态。
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

// TestSafeZoneBindingDoesNotBorrowSourceSequenceAheadOfExistingKills 核对「后续kill未在已完成binding水位后串行提交」与「绑定抢在旧kill来源前保留同一个SourceSeq」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestSafeZoneBindingDoesNotBorrowSourceSequenceAheadOfExistingKills(t *testing.T) {
	// w 接收安全会话绑定房间执行器供后续步骤使用，r 接收安全会话绑定房间执行器供后续步骤使用，goal 保存由安全会话绑定房间执行器取得的目标格。
	w, r, _, goal := safeBindingWorker(t)
	r.KillFacts = []killFact{{ID: "prior-kill"}}
	// before 保存位置，用于当前步骤中的操作前的状态。
	before := r.Player.Pos
	if w.commitWalk(r, &r.Player, goal, 4, time.Now(), 600) || r.Player.Pos != before || r.PendingBinding != nil || len(w.e.jobs) != 0 {
		t.Fatal("绑定抢在旧kill来源前保留同一个SourceSeq")
	}
	r.KillFacts = nil
	w.commitWalk(r, &r.Player, goal, 4, time.Now(), 600)
	// fact 保存待确认会话绑定，用于当前步骤中的已发生的事实。
	fact := r.PendingBinding
	// role 保存由安全会话绑定已提交取得的角色，receipt 保存由安全会话绑定已提交取得的回执。
	role, receipt := safeBindingCommitted(r, fact)
	<-w.e.jobs
	r.KillFacts = []killFact{{ID: "later-kill", Prepared: false}}
	w.finishSafeZoneBinding(fact, role, receipt, nil, nil)
	if r.Role.SourceSeq != 8 || !r.Pending || r.PendingBinding != nil || len(w.e.jobs) != 1 || !r.KillFacts[0].Prepared || w.killSettlement(r, r.KillFacts[0]).SourceSeq != 9 {
		t.Fatal("后续kill未在已完成binding水位后串行提交")
	}
}

// TestSafeZoneBindingCounterExhaustionCannotCreateUnreceiptedWalk 核对「计数溢出仍移动或创造不可对账来源」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestSafeZoneBindingCounterExhaustionCannotCreateUnreceiptedWalk(t *testing.T) {
	// field 接收遍历文本列表时当前的元素。
	for _, field := range []string{"source", "revision"} {
		// t 提供断言失败报告、子用例和清理注册。
		t.Run(field, func(t *testing.T) {
			// w 接收安全会话绑定房间执行器供后续步骤使用，r 接收安全会话绑定房间执行器供后续步骤使用，goal 保存由安全会话绑定房间执行器取得的目标格。
			w, r, _, goal := safeBindingWorker(t)
			if field == "source" {
				r.Role.SourceSeq = math.MaxInt64
			} else {
				r.Role.Revision = math.MaxInt64
			}
			// before 保存位置，用于当前步骤中的操作前的状态。
			before := r.Player.Pos
			if w.commitWalk(r, &r.Player, goal, 4, time.Now(), 600) || r.Player.Pos != before || r.Pending || len(w.e.jobs) != 0 {
				t.Fatal("计数溢出仍移动或创造不可对账来源")
			}
		})
	}
}
