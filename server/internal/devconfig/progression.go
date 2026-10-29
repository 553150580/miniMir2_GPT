package devconfig

import (
	"bytes"
	"encoding/json"
	"errors"
	"regexp"
)

type ProgressionRules struct {
	ExpPerLevel              []int64 `json:"exp_per_level"`
	CurveVersion             string  `json:"curve_version,omitempty"`
	InitialLevelCap          uint32  `json:"initial_level_cap,omitempty"`
	RebirthLevelCapIncrement uint32  `json:"rebirth_level_cap_increment,omitempty"`
	RebirthResetProgression  bool    `json:"rebirth_reset_progression"`
	OperatingLevelCap        uint32  `json:"operating_level_cap,omitempty"`
	CatalogMaxLevel          uint32  `json:"catalog_max_level,omitempty"`
	PendingPoolPolicy        string  `json:"pending_pool_policy,omitempty"`
	NewOverflowPolicy        string  `json:"new_overflow_policy,omitempty"`
	LegacyOverflowPolicy     string  `json:"legacy_overflow_policy,omitempty"`
	LevelUpResources         string  `json:"level_up_resources,omitempty"`
	RebirthEnabled           bool    `json:"rebirth_enabled"`
	present                  map[string]bool
}

var progressionKeys = []string{"exp_per_level", "curve_version", "initial_level_cap", "rebirth_level_cap_increment", "rebirth_reset_progression", "operating_level_cap", "catalog_max_level", "pending_pool_policy", "new_overflow_policy", "legacy_overflow_policy", "level_up_resources", "rebirth_enabled"}

func (p *ProgressionRules) UnmarshalJSON(data []byte) error {
	type fields ProgressionRules
	var decoded fields
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&decoded); err != nil {
		return err
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	decoded.present = map[string]bool{}
	for name, value := range raw {
		decoded.present[name] = !bytes.Equal(bytes.TrimSpace(value), []byte("null"))
	}
	*p = ProgressionRules(decoded)
	return nil
}

// 编码presence不是玩法默认值；false与未配置/null必须区别核验。
func (f *Fixture) ValidateProgression() error {
	p := f.Progression
	for _, amount := range p.ExpPerLevel {
		if amount <= 0 {
			return errors.New("逐级经验必须是连续目录中的正整数")
		}
	}
	if f.RuleVersion < 20 {
		if p.CurveVersion != "" || p.InitialLevelCap != 0 || p.OperatingLevelCap != 0 || p.CatalogMaxLevel != 0 || p.PendingPoolPolicy != "" || p.NewOverflowPolicy != "" || p.LegacyOverflowPolicy != "" || p.LevelUpResources != "" || p.RebirthLevelCapIncrement != 0 || p.RebirthEnabled || p.RebirthResetProgression {
			return errors.New("旧规则版本不能混入新成长合同")
		}
		return nil
	}
	for _, name := range progressionKeys {
		if !p.present[name] {
			return errors.New("成长合同缺少显式字段：" + name)
		}
	}
	if !regexp.MustCompile(`^[a-zA-Z0-9_.-]{1,96}$`).MatchString(p.CurveVersion) || p.InitialLevelCap != 50 || p.RebirthLevelCapIncrement != 1 || p.RebirthResetProgression || p.RebirthEnabled || p.OperatingLevelCap != p.InitialLevelCap || p.CatalogMaxLevel != p.OperatingLevelCap || len(p.ExpPerLevel) != int(p.CatalogMaxLevel) {
		return errors.New("开发成长须连续到已定50级上限且转生关闭，不能由表长隐含升到51")
	}
	if p.PendingPoolPolicy != "one_next_level" || p.NewOverflowPolicy != "reject_new_with_receipt" || p.LegacyOverflowPolicy != "preserve_frozen" || p.LevelUpResources != "source_full_if_alive_original_birth" {
		return errors.New("待用池、旧余额和升级恢复必须采用明确受控合同")
	}
	return nil
}
