// 本文件验证城镇预算及来源地图约束、旧规则隔离和紧凑城镇绑定迁移。

package devconfig

import "testing"

// TestTownRulesRequireCompleteExplicitBudgetsAndKnownSourceMap 核对「非法城镇合同被接纳」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestTownRulesRequireCompleteExplicitBudgetsAndKnownSourceMap(t *testing.T) {
	// cases 建立带初始值的临时结构列表，供本段继续处理。
	cases := []struct {
		// name 记录名称，使用文本表示。
		name string
		// mutate 记录变更，使用供当前流程调用的函数表示。
		mutate func(*Fixture)
	}{
		// f 记录开发测试配置。
		{"missing_travel", func(f *Fixture) { f.TownTravel = nil }},
		// f 记录开发测试配置。
		{"missing_navigation", func(f *Fixture) { f.TownNavigation = nil }},
		// f 记录开发测试配置。
		{"personal_target", func(f *Fixture) { f.TownTravel.TownMapID = 1 }},
		// f 记录开发测试配置。
		{"unknown_target", func(f *Fixture) { f.TownTravel.TownMapID = 99 }},
		// f 记录开发测试配置。
		{"missing_compact_targets", func(f *Fixture) { f.TownTravel.DestinationMapIDs = nil }},
		// f 记录开发测试配置。
		{"duplicate_compact_target", func(f *Fixture) { f.TownTravel.DestinationMapIDs[1] = f.TownTravel.DestinationMapIDs[0] }},
		// f 记录开发测试配置。
		{"legacy_map_as_compact_target", func(f *Fixture) { f.TownTravel.DestinationMapIDs[0] = f.TownTravel.TownMapID }},
		// f 记录开发测试配置。
		{"unmapped_legacy_zone", func(f *Fixture) {
			// m 接收地图“by”标识供后续步骤使用。
			m, _ := f.MapByID(3)
			m.LegacySafeZoneIDs = m.LegacySafeZoneIDs[1:]
		}},
		// f 记录开发测试配置。
		{"duplicate_legacy_zone", func(f *Fixture) {
			// m 接收地图“by”标识供后续步骤使用。
			m, _ := f.MapByID(3)
			m.LegacySafeZoneIDs = append(m.LegacySafeZoneIDs, m.LegacySafeZoneIDs[0])
		}},
		// f 记录开发测试配置。
		{"disabled_navigation", func(f *Fixture) { f.TownNavigation.Enabled = false }},
		// f 记录开发测试配置。
		{"zero_timeout", func(f *Fixture) { f.TownTravel.PlanTTLMS = 0 }},
		// f 记录开发测试配置。
		{"unbounded_quiesce", func(f *Fixture) { f.TownTravel.QuiesceTTLMS = f.TownTravel.PlanTTLMS + 1 }},
		// f 记录开发测试配置。
		{"no_tick_budget", func(f *Fixture) { f.TownNavigation.WorkPerWorkerTick = 0 }},
		// f 记录开发测试配置。
		{"worker_budget_too_small", func(f *Fixture) { f.TownNavigation.WorkPerWorkerTick = f.TownNavigation.WorkPerTask - 1 }},
		// f 记录开发测试配置。
		{"no_pool", func(f *Fixture) { f.TownNavigation.WorkspacesPerWorker = 0 }},
		// f 记录开发测试配置。
		{"no_queue", func(f *Fixture) { f.TownNavigation.QueuePerWorker = 0 }},
		// f 记录开发测试配置。
		{"route_exceeds_grid", func(f *Fixture) { f.TownNavigation.MaxRouteCells = 17*17 + 1 }},
		// f 记录开发测试配置。
		{"missing_memory_limit", func(f *Fixture) { f.TownNavigation.MaxMemoryBytesPerWorker = 0 }},
		// f 记录开发测试配置。
		{"no_plan_history", func(f *Fixture) { f.TownTravel.MaxPlansPerScene = 0 }},
		// f 记录开发测试配置。
		{"unbounded_plan_history", func(f *Fixture) { f.TownTravel.MaxPlansPerScene = 257 }},
		// f 记录开发测试配置。
		{"pool_exceeds_memory", func(f *Fixture) { f.TownNavigation.MaxMemoryBytesPerWorker = 1 }},
	}
	// tc 接收遍历“cases”时当前的元素。
	for _, tc := range cases {
		// t 提供断言失败报告、子用例和清理注册。
		t.Run(tc.name, func(t *testing.T) {
			// cfg 保存由加载取得的服务配置，err 接收加载过程中可能产生的错误，供后续失败分支处理。
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

// TestLegacyFixtureDoesNotInheritTownDefaults 核验旧配置不应隐式获得城镇入口或预算。
// t 提供断言失败报告、子用例和清理注册。
func TestLegacyFixtureDoesNotInheritTownDefaults(t *testing.T) {
	// f 组装开发测试配置所需的数据。
	f := Fixture{RuleVersion: 12}
	// err 接收校验城镇“configuration”过程中可能产生的错误，供后续失败分支处理。
	if err := f.ValidateTownConfiguration(); err != nil {
		t.Fatal(err)
	}
	if f.TownTravel != nil || f.TownNavigation != nil {
		t.Fatal("旧配置不应隐式获得城镇入口或预算")
	}
}

// TestCompactTownKeepsAllLegacySafeZoneMappingsAndRequiresBlockedNPCs 核验历史安全区应映射到地图中心，用诊断「紧凑城镇NPC置于可走入口仍获准启用」定位对应断言。
// t 提供断言失败报告、子用例和清理注册。
func TestCompactTownKeepsAllLegacySafeZoneMappingsAndRequiresBlockedNPCs(t *testing.T) {
	// cfg 保存由加载取得的服务配置，err 接收加载过程中可能产生的错误，供后续失败分支处理。
	cfg, err := Load("../../../conf/dev/server.json")
	if err != nil {
		t.Fatal(err)
	}
	// f 保存当前处理所用的开发测试配置。
	f := &cfg.Fixture
	// legacy 保存由地图“by”标识取得的兼容旧数据。
	legacy, _ := f.MapByID(2)
	// wantMaps 初始化用作期望值地图集合的按数值索引的数值映射。
	wantMaps := map[uint64]uint64{106: 3, 107: 3, 108: 3, 113: 4, 117: 5}
	// zone 接收遍历安全区域集合时当前的元素。
	for _, zone := range legacy.SafeZones {
		// wantMap 保存从期望值地图集合取出的期望值地图。
		wantMap := wantMaps[zone.ID]
		// m 接收地图“by”标识供后续步骤使用，exists 保存由地图“by”标识取得的存在。
		m, exists := f.MapByID(wantMap)
		if !exists {
			t.Fatalf("历史安全区%d缺少当前城镇", zone.ID)
		}
		// center 保存当前处理所用的中心格。
		center := m.SafeZones[0].Center
		// b 建立带初始值的会话绑定点位，供本段继续处理。
		b := BindingPoint{MapID: legacy.ID, X: int32(zone.Center.X), Y: int32(zone.Center.Y)}
		// mapped 保存由城镇瞬移会话绑定取得的“mapped”，ok 表明查询、校验或类型转换是否成功。
		mapped, ok := f.TownTeleportBinding(b, zone.ID)
		if !ok || mapped != (BindingPoint{MapID: wantMap, X: int32(center.X), Y: int32(center.Y)}) {
			t.Fatalf("历史安全区%d没有映射到地图%d中心", zone.ID, wantMap)
		}
		b.X++
		// ok 表明查询、校验或类型转换是否成功。
		if _, ok := f.TownTeleportBinding(b, zone.ID); ok {
			t.Fatalf("历史安全区%d接受错配源坐标", zone.ID)
		}
	}
	// m 接收地图“by”标识供后续步骤使用。
	m, _ := f.MapByID(3)
	m.NPCs[0].Position = m.PlayerSpawn
	if f.ValidateMap() == nil {
		t.Fatal("紧凑城镇NPC置于可走入口仍获准启用")
	}
}
