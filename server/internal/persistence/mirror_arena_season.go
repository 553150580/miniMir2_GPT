// 本文件按原运行服租约推进一个有界镜像赛季结算步骤。
package persistence

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"minimir/internal/devconfig"
	p "minimir/internal/protocol"
)

const (
	mirrorSeasonJournalCollection        = "mirror_arena_seasons"        // 赛季关闭工作游标及固定奖表。
	mirrorSeasonRankCollection           = "mirror_arena_season_ranks"   // 原始竞技分清零后仍用于回榜的永久顺位。
	mirrorSeasonRewardCollection         = "mirror_arena_season_rewards" // 固定到永久角色的独立待领取附件。
	mirrorSeasonBatch                    = 8                             // 一次事务最多处理的角色、排名或退次笔数。
	mirrorSeasonGuardGroup        uint64 = runGuardGroups - 1            // 与运行服租约换代竞争的一个固定写屏障。
)

type mirrorSeasonPhase uint32

const (
	mirrorSeasonCollecting mirrorSeasonPhase = iota + 1 // 允许原有效挑战提交结果。
	mirrorSeasonRanking                                 // 原分数冻结，按稳定总序产出名次和奖品。
	mirrorSeasonCrediting                               // 所有名次与待领已确认，开始结转兑换分。
	mirrorSeasonClosed                                  // 每笔原竞技分已结转并清零。
)

type mirrorSeasonJournal struct {
	ID          string             `bson:"_id"`           // 原运行服、永久入口及上海赛季起点。
	RunID       uint64             `bson:"run_server_id"` // 当前可推进的运行服，不接受客户端指定。
	Origin      uint64             `bson:"origin"`        // 保留原匹配及排名分区。
	SeasonStart time.Time          `bson:"season_start"`  // 冻结的上海运营赛季起点。
	SeasonEnd   time.Time          `bson:"season_end"`    // 原赛季停止新挑战的半开边界。
	TemplateID  uint32             `bson:"template_id"`   // 奖励显示及溯源采用原竞技模板。
	RuleVersion uint64             `bson:"rule_version"`  // 首次明确启用此赛季结算规则的版本。
	ConfigHash  string             `bson:"config_hash"`   // 首次明确奖表的配置摘要。
	Phase       mirrorSeasonPhase  `bson:"phase"`         // 当前持久阶段及其严格推进顺序。
	NextCheckAt time.Time          `bson:"next_check_at"` // 有原运行挑战时让其它入口服先推进。
	PlacementAt time.Time          `bson:"placement_at"`  // 原排行冻结时刻。
	ClosedAt    time.Time          `bson:"closed_at"`     // 全批次已有结果的确认时刻。
	Policy      mirrorSeasonPolicy `bson:"policy"`        // 赛季原资格与奖品模板，重启后无需当前刊例。
	Ranked      uint32             `bson:"ranked"`        // 已提交的顺位数。
	LastRanked  *mirrorSeasonScore `bson:"last_ranked"`   // 排名键集游标，原分清零前保持稳定。
	Credited    uint32             `bson:"credited"`      // 已确认兑换分写入及清零的顺位水位。
}

// mirrorSeasonMigration 保存从旧挑战文档拾取赛季窗口的有界迁移游标。
type mirrorSeasonMigration struct {
	ID        string    `bson:"_id"`
	Origin    uint64    `bson:"origin"`
	LastStart time.Time `bson:"last_start"`
	Completed bool      `bson:"completed"`
}

