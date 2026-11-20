package game

import (
	"testing"
	"time"

	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	p "minimir/internal/protocol"
	"minimir/internal/wire"
)

func TestStationaryAutoWaitsAndCastsFromSameCellOnlineAndOffline(t *testing.T) {
	for _, offline := range []bool{false, true} {
		name := "在线"
		if offline {
			name = "托管"
		}
		t.Run(name, func(t *testing.T) {
			w, r, _ := stageBWorker(t, 1, 1)
			now := time.Unix(1800000000, 0)
			r.Auto, r.Pet = true, nil
			r.Player.Pos = devconfig.Point{X: 2, Y: 2}
			r.LastTick = now.Add(-100 * time.Millisecond)
			if offline {
				r.Conn, r.Ready = nil, false
				r.Role.HostingSeconds, r.HostSession = 5, "stationary"
			}
			origin, movement, mana := r.Player.Pos, r.Player.MovementRevision, r.Player.MP
			monster := r.Monsters[0]
			monster.NextMove, monster.NextAttack = now.Add(time.Minute), now.Add(time.Minute)
			monster.Pos = devconfig.Point{X: 13, Y: 2}
			w.step(r, now)
			w.e.Store.Cfg.Fixture.Map.Blocked = []devconfig.Point{{X: 4, Y: 2}}
			monster.Pos = devconfig.Point{X: 6, Y: 2}
			w.step(r, now.Add(100*time.Millisecond))
			if r.Player.Pos != origin || r.Player.MovementRevision != movement || r.Player.MP != mana ||
				r.Player.CastSequence != 0 || r.Player.AttackSequence != 0 || r.Destination != nil {
				t.Fatal("超射程或遮挡被当作自动追击目标")
			}
			monster.Pos = devconfig.Point{X: 2, Y: 6}
			w.step(r, now.Add(200*time.Millisecond))
			if r.Player.Pos != origin || r.Player.MovementRevision != movement || len(r.Casts) != 1 ||
				r.Casts[0].Skill.ID != 1 || r.Casts[0].Target != monster.Handle || r.Player.MP >= mana {
				t.Fatal("射程内合法目标未在原地完成自动施法")
			}
			if offline && (r.HostMS != 300 || r.Casts[0].HostSession != "stationary") {
				t.Fatal("定点托管未沿用权威权益计时与离线施法归属")
			}
		})
	}
}

func TestStationaryAutoFallsBackToReachableAttackAndAllowsPetAndMonsterMovement(t *testing.T) {
	w, r, _ := stageBWorker(t, 2)
	now := time.Unix(1800000000, 0)
	r.Auto = true
	origin, petOrigin := r.Player.Pos, r.Pet.Pos
	monster := r.Monsters[0]
	monster.Pos = devconfig.Point{X: 12, Y: 12}
	monsterOrigin := monster.Pos
	w.step(r, now)
	if r.Player.Pos != origin || r.Pet.Pos == petOrigin || monster.Pos == monsterOrigin {
		t.Fatal("定点应固定玩家，同时允许宠物和怪物各自移动")
	}
	monster.Pos, monster.NextAttack = stepPoint(origin, 4, 1), now.Add(time.Minute)
	w.step(r, now.Add(100*time.Millisecond))
	if r.Player.Pos != origin || r.Player.AttackSequence != 1 || r.Player.AttackSkillID != 0 {
		t.Fatal("无可用法术时邻格敌人未触发原地普攻")
	}
}

func TestStationaryAutoKeepsManualJoystickAndGroundMovement(t *testing.T) {
	w, r, c := stageBWorker(t, 0)
	now := time.Unix(1800000000, 0)
	r.Auto, r.Pet = true, nil
	monster := r.Monsters[0]
	monster.Pos, monster.NextMove = devconfig.Point{X: 15, Y: 15}, now.Add(time.Minute)
	send := func(seq uint32, intent *pb.MoveIntent) {
		intent.SceneEpoch, intent.InputSequence = r.Scene, seq
		w.command(command{role: "role", conn: c, frame: wire.Message(p.MsgC2SMoveIntent, seq, 3, intent)})
	}
	joystickPoint := stepPoint(r.Player.Pos, 2, 1)
	send(1, &pb.MoveIntent{Direction: 2})
	w.step(r, now)
	if r.Player.Pos != joystickPoint {
		t.Fatal("自动开着时玩家摇杆不能换点")
	}
	send(2, &pb.MoveIntent{ReleaseJoystick: true})
	w.step(r, now.Add(time.Second))
	if r.Player.Pos != joystickPoint {
		t.Fatal("松开摇杆后自动继续移动")
	}
	groundPoint := stepPoint(joystickPoint, 2, 1)
	send(3, &pb.MoveIntent{Destination: pointPB(groundPoint)})
	w.step(r, now.Add(2*time.Second))
	w.step(r, now.Add(3*time.Second))
	w.step(r, now.Add(4*time.Second))
	if r.Player.Pos != groundPoint || r.Destination != nil || r.Navigation.Status != navigationArrived || !r.Auto {
		t.Fatal("玩家点地未到达指定位置，或到达后未保持定点自动")
	}
}

func TestStationaryHostingPausesUnfinishedGroundMoveUntilReattached(t *testing.T) {
	w, r, c := stageBWorker(t, 0)
	now := time.Unix(1800000000, 0)
	r.Auto, r.Pet = true, nil
	r.Monsters[0].Pos, r.Monsters[0].NextMove = devconfig.Point{X: 15, Y: 15}, now.Add(time.Minute)
	origin := r.Player.Pos
	goal := stepPoint(origin, 2, 1)
	r.Destination = &goal
	r.Navigation = navigationState{Destination: &goal, InputSequence: 5, Revision: 6, Status: navigationActive}
	w.command(command{role: "role", conn: c, disconnect: true, at: now})
	r.Role.HostingSeconds = 5
	r.LastTick = now
	w.step(r, now.Add(100*time.Millisecond))
	if r.Player.Pos != origin || r.Destination != &goal || r.Navigation.Status != navigationActive {
		t.Fatal("离线托管继续走动或丢弃了玩家选的地面目标")
	}
	r.Conn, r.Ready = c, true
	w.step(r, now.Add(200*time.Millisecond))
	if r.Player.Pos != goal {
		t.Fatal("健康重连后没有恢复此前玩家主动换点")
	}
}

func TestStationaryAutoRejectsCasterDisplacementEvenIfMarkedAutomatic(t *testing.T) {
	for _, skillID := range []uint32{21, 27} {
		job := uint32(0)
		if skillID == 21 {
			job = 1
		}
		w, r, _ := stageBWorker(t, job, skillID)
		for i := range w.e.Store.Cfg.Fixture.Skills {
			skill := &w.e.Store.Cfg.Fixture.Skills[i]
			if skill.ID == skillID {
				skill.AutoEnabled = true
				if automaticSkillForJob(*skill, job) {
					t.Fatalf("主动位移技能%d误入自动列表", skillID)
				}
			}
		}
		now := time.Unix(1800000000, 0)
		if w.autoSkill(r, now) || len(r.Casts) != 0 || r.Player.CastSequence != 0 {
			t.Fatalf("主动位移技能%d进入自动施法", skillID)
		}
	}
}
