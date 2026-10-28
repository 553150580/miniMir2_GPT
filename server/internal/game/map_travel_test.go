package game

import (
	"fmt"
	"math"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	p "minimir/internal/protocol"
	"minimir/internal/wire"
	"reflect"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
)

type travelConnection struct {
	capturedConnection
	alive   bool
	binding uint64
	closed  []string
}

func (c *travelConnection) Alive() bool         { return c.alive }
func (c *travelConnection) Binding() uint64     { return c.binding }
func (c *travelConnection) SetBinding(v uint64) { c.binding = v }
func (c *travelConnection) Close(reason string) {
	c.alive = false
	c.closed = append(c.closed, reason)
}

func travelID(n int) string { return fmt.Sprintf("%032x", n) }

func travelWorker(t *testing.T) (*worker, *room, *travelConnection, time.Time) {
	t.Helper()
	r, _ := observedRoom()
	w := observedWorker(t, r)
	now := time.Unix(1800000000, 0)
	c := &travelConnection{alive: true, binding: 3}
	r.Conn, r.Role.ID, r.Auth.RoleID, r.Auth.AccountID = c, "role", "role", "account"
	r.MapID = w.e.Store.Cfg.Fixture.Map.ID
	r.Role.Level = 7
	r.Player.Life, r.Pet.Life = 9, 8
	r.Player.HP, r.Player.MP, r.Player.MaxMP = 57, 12, 100
	r.Auto, r.MonsterCount = true, 1
	r.CellEntrySequence = 100
	r.LastTick = now
	w.e.baseline.Store(r.Baseline)
	w.e.Store.Cfg.Fixture.TownTravel = &devconfig.TownTravelConfig{Enabled: true, TownMapID: 2,
		PlanTTLMS: 30000, QuiesceTTLMS: 5000, AckTTLMS: 12000, MaxPlansPerScene: 256}
	if town, exists := w.e.Store.Cfg.Fixture.MapByID(2); !exists || town.SceneKindValue() != devconfig.SceneKindTown || !town.Walkable(town.PlayerSpawn) {
		t.Fatal("本组需要已冻结的完整来源城镇目录")
	}
	return w, r, c, now
}

func prepareTravelAt(w *worker, r *room, n int, destination uint32, now time.Time) wire.Frame {
	frame := wire.Message(p.MsgC2SMapTravelPrepare, uint32(n), uint64(r.Auth.SessionEpoch),
		&pb.MapTravelPrepare{SceneEpoch: r.Scene, TravelId: travelID(n), Destination: destination})
	w.prepareMapTravelAt(r, frame, now)
	return frame
}

func confirmTravelAt(w *worker, r *room, n int, scene uint64, cancel bool, now time.Time) {
	w.commitMapTravelAt(r, wire.Message(p.MsgC2SMapTravelCommit, uint32(100+n), uint64(r.Auth.SessionEpoch),
		&pb.MapTravelCommit{SceneEpoch: scene, TravelId: travelID(n), Cancel: cancel}), now)
}

func lastTravelState(t *testing.T, c *travelConnection) *pb.MapTravelState {
	t.Helper()
	for i := len(c.frames) - 1; i >= 0; i-- {
		if c.frames[i].ID == p.MsgS2CMapTravelState {
			state := &pb.MapTravelState{}
			if err := proto.Unmarshal(c.frames[i].Payload, state); err != nil {
				t.Fatal(err)
			}
			return state
		}
	}
	t.Fatal("没有旅行状态")
	return nil
}

