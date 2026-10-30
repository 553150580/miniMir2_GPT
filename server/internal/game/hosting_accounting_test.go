package game

import (
	"testing"
	"time"

	pb "minimir/api/generated/minimirv1"
	p "minimir/internal/protocol"
	"minimir/internal/wire"
)

func TestHostingFractionSurvivesNonBillablePeriods(t *testing.T) {
	for _, gate := range []string{"online", "pending", "town", "dead", "zero_balance", "auto_off", "not_ready", "isolated", "stopped"} {
		t.Run(gate, func(t *testing.T) {
			w, r, _ := fullBagPolicyWorker(t)
			r.HostMS = 800
			r.Monsters[0].HP, r.Monsters[0].MaxHP = 1000, 1000
			switch gate {
			case "pending":
				r.Conn, r.Pending = nil, true
			case "town":
				r.Conn, r.MapID, r.MonsterCount, r.Monsters = nil, 2, 0, nil
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
			now := time.Unix(1800000000, 0)
			r.AwaitUntil = now.Add(time.Minute)
			r.LastTick = now.Add(-200 * time.Millisecond)
			before := r.Role.HostingSeconds
			w.step(r, now)
			if r.HostMS != 800 || r.Role.HostingSeconds != before || len(w.e.jobs) != 0 {
				t.Fatalf("不计费期间丢失余量或发起扣时: fraction=%d balance=%d jobs=%d", r.HostMS, r.Role.HostingSeconds, len(w.e.jobs))
			}
		})
	}
}

func TestHostingFractionSurvivesHealthyTravelAndRevive(t *testing.T) {
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

func TestHostingFractionExcludesPendingDatabaseWait(t *testing.T) {
	w, r, _ := fullBagPolicyWorker(t)
	r.Conn, r.Pending, r.HostMS = nil, true, 800
	r.Monsters[0].HP, r.Monsters[0].MaxHP = 1000, 1000
	now := time.Unix(1800000000, 0)
	r.LastTick = now.Add(-5 * time.Second)
	w.step(r, now)
	r.Pending = false
	w.step(r, now.Add(100*time.Millisecond))
	if r.HostMS != 900 || len(w.e.jobs) != 0 || r.Role.HostingSeconds != 3 {
		t.Fatal("DB等待混入余量或原余量丢失")
	}
	w.step(r, now.Add(200*time.Millisecond))
	if r.HostMS != 0 || !r.Pending || len(w.e.jobs) != 1 || r.Role.HostingSeconds != 3 {
		t.Fatal("恢复后的实际200ms未沿原事务门槛计费，或提交前改了余额")
	}
}

func TestHostingColdWorldDoesNotRestoreOldFraction(t *testing.T) {
	w, r, _ := fullBagPolicyWorker(t)
	r.HostMS = 800
	w.initialize(r, r.Role)
	if r.HostMS != 0 {
		t.Fatal("故障新现场继承了旧临时计量")
	}
}
