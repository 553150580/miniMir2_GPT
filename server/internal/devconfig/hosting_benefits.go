// 本文件定义唯一玩法工作簿投影的托管权益，不包含报价或充值渠道。
package devconfig

import "errors"

// HostingBenefits 是角色级托管卡和每日手动领时的规则。
type HostingBenefits struct {
	FreeDailySeconds      int64  `json:"free_daily_seconds"`       // 不持卡时每日总领时，单位秒。
	CardDailySeconds      int64  `json:"card_daily_seconds"`       // 托管卡有效时每日总领时，包含免费额度。
	KillExperienceBonusBP uint32 `json:"kill_experience_bonus_bp"` // 普通怪击杀的直接经验加成，万分比。
	DirectGoldBonusBP     uint32 `json:"direct_gold_bonus_bp"`     // 普通怪击杀的直接金币加成，万分比。
}

// validateHostingBenefits 保留旧运行配置的兼容入口，启用后核对领时总额度。
func (f *Fixture) validateHostingBenefits() error {
	if benefits := f.HostingBenefits; benefits != nil && (benefits.FreeDailySeconds <= 0 || benefits.CardDailySeconds < benefits.FreeDailySeconds) {
		return errors.New("托管每日领取额度无效")
	}
	return nil
}
