// 本文件装配抗拒、野蛮、肩撞等候选位移技能的动作、格数和启用边界。

package devconfig

import (
	"encoding/json"
	"errors"
)

// 位移距离、概率与显示节拍只由唯一Excel投影；旧技能不带这组参数。
type SkillDisplacement struct {
	// DistanceBaseByLevel 记录距离基础值“by”等级，使用数值列表表示。
	DistanceBaseByLevel []int `json:"distance_base_by_level"`
	// DistanceRandomBound 记录距离随机值绑定，使用数值表示。
	DistanceRandomBound int64 `json:"distance_random_bound"`
	// SuccessDenominator 记录成功“denominator”，使用数值表示。
	SuccessDenominator int64 `json:"success_denominator"`
	// SuccessBase 记录成功基础值，使用数值表示。
	SuccessBase int64 `json:"success_base"`
	// SuccessPerLevel 记录成功“per”等级，使用数值表示。
	SuccessPerLevel int64 `json:"success_per_level"`
	// StepDisplayMS 记录步骤显示毫秒，使用数值表示。
	StepDisplayMS int64 `json:"step_display_ms"`
}

// UnmarshalJSON 执行反序列化JSON，并把失败交给调用方处理。
// d 是调用该方法的技能位移实例，data 携带供当前流程读取或解析的原始字节。
func (d *SkillDisplacement) UnmarshalJSON(data []byte) error {
	// fields 记录字段集合，使用按文本索引的原始值消息映射表示。
	var fields map[string]json.RawMessage
	// err 接收反序列化过程中可能产生的错误，供后续失败分支处理。
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	// names 初始化用作名称集合的文本列表。
	names := []string{"distance_base_by_level", "distance_random_bound", "success_denominator", "success_base", "success_per_level", "step_display_ms"}
	if len(fields) != len(names) {
		return errors.New("位移参数字段缺失或存在未知字段")
	}
	// name 接收遍历名称集合时当前的元素。
	for _, name := range names {
		// raw 保存从字段集合取出的原始值，ok 表明映射中能否取得指定键。
		if raw, ok := fields[name]; !ok || string(raw) == "null" {
			return errors.New("位移参数必须显式提供，不能省略或null")
		}
	}
	// plain 描述“plain”。
	type plain SkillDisplacement
	return json.Unmarshal(data, (*plain)(d))
}

// validateDisplacementSkill 校验位移技能。
// f 是调用该方法的开发测试配置实例，s 记录技能。
func (f *Fixture) validateDisplacementSkill(s Skill) error {
	// displacement 保存根据行为与当前数据的计算结果与行为与当前数据的计算结果计算的位移。
	displacement := s.Behavior == "repulsion" || s.Behavior == "shoulder_dash"
	if !displacement {
		if s.Displacement != nil {
			return errors.New("非位移技能不得附加隐藏位移参数")
		}
		return nil
	}
	// d 保存当前处理所用的位移。
	d := s.Displacement
	if f.RuleVersion < 11 {
		return errors.New("位移技能必须使用已登记的rule11及后续协议/配置合同")
	}
	if d == nil || len(d.DistanceBaseByLevel) != int(s.MaxLevel)+1 || d.DistanceRandomBound < 1 || d.DistanceRandomBound > int64(f.Map.Width) || d.StepDisplayMS < 1 || d.StepDisplayMS > 30000 {
		return errors.New("位移距离数组、随机边界或显示时长缺失")
	}
	// base 接收遍历距离基础值“by”等级时当前的元素。
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
