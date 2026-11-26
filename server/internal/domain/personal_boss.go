// 本文件保存个人首领的角色级预约、每日成功次数、各模板冷却和通关证明。
package domain

import "time"

// PersonalBossProgress 与角色资产同属一个所有者和修订版本。
type PersonalBossProgress struct {
	ActiveChallengeID string                               `bson:"active_challenge_id,omitempty" json:"active_challenge_id,omitempty"` // 尚未终结的独占挑战编号。
	SuccessDay        string                               `bson:"success_day,omitempty" json:"success_day,omitempty"`                 // 成功次数对应的上海运营日。
	Successes         uint32                               `bson:"successes,omitempty" json:"successes,omitempty"`                     // 当天已经确认的成功次数。
	Cooldowns         map[string]time.Time                 `bson:"cooldowns,omitempty" json:"cooldowns,omitempty"`                     // 各首领模板可再次预约的服务端时间。
	Qualifications    map[string]PersonalBossQualification `bson:"qualifications,omitempty" json:"qualifications,omitempty"`           // 每个模板最近一次真实通关证明，键为十进制模板编号。
}

// PersonalBossQualification 固定某个模板通关时的角色战斗条件与规则身份。
type PersonalBossQualification struct {
	Fingerprint string `bson:"fingerprint" json:"fingerprint"`                       // 原执行器生成的战斗条件摘要。
	RuleVersion uint64 `bson:"rule_version" json:"rule_version"`                     // 通关时采用的战斗规则版本。
	ConfigHash  string `bson:"config_hash" json:"config_hash"`                       // 通关时采用的完整配置摘要。
	ChallengeID string `bson:"challenge_id,omitempty" json:"challenge_id,omitempty"` // 产生这份证明的原真实胜利编号。
}
