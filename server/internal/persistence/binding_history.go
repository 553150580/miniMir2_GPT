// 城镇绑定历史兼容。识别升级前的规则版本和地图、区域身份，允许经核验的规则 24 比奇城旧绑定延续。

package persistence

import "minimir/internal/domain"

// 规则25在比奇城(6,5)、(10,5)新增商人及占位，安全区501的中心与边界均未变。
// 仅接受已发布的7098ef62原回执摘要，且当前仍为289f0939所确认的同一目标目录。
// matchesHistoricalTownBinding 判断既有指定版本绑定能否和升级后的城镇图与安全区目录对应。
// receipt 记录回执，使用会话绑定回执表示，currentRule 记录当前状态规则，使用数值表示，currentMapHash 记录当前状态地图摘要，使用文本表示。
// currentZoneHash 记录当前状态区域摘要，使用文本表示。
func matchesHistoricalTownBinding(receipt *BindingReceipt, currentRule uint64, currentMapHash, currentZoneHash string) bool {
	return currentRule >= 25 && receipt.RuleVersion == 24 &&
		receipt.ConfigHash == "074726ce1df63b58ea21a4ea2892c74bd58815cf591dbbd2adc43c19c9adc418" &&
		receipt.Point == (domain.RoleBinding{MapID: 5, X: 8, Y: 8}) && receipt.ZoneID == 501 &&
		receipt.SourceMapSHA256 == "b40701cea0618ff1bad495f65ffcabab8b9e9b398200fe39424e0e101f6cf832" &&
		receipt.ZoneSetSHA256 == "eabc6551248f156e1881a9ba785d2583c505f150ee3fea4a6fe31118bb60e136" &&
		currentMapHash == "c33708f1c55a3b441e3c4fde4568a45e87c798248bb895ef5de90b33e66ebbe7" &&
		currentZoneHash == "d0c6fd5c9f712efe49b139d14f1051c2e2e6f3f95a03119b0e72b4fcdfb07881"
}
