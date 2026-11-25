// 本文件校验用于任务目标、怪物图鉴和绑定奖励的运行时定义。
package devconfig

import (
	"fmt"
	"minimir/internal/wire"
	"strings"
)

const (
	TaskCodexMain        uint32 = 1 // 主线任务。
	TaskCodexDaily       uint32 = 2 // 每日任务。
	TaskCodexAchievement uint32 = 3 // 永久成就。
	TaskCodexMonster     uint32 = 4 // 怪物图鉴。
	TaskCodexActivity    uint32 = 5 // 当日活跃奖励。
)

const (
	TaskCodexMonsterKill          = "monster_kill"           // 已落账的野外击杀，或按编号指定的挑战怪物击杀。
	TaskCodexItemRecycle          = "item_recycle"           // 已完成的装备回收。
	TaskCodexSkillLearn           = "skill_learn"            // 已确认的新技能习得。
	TaskCodexLevelReached         = "level_reached"          // 角色已确认的等级。
	TaskCodexChallengeMonsterKill = "challenge_monster_kill" // 符合对应玩法资格的挑战怪物实际击杀。
	TaskCodexPersonalBossClear    = "personal_boss_clear"    // 已确认成功的个人首领场次。
	TaskCodexDungeonClear         = "dungeon_clear"          // 已确认通关的讨伐链场次。
	TaskCodexWorldBossKill        = "world_boss_kill"        // 世界首领击杀及该角色的有效参与。
)

// TaskCodexRules 只保存已经可执行的定义和奖励；缺少定义时功能保持未开放。
type TaskCodexRules struct {
	Definitions         []TaskCodexDefinition `json:"definitions"`                     // 以发布顺序展示的任务、成就和图鉴。
	DailyPoolSize       uint32                `json:"daily_pool_size,omitempty"`       // 唯一玩法配置中登记的每日任务池数量。
	DailyClaimLimit     uint32                `json:"daily_claim_limit,omitempty"`     // 单角色同运营日最多领取的每日任务数。
	DailyActivityTarget uint32                `json:"daily_activity_target,omitempty"` // 领取当日活跃奖励所需达到的活跃度。
}

// TaskCodexDefinition 定义一项可持久追踪、可领取的角色目标。
type TaskCodexDefinition struct {
	ID             uint32          `json:"id"`                        // 同类目标中的稳定编号，发布后保留原身份。
	Kind           uint32          `json:"kind"`                      // 主线、每日、成就、图鉴或活跃。
	Name           string          `json:"name"`                      // 面向玩家的目标名称。
	Event          string          `json:"event,omitempty"`           // 由服务端源事务识别的目标事件，活跃使用已确认每日领取。
	Target         int64           `json:"target"`                    // 完成所需数量或达到的等级。
	MonsterID      uint32          `json:"monster_id,omitempty"`      // 目标怪物编号，零接受所有符合条件的怪物。
	MapID          uint64          `json:"map_id,omitempty"`          // 展示所属地图，非图鉴目标用作地图过滤条件。
	TemplateID     uint32          `json:"template_id,omitempty"`     // 同类挑战的模板编号，零接受该玩法全部合格模板。
	MinimumLevel   uint32          `json:"minimum_level,omitempty"`   // 开放最低等级，零不限。
	PrerequisiteID uint32          `json:"prerequisite_id,omitempty"` // 主线前置任务，需已经领取。
	Activity       uint32          `json:"activity,omitempty"`        // 首次领取该每日任务确认的活跃度。
	Reward         TaskCodexReward `json:"reward"`                    // 和掉落独立的绑定奖励。
}

// TaskCodexReward 使用整数最小货币单位，所有奖励按来源绑定。
type TaskCodexReward struct {
	BoundGold         int64                 `json:"bound_gold,omitempty"`          // 绑定金币增量。
	BoundYuanbaoUnits int64                 `json:"bound_yuanbao_units,omitempty"` // 绑定元宝最小单位增量。
	Items             []TaskCodexRewardItem `json:"items,omitempty"`               // 按定义发放的绑定库存物品。
}

