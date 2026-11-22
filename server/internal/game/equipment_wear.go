// 本文件在攻击、受伤和死亡时记录耐久磨损，批量随来源事务结算并验证回执。
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

// equipmentWearState 保存装备磨损状态所需的状态。
type equipmentWearState struct {
	// open 记录打开，使用装备磨损列表表示。
	open []persistence.EquipmentWear
	// inflight 记录“inflight”，使用装备磨损列表表示。
	inflight []persistence.EquipmentWear
	// deadline 记录截止时间，使用时间点表示。
	deadline time.Time
	// urgent 标记是否处于“urgent”状态。
	urgent bool
	// deathID 标识当前业务使用的死亡。
	deathID string
}

// loss 返回损失供调用方使用。
// s 是调用该方法的装备磨损状态实例，item 记录道具，slot 记录槽位，使用数值表示。
func (s *equipmentWearState) loss(item domain.Item, slot uint32) int64 {
	// batch 接收遍历装备磨损列表列表时当前的元素。
	for _, batch := range [][]persistence.EquipmentWear{s.open, s.inflight} {
		// cost 接收遍历批次时当前的元素。
		for _, cost := range batch {
			if cost.Slot == slot && cost.InstanceID == item.ID && cost.Version == item.Version {
				return cost.AttackU + cost.HitU + cost.DeathU
			}
		}
	}
	return 0
}

// weaponWear 处理“weapon”磨损。
// w 是调用该方法的房间执行器实例，r 指向当前执行器拥有并更新的目标房间，now 提供当前时点，作为状态有效期和计时判断的基准。
func (w *worker) weaponWear(r *room, now time.Time) {
	// rules 保存装备耐久，用于当前步骤中的规则集合。
	rules := w.e.Store.Cfg.Fixture.EquipmentDurability
	if rules == nil {
		return
	}
	// current 保存由当前状态地图取得的当前状态，ok 表明查询、校验或类型转换是否成功。
	current, ok := w.currentMap(r)
	if !ok || current.SceneKindValue() == devconfig.SceneKindTown {
		return
	}
	// worn 接收遍历已穿戴装备时当前的元素。
	for _, worn := range r.Role.Equipped {
		if worn.Slot == devconfig.WeaponSlot {
			w.recordWear(r, worn, rules.WeaponLossU, 0, 0, now)
			return
		}
	}
}

// receivedWear 处理“received”磨损。
// w 是调用该方法的房间执行器实例，r 指向当前执行器拥有并更新的目标房间，death 标记是否处于死亡状态，now 提供当前时点，作为状态有效期和计时判断的基准。
func (w *worker) receivedWear(r *room, death bool, now time.Time) {
	// rules 保存装备耐久，用于当前步骤中的规则集合。
	rules := w.e.Store.Cfg.Fixture.EquipmentDurability
	if rules == nil {
		return
	}
	// candidates 记录候选集合，使用数值列表表示。
	var candidates [10]int
	// count 规定数量的初始取值。
	count := 0
	// index 定位已穿戴装备中正在处理的成员，worn 接收遍历已穿戴装备时当前的元素。
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
	// selected 保存从已穿戴装备取出的已选择。
	selected := r.Role.Equipped[candidates[w.draw(int64(count))]]
	if death {
		w.recordWear(r, selected, 0, 0, rules.DeathLossU, now)
	} else {
		w.recordWear(r, selected, 0, rules.ArmorHitLossU, 0, now)
	}
}

// recordWear 处理记录磨损。
// w 是调用该方法的房间执行器实例，r 指向当前执行器拥有并更新的目标房间，worn 记录“worn”，使用已穿戴装备表示，attack 记录攻击，使用数值表示，hit 记录命中，使用数值表示。
// death 记录死亡，使用数值表示，now 提供当前时点，作为状态有效期和计时判断的基准。
func (w *worker) recordWear(r *room, worn domain.Equipped, attack, hit, death int64, now time.Time) {
	// item 保存当前处理所用的道具。
	item := worn.Item
	if r.Pending || r.Isolated || item.OriginalDurabilityU <= 0 {
		return
	}
	// remaining 保存根据耐久u与损失计算的剩余额度。
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
	// index 标记遍历打开时访问的下标。
	for index := range r.Wear.open {
		// cost 保存当前处理所用的消耗。
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
// w 是调用该方法的房间执行器实例，r 指向当前执行器拥有并更新的目标房间，frame 记录协议帧，now 提供当前时点，作为状态有效期和计时判断的基准。
func (w *worker) flushWearBeforeAsset(r *room, frame wire.Frame, now time.Time) bool {
	if r.Pending || len(r.Wear.open) == 0 {
		return false
	}
	// op 记录操作，使用操作上下文表示。
	var op *pb.OperationContext
	// matches 记录“matches”，使用供当前流程调用的函数表示。
	var matches func(persistence.EquipmentWear) bool
	switch frame.ID {
	case p.MsgC2SEquip:
		// request 记录请求，使用穿戴装备请求表示。
		var request pb.EquipRequest
		if proto.Unmarshal(frame.Payload, &request) != nil {
			return false
		}
		op = request.Operation
		// cost 记录消耗，使用装备磨损表示。
		matches = func(cost persistence.EquipmentWear) bool { return cost.Slot == request.Slot }
	case p.MsgC2SSetItemLock:
		// request 记录请求，使用设置道具锁定请求表示。
		var request pb.SetItemLockRequest
		if proto.Unmarshal(frame.Payload, &request) != nil {
			return false
		}
		op = request.Operation
		// id 区分当前步骤指向的目标对象。
		id := domain.Hex(request.ItemInstanceId)
		// cost 记录消耗，使用装备磨损表示。
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
	// cost 接收遍历打开时当前的元素。
	for _, cost := range r.Wear.open {
		if matches(cost) && w.flushWear(r, now, true) {
			w.failure(r, frame, p.CodeEquipmentChanged)
			return true
		}
	}
	return false
}

// flushWear 提交已收集的磨损。
// w 是调用该方法的房间执行器实例，r 指向当前执行器拥有并更新的目标房间，now 提供当前时点，作为状态有效期和计时判断的基准，boundary 标记是否处于边界状态。
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

// attach 关联当前请求或状态，并把结果交给调用方。
// s 是调用该方法的装备磨损状态实例，q 记录结算。
func (s *equipmentWearState) attach(q *persistence.Settlement) {
	if len(s.open) == 0 {
		return
	}
	q.EquipmentWear, q.WearDeathID = s.open, s.deathID
	s.inflight, s.open = s.open, nil
	s.deadline, s.urgent = time.Time{}, false
}

// confirmedWear 判断已确认磨损是否满足要求。
// q 记录结算，op 记录操作。
func confirmedWear(q persistence.Settlement, op persistence.Operation) bool {
	if len(q.EquipmentWear) == 0 {
		return true
	}
	if op.Kind != "settlement" || op.SourceSeq != q.SourceSeq || op.Settlement == nil || op.Settlement.WearDeathID != q.WearDeathID || len(op.Settlement.EquipmentWear) != len(q.EquipmentWear) {
		return false
	}
	// i 定位装备磨损中正在处理的成员，cost 接收遍历装备磨损时当前的元素。
	for i, cost := range q.EquipmentWear {
		if op.Settlement.EquipmentWear[i].Cost != cost {
			return false
		}
	}
	return true
}
