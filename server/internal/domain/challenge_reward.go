// 本文件记录已提交但尚未转入背包的独立挑战奖励。
package domain

import "time"

// ChallengePendingReward 保存原挑战身份、资产所有权和不可变的物品明细。
type ChallengePendingReward struct {
	RewardID    string    `bson:"reward_id" json:"reward_id"`       // 可重试领取的稳定编号。
	SourceKind  string    `bson:"source_kind" json:"source_kind"`   // 带来该批物品的挑战类型。
	SourceID    string    `bson:"source_id" json:"source_id"`       // 原挑战的永久编号。
	Origin      uint64    `bson:"origin" json:"origin"`             // 奖励所属角色的永久入口服。
	TemplateID  uint32    `bson:"template_id" json:"template_id"`   // 原挑战模板编号。
	RuleVersion uint64    `bson:"rule_version" json:"rule_version"` // 结算使用的原规则版本。
	EarnedAt    time.Time `bson:"earned_at" json:"earned_at"`       // 已确认胜利的实际时刻。
	Items       []Item    `bson:"items" json:"items"`               // 已冻结实例、数量与绑定，不参与普通回收。
}
