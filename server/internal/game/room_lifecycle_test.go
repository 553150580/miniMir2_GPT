package game

import (
	"bytes"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/domain"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
)

func TestIdleRoomProtectsExecutionUntilSettlementAndGrace(t *testing.T) {
	for _, tc := range []struct {
		name    string
		protect func(*room)
		clear   func(*room)
	}{
		{"在线", func(r *room) { r.Conn = &travelConnection{alive: true} }, func(r *room) { r.Conn = nil }},
		{"托管", func(r *room) { r.Auto, r.Role.HostingSeconds = true, 60 }, func(r *room) { r.Role.HostingSeconds = 0 }},
		{"提交待确认", func(r *room) { r.Pending, r.PendingRetry = true, &job{} }, func(r *room) { r.Pending, r.PendingRetry = false, nil }},
		{"未结收益", func(r *room) { r.KillFacts = []killFact{{}} }, func(r *room) { r.KillFacts = nil }},
		{"未结磨损", func(r *room) { r.Wear.open = []persistence.EquipmentWear{{}} }, func(r *room) { r.Wear.open = nil }},
		{"已释放技能", func(r *room) { r.Casts = []pendingCast{{}} }, func(r *room) { r.Casts = nil }},
		{"隔离", func(r *room) { r.Isolated = true }, func(r *room) { r.Isolated = false }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w, r, _, now := mapTravelWorker(t)
			r.Conn, r.Auto, r.Role.HostingSeconds = nil, false, 0
			w.rooms = map[string]*room{r.Role.ID: r}
			tc.protect(r)
			r.idleSince = now.Add(-2 * offlineRoomRetention)
			if w.expireIdleRoom(r.Role.ID, r, now) || w.rooms[r.Role.ID] != r || !r.idleSince.IsZero() {
				t.Fatal("仍有业务的房间被回收或累计空闲时间")
			}
			tc.clear(r)
			if w.expireIdleRoom(r.Role.ID, r, now) || r.idleSince != now ||
				w.expireIdleRoom(r.Role.ID, r, now.Add(offlineRoomRetention-time.Nanosecond)) {
				t.Fatal("健康重连窗口内丢失原房间")
			}
			if !w.expireIdleRoom(r.Role.ID, r, now.Add(offlineRoomRetention)) || w.rooms[r.Role.ID] != nil {
				t.Fatal("结清并到期后没有释放房间")
			}
		})
	}
}

func TestReleaseIdleSlotPreservesHealthyRoomsDuringChurn(t *testing.T) {
	w, online, _, now := mapTravelWorker(t)
	online.Role.ID = "在线"
	w.rooms = map[string]*room{online.Role.ID: online}
	// 故意把同一分片反复填满，累计准入超过两倍原配额。
	for cycle := range 3 {
		for slot := 0; slot < maxRoomsPerWorker-1; slot++ {
			roleID := travelID(cycle*maxRoomsPerWorker + slot)
			w.rooms[roleID] = &room{
				Role: domain.Role{ID: roleID}, Scene: 1, Player: entity{HP: 1},
				worldState: &worldState{MapID: online.MapID},
			}
		}
		if len(w.rooms) != maxRoomsPerWorker || w.releaseIdleSlot(now) ||
			w.releaseIdleSlot(now.Add(offlineRoomRetention-time.Nanosecond)) {
			t.Fatal("配额压力提前回收健康重连窗口内的房间")
		}
		now = now.Add(offlineRoomRetention)
		for w.releaseIdleSlot(now) {
		}
		if len(w.rooms) != 1 || w.rooms[online.Role.ID] != online {
			t.Fatal("到期后未归还全部闲置配额，或在线房间被错误清理")
		}
	}
}

func TestIdleRoomDeadColdJoinPreservesOriginalChoice(t *testing.T) {
	w, dead, _, now := mapTravelWorker(t)
	w.e.jobs = make(chan job, 1)
	dead.Conn, dead.Player.HP, dead.DeathID = nil, 0, domain.ID()
	dead.idleSince = now.Add(-offlineRoomRetention)
	if w.expireIdleRoom(dead.Role.ID, dead, now) || w.rooms[dead.Role.ID] != dead ||
		!dead.Pending || len(w.e.jobs) != 1 {
		t.Fatal("死亡标记未确认时房间必须保留并安排提交")
	}
	<-w.e.jobs
	// 存储回归另验真实防重与围栏；此处模拟确定的原标记已落库。
	dead.Pending = false
	dead.Role.RetiredDeathID = dead.DeathID
	if !w.expireIdleRoom(dead.Role.ID, dead, now) {
		t.Fatal("死亡标记确认后应正常释放世界")
	}
	cold := &room{}
	w.initialize(cold, dead.Role)
	conn := &capturedConnection{}
	cold.Conn, cold.Ready = conn, true
	w.deathNotice(cold)
	if cold.Scene == 0 || cold.Scene == dead.Scene || cold.Player.HP != 0 ||
		cold.DeathID != dead.DeathID || len(conn.frames) != 1 || conn.frames[0].ID != p.MsgS2CDeathNotice {
		t.Fatal("冷重入换场景时丢失原死亡状态，或未推送待复活选择")
	}
	notice := &pb.DeathNotice{}
	if err := proto.Unmarshal(conn.frames[0].Payload, notice); err != nil ||
		!bytes.Equal(notice.DeathId, domain.Bytes(dead.DeathID)) {
		t.Fatal("冷重入的死亡选择没有保留原死亡身份")
	}
}

func TestIdleRoomInTownDoesNotConsumeHostingSlot(t *testing.T) {
	w, r, _, now := mapTravelWorker(t)
	r.MapID = w.e.Store.Cfg.Fixture.TownTravel.DestinationMapIDs[0]
	r.Conn, r.Auto, r.Role.HostingSeconds = nil, true, 180
	if w.expireIdleRoom(r.Role.ID, r, now) ||
		!w.expireIdleRoom(r.Role.ID, r, now.Add(offlineRoomRetention)) {
		t.Fatal("城镇角色持有时长不应阻止超期回收")
	}
}
