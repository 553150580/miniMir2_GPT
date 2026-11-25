// 本文件提供有界当前排名、本人准确名次及可跨会话读取的原赛季结转回执。
package persistence

import (
	"context"
	"errors"
	"math"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"minimir/internal/domain"
	p "minimir/internal/protocol"
)

const mirrorRankingPageSize = 20      // 首屏只展示固定数量；本人名次独立查询，结转处理全部角色。
const mirrorSeasonRewardsPageSize = 8 // 每个战报响应限制附带旧赛季奖励数。

// readMirrorSeasonView 在 lobby 的原角色快照事务内读取榜、个人名次和确认后的结转。
func (s *Store) readMirrorSeasonView(ctx context.Context, r domain.Role, scope string, current mirrorSeasonScore, out *MirrorInfo) error {
	collection := s.DB.Collection(mirrorSeasonCollection)
	filter := bson.M{"scope": scope, "points": bson.M{"$gt": 0}}
	cursor, err := collection.Find(ctx, filter, options.Find().SetSort(mirrorRankOrder()).SetLimit(mirrorRankingPageSize))
	if err != nil {
		return err
	}
	var scores []mirrorSeasonScore
	if err := cursor.All(ctx, &scores); err != nil {
		return err
	}
	for index, row := range scores {
		name, err := s.mirrorScoreName(ctx, r.Origin, row)
		if err != nil {
			return err
		}
		standing := MirrorStanding{Rank: uint32(index + 1), Name: name, Points: row.Points, Matches: row.Matches, LastMatchAt: row.LastMatchAt}
		out.Rankings = append(out.Rankings, standing)
		if row.RoleID == r.ID {
			out.OwnRank = standing.Rank
		}
	}
	if out.OwnRank == 0 && current.Points > 0 {
		filter["$or"] = mirrorRankBoundary(current, true)
		before, err := collection.CountDocuments(ctx, filter)
		if err != nil {
			return err
		}
		if before >= math.MaxUint32 {
			return Fail(p.CodeOperationConflict, "镜像赛季排名超出显示范围")
		}
		out.OwnRank = uint32(before + 1)
	}
	var latest mirrorSeasonPlacement
	filter = bson.M{"origin": r.Origin, "role_id": r.ID, "settled_at": bson.M{"$exists": true}}
	order := bson.D{{Key: "season_end", Value: -1}, {Key: "_id", Value: -1}}
	err = s.DB.Collection(mirrorSeasonRankCollection).FindOne(ctx, filter, options.FindOne().SetSort(order)).Decode(&latest)
	if err == nil {
		out.LatestSettlement = &latest.MirrorSeasonReceipt
	} else if !errors.Is(err, mongo.ErrNoDocuments) {
		return err
	}
	out.SeasonPendingRewards, out.SeasonPendingRewardsMore, err = s.queryExternalChallengeRewards(ctx, r, mirrorSeasonRewardCollection, mirrorSeasonRewardsPageSize)
	return err
}

// mirrorSeasonIndexes 为队列、原角色查询和整榜有序回放建立有限的持久索引。
func mirrorSeasonIndexes() map[string][]mongo.IndexModel {
	return map[string][]mongo.IndexModel{
		mirrorSeasonJournalCollection: {{Keys: bson.D{{Key: "run_server_id", Value: 1}, {Key: "next_check_at", Value: 1}, {Key: "_id", Value: 1}},
			Options: options.Index().SetPartialFilterExpression(bson.M{"phase": bson.M{"$lt": mirrorSeasonClosed}})}},
		mirrorSeasonRankCollection: {
			{Keys: bson.D{{Key: "scope", Value: 1}, {Key: "rank", Value: 1}}, Options: options.Index().SetUnique(true)},
			{Keys: bson.D{{Key: "origin", Value: 1}, {Key: "role_id", Value: 1}, {Key: "season_end", Value: -1}, {Key: "_id", Value: -1}},
				Options: options.Index().SetPartialFilterExpression(bson.M{"settled_at": bson.M{"$exists": true}})},
		},
		mirrorSeasonRewardCollection: externalChallengeRewardIndexes(),
	}
}
