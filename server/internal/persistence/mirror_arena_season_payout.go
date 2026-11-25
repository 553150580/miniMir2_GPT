// 本文件按冻结排名与参与标准生成独立附件，再逐行结转兑换分和清理原竞技分。
package persistence

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	p "minimir/internal/protocol"
)

const (
	mirrorRankRewardSource          = "mirror_season_rank"          // 原赛季名次档的附件来源。
	mirrorParticipationRewardSource = "mirror_season_participation" // 原有效挑战次数形成的独立附件。
)

type mirrorSeasonPolicy struct {
	ParticipationMinChallenges uint32                 `bson:"participation_min_challenges"` // 已确认胜负累计次数门槛。
	ParticipationBoundIngot    int64                  `bson:"participation_bound_ingot"`    // 原参与奖绑定元宝最小单位。
	Ranks                      []mirrorSeasonRankTier `bson:"ranks"`                        // 原名次奖表，赛季开始后保持固定。
}

type mirrorSeasonRankTier struct {
	First      uint32 `bson:"first"`       // 含该名次的左侧边界。
	Last       uint32 `bson:"last"`        // 含该名次的右侧边界。
	BoundIngot int64  `bson:"bound_ingot"` // 原绑定元宝最小单位。
}

// MirrorStanding 是单名角色在原冻结或当前竞技榜中的公开投影。
type MirrorStanding struct {
	Rank        uint32    `bson:"rank"`          // 积分降序、最近实战时刻升序后的唯一序号。
	Name        string    `bson:"name"`          // 最近进攻时原角色的显示名。
	Points      int64     `bson:"points"`        // 未清零时的原竞技分。
	Matches     uint32    `bson:"matches"`       // 真正产生胜负且已确认的次数。
	LastMatchAt time.Time `bson:"last_match_at"` // 同分时越早越优先，精确到持久毫秒。
}

// MirrorSeasonReceipt 在竞技分清零后仍完整保留原结转事实。
type MirrorSeasonReceipt struct {
	SeasonID       string    `bson:"season_id"`            // 用于奖品溯源和协议传输的稳定16字节编号。
	SeasonStart    time.Time `bson:"season_start"`         // 原运营赛季起点。
	SeasonEnd      time.Time `bson:"season_end"`           // 原运营赛季终点。
	SettledAt      time.Time `bson:"settled_at,omitempty"` // 原兑换分结转确认时刻。
	Rank           uint32    `bson:"rank"`                 // 该角色的原固定名次。
	Matches        uint32    `bson:"matches"`              // 原赛季有效挑战次数。
	Points         int64     `bson:"points"`               // 排名所用的原积分。
	ExchangeCredit int64     `bson:"exchange_credit"`      // 此来源实际 1:1 进入独立钱包的额度。
}

type mirrorSeasonPlacement struct {
	ID                  string    `bson:"_id"`           // 原赛季范围 + 永久角色。
	Scope               string    `bson:"scope"`         // 结转任务作用域，不跟着后来路由变更。
	Origin              uint64    `bson:"origin"`        // 永久归属，用于跨会话及迁移后读回。
	RoleID              string    `bson:"role_id"`       // 唯一原参赛角色。
	Name                string    `bson:"name"`          // 原角色显示名。
	LastMatch           time.Time `bson:"last_match_at"` // 排名用的最近实战时间。
	MirrorSeasonReceipt `bson:",inline"`
}

func (s *Store) freezeMirrorSeasonPolicy(cfg *devconfig.MirrorArenaSeasonSettlement) (mirrorSeasonPolicy, error) {
	if cfg.RankingTiebreak != devconfig.MirrorArenaRankingEarlierLastChallenge || cfg.ParticipationMinChallenges == 0 || cfg.Participation.BoundYuanbaoUnits <= 0 {
		return mirrorSeasonPolicy{}, Fail(p.CodeOperationConflict, "镜像赛季奖表未明确排名和参与奖")
	}
	policy := mirrorSeasonPolicy{ParticipationMinChallenges: cfg.ParticipationMinChallenges, ParticipationBoundIngot: cfg.Participation.BoundYuanbaoUnits}
	var previous uint32
	for _, rank := range cfg.Ranks {
		if rank.FirstRank <= previous || rank.LastRank < rank.FirstRank || rank.Reward.BoundYuanbaoUnits <= 0 {
			return mirrorSeasonPolicy{}, Fail(p.CodeOperationConflict, "镜像赛季原名次档无效")
		}
		policy.Ranks = append(policy.Ranks, mirrorSeasonRankTier{First: rank.FirstRank, Last: rank.LastRank, BoundIngot: rank.Reward.BoundYuanbaoUnits})
		previous = rank.LastRank
	}
	return policy, nil
}

// mirrorSeasonPublicID 保留旧内部 scope 字符串，另派生适合稳定跨协议传输的附件来源编号。
func mirrorSeasonPublicID(scope string) string { return Hash([]byte("mirror_season:" + scope))[:32] }

