package game

import (
	"google.golang.org/protobuf/proto"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
	"minimir/internal/wire"
	"reflect"
	"testing"
	"time"
)

type capturedConnection struct{ frames []wire.Frame }

func (c *capturedConnection) Alive() bool               { return true }
func (c *capturedConnection) Deliver(f wire.Frame) bool { c.frames = append(c.frames, f); return true }
func (c *capturedConnection) Close(string)              {}
func (c *capturedConnection) SetRole(string)            {}
func (c *capturedConnection) RoleID() string            { return "role" }
func (c *capturedConnection) SetBinding(uint64)         {}
func (c *capturedConnection) Binding() uint64           { return 3 }

func TestFullBagAutoRejectPushesActualHosting(t *testing.T) {
	conn := &capturedConnection{}
	r := &room{Conn: conn, Ready: true, Scene: 20, MonsterCount: 1, Auto: false, Auth: persistence.Auth{SessionEpoch: 3}, Role: domain.Role{PickupStopped: true, HostingSeconds: 42, Revision: 7}}
	w := &worker{rooms: map[string]*room{"role": r}}
	f := wire.Message(p.MsgC2SAutoBattle, 9, 3, &pb.AutoBattleIntent{SceneEpoch: 20, Enabled: true, MonsterCount: 8})
	w.command(command{role: "role", conn: conn, frame: f})
	if len(conn.frames) != 2 || conn.frames[0].ID != p.MsgC2SAutoBattle || conn.frames[0].Request != 9 || conn.frames[0].Error != p.CodeLineForbidden {
		t.Fatalf("满包拒绝没有沿原请求返回错误:%v", conn.frames)
	}
	push := conn.frames[1]
	state := &pb.HostingState{}
	if err := proto.Unmarshal(push.Payload, state); err != nil {
		t.Fatal(err)
	}
	if push.ID != p.MsgS2CHostingState || push.Request != 0 || state.AutoBattleEnabled || state.RemainHostingSec != 42 || state.StateRevision != 7 {
		t.Fatalf("满包拒绝后未补真实hosting:%v %v", push, state)
	}
	if r.Auto || r.MonsterCount != 1 {
		t.Fatal("被拒绝的自动战斗请求改变了场景状态")
	}
}

func TestDeadPlayerCountChangePushesSnapshotWithoutReset(t *testing.T) {
	conn := &capturedConnection{}
	stamp := time.Unix(1800000000, 0)
	r := &room{Conn: conn, Ready: true, Scene: 20, Baseline: 30, MonsterCount: 1, Auto: false, DeathID: "original-death", Auth: persistence.Auth{SessionEpoch: 3}, Role: domain.Role{HostingSeconds: 42, Revision: 7},
		Player: entity{Handle: 1, Kind: 1, HP: 0, MaxHP: 18, MP: 6, MaxMP: 14, Action: 4, Pos: devconfig.Point{X: 8, Y: 8}, NextMove: stamp.Add(time.Second), NextAttack: stamp.Add(2 * time.Second)},
		Pet:    &entity{Handle: 2, Kind: 2, HP: 29, MaxHP: 60, Pos: devconfig.Point{X: 8, Y: 7}, NextMove: stamp.Add(3 * time.Second), NextAttack: stamp.Add(4 * time.Second)}}
	for i := 0; i < 8; i++ {
		r.Monsters = append(r.Monsters, &entity{Handle: uint32(10 + i), Kind: 3, Def: 1, HP: int64(i), MaxHP: 15, Pos: devconfig.Point{X: i + 1, Y: 5}, Action: 3, NextMove: stamp.Add(time.Duration(i+5) * time.Second), NextAttack: stamp.Add(time.Duration(i+15) * time.Second), RespawnAt: stamp.Add(time.Duration(i+25) * time.Second)})
	}
	w := observedWorker(t, r)
	playerBefore, petBefore := r.Player, *r.Pet
	pointers := append([]*entity(nil), r.Monsters...)
	before := make([]entity, len(r.Monsters))
	for i, m := range r.Monsters {
		before[i] = *m
	}
	for index, count := range []uint32{4, 8} {
		conn.frames = nil
		request := uint32(10) + count
		w.command(command{role: "role", conn: conn, frame: wire.Message(p.MsgC2SAutoBattle, request, 3, &pb.AutoBattleIntent{SceneEpoch: 20, Enabled: false, MonsterCount: count})})
		if len(conn.frames) != 2 || conn.frames[0].ID != p.MsgS2CHostingState || conn.frames[0].Request != request {
			t.Fatalf("死亡切怪未沿请求seq回复hosting:%v", conn.frames)
		}
		hosting := &pb.HostingState{}
		if err := proto.Unmarshal(conn.frames[0].Payload, hosting); err != nil {
			t.Fatal(err)
		}
		if hosting.AutoBattleEnabled || hosting.RemainHostingSec != 42 || hosting.StateRevision != 7 {
			t.Fatalf("死亡切怪改变了托管状态:%v", hosting)
		}
		frame := conn.frames[1]
		scene := &pb.SceneBaselineChunk{}
		if err := proto.Unmarshal(frame.Payload, scene); err != nil {
			t.Fatal(err)
		}
		if frame.ID != p.MsgS2CSceneBaseline || frame.Request != 0 || scene.SceneEpoch != 20 || scene.BaselineId != 30 || scene.StateSequence != uint64(index+1) {
			t.Fatalf("死亡切怪未立即同步当前基线:%v %v", frame, scene)
		}
		monsters := 0
		for _, e := range scene.Entities {
			if e.EntityKind == 1 && (e.Hp != 0 || e.Mp != 6 || e.Position.X != 8 || e.Position.Y != 8) {
				t.Fatal("数量切换使死亡玩家复活或位置/MP重置")
			}
			if e.EntityKind == 3 {
				monsters++
				original := before[int(e.EntityHandle)-10]
				if e.Hp != original.HP || e.Position.X != int32(original.Pos.X) || e.Position.Y != int32(original.Pos.Y) {
					t.Fatal("基线改变了怪物HP或位置")
				}
			}
		}
		if monsters != int(count) || r.MonsterCount != int(count) {
			t.Fatalf("基线怪物数=%d，预期%d", monsters, count)
		}
		if !reflect.DeepEqual(playerBefore, r.Player) || !reflect.DeepEqual(petBefore, *r.Pet) || r.DeathID != "original-death" {
			t.Fatal("数量切换重置了玩家/宠物/死亡状态或CD")
		}
		for i, m := range r.Monsters {
			if m != pointers[i] || !reflect.DeepEqual(before[i], *m) {
				t.Fatalf("数量切换重建或重置了怪物%d及其HP/CD/复活时点", i)
			}
		}
	}
}
