package game

import (
	"context"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
	"reflect"
	"testing"
	"time"
)

func worldWorker(t *testing.T) (*worker, *room, *capturedConnection, *worldState, *worldState, time.Time) {
	t.Helper()
	w, r, c := stageBWorker(t, 1, 1, 9)
	installContextTown(w)
	r.MapID = 1
	r.PersonalWorld = r.currentWorld()
	r.TownWorld = r.newWorld(2)
	id := domain.ID()
	r.Role.ID, r.Auth.RoleID, r.Role.Origin = id, id, 1
	w.rooms = map[string]*room{id: r}
	r.Role.Revision, r.Role.SourceSeq = 15, 7
	w.e.ctx, w.e.jobs = context.Background(), make(chan job, 8)
	now := time.Unix(1800000000, 0)
	return w, r, c, r.PersonalWorld, r.TownWorld, now
}

func testWorldTransfer(r *room, target *worldState) *worldTransfer {
	point, pet := devconfig.Point{X: 633, Y: 613}, devconfig.Point{X: 632, Y: 613}
	if target == r.PersonalWorld {
		point, pet = target.PlayerPosition, target.PetPosition
	}
	return &worldTransfer{From: r.currentWorld().Ref, Target: target, PlayerLife: r.Player.Life, Position: point, PetPosition: &pet, OnCommitted: func(_ *worker, r *room, _ time.Time) { r.Scene++; r.Ready = false }}
}

func TestWorldTransferKeepsUniqueTravellerAndDeadAcceptedAction(t *testing.T) {
	w, r, _, personal, town, now := worldWorker(t)
	player, pet := &r.Player, r.Pet
	r.Player.HP = 0 // 已受理21到期可在同life死后执行；新施法仍有原死亡门禁。
	r.Player.MP, r.Player.HealAmount = 37, 11
	r.SkillCooldowns = map[uint32]time.Time{9: now.Add(time.Second)}
	r.Potions = potionState{HP: 12, MP: 13, NextTick: now.Add(time.Second)}
	before, petBefore, potions, cd := r.Player, *r.Pet, r.Potions, r.SkillCooldowns[9]
	oldRef := reference(r, &r.Player)
	if !w.commitWorldTransfer(r, testWorldTransfer(r, town), now) || &r.Player != player || r.Pet != pet || r.Player.HP != 0 || r.Player.Life != before.Life || r.Player.MP != 37 || r.Player.HealAmount != 11 || r.Potions != potions || r.SkillCooldowns[9] != cd || r.Pet.Life != petBefore.Life {
		t.Fatal("移交复制/复活/恢复角色，或取消已受理的同life尸体动作")
	}
	if w.resolveInWorld(r, personal, oldRef) != nil || w.resolveCredit(r, oldRef) != player || w.resolveInWorld(r, town, reference(r, player)) != player {
		t.Fatal("旧图留幽灵可受击成员，或信用引用错误依赖角色仍在原图")
	}
	if _, code := w.prepareCast(r, 1, 0, 0, now, false); code != p.CodeSkillStateBlocked {
		t.Fatal("新施法绕过死人门禁")
	}
	if r.newWorld(3) != nil || len(r.worlds()) != 2 {
		t.Fatal("反复移交可无限增加World")
	}
}

func TestWorldTimelineTransferAtFirstMiddleLastPreservesDueSuffix(t *testing.T) {
	for _, at := range []int{0, 1, 2} {
		t.Run([]string{"first", "middle", "last"}[at], func(t *testing.T) {
			w, r, c, personal, town, now := worldWorker(t)
			skill, _ := w.e.Store.Cfg.Fixture.Skill(1)
			m := personal.Monsters[0]
			for i := 0; i < 3; i++ {
				personal.Casts = append(personal.Casts, pendingCast{Scene: r.Scene, Skill: skill, Caster: reference(r, &r.Player), Target: m.Handle, TargetLife: m.Life, TargetPoint: m.Pos, Amount: 10, Due: now, ID: uint64(i + 1), Order: uint64(i + 1)})
			}
			seen := []uint64{}
			w.advanceEventsUsing(r, now, func(r *room, e dueEvent, stamp time.Time) *worldTransfer {
				seen = append(seen, e.Cast.ID)
				if e.Cast.ID == uint64(at+1) {
					return testWorldTransfer(r, town)
				}
				return w.resolveDueEvent(r, e, stamp)
			})
			if !reflect.DeepEqual(seen, []uint64{1, 2, 3}) || r.eventCount() != 0 || r.InflightEvents != 0 || personal.Monsters[0].HP != 1000-int64(at)*10 || r.currentWorld() != town {
				t.Fatal("21批中移交丢失/重放后缀，或角色动作命中了新World", seen, personal.Monsters[0].HP)
			}
			impacts := 0
			for _, frame := range c.frames {
				if frame.ID == p.MsgS2CCombatImpact {
					impacts++
				}
			}
			if impacts != at {
				t.Fatal("新World泄漏旧后缀观察，或旧World前缀观察丢失", impacts)
			}
		})
	}
}

