// 本文件核验讨伐链显式开启、四段装配、地图隔离以及静态路径口径。
package devconfig

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// TestDungeonReleaseIsExplicitAndIsolated 用原规则快照核对默认关闭和独立候选装载。
func TestDungeonReleaseIsExplicitAndIsolated(t *testing.T) {
	cfg, err := Load("../../../conf/dev/server.json")
	if err != nil {
		t.Fatal(err)
	}
	fixture := &cfg.Fixture
	if len(fixture.DungeonReleases) != 0 || len(fixture.Dungeons) != 0 {
		t.Fatal("可用试玩规则不应投影无已审定来源的讨伐链")
	}
	enabled := dungeonDefinitionForTest(fixture.ChallengeMonsters[0].ID)
	serialized, err := json.Marshal(enabled)
	if err != nil {
		t.Fatal(err)
	}
	var fromJSON DungeonDefinition // 验证投影到游戏时按原 JSON 字段装载。
	if err = json.Unmarshal(serialized, &fromJSON); err != nil {
		t.Fatal(err)
	}
	fixture.DungeonReleases = []DungeonDefinition{fromJSON, {TemplateID: 2}}
	if err = fixture.Validate(); err != nil {
		t.Fatal(err)
	}
	trial, found := fixture.DungeonTemplateByID(enabled.TemplateID)
	if !found || len(fixture.Dungeons) != 1 || trial.Duration != 2*time.Minute || trial.MinimumLevel != 30 ||
		trial.Map.SceneKindValue() != SceneKindDungeon || trial.Map.PlayerSpawn != trial.Stages[0].PlayerSpawns[0] ||
		len(trial.Stages) != 4 || len(trial.Stages[0].Enemies) != 2 || trial.Stages[3].Kind != DungeonStageFinalBoss {
		t.Fatal("原四段或运行模板字段未在独立目录装配")
	}
	if _, found = fixture.DungeonTemplateByID(2); found {
		t.Fatal("关闭的模板出现在运行时招募目录")
	}
	if _, found = fixture.MapByID(trial.Map.ID); found || !trial.Map.Walkable(trial.Map.PlayerSpawn) || trial.Map.Walkable(Point{X: 2, Y: 4}) {
		t.Fatal("讨伐链地图进入普通目录，或静态阻挡没有生效")
	}
	defaultSpawn := trial.Stages[0].PlayerSpawns[0] // 已装配快照应与配置源编辑分离。
	fixture.DungeonReleases[0].Stages[0].PlayerSpawns[0] = Point{}
	fixture.DungeonReleases[0].Map.Blocked[0] = defaultSpawn
	if trial.Stages[0].PlayerSpawns[0] != defaultSpawn || !trial.Map.Walkable(defaultSpawn) {
		t.Fatal("原始模板的切片仍然更改运行时小区")
	}
	fixture.DungeonReleases = nil
	if err = fixture.Validate(); err != nil || len(fixture.Dungeons) != 0 {
		t.Fatalf("再次装配无模板时没有关闭旧目录：%v", err)
	}
}

