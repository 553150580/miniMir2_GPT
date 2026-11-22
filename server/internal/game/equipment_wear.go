package game

import (
	"math"
	"time"

	"minimir/internal/devconfig"
	"minimir/internal/domain"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
	"minimir/internal/wire"

	"google.golang.org/protobuf/proto"
	pb "minimir/api/generated/minimirv1"
)

// 常规磨损按原装备实例合批，遇到损坏或死亡提前确认，避免每次挥击都访问数据库。
const wearFlushInterval = 3 * time.Second

type equipmentWearState struct {
	open     []persistence.EquipmentWear
	inflight []persistence.EquipmentWear
	deadline time.Time
	urgent   bool
	deathID  string
}

func (s *equipmentWearState) loss(item domain.Item, slot uint32) int64 {
	for _, batch := range [][]persistence.EquipmentWear{s.open, s.inflight} {
		for _, cost := range batch {
			if cost.Slot == slot && cost.InstanceID == item.ID && cost.Version == item.Version {
				return cost.AttackU + cost.HitU + cost.DeathU
			}
		}
	}
	return 0
}

func (w *worker) weaponWear(r *room, now time.Time) {
	rules := w.e.Store.Cfg.Fixture.EquipmentDurability
	if rules == nil {
		return
	}
	current, ok := w.currentMap(r)
	if !ok || current.SceneKindValue() == devconfig.SceneKindTown {
		return
	}
	for _, worn := range r.Role.Equipped {
		if worn.Slot == devconfig.WeaponSlot {
			w.recordWear(r, worn, rules.WeaponLossU, 0, 0, now)
			return
		}
	}
}

func (w *worker) receivedWear(r *room, death bool, now time.Time) {
	rules := w.e.Store.Cfg.Fixture.EquipmentDurability
	if rules == nil {
		return
	}
	var candidates [10]int
	count := 0
	for index, worn := range r.Role.Equipped {
		if worn.Slot > 9 || !death && worn.Slot == devconfig.WeaponSlot || worn.Item.OriginalDurabilityU <= 0 || worn.Item.DurabilityU <= r.Wear.loss(worn.Item, worn.Slot) {
			continue
		}
		if count == len(candidates) {
			w.isolateRoom(r)
			return
		}
		candidates[count] = index
		count++
	}
	if count == 0 {
		return
	}
	selected := r.Role.Equipped[candidates[w.draw(int64(count))]]
	if death {
		w.recordWear(r, selected, 0, 0, rules.DeathLossU, now)
	} else {
		w.recordWear(r, selected, 0, rules.ArmorHitLossU, 0, now)
	}
}

func (w *worker) recordWear(r *room, worn domain.Equipped, attack, hit, death int64, now time.Time) {
	item := worn.Item
	if r.Pending || r.Isolated || item.OriginalDurabilityU <= 0 {
		return
	}
	remaining := item.DurabilityU - r.Wear.loss(item, worn.Slot)
	if remaining <= 0 {
		return
	}
	if item.Version <= 0 || item.Version == math.MaxInt64 || len(r.Wear.open) > 10 {
		w.isolateRoom(r)
		return
	}
	if len(r.Wear.open) == 0 {
		r.Wear.deadline = now.Add(wearFlushInterval)
	}
	if death > 0 {
		r.Wear.deathID, r.Wear.urgent = r.DeathID, true
	}
	attack, hit, death = min(attack, remaining), min(hit, remaining), min(death, remaining)
	if attack+hit+death == remaining {
		r.Wear.urgent = true
	}
	for index := range r.Wear.open {
		cost := &r.Wear.open[index]
		if cost.Slot == worn.Slot {
			if cost.InstanceID != item.ID || cost.Version != item.Version {
				w.isolateRoom(r)
				return
			}
			cost.AttackU += attack
			cost.HitU += hit
			cost.DeathU += death
			return
		}
	}
	r.Wear.open = append(r.Wear.open, persistence.EquipmentWear{Slot: worn.Slot, InstanceID: item.ID, Version: item.Version, AttackU: attack, HitU: hit, DeathU: death})
}

// 持久磨损必须先于同一装备的穿卸/加锁；历史操作仍优先查询原回执。
func (w *worker) flushWearBeforeAsset(r *room, frame wire.Frame, now time.Time) bool {
	if r.Pending || len(r.Wear.open) == 0 {
		return false
	}
	var op *pb.OperationContext
	var matches func(persistence.EquipmentWear) bool
	switch frame.ID {
	case p.MsgC2SEquip:
		var request pb.EquipRequest
		if proto.Unmarshal(frame.Payload, &request) != nil {
			return false
		}
		op = request.Operation
		matches = func(cost persistence.EquipmentWear) bool { return cost.Slot == request.Slot }
	case p.MsgC2SSetItemLock:
		var request pb.SetItemLockRequest
		if proto.Unmarshal(frame.Payload, &request) != nil {
			return false
		}
		op = request.Operation
		id := domain.Hex(request.ItemInstanceId)
		matches = func(cost persistence.EquipmentWear) bool { return cost.InstanceID == id }
	case p.MsgC2SReviveChoice:
		op = operationContext(frame)
		matches = func(persistence.EquipmentWear) bool { return true }
	default:
		return false
	}
	if op == nil || len(op.OpId) != 16 || op.CommandSeq != uint64(r.Role.EconomicSeq)+1 || op.ExpectedRevision != uint64(r.Role.Revision) {
		return false
	}
	for _, cost := range r.Wear.open {
		if matches(cost) && w.flushWear(r, now, true) {
			w.failure(r, frame, p.CodeEquipmentChanged)
			return true
		}
	}
	return false
}

func (w *worker) flushWear(r *room, now time.Time, boundary bool) bool {
	if r.Pending || r.Isolated || len(r.Wear.open) == 0 || !boundary && !r.Wear.urgent && now.Before(r.Wear.deadline) {
		return false
	}
	if r.Role.SourceSeq == math.MaxInt64 {
		w.isolateRoom(r)
		return true
	}
	w.settle(r, persistence.Settlement{SourceSeq: r.Role.SourceSeq + 1, EventAt: now})
	return true
}

func (s *equipmentWearState) attach(q *persistence.Settlement) {
	if len(s.open) == 0 {
		return
	}
	q.EquipmentWear, q.WearDeathID = s.open, s.deathID
	s.inflight, s.open = s.open, nil
	s.deadline, s.urgent = time.Time{}, false
}

func confirmedWear(q persistence.Settlement, op persistence.Operation) bool {
	if len(q.EquipmentWear) == 0 {
		return true
	}
	if op.Kind != "settlement" || op.SourceSeq != q.SourceSeq || op.Settlement == nil || op.Settlement.WearDeathID != q.WearDeathID || len(op.Settlement.EquipmentWear) != len(q.EquipmentWear) {
		return false
	}
	for i, cost := range q.EquipmentWear {
		if op.Settlement.EquipmentWear[i].Cost != cost {
			return false
		}
	}
	return true
}
