// 本文件描述独立于背包和普通穿戴栏的角色勋章档案。
package domain

import pb "minimir/api/generated/minimirv1"

// MedalState 以永久角色编号作为唯一主键，保存已确认等级和并发版本。
type MedalState struct {
	OwnerRoleID string `bson:"_id" json:"owner_role_id"` // 勋章终身所属的永久角色编号。
	Level       uint32 `bson:"level" json:"level"`       // 从首次激活起已经确认的勋章等级。
	Version     int64  `bson:"version" json:"version"`   // 报价和事务共同校验的档案版本。
}

// Proto 将已激活勋章同步给装备面板，历史角色未激活时保留缺省状态。
func (m *MedalState) Proto() *pb.MedalState {
	if m == nil {
		return nil
	}
	return &pb.MedalState{Level: m.Level, Version: uint64(m.Version)}
}

// MedalLevel 提供战斗计算使用的已确认等级，未激活角色返回零。
func (r Role) MedalLevel() uint32 {
	if r.Medal == nil {
		return 0
	}
	return r.Medal.Level
}
