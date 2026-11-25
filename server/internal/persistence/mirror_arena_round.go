// 本文件在原角色所属运行服确认镜像预约、健康接回、唯一终态和故障退次。
package persistence

import (
	"context"
	"errors"
	"math"
	"slices"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"minimir/internal/domain"
	p "minimir/internal/protocol"
)

// BeginMirrorChallenge 只接受原角色本日获得的凭证，预占次数与原挑战回执同事务提交。
func (s *Store) BeginMirrorChallenge(ctx context.Context, a Auth, req MirrorBegin) (MirrorChallenge, error) {
	var out MirrorChallenge
	if !lowerHex(req.ChallengeID, 16) || !lowerHex(req.OfferID, 16) {
		return out, Fail(p.CodeInvalidArgument, "镜像挑战缺少原操作号或当日凭证")
	}
	err := s.guardedTx(ctx, a, func(tc context.Context) error {
		out = MirrorChallenge{}
		r, err := s.mirrorRole(tc, a)
		if err != nil {
			return err
		}
		coll := s.DB.Collection(mirrorChallengeCollection)
		prior, err := mirrorOptional[mirrorRecord](tc, coll, req.ChallengeID)
		if err != nil {
			return err
		}
		if prior.ChallengeID != "" {
			if !sameMirrorAttacker(prior, r) || prior.RunID != s.Cfg.RunID || prior.OfferID != req.OfferID {
				return Fail(p.CodeOperationConflict, "原镜像挑战的角色、运行服或候选不一致")
			}
			out = prior.MirrorChallenge
			if prior.Outcome == MirrorActive {
				digest, err := bossDigest(req)
				if err != nil || digest != prior.BeginDigest || !s.ownsMirrorExecution(prior) || prior.SessionEpoch != r.SessionEpoch {
					return Fail(p.CodeLineRecovering, "原镜像活动需要同一现场接回或中断对账")
				}
				defense, err := s.readMirrorDefense(tc, prior.DefenseID)
				if err != nil || defense.Version != prior.DefenseVersion || defense.RoleID != prior.DefenderRoleID || defense.Origin != prior.Origin {
					if err != nil {
						return err
					}
					return Fail(p.CodeOperationConflict, "原镜像活动的防守快照无法核对")
				}
				out.AttackerSnapshot, out.DefenseSnapshot = req.AttackerSnapshot, defense.Snapshot
			}
			return nil
		}
		if req.Execution == 0 || req.Execution > math.MaxInt64 || req.ExpectedRevision != r.Revision ||
			!validMirrorSnapshot(&s.Cfg.Fixture, r, req.AttackerSnapshot) {
			return Fail(p.CodeOperationConflict, "攻方现场执行或原角色属性版本无效")
		}
		now := bossTime(time.Now())
		cfg, seasonStart, seasonEnd, err := s.currentMirrorSeason(now)
		if err != nil {
			return err
		}
		expires := bossTime(now.Add(MirrorPrepareWindow + cfg.Duration))
		if !expires.Before(seasonEnd) {
			return Fail(p.CodeLineForbidden, "本赛季即将结算，剩余窗口不足以完成一场挑战")
		}
		if dungeonProgress(r).ActiveChallengeID != "" || personalBossProgress(r).ActiveChallengeID != "" {
			return Fail(p.CodeLineForbidden, "已有其它独占挑战")
		}
		if err := s.checkMirrorRoleFree(tc, r); err != nil {
			return err
		}
		scope := mirrorScope(s.Cfg.RunID, r.Origin, seasonStart)
		dayKey := mirrorDayKey(scope, r.ID, now)
		day, err := mirrorOptional[mirrorDay](tc, s.DB.Collection(mirrorDayCollection), dayKey)
		if err != nil {
			return err
		}
		f := &s.Cfg.Fixture
		offerIndex := slices.IndexFunc(day.Offers, func(offer mirrorOfferRecord) bool { return offer.ID == req.OfferID })
		if offerIndex < 0 || day.RuleVersion != f.RuleVersion || day.ConfigHash != f.Hash {
			return Fail(p.CodeLineForbidden, "镜像凭证不属于原角色本日的有效候选")
		}
		if day.Entries >= cfg.DailyLimit {
			return Fail(p.CodeLineForbidden, "镜像竞技当日次数已用完")
		}
		offer := day.Offers[offerIndex]
		defense, err := s.readMirrorDefense(tc, offer.DefenseID)
		if err != nil {
			return err
		}
		if defense.ID != offer.DefenseID || defense.RoleID != offer.DefenderRoleID || defense.RoleID == r.ID || defense.AccountID == r.AccountID ||
			defense.AccountID == "" || defense.Origin != r.Origin || defense.RunID != s.Cfg.RunID || !defense.SeasonStart.Equal(seasonStart) ||
			defense.ConfigHash != f.Hash || defense.RuleVersion != f.RuleVersion || defense.Version != offer.DefenseVersion ||
			!cfg.AllowsMatch(day.RatedJob, day.RatedLevel, defense.Snapshot.Job, defense.Snapshot.Level) {
			return Fail(p.CodeOperationConflict, "原候选的防守身份或已固定版本无法确认")
		}
		scoreKey := scope + ":" + r.ID
		score, err := mirrorOptional[mirrorSeasonScore](tc, s.DB.Collection(mirrorSeasonCollection), scoreKey)
		if err != nil {
			return err
		}
		if score.Streak == math.MaxUint32 {
			return Fail(p.CodeOperationConflict, "原进攻连胜版本已达上限")
		}
		if err := s.ensureMirrorSeason(tc, cfg, r.Origin, seasonStart, seasonEnd); err != nil {
			return err
		}
		// 原角色与当前确认运行服共同作用域化客户端操作号和所选凭证。
		// 持久保存原请求指纹与防守摘要，重试绝不再次预占或替换属性。
		out = MirrorChallenge{MirrorSummary: MirrorSummary{ChallengeID: req.ChallengeID, OfferID: offer.ID,
			AttackerRoleID: r.ID, DefenderRoleID: defense.RoleID, AttackerName: r.Name, DefenderName: defense.Snapshot.Name,
			DefenseVersion: defense.Version, StartedAt: now}, ReservationID: domain.ID(), Execution: req.Execution,
			RunID: s.Cfg.RunID, TemplateID: cfg.TemplateID, RuleVersion: f.RuleVersion, ConfigHash: f.Hash,
			AttackerSnapshot: req.AttackerSnapshot, DefenseSnapshot: defense.Snapshot, ExpiresAt: expires,
			SeasonStart: seasonStart, SeasonEnd: seasonEnd, Duration: cfg.Duration, SeasonPoints: score.Points, Streak: score.Streak}
		digest, err := bossDigest(req)
		if err != nil {
			return err
		}
		record := mirrorRecord{MirrorChallenge: out, Origin: r.Origin, OwnerInstance: s.Instance, RunEpoch: s.Epoch,
			SessionEpoch: r.SessionEpoch, BeginDigest: digest, DayKey: dayKey, ScoreKey: scoreKey, DefenseID: defense.ID,
			WinPoints: cfg.WinPoints(score.Streak + 1), LossPoints: cfg.LossPoints}
		if _, err := coll.InsertOne(tc, record); err != nil {
			if mongo.IsDuplicateKeyError(err) {
				return Fail(p.CodeOperationConflict, "镜像挑战操作号已使用")
			}
			return err
		}
		if _, err := s.DB.Collection(mirrorActiveCollection).InsertOne(tc, mirrorActive{ID: mirrorRoleKey(r.Origin, r.ID), ChallengeID: out.ChallengeID}); err != nil {
			if mongo.IsDuplicateKeyError(err) {
				return Fail(p.CodeLineForbidden, "镜像竞技已有原活动场次")
			}
			return err
		}
		if err := s.changeMirrorEntries(tc, dayKey, day.Entries, 1); err != nil {
			return err
		}
		return s.writeMirrorAudit(tc, record, "begin")
	})
	return out, err
}

