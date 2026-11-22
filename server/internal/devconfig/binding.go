// 本文件加载安全区注册的显式来源坐标、旧版本兼容规则和入口启用条件。

package devconfig

import (
	"bytes"
	"encoding/json"
	"errors"
	"math"
)

// 初始绑定必须来自唯一配置；不存在时保持旧版本关闭，不能推定为玩家出生点。
type BindingRules struct {
	// Enabled 标记是否处于已开启状态。
	Enabled bool `json:"enabled"`
	// StartPoints 记录起点或开始时间点位集合，使用会话绑定点位列表表示。
	StartPoints []BindingPoint `json:"start_points"`
	// SafeZoneEnabled 标记是否处于安全区域已开启状态。
	SafeZoneEnabled *bool `json:"safe_zone_enabled,omitempty"`
}

// BindingPoint 保存会话绑定点位所需的状态。
type BindingPoint struct {
	// MapID 标识当前业务使用的地图。
	MapID uint64 `json:"map_id"`
	// X 记录横坐标，使用数值表示。
	X int32 `json:"x"`
	// Y 记录纵坐标，使用数值表示。
	Y int32 `json:"y"`
}

// UnmarshalJSON 执行反序列化JSON，并把失败交给调用方处理。
// p 是调用该方法的会话绑定点位实例，data 携带供当前流程读取或解析的原始字节。
func (p *BindingPoint) UnmarshalJSON(data []byte) error {
	// raw 记录原始值，使用临时结构表示。
	var raw struct {
		// MapID 标识当前业务使用的地图。
		MapID *uint64 `json:"map_id"`
		// X 记录横坐标，使用数值表示。
		X *int32 `json:"x"`
		// Y 记录纵坐标，使用数值表示。
		Y *int32 `json:"y"`
	}
	// decoder 保存由创建“decoder”取得的“decoder”。
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	// err 接收解码过程中可能产生的错误，供后续失败分支处理。
	if err := decoder.Decode(&raw); err != nil {
		return err
	}
	if raw.MapID == nil || raw.X == nil || raw.Y == nil {
		return errors.New("绑定地图和坐标必须显式提供，不能以缺失代替坐标0")
	}
	*p = BindingPoint{MapID: *raw.MapID, X: *raw.X, Y: *raw.Y}
	return nil
}

// UnmarshalJSON 执行反序列化JSON，并把失败交给调用方处理。
// r 是调用该方法的会话绑定规则集合实例，data 携带供当前流程读取或解析的原始字节。
func (r *BindingRules) UnmarshalJSON(data []byte) error {
	// raw 记录原始值，使用临时结构表示。
	var raw struct {
		// Enabled 标记是否处于已开启状态。
		Enabled *bool `json:"enabled"`
		// StartPoints 记录起点或开始时间点位集合，使用会话绑定点位列表表示。
		StartPoints *[]BindingPoint `json:"start_points"`
		// SafeZoneEnabled 记录安全区域已开启，使用原始值消息表示。
		SafeZoneEnabled json.RawMessage `json:"safe_zone_enabled"`
	}
	// decoder 保存由创建“decoder”取得的“decoder”。
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	// err 接收解码过程中可能产生的错误，供后续失败分支处理。
	if err := decoder.Decode(&raw); err != nil {
		return err
	}
	if raw.Enabled == nil || raw.StartPoints == nil {
		return errors.New("绑定开关及起点数组必须显式提供")
	}
	// safeZoneEnabled 标记是否处于安全区域已开启状态。
	var safeZoneEnabled *bool
	if raw.SafeZoneEnabled != nil {
		// value 标记是否处于值状态。
		var value bool
		if bytes.Equal(bytes.TrimSpace(raw.SafeZoneEnabled), []byte("null")) {
			return errors.New("安全区绑定开关必须为显式布尔值，不能为null")
		}
		// err 接收反序列化过程中可能产生的错误，供后续失败分支处理。
		if err := json.Unmarshal(raw.SafeZoneEnabled, &value); err != nil {
			return err
		}
		safeZoneEnabled = &value
	}
	*r = BindingRules{Enabled: *raw.Enabled, StartPoints: *raw.StartPoints, SafeZoneEnabled: safeZoneEnabled}
	return nil
}

// 初始随机池与到访安全区是独立门禁；旧配置不会隐式打开新业务。
// f 是调用该方法的开发测试配置实例。
func (f *Fixture) SafeZoneBindingEnabled() bool {
	return f.RuleVersion >= 14 && f.Binding != nil && f.Binding.SafeZoneEnabled != nil && *f.Binding.SafeZoneEnabled
}

