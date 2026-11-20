package devconfig

import (
	"errors"
	"slices"
	"strconv"
)

// 城镇准入与资源等待是开发合同；不改变续租、数据库事务或战斗动作时限。
type TownTravelConfig struct {
	Enabled           bool     `json:"enabled"`
	TownMapID         uint64   `json:"town_map_id"`
	DestinationMapIDs []uint64 `json:"destination_map_ids,omitempty"`
	PlanTTLMS         int64    `json:"plan_ttl_ms"`
	QuiesceTTLMS      int64    `json:"quiesce_ttl_ms"`
	AckTTLMS          int64    `json:"ack_ttl_ms"`
	MaxPlansPerScene  int      `json:"max_plans_per_scene"`
}

func (t TownTravelConfig) Snapshot() TownTravelConfig {
	t.DestinationMapIDs = slices.Clone(t.DestinationMapIDs)
	return t
}

func (t TownTravelConfig) Matches(other TownTravelConfig) bool {
	return t.Enabled == other.Enabled && t.TownMapID == other.TownMapID && slices.Equal(t.DestinationMapIDs, other.DestinationMapIDs) &&
		t.PlanTTLMS == other.PlanTTLMS && t.QuiesceTTLMS == other.QuiesceTTLMS && t.AckTTLMS == other.AckTTLMS && t.MaxPlansPerScene == other.MaxPlansPerScene
}

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
	Enabled                 bool  `json:"enabled"`
	WorkPerTask             int   `json:"work_per_task"`
	WorkPerWorkerTick       int   `json:"work_per_worker_tick"`
	WorkspacesPerWorker     int   `json:"workspaces_per_worker"`
	QueuePerWorker          int   `json:"queue_per_worker"`
	MaxExpanded             int   `json:"max_expanded"`
	MaxFrontier             int   `json:"max_frontier"`
	MaxRouteCells           int   `json:"max_route_cells"`
	MaxReplans              int   `json:"max_replans"`
	MaxDynamicCells         int   `json:"max_dynamic_cells"`
	MaxMemoryBytesPerWorker int64 `json:"max_memory_bytes_per_worker"`
}

func (f *Fixture) ValidateTownConfiguration() error {
	if f.RuleVersion < 13 && f.TownTravel == nil && f.TownNavigation == nil {
		return nil
	}
	t, n := f.TownTravel, f.TownNavigation
	if t == nil || n == nil {
		return errors.New("rule13城镇旅行与导航合同必须同时显式配置")
	}
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
	maxCells := m.CellBudget()
	if f.RuleVersion >= 24 {
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
	word := int64(strconv.IntSize / 8)
	workspaceBytes := int64(maxCells)*(12+word) + int64(n.MaxFrontier)*word
	routeBytes := int64(n.MaxRouteCells) * 2 * word
	reservedBytes := int64(n.WorkspacesPerWorker)*workspaceBytes + int64(n.WorkspacesPerWorker+n.QueuePerWorker)*routeBytes
	if n.MaxMemoryBytesPerWorker <= 0 || reservedBytes > n.MaxMemoryBytesPerWorker {
		return errors.New("城镇导航工作区及路径预留超过显式每worker内存预算")
	}
	return nil
}

func (f *Fixture) validateCompactTownDestinations(legacy *MapTemplate, ids []uint64) (int, error) {
	if legacy.CompactTown || legacy.Source == nil || len(legacy.SafeZones) == 0 || len(ids) == 0 || len(ids) >= len(f.Maps) {
		return 0, errors.New("紧凑城镇须显式提供有界目录与不开放旅行的完整历史绑定地图")
	}
	legacyZones := make(map[uint64]bool, len(legacy.SafeZones))
	for _, zone := range legacy.SafeZones {
		legacyZones[zone.ID] = true
	}
	seen := make(map[uint64]bool, len(ids))
	maxCells := 0
	for _, id := range ids {
		m, ok := f.MapByID(id)
		if !ok || !m.CompactTown || m.SceneKindValue() != SceneKindTown || m.CellBudget() != 17*17 || len(m.SafeZones) != 1 || seen[id] {
			return 0, errors.New("旅行目的地须为唯一的17×17紧凑城镇")
		}
		seen[id] = true
		maxCells = max(maxCells, m.CellBudget())
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