func TestMapTravelRoundTripKeepsTravellerAndOriginalPersonalWorld(t *testing.T) {
	w, r, c, now := travelWorker(t)
	r.Player.NextMove, r.Player.NextAttack, r.Player.ActionReadyAt = now.Add(time.Second), now.Add(2*time.Second), now.Add(3*time.Second)
	r.Player.HealAmount, r.Player.NextHeal = 17, now.Add(4*time.Second)
	r.Player.Statuses = map[uint32]*combatStatus{1: {ID: 1, TargetLife: r.Player.Life, RemainingMS: 14000, LastProcess: now, NextProcess: now.Add(time.Second)}}
	r.Potions = potionState{HP: 11, MP: 19, LastAppliedSeq: 42, NextTick: now.Add(time.Second), NextUse: now.Add(2 * time.Second)}
	r.NextSpell, r.SkillCooldowns = now.Add(5*time.Second), map[uint32]time.Time{1: now.Add(6 * time.Second)}
	r.Melee = meleeState{HalfMoon: true, Flaming: true, FlamingUntil: now.Add(9 * time.Second), Revision: 4}
	point := devconfig.Point{X: 8, Y: 6}
	r.Destination = &point
	r.Navigation = navigationState{Revision: 27, InputSequence: 13, Destination: &point, Status: navigationActive}
	r.Loot, r.LootUntil = []*entity{{Handle: 3000, Kind: 4, Pos: r.Player.Pos}}, now.Add(7*time.Second)
	r.Monsters[0].HP, r.Monsters[0].Life = 73, 4
	player, pet, petValue, potions, melee := r.Player, r.Pet, *r.Pet, r.Potions, r.Melee
	monsters, loot, lootUntil, navigation, nextSpell := r.Monsters, r.Loot, r.LootUntil, r.Navigation, r.NextSpell
	auth, role := r.Auth, r.Role
	prepareTravelAt(w, r, 1, travelToTown, now)
	if r.MapID != 1 || r.SavedPersonal != nil || !r.Ready || r.Player.Pos != player.Pos {
		t.Fatal("预加载提前移交或改变原world")
	}
	confirmTravelAt(w, r, 1, 20, false, now.Add(time.Millisecond))
	if r.MapID != 2 || r.Scene != 21 || r.Baseline != 31 || r.Ready || r.SavedPersonal == nil || len(r.Monsters) != 0 || r.MonsterCount != 0 || len(r.Loot) != 0 || r.Auto {
		t.Fatal("来源图没有进行一次健康移交或仍保留个人怪物")
	}
	if r.Pet != pet || r.Player.Life != player.Life || r.Pet.Life != petValue.Life || r.Auth != auth || !reflect.DeepEqual(r.Role, role) || r.Potions != potions || r.Melee != melee || !r.NextSpell.Equal(nextSpell) {
		t.Fatal("旅行重建实体、改授权/永久数据，或重置药品/技能/模式时钟")
	}
	after := r.Player
	after.Pos, after.CellEntry, after.MovementRevision, after.MovementKind = player.Pos, player.CellEntry, player.MovementRevision, player.MovementKind
	if !reflect.DeepEqual(after, player) {
		t.Fatal("除落格与运动观察水位外改变了玩家现场")
	}
	if state := lastTravelState(t, c); state.Phase != travelCommitted || state.FromSceneEpoch != 20 || state.SceneEpoch != 21 {
		t.Fatal("提交状态没有原/新scene身份")
	}
	if !w.finishMapTravelAck(r, now.Add(time.Second)) {
		t.Fatal("合法首视口ACK被拒")
	}
	r.Ready = true
	prepareTravelAt(w, r, 2, travelToPersonal, now.Add(2*time.Second))
	confirmTravelAt(w, r, 2, 21, false, now.Add(2*time.Second))
	if r.MapID != 1 || r.Scene != 22 || r.Ready || r.SavedPersonal != nil || r.Player.Pos != player.Pos || r.Pet.Pos != petValue.Pos || !r.Auto {
		t.Fatal("未恢复原个人脚点、单宠和自动偏好")
	}
	if r.Monsters[0] != monsters[0] || r.Loot[0] != loot[0] || !r.LootUntil.Equal(lootUntil) || r.Destination != &point || !reflect.DeepEqual(r.Navigation, navigation) || r.Monsters[0].HP != 73 || r.Monsters[0].Life != 4 {
		t.Fatal("重建个人怪物/掉落、延长截止或丢失原导航目标")
	}
}

