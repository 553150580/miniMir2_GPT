// 本文件确定讨伐链唯一终态，并按原队员、原运营日和现有运行服代次处理接回与退款。
package persistence

import (
	"cmp"
	"context"
	"math"
	"slices"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	p "minimir/internal/protocol"
)

// DungeonFinish 来自原场景权威执行器。普通失败及超时均用 DungeonFailed。
type DungeonFinish struct {
	ChallengeID   string         // 原队伍扣次的业务键。
	ReservationID string         // 初次扣次确认的场次回执。
	Execution     uint64         // 冻结原场次的执行代次。
	Outcome       DungeonOutcome // 原场景确认的唯一终态。
	FinishedAt    time.Time      // 原终态的业务时刻。
	Members       []Auth         // 各角色原所有者确认的当前场内会话，不能由旧请求补写。
}

// DungeonResume 使用健康原场的确切回执和旧席位会话显式接管新认证绑定。
type DungeonResume struct {
	ChallengeID          string // 原队伍扣次的业务键。
	ReservationID        string // 原队伍的扣次回执。
	Execution            uint64 // 原健康场的执行代次。
	PreviousSessionEpoch int64  // 原席位先前确认的会话代次。
}

// FinishDungeon 在全队原所有权下确认唯一终态。系统中断退原日次数，普通失败等保留扣次。
func (s *Store) FinishDungeon(ctx context.Context, q DungeonFinish) (DungeonChallenge, []domain.Role, error) {
	if len(q.Members) == 0 || len(q.Members) > devconfig.DungeonMaxPlayers {
		return DungeonChallenge{}, nil, Fail(p.CodeInvalidArgument, "讨伐链原队员授权无效")
	}
	return s.finishDungeon(ctx, q)
}

// FinishOwnedDungeon 只接受服务器场次执行器的原终态。编号、预约和执行必须已获当前运行服授权。
// 玩家重连或被封禁不会改变已经发生的全队终态。实际写入始终按原成员永久归属和当前角色版本核对。
func (s *Store) FinishOwnedDungeon(ctx context.Context, q DungeonFinish) (DungeonChallenge, []domain.Role, error) {
	if len(q.Members) != 0 {
		return DungeonChallenge{}, nil, Fail(p.CodeInvalidArgument, "运行服结算只读取原场次成员")
	}
	return s.finishDungeon(ctx, q)
}

// finishDungeon 核对原扣次身份、已确定的结果和全员永久状态，任何成员写入失败均回滚整场。
func (s *Store) finishDungeon(ctx context.Context, q DungeonFinish) (DungeonChallenge, []domain.Role, error) {
	if !validDungeonProof(q.ChallengeID, q.ReservationID, q.Execution) || q.Outcome < DungeonSucceeded || q.Outcome > DungeonSystemInterrupted ||
		q.FinishedAt.IsZero() {
		return DungeonChallenge{}, nil, Fail(p.CodeInvalidArgument, "讨伐链原终态提案无效")
	}
	auths := slices.Clone(q.Members)
	slices.SortFunc(auths, func(left, right Auth) int { return cmp.Compare(left.RoleID, right.RoleID) })
	q.FinishedAt, q.Members = bossTime(q.FinishedAt), nil // 会话可经专用接回推进，终态摘要只保留不可变业务事实。
	digest, err := bossDigest(q)
	if err != nil {
		return DungeonChallenge{}, nil, err
	}
	var result DungeonChallenge
	var roles []domain.Role
	err = s.dungeonPartyTx(ctx, auths, func(tc context.Context) error {
		record, err := s.readDungeon(tc, q.ChallengeID)
		if err != nil {
			return authorityReadError(err, p.CodeOperationConflict, "讨伐链原扣次回执不存在")
		}
		if !s.ownsDungeon(record) || record.ReservationID != q.ReservationID || record.Execution != q.Execution ||
			len(record.Members) == 0 || len(record.Members) > devconfig.DungeonMaxPlayers {
			return Fail(p.CodeOperationConflict, "讨伐链原执行、扣次回执或队员已变化")
		}
		roles, err = s.dungeonFinishRoles(tc, record, auths)
		if err != nil {
			return err
		}
		if record.Outcome != DungeonActive {
			if record.FinishDigest != digest {
				return Fail(p.CodeOperationConflict, "讨伐链原场已确认不同终态")
			}
			result = record.DungeonChallenge
			return nil
		}
		if !validDungeonFinishTime(record.DungeonChallenge, q.Outcome, q.FinishedAt, bossTime(time.Now())) {
			return Fail(p.CodeInvalidArgument, "讨伐链原终态时刻与扣次窗口不符")
		}
		record.Outcome, record.FinishedAt, record.FinishDigest = q.Outcome, q.FinishedAt, digest
		record.Refunded = q.Outcome == DungeonSystemInterrupted
		result = record.DungeonChallenge
		return s.closeDungeon(tc, record, roles)
	})
	if err != nil {
		return DungeonChallenge{}, nil, err
	}
	return result, roles, nil
}

