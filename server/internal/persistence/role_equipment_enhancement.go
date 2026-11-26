// 本文件维护独立的角色部位等级，确保换装、托管及旧进程整档写入期间等级仍有效。
package persistence

import (
	"context"
	"errors"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"minimir/internal/domain"
	p "minimir/internal/protocol"
)

// roleEquipmentEnhancementCollection 的唯一主键是永久角色 ID。
const roleEquipmentEnhancementCollection = "role_equipment_enhancements"

// readRoleEquipmentEnhancement 兼容无强化记录的角色，拒绝不完整或超出原规则的等级。
func (s *Store) readRoleEquipmentEnhancement(ctx context.Context, role domain.Role) (*domain.EquipmentEnhancement, error) {
	collection := s.DB.Collection(roleEquipmentEnhancementCollection)
	raw, err := collection.FindOne(ctx, bson.M{"_id": role.ID}).Raw()
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	state := new(domain.EquipmentEnhancement)
	levels, isArray := raw.Lookup("levels").ArrayOK()
	if bson.Unmarshal(raw, state) != nil || !isArray || state.RoleID != role.ID || state.Version <= 0 || state.Version > role.Revision {
		return nil, Fail(p.CodeOperationConflict, "角色装备部位强化档案无效")
	}
	values, err := levels.Values()
	if err != nil || len(values) != domain.EquipmentEnhancementSlots {
		return nil, Fail(p.CodeOperationConflict, "角色装备部位强化部位数量无效")
	}
	for slot, level := range state.Levels {
		value, valid := values[slot].Int64OK()
		if asInt32, ok := values[slot].Int32OK(); ok {
			value, valid = int64(asInt32), true
		}
		if !valid || value != int64(level) || level > role.Level || int(level) > len(s.Cfg.Fixture.EquipmentEnhancement.Levels) {
			return nil, Fail(p.CodeOperationConflict, "角色装备部位强化等级无效")
		}
	}
	return state, nil
}

// writeRoleEquipmentEnhancement 在同一角色资产事务中以原版本更新独立成长档案。
func (s *Store) writeRoleEquipmentEnhancement(ctx context.Context, before *domain.EquipmentEnhancement, after domain.EquipmentEnhancement) error {
	collection := s.DB.Collection(roleEquipmentEnhancementCollection)
	if before == nil {
		_, err := collection.InsertOne(ctx, after)
		return err
	}
	result, err := collection.ReplaceOne(ctx, bson.M{"_id": after.RoleID, "version": before.Version}, after)
	if err != nil {
		return err
	}
	if result.MatchedCount != 1 {
		return Fail(p.CodeOperationConflict, "原角色部位强化版本已变化")
	}
	return nil
}