// TaskCodexRewardItem 指定一种绑定奖励物品的模板和总数量。
type TaskCodexRewardItem struct {
	ItemID   uint32 `json:"item_id"`  // 运行时物品模板编号。
	Quantity int64  `json:"quantity"` // 按库存堆叠规则拆分前的总数量。
}

// ValidateTasksCodex 拒绝没有运行来源、存在循环或超过状态帧容量的发布定义。
func (f *Fixture) ValidateTasksCodex() error {
	rules := f.TasksCodex
	if rules == nil || len(rules.Definitions) == 0 {
		return nil
	}
	// 每个种类、编号的组合独立，图鉴另按怪物唯一身份去重。
	seen := make(map[[2]uint32]bool)
	codexMonsters := make(map[uint32]bool)
	quests := make(map[uint32]uint32)
	dailyCount, activityCount, frameSize := 0, 0, 64
	for _, def := range rules.Definitions {
		identity := [2]uint32{def.Kind, def.ID}
		if def.Kind < TaskCodexMain || def.Kind > TaskCodexActivity || def.ID == 0 || seen[identity] || strings.TrimSpace(def.Name) == "" || def.Target <= 0 {
			return fmt.Errorf("任务图鉴定义 %v 重复或无效", identity)
		}
		seen[identity] = true
		frameSize += 64 + len(def.Name) + 22*len(def.Reward.Items)
		if frameSize > wire.MaxFrame {
			return fmt.Errorf("任务图鉴列表超过单帧容量")
		}
		if !f.validTaskCodexSource(def) {
			return fmt.Errorf("任务图鉴定义 %v 的事件、挑战模板、怪物或地图来源不合法", identity)
		}
		if def.Kind != TaskCodexDaily && def.Activity != 0 || def.Kind != TaskCodexMain && def.PrerequisiteID != 0 {
			return fmt.Errorf("任务图鉴定义 %v 的前置或活跃配置不合法", identity)
		}
		switch def.Kind {
		case TaskCodexMain:
			quests[def.ID] = def.PrerequisiteID
		case TaskCodexDaily:
			if def.Activity == 0 || def.Activity > rules.DailyActivityTarget {
				return fmt.Errorf("每日任务 %d 的活跃度不合法", def.ID)
			}
			dailyCount++
		case TaskCodexMonster:
			if def.Event != TaskCodexMonsterKill || def.MonsterID == 0 || codexMonsters[def.MonsterID] {
				return fmt.Errorf("怪物图鉴 %d 必须按唯一怪物身份累计", def.ID)
			}
			if def.Reward.BoundGold != 0 || len(def.Reward.Items) != 0 {
				return fmt.Errorf("怪物图鉴 %d 的奖励只能配置绑定元宝及收藏资格", def.ID)
			}
			codexMonsters[def.MonsterID] = true
		case TaskCodexActivity:
			if def.Event != "" || def.ID != 1 || def.Target != int64(rules.DailyActivityTarget) || def.MinimumLevel != 0 {
				return fmt.Errorf("每日活跃奖励定义不合法")
			}
			activityCount++
		}
		if def.Reward.BoundGold < 0 || def.Reward.BoundYuanbaoUnits < 0 {
			return fmt.Errorf("任务图鉴定义 %v 的绑定货币不合法", identity)
		}
		for _, item := range def.Reward.Items {
			if _, valid := f.Item(item.ItemID); !valid || item.Quantity <= 0 {
				return fmt.Errorf("任务图鉴定义 %v 的奖励物品不合法", identity)
			}
		}
	}
	if dailyCount > 0 && (uint32(dailyCount) != rules.DailyPoolSize || rules.DailyClaimLimit == 0 || rules.DailyClaimLimit > rules.DailyPoolSize || activityCount != 1) ||
		dailyCount == 0 && activityCount > 0 {
		return fmt.Errorf("每日任务池、领取上限和活跃奖励必须符合发布规则")
	}
	for id := range quests {
		chain := make(map[uint32]bool)
		for prerequisite := id; prerequisite != 0; {
			next, exists := quests[prerequisite]
			if !exists || chain[prerequisite] {
				return fmt.Errorf("主线任务 %d 的前置不存在或存在循环", id)
			}
			chain[prerequisite] = true
			prerequisite = next
		}
	}
	return nil
}
