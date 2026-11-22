// 本文件解析城镇准入、搜索和跨图预算，核对安全落点、源地图和 NPC 前置关系。

package devconfig

import (
	"errors"
	"slices"
	"strconv"
)

// 城镇准入与资源等待是开发合同；不改变续租、数据库事务或战斗动作时限。
type TownTravelConfig struct {
	// Enabled 标记是否处于已开启状态。
	Enabled bool `json:"enabled"`
	// TownMapID 标识当前业务使用的城镇地图。
	TownMapID uint64 `json:"town_map_id"`
	// DestinationMapIDs 记录目的地地图i“ds”，使用数值列表表示。
	DestinationMapIDs []uint64 `json:"destination_map_ids,omitempty"`
	// PlanTTLMS 记录计划“ttlms”，使用数值表示。
	PlanTTLMS int64 `json:"plan_ttl_ms"`
	// QuiesceTTLMS 记录“quiesce”“ttlms”，使用数值表示。
	QuiesceTTLMS int64 `json:"quiesce_ttl_ms"`
	// AckTTLMS 记录确认“ttlms”，使用数值表示。
	AckTTLMS int64 `json:"ack_ttl_ms"`
	// MaxPlansPerScene 记录最大计划集合“per”场景，使用数值表示。
	MaxPlansPerScene int `json:"max_plans_per_scene"`
}

// Snapshot 返回快照供调用方使用。
// t 是调用该方法的城镇地图切换配置实例。
func (t TownTravelConfig) Snapshot() TownTravelConfig {
	t.DestinationMapIDs = slices.Clone(t.DestinationMapIDs)
	return t
}

// Matches 比较并判断是否符合当前请求或状态，并把结果交给调用方。
// t 是调用该方法的城镇地图切换配置实例，other 记录另一项，使用城镇地图切换配置表示。
func (t TownTravelConfig) Matches(other TownTravelConfig) bool {
	return t.Enabled == other.Enabled && t.TownMapID == other.TownMapID && slices.Equal(t.DestinationMapIDs, other.DestinationMapIDs) &&
		t.PlanTTLMS == other.PlanTTLMS && t.QuiesceTTLMS == other.QuiesceTTLMS && t.AckTTLMS == other.AckTTLMS && t.MaxPlansPerScene == other.MaxPlansPerScene
}

// TownDestination 判断城镇目的地是否满足要求。
// f 是调用该方法的开发测试配置实例，id 区分当前步骤指向的目标对象。
func (f *Fixture) TownDestination(id uint64) bool {
	if f.TownTravel == nil {
		return false
	}
	if f.RuleVersion < 24 {
		return id != 0 && id == f.TownTravel.TownMapID
	}
	return slices.Contains(f.TownTravel.DestinationMapIDs, id)
}

// 所有预算由唯一Excel显式提供；每worker的池与每tick总预算约束所有角色。
type TownNavigationConfig struct {
	// Enabled 标记是否处于已开启状态。
	Enabled bool `json:"enabled"`
	// WorkPerTask 记录工作量“per”任务，使用数值表示。
	WorkPerTask int `json:"work_per_task"`
	// WorkPerWorkerTick 记录工作量“per”房间执行器模拟帧，使用数值表示。
	WorkPerWorkerTick int `json:"work_per_worker_tick"`
	// WorkspacesPerWorker 记录工作区集合“per”房间执行器，使用数值表示。
	WorkspacesPerWorker int `json:"workspaces_per_worker"`
	// QueuePerWorker 记录队列“per”房间执行器，使用数值表示。
	QueuePerWorker int `json:"queue_per_worker"`
	// MaxExpanded 记录最大“expanded”，使用数值表示。
	MaxExpanded int `json:"max_expanded"`
	// MaxFrontier 记录最大搜索边界，使用数值表示。
	MaxFrontier int `json:"max_frontier"`
	// MaxRouteCells 记录最大路由地图格集合，使用数值表示。
	MaxRouteCells int `json:"max_route_cells"`
	// MaxReplans 记录最大“replans”，使用数值表示。
	MaxReplans int `json:"max_replans"`
	// MaxDynamicCells 记录最大动态占位地图格集合，使用数值表示。
	MaxDynamicCells int `json:"max_dynamic_cells"`
	// MaxMemoryBytesPerWorker 记录最大“memory”字节“per”房间执行器，使用数值表示。
	MaxMemoryBytesPerWorker int64 `json:"max_memory_bytes_per_worker"`
}

