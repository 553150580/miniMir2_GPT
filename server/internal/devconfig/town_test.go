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
		{"disabled_navigation", func(f *Fixture) { f.TownNavigation.Enabled = false }},
		{"zero_timeout", func(f *Fixture) { f.TownTravel.PlanTTLMS = 0 }},
		{"unbounded_quiesce", func(f *Fixture) { f.TownTravel.QuiesceTTLMS = f.TownTravel.PlanTTLMS + 1 }},
		{"no_tick_budget", func(f *Fixture) { f.TownNavigation.WorkPerWorkerTick = 0 }},
		{"worker_budget_too_small", func(f *Fixture) { f.TownNavigation.WorkPerWorkerTick = f.TownNavigation.WorkPerTask - 1 }},
		{"no_pool", func(f *Fixture) { f.TownNavigation.WorkspacesPerWorker = 0 }},
		{"no_queue", func(f *Fixture) { f.TownNavigation.QueuePerWorker = 0 }},
		{"route_exceeds_grid", func(f *Fixture) { f.TownNavigation.MaxRouteCells = 490001 }},
		{"missing_memory_limit", func(f *Fixture) { f.TownNavigation.MaxMemoryBytesPerWorker = 0 }},
		{"no_plan_history", func(f *Fixture) { f.TownTravel.MaxPlansPerScene = 0 }},
		{"unbounded_plan_history", func(f *Fixture) { f.TownTravel.MaxPlansPerScene = 257 }},
		{"pool_exceeds_memory", func(f *Fixture) { f.TownNavigation.WorkspacesPerWorker = 16 }},
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
