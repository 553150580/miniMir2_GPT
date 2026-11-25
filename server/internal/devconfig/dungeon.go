// 本文件装配讨伐链玩法的独立地图、四段固定流程和挑战模板引用。
package devconfig

import (
	"fmt"
	"math"
	"slices"
	"strings"
	"time"
)

const (
	// DungeonDailyEntries 是上海运营日内的基础入场次数。
	DungeonDailyEntries = 3
	// DungeonHostingBonusEntries 是有效托管卡在入场时提供的额外次数。
	DungeonHostingBonusEntries = 1
	// DungeonMaxPlayers 是一个队伍可用的总席位数。
	DungeonMaxPlayers = 3
	// DungeonLeaderDisconnectGrace 是招募中队长掉线后保留原队伍的时长。
	DungeonLeaderDisconnectGrace = time.Minute
)

// DungeonStageKind 标记工作簿规定的讨伐顺序。
type DungeonStageKind string

const (
	DungeonStageMobsFirst  DungeonStageKind = "mob_segment_1" // 第一段小怪。
	DungeonStageMobsSecond DungeonStageKind = "mob_segment_2" // 第二段小怪。
	DungeonStageElite      DungeonStageKind = "elite"         // 第三段精英。
	DungeonStageFinalBoss  DungeonStageKind = "final_boss"    // 第四段首领。
)

// DungeonArea 保存战斗小区的包含端点。通道须从相邻小区的可达边通过。
type DungeonArea struct {
	Min Point `json:"min"` // 包含的左上格。
	Max Point `json:"max"` // 包含的右下格。
}

// Contains 判断格子是否位于本段的小区内。
func (area DungeonArea) Contains(point Point) bool {
	return point.X >= area.Min.X && point.X <= area.Max.X && point.Y >= area.Min.Y && point.Y <= area.Max.Y
}

// DungeonEnemyDefinition 引用挑战怪物目录，独立于普通挂机刷新与掉落。
type DungeonEnemyDefinition struct {
	MonsterID uint32 `json:"monster_id"` // 当前段使用的挑战怪物编号。
	Spawn     Point  `json:"spawn"`      // 当前怪物在本段内的唯一出生格。
}

// DungeonStageDefinition 保存单段的小区、三个玩家落点和有序敌人。
type DungeonStageDefinition struct {
	Kind         DungeonStageKind         `json:"kind"`          // 固定四段之一。
	Area         DungeonArea              `json:"area"`          // 本段的战斗坐标范围。
	PlayerSpawns []Point                  `json:"player_spawns"` // 真人及未来 AI 依席位使用的三个落点。
	Enemies      []DungeonEnemyDefinition `json:"enemies"`       // 精英或最终段的首个怪物为主怪。
}

// DungeonDefinition 是候选配置的原始投影，缺少显式开启标记的模板不可使用。
type DungeonDefinition struct {
	Enabled      bool                     `json:"enabled"`           // 启用本模板的实验装配。
	TemplateID   uint32                   `json:"template_id"`       // 讨伐链目录中的稳定编号。
	DisplayName  string                   `json:"display_name"`      // 招募时显示的名称。
	MinimumLevel uint32                   `json:"minimum_level"`     // 各真人都必须达到的等级。
	Map          MapTemplate              `json:"map"`               // 四段共用的独立挑战地图。
	DurationMS   int64                    `json:"duration_ms"`       // 候选总时长，以毫秒装配。
	Stages       []DungeonStageDefinition `json:"stages"`            // 原工作簿要求的四段顺序。
	Rewards      *DungeonRewards          `json:"rewards,omitempty"` // 已配置的独立通关抽奖候选。
}

// DungeonEnemy 是运行时读取的原怪物数值与出生格快照。
type DungeonEnemy struct {
	Monster Monster // 已解析的挑战怪物，普通收益及复活均关闭。
	Spawn   Point   // 当前小区出生格。
}

// DungeonStage 是校验通过的小区、玩家出生和怪物阵容。
type DungeonStage struct {
	Kind         DungeonStageKind // 原工作簿指定的段类型。
	Area         DungeonArea      // 本段战斗使用的小区。
	PlayerSpawns []Point          // 三个互异且可达的落点。
	Enemies      []DungeonEnemy   // 原顺序保留，精英或最终段的首位为主怪。
}

