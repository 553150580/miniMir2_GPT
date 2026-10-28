package devconfig

import (
	"bytes"
	"encoding/json"
	"errors"
	"math"
)

// 初始绑定必须来自唯一配置；不存在时保持旧版本关闭，不能推定为玩家出生点。
type BindingRules struct {
	Enabled     bool           `json:"enabled"`
	StartPoints []BindingPoint `json:"start_points"`
}

type BindingPoint struct {
	MapID uint64 `json:"map_id"`
	X     int32  `json:"x"`
	Y     int32  `json:"y"`
}

func (p *BindingPoint) UnmarshalJSON(data []byte) error {
	var raw struct {
		MapID *uint64 `json:"map_id"`
		X     *int32  `json:"x"`
		Y     *int32  `json:"y"`
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&raw); err != nil {
		return err
	}
	if raw.MapID == nil || raw.X == nil || raw.Y == nil {
		return errors.New("绑定地图和坐标必须显式提供，不能以缺失代替坐标0")
	}
	*p = BindingPoint{MapID: *raw.MapID, X: *raw.X, Y: *raw.Y}
	return nil
}

func (r *BindingRules) UnmarshalJSON(data []byte) error {
	var raw struct {
		Enabled     *bool           `json:"enabled"`
		StartPoints *[]BindingPoint `json:"start_points"`
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&raw); err != nil {
		return err
	}
	if raw.Enabled == nil || raw.StartPoints == nil {
		return errors.New("绑定开关及起点数组必须显式提供")
	}
	*r = BindingRules{Enabled: *raw.Enabled, StartPoints: *raw.StartPoints}
	return nil
}

// 绑定只核对已配置目录中的静态可走格；不能据此改写已有角色的永久绑定。
func (f *Fixture) ValidBinding(point BindingPoint) bool {
	if point.MapID == 0 || point.MapID > math.MaxInt64 {
		return false
	}
	m, ok := f.MapByID(point.MapID)
	return ok && m.Walkable(Point{X: int(point.X), Y: int(point.Y)})
}

func (f *Fixture) ValidateBinding() error {
	if f.Binding == nil {
		if f.RuleVersion >= 12 {
			return errors.New("rule12起须显式提供绑定规则，待审定时关闭并保留空起点")
		}
		return nil
	}
	if f.RuleVersion < 12 {
		return errors.New("角色绑定配置须使用rule12及后续版本")
	}
	if !f.Binding.Enabled {
		if len(f.Binding.StartPoints) != 0 {
			return errors.New("绑定入口关闭时不得携带隐含启用的起点")
		}
		return nil
	}
	if len(f.Binding.StartPoints) == 0 {
		return errors.New("启用绑定必须提供已审定的起点")
	}
	for _, point := range f.Binding.StartPoints {
		if !f.ValidBinding(point) {
			return errors.New("绑定起点须属于已配置地图的可走地格")
		}
	}
	return nil
}
