// 本文件定义个人和共享仓库的能力门槛、容量、资产范围与来源规则。

package devconfig

import (
	"bytes"
	"encoding/json"
	"errors"
)

// 仓库总容量与分页窗口分别配置；窗口不是个人仓的最大容量。
type WarehouseRules struct {
	// PersonalInitialSlots 记录个人初始值槽位集合，使用数值表示。
	PersonalInitialSlots uint64 `json:"personal_initial_slots"`
	// PersonalExpansionSlots 记录个人“expansion”槽位集合，使用数值表示。
	PersonalExpansionSlots uint64 `json:"personal_expansion_slots"`
	// PersonalExpansionYuanbaoUnits 记录个人“expansion”元宝单位集合，使用数值表示。
	PersonalExpansionYuanbaoUnits int64 `json:"personal_expansion_yuanbao_units"`
	// SharedSlots 记录共享槽位集合，使用数值表示。
	SharedSlots uint64 `json:"shared_slots"`
	// QueryPageLimit 记录查询页面或分页限额，使用数值表示。
	QueryPageLimit uint32 `json:"query_page_limit"`
}

// UnmarshalJSON 执行反序列化JSON，并把失败交给调用方处理。
// r 是调用该方法的仓库规则集合实例，data 携带供当前流程读取或解析的原始字节。
func (r *WarehouseRules) UnmarshalJSON(data []byte) error {
	// fields 描述字段集合。
	type fields WarehouseRules
	// raw 记录原始值，使用按文本索引的原始值消息映射表示。
	var raw map[string]json.RawMessage
	// err 接收反序列化过程中可能产生的错误，供后续失败分支处理。
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	// key 接收遍历文本列表时当前的元素。
	for _, key := range []string{"personal_initial_slots", "personal_expansion_slots", "personal_expansion_yuanbao_units", "shared_slots", "query_page_limit"} {
		// v 保存从原始值取出的原始值中的指定项，ok 表明映射中能否取得指定键。
		v, ok := raw[key]
		if !ok || bytes.Equal(bytes.TrimSpace(v), []byte("null")) {
			return errors.New("仓储合同缺少显式字段：" + key)
		}
	}
	// out 记录输出，使用字段集合表示。
	var out fields
	// d 接收创建“decoder”供后续步骤使用。
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	// err 接收解码过程中可能产生的错误，供后续失败分支处理。
	if err := d.Decode(&out); err != nil {
		return err
	}
	*r = WarehouseRules(out)
	return nil
}

// ValidateWarehouses 校验仓库集合。
// f 是调用该方法的开发测试配置实例。
func (f *Fixture) ValidateWarehouses() error {
	// w 保存当前处理所用的仓库集合。
	w := f.Warehouses
	if f.RuleVersion < 23 {
		if w != nil {
			return errors.New("旧规则不能混入双仓储合同")
		}
		return nil
	}
	// excelExactInteger 保存根据当前数据与当前数据的计算结果与当前数据计算的“excel”精确整数。
	const excelExactInteger = 1<<53 - 1
	if w == nil || w.PersonalInitialSlots == 0 || w.PersonalExpansionSlots == 0 || w.PersonalInitialSlots > excelExactInteger || w.PersonalExpansionSlots > excelExactInteger || w.PersonalInitialSlots%w.PersonalExpansionSlots != 0 || w.PersonalExpansionYuanbaoUnits <= 0 || w.PersonalExpansionYuanbaoUnits > excelExactInteger || w.SharedSlots == 0 || w.SharedSlots > excelExactInteger || w.QueryPageLimit == 0 || uint64(w.QueryPageLimit) > max(w.PersonalExpansionSlots, w.SharedSlots) {
		return errors.New("仓储容量、费用或有界查询窗口非法")
	}
	return nil
}
