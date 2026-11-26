// 本文件解析开发发布的个人首领配置，并校验独立格子地图与怪物来源。
package devconfig

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"time"
)

// PersonalBossDefinition 保存唯一工作簿投影下来的个人挑战、地图和资格。
type PersonalBossDefinition struct {
	Enabled               bool                 `json:"enabled"`                    // 仅显式开放的候选可预约挑战。
	TemplateID            uint32               `json:"template_id"`                // 原模板唯一编号和冷却键。
	DisplayName           string               `json:"display_name"`               // 界面使用的挑战名称。
	Map                   MapTemplate          `json:"map"`                        // 独享挑战地图，绕开通用换图目录。
	MonsterID             uint32               `json:"monster_id"`                 // 特殊挑战怪物来源的稳定编号。
	MinimumLevel          uint32               `json:"minimum_level"`              // 开始挑战的最低角色等级。
	DurationMS            int64                `json:"duration_ms"`                // 未击杀时首领离开的时限。
	SuccessCooldownMS     int64                `json:"success_cooldown_ms"`        // 击杀后的同模板等待时间。
	RetryCooldownMS       int64                `json:"retry_cooldown_ms"`          // 未击杀后的同模板等待时间。
	DailySuccessLimit     uint32               `json:"daily_success_limit"`        // 每个上海运营日的基础成功次数。
	PrivilegeSuccessBonus uint32               `json:"privilege_success_bonus"`    // 入场时有效特权所增加的成功次数。
	HistoryLimit          uint32               `json:"history_limit"`              // 界面显示的最新场次上限。
	KillRewardGold        int64                `json:"kill_reward_gold,omitempty"` // 独立配置的击杀金币，零表示候选不产生金币。
	ItemReward            *ChallengeItemReward `json:"item_reward,omitempty"`      // 显式配置的胜利附件及待领容量。
}

// PersonalBossConfig 冻结已解析的独享挑战地图、原怪物定义和规则。
type PersonalBossConfig struct {
	TemplateID            uint32               // 原挑战档位的稳定编号。
	DisplayName           string               // 来自发布定义的挑战名称。
	Map                   MapTemplate          // 只由挑战场景使用的独立逻辑网格。
	Monster               Monster              // 格子战斗使用的怪物属性与表现定义。
	MinimumLevel          uint32               // 允许预约的最低角色等级。
	Duration              time.Duration        // 挑战实际时限。
	SuccessCooldown       time.Duration        // 击杀后重复挑战同档位的等待时间。
	RetryCooldown         time.Duration        // 未击杀后重复挑战同档位的等待时间。
	DailySuccessLimit     uint32               // 普通角色单运营日最大成功次数。
	PrivilegeSuccessBonus uint32               // 有效特权角色的次数加成。
	HistoryLimit          uint32               // 最多下发给当前观察者的场次记录。
	KillRewardGold        int64                // 独立的击杀金币产量，预约时固定。
	ItemReward            *ChallengeItemReward // 只有配置了附件的场次预留待领容量。
}

// AcceptsPersonalBossQualification 只校验已持久化的通关证明身份，预约仍需匹配当前完整配置。
func (f *Fixture) AcceptsPersonalBossQualification(rule uint64, hash string) bool {
	if f == nil || rule != f.RuleVersion || hash == "" {
		return false
	}
	if hash == f.Hash {
		return true
	}
	if rule != 30 {
		return false
	}
	// 三份已核对的规则 30 夹具依次新增双卡和道具市场，个人首领战斗输入相同。
	const beforeRoleCards = "4bb20bf3e8d235db1061e3223b8b28169f662d3e7e334291f6f65c09400acb1a"
	const withRoleCards = "6a421806cf5d45a8ead84bfda80be4a57eebcac00d3344140825fc860bfec9a6"
	const withItemMarket = "8cf30edfdf4db1cffbd2c0fe5688652a96496569f00aa19f26581f83a94fb266"
	switch f.Hash {
	case withRoleCards:
		return hash == beforeRoleCards
	case withItemMarket:
		return hash == beforeRoleCards || hash == withRoleCards
	default:
		return false
	}
}

// ChallengeMonster 只从独立挑战怪物目录解析编号。
func (f *Fixture) ChallengeMonster(id uint32) (Monster, bool) {
	for _, monster := range f.ChallengeMonsters {
		if monster.ID == id {
			return monster, true
		}
	}
	return Monster{}, false
}

