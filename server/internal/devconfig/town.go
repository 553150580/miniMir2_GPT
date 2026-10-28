package devconfig

import (
	"errors"
	"strconv"
)

// 城镇准入与资源等待是开发合同；不改变续租、数据库事务或战斗动作时限。
type TownTravelConfig struct {
	Enabled          bool   `json:"enabled"`
	TownMapID        uint64 `json:"town_map_id"`
	PlanTTLMS        int64  `json:"plan_ttl_ms"`
	QuiesceTTLMS     int64  `json:"quiesce_ttl_ms"`
	AckTTLMS         int64  `json:"ack_ttl_ms"`
	MaxPlansPerScene int    `json:"max_plans_per_scene"`
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
	if n.WorkPerTask <= 0 || n.WorkPerWorkerTick < n.WorkPerTask || n.WorkspacesPerWorker <= 0 || n.QueuePerWorker <= 0 || n.MaxExpanded <= 0 || n.MaxFrontier <= 0 || n.MaxRouteCells <= 0 || n.MaxReplans < 0 || n.MaxDynamicCells < 2 {
		return errors.New("城镇导航预算必须完整且显式为有效整数")
	}
	if n.WorkspacesPerWorker > 256 || n.QueuePerWorker > 256 || n.MaxExpanded > m.CellBudget() || n.MaxFrontier > m.CellBudget() || n.MaxRouteCells > m.CellBudget() || n.MaxDynamicCells > m.CellBudget() || n.MaxReplans > m.CellBudget() || n.WorkPerWorkerTick > m.CellBudget() {
		return errors.New("城镇导航预算不得超过已配置图或现有worker准入上限")
	}
	word := int64(strconv.IntSize / 8)
	workspaceBytes := int64(m.CellBudget())*(12+word) + int64(n.MaxFrontier)*word
	routeBytes := int64(n.MaxRouteCells) * 2 * word
	reservedBytes := int64(n.WorkspacesPerWorker)*workspaceBytes + int64(n.WorkspacesPerWorker+n.QueuePerWorker)*routeBytes
	if n.MaxMemoryBytesPerWorker <= 0 || reservedBytes > n.MaxMemoryBytesPerWorker {
		return errors.New("城镇导航工作区及路径预留超过显式每worker内存预算")
	}
	return nil
}