func TestMapTravelQuiescingWaitsWithoutClearingAnyOriginalWork(t *testing.T) {
	for _, field := range []string{"pending", "retry", "kills", "waiting", "casts", "attacks"} {
		t.Run(field, func(t *testing.T) {
			w, r, _, now := travelWorker(t)
			prepareTravelAt(w, r, 1, travelToTown, now)
			switch field {
			case "pending":
				r.Pending = true
			case "retry":
				r.PendingRetry = &job{target: w}
			case "kills":
				r.KillFacts = []killFact{{ID: "original", Prepared: true}}
			case "waiting":
				r.Waiting = []command{{role: "original"}}
			case "casts":
				r.Casts = []pendingCast{{Due: now.Add(time.Second)}}
			case "attacks":
				r.Attacks = []pendingAttack{{Due: now.Add(time.Second)}}
			}
			confirmTravelAt(w, r, 1, 20, false, now)
			if !mapTravelQuiescing(r) || !w.progressMapTravel(r, now.Add(time.Millisecond)) || r.MapID != 1 || r.Scene != 20 || r.SavedPersonal != nil {
				t.Fatal("既有工作未收束就移交")
			}
			if !r.Pending && r.PendingRetry == nil && len(r.KillFacts)+len(r.Waiting)+len(r.Casts)+len(r.Attacks) == 0 {
				t.Fatal("收束通过清队列伪造完成")
			}
			r.Pending, r.PendingRetry, r.KillFacts, r.Waiting, r.Casts, r.Attacks = false, nil, nil, nil, nil, nil
			if !w.progressMapTravel(r, now.Add(2*time.Millisecond)) || r.MapID != 2 {
				t.Fatal("确定收束后没有移交")
			}
		})
	}
}

func TestMapTravelReleasedAttackResolvesOnlyInOriginalScene(t *testing.T) {
	w, r, _, now := travelWorker(t)
	r.Monsters[0].HP, r.Monsters[0].MaxHP = 1000, 1000
	r.Monsters[0].Monster = &devconfig.Monster{Fighter: devconfig.Fighter{Agility: 0}}
	w.roll = func(int64) int64 { return 0 }
	due := now.Add(100 * time.Millisecond)
	r.Attacks = []pendingAttack{{Caster: reference(r, &r.Player), Target: reference(r, r.Monsters[0]), Due: due, Defence: defenceAC, Amount: 5}}
	prepareTravelAt(w, r, 1, travelToTown, now)
	confirmTravelAt(w, r, 1, 20, false, now)
	if r.Scene != 20 || len(r.Attacks) != 1 {
		t.Fatal("提前丢弃已释放攻击")
	}
	w.advanceEvents(r, due)
	if r.Monsters[0].HP >= 1000 || len(r.Attacks) != 0 {
		t.Fatal("已释放攻击没有在旧场景按原时间结算")
	}
	if !w.progressMapTravel(r, due) || r.MapID != 2 || len(r.Attacks) != 0 || r.SavedPersonal.Monsters[0].HP >= 1000 {
		t.Fatal("旧事实跨图或旧怪物伤害丢失")
	}
}

func TestMapTravelReleasedLethalAttackCancelsRatherThanResurrectingInTown(t *testing.T) {
	w, r, c, now := travelWorker(t)
	due := now.Add(100 * time.Millisecond)
	r.Attacks = []pendingAttack{{Caster: reference(r, r.Monsters[0]), Target: reference(r, &r.Player), Due: due, Defence: defenceAC, Amount: 10000}}
	prepareTravelAt(w, r, 1, travelToTown, now)
	confirmTravelAt(w, r, 1, 20, false, now)
	w.advanceEvents(r, due)
	if r.Player.HP != 0 || r.DeathID == "" {
		t.Fatal("原已释放攻击没有形成真实死亡")
	}
	w.progressMapTravel(r, due)
	if r.MapID != 1 || r.Scene != 20 || r.SavedPersonal != nil || r.Player.Life != 9 || lastTravelState(t, c).Phase != travelCancelled {
		t.Fatal("借旅行逃掉旧攻击、复活或带死亡跨图")
	}
}