// validatePersonalBossRelease 在装载时解析来源并拒绝非法开发候选。
func (f *Fixture) validatePersonalBossRelease() error {
	ordinary := make(map[uint32]bool, len(f.Monsters))
	for _, monster := range f.Monsters {
		ordinary[monster.ID] = true
	}
	seen := make(map[uint32]bool, len(f.ChallengeMonsters))
	for _, monster := range f.ChallengeMonsters {
		if !validChallengeMonster(monster) || seen[monster.ID] || ordinary[monster.ID] || monster.RespawnMS != 0 || monster.Exp != 0 || monster.GoldMin != 0 || monster.GoldMax != 0 {
			return fmt.Errorf("挑战怪物 %d 的来源、战斗参数、普通复活或独立奖励边界不合法", monster.ID)
		}
		seen[monster.ID] = true
	}
	def := f.PersonalBossRelease
	if def == nil && len(f.PersonalBossVariantReleases) == 0 {
		return nil
	}
	f.PersonalBoss = nil
	f.PersonalBossVariants = nil
	if def == nil || !def.Enabled {
		if len(f.PersonalBossVariantReleases) > 0 {
			return errors.New("个人首领选项需要启用的主目录")
		}
		return nil
	}
	primary, err := f.parsePersonalBossDefinition(*def)
	if err != nil {
		return err
	}
	f.PersonalBoss = &primary
	templates := map[uint32]bool{primary.TemplateID: true}
	maps := map[uint64]bool{primary.Map.ID: true}
	for _, definition := range f.PersonalBossVariantReleases {
		candidate, err := f.parsePersonalBossDefinition(definition)
		if err != nil {
			return err
		}
		if templates[candidate.TemplateID] || maps[candidate.Map.ID] || candidate.DailySuccessLimit != primary.DailySuccessLimit ||
			candidate.PrivilegeSuccessBonus != primary.PrivilegeSuccessBonus || candidate.HistoryLimit != primary.HistoryLimit {
			return errors.New("个人首领身份重复或同角色次数与历史规则不一致")
		}
		templates[candidate.TemplateID], maps[candidate.Map.ID] = true, true
		f.PersonalBossVariants = append(f.PersonalBossVariants, candidate)
	}
	return nil
}

// parsePersonalBossDefinition 解析和校验单个有明确来源的个人首领。
func (f *Fixture) parsePersonalBossDefinition(def PersonalBossDefinition) (PersonalBossConfig, error) {
	monster, found := f.ChallengeMonster(def.MonsterID)
	maxMS := int64(24 * time.Hour / time.Millisecond)
	if !def.Enabled || !found || def.DurationMS <= 0 || def.DurationMS > maxMS || def.SuccessCooldownMS <= 0 || def.SuccessCooldownMS > maxMS ||
		def.RetryCooldownMS <= 0 || def.RetryCooldownMS > maxMS || strings.TrimSpace(def.DisplayName) == "" || def.MinimumLevel == 0 {
		return PersonalBossConfig{}, errors.New("个人首领开发候选缺少有效的模板、资格或时限")
	}
	candidate := PersonalBossConfig{TemplateID: def.TemplateID, DisplayName: def.DisplayName, Map: def.Map, Monster: monster,
		MinimumLevel: def.MinimumLevel, Duration: time.Duration(def.DurationMS) * time.Millisecond,
		SuccessCooldown: time.Duration(def.SuccessCooldownMS) * time.Millisecond, RetryCooldown: time.Duration(def.RetryCooldownMS) * time.Millisecond,
		DailySuccessLimit: def.DailySuccessLimit, PrivilegeSuccessBonus: def.PrivilegeSuccessBonus, HistoryLimit: def.HistoryLimit,
		KillRewardGold: def.KillRewardGold, ItemReward: def.ItemReward}
	if !f.validPersonalBoss(&candidate) {
		return PersonalBossConfig{}, errors.New("个人首领开发候选的地图或角色次数规则不合法")
	}
	return candidate, nil
}

// ValidPersonalBoss 检查独立地图、出生点、时限和核心战斗属性是否可运行。
func (f *Fixture) ValidPersonalBoss() bool {
	return f.validPersonalBoss(f.PersonalBoss)
}

