// 本文件验证分普通地图的出生槽引用和旧规则边界。
package devconfig

import (
	"slices"
	"testing"
)

// TestOrdinaryMonsterRosterRequiresValidTemplatesAndDistinctMaps 验证每张普通图的独立模板及构建范围。
func TestOrdinaryMonsterRosterRequiresValidTemplatesAndDistinctMaps(t *testing.T) {
	load := func(t *testing.T) Fixture {
		t.Helper()
		cfg, err := Load("../../../conf/dev/server.json")
		if err != nil {
			t.Fatal(err)
		}
		f := cfg.Fixture
		f.RuleVersion = OrdinaryMonsterRosterRule
		id := f.Monsters[0].ID
		f.Map.MonsterSpawnIDs = make([]uint32, OrdinaryMonsterSlots)
		for i := range f.Map.MonsterSpawnIDs {
			f.Map.MonsterSpawnIDs[i] = id
		}
		f.Map.InitialMonsterCount = 1
		target := f.Map
		target.ID, target.SceneKind, target.DisplayName = 50601, SceneKindPersonal, "普通图测试目录"
		target.MonsterSpawnIDs = slices.Clone(f.Map.MonsterSpawnIDs)
		f.Maps = append(f.Maps, target)
		return f
	}
	f := load(t)
	if err := f.ValidateMap(); err != nil {
		t.Fatal(err)
	}
	for name, change := range map[string]func(*Fixture){
		"旧规则忽略分地图配置": func(f *Fixture) { f.RuleVersion = OrdinaryMonsterRosterRule - 1 },
		"槽位错配":       func(f *Fixture) { f.Map.MonsterSpawnIDs = f.Map.MonsterSpawnIDs[:7] },
		"缺少模板":       func(f *Fixture) { f.Map.MonsterSpawnIDs[3] = 0 },
		"默认数量无效":     func(f *Fixture) { f.Map.InitialMonsterCount = 9 },
		"重复地图编号":     func(f *Fixture) { f.Maps[len(f.Maps)-1].ID = f.Map.ID },
		"额外地图尺寸无效":   func(f *Fixture) { f.Maps[len(f.Maps)-1].Width-- },
		"城镇误配怪目录":    func(f *Fixture) { f.Maps[0].MonsterSpawnIDs = []uint32{f.Monsters[0].ID} },
	} {
		t.Run(name, func(t *testing.T) {
			f := load(t)
			change(&f)
			if err := f.ValidateMap(); err == nil {
				t.Fatal("非法地图候选通过校验")
			}
		})
	}
}
