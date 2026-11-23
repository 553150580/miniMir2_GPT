// 本文件限定个人首领挑战的开发装配和其独立格子地图的配置门禁。
package devconfig

import (
	"math"
	"time"
)

// PersonalBossConfig 是开发环境可显式注入的一个完整挑战模板。
type PersonalBossConfig struct {
	TemplateID      uint32        // 原挑战档位的稳定编号。
	Map             MapTemplate   // 只由挑战场景使用的独立逻辑网格。
	Monster         Monster       // 格子战斗使用的怪物属性与表现定义。
	Duration        time.Duration // 从持久批准起计算的实际挑战时限。
	SuccessCooldown time.Duration // 击杀后重复挑战同档位的等待时间。
	RetryCooldown   time.Duration // 未击杀后重复挑战同档位的等待时间。
}

// ValidPersonalBoss 检查独立地图、出生点、时限和核心战斗属性是否可运行。
func (f *Fixture) ValidPersonalBoss() bool {
	cfg := f.PersonalBoss
	if cfg == nil || cfg.TemplateID == 0 || cfg.Map.ID == 0 || cfg.Map.ID > math.MaxInt64 || cfg.Map.SceneKind != SceneKindPersonalBoss ||
		cfg.Map.CellBudget() <= 0 || cfg.Map.CellBudget() > MaxPersonalGridCells || len(cfg.Map.MonsterSpawns) != 1 ||
		cfg.Map.MonsterSpawns[0] == cfg.Map.PlayerSpawn || !cfg.Map.Walkable(cfg.Map.PlayerSpawn) || !cfg.Map.Walkable(cfg.Map.MonsterSpawns[0]) ||
		cfg.Duration < time.Second || cfg.Duration > 24*time.Hour || cfg.SuccessCooldown <= 0 || cfg.RetryCooldown <= 0 ||
		cfg.Duration%time.Millisecond != 0 || cfg.SuccessCooldown%time.Millisecond != 0 || cfg.RetryCooldown%time.Millisecond != 0 ||
		cfg.SuccessCooldown > 24*time.Hour || cfg.RetryCooldown > 24*time.Hour || cfg.Monster.ID == 0 || cfg.Monster.HP <= 0 ||
		cfg.Monster.MoveMS <= 0 || cfg.Monster.AttackMS <= 0 || cfg.Monster.AttackMin < 0 || cfg.Monster.AttackMax < cfg.Monster.AttackMin ||
		cfg.Monster.DefenceMin < 0 || cfg.Monster.DefenceMax < cfg.Monster.DefenceMin || cfg.Monster.Agility < 0 || cfg.Monster.Agility == math.MaxInt64 ||
		len(cfg.Map.SafeZones) != 0 || len(cfg.Map.NPCs) != 0 {
		return false
	}
	_, exists := f.MapByID(cfg.Map.ID)
	return !exists
}
