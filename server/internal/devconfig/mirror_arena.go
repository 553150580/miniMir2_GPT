// 本文件装配异步镜像竞技的赛季规则、候选匹配范围和独立战斗地图。
package devconfig

import (
	"errors"
	"regexp"
	"slices"
	"strings"
	"time"
)

const (
	// MirrorArenaMatchSameJobLevelWindow 仅在明确的等级差内测试同职业镜像。
	MirrorArenaMatchSameJobLevelWindow = "same_job_level_window"
	// MirrorArenaMatchCrossJobLevelWindow 允许等级差内的任意职业，供三职业对照。
	MirrorArenaMatchCrossJobLevelWindow = "cross_job_level_window"
	// MirrorArenaTimeoutEffectiveHPRatio 比较冻结阵容的有效存活生命比例，同值为防守胜。
	MirrorArenaTimeoutEffectiveHPRatio = "effective_surviving_hp_ratio_defender_wins_tie"
	// MirrorArenaPetFollowsOwnerDeath 沿用普通战斗中主人阵亡时清除其参战宠物的规则。
	MirrorArenaPetFollowsOwnerDeath = "owner_death_ends_pet"
	// MirrorArenaPetFightsAfterOwnerDeath 允许尚存活的原参战宠物继续完成镜像战斗。
	MirrorArenaPetFightsAfterOwnerDeath = "surviving_pet_continues"
	// MirrorArenaReadyTimeout 限定挑战场景等待客户端首帧确认的时间。
	MirrorArenaReadyTimeout = 10 * time.Second
)

var (
	mirrorArenaShanghai = time.FixedZone("Asia/Shanghai", 8*60*60) // 当前上海运营日固定 UTC+8。
	mirrorArenaResource = regexp.MustCompile(`^development/maps/[A-Za-z0-9_-]+(?:/[A-Za-z0-9_-]+)*$`)
)

// MirrorArenaStreakPoints 保存连续进攻胜利达到某一档时的本场得分。
type MirrorArenaStreakPoints struct {
	Streak uint32 `json:"streak"` // 至少连续获胜的场数。
	Points int64  `json:"points"` // 本次胜利采用的竞技积分。
}

// MirrorArenaDefinition 保存唯一工作簿投影及本轮显式选择的隔离启动参数。
type MirrorArenaDefinition struct {
	Enabled             bool                      `json:"enabled"`               // 明确启用本开发候选。
	TemplateID          uint32                    `json:"template_id"`           // 本独立战斗模板的编号。
	DisplayName         string                    `json:"display_name"`          // 面向挑战者展示的名称。
	Map                 MapTemplate               `json:"map"`                   // 不含普通刷怪、城镇或跨图服务的网格。
	AttackerSpawn       Point                     `json:"attacker_spawn"`        // 进攻角色的固定落点。
	DefenderSpawn       Point                     `json:"defender_spawn"`        // 防守镜像的固定落点。
	AttackerPetSpawn    Point                     `json:"attacker_pet_spawn"`    // 存在进攻宠物时的固定落点。
	DefenderPetSpawn    Point                     `json:"defender_pet_spawn"`    // 存在防守宠物时的固定落点。
	SeasonAnchor        time.Time                 `json:"season_anchor"`         // 明确选择的首个上海运营日零点。
	SeasonDays          uint32                    `json:"season_days"`           // 每个赛季持续的完整运营日数。
	DailyLimit          uint32                    `json:"daily_limit"`           // 每角色每运营日的挑战次数上限。
	OpponentLimit       uint32                    `json:"opponent_limit"`        // 每角色原运营日最多由系统分配的镜像数。
	AttackHistoryLimit  uint32                    `json:"attack_history_limit"`  // 进攻战报的展示条数上限。
	DefenseHistoryLimit uint32                    `json:"defense_history_limit"` // 被挑战战报的展示条数上限。
	DurationMS          int64                     `json:"duration_ms"`           // 从首帧同步许可后计算的候选战斗时限。
	WinBasePoints       int64                     `json:"win_base_points"`       // 普通进攻胜利的积分。
	LossPoints          int64                     `json:"loss_points"`           // 进攻失败并清除连胜后的积分。
	StreakPoints        []MirrorArenaStreakPoints `json:"streak_points"`         // 按连续获胜场数升序的得分档位。
	TimeoutPolicy       string                    `json:"timeout_policy"`        // 采用工作簿登记的原超时比较规则。
	MatchPolicy         string                    `json:"match_policy"`          // 隔离试测明确选择的一种职业范围。
	MaxLevelGap         uint32                    `json:"max_level_gap"`         // 隔离试测固定等级差，不推算综合战力。
	PetDeathPolicy      string                    `json:"pet_death_policy"`      // 隔离试测明确选择的主人阵亡后宠物规则。
}