// dungeonFinishRoles 从已持久名单读取对应拥有者。带账号授权的路径还要核对原席位会话。
func (s *Store) dungeonFinishRoles(ctx context.Context, record dungeonRecord, auths []Auth) ([]domain.Role, error) {
	if len(auths) == 0 {
		roles := make([]domain.Role, 0, len(record.Members))
		for _, member := range record.Members {
			role, err := s.Role(ctx, member.RoleID)
			if err != nil {
				return nil, err
			}
			if role.RunServerID != record.RunID || role.RunEpoch != record.RunEpoch || role.Owner != record.OwnerInstance ||
				role.AccountID != member.AccountID || role.Origin != member.Origin {
				return nil, Fail(p.CodeRouteChanged, "讨伐链原角色的运行服或永久归属已变化")
			}
			roles = append(roles, role)
		}
		return roles, nil
	}
	if len(auths) != len(record.Members) {
		return nil, Fail(p.CodeOperationConflict, "讨伐链原执行、扣次回执或队员已变化")
	}
	roles, err := s.dungeonRoles(ctx, auths)
	if err != nil {
		return nil, err
	}
	for i, role := range roles {
		member := record.Members[i]
		if role.ID != member.RoleID || role.AccountID != member.AccountID || role.Origin != member.Origin || auths[i].SessionEpoch != member.SessionEpoch {
			return nil, Fail(p.CodeStaleSession, "讨伐链原席位尚未在当前会话确认接回")
		}
	}
	return roles, nil
}

// ResumeDungeon 不改入场计数。新连接必须已通过游戏票且仍在原运行服持有该角色。
func (s *Store) ResumeDungeon(ctx context.Context, auth Auth, q DungeonResume) (DungeonChallenge, error) {
	if !validDungeonProof(q.ChallengeID, q.ReservationID, q.Execution) || q.PreviousSessionEpoch <= 0 || auth.SessionEpoch <= q.PreviousSessionEpoch {
		return DungeonChallenge{}, Fail(p.CodeInvalidArgument, "讨伐链原接回证明无效")
	}
	var result DungeonChallenge
	err := s.dungeonPartyTx(ctx, []Auth{auth}, func(tc context.Context) error {
		roles, err := s.dungeonRoles(tc, []Auth{auth})
		if err != nil {
			return err
		}
		record, err := s.readDungeon(tc, q.ChallengeID)
		if err != nil {
			return authorityReadError(err, p.CodeOperationConflict, "讨伐链原接回记录不存在")
		}
		if !s.ownsDungeon(record) || record.ReservationID != q.ReservationID || record.Execution != q.Execution || record.Outcome != DungeonActive ||
			dungeonProgress(roles[0]).ActiveChallengeID != q.ChallengeID {
			return Fail(p.CodeOperationConflict, "讨伐链不是当前角色的健康原执行")
		}
		for i := range record.Members {
			member := &record.Members[i]
			if member.RoleID != auth.RoleID || member.AccountID != auth.AccountID || member.Origin != roles[0].Origin {
				continue
			}
			if member.SessionEpoch == auth.SessionEpoch && member.PreviousSessionEpoch == q.PreviousSessionEpoch {
				result = record.DungeonChallenge
				return nil
			}
			if member.SessionEpoch != q.PreviousSessionEpoch || !time.Now().Before(record.ExpiresAt) {
				return Fail(p.CodeStaleSession, "讨伐链原接回会话或场次时限已失效")
			}
			member.SessionEpoch, member.PreviousSessionEpoch = auth.SessionEpoch, q.PreviousSessionEpoch
			result = record.DungeonChallenge
			return s.replaceActiveDungeon(tc, record)
		}
		return Fail(p.CodeStaleSession, "当前角色没有该讨伐链原席位")
	})
	if err != nil {
		return DungeonChallenge{}, err
	}
	return result, nil
}

