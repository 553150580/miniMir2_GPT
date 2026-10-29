package devconfig

import (
	"encoding/json"
	"errors"
	"math"
)

const CastingSupplySlot uint32 = 100

type BuffDefinition struct {
	ID                     uint32  `json:"id"`
	Kind                   string  `json:"kind"`
	ValueByLevel           []int64 `json:"value_by_level"`
	TargetLevelDivisor     int64   `json:"target_level_divisor"`
	TargetLevelAdd         int64   `json:"target_level_add"`
	DurationKind           string  `json:"duration_kind"`
	DurationMS             int64   `json:"duration_ms,omitempty"`
	StatAdd                int64   `json:"stat_add"`
	StatMultiplier         int64   `json:"stat_multiplier"`
	DurationSecondsByLevel []int64 `json:"duration_seconds_by_level"`
	TickMS                 int64   `json:"tick_ms"`
	HitDurationLossMSPerHP int64   `json:"hit_duration_loss_ms_per_hp"`
	StackPolicy            string  `json:"stack_policy"`
	RemoveOnPlayerDeath    bool    `json:"remove_on_player_death"`
	AllowDeadTargets       bool    `json:"allow_dead_targets"`
	present                map[string]bool
}

func (b *BuffDefinition) UnmarshalJSON(data []byte) error {
	type plain BuffDefinition
	var value plain
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	*b = BuffDefinition(value)
	b.present = map[string]bool{}
	for key, raw := range fields {
		b.present[key] = string(raw) != "null"
	}
	return nil
}

type CastingSupplyRules struct {
	Enabled bool     `json:"enabled"`
	Slot    uint32   `json:"slot"`
	ItemIDs []uint32 `json:"item_ids"`
	present map[string]bool
}

func (c *CastingSupplyRules) UnmarshalJSON(data []byte) error {
	type plain CastingSupplyRules
	var value plain
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	*c = CastingSupplyRules(value)
	c.present = map[string]bool{}
	for key, raw := range fields {
		c.present[key] = string(raw) != "null"
	}
	return nil
}

func (f *Fixture) Buff(id uint32) (BuffDefinition, bool) {
	for _, b := range f.Buffs {

		if b.ID == id {
			return b, true
		}
	}
	return BuffDefinition{}, false
}

func (f *Fixture) IsCastingSupply(id uint32) bool {
	for _, allowed := range f.CastingSupply.ItemIDs {
		if id == allowed {
			return true
		}
	}
	return false
}

