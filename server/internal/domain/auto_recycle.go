// 本文件定义角色保存的自动回收筛选条件及其线协议表示。

package domain

import pb "minimir/api/generated/minimirv1"

const (
	// RecycleBlockLocked 保存根据当前数据与“iota”计算的回收“block”已锁定。
	RecycleBlockLocked uint32 = 1 << iota
	// RecycleBlockPending 记录回收“block”待确认。
	RecycleBlockPending
	// RecycleBlockReserved 记录回收“block”已保留。
	RecycleBlockReserved
	// RecycleBlockWorn 记录回收“block”“worn”。
	RecycleBlockWorn
)

// 策略只保存配置ID，不保存随实例移除而失效的保护墓碑。
type AutoRecyclePolicy struct {
	// Enabled 标记是否处于已开启状态。
	Enabled bool `bson:"enabled" json:"enabled"`
	// QualityIDs 记录品质i“ds”，使用数值列表表示。
	QualityIDs []uint32 `bson:"quality_ids" json:"quality_ids"`
	// MaxRequiredLevel 记录最大“required”等级，使用数值表示。
	MaxRequiredLevel uint32 `bson:"max_required_level" json:"max_required_level"`
	// ProtectedItemIDs 记录“protected”道具i“ds”，使用数值列表表示。
	ProtectedItemIDs []uint32 `bson:"protected_item_ids" json:"protected_item_ids"`
	// ProtectBound 标记是否处于“protect”绑定状态。
	ProtectBound bool `bson:"protect_bound" json:"protect_bound"`
	// ProtectUnbound 标记是否处于“protect”未绑定状态。
	ProtectUnbound bool `bson:"protect_unbound" json:"protect_unbound"`
	// Revision 记录修订号，使用数值表示。
	Revision int64 `bson:"revision" json:"revision"`
}

// Clone 复制当前请求或状态，并把结果交给调用方。
// p 是调用该方法的自动回收策略实例。
func (p AutoRecyclePolicy) Clone() AutoRecyclePolicy {
	p.QualityIDs = append([]uint32(nil), p.QualityIDs...)
	p.ProtectedItemIDs = append([]uint32(nil), p.ProtectedItemIDs...)
	return p
}

// Proto 返回协议供调用方使用。
// p 是调用该方法的自动回收策略实例。
func (p AutoRecyclePolicy) Proto() *pb.AutoRecyclePolicy {
	return &pb.AutoRecyclePolicy{Enabled: p.Enabled, QualityIds: append([]uint32(nil), p.QualityIDs...), MaxRequiredLevel: p.MaxRequiredLevel, ProtectedItemIds: append([]uint32(nil), p.ProtectedItemIDs...), ProtectBound: p.ProtectBound, ProtectUnbound: p.ProtectUnbound, Revision: uint64(p.Revision)}
}

// RecycleBlockFlags 返回回收“block”状态标记集合供调用方使用。
// i 是调用该方法的道具实例。
func (i Item) RecycleBlockFlags() uint32 {
	// flags 记录状态标记集合，使用数值表示。
	var flags uint32
	if i.Locked {
		flags |= RecycleBlockLocked
	}
	if i.PendingResultID != "" {
		flags |= RecycleBlockPending
	}
	if i.ReservationID != "" {
		flags |= RecycleBlockReserved
	}
	return flags
}

// RecycleBlockFlags 返回回收“block”状态标记集合供调用方使用。
// r 是调用该方法的角色实例，i 记录当前位置的下标，使用道具表示。
func (r Role) RecycleBlockFlags(i Item) uint32 {
	// flags 保存由回收“block”状态标记集合取得的状态标记集合。
	flags := i.RecycleBlockFlags()
	// worn 接收遍历已穿戴装备时当前的元素。
	for _, worn := range r.Equipped {
		if worn.Item.ID == i.ID {
			flags |= RecycleBlockWorn
		}
	}
	return flags
}