// FinishMirrorChallenge 验证原角色会话、准备窗口和执行回执，固定胜负最多结一次积分。
func (s *Store) FinishMirrorChallenge(ctx context.Context, a Auth, req MirrorFinish) (MirrorChallenge, error) {
	var out MirrorChallenge
	if !validMirrorProof(req.ChallengeID, req.ReservationID, req.Execution) ||
		req.Outcome < MirrorWon || req.Outcome > MirrorInterrupted || req.FinishedAt.IsZero() {
		return out, Fail(p.CodeInvalidArgument, "镜像挑战结束证明无效")
	}
	req.FinishedAt = bossTime(req.FinishedAt)
	now := bossTime(time.Now())
	digest, err := bossDigest(req)
	if err != nil {
		return out, err
	}
	err = s.guardedTx(ctx, a, func(tc context.Context) error {
		out = MirrorChallenge{}
		r, err := s.mirrorRole(tc, a)
		if err != nil {
			return err
		}
		record, err := s.readMirrorRecord(tc, req.ChallengeID)
		if err != nil {
			return err
		}
		if !sameMirrorAttacker(record, r) || !s.ownsMirrorExecution(record) || record.SessionEpoch != r.SessionEpoch ||
			record.ReservationID != req.ReservationID || record.Execution != req.Execution {
			return Fail(p.CodeOperationConflict, "镜像原会话、执行或预占证明不符")
		}
		if record.Outcome != MirrorActive {
			if record.FinishDigest != digest {
				return Fail(p.CodeOperationConflict, "原挑战已有与请求不同的终态")
			}
			out = record.MirrorChallenge
			return nil
		}
		if req.FinishedAt.Before(record.StartedAt) || req.FinishedAt.After(now) ||
			req.Outcome != MirrorInterrupted && req.FinishedAt.After(record.ExpiresAt) {
			return Fail(p.CodeInvalidArgument, "原镜像挑战终态时间超出有效窗口")
		}
		record.Outcome, record.FinishedAt, record.FinishDigest = req.Outcome, req.FinishedAt, digest
		if err := s.closeMirrorChallenge(tc, &record); err != nil {
			return err
		}
		out = record.MirrorChallenge
		return nil
	})
	return out, err
}

