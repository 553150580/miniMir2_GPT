// 本文件冻结挑战奖励、按背包容量分流并处理整笔领取。
package persistence

import (
	"slices"

	"minimir/internal/devconfig"
	"minimir/internal/domain"
	p "minimir/internal/protocol"
)

const AssetChallengeRewardClaim = "claim_challenge_reward" // 走原资产事务和操作回执。

// freezePersonalBossItems 在入场时固定实例和装备耐久。未知的配置无法形成奖励。
func freezePersonalBossItems(f *devconfig.Fixture, record *personalBossRecord, rewards *devconfig.ChallengeItemReward) {
	if rewards == nil {
		return
	}
	record.PendingRewardLimit = rewards.PendingLimit
	for _, line := range rewards.Items {
		item, _ := f.Item(line.ItemID) // 入场已验证物品存在和堆叠数量合法。
		asset := domain.Item{ID: domain.ID(), ItemID: line.ItemID, Quantity: line.Quantity, Version: 1,
			SourceKind: "personal_boss", SourceID: record.ChallengeID, Quality: item.DropQuality}
		record.KillRewardItems = append(record.KillRewardItems, InitializeItemDurability(asset, f))
	}
}

// applyPersonalBossItems 在原胜利事务中只发整批。背包放不下时用预约已预留的待领名额。
func applyPersonalBossItems(r *domain.Role, record *personalBossRecord) error {
	if len(record.KillRewardItems) == 0 {
		return nil
	}
	items := slices.Clone(record.KillRewardItems)
	for i := range items {
		items[i].Bound = !record.PrivilegeUntil.After(record.FinishedAt)
	}
	record.RewardItems = items
	if len(items) <= r.Capacity-len(r.Items) {
		r.Items = append(slices.Clone(r.Items), items...)
		return nil
	}
	if len(r.PendingChallengeRewards) >= int(record.PendingRewardLimit) {
		return Fail(p.CodeOperationConflict, "原挑战预留的待领奖励名额丢失")
	}
	reward := domain.ChallengePendingReward{RewardID: record.ChallengeID, SourceKind: "personal_boss", SourceID: record.ChallengeID,
		Origin: r.Origin, TemplateID: record.TemplateID, RuleVersion: record.RuleVersion, EarnedAt: record.FinishedAt, Items: items}
	r.PendingChallengeRewards = append(slices.Clone(r.PendingChallengeRewards), reward)
	record.RewardQueued = true
	return nil
}

// claimChallengeReward 仅转移本角色永久入口服的待领附件。失败保留整笔，可清包后用新操作领取。
func claimChallengeReward(r domain.Role, id string) (domain.Role, *domain.ChallengePendingReward, uint32) {
	if !lowerHex(id, 16) {
		return r, nil, p.CodeInvalidArgument
	}
	for index, reward := range r.PendingChallengeRewards {
		if reward.Origin != r.Origin || reward.RewardID != id {
			continue
		}
		if len(reward.Items) > r.Capacity-len(r.Items) {
			return r, nil, p.CodeEquipmentBagFull
		}
		r.Items = append(slices.Clone(r.Items), reward.Items...)
		r.PendingChallengeRewards = slices.Delete(slices.Clone(r.PendingChallengeRewards), index, index+1)
		return r, &reward, 0
	}
	return r, nil, p.CodeOperationConflict
}