func TestMapTravelDeathAndOriginalIdentityChangesCancelWithoutTransfer(t *testing.T) {
	for _, change := range []string{"death", "life", "auth", "connection", "scene", "configuration"} {
		t.Run(change, func(t *testing.T) {
			w, r, _, now := travelWorker(t)
			prepareTravelAt(w, r, 1, travelToTown, now)
			r.Pending = true
			confirmTravelAt(w, r, 1, 20, false, now)
			switch change {
			case "death":
				r.Player.HP = 0
			case "life":
				r.Player.Life++
			case "auth":
				r.Auth.AuthVersion++
			case "connection":
				r.Conn = &travelConnection{alive: true, binding: 3}
			case "scene":
				r.Scene++
			case "configuration":
				w.e.Store.Cfg.Fixture.Hash += "changed"
			}
			w.progressMapTravel(r, now.Add(time.Millisecond))
			if r.Travel.Phase != travelCancelled || r.MapID != 1 || r.SavedPersonal != nil || !r.Pending {
				t.Fatal("失效身份仍移交或清除原永久Pending")
			}
		})
	}
}

func TestMapTravelPlanAndQuiesceDeadlinesDoNotResetOrReleaseUnknownJob(t *testing.T) {
	w, r, c, now := travelWorker(t)
	prepareTravelAt(w, r, 1, travelToTown, now)
	deadline := r.Travel.Expires
	prepareTravelAt(w, r, 1, travelToTown, now.Add(time.Second))
	if !r.Travel.Expires.Equal(deadline) {
		t.Fatal("同ID重传延长准备期限")
	}
	w.progressMapTravel(r, deadline)
	if lastTravelState(t, c).Phase != travelExpired || r.Scene != 20 || !r.Ready {
		t.Fatal("准备到期没有在原场景结束")
	}
	prepareTravelAt(w, r, 2, travelToTown, deadline)
	r.Pending, r.PendingRetry = true, &job{target: w}
	original := r.PendingRetry
	confirmTravelAt(w, r, 2, 20, false, deadline)
	until := r.Travel.QuiesceUntil
	w.progressMapTravel(r, until)
	if r.Travel.Phase != travelExpired || r.PendingRetry != original || !r.Pending || r.MapID != 1 || r.SavedPersonal != nil {
		t.Fatal("超时释放unknown原job或伪造切图")
	}
}

func TestMapTravelQuiesceHasIndependentDeadlineAndLateCancelKeepsExpiredState(t *testing.T) {
	w, r, c, now := travelWorker(t)
	prepareTravelAt(w, r, 1, travelToTown, now)
	r.Pending = true
	confirmTravelAt(w, r, 1, 20, false, now.Add(29*time.Second))
	w.progressMapTravel(r, now.Add(31*time.Second))
	if !mapTravelQuiescing(r) || r.MapID != 1 {
		t.Fatal("已确认plan仍使用旧的资源准备期限")
	}
	r.Pending = false
	w.progressMapTravel(r, now.Add(31*time.Second))
	if r.MapID != 2 || r.Travel.Phase != travelCommitted {
		t.Fatal("独立收束期限内未提交")
	}
	w, r, c, now = travelWorker(t)
	prepareTravelAt(w, r, 1, travelToTown, now)
	confirmTravelAt(w, r, 1, 20, true, r.Travel.Expires)
	if lastTravelState(t, c).Phase != travelExpired || r.MapID != 1 {
		t.Fatal("已到期plan被迟到取消改写成另一终态")
	}
}

func TestMapTravelDuplicateCommitAndLateOldIDCannotMoveTwice(t *testing.T) {
	w, r, c, now := travelWorker(t)
	prepareTravelAt(w, r, 1, travelToTown, now)
	confirmTravelAt(w, r, 1, 20, false, now)
	scene, baseline, saved, count := r.Scene, r.Baseline, r.SavedPersonal, len(c.frames)
	confirmTravelAt(w, r, 1, 20, false, now.Add(time.Millisecond))
	confirmTravelAt(w, r, 1, 20, true, now.Add(2*time.Millisecond))
	if r.Scene != scene || r.Baseline != baseline || r.SavedPersonal != saved || len(c.frames) != count+2 || lastTravelState(t, c).Phase != travelCommitted {
		t.Fatal("重复提交或提交后取消发生第二次移交/回滚")
	}
	if !w.finishMapTravelAck(r, now.Add(time.Second)) {
		t.Fatal("ACK被拒")
	}
	r.Ready = true
	prepareTravelAt(w, r, 2, travelToPersonal, now.Add(time.Second))
	current := r.Travel
	confirmTravelAt(w, r, 1, 20, false, now.Add(time.Second))
	if r.Travel != current || r.Travel.Phase != travelPrepared || r.MapID != 2 || c.frames[len(c.frames)-1].Error != p.CodeOperationConflict {
		t.Fatal("旧旅行确认被重新绑定到当前plan")
	}
}

