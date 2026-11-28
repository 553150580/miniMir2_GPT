// 本文件解析等级、经验池、成长版本和旧档映射的显式规则。

package devconfig

import (
	"bytes"
	"encoding/json"
	"errors"
	"regexp"
)

// ProgressionRules 保存成长规则集合所需的状态。
type ProgressionRules struct {
	// ExpPerLevel 记录经验“per”等级，使用数值列表表示。
	ExpPerLevel []int64 `json:"exp_per_level"`
	// CurveVersion 记录“curve”版本，使用文本表示。
	CurveVersion string `json:"curve_version,omitempty"`
	// InitialLevelCap 记录初始值等级上限，使用数值表示。
	InitialLevelCap uint32 `json:"initial_level_cap,omitempty"`
	// RebirthLevelCapIncrement 记录“rebirth”等级上限增量，使用数值表示。
	RebirthLevelCapIncrement uint32 `json:"rebirth_level_cap_increment,omitempty"`
	// RebirthResetProgression 标记是否处于“rebirth”重置成长状态。
	RebirthResetProgression bool `json:"rebirth_reset_progression"`
	// OperatingLevelCap 记录“operating”等级上限，使用数值表示。
	OperatingLevelCap uint32 `json:"operating_level_cap,omitempty"`
	// CatalogMaxLevel 记录目录最大等级，使用数值表示。
	CatalogMaxLevel uint32 `json:"catalog_max_level,omitempty"`
	// PendingPoolPolicy 记录待确认连接或数值池策略，使用文本表示。
	PendingPoolPolicy string `json:"pending_pool_policy,omitempty"`
	// NewOverflowPolicy 记录创建溢出策略，使用文本表示。
	NewOverflowPolicy string `json:"new_overflow_policy,omitempty"`
	// LegacyOverflowPolicy 记录兼容旧数据溢出策略，使用文本表示。
	LegacyOverflowPolicy string `json:"legacy_overflow_policy,omitempty"`
	// LevelUpResources 记录等级“up”“resources”，使用文本表示。
	LevelUpResources string `json:"level_up_resources,omitempty"`
	// RebirthEnabled 标记是否处于“rebirth”已开启状态。
	RebirthEnabled bool `json:"rebirth_enabled"`
	// present 记录“present”，使用按文本索引的布尔状态映射表示。
	present map[string]bool
}

// progressionKeys 初始化用作成长键集合的文本列表。
var progressionKeys = []string{"exp_per_level", "curve_version", "initial_level_cap", "rebirth_level_cap_increment", "rebirth_reset_progression", "operating_level_cap", "catalog_max_level", "pending_pool_policy", "new_overflow_policy", "legacy_overflow_policy", "level_up_resources", "rebirth_enabled"}

// UnmarshalJSON 执行反序列化JSON，并把失败交给调用方处理。
// p 是调用该方法的成长规则集合实例，data 携带供当前流程读取或解析的原始字节。
func (p *ProgressionRules) UnmarshalJSON(data []byte) error {
	// fields 描述字段集合。
	type fields ProgressionRules
	// decoded 记录“decoded”，使用字段集合表示。
	var decoded fields
	// decoder 保存由创建“decoder”取得的“decoder”。
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	// err 接收解码过程中可能产生的错误，供后续失败分支处理。
	if err := decoder.Decode(&decoded); err != nil {
		return err
	}
	// raw 记录原始值，使用按文本索引的原始值消息映射表示。
	var raw map[string]json.RawMessage
	// err 接收反序列化过程中可能产生的错误，供后续失败分支处理。
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	decoded.present = map[string]bool{}
	// name 定位原始值中正在处理的成员，value 接收遍历原始值时当前的元素。
	for name, value := range raw {
		decoded.present[name] = !bytes.Equal(bytes.TrimSpace(value), []byte("null"))
	}
	*p = ProgressionRules(decoded)
	return nil
}

// 编码presence不是玩法默认值；false与未配置/null必须区别核验。
// f 是调用该方法的开发测试配置实例。
func (f *Fixture) ValidateProgression() error {
	// p 保存当前处理所用的成长。
	p := f.Progression
	// amount 接收遍历经验“per”等级时当前的元素。
	for _, amount := range p.ExpPerLevel {
		if amount <= 0 {
			return errors.New("逐级经验必须是连续目录中的正整数")
		}
	}
	if f.RuleVersion < 20 {
		if p.CurveVersion != "" || p.InitialLevelCap != 0 || p.OperatingLevelCap != 0 || p.CatalogMaxLevel != 0 || p.PendingPoolPolicy != "" || p.NewOverflowPolicy != "" || p.LegacyOverflowPolicy != "" || p.LevelUpResources != "" || p.RebirthLevelCapIncrement != 0 || p.RebirthEnabled || p.RebirthResetProgression || f.Rebirth != nil {
			return errors.New("旧规则版本不能混入新成长合同")
		}
		return nil
	}
	// name 接收遍历成长键集合时当前的元素。
	for _, name := range progressionKeys {
		if !p.present[name] {
			return errors.New("成长合同缺少显式字段：" + name)
		}
	}
	var stages uint64 // 每档显式放行一级，经验表长度不能单独开放新等级。
	if f.Rebirth != nil {
		stages = uint64(len(f.Rebirth.Stages))
	}
	if !regexp.MustCompile(`^[a-zA-Z0-9_.-]{1,96}$`).MatchString(p.CurveVersion) || p.InitialLevelCap != 50 ||
		p.RebirthLevelCapIncrement != 1 || p.RebirthResetProgression || (p.RebirthEnabled && stages == 0) ||
		uint64(p.OperatingLevelCap) != uint64(p.InitialLevelCap)+stages ||
		p.CatalogMaxLevel != p.OperatingLevelCap || len(p.ExpPerLevel) != int(p.CatalogMaxLevel) {
		return errors.New("成长须从50级起按显式转生档位开放连续等级")
	}
	if p.PendingPoolPolicy != "one_next_level" || p.NewOverflowPolicy != "reject_new_with_receipt" || p.LegacyOverflowPolicy != "preserve_frozen" || p.LevelUpResources != "source_full_if_alive_original_birth" {
		return errors.New("待用池、旧余额和升级恢复必须采用明确受控合同")
	}
	return nil
}
