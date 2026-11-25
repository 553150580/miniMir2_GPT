// 本文件按原挑战来源查询同一永久角色的独立待领附件，并在资产事务中整包入库。
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

// worldBossRewardPageSize 保留世界首领原附件首屏大小。
const worldBossRewardPageSize = 20

// DungeonPendingRewardPageSize 在个人和世界均满屏时，为同帧讨伐附件保留安全空间。
const DungeonPendingRewardPageSize = 8

// externalChallengeRewardRecord 保存原附件和领取状态，物品及来源事实不随领取改变。
type externalChallengeRewardRecord struct {
	ID                            string           `bson:"_id"`     // 原运行服、场次和永久角色对应的附件键。
	RoleID                        string           `bson:"role_id"` // 原获奖角色，按角色和永久入口共同隔离。
	domain.ChallengePendingReward `bson:",inline"` // 物品、场次、时刻及权益证据。
	Claimed                       bool             `bson:"claimed"`                   // 是否随角色资产事务成功领取。
	ClaimedAt                     time.Time        `bson:"claimed_at,omitempty"`      // 首次领取与资产操作同事务确认的时刻。
	ClaimOperation                string           `bson:"claim_operation,omitempty"` // 首次资产领取操作的永久键。
}

// externalChallengeRewardIndexes 按永久角色和领取状态读取原附件，永久记录不使用 TTL。
func externalChallengeRewardIndexes() []mongo.IndexModel {
	return []mongo.IndexModel{{Keys: bson.D{{Key: "role_id", Value: 1}, {Key: "origin", Value: 1}, {Key: "claimed", Value: 1},
		{Key: "earned_at", Value: 1}, {Key: "_id", Value: 1}}}}
}

// externalChallengeRewardFilter 沿用永久入口，已确认附件可在重启或角色迁服后继续领取。
func externalChallengeRewardFilter(role domain.Role) bson.M {
	return bson.M{"role_id": role.ID, "origin": role.Origin, "claimed": false}
}

// queryExternalChallengeRewards 在原角色读快照中列出此来源最早的附件和后续标记。
func (s *Store) queryExternalChallengeRewards(ctx context.Context, role domain.Role, sourceCollection string, pageSize int) ([]domain.ChallengePendingReward, bool, error) {
	// 限定角色和永久入口再读。当前配置可以关闭新奖励，旧附件仍可展示和领取。
	order := bson.D{{Key: "earned_at", Value: 1}, {Key: "_id", Value: 1}}
	cursor, err := s.DB.Collection(sourceCollection).Find(ctx, externalChallengeRewardFilter(role),
		options.Find().SetSort(order).SetLimit(int64(pageSize)+1))
	if err != nil {
		return nil, false, err
	}
	var rows []domain.ChallengePendingReward // 只投影不随领取改变的原附件。
	if err = cursor.All(ctx, &rows); err != nil {
		return nil, false, err
	}
	if len(rows) > pageSize {
		return rows[:pageSize], true, nil
	}
	return rows, false, nil
}

// claimExternalChallengeReward 领取其他挑战已确认的附件，只在原角色资产事务中调用。
func (s *Store) claimExternalChallengeReward(ctx context.Context, role domain.Role, id, operationID string) (domain.Role, *domain.ChallengePendingReward, uint32, error) {
	filter := externalChallengeRewardFilter(role)
	filter["_id"] = id
	for _, name := range [...]string{worldBossRewardCollection, dungeonRewardCollection, mirrorSeasonRewardCollection} {
		collection := s.DB.Collection(name)
		var row externalChallengeRewardRecord // 玩家只提供附件编号，原物品和归属从库中读取。
		if err := collection.FindOne(ctx, filter).Decode(&row); err != nil {
			if errors.Is(err, mongo.ErrNoDocuments) {
				continue
			}
			return role, nil, 0, err
		}
		mirror := name == mirrorSeasonRewardCollection
		if row.RewardID != id || !validExternalRewardSource(name, row.SourceKind) ||
			mirror && (!validMirrorSeasonAssets(row.MirrorAssets) || len(row.Items) != 0) || !mirror && (row.MirrorAssets != nil || len(row.Items) == 0) {
			return role, nil, 0, Fail(p.CodeOperationConflict, "挑战待领附件的原身份不一致")
		}
		if len(row.Items) > 0 && len(row.Items) > role.Capacity-len(role.Items) {
			return role, nil, p.CodeEquipmentBagFull, nil
		}
		updatedRole := role
		if mirror {
			var err error
			if updatedRole, err = grantMirrorSeasonAssets(role, row.MirrorAssets); err != nil {
				return role, nil, 0, err
			}
		}
		// 标记、入包、唯一回执和审计属于一个事务，满包时在这一步之前返回。
		write, err := collection.UpdateOne(ctx, filter, bson.M{"$set": bson.M{"claimed": true, "claim_operation": operationID,
			"claimed_at": bossTime(time.Now())}})
		if err != nil {
			return role, nil, 0, err
		}
		if write.MatchedCount != 1 {
			return role, nil, 0, Fail(p.CodeOperationPending, "挑战奖励领取结果待查证")
		}
		if len(row.Items) > 0 {
			updatedRole.Items = append(slices.Clone(role.Items), row.Items...)
		}
		return updatedRole, &row.ChallengePendingReward, 0, nil
	}
	return role, nil, p.CodeOperationConflict, nil
}

// validExternalRewardSource 拒绝存储在错误来源集合的旧记录。
func validExternalRewardSource(collection, source string) bool {
	return collection == worldBossRewardCollection && (source == worldBossKillReward || source == worldBossTimeoutReward) ||
		collection == dungeonRewardCollection && source == dungeonRewardSource ||
		collection == mirrorSeasonRewardCollection && (source == mirrorRankRewardSource || source == mirrorParticipationRewardSource)
}