func TestMapTravelCancelledIDsNeverReviveAfterSeveralNewPlans(t *testing.T) {
	w, r, c, now := travelWorker(t)
	for id := 1; id <= 4; id++ {
		prepareTravelAt(w, r, id, travelToTown, now)
		confirmTravelAt(w, r, id, 20, true, now)
		if r.Travel.ID != travelID(id) || r.Travel.Phase != travelCancelled {
			t.Fatal("未形成原取消终态")
		}
	}
	current := r.Travel
	for _, id := range []int{1, 2, 3, 1, 3, 2} {
		prepareTravelAt(w, r, id, travelToTown, now)
		if c.frames[len(c.frames)-1].Error != p.CodeOperationConflict || r.Travel != current {
			t.Fatal("较旧取消ID被迟到prepare复活")
		}
		confirmTravelAt(w, r, id, 20, false, now)
		if c.frames[len(c.frames)-1].Error != p.CodeOperationConflict || r.Travel != current || r.Scene != 20 || r.MapID != 1 || r.SavedPersonal != nil {
			t.Fatal("旧取消确认把角色移入Town")
		}
	}
	prepareTravelAt(w, r, 4, travelToTown, now)
	confirmTravelAt(w, r, 4, 20, false, now)
	if lastTravelState(t, c).Phase != travelCancelled || r.Travel != current || len(r.TravelHistory.IDs) != 4 {
		t.Fatal("最近ID未重放终态，或重放增加/遗忘历史")
	}
}

func TestMapTravelHistoryCapacityRejectsNewIDsWithoutEvictingOldOnes(t *testing.T) {
	w, r, c, now := travelWorker(t)
	w.e.Store.Cfg.Fixture.TownTravel.MaxPlansPerScene = 3
	for id := 1; id <= 3; id++ {
		prepareTravelAt(w, r, id, travelToTown, now)
		confirmTravelAt(w, r, id, 20, true, now)
	}
	current, history := r.Travel, r.TravelHistory
	prepareTravelAt(w, r, 4, travelToTown, now.Add(time.Hour))
	if c.frames[len(c.frames)-1].Error != p.CodeServerBusy || r.Travel != current || r.TravelHistory != history || len(history.IDs) != 3 {
		t.Fatal("历史满后仍接受新计划或按时间丢弃墓碑")
	}
	prepareTravelAt(w, r, 1, travelToTown, now.Add(time.Hour))
	if c.frames[len(c.frames)-1].Error != p.CodeOperationConflict || r.MapID != 1 {
		t.Fatal("集合满时遗忘旧取消ID")
	}
	w.cancelMapTravelOnDetach(r)
	if r.TravelHistory != history {
		t.Fatal("单纯detach提前清除了原会话历史")
	}
}

func TestMapTravelExpiredIDsStayRejectedAndNoHistoryBudgetIsInvented(t *testing.T) {
	w, r, c, now := travelWorker(t)
	for id := 1; id <= 3; id++ {
		prepareTravelAt(w, r, id, travelToTown, now)
		now = r.Travel.Expires
		w.progressMapTravel(r, now)
	}
	current := r.Travel
	prepareTravelAt(w, r, 1, travelToTown, now)
	confirmTravelAt(w, r, 1, 20, false, now)
	if c.frames[len(c.frames)-1].Error != p.CodeOperationConflict || r.Travel != current || r.MapID != 1 {
		t.Fatal("旧到期ID复活")
	}
	w, r, c, now = travelWorker(t)
	w.e.Store.Cfg.Fixture.TownTravel.MaxPlansPerScene = 0
	prepareTravelAt(w, r, 1, travelToTown, now)
	if r.Travel != nil || r.TravelHistory != nil || c.frames[len(c.frames)-1].Error != p.CodeLineForbidden {
		t.Fatal("缺少显式历史容量仍补默认并开放旅行")
	}
}

