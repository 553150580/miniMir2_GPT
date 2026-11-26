// 本文件定义装备本身保存的固定槽词条，并投影已付待选到原实例界面。
package domain

import pb "minimir/api/generated/minimirv1"

const EquipmentRefineSlots = 4 // 传说装备的最高槽数，低品质只开放前面的槽。

// EquipmentAffix 保存一槽采纳或候选的定点值。空槽三个字段均为零。
type EquipmentAffix struct {
	AttributeID uint32 `bson:"attribute_id"` // 稳定词条目录编号。
	Tier        uint32 `bson:"tier"`         // 五档中的实际档次。
	ValueUnits  int64  `bson:"value_units"`  // 原实际抽样值，隔离表使用十分之一固定单位。
}

// EquipmentAffixes 使用值类型数组，交易转移、清单核对和快照均可复制比较。
type EquipmentAffixes [EquipmentRefineSlots]EquipmentAffix

// EquipmentRefineState 属于原件，和原件版本同一事务迁移。待选编号沿用物品的通用占用字段。
type EquipmentRefineState struct {
	Current    EquipmentAffixes `bson:"current"`     // 已采纳的左侧词条，只有这些参与战斗。
	Candidate  EquipmentAffixes `bson:"candidate"`   // 已付费最新右侧词条，没有时为零。
	LockedMask uint32           `bson:"locked_mask"` // 上次生成时保留的左侧槽位。
}

// IsZero 兼容历史装备，没有新养成时不额外保存字段。
func (r EquipmentRefineState) IsZero() bool { return r == (EquipmentRefineState{}) }

// Proto 保留槽的位置，空装备用缺省字段而非展示伪造词条。
func (affixes EquipmentAffixes) Proto() []*pb.EquipmentRefineAffix {
	var out []*pb.EquipmentRefineAffix
	for slot, affix := range affixes {
		if affix.AttributeID != 0 {
			out = append(out, &pb.EquipmentRefineAffix{Slot: uint32(slot), AttributeId: affix.AttributeID, Tier: affix.Tier, ValueUnits: affix.ValueUnits})
		}
	}
	return out
}

// RefineLockedSlots 将位图转换成位置稳定的协议槽序。
func RefineLockedSlots(mask uint32) []uint32 {
	var slots []uint32
	for slot := uint32(0); slot < EquipmentRefineSlots; slot++ {
		if mask&(1<<slot) != 0 {
			slots = append(slots, slot)
		}
	}
	return slots
}

// refineProto 只公开真实已采纳或已付候选，不暴露任何随机种子。
func (i Item) refineProto() ([]*pb.EquipmentRefineAffix, *pb.EquipmentRefinePending) {
	current := i.Refine.Current.Proto()
	if i.PendingResultID == "" || i.Refine.Candidate == (EquipmentAffixes{}) {
		return current, nil
	}
	return current, &pb.EquipmentRefinePending{PendingId: Bytes(i.PendingResultID), Affixes: i.Refine.Candidate.Proto(),
		LockedSlots: RefineLockedSlots(i.Refine.LockedMask)}
}
