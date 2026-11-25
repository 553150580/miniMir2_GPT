// 本文件把讨伐链的共享身体和真实战斗表现同步到同场已确认席位。
package game

import (
	"time"

	"google.golang.org/protobuf/proto"
	pb "minimir/api/generated/minimirv1"
)

// appendDungeonViews 发布同场其它已入场角色的当前身体，健康断线仍保留原现场。
func (w *worker) appendDungeonViews(observer *room, baseline *pb.SceneBaselineChunk) {
	a := observer.dungeonCombat
	if a == nil {
		return
	}
	a.eachSeat(func(seat *dungeonCombatSeat) {
		peer := seat.room
		if peer == observer || !seat.started || peer.Isolated {
			return
		}
		avatar := w.worldBossBodyView(peer, &peer.Player)
		avatar.DisplayName = peer.Role.Name
		baseline.Entities = append(baseline.Entities, avatar)
		if pet := peer.worldPet(peer.currentWorld()); pet != nil {
			baseline.Entities = append(baseline.Entities, w.worldBossBodyView(peer, pet))
		}
		for _, corpse := range peer.currentWorld().PetCorpses {
			baseline.Entities = append(baseline.Entities, w.worldBossBodyView(peer, corpse.Body))
		}
	})
}

// observesDungeon 要求当前传输连接仍与这个已确认的原角色会话绑定。
func (seat *dungeonCombatSeat) observesDungeon() bool {
	child := seat.room
	return seat.started && child.Ready && !child.Isolated && child.Conn != nil && child.Conn.Alive() &&
		child.Conn.Binding() == uint64(child.Auth.SessionEpoch)
}

// sendDungeonObservation 向其他席位转发本场唯一事实，按各自观察场景改写协议身份。
func (w *worker) sendDungeonObservation(r *room, id uint16, message proto.Message) {
	a := r.dungeonCombat
	source := a.seats[r.Role.ID]
	if source == nil || source.room != r || !source.started || r.Isolated {
		return
	}
	a.eachSeat(func(seat *dungeonCombatSeat) {
		if !seat.observesDungeon() {
			return
		}
		if seat.room == r {
			w.send(r, id, 0, message)
		} else if peer := combatObservationAt(message, seat.room.Scene); peer != nil {
			w.send(seat.room, id, 0, peer)
		}
	})
}

// snapshotDungeonSeats 在换段与死亡等关键时点立即同步全场，常规行动仍由公共周期同步。
func (w *worker) snapshotDungeonSeats(a *dungeonArena, now time.Time) {
	a.eachSeat(func(seat *dungeonCombatSeat) {
		if seat.observesDungeon() {
			seat.room.LastBroadcast = now
			w.snapshot(seat.room)
		}
	})
}