func TestWorldTimelineRestoresPendingSuffixWithOriginalWorldAndBudget(t *testing.T) {
	w, r, _, personal, town, now := worldWorker(t)
	for i := 1; i <= 3; i++ {
		personal.Casts = append(personal.Casts, pendingCast{Caster: reference(r, &r.Player), Due: now, Order: uint64(i), ID: uint64(i)})
	}
	seen := []uint64{}
	w.advanceEventsUsing(r, now, func(r *room, e dueEvent, _ time.Time) *worldTransfer {
		seen = append(seen, e.Cast.ID)
		if r.eventCount() != 2 {
			t.Fatal("摘入局部批后共享容量提前归还", r.eventCount())
		}
		r.Pending = true
		return testWorldTransfer(r, town) // Pending拒移交，原后缀必须仍回原World。
	})
	if r.currentWorld() != personal || r.InflightEvents != 0 || r.eventCount() != 2 || len(personal.Casts) != 2 || personal.Casts[0].Order != 2 || !personal.Casts[0].Due.Equal(now) {
		t.Fatal("Pending丢失/改时后缀或串World")
	}
	r.Pending = false
	w.advanceEventsUsing(r, now, func(_ *room, e dueEvent, _ time.Time) *worldTransfer { seen = append(seen, e.Cast.ID); return nil })
	if !reflect.DeepEqual(seen, []uint64{1, 2, 3}) || r.eventCount() != 0 {
		t.Fatal("恢复后重放或漏执行后缀", seen)
	}
}

func TestWorldTimelineMergesBothWorldsByDueAndOwnerOrder(t *testing.T) {
	w, r, _, personal, town, now := worldWorker(t)
	personal.Casts = []pendingCast{{Due: now.Add(time.Second), Order: 4, ID: 4}, {Due: now, Order: 2, ID: 2}}
	town.Attacks = []pendingAttack{{Due: now, Order: 3, Sequence: 3}}
	seen := []uint64{}
	w.advanceEventsUsing(r, now.Add(time.Second), func(_ *room, e dueEvent, _ time.Time) *worldTransfer { seen = append(seen, e.Order); return nil })
	if !reflect.DeepEqual(seen, []uint64{2, 3, 4}) || r.eventCount() != 0 {
		t.Fatal("按World枚举顺序抢先结算而非统一Due/Order", seen)
	}
}

func TestWorldHellFireContinuesOriginalMapWithoutGhostOrNewSceneFrames(t *testing.T) {
	w, r, c, personal, town, now := worldWorker(t)
	point := stepPoint(r.Player.Pos, 0, 1)
	personal.Monsters[0].Pos = point
	// 同坐标在新Town阻挡，旧传播必须仍读原个人地图。
	m, _ := w.e.Store.Cfg.Fixture.MapByID(2)
	cell := point.Y*m.Width + point.X
	m.CollisionBits[cell/8] |= 1 << uint(cell%8)
	if w.castDirected(r, 9, 0, 1, 0, now) != 0 {
		t.Fatal("未受理来源地图动作")
	}
	original := personal.Casts[0]
	if !w.commitWorldTransfer(r, testWorldTransfer(r, town), now.Add(200*time.Millisecond)) {
		t.Fatal("移交失败")
	}
	c.frames = nil
	w.advanceEvents(r, original.Due)
	if personal.Monsters[0].HP >= 1000 || len(personal.Casts) != 1 || len(town.Casts) != 0 || !personal.Casts[0].Due.Equal(original.Due.Add(100*time.Millisecond)) || w.resolveInWorld(r, personal, original.Caster) != nil || w.resolveCredit(r, original.Caster) != &r.Player {
		t.Fatal("地图事件未继续原World，或需要幽灵Player才能结算")
	}
	for _, frame := range c.frames {
		if frame.ID == p.MsgS2CCastStep || frame.ID == p.MsgS2CCombatImpact {
			t.Fatal("旧World事件包装成新Scene")
		}
	}
	before := personal.Monsters[0].HP
	r.Player.Life++
	w.advanceEvents(r, personal.Casts[0].Due)
	if personal.Monsters[0].HP != before || len(personal.Casts) != 0 {
		t.Fatal("原信用Life已失效仍继续传播")
	}
}

