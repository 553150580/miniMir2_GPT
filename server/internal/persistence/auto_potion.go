// 自动补蓝的持久库存准入规则。房间选药和扣药事务共同用它核对配置、实例状态与职业资格。

package persistence

import (
	"math"
	"slices"

	"minimir/internal/devconfig"
	"minimir/internal/domain"
)

// 场景选择库存实例与事务内复核使用同一套自动补蓝资格。
// item 记录道具，job 记录任务或职业编号，使用数值表示，fixture 记录开发测试配置。
func AutoManaPotionEligible(item domain.Item, job uint32, fixture devconfig.Fixture) bool {
	if fixture.RuleVersion < 24 || len(item.ID) != 32 || item.Locked || item.PendingResultID != "" || item.ReservationID != "" ||
		item.Quantity <= 0 || item.Version <= 0 || item.Version == math.MaxInt64 {
		return false
	}
	// effect 接收效果供后续步骤使用，hasEffect 保存由效果取得的“has”效果。
	effect, hasEffect := fixture.Potions.Effect(item.ItemID)
	// definition 保存由道具取得的定义，known 保存由道具取得的已知状态。
	definition, known := fixture.Item(item.ItemID)
	return hasEffect && effect.MP > 0 && effect.HP == 0 && known && !definition.IsEquipment &&
		(len(definition.AllowedJobs) == 0 || slices.Contains(definition.AllowedJobs, job))
}
