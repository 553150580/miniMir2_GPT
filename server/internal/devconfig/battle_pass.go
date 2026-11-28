// 本文件定义战令周期、现金档位及按档位计算的绑定总奖励。
package devconfig

import (
	"fmt"
	"slices"
	"time"
)

const (
	BattlePassDays           = 30 // 从上海运营日开始计算的完整周期。
	BattlePassFree    uint32 = 1  // 无现金权益的基础档位。
	BattlePassMaxTier uint32 = 3  // 当前方案的最高档位。
)

// BattlePassRules 把运营日历和唯一玩法工作簿的档位总奖装配到服务端。
type BattlePassRules struct {
	AnchorDay string           `json:"anchor_day"` // 首个 30 天周期开始的上海日期。
	Tiers     []BattlePassTier `json:"tiers"`      // 免费和现金档按购买顺序排列。
}

// BattlePassTier 保存该档全部应得的物品和绑定钱包增量。
type BattlePassTier struct {
	ID      uint32          `json:"id" bson:"id"`             // 领取和现金商品的稳定档位编号。
	CashFen int64           `json:"cash_fen" bson:"cash_fen"` // 该档完整人民币价格，升档补差。
	Daily   TaskCodexReward `json:"daily" bson:"daily"`       // 一个合格运营日的全部应得奖励。
	Season  TaskCodexReward `json:"season" bson:"season"`     // 整个周期只领取一次的全部应得奖励。
}

// validateBattlePass 拒绝缺失活动来源、越级负差额或绑定奖品不能正常入包的配置。
func (f *Fixture) validateBattlePass() error {
	rules := f.BattlePass
	if rules == nil {
		return nil
	}
	anchor, err := time.Parse(time.DateOnly, rules.AnchorDay)
	if err != nil || anchor.Year() < 2020 || anchor.Year() > 2099 || anchor.Format(time.DateOnly) != rules.AnchorDay ||
		f.RuleVersion < 30 || f.TasksCodex == nil || f.TasksCodex.DailyClaimLimit == 0 || f.TasksCodex.DailyActivityTarget == 0 ||
		len(rules.Tiers) != int(BattlePassMaxTier) || rules.Tiers[0].CashFen != 0 {
		return fmt.Errorf("战令需要固定上海运营日起点、已配置日常活跃和三个档位")
	}
	reachable := make([]uint32, 0, f.TasksCodex.DailyPoolSize) // 每个日常任务能提供的活跃度。
	for _, def := range f.TasksCodex.Definitions {
		if def.Kind == TaskCodexDaily {
			reachable = append(reachable, def.Activity)
		}
	}
	// 工作簿中高活跃门槛必须能被合法领取次数覆盖。
	if dailyBattlePassMaximum(reachable, f.TasksCodex.DailyClaimLimit) < uint64(f.TasksCodex.DailyActivityTarget) {
		return fmt.Errorf("战令活跃门槛超出当前每日任务可领取上限")
	}
	var previous BattlePassTier // 已核验的前一档完整价格和总奖。
	for index, tier := range rules.Tiers {
		if tier.ID != uint32(index+1) || index > 0 && tier.CashFen <= previous.CashFen || tier.CashFen > 1_000_000_000 ||
			len(tier.Daily.Items) == 0 || !f.validBattlePassReward(tier.Daily, previous.Daily) || !f.validBattlePassReward(tier.Season, previous.Season) {
			return fmt.Errorf("战令第 %d 档的原金额或绑定总奖无效", index+1)
		}
		previous = tier
	}
	return nil
}

// dailyBattlePassMaximum 只在加载时计算取活跃度最高的若干个日常任务能达到的总量。
func dailyBattlePassMaximum(activity []uint32, limit uint32) uint64 {
	slices.Sort(activity)
	var total uint64
	for _, value := range activity[len(activity)-min(len(activity), int(limit)):] {
		total += uint64(value)
	}
	return total
}

// validBattlePassReward 校验可堆叠成长材料、钱包整数和跨档单调性。
func (f *Fixture) validBattlePassReward(reward, lower TaskCodexReward) bool {
	if reward.BoundGold < lower.BoundGold || reward.BoundGold > 1_000_000_000 ||
		reward.BoundYuanbaoUnits < lower.BoundYuanbaoUnits || reward.BoundYuanbaoUnits > 1_000_000_000 || len(reward.Items) > 8 {
		return false
	}
	var previous uint32
	matched := 0
	for _, item := range reward.Items {
		definition, found := f.Item(item.ItemID)
		if !found || !definition.Stackable || definition.IsEquipment || definition.BookSkillID != 0 || definition.MaxStack <= 0 ||
			item.ItemID <= previous || item.Quantity <= 0 || item.Quantity > 1000 || f.IsCastingSupply(item.ItemID) {
			return false
		}
		previous = item.ItemID
		if matched < len(lower.Items) && lower.Items[matched].ItemID <= item.ItemID {
			if lower.Items[matched].ItemID != item.ItemID || lower.Items[matched].Quantity > item.Quantity {
				return false
			}
			matched++
		}
	}
	return matched == len(lower.Items)
}
