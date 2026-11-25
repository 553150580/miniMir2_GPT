// 本文件只在入场事务内转交仍有原执行现场的个人首领预约。
package persistence

import (
	"context"

	"go.mongodb.org/mongo-driver/v2/bson"
	"minimir/internal/domain"
	p "minimir/internal/protocol"
)

// personalBossResume 包含原分片提供的受保护会话与预约身份，客户端票据不携带这些字段。
type personalBossResume struct {
	previous Auth                  // 本次换会话前仍能确认永久结果的角色授权。
	grant    PersonalBossChallenge // 本地活场持有的原永久预约。
}

// transferPersonalBossSession 在票据事务内仅迁移原预约的会话围栏，保留执行代次与全部原事实。
func (s *Store) transferPersonalBossSession(ctx context.Context, role domain.Role, resume personalBossResume) error {
	grant := resume.grant
	if err := s.checkRole(role, resume.previous); err != nil {
		return err
	}
	if grant.ChallengeID == "" || grant.RoleID != role.ID || grant.RunID != s.Cfg.RunID ||
		role.PersonalBoss == nil || role.PersonalBoss.ActiveChallengeID != grant.ChallengeID {
		return Fail(p.CodeOperationConflict, "原个人首领预约已变化")
	}
	filter := bson.M{"_id": grant.ChallengeID, "role_id": role.ID, "run_server_id": s.Cfg.RunID, "run_epoch": s.Epoch,
		"owner_instance": s.Instance, "session_epoch": role.SessionEpoch, "outcome": PersonalBossActive,
		"reservation_id": grant.ReservationID, "execution": grant.Execution, "rule_version": grant.RuleVersion, "config_hash": grant.ConfigHash,
		"combat_snapshot": grant.CombatSnapshot}
	changed, err := s.DB.Collection(personalBossCollection).UpdateOne(ctx, filter, bson.M{"$set": bson.M{"session_epoch": role.SessionEpoch + 1}})
	if err != nil {
		return err
	}
	if changed.MatchedCount != 1 {
		return Fail(p.CodeOperationConflict, "个人首领原执行无法接回")
	}
	return nil
}
