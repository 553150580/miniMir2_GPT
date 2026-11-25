// 本文件解析挑战事实的来源，供配置装载和结算在相同的过滤条件下使用。
package devconfig

import "slices"

// MatchesChallengeTaskEvent 判断一项任务是否消费指定挑战事实。
// 图鉴的地图仅用于分组。同一已登记怪物在不同地图的合格击杀均计入图鉴。
func (d TaskCodexDefinition) MatchesChallengeTaskEvent(event string, templateID uint32, mapID uint64, monsterID uint32) bool {
	if d.TemplateID != 0 && d.TemplateID != templateID || d.MonsterID != 0 && d.MonsterID != monsterID ||
		d.Kind != TaskCodexMonster && d.MapID != 0 && d.MapID != mapID {
		return false
	}
	return d.Event == event || event == TaskCodexChallengeMonsterKill && d.Event == TaskCodexMonsterKill && (d.MonsterID != 0 || d.MapID != 0)
}

// HasWorldBossTaskProgress 判断当前定义是否可能持有历史世界计数；关闭场次或奖表不撤销已确认进度。
func (f *Fixture) HasWorldBossTaskProgress() bool {
	if f.TasksCodex == nil {
		return false
	}
	return slices.ContainsFunc(f.TasksCodex.Definitions, func(d TaskCodexDefinition) bool {
		switch d.Event {
		case TaskCodexWorldBossKill, TaskCodexChallengeMonsterKill:
			return true
		case TaskCodexMonsterKill:
			if d.MonsterID != 0 {
				return !slices.ContainsFunc(f.Monsters, func(m Monster) bool { return m.ID == d.MonsterID })
			}
			_, ordinary := f.MapByID(d.MapID)
			return d.MapID != 0 && !ordinary
		default:
			return false
		}
	})
}

// HasWorldBossTaskTargets 在终态事务前排除未被任何目标消费的世界首领事实。
func (f *Fixture) HasWorldBossTaskTargets(templateID uint32, mapID uint64, monsterID uint32) bool {
	return f.hasChallengeTaskEvent(TaskCodexWorldBossKill, templateID, mapID, 0) ||
		f.hasChallengeTaskEvent(TaskCodexChallengeMonsterKill, templateID, mapID, monsterID)
}

// HasDungeonTaskTargets 判断模板是否有待提交的成就、每日目标或实际击杀事实。
func (f *Fixture) HasDungeonTaskTargets(cfg *DungeonConfig) bool {
	if f.TasksCodex == nil {
		return false
	}
	if f.hasChallengeTaskEvent(TaskCodexDungeonClear, cfg.TemplateID, cfg.Map.ID, 0) {
		return true
	}
	for _, stage := range cfg.Stages {
		for _, enemy := range stage.Enemies {
			if f.hasChallengeTaskEvent(TaskCodexChallengeMonsterKill, cfg.TemplateID, cfg.Map.ID, enemy.Monster.ID) {
				return true
			}
		}
	}
	return false
}

func (f *Fixture) hasChallengeTaskEvent(event string, templateID uint32, mapID uint64, monsterID uint32) bool {
	return f.TasksCodex != nil && slices.ContainsFunc(f.TasksCodex.Definitions, func(d TaskCodexDefinition) bool {
		return d.MatchesChallengeTaskEvent(event, templateID, mapID, monsterID)
	})
}

func (f *Fixture) hasWorldBossTaskTemplate(match func(WorldBossConfig) bool) bool {
	qualified := func(cfg WorldBossConfig) bool { return cfg.Rewards != nil && match(cfg) }
	return f.WorldBoss != nil && qualified(*f.WorldBoss) || slices.ContainsFunc(f.WorldBossVariants, qualified)
}

// validTaskCodexSource 要求事件可由实际开启的场景产生，世界首领另需明确的参与资格。
func (f *Fixture) validTaskCodexSource(d TaskCodexDefinition) bool {
	validScope := func(templateID uint32, mapID uint64) bool {
		return (d.TemplateID == 0 || d.TemplateID == templateID) && (d.MapID == 0 || d.MapID == mapID)
	}
	validMonster := func(id uint32) bool { return d.MonsterID == 0 || d.MonsterID == id }
	personal := func(cfg PersonalBossConfig) bool {
		return validScope(cfg.TemplateID, cfg.Map.ID) && validMonster(cfg.Monster.ID)
	}
	world := func(cfg WorldBossConfig) bool {
		return validScope(cfg.TemplateID, cfg.Map.ID) && validMonster(cfg.Monster.ID)
	}
	switch d.Event {
	case "", TaskCodexItemRecycle, TaskCodexSkillLearn, TaskCodexLevelReached:
		return (d.Event == "") == (d.Kind == TaskCodexActivity) && d.MonsterID == 0 && d.MapID == 0 && d.TemplateID == 0
	case TaskCodexPersonalBossClear:
		return d.MonsterID == 0 && (f.PersonalBoss != nil && personal(*f.PersonalBoss) || slices.ContainsFunc(f.PersonalBossVariants, personal))
	case TaskCodexWorldBossKill:
		return d.MonsterID == 0 && f.hasWorldBossTaskTemplate(world)
	case TaskCodexDungeonClear:
		return d.MonsterID == 0 && slices.ContainsFunc(f.Dungeons, func(cfg DungeonConfig) bool {
			return validScope(cfg.TemplateID, cfg.Map.ID)
		})
	case TaskCodexMonsterKill, TaskCodexChallengeMonsterKill:
		if d.TemplateID != 0 {
			return false
		}
		if d.Event == TaskCodexMonsterKill {
			_, knownMap := f.MapByID(d.MapID)
			if (d.MapID == 0 || knownMap) && (d.MonsterID == 0 || slices.ContainsFunc(f.Monsters, func(m Monster) bool { return m.ID == d.MonsterID })) {
				return true
			}
		}
		if f.PersonalBoss != nil && personal(*f.PersonalBoss) || slices.ContainsFunc(f.PersonalBossVariants, personal) || f.hasWorldBossTaskTemplate(world) {
			return true
		}
		for _, cfg := range f.Dungeons {
			if !validScope(cfg.TemplateID, cfg.Map.ID) {
				continue
			}
			for _, stage := range cfg.Stages {
				if slices.ContainsFunc(stage.Enemies, func(enemy DungeonEnemy) bool { return validMonster(enemy.Monster.ID) }) {
					return true
				}
			}
		}
	}
	return false
}
