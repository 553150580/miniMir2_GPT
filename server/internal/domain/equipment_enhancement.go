// 本文件定义角色拥有的十个装备强化等级及其协议观察。
package domain

import pb "minimir/api/generated/minimirv1"

// EquipmentEnhancementSlots 使用穿戴栏编号。翅膀、勋章和施法耗材没有对应编号。
const EquipmentEnhancementSlots = 10

// EquipmentEnhancement 保存独立角色成长。角色整档写入和旧二进制均不会覆盖这份资料。
type EquipmentEnhancement struct {
	RoleID  string                            `bson:"_id" json:"-"`         // 原角色永久身份。
	Version int64                             `bson:"version" json:"-"`     // 最近一次成功强化时的角色修订。
	Levels  [EquipmentEnhancementSlots]uint32 `bson:"levels" json:"levels"` // 按原装备栏顺序排列的十个强化等级。
}

// EnhancementLevel 返回角色部位的已确认等级。没有历史记录的角色由零开始。
func (r Role) EnhancementLevel(slot uint32) uint32 {
	if r.EquipmentEnhancement == nil || slot >= EquipmentEnhancementSlots {
		return 0
	}
	return r.EquipmentEnhancement.Levels[slot]
}

// EnhancementProto 仅由开放强化的规则调用，空角色也会得到十个可辨认的部位。
func (r Role) EnhancementProto() []*pb.EquipmentEnhancementSlot {
	result := make([]*pb.EquipmentEnhancementSlot, EquipmentEnhancementSlots)
	for slot := uint32(0); slot < EquipmentEnhancementSlots; slot++ {
		result[slot] = &pb.EquipmentEnhancementSlot{Slot: slot, Level: r.EnhancementLevel(slot)}
	}
	return result
}
