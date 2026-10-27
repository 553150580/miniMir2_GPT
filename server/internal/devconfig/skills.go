package devconfig

import (
	"encoding/json"
	"errors"
	"math"
)

type Skill struct {
	ID                        uint32   `json:"id"`
	Name                      string   `json:"name"`
	JobID                     uint32   `json:"job_id"`
	BookItemID                uint32   `json:"book_item_id"`
	LearnLevel                uint32   `json:"learn_level"`
	LearnedLevel              uint32   `json:"learned_level"`
	MaxLevel                  uint32   `json:"max_level"`
	Behavior                  string   `json:"behavior"`
	BuffID                    uint32   `json:"buff_id"`
	MaterialItemID            uint32   `json:"material_item_id"`
	MaterialQuantity          int64    `json:"material_quantity"`
	ActivationKind            string   `json:"activation_kind"`
	MPBaseCost                int64    `json:"mp_base_cost"`
	MPLevelCost               int64    `json:"mp_level_cost"`
	CooldownBaseMS            int64    `json:"cooldown_base_ms"`
	CooldownReductionMS       int64    `json:"cooldown_reduction_ms"`
	GlobalSpellMS             int64    `json:"global_spell_ms"`
	ActionMS                  int64    `json:"action_ms"`
	RangeCells                int      `json:"range_cells"`
	DelayBaseMS               int64    `json:"delay_base_ms"`
	ProjectileMSPerCell       int64    `json:"projectile_ms_per_cell"`
	TargetMoveToleranceCells  int      `json:"target_move_tolerance_cells"`
	AccuracyPerLevel          int64    `json:"accuracy_per_level"`
	AccuracyByLevel           []int64  `json:"accuracy_by_level"`
	TargetKind                string   `json:"target_kind"`
	AreaRadius                int      `json:"area_radius"`
	RequireLineOfSight        bool     `json:"require_line_of_sight"`
	HealingStatMultiplier     int64    `json:"healing_stat_multiplier"`
	AddCasterLevel            bool     `json:"add_caster_level"`
	UndeadMultiplierPPM       int64    `json:"undead_multiplier_ppm"`
	AutoPriority              int      `json:"auto_priority"`
	AutoMinTargets            int      `json:"auto_min_targets"`
	PowerBase                 int64    `json:"power_base"`
	PowerBonus                int64    `json:"power_bonus"`
	MPowerBase                int64    `json:"mpower_base"`
	MPowerBonus               int64    `json:"mpower_bonus"`
	MultiplierBasePPM         int64    `json:"multiplier_base_ppm"`
	MultiplierBonusPPM        int64    `json:"multiplier_bonus_ppm"`
	MaxAttackBonusByLevel     []int64  `json:"max_attack_bonus_by_level"`
	ReadyProcDenominator      uint32   `json:"ready_proc_denominator"`
	ReadyProcNumerators       []uint32 `json:"ready_proc_numerators"`
	SecondaryDirectionOffsets []int32  `json:"secondary_direction_offsets"`
	SecondaryDistanceCells    int      `json:"secondary_distance_cells"`
	SecondaryImpactMS         int64    `json:"secondary_impact_ms"`
	ChargeWindowMS            int64    `json:"charge_window_ms"`
	LineCells                 int      `json:"line_cells"`
	LineStepMS                int64    `json:"line_step_ms"`
	LineStopOnBlocked         bool     `json:"line_stop_on_blocked"`
	NonUndeadDivisor          int64    `json:"non_undead_divisor"`
	present                   map[string]bool
}

func (s *Skill) UnmarshalJSON(data []byte) error {
	type rawSkill Skill
	var raw rawSkill
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	*s = Skill(raw)
	s.present = map[string]bool{}
	for name, value := range fields {
		s.present[name] = string(value) != "null"
	}
	return nil
}

