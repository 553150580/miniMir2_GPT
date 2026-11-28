// 本文件负责扩展增伤与吸收的统一封顶及整型伤害计算。
package devconfig

import (
	"errors"
	"math"
)

const (
	maxExpandedHitBP uint32 = 2_000  // 工作簿当前两组独立候选上限均为 20%。
	hitBasis         uint64 = 10_000 // 扩展伤害属性的万分比单位。
)

// ExpandedHitBudget 保存命中结算使用的两组独立万分比上限。
type ExpandedHitBudget struct {
	DamageCapBP     uint32 `json:"damage_cap_bp"`     // 所有扩展增伤来源的组上限。
	AbsorptionCapBP uint32 `json:"absorption_cap_bp"` // 所有扩展吸收来源的组上限。
}

// HitBonuses 保存运行时投影。缺省字段省略，以兼容旧挑战快照及其内容摘要。
type HitBonuses struct {
	DamageBP     uint32 `json:"expanded_damage_bp,omitempty" bson:"expanded_damage_bp,omitempty"`         // 自身命中的扩展增伤。
	AbsorptionBP uint32 `json:"expanded_absorption_bp,omitempty" bson:"expanded_absorption_bp,omitempty"` // 自身受到命中的扩展吸收。
}

// ValidSnapshot 核对持久化挑战快照的扩展属性范围。
func (b HitBonuses) ValidSnapshot() bool {
	return b.DamageBP <= maxExpandedHitBP && b.AbsorptionBP <= maxExpandedHitBP
}

// trackLimit 返回指定炼体系别所属的扩展属性组上限。
func (b *ExpandedHitBudget) trackLimit(track uint32) uint32 {
	if b == nil {
		return 0
	}
	if track == BodyDamage {
		return b.DamageCapBP
	}
	return b.AbsorptionCapBP
}

// Combine 把同一角色的多来源贡献分别相加，按各自组上限截断。
func (b *ExpandedHitBudget) Combine(sources ...HitBonuses) HitBonuses {
	var total HitBonuses
	if b == nil {
		return total
	}
	for _, source := range sources {
		total.DamageBP += min(source.DamageBP, b.DamageCapBP-total.DamageBP)
		total.AbsorptionBP += min(source.AbsorptionBP, b.AbsorptionCapBP-total.AbsorptionBP)
	}
	return total
}

// Apply 保留原判定的未命中。对净伤害先取整增加攻击方贡献，再取整扣除目标方吸收。
func (b *ExpandedHitBudget) Apply(net int64, damageBP, absorptionBP uint32) int64 {
	if b == nil || net <= 0 {
		return net
	}
	amount := uint64(net)
	amount += hitPortion(amount, min(damageBP, b.DamageCapBP))
	amount -= hitPortion(amount, min(absorptionBP, b.AbsorptionCapBP))
	return int64(min(amount, math.MaxInt64))
}

// hitPortion 分开计算商和余数，避免大伤害与万分比直接相乘。
func hitPortion(amount uint64, bp uint32) uint64 {
	rate := uint64(bp)
	return amount/hitBasis*rate + amount%hitBasis*rate/hitBasis
}

// validateExpandedHit 校验战斗预算，禁止基础模板绕过来源与组上限。
func (f *Fixture) validateExpandedHit() error {
	if b := f.ExpandedHitBudget; b != nil && (f.RuleVersion < 30 || b.DamageCapBP == 0 ||
		b.DamageCapBP > maxExpandedHitBP || b.AbsorptionCapBP == 0 || b.AbsorptionCapBP > maxExpandedHitBP) {
		return errors.New("扩展增伤和吸收的战斗属性组上限无效")
	}
	for _, fighter := range f.Players {
		if fighter.HitBonuses != (HitBonuses{}) {
			return errors.New("角色基础模板不能直接配置扩展命中属性")
		}
	}
	for _, roster := range [][]Monster{f.Monsters, f.ChallengeMonsters} {
		for _, monster := range roster {
			if monster.HitBonuses != (HitBonuses{}) {
				return errors.New("怪物基础模板不能直接配置扩展命中属性")
			}
		}
	}
	if f.Pet.HitBonuses != (HitBonuses{}) {
		return errors.New("宠物基础模板不能直接配置扩展命中属性")
	}
	return nil
}
