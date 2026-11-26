// 本文件汇总镜像样本，计算每组二项区间和同一随机种子换边后的胜率区间。
package game

import (
	"math"
	"math/rand"
	"slices"
)

// mirrorBalanceRate 以全部尝试场次为分母，保留原计数及 95% Wilson 区间。
type mirrorBalanceRate struct {
	Count int     `json:"count"`  // 观察到该事件的场数。
	Rate  float64 `json:"rate"`   // 0 到 1 的原始比例。
	Low   float64 `json:"low95"`  // 95% 区间下界。
	High  float64 `json:"high95"` // 95% 区间上界。
}

// mirrorBalanceWilson 避免全胜、全负或小样本时产生小于零或大于一的区间。
func mirrorBalanceWilson(wins, trials int) mirrorBalanceRate {
	if trials == 0 {
		return mirrorBalanceRate{High: 1}
	}
	z := 1.959963984540054
	n := float64(trials)
	rate := float64(wins) / n
	central := (rate + z*z/(2*n)) / (1 + z*z/n)
	radius := z * math.Sqrt(rate*(1-rate)/n+z*z/(4*n*n)) / (1 + z*z/n)
	return mirrorBalanceRate{Count: wins, Rate: rate, Low: max(0, central-radius), High: min(1, central+radius)}
}

// mirrorBalancePairRate 按种子成对抽样，每次同时保留相同种子的两个席位结果。
func mirrorBalancePairRate(first, reversed []uint32) mirrorBalanceRate {
	if len(first) == 0 || len(first) != len(reversed) {
		return mirrorBalanceRate{High: 1}
	}
	const simulatedSamples = 1024
	n := len(first)
	var values [3]int
	total := 0
	for index, outcome := range first {
		wins := 0
		if outcome == 1 {
			wins++
		}
		if reversed[index] == 2 {
			wins++
		}
		values[wins]++
		total += wins
	}
	rng := rand.New(rand.NewSource(20261126))
	var estimates [simulatedSamples]int
	for index := range estimates {
		for draw := 0; draw < n; draw++ {
			bin := rng.Intn(n)
			if bin >= values[0] {
				estimates[index]++
				if bin >= values[0]+values[1] {
					estimates[index]++
				}
			}
		}
	}
	slices.Sort(estimates[:])
	denominator := float64(2 * n)
	return mirrorBalanceRate{Count: total, Rate: float64(total) / denominator,
		Low: float64(estimates[25]) / denominator, High: float64(estimates[998]) / denominator}
}

// mirrorBalanceElapsed 以最近排名分位报告原服裁决耗时。
func mirrorBalanceElapsed(values []int64, fraction float64) int64 {
	index := int(math.Ceil(float64(len(values)) * fraction))
	if index < 1 {
		return 0
	}
	return values[index-1]
}

// mirrorBalanceSideTotals 累加同组一侧的原状态和按物品、技能分类的次数。
func mirrorBalanceSideTotals(dst *mirrorBalanceSideResult, input mirrorBalanceSideResult) {
	dst.EntryHP += input.EntryHP
	dst.RemainingHP += input.RemainingHP
	dst.OwnerHP += input.OwnerHP
	dst.EntryMP += input.EntryMP
	dst.RemainingMP += input.RemainingMP
	dst.SpellMPCost += input.SpellMPCost
	dst.Damage += input.Damage
	dst.MovementSteps += input.MovementSteps
	dst.MaterialItemID = input.MaterialItemID
	dst.MaterialSpent += input.MaterialSpent
	dst.SummonCalls += input.SummonCalls
	dst.FirstBirths += input.FirstBirths
	dst.ReplacementBirths += input.ReplacementBirths
	dst.PetDeaths += input.PetDeaths
	dst.IdleWhileAliveTicks += input.IdleWhileAliveTicks
	for id, count := range input.Potions {
		dst.Potions[id] += count
	}
	for id, count := range input.Spells {
		dst.Spells[id] += count
	}
	for id, count := range input.Attacks {
		dst.Attacks[id] += count
	}
}
