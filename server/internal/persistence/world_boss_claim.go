// 本文件查询同一永久角色的世界首领待领附件，并在资产事务中完成整笔领取。
package persistence

import (
	"context"
	"errors"
	"slices"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"minimir/internal/domain"
	p "minimir/internal/protocol"
)

// worldBossPendingPageSize 限制同一查询返回的附件数量，领取后旧附件移出首屏。
const worldBossPendingPageSize = 20

// worldBossRewardIndexes 按永久角色和待领状态读取最早的原奖励，永久附件不使用 TTL。
func worldBossRewardIndexes() []mongo.IndexModel {
	return []mongo.IndexModel{{Keys: bson.D{{Key: "role_id", Value: 1}, {Key: "origin", Value: 1}, {Key: "claimed", Value: 1},
		{Key: "earned_at", Value: 1}, {Key: "_id", Value: 1}}}}
}

// worldBossPendingFilter 沿用角色永久入口，已确认附件可随角色在冷重启或路由调整后领取。
func worldBossPendingFilter(role domain.Role) bson.M {
	return bson.M{"role_id": role.ID, "origin": role.Origin, "claimed": false}
}

// queryPendingWorldBossRewards 在原角色只读快照中列出最早二十笔，以及背后是否仍有附件。
func (s *Store) queryPendingWorldBossRewards(ctx context.Context, role domain.Role) ([]domain.ChallengePendingReward, bool, error) {
	// 限定角色和永久入口再读。当前配置可以关闭新奖励，旧附件仍可展示和领取。
	order := bson.D{{Key: "earned_at", Value: 1}, {Key: "_id", Value: 1}}
	cursor, err := s.DB.Collection(worldBossRewardCollection).Find(ctx, worldBossPendingFilter(role),
		options.Find().SetSort(order).SetLimit(worldBossPendingPageSize+1))
	if err != nil {
		return nil, false, err
	}
	var rows []domain.ChallengePendingReward // 只投影不随领取改变的原附件。
	if err = cursor.All(ctx, &rows); err != nil {
		return nil, false, err
	}
	if len(rows) > worldBossPendingPageSize {
		return rows[:worldBossPendingPageSize], true, nil
	}
	return rows, false, nil
}

// claimWorldBossReward 领取已确认附件。该方法只在原角色资产事务内调用。
func (s *Store) claimWorldBossReward(ctx context.Context, role domain.Role, id, operationID string) (domain.Role, *domain.ChallengePendingReward, uint32, error) {
	filter := worldBossPendingFilter(role)
	filter["_id"] = id
	collection := s.DB.Collection(worldBossRewardCollection)
	var row worldBossRewardRecord // 原发放时冻结的物品和归属，玩家只提供附件编号。
	if err := collection.FindOne(ctx, filter).Decode(&row); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return role, nil, p.CodeOperationConflict, nil
		}
		return role, nil, 0, err
	}
	if row.RewardID != id || len(row.Items) == 0 || row.SourceKind != worldBossKillReward && row.SourceKind != worldBossTimeoutReward {
		return role, nil, 0, Fail(p.CodeOperationConflict, "世界首领待领附件的原身份不一致")
	}
	if len(row.Items) > role.Capacity-len(role.Items) {
		return role, nil, p.CodeEquipmentBagFull, nil
	}
	// 标记、入包、唯一回执和审计属于一个事务，满包时在这一步之前返回。
	write, err := collection.UpdateOne(ctx, filter, bson.M{"$set": bson.M{"claimed": true, "claim_operation": operationID,
		"claimed_at": worldBossTime(time.Now())}})
	if err != nil {
		return role, nil, 0, err
	}
	if write.MatchedCount != 1 {
		return role, nil, 0, Fail(p.CodeOperationPending, "世界首领奖励领取结果待查证")
	}
	role.Items = append(slices.Clone(role.Items), row.Items...)
	return role, &row.ChallengePendingReward, 0, nil
}