func TestMapTravelHistoryNamespaceChangesOnlyWithRealAttachmentOrScene(t *testing.T) {
	w, r, c, now := travelWorker(t)
	prepareTravelAt(w, r, 1, travelToTown, now)
	confirmTravelAt(w, r, 1, 20, true, now)
	original := r.TravelHistory
	r.Player.Life++
	prepareTravelAt(w, r, 2, travelToTown, now)
	confirmTravelAt(w, r, 2, 20, true, now)
	if r.TravelHistory != original || len(original.IDs) != 2 {
		t.Fatal("仅生命变化不应忘记同场景取消记录")
	}
	w.cancelMapTravelOnDetach(r)
	c.binding, r.Auth.SessionEpoch = 4, 4
	prepareTravelAt(w, r, 1, travelToTown, now)
	if r.TravelHistory == original || r.Travel.Phase != travelPrepared || len(r.TravelHistory.IDs) != 1 || r.Travel.Auth.SessionEpoch != 4 {
		t.Fatal("真实新会话未建立独立旅行命名空间")
	}
	oldBinding := wire.Message(p.MsgC2SMapTravelCommit, 90, 3, &pb.MapTravelCommit{SceneEpoch: 20, TravelId: travelID(1)})
	w.commitMapTravelAt(r, oldBinding, now)
	if r.Travel.Phase != travelPrepared || r.MapID != 1 || c.frames[len(c.frames)-1].Error != p.CodeStaleSession {
		t.Fatal("旧binding确认进入新会话计划")
	}
	confirmTravelAt(w, r, 1, 20, false, now)
	if r.MapID != 2 || r.TravelHistory != nil {
		t.Fatal("实际换scene没有释放旧命名空间")
	}
	if !w.finishMapTravelAck(r, now.Add(time.Millisecond)) {
		t.Fatal("ACK被拒")
	}
	r.Ready = true
	prepareTravelAt(w, r, 3, travelToPersonal, now.Add(time.Millisecond))
	if r.TravelHistory == nil || r.TravelHistory.Scene != 21 || len(r.TravelHistory.IDs) != 1 {
		t.Fatal("新scene没有独立历史")
	}
	w.commitMapTravelAt(r, wire.Message(p.MsgC2SMapTravelCommit, 91, 4,
		&pb.MapTravelCommit{SceneEpoch: 20, TravelId: travelID(1)}), now.Add(time.Millisecond))
	if r.MapID != 2 || r.Travel.Phase != travelPrepared || c.frames[len(c.frames)-1].Error != p.CodeOperationConflict {
		t.Fatal("旧scene确认影响了新scene旅行")
	}
}

func TestMapTravelCancelOnlyCancelsPlanAndKeepsNavigationAndWorld(t *testing.T) {
	w, r, c, now := travelWorker(t)
	point := devconfig.Point{X: 8, Y: 6}
	r.Destination = &point
	prepareTravelAt(w, r, 1, travelToTown, now)
	confirmTravelAt(w, r, 1, 20, true, now)
	if r.Travel.Phase != travelCancelled || r.MapID != 1 || r.Scene != 20 || r.Destination != &point || !r.Auto || !r.Ready || lastTravelState(t, c).SceneEpoch != 20 {
		t.Fatal("取消改变了健康个人现场")
	}
}

