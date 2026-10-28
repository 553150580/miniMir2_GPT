package devconfig

import (
	"encoding/json"
	"errors"
)

// 位移距离、概率与显示节拍只由唯一Excel投影；旧技能不带这组参数。
type SkillDisplacement struct {
	DistanceBaseByLevel []int `json:"distance_base_by_level"`
	DistanceRandomBound int64 `json:"distance_random_bound"`
	SuccessDenominator  int64 `json:"success_denominator"`
	SuccessBase         int64 `json:"success_base"`
	SuccessPerLevel     int64 `json:"success_per_level"`
	StepDisplayMS       int64 `json:"step_display_ms"`
}

func (d *SkillDisplacement) UnmarshalJSON(data []byte) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	names := []string{"distance_base_by_level", "distance_random_bound", "success_denominator", "success_base", "success_per_level", "step_display_ms"}
	if len(fields) != len(names) {
		return errors.New("位移参数字段缺失或存在未知字段")
	}
	for _, name := range names {
		if raw, ok := fields[name]; !ok || string(raw) == "null" {
			return errors.New("位移参数必须显式提供，不能省略或null")
		}
	}
	type plain SkillDisplacement
	return json.Unmarshal(data, (*plain)(d))
}

func (f *Fixture) validateDisplacementSkill(s Skill) error {
	displacement := s.Behavior == "repulsion" || s.Behavior == "shoulder_dash"
	if !displacement {
		if s.Displacement != nil {
			return errors.New("非位移技能不得附加隐藏位移参数")
		}
		return nil
	}
	d := s.Displacement
	if f.RuleVersion < 11 {
		return errors.New("位移技能必须使用已登记的rule11及后续协议/配置合同")
	}
	if d == nil || len(d.DistanceBaseByLevel) != int(s.MaxLevel)+1 || d.DistanceRandomBound < 1 || d.DistanceRandomBound > int64(f.Map.Width) || d.StepDisplayMS < 1 || d.StepDisplayMS > 30000 {
		return errors.New("位移距离数组、随机边界或显示时长缺失")
	}
	for _, base := range d.DistanceBaseByLevel {
		if base < 1 || base+int(d.DistanceRandomBound)-1 > f.Map.Width {
			return errors.New("位移路径必须有界且不超过配置地图宽度")
		}
	}
	if s.AutoEnabled || s.DelayBaseMS != 0 || s.ProjectileMSPerCell != 0 || s.MaterialQuantity != 0 || s.RangeCells != 0 || s.MaxLevel != 3 {
		return errors.New("当前位移技能仅手动即时受理、原生0至3阶，不附加永久材料")
	}
	if s.Behavior == "repulsion" {
		if s.TargetKind != "enemy_self" || s.AreaRadius != 1 || d.SuccessDenominator < 1 || d.SuccessDenominator > 1000000 || d.SuccessBase < 0 || d.SuccessPerLevel < 0 || d.SuccessBase+d.SuccessPerLevel*int64(s.MaxLevel) > 1000000 {
			return errors.New("抗拒扫描、等级概率合同缺失")
		}
	} else if s.TargetKind != "enemy_direction" || s.AreaRadius != 0 || d.SuccessDenominator != 0 || d.SuccessBase != 0 || d.SuccessPerLevel != 0 {
		return errors.New("野蛮使用首格目标与固定方向，不能附加抗拒概率")
	}
	return nil
}
