// 本文件读取战斗状态定义，区分有来源效果和关闭的未知耗材规则。

package devconfig

import (
	"encoding/json"
	"errors"
	"math"
)

// CastingSupplySlot 规定施法耗材槽位的初始取值。
const CastingSupplySlot uint32 = 100

// BuffDefinition 保存状态效果定义所需的状态。
type BuffDefinition struct {
	// ID 区分当前步骤指向的目标对象。
	ID uint32 `json:"id"`
	// Kind 记录类型，使用文本表示。
	Kind string `json:"kind"`
	// ValueByLevel 记录值“by”等级，使用数值列表表示。
	ValueByLevel []int64 `json:"value_by_level"`
	// TargetLevelDivisor 记录目标等级“divisor”，使用数值表示。
	TargetLevelDivisor int64 `json:"target_level_divisor"`
	// TargetLevelAdd 记录目标等级新增，使用数值表示。
	TargetLevelAdd int64 `json:"target_level_add"`
	// DurationKind 记录持续时间类型，使用文本表示。
	DurationKind string `json:"duration_kind"`
	// DurationMS 记录持续时间毫秒，使用数值表示。
	DurationMS int64 `json:"duration_ms,omitempty"`
	// StatAdd 记录属性新增，使用数值表示。
	StatAdd int64 `json:"stat_add"`
	// StatMultiplier 记录属性倍率，使用数值表示。
	StatMultiplier int64 `json:"stat_multiplier"`
	// DurationSecondsByLevel 记录持续时间秒集合“by”等级，使用数值列表表示。
	DurationSecondsByLevel []int64 `json:"duration_seconds_by_level"`
	// TickMS 记录模拟帧毫秒，使用数值表示。
	TickMS int64 `json:"tick_ms"`
	// HitDurationLossMSPerHP 记录命中持续时间损失毫秒“per”生命值，使用数值表示。
	HitDurationLossMSPerHP int64 `json:"hit_duration_loss_ms_per_hp"`
	// StackPolicy 记录堆叠策略，使用文本表示。
	StackPolicy string `json:"stack_policy"`
	// RemoveOnPlayerDeath 标记是否处于移除启用状态玩家死亡状态。
	RemoveOnPlayerDeath bool `json:"remove_on_player_death"`
	// AllowDeadTargets 标记是否处于“allow”死亡状态目标集合状态。
	AllowDeadTargets bool `json:"allow_dead_targets"`
	// present 记录“present”，使用按文本索引的布尔状态映射表示。
	present map[string]bool
}

// UnmarshalJSON 执行反序列化JSON，并把失败交给调用方处理。
// b 是调用该方法的状态效果定义实例，data 携带供当前流程读取或解析的原始字节。
func (b *BuffDefinition) UnmarshalJSON(data []byte) error {
	// plain 描述“plain”。
	type plain BuffDefinition
	// value 记录值，使用“plain”表示。
	var value plain
	// err 接收反序列化过程中可能产生的错误，供后续失败分支处理。
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	// fields 记录字段集合，使用按文本索引的原始值消息映射表示。
	var fields map[string]json.RawMessage
	// err 接收反序列化过程中可能产生的错误，供后续失败分支处理。
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	*b = BuffDefinition(value)
	b.present = map[string]bool{}
	// key 定位字段集合中正在处理的成员，raw 接收遍历字段集合时当前的元素。
	for key, raw := range fields {
		b.present[key] = string(raw) != "null"
	}
	return nil
}

// CastingSupplyRules 保存施法耗材规则集合所需的状态。
type CastingSupplyRules struct {
	// Enabled 标记是否处于已开启状态。
	Enabled bool `json:"enabled"`
	// Slot 记录槽位，使用数值表示。
	Slot uint32 `json:"slot"`
	// ItemIDs 记录道具i“ds”，使用数值列表表示。
	ItemIDs []uint32 `json:"item_ids"`
	// present 记录“present”，使用按文本索引的布尔状态映射表示。
	present map[string]bool
}

// UnmarshalJSON 执行反序列化JSON，并把失败交给调用方处理。
// c 是调用该方法的施法耗材规则集合实例，data 携带供当前流程读取或解析的原始字节。
func (c *CastingSupplyRules) UnmarshalJSON(data []byte) error {
	// plain 描述“plain”。
	type plain CastingSupplyRules
	// value 记录值，使用“plain”表示。
	var value plain
	// err 接收反序列化过程中可能产生的错误，供后续失败分支处理。
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	// fields 记录字段集合，使用按文本索引的原始值消息映射表示。
	var fields map[string]json.RawMessage
	// err 接收反序列化过程中可能产生的错误，供后续失败分支处理。
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	*c = CastingSupplyRules(value)
	c.present = map[string]bool{}
	// key 定位字段集合中正在处理的成员，raw 接收遍历字段集合时当前的元素。
	for key, raw := range fields {
		c.present[key] = string(raw) != "null"
	}
	return nil
}

