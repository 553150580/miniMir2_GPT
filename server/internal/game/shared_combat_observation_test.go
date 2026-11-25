// 本文件验证逐格施法只到同场、已确认并仍持有认证代次的观察者。
package game

import (
	"testing"

	"google.golang.org/protobuf/proto"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/domain"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
)

// TestWorldBossCastStepKeepsOriginalCasterAndSkipsStaleObservers 覆盖同场转发和失效附着门禁。
func TestWorldBossCastStepKeepsOriginalCasterAndSkipsStaleObservers(t *testing.T) {
	arena := &worldBossArena{occurrence: worldBossOccurrence{id: "test-cast", runID: 8}, seats: make(map[string]*worldBossSeat)}
	register := func(id string, scene, binding uint64) (*worldBossSeat, *travelConnection) {
		conn := &travelConnection{alive: true, binding: binding}
		auth := persistence.Auth{RoleID: id, SessionEpoch: int64(binding)}
		room := &room{Conn: conn, Auth: auth, Scene: scene, Ready: true, Role: domain.Role{ID: id}, worldBossCombat: arena}
		obs := &worldBossObservation{conn: conn, auth: auth, scene: scene, occurrence: arena.occurrence}
		seat := &worldBossSeat{observation: obs, room: room, shown: true}
		arena.seats[id] = seat
		return seat, conn
	}
	origin, owningConn := register("施法", 31, 21)
	peer, peerConn := register("观战", 32, 22)
	_, staleConn := register("失效", 33, 23)
	staleConn.SetBinding(24)
	awaiting, awaitingConn := register("未确认", 34, 25)
	awaiting.room.Ready = false
	otherScene, wrongScene := register("旧场景", 35, 26)
	otherScene.room.Scene++
	w := &worker{}
	originMsg := &pb.CastStep{SceneEpoch: origin.room.Scene, CastId: 3, SkillId: 9, CasterHandle: 101, CasterLife: 4,
		Grid: &pb.GridPoint{X: 5, Y: 6}, Direction: 2, StepIndex: 2, SkillLevel: 1}
	w.sendCombatObservation(origin.room, p.MsgS2CCastStep, originMsg)
	if len(owningConn.frames) != 1 || len(peerConn.frames) != 1 || len(staleConn.frames)+len(awaitingConn.frames)+len(wrongScene.frames) != 0 {
		t.Fatal("逐格施法重复投递或串发给未确认、换代连接")
	}
	frame := peerConn.frames[0]
	actual := &pb.CastStep{}
	if err := proto.Unmarshal(frame.Payload, actual); err != nil {
		t.Fatal(err)
	}
	expected := proto.Clone(originMsg).(*pb.CastStep)
	expected.SceneEpoch = peer.room.Scene
	if frame.Binding != peerConn.Binding() || frame.Request != 0 || !proto.Equal(actual, expected) || originMsg.SceneEpoch != origin.room.Scene {
		t.Fatal("接收端投影改写了真实的施法者或逐格事实", actual)
	}
	peer.observation.revoked.Store(true)
	w.sendCombatObservation(origin.room, p.MsgS2CCastStep, originMsg)
	if len(peerConn.frames) != 1 {
		t.Fatal("已撤销席位仍收到后续施法帧")
	}
}
