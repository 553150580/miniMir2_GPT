// 本文件记录已提交但尚未转入背包的独立挑战奖励。
package domain

import "time"

// ChallengePendingReward 保存原挑战身份、资产所有权和不可变的物品明细。
type ChallengePendingReward struct {
	RewardID    string    `bson:"reward_id" json:"reward_id"`       // 可重试领取的稳定编号。
	SourceKind  string    `bson:"source_kind" json:"source_kind"`   // 带来该批物品的挑战类型与终态。
	SourceID    string    `bson:"source_id" json:"source_id"`       // 原挑战的永久编号。
	Origin      uint64    `bson:"origin" json:"origin"`             // 奖励所属角色的永久入口服。
	TemplateID  uint32    `bson:"template_id" json:"template_id"`   // 原挑战模板编号。
	RuleVersion uint64    `bson:"rule_version" json:"rule_version"` // 结算使用的原规则版本。
	EarnedAt    time.Time `bson:"earned_at" json:"earned_at"`       // 原挑战实际确认的奖励时刻。
	Items       []Item    `bson:"items" json:"items"`               // 已冻结实例、数量与绑定，不参与普通回收。

	SourceRunID    uint64              `bson:"source_run_id,omitempty" json:"source_run_id,omitempty"`     // 世界奖励归属的原运行服，迁移后继续留存。
	SourceRank     uint32              `bson:"source_rank,omitempty" json:"source_rank,omitempty"`         // 世界首领有效伤害榜的实际名次。
	ConfigHash     string              `bson:"config_hash,omitempty" json:"config_hash,omitempty"`         // 生成世界附件的完整配置摘要。
	PrivilegeUntil time.Time           `bson:"privilege_until,omitempty" json:"privilege_until,omitempty"` // 世界首领奖励采用的原特权截止时间。
	MirrorAssets   *MirrorSeasonAssets `bson:"mirror_assets,omitempty" json:"mirror_assets,omitempty"`     // 镜像赛季确认时冻结的绑定元宝。
	Soul           *SoulAward          `bson:"soul,omitempty" json:"soul,omitempty"`                       // 世界首领独立角色奖项。
}