// RecoverDungeon 只允许较新的同运行服代次处理失去原执行的挑战。未登录角色也按新围栏及当前修订对账。
func (s *Store) RecoverDungeon(ctx context.Context, challengeID string) (DungeonChallenge, []domain.Role, error) {
	if !lowerHex(challengeID, 16) {
		return DungeonChallenge{}, nil, Fail(p.CodeInvalidArgument, "讨伐链恢复场次号无效")
	}
	var result DungeonChallenge
	var roles []domain.Role
	err := s.dungeonPartyTx(ctx, nil, func(tc context.Context) error {
		record, err := s.readDungeon(tc, challengeID)
		if err != nil {
			return authorityReadError(err, p.CodeOperationConflict, "讨伐链原恢复记录不存在")
		}
		if s.Cfg.RunID != record.RunID || record.RunEpoch >= s.Epoch {
			return Fail(p.CodeRouteChanged, "讨伐链原执行尚未失权或不属于当前运行服")
		}
		if record.Outcome != DungeonActive {
			result, roles = record.DungeonChallenge, nil
			return nil
		}
		roles = make([]domain.Role, 0, len(record.Members))
		for _, member := range record.Members {
			role, err := s.Role(tc, member.RoleID)
			if err != nil {
				return err
			}
			if role.RunServerID != record.RunID || role.RunEpoch < record.RunEpoch || role.RunEpoch > s.Epoch ||
				role.RunEpoch == s.Epoch && role.Owner != s.Instance {
				return Fail(p.CodeRouteChanged, "不得向已迁往其他运行服的角色恢复原讨伐链")
			}
			roles = append(roles, role)
		}
		finishedAt := bossTime(time.Now())
		if finishedAt.Before(record.StartedAt) {
			return Fail(p.CodeOperationPending, "当前时间尚不能核对原讨伐链中断")
		}
		record.Outcome, record.FinishedAt = DungeonSystemInterrupted, finishedAt
		record.Refunded, record.Recovered = true, true
		result = record.DungeonChallenge
		return s.closeDungeon(tc, record, roles)
	})
	if err != nil {
		return DungeonChallenge{}, nil, err
	}
	return result, roles, nil
}

func validDungeonProof(id, reservation string, execution uint64) bool {
	return lowerHex(id, 16) && lowerHex(reservation, 16) && execution > 0 && execution <= math.MaxInt64
}

func validDungeonFinishTime(challenge DungeonChallenge, outcome DungeonOutcome, finished, now time.Time) bool {
	if finished.Before(challenge.StartedAt) || finished.After(now) {
		return false
	}
	return outcome == DungeonSystemInterrupted || outcome == DungeonFailed && !finished.After(challenge.ExpiresAt) ||
		outcome == DungeonSucceeded && finished.Before(challenge.ExpiresAt)
}

// closeDungeon 将原终态和全员原日的占用删除或退款一并确认。
func (s *Store) closeDungeon(ctx context.Context, record dungeonRecord, roles []domain.Role) error {
	for i := range roles {
		if err := closeDungeonMember(&roles[i], record.DungeonChallenge, record.Members[i]); err != nil {
			return err
		}
	}
	if err := s.replaceActiveDungeon(ctx, record); err != nil {
		return err
	}
	return s.writeDungeonRoles(ctx, record.DungeonChallenge, roles, "finish")
}

func closeDungeonMember(role *domain.Role, challenge DungeonChallenge, member DungeonMember) error {
	state := dungeonProgress(*role)
	if role.ID != member.RoleID || role.AccountID != member.AccountID || role.Origin != member.Origin || role.Revision == math.MaxInt64 ||
		state.ActiveChallengeID != challenge.ChallengeID || state.EntryDay != member.EntryDay || state.Entries != member.EntriesAfter || state.Entries == 0 {
		return Fail(p.CodeOperationConflict, "讨伐链原角色的占用、扣次及退款账不一致")
	}
	if challenge.Refunded {
		state.Entries--
	}
	state.ActiveChallengeID = ""
	role.Dungeon = &state
	role.Revision++
	return nil
}

func (s *Store) replaceActiveDungeon(ctx context.Context, record dungeonRecord) error {
	filter := bson.M{"_id": record.ChallengeID, "run_server_id": record.RunID, "run_epoch": record.RunEpoch,
		"owner_instance": record.OwnerInstance, "outcome": DungeonActive, "reservation_id": record.ReservationID}
	update, err := s.DB.Collection(dungeonCollection).ReplaceOne(ctx, filter, record)
	if err != nil {
		return err
	}
	if update.MatchedCount != 1 {
		return Fail(p.CodeOperationPending, "讨伐链原场次结束结果待查证")
	}
	return nil
}