// TestDungeonReleaseRejectsInvalidGeometryAndSources 用合法底图逐项检查失败边界。
func TestDungeonReleaseRejectsInvalidGeometryAndSources(t *testing.T) {
	tests := []struct {
		name string         // 用例对应的失败条件。
		edit func(*Fixture) // 只修改本例的原配置。
		want string         // 确认命中预期校验而非其它条件。
	}{
		{"段数", func(f *Fixture) { f.DungeonReleases[0].Stages = f.DungeonReleases[0].Stages[:3] }, "四段"},
		{"顺序", func(f *Fixture) { f.DungeonReleases[0].Stages[1].Kind = DungeonStageElite }, "顺序"},
		{"越界", func(f *Fixture) { f.DungeonReleases[0].Stages[3].Area.Max.X++ }, "范围"},
		{"反向范围", func(f *Fixture) { f.DungeonReleases[0].Stages[3].Area.Min.Y = 5 }, "范围"},
		{"重叠", func(f *Fixture) { f.DungeonReleases[0].Stages[1].Area.Min.X-- }, "重叠"},
		{"不足三出生点", func(f *Fixture) { f.DungeonReleases[0].Stages[0].PlayerSpawns = []Point{{X: 1, Y: 1}} }, "落点"},
		{"重复出生", func(f *Fixture) { f.DungeonReleases[0].Stages[0].PlayerSpawns[1] = Point{X: 1, Y: 1} }, "重复"},
		{"兵种撞出生", func(f *Fixture) { f.DungeonReleases[0].Stages[0].Enemies[0].Spawn = Point{X: 1, Y: 1} }, "怪物"},
		{"外部怪物", func(f *Fixture) { f.DungeonReleases[0].Stages[0].Enemies[0].Spawn.X = 10 }, "怪物"},
		{"未知怪物", func(f *Fixture) { f.DungeonReleases[0].Stages[0].Enemies[0].MonsterID = 911_911 }, "怪物"},
		{"误接普通掉落", func(f *Fixture) { f.ChallengeMonsters[0].DropPoolID = 1 }, "普通收益"},
		{"误接复活", func(f *Fixture) { f.ChallengeMonsters[0].RespawnMS = 1_000 }, "复活"},
		{"禁用可收怪", func(f *Fixture) { f.ChallengeMonsters[0].CanTame = true }, "挑战目录"},
		{"重复地图", func(f *Fixture) { f.DungeonReleases[0].Map.ID = f.Map.ID }, "编号已被使用"},
		{"挑战地图冲突", func(f *Fixture) { f.WorldBoss = &WorldBossConfig{Map: f.DungeonReleases[0].Map} }, "编号已被使用"},
		{"复制模板", func(f *Fixture) {
			fourth := f.DungeonReleases[0]
			fourth.Map.ID++
			f.DungeonReleases = append(f.DungeonReleases, fourth)
		}, "编号已被使用"},
		{"地图类型错误", func(f *Fixture) { f.DungeonReleases[0].Map.SceneKind = SceneKindWorldBoss }, "地图"},
		{"无时长", func(f *Fixture) { f.DungeonReleases[0].DurationMS = 0 }, "时长"},
		{"同小区内无法接近怪物", func(f *Fixture) {
			f.DungeonReleases[0].Map.Blocked = []Point{{X: 2, Y: 1}, {X: 2, Y: 2}, {X: 2, Y: 3}, {X: 2, Y: 4}}
			f.DungeonReleases[0].Stages[0].Enemies[0].Spawn = Point{X: 3, Y: 2}
		}, "出生"},
		{"相邻边完全被阻挡", func(f *Fixture) {
			f.DungeonReleases[0].Map.Blocked = []Point{{X: 3, Y: 1}, {X: 3, Y: 2}, {X: 3, Y: 3}, {X: 3, Y: 4}}
			f.DungeonReleases[0].Stages[0].Enemies[1].Spawn = Point{X: 2, Y: 3}
		}, "相邻边"},
	}
	for _, row := range tests {
		t.Run(row.name, func(t *testing.T) {
			monster := Monster{ID: 92_001, Fighter: Fighter{HP: 24, MoveMS: 700, AttackMS: 900}}
			fixture := Fixture{Map: MapTemplate{ID: 1}, ChallengeMonsters: []Monster{monster}, DungeonReleases: []DungeonDefinition{dungeonDefinitionForTest(monster.ID)}}
			row.edit(&fixture)
			if err := fixture.validateDungeonReleases(); err == nil || !strings.Contains(err.Error(), row.want) || len(fixture.Dungeons) != 0 {
				t.Fatalf("非法讨伐链应在装配前拒绝并不留下可入场模板：%v", err)
			}
		})
	}
}

// dungeonDefinitionForTest 使用四段相邻小区和每段两个怪物构造仅供测试的地图。
func dungeonDefinitionForTest(monsterID uint32) DungeonDefinition {
	result := DungeonDefinition{Enabled: true, TemplateID: 71, DisplayName: "隔离讨伐链", MinimumLevel: 30, DurationMS: 120_000,
		Map: MapTemplate{ID: 1_001, Width: 13, Height: 7, SceneKind: SceneKindDungeon, Blocked: []Point{{X: 2, Y: 4}}}}
	for i, kind := range []DungeonStageKind{DungeonStageMobsFirst, DungeonStageMobsSecond, DungeonStageElite, DungeonStageFinalBoss} {
		x := i*3 + 1
		result.Stages = append(result.Stages, DungeonStageDefinition{Kind: kind, Area: DungeonArea{Min: Point{X: x, Y: 1}, Max: Point{X: x + 2, Y: 4}},
			PlayerSpawns: []Point{{X: x, Y: 1}, {X: x, Y: 2}, {X: x, Y: 3}},
			Enemies:      []DungeonEnemyDefinition{{MonsterID: monsterID, Spawn: Point{X: x + 1, Y: 2}}, {MonsterID: monsterID, Spawn: Point{X: x + 2, Y: 3}}}})
	}
	return result
}