// ResumeMirrorChallenge 仅推进同一健康现场的角色会话，不重置时限、属性或次数。
func (s *Store) ResumeMirrorChallenge(ctx context.Context, a Auth, req MirrorResume) (MirrorChallenge, error) {
	var out MirrorChallenge
	if !validMirrorProof(req.ChallengeID, req.ReservationID, req.Execution) || req.PreviousSessionEpoch <= 0 || a.SessionEpoch <= req.PreviousSessionEpoch {
		return out, Fail(p.CodeInvalidArgument, "镜像挑战接回的原执行和会话无效")
	}
	err := s.guardedTx(ctx, a, func(tc context.Context) error {
		out = MirrorChallenge{}
		r, err := s.mirrorRole(tc, a)
		if err != nil {
			return err
		}
		record, err := s.readMirrorRecord(tc, req.ChallengeID)
		if err != nil {
			return err
		}
		if !sameMirrorAttacker(record, r) || !s.ownsMirrorExecution(record) || record.ReservationID != req.ReservationID ||
			record.Execution != req.Execution {
			return Fail(p.CodeOperationConflict, "镜像原预约无法健康接回")
		}
		if record.SessionEpoch == a.SessionEpoch && record.PreviousSessionEpoch == req.PreviousSessionEpoch {
			out = record.MirrorChallenge
			return nil
		}
		if record.SessionEpoch != req.PreviousSessionEpoch {
			return Fail(p.CodeOperationConflict, "镜像原会话已被其他执行接管")
		}
		if record.Outcome != MirrorActive {
			// 原结算可能刚提交但旧连接没有收到结果。向同一新票返回既有终态，不推进终态文档。
			out = record.MirrorChallenge
			return nil
		}
		result, err := s.DB.Collection(mirrorChallengeCollection).UpdateOne(tc,
			bson.M{"_id": record.ChallengeID, "outcome": MirrorActive, "session_epoch": req.PreviousSessionEpoch},
			bson.M{"$set": bson.M{"session_epoch": a.SessionEpoch, "previous_session_epoch": req.PreviousSessionEpoch}})
		if err != nil {
			return err
		}
		if result.MatchedCount != 1 {
			return Fail(p.CodeOperationConflict, "原镜像挑战绑定已变化")
		}
		out = record.MirrorChallenge
		return nil
	})
	return out, err
}

