// 本文件定义讨伐链通关抽奖的候选附件、来源绑定及运行时装配边界。
package devconfig

import (
	"math"
	"slices"
)

// maxDungeonRewardOptions 限制单个奖表扫描的候选包数量。
const maxDungeonRewardOptions = 64

// DungeonRewardBindPolicy 区分事件时权益判定与独立来源的固定绑定。
type DungeonRewardBindPolicy string

const (
	DungeonRewardBindOnKill  DungeonRewardBindPolicy = "kill_privilege" // 最终首领击杀时有有效特权卡才非绑。
	DungeonRewardAlwaysBound DungeonRewardBindPolicy = "always_bound"   // 此来源的物品固定绑定。
)

// DungeonRewards 供每位原真人在通关后独立抽取一次。
type DungeonRewards struct {
	Options []DungeonRewardOption `json:"options"` // 原配置顺序决定各选项的权重区间。
}

// DungeonRewardOption 是一次抽中时获得的整笔附件，空列表表示本次未抽中物品。
type DungeonRewardOption struct {
	Weight uint32              `json:"weight"` // 本选项在总权重中占据的份额。
	Items  []DungeonRewardItem `json:"items"`  // 候选包内保持原顺序的独立物品实例。
}

// DungeonRewardItem 指定物品目录、单实例数量与其来源绑定策略。
type DungeonRewardItem struct {
	ItemID     uint32                  `json:"item_id"`     // 物品目录编号。
	Quantity   int64                   `json:"quantity"`    // 此实例的原数量。
	BindPolicy DungeonRewardBindPolicy `json:"bind_policy"` // 此物品的绑定规则。
}

// ValidDungeonRewards 检查待领奖包与抽奖区间。缺失奖表时保持已有挑战模板可战斗。
func (f *Fixture) ValidDungeonRewards(rules *DungeonRewards) bool {
	if rules == nil {
		return true
	}
	if len(rules.Options) == 0 || len(rules.Options) > maxDungeonRewardOptions {
		return false
	}
	var totalWeight uint64 // 用更宽整数累加，再验证统一的抽奖区间上界。
	hasItems := false
	for _, option := range rules.Options {
		totalWeight += uint64(option.Weight)
		if option.Weight == 0 || totalWeight > math.MaxUint32 || len(option.Items) > maxChallengeRewardItems {
			return false
		}
		for _, line := range option.Items {
			item, found := f.Item(line.ItemID)
			if !found || !item.validUnbudgetedChallengeReward(line.Quantity) {
				return false
			}
			switch line.BindPolicy {
			case DungeonRewardBindOnKill:
			case DungeonRewardAlwaysBound:
				if item.IsEquipment {
					return false
				}
			default:
				return false
			}
			hasItems = true
		}
	}
	return hasItems
}

// clone 保留抽奖和附件的原顺序，阻止候选配置被修改后更改已装配的奖表。
func (r *DungeonRewards) clone() *DungeonRewards {
	if r == nil {
		return nil
	}
	frozen := DungeonRewards{Options: slices.Clone(r.Options)}
	for i := range frozen.Options {
		frozen.Options[i].Items = slices.Clone(r.Options[i].Items)
	}
	return &frozen
}
