package devconfig

import (
	"bytes"
	"encoding/json"
	"errors"
)

// 数值只由唯一Excel导出；这些字段明确已选择的来源随机规则，不能缺省成当前图随机空格。
type SkillTeleportRules struct {
	MaxAttempts         int    `json:"max_attempts"`
	AxisRule            string `json:"axis_rule"`
	LandingRule         string `json:"landing_rule"`
	AllowOccupied       bool   `json:"allow_occupied"`
	AllowAcceptedDead   bool   `json:"allow_accepted_dead"`
	CheckSourceMapAtDue bool   `json:"check_source_map_at_due"`
}

func (r *SkillTeleportRules) UnmarshalJSON(data []byte) error {
	var raw struct {
		MaxAttempts         *int    `json:"max_attempts"`
		AxisRule            *string `json:"axis_rule"`
		LandingRule         *string `json:"landing_rule"`
		AllowOccupied       *bool   `json:"allow_occupied"`
		AllowAcceptedDead   *bool   `json:"allow_accepted_dead"`
		CheckSourceMapAtDue *bool   `json:"check_source_map_at_due"`
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&raw); err != nil {
		return err
	}
	if raw.MaxAttempts == nil || raw.AxisRule == nil || raw.LandingRule == nil || raw.AllowOccupied == nil || raw.AllowAcceptedDead == nil || raw.CheckSourceMapAtDue == nil {
		return errors.New("瞬移规则必须完整显式提供，不能缺失或为null")
	}
	*r = SkillTeleportRules{*raw.MaxAttempts, *raw.AxisRule, *raw.LandingRule, *raw.AllowOccupied, *raw.AllowAcceptedDead, *raw.CheckSourceMapAtDue}
	return nil
}

func (f *Fixture) ValidateTeleport() error {
	all := append([]MapTemplate{f.Map}, f.Maps...)
	if f.RuleVersion < 15 {
		for _, m := range all {
			if m.NoTeleport != nil || len(m.SourceStaticBits) != 0 {
				return errors.New("原图静态瞬移及禁传合同须使用rule15及后续版本")
			}
		}
		for _, s := range f.Skills {
			if s.Teleport != nil || s.Behavior == "teleport" {
				return errors.New("瞬息移动须使用rule15及后续版本")
			}
		}
		return nil
	}
	for _, m := range all {
		if m.NoTeleport == nil {
			return errors.New("所有运行地图须显式提供独立NoTeleport规则")
		}
		if m.SceneKindValue() == SceneKindTown {
			if m.Source == nil || len(m.SourceStaticBits) != collisionByteCount(m.CellBudget()) || len(m.SourceStaticBits) == 0 || len(m.CollisionBits) != len(m.SourceStaticBits) {
				return errors.New("来源城镇须明确原图静态位图，不能由闭门位图替代")
			}
			for i, b := range m.SourceStaticBits {
				if b & ^m.CollisionBits[i] != 0 {
					return errors.New("原图静态阻挡不能超出闭门运行阻挡")
				}
			}
			cells := m.CellBudget()
			if cells%8 != 0 && m.SourceStaticBits[len(m.SourceStaticBits)-1]>>uint(cells%8) != 0 {
				return errors.New("原图静态位图尾部补位必须为0")
			}
		} else if len(m.SourceStaticBits) != 0 {
			return errors.New("本作个人图只使用唯一作者碰撞，不携带另一份静态位图")
		}
	}
	for _, s := range f.Skills {
		if s.Behavior != "teleport" {
			if s.Teleport != nil {
				return errors.New("非瞬移技能不能附带瞬移规则")
			}
			continue
		}
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
