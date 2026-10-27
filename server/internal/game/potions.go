package game

import (
	"bytes"
	"google.golang.org/protobuf/proto"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/domain"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
	"minimir/internal/wire"
	"strconv"
	"time"
)

// 恢复队列、计时与药效水位仅属于当前场景；健康重连保留，故障新场景不重放。
type potionState struct {
	HP             int64
	MP             int64
	NextTick       time.Time
	NextUse        time.Time
	LastAppliedSeq int64
}

func potionGate(r *room, scene uint64, now time.Time) uint32 {
	if scene != r.Scene {
		return p.CodeStaleSession
	}
	if r.Player.HP <= 0 {
		return p.CodeItemUseBlocked
	}
	if now.Before(r.Potions.NextUse) {
		return p.CodeItemUseCooldown
	}
	return 0
}

func potionFingerprint(f wire.Frame) string {
	return persistence.Hash(append([]byte{byte(f.ID >> 8), byte(f.ID)}, f.Payload...))
}

func (w *worker) queuePotion(r *room, c command) {
	const waitingLimit = 2 // 与现有重连等待共用有界队列，不能无限积累请求。
	var request pb.UseItemRequest
	if proto.Unmarshal(c.frame.Payload, &request) != nil || request.Operation == nil || len(request.Operation.OpId) != 16 {
		w.failure(r, c.frame, p.CodeInvalidArgument)
		return
	}
	id := request.Operation.OpId
	digest := potionFingerprint(c.frame)
	known := r.PendingOperationID == domain.Hex(id)
	conflict := known && digest != r.PendingFingerprint
	for _, waiting := range r.Waiting {
		if waiting.frame.ID == p.MsgC2SUseItem && bytes.Equal(operationID(waiting.frame), id) {
			known = true
			conflict = conflict || potionFingerprint(waiting.frame) != digest
		}
	}
	if conflict {
		w.failure(r, c.frame, p.CodeOperationConflict)
		return
	}
	if len(r.Waiting) >= waitingLimit {
		historical := request.Operation.CommandSeq > 0 && r.Role.EconomicSeq >= 0 && request.Operation.CommandSeq <= uint64(r.Role.EconomicSeq)
		if known || historical {
			// 已在途或落在历史经济水位内的请求可能已经提交；未查原回执前不能当作未受理。
			w.send(r, p.MsgS2COperationResult, c.frame.Request, &pb.OperationResult{OpId: id, CommandSeq: uint64(r.Role.EconomicSeq), NewRevision: uint64(r.Role.Revision), ErrorCode: p.CodeOperationPending, Pending: true})
		} else {
			w.failure(r, c.frame, p.CodeServerBusy)
		}
		return
	}
	c.frame.Payload = append([]byte(nil), c.frame.Payload...)
	r.Waiting = append(r.Waiting, c)
	// 首次排队不发假Pending，原request保留到真正执行完毕。
}

func queueAdd(queue, amount, cap int64) int64 {
	if queue >= cap || amount >= cap-queue {
		return cap
	}
	return queue + amount
}

func (w *worker) applyPotionReceipt(r *room, receipt persistence.Operation, now time.Time) bool {
	effect := receipt.Potion
	if receipt.Kind != "use_item" || receipt.Code != 0 || effect == nil || effect.Scene != strconv.FormatUint(r.Scene, 10) || receipt.CommandSeq <= r.Potions.LastAppliedSeq {
		return false
	}
	rules := w.e.Store.Cfg.Fixture.Potions
	r.Potions.LastAppliedSeq = receipt.CommandSeq
	r.Potions.HP = queueAdd(r.Potions.HP, effect.HP, rules.QueueCap)
	r.Potions.MP = queueAdd(r.Potions.MP, effect.MP, rules.QueueCap)
	r.Potions.NextUse = now.Add(time.Duration(rules.UseDelayMS) * time.Millisecond)
	return true
}

func restorePool(value, maximum, queued, perTick int64) (int64, int64) {
	if value >= maximum {
		return value, 0
	}
	amount := queued
	if amount > perTick {
		amount = perTick
	}
	if amount > maximum-value {
		amount = maximum - value
	}
	value += amount
	queued -= amount
	if value == maximum {
		queued = 0
	}
	return value, queued
}

func (w *worker) tickPotions(r *room, now time.Time) bool {
	state := &r.Potions
	if r.Player.HP <= 0 {
		return false
	}
	rules := w.e.Store.Cfg.Fixture.Potions
	if rules.TickMS <= 0 || rules.LevelsPerStep <= 0 {
		return false
	}
	beforeHP, beforeMP := r.Player.HP, r.Player.MP
	if now.After(state.NextTick) {
		// 空队列也推进节拍，喝药不重启时钟；长时间间隔最多处理一跳。
		state.NextTick = now.Add(time.Duration(rules.TickMS) * time.Millisecond)
		perTick := rules.BasePerTick
		step := int64(r.Role.Level) / rules.LevelsPerStep
		if perTick >= rules.QueueCap || step >= rules.QueueCap-perTick {
			perTick = rules.QueueCap
		} else {
			perTick += step
		}
		r.Player.HP, state.HP = restorePool(r.Player.HP, r.Player.MaxHP, state.HP, perTick)
		r.Player.MP, state.MP = restorePool(r.Player.MP, r.Player.MaxMP, state.MP, perTick)
	}
	// 满值清队列不受下一跳时间限制；另一条恢复队列继续保留。
	if r.Player.HP >= r.Player.MaxHP {
		state.HP = 0
	}
	if r.Player.MP >= r.Player.MaxMP {
		state.MP = 0
	}
	return r.Player.HP != beforeHP || r.Player.MP != beforeMP
}