// RecoverMirrorChallenge 由已确认没有健康原现场的新执行只回收未终结预占，不重演战斗。
func (s *Store) RecoverMirrorChallenge(ctx context.Context, a Auth) (MirrorChallenge, error) {
	var out MirrorChallenge
	now := bossTime(time.Now())
	err := s.guardedTx(ctx, a, func(tc context.Context) error {
		out = MirrorChallenge{}
		r, err := s.mirrorRole(tc, a)
		if err != nil {
			return err
		}
		active, err := mirrorOptional[mirrorActive](tc, s.DB.Collection(mirrorActiveCollection), mirrorRoleKey(r.Origin, r.ID))
		if err != nil || active.ChallengeID == "" {
			return err
		}
		record, err := s.readMirrorRecord(tc, active.ChallengeID)
		if err != nil {
			return err
		}
		if !sameMirrorAttacker(record, r) || record.RunID != s.Cfg.RunID || record.Outcome != MirrorActive || now.Before(record.StartedAt) ||
			s.ownsMirrorExecution(record) && r.SessionEpoch <= record.SessionEpoch {
			return Fail(p.CodeOperationConflict, "原镜像活动仍有效或不属于本运行服")
		}
		record.Outcome, record.FinishedAt = MirrorInterrupted, now
		if err := s.closeMirrorChallenge(tc, &record); err != nil {
			return err
		}
		out = record.MirrorChallenge
		return nil
	})
	return out, err
}

func sameMirrorAttacker(record mirrorRecord, role domain.Role) bool {
	return record.Origin == role.Origin && record.AttackerRoleID == role.ID
}

func (s *Store) ownsMirrorExecution(record mirrorRecord) bool {
	return record.RunID == s.Cfg.RunID && record.RunEpoch == s.Epoch && record.OwnerInstance == s.Instance
}

func validMirrorProof(challenge, reservation string, execution uint64) bool {
	return lowerHex(challenge, 16) && lowerHex(reservation, 16) && execution != 0 && execution <= math.MaxInt64
}

func (s *Store) readMirrorRecord(ctx context.Context, id string) (mirrorRecord, error) {
	var record mirrorRecord
	err := s.DB.Collection(mirrorChallengeCollection).FindOne(ctx, bson.M{"_id": id}).Decode(&record)
	if errors.Is(err, mongo.ErrNoDocuments) {
		err = Fail(p.CodeOperationConflict, "未获得原镜像竞技场次")
	}
	return record, err
}

func (s *Store) readMirrorDefense(ctx context.Context, id string) (MirrorDefense, error) {
	defense, err := mirrorOptional[MirrorDefense](ctx, s.DB.Collection(mirrorDefenseCollection), id)
	if err != nil {
		return MirrorDefense{}, err
	}
	if digest, err := bossDigest(defense.Snapshot); defense.ID == "" || err != nil || defense.Digest != digest {
		return MirrorDefense{}, Fail(p.CodeOperationConflict, "原防守快照内容无法按原指纹核对")
	}
	return defense, nil
}

