package devconfig

import (
	"bytes"
	"encoding/json"
	"errors"
)

// 仓库总容量与分页窗口分别配置；窗口不是个人仓的最大容量。
type WarehouseRules struct {
	PersonalInitialSlots          uint64 `json:"personal_initial_slots"`
	PersonalExpansionSlots        uint64 `json:"personal_expansion_slots"`
	PersonalExpansionYuanbaoUnits int64  `json:"personal_expansion_yuanbao_units"`
	SharedSlots                   uint64 `json:"shared_slots"`
	QueryPageLimit                uint32 `json:"query_page_limit"`
}

func (r *WarehouseRules) UnmarshalJSON(data []byte) error {
	type fields WarehouseRules
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	for _, key := range []string{"personal_initial_slots", "personal_expansion_slots", "personal_expansion_yuanbao_units", "shared_slots", "query_page_limit"} {
		v, ok := raw[key]
		if !ok || bytes.Equal(bytes.TrimSpace(v), []byte("null")) {
			return errors.New("仓储合同缺少显式字段：" + key)
		}
	}
	var out fields
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if err := d.Decode(&out); err != nil {
		return err
	}
	*r = WarehouseRules(out)
	return nil
}

func (f *Fixture) ValidateWarehouses() error {
	w := f.Warehouses
	if f.RuleVersion < 23 {
		if w != nil {
			return errors.New("旧规则不能混入双仓储合同")
		}
		return nil
	}
	const excelExactInteger = 1<<53 - 1
	if w == nil || w.PersonalInitialSlots == 0 || w.PersonalExpansionSlots == 0 || w.PersonalInitialSlots > excelExactInteger || w.PersonalExpansionSlots > excelExactInteger || w.PersonalInitialSlots%w.PersonalExpansionSlots != 0 || w.PersonalExpansionYuanbaoUnits <= 0 || w.PersonalExpansionYuanbaoUnits > excelExactInteger || w.SharedSlots == 0 || w.SharedSlots > excelExactInteger || w.QueryPageLimit == 0 || uint64(w.QueryPageLimit) > max(w.PersonalExpansionSlots, w.SharedSlots) {
		return errors.New("仓储容量、费用或有界查询窗口非法")
	}
	return nil
}