// 绑定只核对已配置目录中的静态可走格；不能据此改写已有角色的永久绑定。
// f 是调用该方法的开发测试配置实例，point 记录点位，使用会话绑定点位表示。
func (f *Fixture) ValidBinding(point BindingPoint) bool {
	if point.MapID == 0 || point.MapID > math.MaxInt64 {
		return false
	}
	// m 接收地图“by”标识供后续步骤使用，ok 表明查询、校验或类型转换是否成功。
	m, ok := f.MapByID(point.MapID)
	return ok && m.Walkable(Point{X: int(point.X), Y: int(point.Y)})
}

// 保留存档与原回执的坐标；这里只解析已经核验的安全区到本次瞬移目标。
// f 是调用该方法的开发测试配置实例，point 记录点位，使用会话绑定点位表示，zoneID 标识当前业务使用的区域。
func (f *Fixture) TownTeleportBinding(point BindingPoint, zoneID uint64) (BindingPoint, bool) {
	// m 接收地图“by”标识供后续步骤使用，ok 表明查询、校验或类型转换是否成功。
	m, ok := f.MapByID(point.MapID)
	if !ok || m.SceneKindValue() != SceneKindTown || !m.CompactTown && m.Source == nil || zoneID == 0 || !m.TeleportWalkable(Point{X: int(point.X), Y: int(point.Y)}) {
		return BindingPoint{}, false
	}
	// knownZone 保存当前处理所用的已知状态区域。
	knownZone := false
	// zone 接收遍历安全区域集合时当前的元素。
	for _, zone := range m.SafeZones {
		if zone.ID == zoneID && zone.Center == (Point{X: int(point.X), Y: int(point.Y)}) {
			knownZone = true
			break
		}
	}
	if !knownZone {
		return BindingPoint{}, false
	}
	if f.RuleVersion < 24 || f.TownDestination(m.ID) {
		return point, true
	}
	if f.TownTravel == nil || m.ID != f.TownTravel.TownMapID || m.CompactTown {
		return BindingPoint{}, false
	}
	// id 接收遍历目的地地图i“ds”时当前的元素。
	for _, id := range f.TownTravel.DestinationMapIDs {
		// town 保存由地图“by”标识取得的城镇，ok 表明查询、校验或类型转换是否成功。
		town, ok := f.MapByID(id)
		if !ok || !town.CompactTown || len(town.SafeZones) != 1 {
			continue
		}
		// legacyID 接收遍历兼容旧数据安全区域i“ds”时当前的元素。
		for _, legacyID := range town.LegacySafeZoneIDs {
			// center 保存当前处理所用的中心格。
			center := town.SafeZones[0].Center
			if legacyID == zoneID && town.TeleportWalkable(center) {
				return BindingPoint{MapID: town.ID, X: int32(center.X), Y: int32(center.Y)}, true
			}
		}
	}
	return BindingPoint{}, false
}

// ValidateBinding 校验会话绑定。
// f 是调用该方法的开发测试配置实例。
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
	if f.RuleVersion >= 14 && f.Binding.SafeZoneEnabled == nil {
		return errors.New("rule14起须显式提供独立安全区绑定开关")
	}
	if f.RuleVersion < 14 && f.Binding.SafeZoneEnabled != nil {
		return errors.New("安全区绑定开关须使用rule14及后续版本")
	}
	if f.SafeZoneBindingEnabled() {
		if len(f.Map.SafeZones) != 0 {
			return errors.New("本阶段安全区绑定仅接已核来源城镇，不接个人图临时区域")
		}
		// found 保存当前处理所用的已找到标记。
		found := false
		// i 标记遍历地图集合时访问的下标。
		for i := range f.Maps {
			// m 保存当前处理所用的地图集合中的指定项。
			m := &f.Maps[i]
			if m.SceneKindValue() != SceneKindTown || m.Source == nil && !m.CompactTown || len(m.SafeZones) == 0 || f.RuleVersion >= 24 && !f.TownDestination(m.ID) {
				continue
			}
			found = true
			// zone 接收遍历安全区域集合时当前的元素。
			for _, zone := range m.SafeZones {
				if !m.Walkable(zone.Center) {
					return errors.New("安全区绑定中心须属于来源城镇的可走地格")
				}
			}
		}
		if !found {
			return errors.New("安全区绑定须先配置来源城镇及完整安全区")
		}
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
	// point 接收遍历起点或开始时间点位集合时当前的元素。
	for _, point := range f.Binding.StartPoints {
		if !f.ValidBinding(point) {
			return errors.New("绑定起点须属于已配置地图的可走地格")
		}
	}
	return nil
}
