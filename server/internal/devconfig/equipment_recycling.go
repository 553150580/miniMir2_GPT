// 本文件装配版本化自动回收及出售规则，并限制合法价格来源和支付币种。

package devconfig

import (
	"bytes"
	"encoding/json"
	"errors"
)

// 仅显式声明开发金币子集，正式分解表空值不能被当成零副产物规则。
type EquipmentRecyclingRules struct {
	// Mode 记录模式，使用文本表示。
	Mode string `json:"mode"`
	// Outputs 记录“outputs”，使用原始值消息列表表示。
	Outputs []json.RawMessage `json:"outputs"`
}

// 金币0和普通品质0都是有效值，必须与来源缺失/null严格区分。
// i 是调用该方法的道具实例，data 携带供当前流程读取或解析的原始字节。
func (i *Item) UnmarshalJSON(data []byte) error {
	// itemFields 描述道具字段集合。
	type itemFields Item
	// decoded 记录“decoded”，使用道具字段集合表示。
	var decoded itemFields
	// err 接收反序列化过程中可能产生的错误，供后续失败分支处理。
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	// fields 记录字段集合，使用按文本索引的原始值消息映射表示。
	var fields map[string]json.RawMessage
	// err 接收反序列化过程中可能产生的错误，供后续失败分支处理。
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	// gold 保存从字段集合取出的金币，hasGold 保存从字段集合取出的“has”金币。
	gold, hasGold := fields["recycle_gold"]
	// quality 保存从字段集合取出的品质，hasQuality 保存从字段集合取出的“has”品质。
	quality, hasQuality := fields["drop_quality"]
	decoded.recycleGoldExplicit = hasGold && string(gold) != "null"
	decoded.dropQualityExplicit = hasQuality && string(quality) != "null"
	// n 定位装备属性字段集合中正在处理的成员，name 接收遍历装备属性字段集合时当前的元素。
	for n, name := range equipmentStatFields {
		// value 保存从字段集合取出的值，exists 保存从字段集合取出的存在。
		value, exists := fields[name]
		if exists && bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return errors.New("装备基础属性不能为null：" + name)
		}
		decoded.equipmentStatsExplicit[n] = exists
	}
	*i = Item(decoded)
	return nil
}

// EquipmentRecycleKnown 判断装备回收已知状态是否满足要求。
// i 是调用该方法的道具实例。
func (i Item) EquipmentRecycleKnown() bool {
	return i.IsEquipment && i.RequiredLevel != nil && *i.RequiredLevel >= 1 && i.recycleGoldExplicit && i.dropQualityExplicit && i.RecycleGold >= 0 && i.DropQuality <= 4
}

// ValidateEquipmentRecycling 校验装备自动回收。
// f 是调用该方法的开发测试配置实例。
func (f *Fixture) ValidateEquipmentRecycling() error {
	// rules 保存装备自动回收，用于当前步骤中的规则集合。
	rules := f.EquipmentRecycling
	if f.RuleVersion < 19 {
		if rules != nil {
			return errors.New("旧规则版本不能混入装备自动回收合同")
		}
		return nil
	}
	if rules == nil || rules.Mode != "development_gold_only" || rules.Outputs == nil || len(rules.Outputs) != 0 {
		return errors.New("装备回收须显式声明development_gold_only及空outputs，不猜正式石油产物")
	}
	// item 接收遍历道具集合时当前的元素。
	for _, item := range f.Items {
		if item.IsEquipment && !item.EquipmentRecycleKnown() {
			return errors.New("开发回收装备须有来源需求等级及非负明确金币")
		}
	}
	return nil
}
