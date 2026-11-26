// 本文件在固定时钟与种子下运行实际镜像格战，从原事件和两侧临时状态采集对照结果。
package game

import (
	"fmt"
	"math/rand"
	"time"

	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
)

const mirrorBalanceTick = 100 * time.Millisecond // 与正常 GameNode 的格战推进间隔一致。

// mirrorBalanceSideResult 保存一场内原临时身体与耗材的变化。
type mirrorBalanceSideResult struct {
	EntryHP             int64            `json:"entry_hp"`               // 原入场主人和原宠的超时分母。
	RemainingHP         int64            `json:"remaining_counted_hp"`   // 终场仍计入超时比较的原生命。
	OwnerHP             int64            `json:"remaining_owner_hp"`     // 终场主人自身生命。
	EntryMP             int64            `json:"entry_mp"`               // 当前快照主人魔法上限。
	RemainingMP         int64            `json:"remaining_mp"`           // 最后一帧主人当前魔法。
	SpellMPCost         int64            `json:"spell_mp_cost"`          // 已受理技能的原蓝耗总量，与喝药后的净变化独立。
	Damage              int64            `json:"damage"`                 // 命中事件确认的实际输出。
	MovementSteps       uint64           `json:"owner_movement_steps"`   // 主人实际提交的移动修订量。
	MaterialItemID      uint32           `json:"material_item_id"`       // 原专槽的唯一耗材模板。
	MaterialSpent       int64            `json:"material_spent"`         // 本场实际扣除的临时耗材张数或份数。
	Potions             map[uint32]int64 `json:"potions_by_item_id"`     // 按原物品模板统计已使用药水量。
	Spells              map[uint32]int64 `json:"spells_by_skill_id"`     // 按原技能号统计受理的法术。
	Attacks             map[uint32]int64 `json:"attacks_by_skill_id"`    // 按原技能号统计受理的攻击，零是普通攻击。
	SummonCalls         int64            `json:"summon_casts_accepted"`  // 实际受理的骷髅或神兽施法次数。
	FirstBirths         int64            `json:"first_births"`           // 无入场宠时本场首个成功新生。
	ReplacementBirths   int64            `json:"replacement_births"`     // 入场宠或首个新宠阵亡后的新生次数。
	PetDeaths           int64            `json:"pet_deaths"`             // 实际死亡的原宠和场内新宠数量。
	IdleWhileAliveTicks int64            `json:"idle_while_alive_ticks"` // 双方主人存活时本侧未移动或受理动作的帧数。
}

// mirrorBalanceBattleResult 保留计算最终比分及成对估计需要的真实单场观测。
type mirrorBalanceBattleResult struct {
	winner     uint32                     // 1 进攻胜，2 防守胜，0 系统中断。
	cause      uint32                     // 原场终止原因。
	durationMS int64                      // 使用原裁决时刻计算的虚拟耗时。
	timeoutTie bool                       // 超时双方有效生命比例恰好相同。
	sides      [2]mirrorBalanceSideResult // 攻守角色的实际临时结果。
}

// mirrorBalanceActorSide 读取场内固定实体身份。每侧新宠的句柄按奇偶交替。
func mirrorBalanceActorSide(handle uint32) (int, bool) {
	switch {
	case handle == 1 || handle == 2:
		return int(handle - 1), true
	case handle >= 101 && handle <= 102+2*devconfig.MirrorArenaMaxBirthsPerSide:
		return int((handle - 101) % 2), true
	default:
		return 0, false
	}
}

// mirrorBalanceObserve 消费原格战实际发出的动作数据，不从伤害公式重算命中。
func mirrorBalanceObserve(fixture *devconfig.Fixture, battle *mirrorCombat, output *mirrorBalanceBattleResult) error {
	if len(battle.observations) == mirrorObservationLimit {
		return fmt.Errorf("镜像模拟单帧观察达到容量边界")
	}
	for _, entry := range battle.drainObservations() {
		switch message := entry.Message.(type) {
		case *pb.CastAccepted:
			index, mapped := mirrorBalanceActorSide(message.CasterHandle)
			skill, found := fixture.Skill(message.SkillId)
			if !mapped || !found {
				return fmt.Errorf("实际施法未找到原侧或技能 %d/%d", message.CasterHandle, message.SkillId)
			}
			side := &output.sides[index]
			cost, valid := teleportManaCost(&battle.sides[index].room.Player, skill, message.SkillLevel)
			if !valid {
				return fmt.Errorf("实际施法无法复核原蓝耗 %d", skill.ID)
			}
			side.SpellMPCost += cost
			side.Spells[skill.ID]++
			if skill.AttackSlotSummon() {
				side.SummonCalls++
			}
		case *pb.AttackAccepted:
			if index, mapped := mirrorBalanceActorSide(message.CasterHandle); mapped {
				output.sides[index].Attacks[message.SkillId]++
			}
		case *pb.CombatImpact:
			if index, mapped := mirrorBalanceActorSide(message.CasterHandle); mapped && message.Outcome == 1 && !message.Healing && message.Amount > 0 {
				output.sides[index].Damage += message.Amount
			}
		}
	}
	return nil
}

