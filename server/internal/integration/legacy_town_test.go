package integration

import (
	"minimir/internal/devconfig"
	"minimir/internal/persistence"
	"testing"
)

// 历史WS回归沿用目录保留的原图与原区坐标，并固定旧旅行协议及大图寻路预算。
func historicalSourceTownConfig(t *testing.T) devconfig.Config {
	t.Helper()
	cfg, err := devconfig.Load("../../../conf/dev/server.json")
	must(t, err)
	f := &cfg.Fixture
	legacy, exists := f.MapByID(f.TownTravel.TownMapID)
	if !exists || legacy.CompactTown || legacy.Source == nil {
		t.Fatal("当前目录缺少供兼容回归使用的完整历史地图")
	}
	f.Maps = []devconfig.MapTemplate{*legacy}
	f.RuleVersion = 23
	f.Hash = persistence.Hash([]byte(f.Hash + ":historical-town-rule23-test"))
	f.TownTravel.DestinationMapIDs = nil
	n := f.TownNavigation
	n.WorkPerTask, n.WorkPerWorkerTick = 1024, 8192
	n.MaxExpanded, n.MaxFrontier, n.MaxRouteCells = 490000, 65536, 4096
	must(t, f.Validate())
	return cfg
}
