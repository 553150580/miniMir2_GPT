// 本文件装配瞬移来源地形、候选落点、方向、持续状态与失败策略。

package devconfig

import (
	"bytes"
	"encoding/json"
	"errors"
)

// 数值只由唯一Excel导出；这些字段明确已选择的来源随机规则，不能缺省成当前图随机空格。
type SkillTeleportRules struct {
	// MaxAttempts 记录最大“attempts”，使用数值表示。
	MaxAttempts int `json:"max_attempts"`
	// AxisRule 记录“axis”规则，使用文本表示。
	AxisRule string `json:"axis_rule"`
	// LandingRule 记录落点规则，使用文本表示。
	LandingRule string `json:"landing_rule"`
	// AllowOccupied 标记是否处于“allow”已占用状态。
	AllowOccupied bool `json:"allow_occupied"`
	// AllowAcceptedDead 标记是否处于“allow”已受理死亡状态状态。
	AllowAcceptedDead bool `json:"allow_accepted_dead"`
	// CheckSourceMapAtDue 标记是否处于“check”来源地图发生时间已到期状态。
	CheckSourceMapAtDue bool `json:"check_source_map_at_due"`
}

// UnmarshalJSON 执行反序列化JSON，并把失败交给调用方处理。
// r 是调用该方法的技能瞬移规则集合实例，data 携带供当前流程读取或解析的原始字节。
func (r *SkillTeleportRules) UnmarshalJSON(data []byte) error {
	// raw 记录原始值，使用临时结构表示。
	var raw struct {
		// MaxAttempts 记录最大“attempts”，使用数值表示。
		MaxAttempts *int `json:"max_attempts"`
		// AxisRule 记录“axis”规则，使用文本表示。
		AxisRule *string `json:"axis_rule"`
		// LandingRule 记录落点规则，使用文本表示。
		LandingRule *string `json:"landing_rule"`
		// AllowOccupied 标记是否处于“allow”已占用状态。
		AllowOccupied *bool `json:"allow_occupied"`
		// AllowAcceptedDead 标记是否处于“allow”已受理死亡状态状态。
		AllowAcceptedDead *bool `json:"allow_accepted_dead"`
		// CheckSourceMapAtDue 标记是否处于“check”来源地图发生时间已到期状态。
		CheckSourceMapAtDue *bool `json:"check_source_map_at_due"`
	}
	// decoder 保存由创建“decoder”取得的“decoder”。
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	// err 接收解码过程中可能产生的错误，供后续失败分支处理。
	if err := decoder.Decode(&raw); err != nil {
		return err
	}
	if raw.MaxAttempts == nil || raw.AxisRule == nil || raw.LandingRule == nil || raw.AllowOccupied == nil || raw.AllowAcceptedDead == nil || raw.CheckSourceMapAtDue == nil {
		return errors.New("瞬移规则必须完整显式提供，不能缺失或为null")
	}
	*r = SkillTeleportRules{*raw.MaxAttempts, *raw.AxisRule, *raw.LandingRule, *raw.AllowOccupied, *raw.AllowAcceptedDead, *raw.CheckSourceMapAtDue}
	return nil
}

// ValidateTeleport 校验瞬移。
// f 是调用该方法的开发测试配置实例。
func (f *Fixture) ValidateTeleport() error {
	// all 保留加入新条目后的全部。
	all := append([]MapTemplate{f.Map}, f.Maps...)
	if f.RuleVersion < 15 {
		// m 接收遍历全部时当前的元素。
		for _, m := range all {
			if m.NoTeleport != nil || len(m.SourceStaticBits) != 0 {
				return errors.New("原图静态瞬移及禁传合同须使用rule15及后续版本")
			}
		}
		// s 接收遍历技能集合时当前的元素。
		for _, s := range f.Skills {
			if s.Teleport != nil || s.Behavior == "teleport" {
				return errors.New("瞬息移动须使用rule15及后续版本")
			}
		}
		return nil
	}
	// m 接收遍历全部时当前的元素。
	for _, m := range all {
		if m.NoTeleport == nil {
			return errors.New("所有运行地图须显式提供独立NoTeleport规则")
		}
		if m.SceneKindValue() == SceneKindTown && !m.CompactTown {
			if m.Source == nil || len(m.SourceStaticBits) != collisionByteCount(m.CellBudget()) || len(m.SourceStaticBits) == 0 || len(m.CollisionBits) != len(m.SourceStaticBits) {
				return errors.New("来源城镇须明确原图静态位图，不能由闭门位图替代")
			}
			// i 定位来源“static”二进制位中正在处理的成员，b 接收遍历来源“static”二进制位时当前的元素。
			for i, b := range m.SourceStaticBits {
				if b & ^m.CollisionBits[i] != 0 {
					return errors.New("原图静态阻挡不能超出闭门运行阻挡")
				}
			}
			// cells 保存由地图格预算取得的地图格集合。
			cells := m.CellBudget()
			if cells%8 != 0 && m.SourceStaticBits[len(m.SourceStaticBits)-1]>>uint(cells%8) != 0 {
				return errors.New("原图静态位图尾部补位必须为0")
			}
		} else if len(m.SourceStaticBits) != 0 {
			return errors.New("作者布局只使用唯一阻挡格，不携带另一份静态位图")
		}
	}
	// s 接收遍历技能集合时当前的元素。
	for _, s := range f.Skills {
		if s.Behavior != "teleport" {
			if s.Teleport != nil {
				return errors.New("非瞬移技能不能附带瞬移规则")
			}
			continue
		}
		// r 保存当前处理所用的瞬移。
		r := s.Teleport
		if s.ID != 21 || s.JobID != 1 || s.LearnedLevel != 0 || s.MaxLevel != 3 || r == nil || r.MaxAttempts <= 0 || r.MaxAttempts > 4096 || r.AxisRule != "map_dimension_divided_by_rank_plus_one" || r.LandingRule != "source_static_walk" || !r.AllowOccupied || !r.AllowAcceptedDead || !r.CheckSourceMapAtDue {
			return errors.New("瞬息移动身份或来源随机/落点规则不完整")
		}
		if s.TargetKind != "friendly_self" || s.Activation() != "cast" || s.AutoEnabled || s.RangeCells != 0 || s.DelayBaseMS <= 0 || s.ProjectileMSPerCell != 0 || s.MaterialItemID != 0 || s.MaterialQuantity != 0 || s.Displacement != nil || s.BuffID == 0 {
			return errors.New("瞬息移动须为手动无材料自身延迟技能，不能套用立即位移")
		}
		if !f.SafeZoneBindingEnabled() {
			return errors.New("瞬息移动须先接入可信安全区绑定")
		}
	}
	return nil
}