// PersonalBossByTemplate 返回按唯一模板定位、且已经通过独立地图验证的首领。
func (f *Fixture) PersonalBossByTemplate(id uint32) *PersonalBossConfig {
	if f.PersonalBoss != nil && f.PersonalBoss.TemplateID == id && f.ValidPersonalBoss() {
		return f.PersonalBoss
	}
	for i := range f.PersonalBossVariants {
		entry := &f.PersonalBossVariants[i]
		if entry.TemplateID == id {
			return entry
		}
	}
	return nil
}

// validPersonalBoss 校验单个模板的独立地图与当前角色共用约束。
func (f *Fixture) validPersonalBoss(cfg *PersonalBossConfig) bool {
	if cfg == nil || cfg.TemplateID == 0 || cfg.Map.ID == 0 || cfg.Map.ID > math.MaxInt64 || cfg.Map.SceneKind != SceneKindPersonalBoss ||
		cfg.Map.CellBudget() <= 0 || cfg.Map.Width > maxPersonalGridSide || cfg.Map.Height > maxPersonalGridSide || len(cfg.Map.MonsterSpawns) != 1 ||
		cfg.Map.MonsterSpawns[0] == cfg.Map.PlayerSpawn || !cfg.Map.Walkable(cfg.Map.PlayerSpawn) || !cfg.Map.Walkable(cfg.Map.MonsterSpawns[0]) ||
		cfg.Duration < time.Second || cfg.Duration > 24*time.Hour || cfg.SuccessCooldown <= 0 || cfg.RetryCooldown <= 0 ||
		cfg.Duration%time.Millisecond != 0 || cfg.SuccessCooldown%time.Millisecond != 0 || cfg.RetryCooldown%time.Millisecond != 0 ||
		cfg.SuccessCooldown > 24*time.Hour || cfg.RetryCooldown > 24*time.Hour || !validChallengeMonster(cfg.Monster) ||
		cfg.DailySuccessLimit == 0 || cfg.PrivilegeSuccessBonus > math.MaxUint32-cfg.DailySuccessLimit || cfg.HistoryLimit == 0 || cfg.HistoryLimit > 100 || cfg.KillRewardGold < 0 ||
		cfg.Map.CompactTown || len(cfg.Map.CollisionBits) != 0 || len(cfg.Map.SafeZones) != 0 || len(cfg.Map.LegacySafeZoneIDs) != 0 || len(cfg.Map.NPCs) != 0 ||
		!f.validChallengeItemReward(cfg.ItemReward) {
		return false
	}
	_, exists := f.MapByID(cfg.Map.ID)
	return !exists && personalBossSpawnsConnected(&cfg.Map)
}

// validChallengeMonster 拒绝无实体身份或超出格战数值边界的挑战怪物。
func validChallengeMonster(m Monster) bool {
	return m.ID != 0 && m.HP > 0 && m.HP < 1000000 && m.MoveMS >= 100 && m.AttackMS >= 100 &&
		m.AttackMin >= 0 && m.AttackMax >= m.AttackMin && m.AttackMax < 100000 && m.DefenceMin >= 0 &&
		m.DefenceMax >= m.DefenceMin && m.Agility >= 0 && m.Agility < math.MaxInt64
}

// personalBossSpawnsConnected 验证首领与玩家之间可以走通，避免候选图永远无法近战。
func personalBossSpawnsConnected(m *MapTemplate) bool {
	blocked := make(map[Point]bool, len(m.Blocked))
	for _, point := range m.Blocked {
		if !m.Contains(point) || blocked[point] {
			return false
		}
		blocked[point] = true
	}
	queue := []Point{m.PlayerSpawn}
	visited := map[Point]bool{m.PlayerSpawn: true}
	for step := 0; step < len(queue); step++ {
		cell := queue[step]
		if cell == m.MonsterSpawns[0] {
			return true
		}
		for _, next := range []Point{{X: cell.X + 1, Y: cell.Y}, {X: cell.X - 1, Y: cell.Y}, {X: cell.X, Y: cell.Y + 1}, {X: cell.X, Y: cell.Y - 1}} {
			if m.Contains(next) && !blocked[next] && !visited[next] {
				visited[next] = true
				queue = append(queue, next)
			}
		}
	}
	return false
}