// MirrorArenaConfig 冻结只读的地图、时限、赛季和积分范围供模拟与账本共用。
type MirrorArenaConfig struct {
	TemplateID          uint32                    // 稳定的挑战模板编号。
	DisplayName         string                    // 当前镜像模板的客户端名称。
	Map                 MapTemplate               // 经过碰撞和连通性验证的战斗地图。
	AttackerSpawn       Point                     // 进攻角色固定落点。
	DefenderSpawn       Point                     // 防守镜像固定落点。
	AttackerPetSpawn    Point                     // 进攻宠物固定落点。
	DefenderPetSpawn    Point                     // 防守宠物固定落点。
	SeasonAnchor        time.Time                 // 重复赛季序列的上海运营日起点。
	SeasonDays          uint32                    // 每赛季的完整运营日数。
	DailyLimit          uint32                    // 每日挑战次数上限。
	OpponentLimit       uint32                    // 原运营日的系统候选数上限。
	AttackHistoryLimit  uint32                    // 进攻战报展示的最大条数。
	DefenseHistoryLimit uint32                    // 防守战报展示的最大条数。
	Duration            time.Duration             // 单场首帧确认后可战斗的时长。
	WinBasePoints       int64                     // 进攻胜利基础得分。
	LossPoints          int64                     // 进攻失败得分。
	StreakPoints        []MirrorArenaStreakPoints // 连胜达到各档时的实际胜利积分。
	TimeoutPolicy       string                    // 超时结算依据。
	MatchPolicy         string                    // 当前隔离候选是否允许跨职业。
	MaxLevelGap         uint32                    // 当前候选允许的绝对等级差。
	PetDeathPolicy      string                    // 主体阵亡后已参战宠物如何处理。
}

// SeasonAt 返回包含服务端事件时刻的半开赛季；首个赛季开始前没有可参加的赛季。
func (c *MirrorArenaConfig) SeasonAt(at time.Time) (start, end time.Time, ok bool) {
	if c == nil || c.SeasonAnchor.IsZero() || c.SeasonDays == 0 || at.Before(c.SeasonAnchor) {
		return time.Time{}, time.Time{}, false
	}
	span := int64(c.SeasonDays) * 24 * 60 * 60
	anchor := c.SeasonAnchor.Unix()
	first := anchor + (at.Unix()-anchor)/span*span
	return time.Unix(first, 0).In(mirrorArenaShanghai), time.Unix(first+span, 0).In(mirrorArenaShanghai), true
}

// WinPoints 使用达到的最高连胜档；零连胜或没有适用档时使用基础胜利积分。
func (c *MirrorArenaConfig) WinPoints(streak uint32) int64 {
	points := c.WinBasePoints
	for _, tier := range c.StreakPoints {
		if tier.Streak > streak {
			break
		}
		points = tier.Points
	}
	return points
}

// AllowsMatch 按显式候选规则筛掉等级与职业范围之外的快照。
func (c *MirrorArenaConfig) AllowsMatch(attackerJob, attackerLevel, defenderJob, defenderLevel uint32) bool {
	if c == nil || attackerJob > 2 || defenderJob > 2 || attackerLevel == 0 || defenderLevel == 0 ||
		max(attackerLevel, defenderLevel)-min(attackerLevel, defenderLevel) > c.MaxLevelGap {
		return false
	}
	return c.MatchPolicy == MirrorArenaMatchCrossJobLevelWindow ||
		c.MatchPolicy == MirrorArenaMatchSameJobLevelWindow && attackerJob == defenderJob
}