type HealingRules struct {
	TickMS               int64 `json:"tick_ms"`
	BasePerTick          int64 `json:"base_per_tick"`
	LevelsPerStep        int64 `json:"levels_per_step"`
	QueueFractionDivisor int64 `json:"queue_fraction_divisor"`
	QueueCap             int64 `json:"queue_cap"`
	PetBasePerTick       int64 `json:"pet_base_per_tick"`
}
type InitialSkillBook struct {
	JobID  uint32 `json:"job_id"`
	ItemID uint32 `json:"item_id"`
}
type PlayerGrowth struct {
	JobID                  uint32  `json:"job_id"`
	HPBase                 int64   `json:"hp_base"`
	MPBase                 int64   `json:"mp_base"`
	HPGain                 float32 `json:"hp_gain"`
	HPGainRate             float32 `json:"hp_gain_rate"`
	MPGain                 float32 `json:"mp_gain"`
	MPGainRate             float32 `json:"mp_gain_rate"`
	AttackDivisor          float32 `json:"attack_divisor"`
	DefenceMaxDivisor      float32 `json:"defence_max_divisor"`
	MagicDivisor           float32 `json:"magic_divisor"`
	TaoDivisor             float32 `json:"tao_divisor"`
	MagicDefenceMinDivisor float32 `json:"magic_defence_min_divisor"`
	MagicDefenceMaxDivisor float32 `json:"magic_defence_max_divisor"`
}

// Crystal MagicInfo.cs:180–211：显式逐步转回float32，保留C# Single中间舍入。
func (s Skill) SourcePower(level uint32, mpower, power int64) int64 {
	quarter := float32(float32(mpower) / float32(4))
	scaled := float32(quarter * float32(level+1))
	return int64(math.RoundToEven(float64(float32(scaled + float32(power)))))
}

func (s Skill) SourceDamage(level uint32, base, power int64) int64 {
	baseMultiplier := float32(float64(s.MultiplierBasePPM) / 1000000)
	bonusMultiplier := float32(float64(s.MultiplierBonusPPM) / 1000000)
	bonus := float32(float32(level) * bonusMultiplier)
	multiplier := float32(baseMultiplier + bonus)
	amount := float32(float32(base+power) * multiplier)
	if float64(amount) >= math.MaxInt32 {
		return math.MaxInt32
	}
	return int64(amount)
}

func (f *Fixture) Skill(id uint32) (Skill, bool) {
	for _, s := range f.Skills {
		if s.ID == id {
			return s, true
		}
	}
	return Skill{}, false
}

func (f *Fixture) PlayerAtLevel(job, level uint32) (Fighter, bool) {
	p, ok := f.Player(job)
	if !ok {
		return p, false
	}
	p.Level = level
	for _, g := range f.PlayerGrowth {
		if g.JobID != job {
			continue
		}
		l := float32(level)
		// Crystal Shared/BaseStats.cs:155–182；中间运算保留C# float32，最后截断。
		hp := float32(g.HPBase) + float32(float32(l/g.HPGain)+g.HPGainRate)*l
		if job == 0 {
			hp = float32(g.HPBase) + float32(float32(float32(l/g.HPGain)+g.HPGainRate)+float32(l/20))*l
		}
		mp := float32(g.MPBase) + float32(l*g.MPGain) + float32(l*g.MPGainRate)
		if job == 1 {
			mp = float32(float32(g.MPBase)+float32(float32(float32(l/g.MPGain)+2)*2.2)*l) + float32(l*g.MPGainRate)
		}
		if job == 2 {
			mp = float32(float32(g.MPBase)+float32(float32(l/g.MPGain)*2.2)*l) + float32(l*g.MPGainRate)
		}
		p.HP = int64(math.Min(float64(hp), math.MaxInt32))
		p.MP = int64(math.Min(float64(mp), math.MaxInt32))
		stat := func(div float32) int64 {
			if div <= 0 {
				return 0
			}
			return int64(l / div)
		}
		p.AttackMin = stat(g.AttackDivisor)
		p.AttackMax = p.AttackMin
		p.DefenceMax = stat(g.DefenceMaxDivisor)
		p.MagicMin = stat(g.MagicDivisor)
		p.MagicMax = p.MagicMin
		p.TaoMin = stat(g.TaoDivisor)
		p.TaoMax = p.TaoMin
		p.MagicDefenceMin = stat(g.MagicDefenceMinDivisor)
		p.MagicDefenceMax = stat(g.MagicDefenceMaxDivisor)
		break
	}
	return p, true
}

