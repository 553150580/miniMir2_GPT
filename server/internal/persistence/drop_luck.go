// 本文件只为已经确认去向的原怪物掉落装备抽取初始幸运。
package persistence

import (
	"math"
	"strconv"

	"minimir/internal/devconfig"
	"minimir/internal/domain"
)

const dropLuckPPM uint32 = 1_000_000 // 初始幸运触发率按百万分率计。

// settleDropInitialLuck 使用独立的来源种子，保持原怪物、掉落概率、额度和保底的抽样不变。
// 额度降档共用原实例编号，即使实际发放档位发生变化仍对应同一个初始幸运抽样。
func settleDropInitialLuck(roleID string, q Settlement, f *devconfig.Fixture, receipt *SettlementReceipt) *domain.Item {
	item, rules := q.Item, f.CombatLuck
	if rules == nil || rules.DropPPM == 0 || rules.DropPPM > dropLuckPPM || item == nil || q.Kills <= 0 ||
		item.Quantity != 1 || item.Version != 1 || item.InitialLuck != 0 || item.SourceKind != ordinaryEquipmentItemSource ||
		q.SourceSeq <= 0 || item.SourceID != strconv.FormatInt(q.SourceSeq, 10) || !lowerHex(item.ID, 16) {
		return item
	}
	definition, exists := f.Item(item.ItemID)
	if !exists || !definition.IsEquipment {
		return item
	}
	seed := "minimir:equipment-drop-initial-luck:v1:" + roleID + ":" + item.SourceID + ":" + item.ID
	if sourceDraw(seed+":trigger", dropLuckPPM) >= rules.DropPPM {
		return item
	}
	var total uint64 // 配置已校验，旧数据或不完整夹具仍不能使抽样上限溢出。
	if len(rules.InitialWeights) != 3 {
		return item
	}
	for _, weight := range rules.InitialWeights {
		if weight == 0 {
			return item
		}
		total += uint64(weight)
	}
	if total > math.MaxUint32 {
		return item
	}
	roll := sourceDraw(seed+":value", uint32(total))
	for index, weight := range rules.InitialWeights {
		if roll < weight {
			lucky := *item
			lucky.InitialLuck = uint32(index + 1)
			receipt.LuckyDrop = &lucky
			return &lucky
		}
		roll -= weight
	}
	return item
}
