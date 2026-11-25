// 提供历史城镇兼容测试的配置辅助，按旧版地图、旅行协议和寻路预算冻结测试条件。

package integration

import (
	"minimir/internal/devconfig"
	"minimir/internal/persistence"
	"slices"
	"testing"
)

// historicalOrdinaryFixture 用主图首槽的原模板恢复旧规则，仅作用于本次验收配置。
func historicalOrdinaryFixture(t *testing.T, f *devconfig.Fixture) {
	t.Helper()
	if len(f.Map.MonsterSpawnIDs) == 0 {
		return
	}
	original, ok := f.MonsterByID(f.Map.MonsterSpawnIDs[0])
	if !ok {
		t.Fatal("当前主图缺少用于历史验收的首槽怪物")
	}
	oldMonster := *original
	oldMonster.DropPoolID = 0
	f.Monsters = []devconfig.Monster{oldMonster}
	f.OrdinaryDropPools = nil
	f.OrdinaryEquipmentQuotas = nil // 历史规则没有普通怪专属高品质配额。
	f.Map.MonsterSpawnIDs, f.Map.InitialMonsterCount = nil, 0
	f.Maps = slices.DeleteFunc(slices.Clone(f.Maps), func(m devconfig.MapTemplate) bool {
		return m.SceneKind == devconfig.SceneKindPersonal
	})
}

// 历史WS回归沿用目录保留的原图与原区坐标，并固定旧旅行协议及大图寻路预算。
// t 提供断言失败报告、子用例和清理注册。
func historicalSourceTownConfig(t *testing.T) devconfig.Config {
	t.Helper()
	// cfg 保存由加载取得的服务配置，err 接收加载过程中可能产生的错误，供后续失败分支处理。
	cfg, err := loadIntegrationConfig()
	must(t, err)
	// f 保存当前处理所用的开发测试配置。
	f := &cfg.Fixture
	historicalOrdinaryFixture(t, f)
	// legacy 保存由地图“by”标识取得的兼容旧数据，exists 保存由地图“by”标识取得的存在。
	legacy, exists := f.MapByID(f.TownTravel.TownMapID)
	if !exists || legacy.CompactTown || legacy.Source == nil {
		t.Fatal("当前目录缺少供兼容回归使用的完整历史地图")
	}
	f.Maps = []devconfig.MapTemplate{*legacy}
	f.RuleVersion = 23
	// 保留旧版免费宠物，确保旅行测试以原战斗路径准备首杀和地面掉落。
	f.PetRuntime.BootstrapMode = "legacy_untrained_pet"
	if f.Map.Width == 13 {
		f.Map.Width, f.Map.Height = 17, 17
	}
	f.NPCShops = nil
	f.EquipmentDurability = nil
	f.Hash = persistence.Hash([]byte(f.Hash + ":historical-town-rule23-test"))
	f.TownTravel.DestinationMapIDs = nil
	// n 保存当前处理所用的城镇导航。
	n := f.TownNavigation
	n.WorkPerTask, n.WorkPerWorkerTick = 1024, 8192
	n.MaxExpanded, n.MaxFrontier, n.MaxRouteCells = 490000, 65536, 4096
	must(t, f.Validate())
	return cfg
}
