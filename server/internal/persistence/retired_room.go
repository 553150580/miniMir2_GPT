// 回收已死亡房间前的持久标记。仅保留死亡身份，便于冷重入继续原复活选择。

package persistence

import (
	"context"

	"go.mongodb.org/mongo-driver/v2/bson"
	"minimir/internal/domain"
	p "minimir/internal/protocol"
)

// PreserveRetiredDeath 保存回收后的原死亡选择。待结算的经济结果仍走原有流水。
// PreserveRetiredDeath 在角色仍由本会话持有时保存原死亡身份，以便回收房间后继续复活。
// s 是调用该方法的持久层存储实例，ctx 向下传递当前请求的截止时间和取消信号，a 保存本次调用使用的账号或角色授权，deathID 标识当前业务使用的死亡。
func (s *Store) PreserveRetiredDeath(ctx context.Context, a Auth, deathID string) error {
	if len(domain.Bytes(deathID)) != 16 {
		return Fail(p.CodeInvalidArgument, "退役房间缺少有效死亡标记")
	}
	// tc 记录MongoDB 事务上下文，使用请求或事务上下文表示。
	return s.guardedTx(ctx, a, func(tc context.Context) error {
		// err 接收权限围栏过程中可能产生的错误，供后续失败分支处理。
		if err := s.fence(tc, a); err != nil {
			return err
		}
		// 保存退役标记的条件也要标明当前运行服。同节点的两个运行服可能恰好拥有相同运行代次。
		filter := bson.M{
			"_id": a.RoleID, "account_id": a.AccountID, "owner_instance": s.Instance,
			"run_server_id": s.Cfg.RunID, "run_epoch": s.Epoch, "session_epoch": a.SessionEpoch,
			"$or": bson.A{
				bson.M{"retired_death_id": bson.M{"$exists": false}}, bson.M{"retired_death_id": deathID},
			},
		}
		if s.acceptsLegacyRoleScope() {
			filter["run_server_id"] = bson.M{"$in": bson.A{s.Cfg.RunID, 0, nil}}
		}
		result, err := s.DB.Collection("roles").UpdateOne(tc, filter, bson.M{"$set": bson.M{"retired_death_id": deathID}})
		if err != nil {
			return err
		}
		if result.MatchedCount != 1 {
			return Fail(p.CodeOperationConflict, "死亡标记或角色归属已变化")
		}
		return nil
	})
}
