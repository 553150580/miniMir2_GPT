// 本文件校验药水指令和共享冷却，应用确认回执并以独立 HP、MP 队列逐帧恢复。
package game

import (
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
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
