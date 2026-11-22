package persistence

import (
	"context"

	"go.mongodb.org/mongo-driver/v2/bson"
	"minimir/internal/domain"
	p "minimir/internal/protocol"
)

// PreserveRetiredDeath 保存回收后的原死亡选择。待结算的经济结果仍走原有流水。
func (s *Store) PreserveRetiredDeath(ctx context.Context, a Auth, deathID string) error {
	if len(domain.Bytes(deathID)) != 16 {
		return Fail(p.CodeInvalidArgument, "退役房间缺少有效死亡标记")
	}
	return s.guardedTx(ctx, a, func(tc context.Context) error {
		if err := s.fence(tc, a); err != nil {
			return err
		}
		result, err := s.DB.Collection("roles").UpdateOne(tc, bson.M{
			"_id": a.RoleID, "account_id": a.AccountID, "owner_instance": s.Instance,
			"run_epoch": s.Epoch, "session_epoch": a.SessionEpoch,
			"$or": bson.A{
				bson.M{"retired_death_id": bson.M{"$exists": false}}, bson.M{"retired_death_id": deathID},
			},
		}, bson.M{"$set": bson.M{"retired_death_id": deathID}})
		if err != nil {
			return err
		}
		if result.MatchedCount != 1 {
			return Fail(p.CodeOperationConflict, "死亡标记或角色归属已变化")
		}
		return nil
	})
}
