// 本文件约束挑战奖励的开发输入和待领协议的最大体积。
package devconfig

// 待领查询一并返回原附件。硬上限保障即使历史记录同时返回也不会超过协议包大小。
const maxChallengePendingBundles = 20
const maxChallengeRewardItems = 8

// ChallengeItemReward 是同一场胜利可授予的一批物品及其排队容量。
type ChallengeItemReward struct {
	PendingLimit uint32                `json:"pending_limit"`         // 单角色可积累的未领挑战笔数。
	Items        []ChallengeRewardItem `json:"items"`                 // 按原击杀规则判定绑定的固定附件。
	BoundItems   []ChallengeRewardItem `json:"bound_items,omitempty"` // 始终绑定的固定附件，计入同一胜利批次。
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
	total := len(reward.Items) + len(reward.BoundItems) // 按两个来源的总格数控制待领体积。
	if reward.PendingLimit == 0 || reward.PendingLimit > maxChallengePendingBundles ||
		total == 0 || total > maxChallengeRewardItems {
		return false
	}
	for _, group := range [][]ChallengeRewardItem{reward.Items, reward.BoundItems} {
		for _, line := range group {
			item, exists := f.Item(line.ItemID)
			if !exists || !item.validChallengeRewardQuantity(line.Quantity) {
				return false
			}
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

// validUnbudgetedChallengeReward 验证尚未接入独立供给预算的挑战来源可以发放该物品。
func (i Item) validUnbudgetedChallengeReward(quantity int64) bool {
	return i.ID != 0 && (!i.IsEquipment || i.DropQuality < 2) && i.validChallengeRewardQuantity(quantity)
}
