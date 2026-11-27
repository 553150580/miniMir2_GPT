// 本文件读取角色勋章档案，并在当前资产事务内按原版本进行写入。
package persistence

import (
	"context"
	"errors"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"minimir/internal/domain"
	p "minimir/internal/protocol"
)

const roleMedalCollection = "role_medals" // 独立角色勋章档案，主键保证一角一章。

// readRoleMedal 只读取当前永久角色的档案；历史角色缺省代表未激活。
func (s *Store) readRoleMedal(ctx context.Context, role domain.Role) (*domain.MedalState, error) {
	var medal domain.MedalState // 解码后核对的原角色勋章档案。
	err := s.DB.Collection(roleMedalCollection).FindOne(ctx, bson.M{"_id": role.ID}).Decode(&medal)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	_, configured := s.Cfg.Fixture.Medal.Level(medal.Level)
	if medal.OwnerRoleID != role.ID || !lowerHex(role.ID, 16) || !configured ||
		medal.Level == 0 || medal.Level > role.Level || medal.Version <= 0 {
		return nil, Fail(p.CodeOperationConflict, "角色勋章档案的归属或等级无效")
	}
	return &medal, nil
}

// writeRoleMedal 首次激活只允许插入，升级只允许替换同一角色的原版本。
func (s *Store) writeRoleMedal(ctx context.Context, before *domain.MedalState, after domain.MedalState) error {
	collection := s.DB.Collection(roleMedalCollection) // 写入和扣包、回执处于同一个事务。
	if before == nil {
		_, err := collection.InsertOne(ctx, after)
		return err
	}
	result, err := collection.ReplaceOne(ctx, bson.M{"_id": before.OwnerRoleID, "version": before.Version}, after)
	if err != nil {
		return err
	}
	if result.MatchedCount != 1 {
		return Fail(p.CodeOperationConflict, "原勋章档案已变化，请刷新后重新确认")
	}
	return nil
}