// AdvanceMirrorSeason 从已确认窗口选择一个步骤，不要求参与者仍在线。
// now 只由游戏节点调度器提供，用于判断赛季边界；more 表示还有可继续推进的工作。
func (s *Store) AdvanceMirrorSeason(ctx context.Context, now time.Time) (more bool, err error) {
	cfg := s.Cfg.Fixture.MirrorArena
	if cfg == nil || cfg.Settlement == nil || now.IsZero() {
		return false, nil
	}
	now = bossTime(now)
	collection := s.DB.Collection(mirrorSeasonJournalCollection)
	filter := bson.M{"run_server_id": s.Cfg.RunID, "phase": bson.M{"$lt": mirrorSeasonClosed}, "next_check_at": bson.M{"$lte": now}}
	var due mirrorSeasonJournal
	err = collection.FindOne(ctx, filter, options.FindOne().SetSort(bson.D{{Key: "next_check_at", Value: 1}, {Key: "_id", Value: 1}})).Decode(&due)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return s.enrollLegacyMirrorSeason(ctx, cfg)
	}
	if err != nil {
		return false, err
	}
	err = s.mirrorSeasonTx(ctx, func(tc context.Context) error {
		current, err := mirrorOptional[mirrorSeasonJournal](tc, collection, due.ID)
		if err != nil {
			return err
		}
		more = false
		if current.ID == "" || current.RunID != s.Cfg.RunID || current.NextCheckAt.After(now) || current.Phase == mirrorSeasonClosed {
			return nil
		}
		switch current.Phase {
		case mirrorSeasonCollecting:
			err = s.freezeMirrorSeason(tc, &current, now)
		case mirrorSeasonRanking:
			err = s.rankMirrorSeason(tc, &current, now)
		case mirrorSeasonCrediting:
			err = s.creditMirrorSeason(tc, &current, now)
		default:
			return Fail(p.CodeOperationConflict, "镜像赛季保存阶段无效")
		}
		if err != nil {
			return err
		}
		_, err = collection.ReplaceOne(tc, bson.M{"_id": current.ID}, current)
		more = !current.NextCheckAt.After(now)
		return err
	})
	return more && err == nil, err
}

// mirrorSeasonTx 使用运行服写屏障，不用参与者的账号会话修改独立结转资产。
func (s *Store) mirrorSeasonTx(ctx context.Context, fn func(context.Context) error) error {
	if s.Cfg.RunID == 0 || s.Epoch <= 0 || s.runGuardSlots == nil {
		return Fail(p.CodeRouteChanged, "未取得镜像赛季所属运行服授权")
	}
	ctx, cancel := context.WithTimeout(ctx, transactionTimeout)
	defer cancel()
	slot, err := s.runGuardSlots.acquire(ctx, mirrorSeasonGuardGroup)
	if err != nil {
		return Fail(p.CodeOperationPending, "镜像赛季原步骤结果待查证")
	}
	defer func() { <-slot }()
	return s.tx(ctx, func(tc context.Context) error {
		guard, err := s.runFenceWrite(tc, mirrorSeasonGuardGroup)
		if err != nil {
			return err
		}
		write, err := s.Client.BulkWrite(tc, []mongo.ClientBulkWrite{guard}, options.ClientBulkWrite().SetVerboseResults(true))
		if err != nil {
			return transactionBulkWriteError(err)
		}
		if !write.Acknowledged || write.UpdateResults[0].MatchedCount != 1 {
			return Fail(p.CodeRouteChanged, "镜像赛季运行服写屏障失效")
		}
		return fn(tc)
	})
}

// ensureMirrorSeason 在首次挑战的原事务内记录原赛季时间和原奖表。
func (s *Store) ensureMirrorSeason(ctx context.Context, cfg *devconfig.MirrorArenaConfig, origin uint64, start, end time.Time) error {
	id := mirrorScope(s.Cfg.RunID, origin, start)
	collection := s.DB.Collection(mirrorSeasonJournalCollection)
	old, err := mirrorOptional[mirrorSeasonJournal](ctx, collection, id)
	if err != nil {
		return err
	}
	if old.ID != "" {
		if old.Origin != origin || old.RunID != s.Cfg.RunID || !old.SeasonStart.Equal(start) || !old.SeasonEnd.Equal(end) {
			return Fail(p.CodeOperationConflict, "原镜像赛季时间或归属已变化")
		}
		if old.Phase != mirrorSeasonCollecting {
			return Fail(p.CodeLineForbidden, "原镜像赛季已停止记分")
		}
		return nil
	}
	if cfg.Settlement == nil {
		return nil
	}
	policy, err := s.freezeMirrorSeasonPolicy(cfg.Settlement)
	if err != nil {
		return err
	}
	journal := mirrorSeasonJournal{ID: id, RunID: s.Cfg.RunID, Origin: origin, SeasonStart: start, SeasonEnd: end,
		TemplateID: cfg.TemplateID, RuleVersion: s.Cfg.Fixture.RuleVersion, ConfigHash: s.Cfg.Fixture.Hash,
		Phase: mirrorSeasonCollecting, NextCheckAt: end, Policy: policy}
	_, err = collection.InsertOne(ctx, journal)
	if mongo.IsDuplicateKeyError(err) {
		// 另一个原角色可能同时首次预占此赛季。保留本次挑战身份待查证，整个事务一起重试。
		return Fail(p.CodeOperationPending, "镜像原赛季正在确认首笔预约")
	}
	return err
}

