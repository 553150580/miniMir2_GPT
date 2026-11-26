// 本文件以永久角色主键维护独立翅膀文档，沿角色事务装配和比较原实例。
package persistence

import (
	"context"
	"errors"
	"slices"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	p "minimir/internal/protocol"
)

const roleWingCollection = "role_wings" // 永久角色翅膀；旧二进制不会随角色整档覆盖此表。

// wingIndexes 以永久角色编号主键保证一角一翼，再约束翅膀的实例编号全库唯一。
func wingIndexes() []mongo.IndexModel {
	return []mongo.IndexModel{{Keys: bson.D{{Key: "wing_id", Value: 1}}, Options: options.Index().SetUnique(true)}}
}

// validRoleWing 校验已存实例身份与合法状态。成本缺省的阶位不影响旧实例观察或重放。
func validRoleWing(wing *domain.WingState, owner string, rules *devconfig.WingConfig) bool {
	return wing != nil && rules != nil && wing.OwnerRoleID == owner && lowerHex(owner, 16) && lowerHex(wing.ID, 16) &&
		wing.Rank > 0 && wing.Rank <= rules.MaxRank && wing.Version > 0 && wing.Luck <= 9 &&
		wing.AppearanceTier > 0 && wing.AppearanceTier <= wing.Rank && slices.Contains(rules.AppearanceTiers, wing.AppearanceTier) &&
		(wing.Luck >= 3 || wing.OilToSix == 0) && (wing.Luck >= 6 || wing.OilToNine == 0)
}

// readRoleWing 在已知的永久角色归属下读取一个独立实例，历史角色缺省代表未激活。
func (s *Store) readRoleWing(ctx context.Context, owner string) (*domain.WingState, error) {
	var wing domain.WingState
	err := s.DB.Collection(roleWingCollection).FindOne(ctx, bson.M{"_id": owner}).Decode(&wing)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !validRoleWing(&wing, owner, s.Cfg.Fixture.Wings) {
		return nil, Fail(p.CodeOperationConflict, "原翅膀实例的身份或状态无效")
	}
	return &wing, nil
}

// writeRoleWing 只能在当前角色资产事务内写入。旧版本、其他角色和其他实例不会被覆盖。
func (s *Store) writeRoleWing(ctx context.Context, before *domain.WingState, after domain.WingState) error {
	collection := s.DB.Collection(roleWingCollection)
	if before == nil {
		_, err := collection.InsertOne(ctx, after)
		return err
	}
	filter := bson.M{"_id": before.OwnerRoleID, "wing_id": before.ID, "version": before.Version}
	result, err := collection.ReplaceOne(ctx, filter, after)
	if err != nil {
		return err
	}
	if result.MatchedCount != 1 {
		return Fail(p.CodeOperationConflict, "原翅膀版本已改变，请刷新后重新确认")
	}
	return nil
}