// DungeonConfig 提供招募、原次数事务和挑战场次共用的配置快照。
type DungeonConfig struct {
	TemplateID   uint32          // 讨伐链目录编号。
	DisplayName  string          // 客户端显示名称。
	MinimumLevel uint32          // 每位真人的准入等级。
	Map          MapTemplate     // 独立的场景底图。
	Duration     time.Duration   // 整场原时长，不按战斗段重置。
	Stages       []DungeonStage  // 在同一地图内顺序开放的四段。
	Rewards      *DungeonRewards // 装配时冻结的通关奖表，缺失时不发放附件。
}

// DungeonTemplateByID 返回显式开启且已校验的模板快照。
func (f *Fixture) DungeonTemplateByID(id uint32) (*DungeonConfig, bool) {
	for index := range f.Dungeons {
		if f.Dungeons[index].TemplateID == id {
			return &f.Dungeons[index], true
		}
	}
	return nil, false
}

// validateDungeonReleases 排除目录冲突后一次替换已装配快照，空目录关闭讨伐链。
func (f *Fixture) validateDungeonReleases() error {
	f.Dungeons = nil
	var configs []DungeonConfig              // 只在整批通过后安装。
	templateIDs := make(map[uint32]struct{}) // 已获准模板编号，用于阻止歧义。
	mapIDs := make(map[uint64]struct{})      // 其它挑战及本批模板已占用的地图编号。
	if f.PersonalBoss != nil {
		mapIDs[f.PersonalBoss.Map.ID] = struct{}{}
	}
	for _, personal := range f.PersonalBossVariants {
		mapIDs[personal.Map.ID] = struct{}{}
	}
	if f.WorldBoss != nil {
		mapIDs[f.WorldBoss.Map.ID] = struct{}{}
	}
	for _, world := range f.WorldBossVariants {
		mapIDs[world.Map.ID] = struct{}{}
	}
	for _, release := range f.DungeonReleases {
		if !release.Enabled {
			continue
		}
		_, duplicateTemplate := templateIDs[release.TemplateID]
		_, duplicateMap := mapIDs[release.Map.ID]
		_, ordinaryMap := f.MapByID(release.Map.ID)
		if duplicateTemplate || duplicateMap || ordinaryMap {
			return fmt.Errorf("讨伐链模板 %d 的模板或地图编号已被使用", release.TemplateID)
		}
		config, err := f.assembleDungeon(release)
		if err != nil {
			return fmt.Errorf("讨伐链模板 %d：%w", release.TemplateID, err)
		}
		templateIDs[config.TemplateID] = struct{}{}
		mapIDs[config.Map.ID] = struct{}{}
		configs = append(configs, config)
	}
	f.Dungeons = configs
	return nil
}

// assembleDungeon 把原声明的四段转成不会引用候选切片的运行态投影。
func (f *Fixture) assembleDungeon(release DungeonDefinition) (DungeonConfig, error) {
	kinds := [...]DungeonStageKind{DungeonStageMobsFirst, DungeonStageMobsSecond, DungeonStageElite, DungeonStageFinalBoss}
	if release.TemplateID == 0 || strings.TrimSpace(release.DisplayName) == "" || release.MinimumLevel == 0 ||
		release.DurationMS < 1_000 || release.DurationMS > int64(24*time.Hour/time.Millisecond) || len(release.Stages) != len(kinds) ||
		!validDungeonMap(&release.Map) {
		return DungeonConfig{}, fmt.Errorf("目录、时长、地图或四段数量不合法")
	}
	if !f.ValidDungeonRewards(release.Rewards) {
		return DungeonConfig{}, fmt.Errorf("讨伐链奖励的选项、权重、物品或绑定策略不合法")
	}
	config := DungeonConfig{TemplateID: release.TemplateID, DisplayName: release.DisplayName, MinimumLevel: release.MinimumLevel,
		Map: cloneDungeonMap(release.Map), Duration: time.Duration(release.DurationMS) * time.Millisecond, Rewards: release.Rewards.clone()}
	var previous map[Point]struct{} // 前一段从原落点能走到的格子。
	for index, definition := range release.Stages {
		if definition.Kind != kinds[index] || !config.Map.Contains(definition.Area.Min) || !config.Map.Contains(definition.Area.Max) ||
			!definition.Area.Contains(definition.Area.Min) || len(definition.PlayerSpawns) != DungeonMaxPlayers || len(definition.Enemies) == 0 {
			return DungeonConfig{}, fmt.Errorf("第 %d 段的顺序、范围、落点或怪物数量不合法", index+1)
		}
		for _, prior := range config.Stages {
			if overlapDungeonAreas(prior.Area, definition.Area) {
				return DungeonConfig{}, fmt.Errorf("第 %d 段与前面的小区重叠", index+1)
			}
		}
		stage, reachable, err := f.assembleDungeonStage(&config.Map, definition)
		if err != nil {
			return DungeonConfig{}, fmt.Errorf("第 %d 段：%w", index+1, err)
		}
		if index > 0 && !adjacentDungeonAreas(previous, reachable) {
			return DungeonConfig{}, fmt.Errorf("第 %d 段与上一段没有可通行的相邻边", index+1)
		}
		config.Stages = append(config.Stages, stage)
		previous = reachable
	}
	config.Map.PlayerSpawn = config.Stages[0].PlayerSpawns[0]
	return config, nil
}