// mirrorRankOrder 为实时榜、冻结榜和键集游标提供同一稳定排序。
func mirrorRankOrder() bson.D {
	return bson.D{{Key: "points", Value: -1}, {Key: "last_match_at", Value: 1}, {Key: "role_id", Value: 1}}
}

// mirrorRankBoundary 选择严格排在 row 以前或以后的成绩；完全同分同毫秒用永久角色编号收敛。
func mirrorRankBoundary(row mirrorSeasonScore, before bool) bson.A {
	pointsRelation, matchRelation := "$lt", "$gt"
	if before {
		pointsRelation, matchRelation = "$gt", "$lt"
	}
	return bson.A{
		bson.M{"points": bson.M{pointsRelation: row.Points}},
		bson.M{"points": row.Points, "last_match_at": bson.M{matchRelation: row.LastMatchAt}},
		bson.M{"points": row.Points, "last_match_at": row.LastMatchAt, "role_id": bson.M{matchRelation: row.RoleID}},
	}
}

// rankMirrorSeason 读取最多一批冻结成绩，在同事务写入排序投影、奖品与下次恢复位置。
func (s *Store) rankMirrorSeason(ctx context.Context, journal *mirrorSeasonJournal, now time.Time) error {
	filter := bson.M{"scope": journal.ID, "points": bson.M{"$gt": 0}}
	if journal.LastRanked != nil {
		filter["$or"] = mirrorRankBoundary(*journal.LastRanked, false)
	}
	cursor, err := s.DB.Collection(mirrorSeasonCollection).Find(ctx, filter, options.Find().SetSort(mirrorRankOrder()).SetLimit(mirrorSeasonBatch))
	if err != nil {
		return err
	}
	var scores []mirrorSeasonScore
	if err := cursor.All(ctx, &scores); err != nil {
		return err
	}
	for _, score := range scores {
		if score.ID != journal.ID+":"+score.RoleID || !lowerHex(score.RoleID, 16) || score.Matches == 0 || score.LastMatchAt.IsZero() ||
			score.LastMatchAt.After(journal.SeasonEnd) || journal.Ranked == math.MaxUint32 {
			return Fail(p.CodeOperationConflict, "镜像赛季原积分记录不完整")
		}
		name, err := s.mirrorScoreName(ctx, journal.Origin, score)
		if err != nil {
			return err
		}
		placement := mirrorSeasonPlacement{ID: score.ID, Scope: journal.ID, Origin: journal.Origin, RoleID: score.RoleID, Name: name, LastMatch: score.LastMatchAt,
			MirrorSeasonReceipt: MirrorSeasonReceipt{SeasonID: mirrorSeasonPublicID(journal.ID), SeasonStart: journal.SeasonStart,
				SeasonEnd: journal.SeasonEnd, Rank: journal.Ranked + 1, Matches: score.Matches, Points: score.Points}}
		if _, err := s.DB.Collection(mirrorSeasonRankCollection).InsertOne(ctx, placement); err != nil {
			return err
		}
		if err := s.mirrorSeasonPending(ctx, *journal, placement); err != nil {
			return err
		}
		journal.LastRanked, journal.Ranked = &score, placement.Rank
	}
	journal.NextCheckAt = now
	if len(scores) < mirrorSeasonBatch {
		journal.Phase = mirrorSeasonCrediting
	}
	return nil
}

// mirrorSeasonPending 先发确定的名次附件，再发可能同时满足的独立参与奖附件。
func (s *Store) mirrorSeasonPending(ctx context.Context, journal mirrorSeasonJournal, row mirrorSeasonPlacement) error {
	for _, tier := range journal.Policy.Ranks {
		if tier.First <= row.Rank && row.Rank <= tier.Last {
			if err := s.addMirrorSeasonReward(ctx, journal, row, mirrorRankRewardSource, tier.BoundIngot, row.Rank); err != nil {
				return err
			}
			break
		}
	}
	if row.Matches >= journal.Policy.ParticipationMinChallenges {
		return s.addMirrorSeasonReward(ctx, journal, row, mirrorParticipationRewardSource, journal.Policy.ParticipationBoundIngot, 0)
	}
	return nil
}

