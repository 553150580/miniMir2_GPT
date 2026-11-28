// 本文件定义共享首领奖励的独立资格和物品配置，并约束单角色一次可领取的实例量。
package devconfig

import "slices"

// WorldBossRewards 按场次原真实贡献评估资格，击杀与超时分别配置物品。
type WorldBossRewards struct {
	MinimumDamage        int64                 `json:"minimum_damage"`                  // 达到本次有效扣血门槛才可获奖，席位或持卡本身不提供资格。
	AllowDisconnected    bool                  `json:"allow_disconnected"`              // 允许终态时仍处于可接回且离线的原席位获奖。
	AllowWithdrawn       bool                  `json:"allow_withdrawn"`                 // 允许已主动退出但达到贡献门槛的角色获奖。
	AllowDefeated        bool                  `json:"allow_defeated"`                  // 允许已战败且达到贡献门槛的角色获奖。
	Participation        []ChallengeRewardItem `json:"participation,omitempty"`         // 击杀时每位符合资格角色的参与附件。
	Ranks                []WorldBossRankReward `json:"ranks,omitempty"`                 // 击杀追加排名附件，按原公开榜名次匹配至多一档。
	TimeoutParticipation []ChallengeRewardItem `json:"timeout_participation,omitempty"` // 超时追加来源独立，全为绑定，留空时无超时奖励。
}

// WorldBossRankReward 是原公开榜从第一名开始计数的一段闭区间。
type WorldBossRankReward struct {
	FirstRank uint32                `json:"first_rank"` // 包含此名次，必须大于前一档末名。
	LastRank  uint32                `json:"last_rank"`  // 包含此名次，且不得超出原场席位数。
	Items     []ChallengeRewardItem `json:"items"`      // 仅由同一个击杀事件产生的排名附件，不补发给下一名。
}

// clone 为已装配的首领复制奖表，避免原定义的切片改动影响运行快照。
func (r *WorldBossRewards) clone() *WorldBossRewards {
	if r == nil {
		return nil
	}
	snapshot := *r
	snapshot.Participation = slices.Clone(r.Participation)
	snapshot.TimeoutParticipation = slices.Clone(r.TimeoutParticipation)
	snapshot.Ranks = slices.Clone(r.Ranks)
	for index := range snapshot.Ranks {
		snapshot.Ranks[index].Items = slices.Clone(r.Ranks[index].Items)
	}
	return &snapshot
}

// ValidWorldBossRewards 校验指定模板的奖励装配；模板存在且未装配奖表时奖励合法关闭。
func (f *Fixture) ValidWorldBossRewards(cfg *WorldBossConfig) bool {
	if cfg == nil {
		return false
	}
	rules := cfg.Rewards
	if rules == nil {
		return true
	}
	hasSoul := f.Souls != nil && f.Souls.validDrop(f.Souls.World[cfg.TemplateID])
	if cfg.ParticipantLimit <= 0 || rules.MinimumDamage <= 0 || rules.MinimumDamage > cfg.Monster.HP ||
		len(rules.Participation)+len(rules.Ranks)+len(rules.TimeoutParticipation) == 0 && !hasSoul ||
		!f.validWorldBossRewardItems(rules.Participation) || !f.validWorldBossRewardItems(rules.TimeoutParticipation) ||
		len(rules.TimeoutParticipation) > 0 && rules.MinimumDamage == cfg.Monster.HP {
		return false
	}
	var previousRank uint32 // 前一段的包含末名，零允许从第一名开始。
	for _, rank := range rules.Ranks {
		if rank.FirstRank <= previousRank || rank.LastRank < rank.FirstRank || uint64(rank.LastRank) > uint64(cfg.ParticipantLimit) ||
			len(rank.Items) == 0 || len(rules.Participation)+len(rank.Items) > maxChallengeRewardItems || !f.validWorldBossRewardItems(rank.Items) {
			return false
		}
		previousRank = rank.LastRank
	}
	return true
}

// validWorldBossRewardItems 校验预发附件，稀有及以上装备需另有世界来源预算才可开放。
func (f *Fixture) validWorldBossRewardItems(items []ChallengeRewardItem) bool {
	if len(items) > maxChallengeRewardItems {
		return false
	}
	for _, line := range items {
		item, found := f.Item(line.ItemID)
		if !found || !item.validUnbudgetedChallengeReward(line.Quantity) {
			return false
		}
	}
	return true
}