// ValidateMirrorArena 拒绝任何未明确选择的开场、匹配、宠物、地图与座位规则。
func (f *Fixture) ValidateMirrorArena() error {
	f.MirrorArena = nil
	definition := f.MirrorArenaRelease
	if definition == nil || !definition.Enabled {
		return nil
	}
	anchor := definition.SeasonAnchor.In(mirrorArenaShanghai)
	if anchor.Before(time.Unix(0, 0)) || anchor.Hour() != 0 || anchor.Minute() != 0 || anchor.Second() != 0 || anchor.Nanosecond() != 0 ||
		definition.SeasonDays == 0 || definition.SeasonDays > 366 || definition.DurationMS < 1000 || definition.DurationMS > int64(time.Hour/time.Millisecond) {
		return errors.New("镜像竞技需要明确的上海零点、赛季天数与有限场次时长")
	}
	if definition.TimeoutPolicy != MirrorArenaTimeoutEffectiveHPRatio ||
		definition.MatchPolicy != MirrorArenaMatchSameJobLevelWindow && definition.MatchPolicy != MirrorArenaMatchCrossJobLevelWindow ||
		definition.PetDeathPolicy != MirrorArenaPetFollowsOwnerDeath && definition.PetDeathPolicy != MirrorArenaPetFightsAfterOwnerDeath {
		return errors.New("镜像竞技的超时、匹配或宠物阵亡规则不合法")
	}
	if definition.DailyLimit == 0 || definition.OpponentLimit == 0 || definition.OpponentLimit > 100 ||
		definition.AttackHistoryLimit == 0 || definition.AttackHistoryLimit > 100 || definition.DefenseHistoryLimit == 0 || definition.DefenseHistoryLimit > 100 ||
		!validMirrorArenaPoints(definition) {
		return errors.New("镜像竞技的每日次数、候选、历史展示或积分档不合法")
	}
	if definition.TemplateID == 0 || strings.TrimSpace(definition.DisplayName) == "" || !f.validMirrorArenaMap(definition) {
		return errors.New("镜像竞技缺少合法的独立地图、显示名称或连通落点")
	}
	grid := cloneDungeonMap(definition.Map)
	grid.PlayerSpawn, grid.PetSpawn = definition.AttackerSpawn, definition.AttackerPetSpawn
	noTeleport, noPets := true, false
	grid.NoTeleport, grid.NoPets = &noTeleport, &noPets
	f.MirrorArena = &MirrorArenaConfig{
		TemplateID: definition.TemplateID, DisplayName: strings.TrimSpace(definition.DisplayName), Map: grid,
		AttackerSpawn: definition.AttackerSpawn, DefenderSpawn: definition.DefenderSpawn,
		AttackerPetSpawn: definition.AttackerPetSpawn, DefenderPetSpawn: definition.DefenderPetSpawn,
		SeasonAnchor: anchor, SeasonDays: definition.SeasonDays,
		DailyLimit: definition.DailyLimit, OpponentLimit: definition.OpponentLimit,
		AttackHistoryLimit: definition.AttackHistoryLimit, DefenseHistoryLimit: definition.DefenseHistoryLimit,
		Duration: time.Duration(definition.DurationMS) * time.Millisecond, WinBasePoints: definition.WinBasePoints,
		LossPoints: definition.LossPoints, StreakPoints: slices.Clone(definition.StreakPoints), TimeoutPolicy: definition.TimeoutPolicy,
		MatchPolicy: definition.MatchPolicy, MaxLevelGap: definition.MaxLevelGap, PetDeathPolicy: definition.PetDeathPolicy,
	}
	return nil
}

// validMirrorArenaPoints 兼容明确配置的积分阶梯，禁止重复、倒序或倒扣档位。
func validMirrorArenaPoints(definition *MirrorArenaDefinition) bool {
	tiers := definition.StreakPoints
	if definition.WinBasePoints <= 0 || definition.LossPoints < 0 || len(tiers) == 0 || len(tiers) > 32 ||
		tiers[0].Streak != 1 || tiers[0].Points != definition.WinBasePoints {
		return false
	}
	for index := 1; index < len(tiers); index++ {
		if tiers[index].Streak <= tiers[index-1].Streak || tiers[index].Points < tiers[index-1].Points {
			return false
		}
	}
	return true
}

// validMirrorArenaMap 保护各挑战的原地图身份并校验双方落点能沿合法网格交战。
func (f *Fixture) validMirrorArenaMap(definition *MirrorArenaDefinition) bool {
	grid := &definition.Map
	if !validChallengeGrid(grid, SceneKindMirrorArena) || strings.TrimSpace(grid.DisplayName) == "" || !mirrorArenaResource.MatchString(grid.ResourceKey) ||
		grid.Source != nil || len(grid.SourceStaticBits) != 0 || grid.NoPets != nil && *grid.NoPets || grid.NoTeleport != nil && !*grid.NoTeleport {
		return false
	}
	_, ordinary := f.MapByID(grid.ID)
	if ordinary || f.PersonalBoss != nil && f.PersonalBoss.Map.ID == grid.ID || f.WorldBoss != nil && f.WorldBoss.Map.ID == grid.ID ||
		slices.ContainsFunc(f.PersonalBossVariants, func(cfg PersonalBossConfig) bool { return cfg.Map.ID == grid.ID }) ||
		slices.ContainsFunc(f.WorldBossVariants, func(cfg WorldBossConfig) bool { return cfg.Map.ID == grid.ID }) ||
		slices.ContainsFunc(f.Dungeons, func(cfg DungeonConfig) bool { return cfg.Map.ID == grid.ID }) {
		return false
	}
	area := DungeonArea{Max: Point{X: grid.Width - 1, Y: grid.Height - 1}}
	reachable := dungeonAreaReachable(grid, area, definition.AttackerSpawn)
	occupied := make(map[Point]struct{}, 4)
	for _, spawn := range [...]Point{definition.AttackerSpawn, definition.DefenderSpawn, definition.AttackerPetSpawn, definition.DefenderPetSpawn} {
		_, connected := reachable[spawn]
		if _, duplicate := occupied[spawn]; !connected || duplicate {
			return false
		}
		occupied[spawn] = struct{}{}
	}
	return true
}
