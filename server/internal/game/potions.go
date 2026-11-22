// 本文件校验药水指令和共享冷却，应用确认回执并以独立 HP、MP 队列逐帧恢复。
package game

import (
	"bytes"
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
	// HP 记录生命值，使用数值表示。
	HP int64
	// MP 记录魔法值，使用数值表示。
	MP int64
	// NextTick 记录下一模拟帧时间，使用时间点表示。
	NextTick time.Time
	// NextUse 记录下一状态使用，使用时间点表示。
	NextUse time.Time
	// LastAppliedSeq 记录上一次状态“applied”序号，使用数值表示。
	LastAppliedSeq int64
	// LastSourceSeq 记录上一次状态来源序号，使用数值表示。
	LastSourceSeq int64
}

// potionGate 检查原场景、存活和冷却等用药前置条件。
// r 指向当前执行器拥有并更新的目标房间，scene 记录场景，使用数值表示，now 提供当前时点，作为状态有效期和计时判断的基准。
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

// potionFingerprint 计算原用药帧的内容指纹，防止历史编号对应不同请求。
// f 记录协议帧。
func potionFingerprint(f wire.Frame) string {
	return persistence.Hash(append([]byte{byte(f.ID >> 8), byte(f.ID)}, f.Payload...))
}

// queuePotion 暂存忙时的原用药请求，保留后续核对所需身份。
// w 是调用该方法的房间执行器实例，r 指向当前执行器拥有并更新的目标房间，c 记录命令。
func (w *worker) queuePotion(r *room, c command) {
	const waitingLimit = 2 // 与现有重连等待共用有界队列，不能无限积累请求。
	// op 保存由操作上下文取得的操作。
	op := operationContext(c.frame)
	if op == nil || len(op.OpId) != 16 {
		w.failure(r, c.frame, p.CodeInvalidArgument)
		return
	}
	// id 区分当前步骤指向的目标对象。
	id := op.OpId
	// digest 保存由药水“fingerprint”取得的摘要。
	digest := potionFingerprint(c.frame)
	// known 保存根据待确认操作标识与十六进制计算的已知状态。
	known := r.PendingOperationID == domain.Hex(id)
	// conflict 保存根据已知状态与摘要与待确认“fingerprint”的计算结果计算的冲突。
	conflict := known && digest != r.PendingFingerprint
	// waiting 接收遍历等待时当前的元素。
	for _, waiting := range r.Waiting {
		if economic(waiting.frame.ID) && bytes.Equal(operationID(waiting.frame), id) {
			known = true
			conflict = conflict || potionFingerprint(waiting.frame) != digest
		}
	}
	if conflict {
		w.failure(r, c.frame, p.CodeOperationConflict)
		return
	}
	if len(r.Waiting) >= waitingLimit {
		// historical 保存根据客户端操作序号与当前数据的计算结果与经济操作序号与当前数据的计算结果的计算结果与客户端操作序号与无符号整数“64”的计算结果计算的“historical”。
		historical := op.CommandSeq > 0 && r.Role.EconomicSeq >= 0 && op.CommandSeq <= uint64(r.Role.EconomicSeq)
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

// queueAdd 在设定恢复上限内合并已确认的剩余恢复量。
// queue 记录队列，使用数值表示，amount 记录数量，使用数值表示，cap 记录上限，使用数值表示。
func queueAdd(queue, amount, cap int64) int64 {
	if queue >= cap || amount >= cap-queue {
		return cap
	}
	return queue + amount
}

// applyPotionReceipt 依原角色、水位与药水回执加载一次性恢复量。
// w 是调用该方法的房间执行器实例，r 指向当前执行器拥有并更新的目标房间，receipt 记录回执，使用操作表示，now 提供当前时点，作为状态有效期和计时判断的基准。
func (w *worker) applyPotionReceipt(r *room, receipt persistence.Operation, now time.Time) bool {
	// effect 保存药水，用于当前步骤中的效果。
	effect := receipt.Potion
	if receipt.Code != 0 || effect == nil || effect.Scene != strconv.FormatUint(r.Scene, 10) {
		return false
	}
	// watermark 保存当前处理所用的最高已推进序号，sequence 保存客户端操作序号，用于当前步骤中的序号。
	watermark, sequence := &r.Potions.LastAppliedSeq, receipt.CommandSeq
	switch receipt.Kind {
	case "use_item":
	case persistence.AssetAutoUsePotion:
		if effect.HP != 0 || effect.MP <= 0 {
			return false
		}
		watermark, sequence = &r.Potions.LastSourceSeq, receipt.SourceSeq
	default:
		return false
	}
	if sequence <= *watermark {
		return false
	}
	// rules 保存药水集合，用于当前步骤中的规则集合。
	rules := w.e.Store.Cfg.Fixture.Potions
	*watermark = sequence
	if effect.HP > 0 {
		r.Potions.HP = queueAdd(r.Potions.HP, effect.HP, rules.QueueCap)
	}
	if effect.MP > 0 {
		r.Potions.MP = queueAdd(r.Potions.MP, effect.MP, rules.QueueCap)
	}
	r.Potions.NextUse = now.Add(time.Duration(rules.UseDelayMS) * time.Millisecond)
	return true
}

// restorePool 计算本次恢复实际作用值与队列剩余量。
// value 记录值，使用数值表示，maximum 记录最大值，使用数值表示，queued 记录已排队，使用数值表示，perTick 记录“per”模拟帧，使用数值表示。
func restorePool(value, maximum, queued, perTick int64) (int64, int64) {
	if value >= maximum {
		return value, 0
	}
	// amount 保存当前处理所用的数量。
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

// tickPotions 在合法时钟下消耗已确认的生命和魔法恢复池。
// w 是调用该方法的房间执行器实例，r 指向当前执行器拥有并更新的目标房间，now 提供当前时点，作为状态有效期和计时判断的基准。
func (w *worker) tickPotions(r *room, now time.Time) bool {
	// state 保存当前处理所用的状态。
	state := &r.Potions
	if r.Player.HP <= 0 {
		return false
	}
	// rules 保存药水集合，用于当前步骤中的规则集合。
	rules := w.e.Store.Cfg.Fixture.Potions
	if rules.TickMS <= 0 || rules.LevelsPerStep <= 0 {
		return false
	}
	// beforeHP 保存生命值，用于当前步骤中的操作前的状态生命值，beforeMP 保存魔法值，用于当前步骤中的操作前的状态魔法值。
	beforeHP, beforeMP := r.Player.HP, r.Player.MP
	if now.After(state.NextTick) {
		// 空队列也推进节拍，喝药不重启时钟；长时间间隔最多处理一跳。
		state.NextTick = now.Add(time.Duration(rules.TickMS) * time.Millisecond)
		// perTick 保存基础值“per”模拟帧，用于当前步骤中的“per”模拟帧。
		perTick := rules.BasePerTick
		// step 保存根据整数“64”与等级集合“per”步骤计算的步骤。
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
