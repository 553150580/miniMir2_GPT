// 本文件冻结并校验普通怪击杀时的托管卡直接收益明细。
package persistence

import (
	"math"
	"time"

	"minimir/internal/devconfig"
	p "minimir/internal/protocol"
)

// HostingKillBonus 保存击杀时有效的卡截止时刻、原直接收益与额外部分。
type HostingKillBonus struct {
	CardUntil       time.Time `json:"card_until" bson:"card_until"`               // 原击杀时有效的托管卡截止时刻。
	BaseExperience  int64     `json:"base_experience" bson:"base_experience"`     // 原怪直接经验。
	BonusExperience int64     `json:"bonus_experience" bson:"bonus_experience"`   // 卡对原怪直接经验额外增加的数量。
	BaseDirectGold  int64     `json:"base_direct_gold" bson:"base_direct_gold"`   // 互斥掉落分支实际选定的原直接金币。
	BonusDirectGold int64     `json:"bonus_direct_gold" bson:"bonus_direct_gold"` // 卡对直接金币额外增加的数量，回收金币不在此处。
}

// NewHostingKillBonus 按同一版本的整数万分比计算逐杀加成，并验证应发总额可以入账。
// until 是原卡截止时刻，experience 和 gold 是原直接经验和原直接金币，rules 是启用的同版本权益。
func NewHostingKillBonus(until time.Time, experience, gold int64, rules *devconfig.HostingBenefits) (*HostingKillBonus, error) {
	if until.IsZero() || rules == nil {
		return nil, Fail(p.CodeOperationConflict, "托管击杀缺少原有效期或权益配置")
	}
	extraExperience, expOK := hostingKillExtra(experience, rules.KillExperienceBonusBP)
	extraGold, goldOK := hostingKillExtra(gold, rules.DirectGoldBonusBP)
	if !expOK || !goldOK {
		return nil, Fail(p.CodeOperationConflict, "托管击杀直接收益超出结算范围")
	}
	return &HostingKillBonus{
		CardUntil: until.UTC(), BaseExperience: experience, BonusExperience: extraExperience,
		BaseDirectGold: gold, BonusDirectGold: extraGold,
	}, nil
}

// hostingKillExtra 按万分比向下取整，避免原收益直接乘万分比导致中间值溢出。
func hostingKillExtra(base int64, basisPoints uint32) (int64, bool) {
	const scale int64 = 10000 // 一个整数收益单位对应的万分比满额。
	if base < 0 || basisPoints > uint32(scale) {
		return 0, false
	}
	rate := int64(basisPoints)                         // 已核对上界的额外收益比例。
	extra := base/scale*rate + (base%scale*rate)/scale // 整段及余数向下取整后的加成。
	return extra, base <= math.MaxInt64-extra
}

// validateHostingKillBonus 校验原怪直接收益及核对时间，结算时不再读取当前角色卡期限。
func validateHostingKillBonus(q Settlement, f *devconfig.Fixture) error {
	bonus := q.HostingKillBonus // 没有卡收益的旧来源与新来源均保留原结算格式。
	if bonus == nil {
		return nil
	}
	if q.Kills != 1 || q.StopReason != 0 || q.EventAt.IsZero() || !bonus.CardUntil.After(q.EventAt) {
		return Fail(p.CodeOperationConflict, "托管卡加成的原击杀或有效期无效")
	}
	monster, known := f.MonsterByID(q.MonsterID)
	scene, ordinary := f.MapByID(q.MapID)
	if !known || !ordinary || scene.SceneKindValue() != devconfig.SceneKindPersonal || bonus.BaseExperience != monster.Exp {
		return Fail(p.CodeOperationConflict, "托管卡加成不属于原普通怪击杀")
	}
	if bonus.BaseDirectGold > 0 && (q.Item != nil || bonus.BaseDirectGold < monster.GoldMin || bonus.BaseDirectGold > monster.GoldMax) {
		return Fail(p.CodeOperationConflict, "托管卡加成不属于原直接金币结果")
	}
	expected, err := NewHostingKillBonus(bonus.CardUntil, bonus.BaseExperience, bonus.BaseDirectGold, f.HostingBenefits)
	if err != nil {
		return err
	}
	if !sameHostingKillBonus(bonus, expected) ||
		q.Experience != expected.BaseExperience+expected.BonusExperience ||
		q.Gold != expected.BaseDirectGold+expected.BonusDirectGold {
		return Fail(p.CodeOperationConflict, "托管卡直接奖励和原加成不一致")
	}
	return nil
}

// sameHostingKillBonus 同时校验递交和已落库回执，按 BSON 日期精度比较同一截止时刻。
func sameHostingKillBonus(left, right *HostingKillBonus) bool {
	if left == nil || right == nil {
		return left == right
	}
	return left.CardUntil.UnixMilli() == right.CardUntil.UnixMilli() &&
		left.BaseExperience == right.BaseExperience && left.BonusExperience == right.BonusExperience &&
		left.BaseDirectGold == right.BaseDirectGold && left.BonusDirectGold == right.BonusDirectGold
}