func (s *Store) addMirrorSeasonReward(ctx context.Context, journal mirrorSeasonJournal, row mirrorSeasonPlacement, kind string, boundIngot int64, rank uint32) error {
	if boundIngot <= 0 {
		return Fail(p.CodeOperationConflict, "镜像赛季原绑定奖励无效")
	}
	id := Hash(fmt.Appendf(nil, "%s:%s:%s", kind, row.SeasonID, row.RoleID))[:32]
	_, err := s.DB.Collection(mirrorSeasonRewardCollection).InsertOne(ctx, externalChallengeRewardRecord{ID: id, RoleID: row.RoleID,
		ChallengePendingReward: domain.ChallengePendingReward{RewardID: id, SourceKind: kind, SourceID: row.SeasonID, Origin: row.Origin,
			TemplateID: journal.TemplateID, RuleVersion: journal.RuleVersion, EarnedAt: journal.SeasonEnd, SourceRunID: journal.RunID,
			SourceRank: rank, ConfigHash: journal.ConfigHash, MirrorAssets: &domain.MirrorSeasonAssets{BoundIngot: boundIngot}}})
	if err != nil {
		return err
	}
	_, err = s.DB.Collection("audit_outbox").InsertOne(ctx, bson.M{"_id": id + ":pending", "kind": kind,
		"role_id": row.RoleID, "origin": row.Origin, "run_server_id": journal.RunID, "season_id": row.SeasonID,
		"reward_id": id, "bound_yuanbao": boundIngot, "season_rank": row.Rank, "effective_challenges": row.Matches,
		"rule_version": journal.RuleVersion, "config_hash": journal.ConfigHash, "at": journal.SeasonEnd})
	return err
}

// creditMirrorSeason 只在整榜的奖励已保存后消费持久排名，兑换入账确认与原竞技分清零同事务提交。
func (s *Store) creditMirrorSeason(ctx context.Context, journal *mirrorSeasonJournal, now time.Time) error {
	collection := s.DB.Collection(mirrorSeasonRankCollection)
	filter := bson.M{"scope": journal.ID, "rank": bson.M{"$gt": journal.Credited}}
	cursor, err := collection.Find(ctx, filter, options.Find().SetSort(bson.D{{Key: "rank", Value: 1}}).SetLimit(mirrorSeasonBatch))
	if err != nil {
		return err
	}
	var rows []mirrorSeasonPlacement
	if err := cursor.All(ctx, &rows); err != nil {
		return err
	}
	for _, row := range rows {
		if journal.Credited >= journal.Ranked || row.Rank != journal.Credited+1 || row.Origin != journal.Origin || row.Points <= 0 || !row.SettledAt.IsZero() {
			return Fail(p.CodeOperationConflict, "镜像赛季结转指针与冻结榜不符")
		}
		if err := s.creditMirrorExchangePoints(ctx, row.Origin, row.RoleID, journal.ID, row.Points, now); err != nil {
			return err
		}
		write, err := s.DB.Collection(mirrorSeasonCollection).UpdateOne(ctx, bson.M{"_id": row.ID, "scope": journal.ID, "points": row.Points, "matches": row.Matches},
			bson.M{"$set": bson.M{"points": 0, "streak": 0}})
		if err != nil {
			return err
		}
		if write.MatchedCount != 1 {
			return Fail(p.CodeOperationConflict, "待清零的镜像赛季成绩与原冻结榜不符")
		}
		if _, err := collection.UpdateOne(ctx, bson.M{"_id": row.ID}, bson.M{"$set": bson.M{"settled_at": now, "exchange_credit": row.Points}}); err != nil {
			return err
		}
		if _, err := s.DB.Collection("audit_outbox").InsertOne(ctx, bson.M{"_id": row.ID + ":mirror_credit", "kind": "mirror_season_credit",
			"role_id": row.RoleID, "origin": row.Origin, "run_server_id": journal.RunID, "season_id": row.SeasonID,
			"season_rank": row.Rank, "exchange_points": row.Points, "at": now, "rule_version": journal.RuleVersion,
			"config_hash": journal.ConfigHash}); err != nil {
			return err
		}
		journal.Credited++
	}
	if journal.Credited == journal.Ranked {
		journal.Phase, journal.ClosedAt = mirrorSeasonClosed, now
	} else if len(rows) == 0 {
		return Fail(p.CodeOperationConflict, "镜像赛季冻结榜遗漏待结转条目")
	}
	return nil
}

// mirrorScoreName 兼容没有保存显示名的旧成绩，先回原赛季战报，再取原角色资料。
func (s *Store) mirrorScoreName(ctx context.Context, origin uint64, row mirrorSeasonScore) (string, error) {
	if row.Name != "" {
		return row.Name, nil
	}
	var evidence struct {
		Name string `bson:"attacker_name"`
	}
	err := s.DB.Collection(mirrorChallengeCollection).FindOne(ctx, bson.M{"origin": origin, "attacker_role_id": row.RoleID, "finished_at": row.LastMatchAt},
		options.FindOne().SetProjection(bson.M{"attacker_name": 1})).Decode(&evidence)
	if err == nil && evidence.Name != "" {
		return evidence.Name, nil
	}
	if err != nil && !errors.Is(err, mongo.ErrNoDocuments) {
		return "", err
	}
	role, err := s.Role(ctx, row.RoleID)
	if err == nil {
		return role.Name, nil
	}
	if errors.Is(err, mongo.ErrNoDocuments) {
		return "已退役角色", nil
	}
	return "", err
}
