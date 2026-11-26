// 本文件为实际手动分解的原装备生成并核对祝福油，保存逐件抽样和原产物身份。
package persistence

import (
	"crypto/rand"
	"encoding/hex"
	"math"

	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	p "minimir/internal/protocol"
)

const equipmentRecycleOilSource = "equipment_recycle" // 油携带被销毁装备的原实例号作为来源。

// RecycleOilRoll 冻结一个实际分解原件的幸运、抽样和最多一瓶原产物。
type RecycleOilRoll struct {
	EquipmentInstanceID string       `bson:"equipment_instance_id"` // 已销毁的原装备实例。
	Luck                uint32       `bson:"luck"`                  // 实际用于判定的当前幸运。
	Denominator         uint32       `bson:"denominator"`           // 抽样取值范围的排他上界。
	Draw                uint32       `bson:"draw"`                  // 服务端抽中的整数，仅在永久账本可查。
	Oil                 *domain.Item `bson:"oil,omitempty"`         // 命中后唯一入包的原油，不命中时为空。
}

// recycleOilAttempt 在一次资产事务的全部驱动重试之间保留原身份与抽样种子。
type recycleOilAttempt struct {
	operationID string // 现有永久操作主键，固定产油身份的来源。
	seed        string // 只在首次需要抽样时生成，既不保存也不发送给客户端。
}

// roll 使用私有种子和原装备身份，避免请求顺序改变或 MongoDB 重试造成重复抽样。
func (a *recycleOilAttempt) roll(itemID string, denominator uint32) (uint32, error) {
	if a.seed == "" {
		var bytes [32]byte
		if _, err := rand.Read(bytes[:]); err != nil {
			return 0, err
		}
		a.seed = hex.EncodeToString(bytes[:])
	}
	return sourceDraw("equipment-recycle-oil:draw:"+a.seed+":"+a.operationID+":"+itemID, denominator), nil
}

// validEquipmentRecycleOil 验证本阶段新来源的单瓶资产，不把其他目录、可合并的假油或伪原件放行。
func validEquipmentRecycleOil(item domain.Item, definition devconfig.Item) bool {
	return item.SourceKind == equipmentRecycleOilSource && item.ItemID == definition.ID && definition.EquipmentRecycleMaterial &&
		!definition.IsEquipment && definition.Stackable && definition.MaxStack >= 1 && item.Quantity == 1 &&
		item.Version > 0 && item.Version < math.MaxInt64 && lowerHex(item.ID, 16) && lowerHex(item.SourceID, 16) && item.ID != item.SourceID &&
		item.Quality == 0 && item.OrePurity == 0 && item.InitialLuck == 0 && item.CurrentLuck == 0 &&
		item.LuckOilToSix == 0 && item.LuckOilToNine == 0 && item.DurabilityU == 0 && item.MaxDurabilityU == 0 && item.OriginalDurabilityU == 0
}

// equipmentRecycleLuck 只读取原装备部位允许的幸运范围。非武器只能使用初始幸运。
func equipmentRecycleLuck(item domain.Item, f *devconfig.Fixture) uint32 {
	definition, found := f.Item(item.ItemID)
	if !found || !definition.IsEquipment || definition.EquipmentSlot > 9 || item.InitialLuck > 3 {
		return 0
	}
	if definition.EquipmentSlot != devconfig.WeaponSlot {
		return item.InitialLuck
	}
	luck := item.EffectiveLuck()
	if luck < item.InitialLuck || luck > weaponOilMaxLuck {
		return 0
	}
	return luck
}

// applyManualRecycleOil 给已经移除原装备的背包追加各自独立的油瓶，既有油不会被跨来源合并。
func applyManualRecycleOil(before domain.Role, out *domain.Role, receipt *EquipmentRecyclingReceipt, f *devconfig.Fixture, attempt *recycleOilAttempt) error {
	rules := f.EquipmentRecycleOil
	if rules == nil {
		return nil
	}
	if f.LuckOil == nil || rules.Denominator == 0 || attempt == nil || attempt.operationID == "" {
		return Fail(p.CodeOperationConflict, "分解祝福油来源规则不可用")
	}
	definition, found := f.Item(f.LuckOil.ItemID)
	if !found || !definition.EquipmentRecycleMaterial || definition.IsEquipment || !definition.Stackable || definition.MaxStack < 1 {
		return Fail(p.CodeOperationConflict, "分解祝福油物品目录不可用")
	}
	for _, item := range receipt.Recycled {
		luck := equipmentRecycleLuck(item, f)
		if luck == 0 || !item.HasValidDurability() || item.DurabilityU <= 0 {
			continue
		}
		draw, err := attempt.roll(item.ID, rules.Denominator)
		if err != nil {
			return err
		}
		fact := RecycleOilRoll{EquipmentInstanceID: item.ID, Luck: luck, Denominator: rules.Denominator, Draw: draw}
		if draw < luck {
			id := Hash([]byte("equipment-recycle-oil:item:" + attempt.operationID + ":" + item.ID))[:32]
			oil := domain.Item{ID: id, ItemID: definition.ID, Quantity: 1, Version: 1, Bound: item.Bound,
				SourceKind: equipmentRecycleOilSource, SourceID: item.ID}
			if !validEquipmentRecycleOil(oil, definition) || before.HasItemInstance(id) || out.HasItemInstance(id) {
				return Fail(p.CodeOperationConflict, "分解产物与已有实例冲突")
			}
			if len(out.Items) >= out.Capacity {
				return Fail(p.CodeEquipmentBagFull, "分解后的背包没有产物位置")
			}
			out.Items = append(out.Items, oil)
			fact.Oil = &oil
		}
		receipt.OilRolls = append(receipt.OilRolls, fact)
	}
	return nil
}

// oilProto 只发送已确认的原件幸运及产物。原抽样和分母留在服务端审计。
func (r *EquipmentRecyclingReceipt) oilProto() *pb.RecycleOilReceipt {
	if r == nil || len(r.OilRolls) == 0 {
		return nil
	}
	result := &pb.RecycleOilReceipt{Rolls: make([]*pb.RecycleOilRoll, 0, len(r.OilRolls))}
	for _, fact := range r.OilRolls {
		view := &pb.RecycleOilRoll{EquipmentInstanceId: domain.Bytes(fact.EquipmentInstanceID), Luck: fact.Luck}
		if fact.Oil != nil {
			view.Oil = fact.Oil.Proto()
		}
		result.Rolls = append(result.Rolls, view)
	}
	return result
}
