// 本文件限定世界首领的独立场次地图、上海运营时刻和开发装配预算。
package devconfig

import (
	"cmp"
	"errors"
	"math"
	"slices"
	"strings"
	"time"
)

// WorldBossDefinition 保存唯一配置来源使用的稳定编号、场次和相对毫秒。
type WorldBossDefinition struct {
	Enabled            bool        `json:"enabled"`               // 显式开放本开发候选。
	TemplateID         uint32      `json:"template_id"`           // 唯一世界首领模板编号。
	DisplayName        string      `json:"display_name"`          // 客户端显示名称。
	Map                MapTemplate `json:"map"`                   // 独立共享网格地图。
	MonsterID          uint32      `json:"monster_id"`            // 关联独立挑战怪物目录。
	MinimumLevel       uint32      `json:"minimum_level"`         // 最低参与等级。
	DailyOpenOffsetsMS []int64     `json:"daily_open_offsets_ms"` // 上海运营日内开场毫秒列表。
	WindowMS           int64       `json:"window_ms"`             // 场次有效窗口。
	ReturnDelayMS      int64       `json:"return_delay_ms"`       // 击杀/逃走后展示终态再返回的时间。
	ParticipantLimit   int         `json:"participant_limit"`     // 每运行服单场有效参与人数。

	Rewards *WorldBossRewards `json:"rewards,omitempty"` // 仅有独立来源且显式装配时开放奖励。
}

// WorldBossConfig 冻结每个运行服使用的一个候选首领模板和场次日程。
type WorldBossConfig struct {
	TemplateID       uint32          // 共享首领模板的稳定目录编号。
	DisplayName      string          // 从原配置读取的展示名称。
	MinimumLevel     uint32          // 获准进入本场的最低等级。
	Map              MapTemplate     // 玩家和唯一共享首领共用的可走网格。
	Monster          Monster         // 权威怪物实体的属性和表现定义。
	DailyOpenOffsets []time.Duration // 上海运营日内的开场偏移，严格递增且窗口不重叠。
	Window           time.Duration   // 每场从真实开场起生效的可参加时长。
	ReturnDelay      time.Duration   // 击杀和到期时终态展示后返回原挂机图的时间。
	ParticipantLimit int             // 计入本场终生参战名额，离开后仍保留原归属贡献。

	Rewards *WorldBossRewards // 本模板在装载时冻结的参与、排名和超时奖励。
}

// validateWorldBossRelease 解析独立怪物和归一化日程，拒绝未校验的发布模板。
func (f *Fixture) validateWorldBossRelease() error {
	def := f.WorldBossRelease
	if def == nil && len(f.WorldBossVariantReleases) == 0 {
		return nil
	}
	f.WorldBoss = nil
	f.WorldBossVariants = nil
	if def == nil || !def.Enabled {
		if len(f.WorldBossVariantReleases) > 0 {
			return errors.New("世界首领轮换需要启用的主目录")
		}
		return nil
	}
	primary, err := f.parseWorldBossDefinition(*def)
	if err != nil {
		return err
	}
	f.WorldBoss = &primary
	templates := map[uint32]bool{primary.TemplateID: true}
	maps := map[uint64]bool{primary.Map.ID: true}
	for _, definition := range f.WorldBossVariantReleases {
		candidate, err := f.parseWorldBossDefinition(definition)
		if err != nil {
			return err
		}
		if templates[candidate.TemplateID] || maps[candidate.Map.ID] {
			return errors.New("世界首领的模板和地图编号必须唯一")
		}
		templates[candidate.TemplateID], maps[candidate.Map.ID] = true, true
		f.WorldBossVariants = append(f.WorldBossVariants, candidate)
	}
	return f.validateWorldBossRotation()
}

// parseWorldBossDefinition 解析独立模板并保留只属于该模板的各个开场时间。
func (f *Fixture) parseWorldBossDefinition(def WorldBossDefinition) (WorldBossConfig, error) {
	const dayMS = int64(24 * time.Hour / time.Millisecond)
	monster, ok := f.ChallengeMonster(def.MonsterID)
	if !def.Enabled || !ok || def.WindowMS <= 0 || def.WindowMS >= dayMS || def.ReturnDelayMS < 0 || def.ReturnDelayMS >= dayMS ||
		strings.TrimSpace(def.DisplayName) == "" || def.MinimumLevel == 0 {
		return WorldBossConfig{}, errors.New("世界首领开发候选的怪物、时段、场次或等级无效")
	}
	offsets := make([]time.Duration, len(def.DailyOpenOffsetsMS))
	for i, ms := range def.DailyOpenOffsetsMS {
		if ms < 0 || ms >= dayMS {
			return WorldBossConfig{}, errors.New("世界首领开场时间超出运营日")
		}
		offsets[i] = time.Duration(ms) * time.Millisecond
	}
	candidate := WorldBossConfig{TemplateID: def.TemplateID, DisplayName: def.DisplayName, MinimumLevel: def.MinimumLevel, Map: def.Map, Monster: monster,
		DailyOpenOffsets: offsets, Window: time.Duration(def.WindowMS) * time.Millisecond, ReturnDelay: time.Duration(def.ReturnDelayMS) * time.Millisecond,
		ParticipantLimit: def.ParticipantLimit, Rewards: def.Rewards.clone()}
	if !f.ValidWorldBossRewards(&candidate) {
		return WorldBossConfig{}, errors.New("世界首领的贡献门槛、排名档或物品奖励无效")
	}
	if !f.validWorldBoss(&candidate) {
		return WorldBossConfig{}, errors.New("世界首领独立地图或窗口配置无效")
	}
	return candidate, nil
}

