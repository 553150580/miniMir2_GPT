package devconfig

import (
	"bytes"
	"encoding/json"
	"errors"
)

// 仅显式声明开发金币子集，正式分解表空值不能被当成零副产物规则。
type EquipmentRecyclingRules struct {
	Mode    string            `json:"mode"`
	Outputs []json.RawMessage `json:"outputs"`
}

// 金币0和普通品质0都是有效值，必须与来源缺失/null严格区分。
func (i *Item) UnmarshalJSON(data []byte) error {
	type itemFields Item
	var decoded itemFields
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	gold, hasGold := fields["recycle_gold"]
	quality, hasQuality := fields["drop_quality"]
	decoded.recycleGoldExplicit = hasGold && string(gold) != "null"
	decoded.dropQualityExplicit = hasQuality && string(quality) != "null"
	for n, name := range equipmentStatFields {
		value, exists := fields[name]
		if exists && bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return errors.New("装备基础属性不能为null：" + name)
		}
		decoded.equipmentStatsExplicit[n] = exists
	}
	*i = Item(decoded)
	return nil
}

func (i Item) EquipmentRecycleKnown() bool {
	return i.IsEquipment && i.RequiredLevel != nil && *i.RequiredLevel >= 1 && i.recycleGoldExplicit && i.dropQualityExplicit && i.RecycleGold >= 0 && i.DropQuality <= 4
}

func (f *Fixture) ValidateEquipmentRecycling() error {
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
	for _, item := range f.Items {
		if item.IsEquipment && !item.EquipmentRecycleKnown() {
			return errors.New("开发回收装备须有来源需求等级及非负明确金币")
		}
	}
	return nil
}