func (f *Fixture) ValidateSkills() error {
	seen := map[uint32]bool{}
	for _, s := range f.Skills {
		book, ok := f.Item(s.BookItemID)
		if s.ID == 0 || seen[s.ID] || s.Name == "" || s.JobID > 2 || !ok || book.BookSkillID != s.ID || s.LearnedLevel > 255 || s.RangeCells < 0 || s.RangeCells > f.Map.Width || s.TargetMoveToleranceCells < 0 || s.TargetMoveToleranceCells > f.Map.Width {
			return errors.New("技能身份、职业、书或范围缺失")
		}
		if !supportedSkill(s.Behavior) {
			return errors.New("当前候选不支持此技能行为")
		}
		for _, v := range []int64{s.MPBaseCost, s.MPLevelCost, s.CooldownBaseMS, s.CooldownReductionMS, s.GlobalSpellMS, s.ActionMS, s.DelayBaseMS, s.ProjectileMSPerCell, s.AccuracyPerLevel, s.PowerBase, s.PowerBonus, s.MPowerBase, s.MPowerBonus, s.MultiplierBasePPM, s.MultiplierBonusPPM} {
			if v < 0 || v > math.MaxInt32 {
				return errors.New("技能参数越界")
			}
		}
		if s.MaxLevel < s.LearnedLevel || s.MaxLevel > 255 || s.CooldownReductionMS*int64(s.MaxLevel) > s.CooldownBaseMS {
			return errors.New("技能冷却不能为负")
		}
		seen[s.ID] = true
	}
	if len(seen) == 0 || len(f.PlayerGrowth) != len(f.Players) {
		return errors.New("职业成长与技能定义不完整")
	}
	growth := map[uint32]bool{}
	for _, g := range f.PlayerGrowth {
		if g.JobID > 2 || growth[g.JobID] || g.HPBase <= 0 || g.MPBase < 0 || g.HPGain <= 0 || g.MPGain <= 0 {
			return errors.New("基础成长参数无效")
		}
		for _, value := range []float32{g.HPGain, g.HPGainRate, g.MPGain, g.MPGainRate, g.AttackDivisor, g.DefenceMaxDivisor, g.MagicDivisor, g.TaoDivisor, g.MagicDefenceMinDivisor, g.MagicDefenceMaxDivisor} {
			if value < 0 || math.IsNaN(float64(value)) || math.IsInf(float64(value), 0) {
				return errors.New("成长参数须为有限非负数")
			}
		}
		growth[g.JobID] = true
	}
	h := f.Healing
	if h.TickMS <= 0 || h.TickMS > math.MaxInt32 || h.BasePerTick <= 0 || h.LevelsPerStep <= 0 || h.QueueFractionDivisor <= 0 || h.QueueCap <= 0 || h.QueueCap > math.MaxInt32 || h.PetBasePerTick <= 0 {
		return errors.New("治疗队列参数无效")
	}
	for _, b := range f.InitialSkillBooks {
		item, ok := f.Item(b.ItemID)
		s, exists := f.Skill(item.BookSkillID)
		if !ok || !exists || s.JobID != b.JobID {
			return errors.New("初始职业书引用不符")
		}
	}
	for _, a := range append(append(append([]Fighter{}, f.Players...), f.Pet), monsterFighters(f.Monsters)...) {
		if a.MagicMin < 0 || a.MagicMax < a.MagicMin || a.TaoMin < 0 || a.TaoMax < a.TaoMin || a.DefenceMin < 0 || a.DefenceMax < a.DefenceMin || a.MagicDefenceMin < 0 || a.MagicDefenceMax < a.MagicDefenceMin || a.Accuracy < 0 || a.Agility < 0 || a.MagicResist < 0 || a.MagicResistWeight <= 0 || a.MagicResist > a.MagicResistWeight {
			return errors.New("AC/MAC/命中/抗魔区间无效")
		}
	}
	upgradesEnabled := len(f.SkillUpgrades) > 0 || len(f.SkillShop) > 0 || f.SkillEconomy.PageItemID != 0
	if f.CombatTiming.PlayerAttackBaseMS > 0 {
		if err := f.ValidateCombat(); err != nil {
			return err
		}
	}
	for _, s := range f.Skills {
		upgradesEnabled = upgradesEnabled || s.MaxLevel > s.LearnedLevel
	}
	if err := f.ValidateBuffs(); err != nil {
		return err
	}
	if upgradesEnabled {
		return f.ValidateSkillEconomy()
	}
	return nil
}

func monsterFighters(monsters []Monster) []Fighter {
	out := make([]Fighter, 0, len(monsters))
	for _, monster := range monsters {
		out = append(out, monster.Fighter)
	}
	return out
}
