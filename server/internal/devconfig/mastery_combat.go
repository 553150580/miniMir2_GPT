// 本文件定义暴击、攻速与全抗的统一预算和隔离怪物控制效果合同。
package devconfig

import "errors"

const (
	maxMasteryChanceBP uint32 = 2_000 // 限制隔离舞台内扩展概率和削减幅度的校验边界。
	maxMasteryExtraBP  uint32 = 5_000 // 一次直接命中额外伤害的结构校验边界。
	MasteryFrozen      uint32 = 91    // 冰冻观察状态编号。
	MasteryParalysis   uint32 = 92    // 麻痹观察状态编号。
	MasteryPetrified   uint32 = 93    // 石化观察状态编号。
)

// MasteryCombat 保存扩展贡献的组上限、暴击伤害上限和可独立验证的怪物控制来源。
type MasteryCombat struct {
	CriticalCapBP    uint32                    `json:"critical_cap_bp"`            // 合计暴击率上限。
	CriticalExtraBP  uint32                    `json:"critical_extra_bp"`          // 暴击额外伤害相对原净伤的比例。
	AttackSpeedCapBP uint32                    `json:"attack_speed_cap_bp"`        // 合计普攻间隔缩减上限。
	ResistanceCapBP  uint32                    `json:"resistance_cap_bp"`          // 全抗来源合计上限。
	MonsterControls  map[uint32]MonsterControl `json:"monster_controls,omitempty"` // 按实际怪物模板登记命中后控制效果。
}

// MonsterControl 针对确认的普通攻击命中声明一种离散状态。
type MonsterControl struct {
	StatusID   uint32 `json:"status_id"`   // 冰冻、麻痹或石化的状态编号。
	ChancePPM  uint32 `json:"chance_ppm"`  // 原命中之后触发的百万分率。
	DurationMS uint32 `json:"duration_ms"` // 无抗性时的持续毫秒。
}

// MasteryBonuses 是合并后的角色战斗投影，零值在旧挑战快照中不产生额外字段。
type MasteryBonuses struct {
	CriticalBP    uint32 `json:"critical_bp,omitempty" bson:"critical_bp,omitempty"`             // 当前直接命中的暴击率。
	AttackSpeedBP uint32 `json:"attack_speed_bp,omitempty" bson:"attack_speed_bp,omitempty"`     // 当前普攻间隔的扩展缩减。
	ResistanceBP  uint32 `json:"all_resistance_bp,omitempty" bson:"all_resistance_bp,omitempty"` // 同时用于六项各自的抵抗规则。
}

// ValidSnapshot 拒绝损坏或不受原预算约束的持久挑战属性。
func (b MasteryBonuses) ValidSnapshot() bool {
	return b.CriticalBP <= maxMasteryChanceBP && b.AttackSpeedBP <= maxMasteryChanceBP && b.ResistanceBP <= maxMasteryChanceBP
}

// trackLimit 返回元宝炼体系别所属的组上限。
func (m *MasteryCombat) trackLimit(track uint32) uint32 {
	if m == nil {
		return 0
	}
	switch track {
	case BodyCritical:
		return m.CriticalCapBP
	case BodySpeed:
		return m.AttackSpeedCapBP
	case BodyResistance:
		return m.ResistanceCapBP
	}
	return 0
}

// Combine 在相同属性组内合并来源并封顶，之后才允许概率、时钟和抗性计算。
func (m *MasteryCombat) Combine(sources ...MasteryBonuses) MasteryBonuses {
	var total MasteryBonuses
	if m == nil {
		return total
	}
	for _, source := range sources {
		total.CriticalBP += min(source.CriticalBP, m.CriticalCapBP-total.CriticalBP)
		total.AttackSpeedBP += min(source.AttackSpeedBP, m.AttackSpeedCapBP-total.AttackSpeedBP)
		total.ResistanceBP += min(source.ResistanceBP, m.ResistanceCapBP-total.ResistanceBP)
	}
	return total
}

// BodyMasteryBonuses 读取实际角色等级内的已确认炼体贡献。
func (f *Fixture) BodyMasteryBonuses(level, critical, speed, resistance uint32) MasteryBonuses {
	c := f.BodyTraining
	if c == nil || f.MasteryCombat == nil || critical == 0 && speed == 0 && resistance == 0 {
		return MasteryBonuses{}
	}
	limit := min(uint32(len(c.Steps)), level)
	return MasteryBonuses{
		CriticalBP:    c.trackBasisPoints(BodyCritical, min(critical, limit)),
		AttackSpeedBP: c.trackBasisPoints(BodySpeed, min(speed, limit)),
		ResistanceBP:  c.trackBasisPoints(BodyResistance, min(resistance, limit)),
	}
}

// ReduceByBP 从一个非负的原定点量扣除已封顶的份额，去掉小数部分后再减。
func ReduceByBP(base int64, bp uint32) int64 {
	if base <= 0 || bp == 0 {
		return base
	}
	return base - int64(hitPortion(uint64(base), min(bp, uint32(hitBasis))))
}

// validateMasteryCombat 检查组预算、前置战斗能力与实际怪物引用。
func (f *Fixture) validateMasteryCombat() error {
	m := f.MasteryCombat
	if m == nil {
		return nil
	}
	if f.RuleVersion < 30 || f.DamageFormula != "development_source_combat" || f.CombatLuck == nil || f.ExpandedHitBudget == nil ||
		m.CriticalExtraBP == 0 || m.CriticalExtraBP > maxMasteryExtraBP || m.CriticalCapBP == 0 ||
		m.AttackSpeedCapBP == 0 || m.ResistanceCapBP == 0 ||
		!(MasteryBonuses{m.CriticalCapBP, m.AttackSpeedCapBP, m.ResistanceCapBP}).ValidSnapshot() {
		return errors.New("暴击、攻速与全抗需要完整的战斗预算和前置战斗配置")
	}
	for id, control := range m.MonsterControls {
		_, statusInUse := f.Buff(control.StatusID)
		if !f.hasCombatMonster(id) || control.StatusID < MasteryFrozen || control.StatusID > MasteryPetrified ||
			statusInUse ||
			control.ChancePPM == 0 || control.ChancePPM > BodyChancePPM || control.DurationMS < 100 || control.DurationMS > 10_000 {
			return errors.New("怪物控制效果缺少有效模板、状态、概率或持续时间")
		}
	}
	return nil
}

// hasCombatMonster 只接受当前快照实际可用的普通或挑战怪物编号。
func (f *Fixture) hasCombatMonster(id uint32) bool {
	for _, roster := range [][]Monster{f.Monsters, f.ChallengeMonsters} {
		for _, monster := range roster {
			if id != 0 && monster.ID == id {
				return true
			}
		}
	}
	return false
}
