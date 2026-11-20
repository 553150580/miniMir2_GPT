package devconfig

import "testing"

func TestTownRulesRequireCompleteExplicitBudgetsAndKnownSourceMap(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*Fixture)
	}{
		{"missing_travel", func(f *Fixture) { f.TownTravel = nil }},
		{"missing_navigation", func(f *Fixture) { f.TownNavigation = nil }},
		{"personal_target", func(f *Fixture) { f.TownTravel.TownMapID = 1 }},
		{"unknown_target", func(f *Fixture) { f.TownTravel.TownMapID = 99 }},
		{"missing_compact_targets", func(f *Fixture) { f.TownTravel.DestinationMapIDs = nil }},
		{"duplicate_compact_target", func(f *Fixture) { f.TownTravel.DestinationMapIDs[1] = f.TownTravel.DestinationMapIDs[0] }},
		{"legacy_map_as_compact_target", func(f *Fixture) { f.TownTravel.DestinationMapIDs[0] = f.TownTravel.TownMapID }},
		{"unmapped_legacy_zone", func(f *Fixture) {
			m, _ := f.MapByID(3)
			m.LegacySafeZoneIDs = m.LegacySafeZoneIDs[1:]
		}},
		{"duplicate_legacy_zone", func(f *Fixture) {
			m, _ := f.MapByID(3)
			m.LegacySafeZoneIDs = append(m.LegacySafeZoneIDs, m.LegacySafeZoneIDs[0])
		}},
		{"disabled_navigation", func(f *Fixture) { f.TownNavigation.Enabled = false }},
		{"zero_timeout", func(f *Fixture) { f.TownTravel.PlanTTLMS = 0 }},
		{"unbounded_quiesce", func(f *Fixture) { f.TownTravel.QuiesceTTLMS = f.TownTravel.PlanTTLMS + 1 }},
		{"no_tick_budget", func(f *Fixture) { f.TownNavigation.WorkPerWorkerTick = 0 }},
		{"worker_budget_too_small", func(f *Fixture) { f.TownNavigation.WorkPerWorkerTick = f.TownNavigation.WorkPerTask - 1 }},
		{"no_pool", func(f *Fixture) { f.TownNavigation.WorkspacesPerWorker = 0 }},
		{"no_queue", func(f *Fixture) { f.TownNavigation.QueuePerWorker = 0 }},
		{"route_exceeds_grid", func(f *Fixture) { f.TownNavigation.MaxRouteCells = 17*17 + 1 }},
		{"missing_memory_limit", func(f *Fixture) { f.TownNavigation.MaxMemoryBytesPerWorker = 0 }},
		{"no_plan_history", func(f *Fixture) { f.TownTravel.MaxPlansPerScene = 0 }},
		{"unbounded_plan_history", func(f *Fixture) { f.TownTravel.MaxPlansPerScene = 257 }},
		{"pool_exceeds_memory", func(f *Fixture) { f.TownNavigation.MaxMemoryBytesPerWorker = 1 }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg, err := Load("../../../conf/dev/server.json")
			if err != nil {
				t.Fatal(err)
			}
			tc.mutate(&cfg.Fixture)
			if cfg.Fixture.ValidateTownConfiguration() == nil {
				t.Fatal("非法城镇合同被接纳")
			}
		})
	}
}

func TestLegacyFixtureDoesNotInheritTownDefaults(t *testing.T) {
	f := Fixture{RuleVersion: 12}
	if err := f.ValidateTownConfiguration(); err != nil {
		t.Fatal(err)
	}
	if f.TownTravel != nil || f.TownNavigation != nil {
		t.Fatal("旧配置不应隐式获得城镇入口或预算")
	}
}

func TestCompactTownKeepsAllLegacySafeZoneMappingsAndRequiresBlockedNPCs(t *testing.T) {
	cfg, err := Load("../../../conf/dev/server.json")
	if err != nil {
		t.Fatal(err)
	}
	f := &cfg.Fixture
	legacy, _ := f.MapByID(2)
	wantMaps := map[uint64]uint64{106: 3, 107: 3, 108: 3, 113: 4, 117: 5}
	for _, zone := range legacy.SafeZones {
		wantMap := wantMaps[zone.ID]
		m, exists := f.MapByID(wantMap)
		if !exists {
			t.Fatalf("历史安全区%d缺少当前城镇", zone.ID)
		}
		center := m.SafeZones[0].Center
		b := BindingPoint{MapID: legacy.ID, X: int32(zone.Center.X), Y: int32(zone.Center.Y)}
		mapped, ok := f.TownTeleportBinding(b, zone.ID)
		if !ok || mapped != (BindingPoint{MapID: wantMap, X: int32(center.X), Y: int32(center.Y)}) {
			t.Fatalf("历史安全区%d没有映射到地图%d中心", zone.ID, wantMap)
		}
		b.X++
		if _, ok := f.TownTeleportBinding(b, zone.ID); ok {
			t.Fatalf("历史安全区%d接受错配源坐标", zone.ID)
		}
	}
	m, _ := f.MapByID(3)
	m.NPCs[0].Position = m.PlayerSpawn
	if f.ValidateMap() == nil {
		t.Fatal("紧凑城镇NPC置于可走入口仍获准启用")
	}
}
