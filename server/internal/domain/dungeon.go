// 本文件保存角色的讨伐链每日入场流水与独占场次指针。
package domain

// DungeonProgress 和角色资产使用同一个修订版本。
type DungeonProgress struct {
	ActiveChallengeID string `bson:"active_challenge_id,omitempty" json:"active_challenge_id,omitempty"` // 已扣次且尚未终结的原场次。
	EntryDay          string `bson:"entry_day,omitempty" json:"entry_day,omitempty"`                     // 最近扣次所属的上海运营日。
	Entries           uint32 `bson:"entries,omitempty" json:"entries,omitempty"`                         // 原运营日已扣且尚未退款的入场次数。
}