// ValidateTownConfiguration 校验城镇“configuration”。
// f 是调用该方法的开发测试配置实例。
func (f *Fixture) ValidateTownConfiguration() error {
	if f.RuleVersion < 13 && f.TownTravel == nil && f.TownNavigation == nil {
		return nil
	}
	// t 保存城镇地图切换，用于当前步骤中的t，n 保存当前处理所用的城镇导航。
	t, n := f.TownTravel, f.TownNavigation
	if t == nil || n == nil {
		return errors.New("rule13城镇旅行与导航合同必须同时显式配置")
	}
	// m 接收地图“by”标识供后续步骤使用，ok 表明查询、校验或类型转换是否成功。
	m, ok := f.MapByID(t.TownMapID)
	if !ok || m.SceneKindValue() != SceneKindTown || t.PlanTTLMS <= 0 || t.QuiesceTTLMS <= 0 || t.AckTTLMS <= 0 || t.PlanTTLMS > 86400000 || t.QuiesceTTLMS > t.PlanTTLMS || t.AckTTLMS > 86400000 {
		return errors.New("城镇旅行须引用已配置城镇及有效有界等待时限")
	}
	if t.Enabled && !n.Enabled {
		return errors.New("城镇旅行启用前必须启用有界导航")
	}
	if t.MaxPlansPerScene <= 0 || t.MaxPlansPerScene > 256 {
		return errors.New("每附着场景旅行ID历史必须显式有界，满时拒绝新计划而不遗忘取消记录")
	}
	// maxCells 保存由地图格预算取得的最大地图格集合。
	maxCells := m.CellBudget()
	if f.RuleVersion >= 24 {
		// err 保存当前操作的失败原因，供调用方检查或返回。
		var err error
		maxCells, err = f.validateCompactTownDestinations(m, t.DestinationMapIDs)
		if err != nil {
			return err
		}
	} else if len(t.DestinationMapIDs) != 0 {
		return errors.New("显式城镇目的地列表须使用rule24及后续版本")
	}
	if n.WorkPerTask <= 0 || n.WorkPerWorkerTick < n.WorkPerTask || n.WorkspacesPerWorker <= 0 || n.QueuePerWorker <= 0 || n.MaxExpanded <= 0 || n.MaxFrontier <= 0 || n.MaxRouteCells <= 0 || n.MaxReplans < 0 || n.MaxDynamicCells < 2 {
		return errors.New("城镇导航预算必须完整且显式为有效整数")
	}
	if n.WorkspacesPerWorker > 256 || n.QueuePerWorker > 256 || n.MaxExpanded > maxCells || n.MaxFrontier > maxCells || n.MaxRouteCells > maxCells || n.MaxDynamicCells > maxCells || n.MaxReplans > maxCells || n.WorkPerWorkerTick > maxCells {
		return errors.New("城镇导航预算不得超过已配置图或现有worker准入上限")
	}
	// word 保存由整数“64”取得的“word”。
	word := int64(strconv.IntSize / 8)
	// workspaceBytes 保存根据整数“64”与当前数据与“word”的计算结果的计算结果与整数“64”与“word”的计算结果计算的搜索工作区字节。
	workspaceBytes := int64(maxCells)*(12+word) + int64(n.MaxFrontier)*word
	// routeBytes 保存根据整数“64”与当前数据的计算结果与“word”计算的路由字节。
	routeBytes := int64(n.MaxRouteCells) * 2 * word
	// reservedBytes 保存根据整数“64”与搜索工作区字节的计算结果与整数“64”与路由字节的计算结果计算的已保留字节。
	reservedBytes := int64(n.WorkspacesPerWorker)*workspaceBytes + int64(n.WorkspacesPerWorker+n.QueuePerWorker)*routeBytes
	if n.MaxMemoryBytesPerWorker <= 0 || reservedBytes > n.MaxMemoryBytesPerWorker {
		return errors.New("城镇导航工作区及路径预留超过显式每worker内存预算")
	}
	return nil
}

// validateCompactTownDestinations 校验紧凑城镇目的地集合。
// f 是调用该方法的开发测试配置实例，legacy 记录兼容旧数据，使用地图模板表示，ids 记录标识集合，使用数值列表表示。
func (f *Fixture) validateCompactTownDestinations(legacy *MapTemplate, ids []uint64) (int, error) {
	if legacy.CompactTown || legacy.Source == nil || len(legacy.SafeZones) == 0 || len(ids) == 0 || len(ids) >= len(f.Maps) {
		return 0, errors.New("紧凑城镇须显式提供有界目录与不开放旅行的完整历史绑定地图")
	}
	// legacyZones 创建用于维护兼容旧数据区域集合的容器或通道。
	legacyZones := make(map[uint64]bool, len(legacy.SafeZones))
	// zone 接收遍历安全区域集合时当前的元素。
	for _, zone := range legacy.SafeZones {
		legacyZones[zone.ID] = true
	}
	// seen 创建用于维护“seen”的容器或通道。
	seen := make(map[uint64]bool, len(ids))
	// maxCells 规定最大地图格集合的初始取值。
	maxCells := 0
	// id 接收遍历标识集合时当前的元素。
	for _, id := range ids {
		// m 接收地图“by”标识供后续步骤使用，ok 表明查询、校验或类型转换是否成功。
		m, ok := f.MapByID(id)
		if !ok || !m.CompactTown || m.SceneKindValue() != SceneKindTown || m.CellBudget() != 17*17 || len(m.SafeZones) != 1 || seen[id] {
			return 0, errors.New("旅行目的地须为唯一的17×17紧凑城镇")
		}
		seen[id] = true
		maxCells = max(maxCells, m.CellBudget())
		// zoneID 接收遍历兼容旧数据安全区域i“ds”时当前的元素。
		for _, zoneID := range m.LegacySafeZoneIDs {
			if !legacyZones[zoneID] {
				return 0, errors.New("历史安全区映射须引用已知原区并且只能归属一个新城镇")
			}
			delete(legacyZones, zoneID)
		}
	}
	if len(legacyZones) != 0 {
		return 0, errors.New("历史安全区须全部提供明确的新城镇映射")
	}
	return maxCells, nil
}