// runMirrorBalanceBattle 使用原开场、棋盘及事件推进。每场重置随机源，随机序列只由输入种子决定。
func runMirrorBalanceBattle(w *worker, arena *devconfig.MirrorArenaConfig, snapshots [2]domain.MirrorCombatSnapshot, seed int64) (mirrorBalanceBattleResult, error) {
	var output mirrorBalanceBattleResult
	if arena.Duration <= 0 || arena.Duration%mirrorBalanceTick != 0 {
		return output, fmt.Errorf("镜像候选时限与正常游戏节拍不整除: %s", arena.Duration)
	}
	begin := time.Unix(1800000000, 0).UTC()
	end := begin.Add(arena.Duration)
	w.roll = rand.New(rand.NewSource(seed)).Int63n
	battle := w.newMirrorCombat(arena, snapshots, begin, end, 1)
	if battle == nil {
		return output, fmt.Errorf("实际格战拒绝冻结快照")
	}
	var lastMovement [2]uint64
	for index := range battle.sides {
		output.sides[index].Potions = make(map[uint32]int64)
		output.sides[index].Spells = make(map[uint32]int64)
		output.sides[index].Attacks = make(map[uint32]int64)
	}
	fixture := &w.e.Store.Cfg.Fixture
	for at := begin; !at.After(end) && battle.result.Cause == 0; at = at.Add(mirrorBalanceTick) {
		previousMoves := lastMovement
		previousCasts := [2]uint32{battle.sides[0].room.Player.CastSequence, battle.sides[1].room.Player.CastSequence}
		previousAttacks := [2]uint32{battle.sides[0].room.Player.AttackSequence, battle.sides[1].room.Player.AttackSequence}
		w.advanceMirrorCombat(battle, at)
		if err := mirrorBalanceObserve(fixture, battle, &output); err != nil {
			return output, err
		}
		for index, side := range battle.sides {
			body := &side.room.Player
			lastMovement[index] = body.MovementRevision
			if battle.result.Cause == 0 && body.HP > 0 && battle.sides[1-index].room.Player.HP > 0 &&
				body.MovementRevision == previousMoves[index] && body.CastSequence == previousCasts[index] && body.AttackSequence == previousAttacks[index] {
				output.sides[index].IdleWhileAliveTicks++
			}
		}
	}
	if battle.result.Cause == 0 || battle.result.FinishedAt.Before(begin) || battle.result.FinishedAt.After(end) {
		return output, fmt.Errorf("实际格战没有在冻结时限内形成合法终态")
	}
	output.winner, output.cause = battle.result.Winner, battle.result.Cause
	output.durationMS = battle.result.FinishedAt.Sub(begin).Milliseconds()
	left, right := battle.sides[0], battle.sides[1]
	output.timeoutTie = output.cause == mirrorTimeout && left.survivingLife()*right.initialLife == right.survivingLife()*left.initialLife
	for index, side := range battle.sides {
		entry, result := snapshots[index], &output.sides[index]
		result.EntryHP, result.RemainingHP, result.OwnerHP = side.initialLife, side.survivingLife(), side.room.Player.HP
		result.EntryMP, result.RemainingMP = entry.Fighter.MP, side.room.Player.MP
		result.MovementSteps = lastMovement[index]
		if entry.Material != nil {
			result.MaterialItemID, result.MaterialSpent = entry.Material.ItemID, entry.Material.Quantity-side.material.Quantity
		}
		for offset, potion := range entry.Potions {
			if used := potion.Quantity - side.potions[offset].Quantity; used != 0 {
				result.Potions[potion.ItemID] = used
			}
		}
		result.ReplacementBirths = int64(side.births)
		result.PetDeaths = int64(side.births)
		if entry.Pet != nil {
			result.PetDeaths++
		}
		if side.room.Pet != nil && side.room.Pet.HP > 0 {
			result.PetDeaths--
		}
		if entry.Pet == nil && side.births != 0 {
			result.FirstBirths++
			result.ReplacementBirths--
		}
		if result.MaterialSpent < 0 || result.SummonCalls < int64(side.births) || result.RemainingHP > result.EntryHP || result.PetDeaths < 0 {
			return output, fmt.Errorf("第%d侧原耗材、出生次数或超时生命失衡", index+1)
		}
	}
	return output, nil
}
