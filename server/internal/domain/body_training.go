// 本文件保存角色生命、魔法两个独立的已完成炼体等级。
package domain

import (
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
)

// BodyTrainingState 随原角色聚合事务保存，旧档从两个零等级开始。
type BodyTrainingState struct {
	Health uint32 `bson:"health" json:"health"` // 已完成的生命系等级。
	Mana   uint32 `bson:"mana" json:"mana"`     // 已完成的魔法系等级。
}

// BodyTrainingLevels 复制当前状态，事务计算不改动原角色聚合。
func (r Role) BodyTrainingLevels() BodyTrainingState {
	if r.BodyTraining == nil {
		return BodyTrainingState{}
	}
	return *r.BodyTraining
}

// Level 返回原系别的已完成等级。
func (s BodyTrainingState) Level(track uint32) uint32 {
	switch track {
	case devconfig.BodyHealth:
		return s.Health
	case devconfig.BodyMana:
		return s.Mana
	}
	return 0
}

// Advance 记录一次已经确认成功的单系升级。
func (s BodyTrainingState) Advance(track uint32) *BodyTrainingState {
	switch track {
	case devconfig.BodyHealth:
		s.Health++
	case devconfig.BodyMana:
		s.Mana++
	}
	return &s
}

// Proto 生成同一库存修订的两系状态。
func (s BodyTrainingState) Proto() *pb.BodyTrainingState {
	return &pb.BodyTrainingState{HealthLevel: s.Health, ManaLevel: s.Mana}
}
