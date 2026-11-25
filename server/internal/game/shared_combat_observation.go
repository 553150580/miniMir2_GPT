// 本文件把真实施法表现投影到同一共享场景内已确认的观察者。
package game

import (
	"math"

	"google.golang.org/protobuf/proto"
	pb "minimir/api/generated/minimirv1"
)

// castSequenceFull 是施法受理前的本地或共享施法编号上界。
func (r *room) castSequenceFull() bool {
	return r.CastID == math.MaxUint64 || r.worldBossCombat != nil && r.worldBossCombat.castID == math.MaxUint64 ||
		r.dungeonCombat != nil && r.dungeonCombat.castID == math.MaxUint64 ||
		r.worldState != nil && r.worldState.mirrorCombat != nil && r.worldState.mirrorCombat.castID == math.MaxUint64
}

// advanceCastID 在共享首领场用单一 actor 的编号，避免不同角色各自从1引起前端去重。
func (r *room) advanceCastID() {
	if arena := r.currentWorld().mirrorCombat; arena != nil {
		arena.castID++
		r.CastID = arena.castID
		return
	}
	if arena := r.dungeonCombat; arena != nil {
		arena.castID++
		r.CastID = arena.castID
		return
	}
	if arena := r.worldBossCombat; arena != nil {
		arena.castID++
		r.CastID = arena.castID
		return
	}
	r.CastID++
}

// visibleBossSeat 只承认同场当前连接、入场确认和观测代次都一致的参战席位。
func visibleBossSeat(arena *worldBossArena, seat *worldBossSeat) bool {
	if seat == nil || seat.observation == nil || seat.room == nil || !seat.shown {
		return false
	}
	peer, obs := seat.room, seat.observation
	return peer.worldBossCombat == arena && peer.Ready && !peer.Isolated && peer.Scene == obs.scene &&
		peer.Auth == obs.auth && peer.Conn == obs.conn && obs.conn != nil && obs.conn.Alive() &&
		obs.conn.Binding() == uint64(obs.auth.SessionEpoch) && !obs.revoked.Load() && obs.occurrence.id == arena.occurrence.id &&
		obs.occurrence.runID == arena.occurrence.runID
}

// sendCombatObservation 保留本人真实动作，并给其他共享席位发唯一事实的副本。
// msg 仅允许由施法、逐格或命中入口显式交进来，不经过经济回执和失败响应。
func (w *worker) sendCombatObservation(r *room, id uint16, msg proto.Message) {
	if arena := r.currentWorld().mirrorCombat; arena != nil {
		arena.addObservation(id, msg)
		return
	}
	if r.dungeonCombat != nil {
		w.sendDungeonObservation(r, id, msg)
		return
	}
	arena := r.worldBossCombat
	if arena == nil {
		if !w.sendTownCombatObservation(r, id, msg) {
			w.send(r, id, 0, msg)
		}
		return
	}
	sender := arena.seats[r.Role.ID]
	if !visibleBossSeat(arena, sender) || sender.room != r {
		return
	}
	w.send(r, id, 0, msg)
	for _, seat := range arena.seats {
		if seat.room == r || !visibleBossSeat(arena, seat) {
			continue
		}
		peer := seat.room
		event := combatObservationAt(msg, peer.Scene)
		if event != nil {
			w.send(peer, id, 0, event)
		}
	}
}

// combatObservationAt 复制自有表现，替换观察场景，并清去只属于原输入一端的序号。
func combatObservationAt(msg proto.Message, scene uint64) proto.Message {
	switch source := msg.(type) {
	case *pb.CastAccepted:
		event := proto.Clone(source).(*pb.CastAccepted)
		event.SceneEpoch, event.InputSequence = scene, 0
		return event
	case *pb.AttackAccepted:
		event := proto.Clone(source).(*pb.AttackAccepted)
		event.SceneEpoch, event.InputSequence = scene, 0
		return event
	case *pb.CastStep:
		event := proto.Clone(source).(*pb.CastStep)
		event.SceneEpoch = scene
		return event
	case *pb.CombatImpact:
		event := proto.Clone(source).(*pb.CombatImpact)
		event.SceneEpoch = scene
		return event
	case *pb.EntityDisplacement:
		event := proto.Clone(source).(*pb.EntityDisplacement)
		event.SceneEpoch = scene
		return event
	default:
		return nil
	}
}
