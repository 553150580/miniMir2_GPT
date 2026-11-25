// 本文件在原运行服围栏下查证并撤销未安装战斗的讨伐链扣次。
package persistence

import (
	"context"
	"errors"
	"time"

	"go.mongodb.org/mongo-driver/v2/mongo"
	"minimir/internal/domain"
	p "minimir/internal/protocol"
)

// AbortUnattachedDungeon 由原开战协调者在确定未安装战斗后调用。
// 沿用原扣次事务的运行服写围栏，查不到账时原扣次事务也不能迟到生效。
func (s *Store) AbortUnattachedDungeon(ctx context.Context, q DungeonBegin) (DungeonChallenge, []domain.Role, error) {
	q, err := normalizeDungeonBegin(q)
	if err != nil {
		return DungeonChallenge{}, nil, err
	}
	digest, err := bossDigest(q)
	if err != nil {
		return DungeonChallenge{}, nil, err
	}
	auths := make([]Auth, len(q.Members))
	for i, member := range q.Members {
		auths[i] = member.Auth
	}
	var result DungeonChallenge
	var roles []domain.Role
	err = s.dungeonPartyRunTx(ctx, auths, false, func(tc context.Context) error {
		// 事务回调可能在出现写冲突后重试，只有本次读取的事实可以作为最终回执。
		result, roles = DungeonChallenge{}, nil
		record, err := s.readDungeon(tc, q.ChallengeID)
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil
		}
		if err != nil {
			return err
		}
		if !s.ownsDungeon(record) || record.BeginDigest != digest || len(record.Members) != len(q.Members) {
			return Fail(p.CodeOperationConflict, "讨伐链撤销未匹配原扣次请求")
		}
		if record.Outcome != DungeonActive && (record.Outcome != DungeonSystemInterrupted || !record.Refunded) {
			return Fail(p.CodeOperationConflict, "讨伐链原场已确认其他终态")
		}
		roles, err = s.dungeonFinishRoles(tc, record, nil)
		if err != nil {
			return err
		}
		if record.Outcome == DungeonSystemInterrupted {
			result = record.DungeonChallenge
			return nil
		}
		at := bossTime(time.Now())
		if at.Before(record.StartedAt) {
			return Fail(p.CodeOperationPending, "当前时间尚不能确认原开战中断")
		}
		proposal := DungeonFinish{ChallengeID: record.ChallengeID, ReservationID: record.ReservationID, Execution: record.Execution,
			Outcome: DungeonSystemInterrupted, FinishedAt: at}
		record.FinishDigest, err = bossDigest(proposal)
		if err != nil {
			return err
		}
		record.Outcome, record.FinishedAt, record.Refunded = proposal.Outcome, at, true
		result = record.DungeonChallenge
		return s.closeDungeon(tc, record, roles)
	})
	if err != nil {
		return DungeonChallenge{}, nil, err
	}
	return result, roles, nil
}
