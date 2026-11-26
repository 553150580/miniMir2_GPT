// 本文件描述独立于背包和普通穿戴栏的角色专属翅膀实例。
package domain

import pb "minimir/api/generated/minimirv1"

// WingState 独立保存翅膀阶数、外观、幸运及两个分段累计。永久角色编号是该文档的主键。
type WingState struct {
	OwnerRoleID    string `bson:"_id" json:"owner_role_id"`               // 翅膀终身归属的永久角色，与运行服或连接无关。
	ID             string `bson:"wing_id" json:"wing_id"`                 // 首次激活创建的唯一 wing_id。
	Rank           uint32 `bson:"rank" json:"rank"`                       // 当前确认的阶数，实际激活后从一阶开始。
	AppearanceTier uint32 `bson:"appearance_tier" json:"appearance_tier"` // 已解锁并选用的外观进化阶位。
	Hidden         bool   `bson:"hidden" json:"hidden"`                   // 只控制是否显示外观，不影响幸运与属性。
	Luck           uint32 `bson:"luck" json:"luck"`                       // 本实例的当前幸运，首次激活时为零。
	OilToSix       uint32 `bson:"oil_to_six" json:"oil_to_six"`           // 幸运 3→6 阶段累计实际使用的油瓶数。
	OilToNine      uint32 `bson:"oil_to_nine" json:"oil_to_nine"`         // 幸运 6→9 阶段累计实际使用的油瓶数。
	Version        int64  `bson:"version" json:"version"`                 // 角色修改本实例时核对的并发版本。
}

// Proto 发布原实例的当前状态，旧角色未激活时自然省略此观察。
func (w *WingState) Proto() *pb.WingState {
	if w == nil {
		return nil
	}
	return &pb.WingState{WingId: Bytes(w.ID), Rank: w.Rank, AppearanceTier: w.AppearanceTier, Hidden: w.Hidden,
		Luck: w.Luck, OilToSix: w.OilToSix, OilToNine: w.OilToNine, Version: uint64(w.Version)}
}