func TestMapTravelACKExpiryKeepsCommittedWorldAndHealthyReconnect(t *testing.T) {
	w, r, c, now := travelWorker(t)
	prepareTravelAt(w, r, 1, travelToTown, now)
	confirmTravelAt(w, r, 1, 20, false, now)
	scene, personal := r.Scene, r.SavedPersonal
	w.progressMapTravel(r, r.Travel.AckUntil)
	if len(c.closed) != 1 || w.finishMapTravelAck(r, r.Travel.AckUntil) || r.MapID != 2 || r.Scene != scene || r.SavedPersonal != personal || r.Ready {
		t.Fatal("ACK失败回滚/重建world或激活未准备地图")
	}
	w.cancelMapTravelOnDetach(r)
	r.Conn = &travelConnection{alive: true, binding: 4}
	r.Auth.SessionEpoch = 4
	r.Baseline++
	if !w.finishMapTravelAck(r, now.Add(time.Hour)) || r.MapID != 2 || r.SavedPersonal != personal {
		t.Fatal("健康重连不能同步既有Town及原个人现场")
	}
}

func TestMapTravelDeadPetIsMovedWithoutResurrectionAndNoConfiguredTownPetSpawn(t *testing.T) {
	w, r, _, now := travelWorker(t)
	r.Pet.HP, r.Pet.Action, r.Pet.Life = 0, 4, 77
	pet := r.Pet
	prepareTravelAt(w, r, 1, travelToTown, now)
	confirmTravelAt(w, r, 1, 20, false, now)
	if r.MapID != 2 || r.Pet != pet || r.Pet.HP != 0 || r.Pet.Action != 4 || r.Pet.Life != 77 || !w.walkable(r, r.Pet.Pos) || r.Pet.Pos == r.Player.Pos {
		t.Fatal("借Town重召宠物或用非法固定出生格")
	}
}

func TestMapTravelPetPointDoesNotCrossBlockedDiagonalCorners(t *testing.T) {
	m := &devconfig.MapTemplate{Width: 3, Height: 3, Blocked: []devconfig.Point{{X: 1, Y: 0}, {X: 2, Y: 1}, {X: 1, Y: 2}, {X: 0, Y: 1}}}
	center := devconfig.Point{X: 1, Y: 1}
	if _, ok := travelPetPoint(m, center, nil, nil, 0); ok {
		t.Fatal("Town宠物被放到只能切角到达的邻格")
	}
	original := devconfig.Point{X: 0, Y: 0}
	if point, ok := travelPetPoint(m, center, &original, nil, 0); !ok || point != original {
		t.Fatal("返回个人现场没有保留原合法宠物位置")
	}
}

func TestMapTravelPersonalDeadlinesKeepOriginalCutoffAndExpireOnceOnReturn(t *testing.T) {
	w, r, _, now := travelWorker(t)
	r.Auto, r.Pet = false, nil
	monster := r.Monsters[0]
	monster.HP, monster.Life, monster.RespawnAt = 0, 4, now.Add(2*time.Second)
	monster.NextMove, monster.NextAttack = now.Add(time.Hour), now.Add(time.Hour)
	r.Loot, r.LootUntil = []*entity{{Handle: 3000, Kind: 4}}, now.Add(time.Second)
	originalRespawn, originalLoot := monster.RespawnAt, r.LootUntil
	prepareTravelAt(w, r, 1, travelToTown, now)
	confirmTravelAt(w, r, 1, 20, false, now)
	w.progressMapTravel(r, now.Add(3*time.Second))
	if monster.HP != 0 || monster.Life != 4 || !monster.RespawnAt.Equal(originalRespawn) || len(r.SavedPersonal.Loot) != 1 || !r.SavedPersonal.LootUntil.Equal(originalLoot) {
		t.Fatal("Town期间刷新个人怪物或重设掉落期限")
	}
	if !w.finishMapTravelAck(r, now.Add(3*time.Second)) {
		t.Fatal("ACK被拒")
	}
	r.Ready = true
	prepareTravelAt(w, r, 2, travelToPersonal, now.Add(3*time.Second))
	confirmTravelAt(w, r, 2, 21, false, now.Add(3*time.Second))
	if !monster.RespawnAt.Equal(originalRespawn) || !r.LootUntil.Equal(originalLoot) {
		t.Fatal("返回重设成新的相对期限")
	}
	w.step(r, now.Add(3100*time.Millisecond))
	if monster.HP != monster.MaxHP || monster.Life != 5 || !monster.RespawnAt.IsZero() || len(r.Loot) != 0 {
		t.Fatal("返回没有依原截止处理到期事实")
	}
	w.step(r, now.Add(3200*time.Millisecond))
	if monster.Life != 5 || r.Pending || len(r.KillFacts) != 0 {
		t.Fatal("重放了休眠期复活/收益")
	}
}