// assembleDungeonStage 确认落点、怪物和可达偏移。实体的位置只来自这一处配置。
func (f *Fixture) assembleDungeonStage(m *MapTemplate, def DungeonStageDefinition) (DungeonStage, map[Point]struct{}, error) {
	reachable := dungeonAreaReachable(m, def.Area, def.PlayerSpawns[0])
	occupied := make(map[Point]struct{}, len(def.PlayerSpawns)+len(def.Enemies)) // 入场时不能共用出生格。
	reserve := func(point Point) bool {
		if _, ok := reachable[point]; !ok {
			return false
		}
		if _, duplicate := occupied[point]; duplicate {
			return false
		}
		occupied[point] = struct{}{}
		return true
	}
	for _, point := range def.PlayerSpawns {
		if !reserve(point) {
			return DungeonStage{}, nil, fmt.Errorf("玩家落点不可达或重复")
		}
	}
	stage := DungeonStage{Kind: def.Kind, Area: def.Area, PlayerSpawns: slices.Clone(def.PlayerSpawns)}
	for _, enemy := range def.Enemies {
		monster, known := f.ChallengeMonster(enemy.MonsterID)
		if !reserve(enemy.Spawn) || !known || !validChallengeMonster(monster) || monster.Exp != 0 || monster.GoldMin != 0 ||
			monster.GoldMax != 0 || monster.RespawnMS != 0 || monster.DropPoolID != 0 || monster.CanTame {
			return DungeonStage{}, nil, fmt.Errorf("怪物 %d 不属于讨伐链挑战目录，或出生、普通收益和复活不合法", enemy.MonsterID)
		}
		stage.Enemies = append(stage.Enemies, DungeonEnemy{Monster: monster, Spawn: enemy.Spawn})
	}
	return stage, reachable, nil
}

// validDungeonMap 拒绝普通刷怪、城镇或其它特殊场景的配置混入同一地图。
func validDungeonMap(m *MapTemplate) bool {
	if m.ID == 0 || m.ID > math.MaxInt64 || m.SceneKind != SceneKindDungeon || m.CellBudget() == 0 ||
		m.Width > maxPersonalGridSide || m.Height > maxPersonalGridSide || m.CompactTown || len(m.CollisionBits) != 0 ||
		len(m.NPCs) != 0 || len(m.SafeZones) != 0 || len(m.LegacySafeZoneIDs) != 0 || len(m.MonsterSpawns) != 0 ||
		len(m.MonsterSpawnIDs) != 0 || m.InitialMonsterCount != 0 {
		return false
	}
	blocked := make(map[Point]struct{}, len(m.Blocked)) // 静态阻挡必须是唯一地图格。
	for _, point := range m.Blocked {
		if _, duplicate := blocked[point]; duplicate || !m.Contains(point) {
			return false
		}
		blocked[point] = struct{}{}
	}
	return true
}

// cloneDungeonMap 让运行装配拥有独立的地形及来源字段。
func cloneDungeonMap(m MapTemplate) MapTemplate {
	m.Blocked = slices.Clone(m.Blocked)
	m.SourceStaticBits = slices.Clone(m.SourceStaticBits)
	if m.NoTeleport != nil {
		value := *m.NoTeleport
		m.NoTeleport = &value
	}
	if m.NoPets != nil {
		value := *m.NoPets
		m.NoPets = &value
	}
	if m.Source != nil {
		value := *m.Source
		m.Source = &value
	}
	return m
}