func (f *Fixture) ValidateBuffs() error {
	if len(f.Buffs) == 0 {
		if f.RuleVersion >= 9 {
			return errors.New("v9状态子集缺少唯一状态定义")
		}
		for _, s := range f.Skills {
			if s.BuffID != 0 || s.MaterialItemID != 0 || s.MaterialQuantity != 0 {
				return errors.New("状态技能缺少唯一状态定义")
			}
		}
		return nil
	}
	if f.CastingSupply.Slot != CastingSupplySlot || len(f.CastingSupply.ItemIDs) == 0 || f.Pet.Level == 0 {
		return errors.New("状态场景须明确独立耗材槽、材料白名单与宠物定义等级")
	}
	if f.RuleVersion >= 9 {
		for _, field := range []string{"enabled", "slot", "item_ids"} {
			if !f.CastingSupply.present[field] {
				return errors.New("v9耗材开关、专槽与白名单必须显式输出：" + field)
			}
		}
	}
	supplies := map[uint32]bool{}
	for _, id := range f.CastingSupply.ItemIDs {
		item, ok := f.Item(id)
		if !ok || supplies[id] || item.IsEquipment || item.EquipmentSlot != CastingSupplySlot || !item.Stackable || item.MaxStack < 0 || f.CastingSupply.Enabled && item.MaxStack == 0 {
			return errors.New("护符须为有界数量单位，未定单位只可停用，不得伪装成强化装备")
		}
		supplies[id] = true
	}
	seen := map[uint32]bool{}
	kinds := map[string]bool{}
	for _, b := range f.Buffs {
		if f.RuleVersion >= 9 {
			for _, field := range []string{"id", "kind", "value_by_level", "target_level_divisor", "target_level_add", "duration_kind", "stat_add", "stat_multiplier", "duration_seconds_by_level", "tick_ms", "hit_duration_loss_ms_per_hp", "stack_policy", "remove_on_player_death", "allow_dead_targets"} {
				if !b.present[field] {
					return errors.New("v9状态参数必须显式输出，不能缺失或为null：" + field)
				}
			}
		}
		if b.ID == 0 || seen[b.ID] || kinds[b.Kind] || b.TickMS <= 0 || b.TickMS > math.MaxInt32 {
			return errors.New("状态身份或离散时钟无效")
		}
		seen[b.ID] = true
		kinds[b.Kind] = true
		for _, value := range append(append([]int64{b.TargetLevelDivisor, b.TargetLevelAdd, b.StatAdd, b.StatMultiplier, b.HitDurationLossMSPerHP, b.DurationMS}, b.ValueByLevel...), b.DurationSecondsByLevel...) {
			if value < 0 || value > math.MaxInt32 {
				return errors.New("状态数值必须为有界非负整数")
			}
		}
		switch b.Kind {
		case "teleport_mana_penalty":
			if f.RuleVersion < 15 || b.DurationKind != "fixed_ms" || b.DurationMS <= 0 || !b.present["duration_ms"] || b.StackPolicy != "refresh_duration_keep_values" || b.RemoveOnPlayerDeath || !b.AllowDeadTargets || len(b.DurationSecondsByLevel) != 0 || b.TargetLevelDivisor != 0 || b.TargetLevelAdd != 0 || b.StatAdd != 0 || b.StatMultiplier != 0 || b.HitDurationLossMSPerHP != 0 {
				return errors.New("传送耗蓝惩罚须明确单一固定时长、尸体及重施保留规则")
			}
			for _, value := range b.ValueByLevel {
				if value > 100 {
					return errors.New("传送耗蓝惩罚百分比越界")
				}
			}
		case "magic_shield":
			if b.DurationKind != "mc_source_power" || b.StackPolicy != "ignore_present_on_impact" || !b.RemoveOnPlayerDeath || b.AllowDeadTargets || b.HitDurationLossMSPerHP <= 0 || b.TargetLevelDivisor != 0 || b.TargetLevelAdd != 0 || b.StatMultiplier != 0 {
				return errors.New("魔法盾的生效、重施、死亡与削时管线不完整")
			}
			for _, percent := range b.ValueByLevel {
				if percent > 100 {
					return errors.New("状态减伤百分比越界")
				}
			}
			for _, seconds := range b.DurationSecondsByLevel {
				if seconds != 0 {
					return errors.New("魔法盾不可携带未参与公式的固定秒数")
				}
			}
		case "max_physical_defence", "max_magic_defence":
			if b.DurationKind != "sc_rank_linear" || b.StackPolicy != "refresh_duration_keep_values" || b.RemoveOnPlayerDeath || !b.AllowDeadTargets || b.TargetLevelDivisor <= 0 || b.HitDurationLossMSPerHP != 0 || b.StatAdd != 0 {
				return errors.New("道士防御状态必须保留初值、尸体与持续时钟规则")
			}
			for _, value := range b.ValueByLevel {
				if value != 0 {
					return errors.New("防御状态不可携带未参与公式的减伤比例")
				}
			}
		default:
			return errors.New("当前状态子集不支持该状态类型")
		}
		if b.Kind != "teleport_mana_penalty" && b.DurationMS != 0 {
			return errors.New("动态时长状态不能另带未使用的固定毫秒值")
		}
	}
	used := map[uint32]bool{}
	for _, s := range f.Skills {
		if f.RuleVersion >= 9 {
			for _, field := range []string{"buff_id", "material_item_id", "material_quantity"} {
				if !s.present[field] {
					return errors.New("v9技能状态与耗材字段必须显式输出：" + field)
				}
			}
		}
		if s.BuffID == 0 {
			if s.Behavior == "summon_pet" {
				if err := validateSummonSkill(s); err != nil {
					return err
				}
				if !supplies[s.MaterialItemID] {
					return errors.New("召唤材料不在独立护符白名单中")
				}
				continue
			}
			if s.MaterialItemID != 0 || s.MaterialQuantity != 0 {
				return errors.New("非状态技能不能附加未实现的施法耗材")
			}
			continue
		}
		b, ok := f.Buff(s.BuffID)
		if !ok || used[b.ID] || len(b.ValueByLevel) != int(s.MaxLevel)+1 || b.Kind != "teleport_mana_penalty" && len(b.DurationSecondsByLevel) != int(s.MaxLevel)+1 {
			return errors.New("技能状态引用或逐阶状态参数不完整")
		}
		used[b.ID] = true
		wanted := map[string]string{"magic_shield": "magic_shield", "soul_shield": "max_magic_defence", "blessed_armour": "max_physical_defence", "teleport": "teleport_mana_penalty"}[s.Behavior]
		withoutMaterial := b.Kind == "magic_shield" || b.Kind == "teleport_mana_penalty"
		if b.Kind != wanted || withoutMaterial && (s.MaterialItemID != 0 || s.MaterialQuantity != 0) || !withoutMaterial && (!supplies[s.MaterialItemID] || s.MaterialQuantity <= 0) {
			return errors.New("状态技能行为与永久耗材引用不一致")
		}
	}
	if len(used) != len(f.Buffs) {
		return errors.New("状态定义不能包含未开放技能的隐形效果")
	}
	return nil
}
