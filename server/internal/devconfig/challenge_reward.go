// 本文件约束挑战奖励的开发输入和待领协议的最大体积。
package devconfig

// 待领查询一并返回原附件。硬上限保障即使历史记录同时返回也不会超过协议包大小。
const maxChallengePendingBundles = 20
const maxChallengeRewardItems = 8

// ChallengeItemReward 是同一场胜利可授予的一批物品及其排队容量。
type ChallengeItemReward struct {
	PendingLimit uint32                `json:"pending_limit"` // 单角色可积累的未领挑战笔数。
	Items        []ChallengeRewardItem `json:"items"`         // 此挑战会产生的固定物品列表。
}

// ChallengeRewardItem 是一个独立实例或一份可堆叠的物品。
type ChallengeRewardItem struct {
	ItemID   uint32 `json:"item_id"`  // 原目录物品定义。
	Quantity int64  `json:"quantity"` // 此实例携带的合法数量。
}

// validChallengeItemReward 核对奖励数量、背包槽预算及物品来源。
func (f *Fixture) validChallengeItemReward(reward *ChallengeItemReward) bool {
	if reward == nil {
		return true
	}
	if reward.PendingLimit == 0 || reward.PendingLimit > maxChallengePendingBundles ||
		len(reward.Items) == 0 || len(reward.Items) > maxChallengeRewardItems {
		return false
	}
	for _, line := range reward.Items {
		item, exists := f.Item(line.ItemID)
		if !exists || !item.validChallengeRewardQuantity(line.Quantity) {
			return false
		}
	}
	return true
}

// validChallengeRewardQuantity 检查一个附件实例能保存的数量，按原物品定义区分单件与堆叠。
func (i Item) validChallengeRewardQuantity(quantity int64) bool {
	if i.Stackable {
		return quantity > 0 && quantity <= i.MaxStack
	}
	return quantity == 1
}