// closeMirrorChallenge 在同一事务竞争原唯一终态、释放活跃占位，并入积分或退还原日次数。
func (s *Store) closeMirrorChallenge(ctx context.Context, record *mirrorRecord) error {
	if record.Outcome == MirrorInterrupted {
		day, err := mirrorOptional[mirrorDay](ctx, s.DB.Collection(mirrorDayCollection), record.DayKey)
		if err != nil {
			return err
		}
		if day.ID == "" || day.Entries == 0 {
			return Fail(p.CodeOperationConflict, "镜像原预占运营日已缺少合法次数")
		}
		if err := s.changeMirrorEntries(ctx, day.ID, day.Entries, -1); err != nil {
			return err
		}
	} else if err := s.writeMirrorMatchScore(ctx, record); err != nil {
		return err
	}
	result, err := s.DB.Collection(mirrorChallengeCollection).ReplaceOne(ctx, bson.M{"_id": record.ChallengeID, "outcome": MirrorActive,
		"execution": record.Execution, "session_epoch": record.SessionEpoch}, *record)
	if err != nil {
		return err
	}
	if result.MatchedCount != 1 {
		return Fail(p.CodeOperationConflict, "原镜像挑战已有其它唯一终态")
	}
	cleared, err := s.DB.Collection(mirrorActiveCollection).DeleteOne(ctx, bson.M{"_id": mirrorRoleKey(record.Origin, record.AttackerRoleID), "challenge_id": record.ChallengeID})
	if err != nil {
		return err
	}
	if cleared.DeletedCount != 1 {
		return Fail(p.CodeOperationConflict, "原角色的独占竞技挑战已变化")
	}
	return s.writeMirrorAudit(ctx, *record, "finish")
}

// writeMirrorMatchScore 只更新原攻方并沿用已冻结的获胜与失败分，防守者的进攻连胜保持。
func (s *Store) writeMirrorMatchScore(ctx context.Context, record *mirrorRecord) error {
	scope := mirrorScope(record.RunID, record.Origin, record.SeasonStart)
	journal, err := mirrorOptional[mirrorSeasonJournal](ctx, s.DB.Collection(mirrorSeasonJournalCollection), scope)
	if err != nil {
		return err
	}
	if journal.ID != "" && journal.Phase != mirrorSeasonCollecting {
		return Fail(p.CodeOperationConflict, "原镜像赛季已冻结排名")
	}
	collection := s.DB.Collection(mirrorSeasonCollection)
	score, err := mirrorOptional[mirrorSeasonScore](ctx, collection, record.ScoreKey)
	if err != nil {
		return err
	}
	if score.Points != record.SeasonPoints || score.Streak != record.Streak || score.Matches == math.MaxUint32 ||
		!score.LastMatchAt.IsZero() && record.FinishedAt.Before(score.LastMatchAt) {
		return Fail(p.CodeOperationConflict, "原赛季积分或连胜水位已变化")
	}
	points := record.LossPoints
	streak := uint32(0)
	if record.Outcome == MirrorWon {
		points, streak = record.WinPoints, score.Streak+1
	}
	total, err := add(score.Points, points)
	if err != nil {
		return err
	}
	record.Points, record.SeasonPoints, record.Streak = points, total, streak
	score.ID, score.RoleID = record.ScoreKey, record.AttackerRoleID
	score.Scope, score.Name = scope, record.AttackerName
	score.Points, score.Streak, score.Matches, score.LastMatchAt = total, streak, score.Matches+1, record.FinishedAt
	_, err = collection.ReplaceOne(ctx, bson.M{"_id": score.ID}, score, options.Replace().SetUpsert(true))
	return err
}

func (s *Store) changeMirrorEntries(ctx context.Context, day string, original uint32, delta int) error {
	result, err := s.DB.Collection(mirrorDayCollection).UpdateOne(ctx, bson.M{"_id": day, "entries": original}, bson.M{"$inc": bson.M{"entries": delta}})
	if err != nil {
		return err
	}
	if result.MatchedCount != 1 {
		return Fail(p.CodeOperationConflict, "原上海运营日镜像挑战次数已变化")
	}
	return nil
}

func (s *Store) writeMirrorAudit(ctx context.Context, record mirrorRecord, phase string) error {
	_, err := s.DB.Collection("audit_outbox").InsertOne(ctx, bson.M{"_id": "mirror:" + record.ChallengeID + ":" + phase,
		"kind": "mirror_" + phase, "role_id": record.AttackerRoleID, "origin": record.Origin, "run_server_id": record.RunID,
		"challenge_id": record.ChallengeID, "defender_role_id": record.DefenderRoleID, "defense_version": record.DefenseVersion,
		"outcome": record.Outcome, "points": record.Points, "entry_day_key": record.DayKey, "at": time.Now()})
	return err
}