func TestWorldEventCapacityIncludesInactiveWorldBeforeCosts(t *testing.T) {
	w, r, _, personal, town, now := worldWorker(t)
	personal.Casts = make([]pendingCast, w.eventCapacity())
	if !w.commitWorldTransfer(r, testWorldTransfer(r, town), now) {
		t.Fatal("不能只因未来事件存在而等待传送")
	}
	r.Ready = true
	mp := r.Player.MP
	if _, code := w.prepareCast(r, 9, 0, 0, now, false); code != p.CodeSkillStateBlocked || r.Player.MP != mp || r.eventCount() != w.eventCapacity() {
		t.Fatal("跨World重新获得事件容量或先扣成本")
	}
}

func TestWorldSettlementWritesOnlyOriginLootAndRetainsUnknownOwnerJob(t *testing.T) {
	w, r, _, personal, town, now := worldWorker(t)
	point := r.Player.Pos
	q := persistence.Settlement{SourceSeq: r.Role.SourceSeq + 1, Item: &domain.Item{ItemID: 218}}
	if !w.commitWorldTransfer(r, testWorldTransfer(r, town), now) {
		t.Fatal("测试前态移交失败")
	}
	w.settleInWorld(r, personal, point, q)
	binding := settlementBinding{room: r, auth: r.Auth, world: personal.Ref, point: point, generation: r.SettlementGeneration, id: r.PendingOperationID, fingerprint: r.PendingFingerprint}
	// 在Town时，已释放旧World动作形成原World结算；永久owner始终同一个。
	retry := &job{}
	w.finishSettlement(binding, q, domain.Role{ID: "role", Revision: 99}, persistence.Fail(p.CodeOperationPending, "unknown"), retry)
	if !r.Pending || r.PendingRetry != retry || r.Role.Revision == 99 || len(town.Loot) != 0 {
		t.Fatal("unknown采用out Role或丢原job")
	}
	role := r.Role
	role.Revision++
	role.SourceSeq = q.SourceSeq
	w.finishSettlement(binding, q, role, nil, retry)
	if r.Pending || len(personal.Loot) != 1 || personal.Loot[0].Pos != point || len(town.Loot) != 0 || personal.LootUntil.IsZero() {
		t.Fatal("原World奖励回调写到当前Town/丢失原期限")
	}
	deadline := personal.LootUntil
	w.finishSettlement(binding, q, role, nil, retry)
	if personal.LootUntil != deadline {
		t.Fatal("重复回调刷新掉落截止")
	}
}

func TestWorldDormantDeadlinesStayParkedButReleasedMapWorkAdvancesOriginalCutoff(t *testing.T) {
	for _, reactive := range []bool{false, true} {
		t.Run(map[bool]string{false: "dormant", true: "map_action"}[reactive], func(t *testing.T) {
			w, r, c, personal, town, now := worldWorker(t)
			m := personal.Monsters[0]
			m.HP, m.Life, m.RespawnAt = 0, 4, now.Add(-time.Second)
			personal.Loot, personal.LootUntil = []*entity{{Handle: 1000, Kind: 4}}, now.Add(-time.Second)
			if reactive && w.castDirected(r, 9, 0, 1, 0, now) != 0 {
				t.Fatal("未受理地图动作")
			}
			if !w.commitWorldTransfer(r, testWorldTransfer(r, town), now) {
				t.Fatal("移交失败")
			}
			r.AwaitUntil = now.Add(10 * time.Second)
			r.Manual, r.ManualDirection = true, 2
			standing := r.Player.Pos
			c.frames = nil
			w.step(r, now.Add(100*time.Millisecond))
			if r.Player.Pos != standing {
				t.Fatal("无Travel plan的技能attachment ACK前新增移动")
			}
			if reactive {
				if m.HP != m.MaxHP || m.Life != 5 || !m.RespawnAt.IsZero() || len(personal.Loot) != 0 {
					t.Fatal("旧地图动作所需原截止未推进，或Life刷新错误")
				}
			} else if m.HP != 0 || m.Life != 4 || len(personal.Loot) != 1 || !m.RespawnAt.Equal(now.Add(-time.Second)) {
				t.Fatal("静默World在首次返回baseline前被无条件重生/清理")
			}
			if len(c.frames) != 0 {
				t.Fatal("未ACK时泄漏旧World观察")
			}
		})
	}
}