func TestMapTravelOverflowAndBadDestinationFailWithoutPartialTransfer(t *testing.T) {
	for _, counter := range []string{"scene", "baseline", "movement", "cell"} {
		t.Run(counter, func(t *testing.T) {
			w, r, _, now := travelWorker(t)
			if counter == "scene" {
				r.Scene = math.MaxUint64
			}
			scene := r.Scene
			prepareTravelAt(w, r, 1, travelToTown, now)
			if counter != "scene" {
				switch counter {
				case "baseline":
					r.Baseline = math.MaxUint64
				case "movement":
					r.Pet.MovementRevision = math.MaxUint64
				case "cell":
					r.CellEntrySequence = math.MaxUint64 - 1
				}
				confirmTravelAt(w, r, 1, scene, false, now)
			}
			if r.MapID != 1 || r.Scene != scene || r.SavedPersonal != nil || r.Player.HP != 57 || r.Isolated {
				t.Fatal("计数器溢出造成半次移交或回绕")
			}
		})
	}
	w, r, c, now := travelWorker(t)
	prepareTravelAt(w, r, 1, travelToPersonal, now)
	if r.Travel != nil || c.frames[len(c.frames)-1].Error != p.CodeLineForbidden {
		t.Fatal("没有个人保留现场却接受返回")
	}
	for _, destination := range []uint32{0, 3, math.MaxUint32} {
		prepareTravelAt(w, r, 2, destination, now)
		if r.Travel != nil || c.frames[len(c.frames)-1].Error != p.CodeInvalidArgument {
			t.Fatal("客户端任意目的地被接受")
		}
	}
}

func TestMapTravelRuntimeCommandsReuseEnterACKAndOldSceneCannotActivate(t *testing.T) {
	w, r, c, _ := travelWorker(t)
	w.command(command{role: "role", conn: c, frame: wire.Message(p.MsgC2SMapTravelPrepare, 1, 3,
		&pb.MapTravelPrepare{SceneEpoch: 20, TravelId: travelID(1), Destination: travelToTown})})
	w.command(command{role: "role", conn: c, frame: wire.Message(p.MsgC2SMapTravelCommit, 2, 3,
		&pb.MapTravelCommit{SceneEpoch: 20, TravelId: travelID(1)})})
	if r.MapID != 2 || r.Ready || r.Travel.Phase != travelCommitted {
		t.Fatal("正常命令入口未切图或提前Ready")
	}
	committed, entry, baseline := -1, -1, -1
	for index, frame := range c.frames {
		if frame.ID == p.MsgS2CMapTravelState {
			state := &pb.MapTravelState{}
			if proto.Unmarshal(frame.Payload, state) == nil && state.Phase == travelCommitted {
				committed = index
			}
		}
		if frame.ID == p.MsgS2CEnterLine {
			entry = index
			if frame.Request != 0 {
				t.Fatal("新场景入场冒用旅行请求的回复槽")
			}
		}
		if frame.ID == p.MsgS2CSceneBaseline {
			baseline = index
		}
	}
	if committed < 0 || entry <= committed || baseline <= entry {
		t.Fatal("未按Committed、EnterLine、基线顺序附着")
	}
	w.command(command{role: "role", conn: c, frame: wire.Message(p.MsgC2SEnterWorldAck, 3, 3,
		&pb.EnterWorldAck{SceneEpoch: 20, BaselineId: r.Baseline})})
	if r.Ready {
		t.Fatal("旧scene ACK激活了新图")
	}
	w.command(command{role: "role", conn: c, frame: wire.Message(p.MsgC2SEnterWorldAck, 4, 3,
		&pb.EnterWorldAck{SceneEpoch: r.Scene, BaselineId: r.Baseline})})
	if !r.Ready || !r.Travel.Acked || c.frames[len(c.frames)-1].ID != p.MsgS2CSyncReady {
		t.Fatal("合法ACK未复用SyncReady握手")
	}
}