// Buff 处理状态效果。
// f 是调用该方法的开发测试配置实例，id 区分当前步骤指向的目标对象。
func (f *Fixture) Buff(id uint32) (BuffDefinition, bool) {
	// b 接收遍历“buffs”时当前的元素。
	for _, b := range f.Buffs {

		if b.ID == id {
			return b, true
		}
	}
	return BuffDefinition{}, false
}

// IsCastingSupply 判断是否属于施法耗材。
// f 是调用该方法的开发测试配置实例，id 区分当前步骤指向的目标对象。
func (f *Fixture) IsCastingSupply(id uint32) bool {
	// allowed 接收遍历道具i“ds”时当前的元素。
	for _, allowed := range f.CastingSupply.ItemIDs {
		if id == allowed {
			return true
		}
	}
	return false
}

// ValidateBuffs 校验“buffs”。
// f 是调用该方法的开发测试配置实例。
func (f *Fixture) ValidateBuffs() error {
	if len(f.Buffs) == 0 {
		if f.RuleVersion >= 9 {
			return errors.New("v9状态子集缺少唯一状态定义")
		}
		// s 接收遍历技能集合时当前的元素。
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
		// field 接收遍历文本列表时当前的元素。
		for _, field := range []string{"enabled", "slot", "item_ids"} {
			if !f.CastingSupply.present[field] {
				return errors.New("v9耗材开关、专槽与白名单必须显式输出：" + field)
			}
		}
	}
	// supplies 建立带初始值的按数值索引的布尔状态映射，供本段继续处理。
	supplies := map[uint32]bool{}
	// id 接收遍历道具i“ds”时当前的元素。
	for _, id := range f.CastingSupply.ItemIDs {
		// item 接收道具供后续步骤使用，ok 表明查询、校验或类型转换是否成功。
		item, ok := f.Item(id)
		if !ok || supplies[id] || item.IsEquipment || item.EquipmentSlot != CastingSupplySlot || !item.Stackable || item.MaxStack < 0 || f.CastingSupply.Enabled && item.MaxStack == 0 {
			return errors.New("护符须为有界数量单位，未定单位只可停用，不得伪装成强化装备")
		}
		supplies[id] = true
	}
	// seen 建立带初始值的按数值索引的布尔状态映射，供本段继续处理。
	seen := map[uint32]bool{}
	// kinds 建立带初始值的按文本索引的布尔状态映射，供本段继续处理。
	kinds := map[string]bool{}
	// b 接收遍历“buffs”时当前的元素。
	for _, b := range f.Buffs {
		if f.RuleVersion >= 9 {
			// field 接收遍历文本列表时当前的元素。
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
		// value 接收遍历“append”时当前的元素。
		for _, value := range append(append([]int64{b.TargetLevelDivisor, b.TargetLevelAdd, b.StatAdd, b.StatMultiplier, b.HitDurationLossMSPerHP, b.DurationMS}, b.ValueByLevel...), b.DurationSecondsByLevel...) {
			if value < 0 || value > math.MaxInt32 {
				return errors.New("状态数值必须为有界非负整数")
			}
		}
		switch b.Kind {
		case "revelation":
			if b.DurationKind != "sc_source_power" || b.StatMultiplier != 0 || b.StatAdd != 0 || b.StackPolicy != "ignore_present_on_impact" || !b.RemoveOnPlayerDeath || b.AllowDeadTargets || b.TargetLevelDivisor != 0 || b.TargetLevelAdd != 0 || b.HitDurationLossMSPerHP != 0 {
				return errors.New("心灵启示须在原怪物生命上按道术计时，且重施不能刷新")
			}
			for rank, seconds := range b.DurationSecondsByLevel {
				if seconds != 0 || rank >= len(b.ValueByLevel) || b.ValueByLevel[rank] != 0 {
					return errors.New("心灵启示只显示怪物生命，不附加战斗属性")
				}
			}
		case "hiding", "mass_hiding":
			// 按技能区分单人隐身和集体隐身的道术换算与每阶固定秒数。
			durationKind, secondsPerRank := "sc_rank_linear", int64(5)
			if b.Kind == "mass_hiding" {
				durationKind, secondsPerRank = "sc_half_rank_linear", 2
			}
			if b.DurationKind != durationKind || b.StatMultiplier != 1 || b.StatAdd != 0 || b.StackPolicy != "refresh_duration_keep_values" || !b.RemoveOnPlayerDeath || b.AllowDeadTargets || b.TargetLevelDivisor != 0 || b.TargetLevelAdd != 0 || b.HitDurationLossMSPerHP != 0 {
				return errors.New("隐身状态必须具备原道术时长、原生命和重施规则")
			}
			for rank, seconds := range b.DurationSecondsByLevel {
				if seconds != int64(rank+1)*secondsPerRank || rank >= len(b.ValueByLevel) || b.ValueByLevel[rank] != 0 {
					return errors.New("隐身须使用对应技能的每阶持续基数")
				}
			}
		case "teleport_mana_penalty":
			if f.RuleVersion < 15 || b.DurationKind != "fixed_ms" || b.DurationMS <= 0 || !b.present["duration_ms"] || b.StackPolicy != "refresh_duration_keep_values" || b.RemoveOnPlayerDeath || !b.AllowDeadTargets || len(b.DurationSecondsByLevel) != 0 || b.TargetLevelDivisor != 0 || b.TargetLevelAdd != 0 || b.StatAdd != 0 || b.StatMultiplier != 0 || b.HitDurationLossMSPerHP != 0 {
				return errors.New("传送耗蓝惩罚须明确单一固定时长、尸体及重施保留规则")
			}
			// value 接收遍历值“by”等级时当前的元素。
			for _, value := range b.ValueByLevel {
				if value > 100 {
					return errors.New("传送耗蓝惩罚百分比越界")
				}
			}
		case "magic_shield":
			if b.DurationKind != "mc_source_power" || b.StackPolicy != "ignore_present_on_impact" || !b.RemoveOnPlayerDeath || b.AllowDeadTargets || b.HitDurationLossMSPerHP <= 0 || b.TargetLevelDivisor != 0 || b.TargetLevelAdd != 0 || b.StatMultiplier != 0 {
				return errors.New("魔法盾的生效、重施、死亡与削时管线不完整")
			}
			// percent 接收遍历值“by”等级时当前的元素。
			for _, percent := range b.ValueByLevel {
				if percent > 100 {
					return errors.New("状态减伤百分比越界")
				}
			}
			// seconds 接收遍历持续时间秒集合“by”等级时当前的元素。
			for _, seconds := range b.DurationSecondsByLevel {
				if seconds != 0 {
					return errors.New("魔法盾不可携带未参与公式的固定秒数")
				}
			}
		case "max_physical_defence", "max_magic_defence":
			if b.DurationKind != "sc_rank_linear" || b.StackPolicy != "refresh_duration_keep_values" || b.RemoveOnPlayerDeath || !b.AllowDeadTargets || b.TargetLevelDivisor <= 0 || b.HitDurationLossMSPerHP != 0 || b.StatAdd != 0 {
				return errors.New("道士防御状态必须保留初值、尸体与持续时钟规则")
			}
			// value 接收遍历值“by”等级时当前的元素。
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
	// used 建立带初始值的按数值索引的布尔状态映射，供本段继续处理。
	used := map[uint32]bool{}
	// s 接收遍历技能集合时当前的元素。
	for _, s := range f.Skills {
		if f.RuleVersion >= 9 {
			// field 接收遍历文本列表时当前的元素。
			for _, field := range []string{"buff_id", "material_item_id", "material_quantity"} {
				if !s.present[field] {
					return errors.New("v9技能状态与耗材字段必须显式输出：" + field)
				}
			}
		}
		if s.BuffID == 0 {
			if s.Behavior == "poisoning" {
				if err := validatePoisonMaterials(s, supplies); err != nil {
					return err
				}
				continue
			}
			if s.Behavior == "soul_fire_ball" || s.Behavior == TrapHexagonBehavior {
				if !supplies[s.MaterialItemID] || s.MaterialQuantity != 1 {
					return errors.New("灵魂火符与困魔咒必须消耗白名单中的一个原护符单位")
				}
				continue
			}
			if s.Behavior == "summon_pet" {
				// err 接收校验召唤技能过程中可能产生的错误，供后续失败分支处理。
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
		// b 保存状态效果返回的结果，ok 表明查询、校验或类型转换是否成功。
		b, ok := f.Buff(s.BuffID)
		if !ok || used[b.ID] || len(b.ValueByLevel) != int(s.MaxLevel)+1 || b.Kind != "teleport_mana_penalty" && len(b.DurationSecondsByLevel) != int(s.MaxLevel)+1 {
			return errors.New("技能状态引用或逐阶状态参数不完整")
		}
		used[b.ID] = true
		// wanted 保存从按文本索引的文本映射取出的期望状态。
		wanted := map[string]string{"magic_shield": "magic_shield", "soul_shield": "max_magic_defence", "blessed_armour": "max_physical_defence", "teleport": "teleport_mana_penalty", "hiding": "hiding", "mass_hiding": "mass_hiding", "revelation": "revelation"}[s.Behavior]
		// withoutMaterial 保存根据类型与当前数据的计算结果与类型与当前数据的计算结果计算的“without”材料。
		withoutMaterial := b.Kind == "magic_shield" || b.Kind == "teleport_mana_penalty" || b.Kind == "revelation"
		if b.Kind != wanted || withoutMaterial && (s.MaterialItemID != 0 || s.MaterialQuantity != 0) || !withoutMaterial && (!supplies[s.MaterialItemID] || s.MaterialQuantity <= 0) {
			return errors.New("状态技能行为与永久耗材引用不一致")
		}
		if (s.Behavior == "hiding" || s.Behavior == "mass_hiding") && s.MaterialQuantity != 1 {
			return errors.New("隐身术须消费原护符一个单位")
		}
	}
	if len(used) != len(f.Buffs) {
		return errors.New("状态定义不能包含未开放技能的隐形效果")
	}
	return nil
}
