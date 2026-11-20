package persistence

import (
	"math"
	"slices"

	"minimir/internal/devconfig"
	"minimir/internal/domain"
)

// 场景选择库存实例与事务内复核使用同一套自动补蓝资格。
func AutoManaPotionEligible(item domain.Item, job uint32, fixture devconfig.Fixture) bool {
	if fixture.RuleVersion < 24 || len(item.ID) != 32 || item.Locked || item.PendingResultID != "" || item.ReservationID != "" ||
		item.Quantity <= 0 || item.Version <= 0 || item.Version == math.MaxInt64 {
		return false
	}
	effect, hasEffect := fixture.Potions.Effect(item.ItemID)
	definition, known := fixture.Item(item.ItemID)
	return hasEffect && effect.MP > 0 && effect.HP == 0 && known && !definition.IsEquipment &&
		(len(definition.AllowedJobs) == 0 || slices.Contains(definition.AllowedJobs, job))
}
