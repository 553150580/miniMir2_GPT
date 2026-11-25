// 本文件按已确认的同镇席位投影真实施法表现，城镇不会因此合并角色战斗或资产。
package game

import (
	"math"

	"google.golang.org/protobuf/proto"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/wire"
)

// townCombatObserver 保存跨分片发送所需的不可变附着身份。
type townCombatObserver struct {
	conn    Connection // 已确认连接的线程安全投递入口。
	scene   uint64     // 该连接看到当前城镇的场景代次。
	binding uint64     // 该连接首次确认本城镇时的会话围栏。
}

// live 检查健康连接是否仍持有发布时的认证代次。
func (o townCombatObserver) live() bool {
	return o.conn != nil && o.conn.Alive() && o.conn.Binding() == o.binding
}

// sendTownCombatObservation 按城镇锁同步分配接收序号和入队，通过目录保护换图与续接边界。
func (w *worker) sendTownCombatObservation(r *room, id uint16, msg proto.Message) bool {
	membership := r.townMembership
	if membership.owner == 0 || !r.Ready || r.Isolated || suppressOrdinaryObservation(r, id) {
		return false
	}
	d := &w.e.sharedTowns
	d.mu.Lock()
	defer d.mu.Unlock()
	town := d.towns[membership.key]
	if town == nil {
		return false
	}
	sender := town.residents[membership.owner]
	if sender == nil || sender.view == nil || sender.combat.conn != r.Conn || sender.combat.scene != r.Scene ||
		sender.combat.binding != uint64(r.Auth.SessionEpoch) || !sender.combat.live() {
		return false
	}
	switch event := msg.(type) {
	case *pb.CastAccepted:
		if town.castID == math.MaxUint64 {
			return false
		}
		town.castID++
		event.CastId = town.castID
	case *pb.CombatImpact:
		if town.eventID == math.MaxUint64 {
			return false
		}
		town.eventID++
		event.EventId = town.eventID
	}
	// 受理编号和每个连接入队顺序共同保护现有客户端的全场高水位。
	sender.combat.conn.Deliver(wire.Message(id, 0, sender.combat.binding, msg))
	for owner, resident := range town.residents {
		if owner == membership.owner || resident.view == nil || !resident.combat.live() {
			continue
		}
		event := townCombatObservationAt(msg, resident.combat.scene, r.Player.Handle, sender.handle)
		if event != nil {
			resident.combat.conn.Deliver(wire.Message(id, 0, resident.combat.binding, event))
		}
	}
	return true
}

// townCombatObservationAt 将本地角色句柄转换为同镇共享句柄，隐藏未共享的宠物目标。
func townCombatObservationAt(msg proto.Message, scene uint64, localHandle, sharedHandle uint32) proto.Message {
	switch event := combatObservationAt(msg, scene).(type) {
	case *pb.CastAccepted:
		if event.CasterHandle != localHandle {
			return nil
		}
		event.CasterHandle = sharedHandle
		if event.TargetHandle == localHandle {
			event.TargetHandle = sharedHandle
		} else if event.TargetHandle != 0 {
			event.TargetHandle, event.TargetLife = 0, 0
		}
		return event
	case *pb.CastStep:
		if event.CasterHandle != localHandle {
			return nil
		}
		event.CasterHandle = sharedHandle
		return event
	case *pb.CombatImpact:
		if event.CasterHandle != localHandle || event.TargetHandle != 0 && event.TargetHandle != localHandle {
			return nil
		}
		event.CasterHandle = sharedHandle
		if event.TargetHandle == localHandle {
			event.TargetHandle = sharedHandle
		}
		return event
	default:
		return nil
	}
}