// ValidWorldBoss 校验共享模板、单个怪物、独立地图以及不可相互跨越的日程。
func (f *Fixture) ValidWorldBoss() bool {
	return f.validWorldBoss(f.WorldBoss) && f.ValidWorldBossRewards(f.WorldBoss)
}

// validWorldBoss 校验单个模板，同时排除已用于个人首领的地图。
func (f *Fixture) validWorldBoss(cfg *WorldBossConfig) bool {
	if cfg == nil || cfg.TemplateID == 0 || cfg.Map.ID == 0 || cfg.Map.ID > math.MaxInt64 || cfg.Map.SceneKind != SceneKindWorldBoss ||
		cfg.Map.CellBudget() <= 0 || cfg.Map.Width > maxPersonalGridSide || cfg.Map.Height > maxPersonalGridSide || len(cfg.Map.MonsterSpawns) != 1 ||
		cfg.Map.MonsterSpawns[0] == cfg.Map.PlayerSpawn || !cfg.Map.Walkable(cfg.Map.PlayerSpawn) || !cfg.Map.Walkable(cfg.Map.MonsterSpawns[0]) ||
		cfg.Window < time.Second || cfg.Window >= 24*time.Hour || cfg.Window%time.Millisecond != 0 || cfg.ReturnDelay < 0 ||
		cfg.ReturnDelay >= cfg.Window || cfg.ParticipantLimit < 2 || cfg.ParticipantLimit >= cfg.Map.CellBudget() ||
		len(cfg.DailyOpenOffsets) == 0 || cfg.Map.CompactTown || len(cfg.Map.SafeZones) != 0 || len(cfg.Map.NPCs) != 0 || len(cfg.Map.LegacySafeZoneIDs) != 0 ||
		!validChallengeMonster(cfg.Monster) || cfg.Monster.Exp != 0 || cfg.Monster.GoldMin != 0 || cfg.Monster.GoldMax != 0 || cfg.Monster.RespawnMS != 0 ||
		!personalBossSpawnsConnected(&cfg.Map) {
		return false
	}
	if _, exists := f.MapByID(cfg.Map.ID); exists || f.PersonalBoss != nil && f.PersonalBoss.Map.ID == cfg.Map.ID {
		return false
	}
	for _, personal := range f.PersonalBossVariants {
		if personal.Map.ID == cfg.Map.ID {
			return false
		}
	}
	for i, offset := range cfg.DailyOpenOffsets {
		if offset < 0 || offset >= 24*time.Hour || offset%time.Millisecond != 0 || i > 0 && offset < cfg.DailyOpenOffsets[i-1]+cfg.Window+cfg.ReturnDelay {
			return false
		}
	}
	first, last := cfg.DailyOpenOffsets[0], cfg.DailyOpenOffsets[len(cfg.DailyOpenOffsets)-1]
	return 24*time.Hour+first-last >= cfg.Window+cfg.ReturnDelay
}

// validateWorldBossRotation 拒绝任意模板之间的重复时段或终态展示重叠。
func (f *Fixture) validateWorldBossRotation() error {
	type period struct{ start, end time.Duration }
	var windows []period
	for _, entry := range append([]WorldBossConfig{*f.WorldBoss}, f.WorldBossVariants...) {
		for _, offset := range entry.DailyOpenOffsets {
			windows = append(windows, period{offset, offset + entry.Window + entry.ReturnDelay})
		}
	}
	slices.SortFunc(windows, func(a, b period) int { return cmp.Compare(a.start, b.start) })
	for i, value := range windows {
		next := windows[(i+1)%len(windows)].start
		if i == len(windows)-1 {
			next += 24 * time.Hour
		}
		if next < value.end {
			return errors.New("世界首领轮换时段重叠")
		}
	}
	return nil
}