// enrollLegacyMirrorSeason 对每个运行服只扫描一次旧挑战作用域，每轮最多登记一个赛季。
func (s *Store) enrollLegacyMirrorSeason(ctx context.Context, cfg *devconfig.MirrorArenaConfig) (bool, error) {
	id := fmt.Sprintf("migration:%d", s.Cfg.RunID)
	journals := s.DB.Collection(mirrorSeasonJournalCollection)
	marker, err := mirrorOptional[mirrorSeasonMigration](ctx, journals, id)
	if err != nil || marker.Completed {
		return false, err
	}
	more := false
	err = s.mirrorSeasonTx(ctx, func(tc context.Context) error {
		marker, err := mirrorOptional[mirrorSeasonMigration](tc, journals, id)
		if err != nil || marker.Completed {
			return err
		}
		marker.ID = id
		filter := bson.M{"run_server_id": s.Cfg.RunID}
		if marker.Origin != 0 {
			filter["$or"] = bson.A{bson.M{"origin": bson.M{"$gt": marker.Origin}},
				bson.M{"origin": marker.Origin, "season_start": bson.M{"$gt": marker.LastStart}}}
		}
		var witness mirrorRecord
		order := bson.D{{Key: "origin", Value: 1}, {Key: "season_start", Value: 1}}
		err = s.DB.Collection(mirrorChallengeCollection).FindOne(tc, filter, options.FindOne().SetSort(order)).Decode(&witness)
		if errors.Is(err, mongo.ErrNoDocuments) {
			marker.Completed = true
		} else if err != nil {
			return err
		} else {
			if witness.Origin == 0 || witness.SeasonStart.IsZero() || !witness.SeasonEnd.After(witness.SeasonStart) {
				return Fail(p.CodeOperationConflict, "旧镜像挑战没有有效赛季身份")
			}
			scope := mirrorScope(s.Cfg.RunID, witness.Origin, witness.SeasonStart)
			err = journals.FindOne(tc, bson.M{"_id": scope}).Err()
			if errors.Is(err, mongo.ErrNoDocuments) {
				err = s.ensureMirrorSeason(tc, cfg, witness.Origin, witness.SeasonStart, witness.SeasonEnd)
			}
			if err != nil {
				return err
			}
			marker.Origin, marker.LastStart = witness.Origin, witness.SeasonStart
		}
		_, err = journals.ReplaceOne(tc, bson.M{"_id": id}, marker, options.Replace().SetUpsert(true))
		more = !marker.Completed
		return err
	})
	return more && err == nil, err
}

// freezeMirrorSeason 等待原游戏进程在截止内已产生的可核对结算；仅退回已失权的原挑战预占。
func (s *Store) freezeMirrorSeason(ctx context.Context, journal *mirrorSeasonJournal, now time.Time) error {
	if journal.SeasonEnd.After(now) {
		return Fail(p.CodeLineForbidden, "镜像赛季尚未到原结算时间")
	}
	filter := bson.M{"run_server_id": journal.RunID, "origin": journal.Origin, "season_start": journal.SeasonStart, "outcome": MirrorActive}
	cursor, err := s.DB.Collection(mirrorChallengeCollection).Find(ctx, filter, options.Find().SetLimit(mirrorSeasonBatch))
	if err != nil {
		return err
	}
	var active []mirrorRecord
	if err = cursor.All(ctx, &active); err != nil {
		return err
	}
	for _, row := range active {
		if s.ownsMirrorExecution(row) {
			journal.NextCheckAt = now.Add(time.Second)
			continue
		}
		row.Outcome, row.FinishedAt = MirrorInterrupted, now
		if err = s.closeMirrorChallenge(ctx, &row); err != nil {
			return err
		}
	}
	if len(active) != 0 {
		return nil // 下个有界步骤再核对没有在途原场次，避免跳过已确认尾局。
	}
	journal.Phase, journal.PlacementAt, journal.NextCheckAt = mirrorSeasonRanking, now, now
	return nil
}
