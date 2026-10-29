package domain

import pb "minimir/api/generated/minimirv1"

const (
	RecycleBlockLocked uint32 = 1 << iota
	RecycleBlockPending
	RecycleBlockReserved
	RecycleBlockWorn
)

// 策略只保存配置ID，不保存随实例移除而失效的保护墓碑。
type AutoRecyclePolicy struct {
	Enabled          bool     `bson:"enabled" json:"enabled"`
	QualityIDs       []uint32 `bson:"quality_ids" json:"quality_ids"`
	MaxRequiredLevel uint32   `bson:"max_required_level" json:"max_required_level"`
	ProtectedItemIDs []uint32 `bson:"protected_item_ids" json:"protected_item_ids"`
	ProtectBound     bool     `bson:"protect_bound" json:"protect_bound"`
	ProtectUnbound   bool     `bson:"protect_unbound" json:"protect_unbound"`
	Revision         int64    `bson:"revision" json:"revision"`
}

func (p AutoRecyclePolicy) Clone() AutoRecyclePolicy {
	p.QualityIDs = append([]uint32(nil), p.QualityIDs...)
	p.ProtectedItemIDs = append([]uint32(nil), p.ProtectedItemIDs...)
	return p
}

func (p AutoRecyclePolicy) Proto() *pb.AutoRecyclePolicy {
	return &pb.AutoRecyclePolicy{Enabled: p.Enabled, QualityIds: append([]uint32(nil), p.QualityIDs...), MaxRequiredLevel: p.MaxRequiredLevel, ProtectedItemIds: append([]uint32(nil), p.ProtectedItemIDs...), ProtectBound: p.ProtectBound, ProtectUnbound: p.ProtectUnbound, Revision: uint64(p.Revision)}
}

func (i Item) RecycleBlockFlags() uint32 {
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

func (r Role) RecycleBlockFlags(i Item) uint32 {
	flags := i.RecycleBlockFlags()
	for _, worn := range r.Equipped {
		if worn.Item.ID == i.ID {
			flags |= RecycleBlockWorn
		}
	}
	return flags
}
