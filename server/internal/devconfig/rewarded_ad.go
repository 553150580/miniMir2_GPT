// 本文件定义激励视频的时长、渠道身份与三种运营日日限量。
package devconfig

import "fmt"

// RewardedAdRules 只在已有渠道校验器装配时为玩家签发观看凭据。
type RewardedAdRules struct {
	Provider    string `json:"provider" bson:"provider"`                             // 可信广告渠道的配置标识。
	Placement   string `json:"placement" bson:"placement"`                           // 本游戏中对应的激励视频广告位。
	Seconds     uint32 `json:"reward_seconds" bson:"reward_seconds"`                 // 一次通过核验的广告增加的托管秒数。
	AccountCap  uint32 `json:"account_daily" bson:"account_daily"`                   // 账号跨所有角色及区服共用的日限量。
	DeviceCap   uint32 `json:"device_daily" bson:"device_daily"`                     // 同一可信设备的日限量。
	NetworkCap  uint32 `json:"ip_daily" bson:"ip_daily"`                             // 同一可信客户端 IP 的日限量。
	WatchSec    uint32 `json:"watch_seconds" bson:"watch_seconds"`                   // 从签发到广告必须播放完的秒数。
	CallbackSec uint32 `json:"callback_grace_seconds" bson:"callback_grace_seconds"` // 原播放已完成之后允许渠道送达的时限。
}

// ValidRewardedAdCode 限定渠道和广告位的可持久化标识。
func ValidRewardedAdCode(value string) bool {
	if len(value) < 1 || len(value) > 64 {
		return false
	}
	for _, c := range value {
		if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '_' || c == '-') {
			return false
		}
	}
	return true
}

// validateRewardedAd 拒绝缺少任何一类日限额的装配，保持原默认配置兼容。
func (f *Fixture) validateRewardedAd() error {
	rules := f.RewardedAd
	if rules == nil {
		return nil
	}
	if f.RuleVersion < 30 || !ValidRewardedAdCode(rules.Provider) || !ValidRewardedAdCode(rules.Placement) ||
		rules.Seconds == 0 || rules.Seconds > 86400 || rules.AccountCap == 0 || rules.DeviceCap == 0 || rules.NetworkCap == 0 ||
		max(rules.AccountCap, rules.DeviceCap, rules.NetworkCap) > 1000 || rules.WatchSec == 0 || rules.WatchSec > 900 ||
		rules.CallbackSec == 0 || rules.CallbackSec > 86400 {
		return fmt.Errorf("激励视频的广告位、时长、核验窗口或三类运营日限额无效")
	}
	return nil
}