func TestWorldBindingCandidateSharesOwnerOrderWithoutDelayingTransferForOldKills(t *testing.T) {
	w, r, _, personal, town, now := worldWorker(t)
	m := personal.Monsters[0]
	w.enqueueKillFactInWorld(r, personal, m, now, &r.Player, "", r.Role.PrivilegeUntil)
	first := r.KillFacts[0]
	if !w.commitWorldTransfer(r, testWorldTransfer(r, town), now) {
		t.Fatal("21因未排号旧kill事实等待，未在本批边界移交")
	}
	// 本例仅检验内部事实排序；来源动作/地形及永久receipt由正式Store/WS用例负责。
	candidate := &safeZoneBindingFact{request: persistence.SafeZoneBindingRequest{MapID: 2, Position: r.Player.Pos, Action: "walk", EventID: domain.ID(), ExpectedRuleVersion: w.e.Store.Cfg.Fixture.RuleVersion, ExpectedConfigHash: w.e.Store.Cfg.Fixture.Hash}, center: domain.RoleBinding{MapID: 2, X: 650, Y: 631}, zoneID: 106}
	if !w.queueBindingCandidate(r, candidate) || candidate.request.SourceSeq != 0 || candidate.request.ExpectedRevision != 0 || candidate.order <= first.Order {
		t.Fatal("成功动作候选提前借用SourceSeq，或未保留owner发生顺序")
	}
	second := *m
	second.Handle++
	w.enqueueKillFactInWorld(r, personal, &second, now, &r.Player, "", r.Role.PrivilegeUntil)
	w.flushKillFacts(r)
	if !r.Pending || r.PendingBinding != nil || r.BindingCandidate != candidate || candidate.request.SourceSeq != 0 || !r.KillFacts[0].Prepared {
		t.Fatal("旧kill头未先占永久slot")
	}
	q := w.killSettlement(r, first)
	binding := settlementBinding{room: r, auth: r.Auth, world: personal.Ref, point: first.DropPoint, generation: r.SettlementGeneration, id: r.PendingOperationID, fingerprint: r.PendingFingerprint}
	role := r.Role
	role.Revision++
	role.SourceSeq = q.SourceSeq
	w.finishSettlement(binding, q, role, nil, &job{}, func() {
		if r.KillFacts[0].ID == first.ID {
			r.KillFacts = r.KillFacts[1:]
		}
	})
	if r.PendingBinding != candidate || r.BindingCandidate != nil || candidate.request.SourceSeq != q.SourceSeq+1 || candidate.request.ExpectedRevision != role.Revision || len(r.KillFacts) != 1 || r.KillFacts[0].Prepared {
		t.Fatal("候选未在旧kill后/新kill前正式冻结原q")
	}
	bound, receipt := safeBindingCommitted(r, candidate)
	w.finishSafeZoneBinding(candidate, bound, receipt, nil, &job{})
	if !r.Pending || r.PendingBinding != nil || len(r.KillFacts) != 1 || !r.KillFacts[0].Prepared || w.killSettlement(r, r.KillFacts[0]).SourceSeq != candidate.request.SourceSeq+1 {
		t.Fatal("后缀旧World kill抢号，或绑定完成没有继续唯一owner队列")
	}
}

func TestWorldSettlementOldSlotAndNewOwnerNeverReleaseCurrentPending(t *testing.T) {
	for _, replacement := range []string{"pending", "auth", "owner"} {
		t.Run(replacement, func(t *testing.T) {
			w, r, _, personal, _, _ := worldWorker(t)
			q := persistence.Settlement{SourceSeq: r.Role.SourceSeq + 1}
			w.settleInWorld(r, personal, r.Player.Pos, q)
			binding := settlementBinding{room: r, auth: r.Auth, world: personal.Ref, generation: r.SettlementGeneration, id: r.PendingOperationID, fingerprint: r.PendingFingerprint}
			switch replacement {
			case "pending":
				r.SettlementGeneration++
				r.PendingOperationID = "new-slot"
			case "auth":
				r.Auth.SessionEpoch++
			case "owner":
				w.rooms[r.Role.ID] = &room{worldState: &worldState{}, Role: r.Role, Pending: true, PendingOperationID: "new-owner"}
			}
			beforeID, beforeRole := r.PendingOperationID, r.Role.Revision
			role := r.Role
			role.Revision++
			w.finishSettlement(binding, q, role, nil, &job{})
			if !r.Pending || r.PendingOperationID != beforeID || r.Role.Revision != beforeRole {
				t.Fatal("旧永久回调清新slot/Auth/owner")
			}
		})
	}
}
