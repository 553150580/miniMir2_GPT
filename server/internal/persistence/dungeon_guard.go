// 本文件为讨伐链多角色事务串行占用现有围栏组并复查各成员授权。
package persistence

import (
	"context"
	"maps"
	"slices"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"minimir/internal/devconfig"
	p "minimir/internal/protocol"
)

// dungeonPartyTx 同事务核对原运行服和所有成员账号。没有在线成员授权时仅允许调用受控恢复事务。
func (s *Store) dungeonPartyTx(ctx context.Context, auths []Auth, fn func(context.Context) error) error {
	if s.Cfg.RunID == 0 || s.Epoch <= 0 || s.runGuardSlots == nil {
		return Fail(p.CodeRouteChanged, "没有讨伐链所属运行服的授权")
	}
	if len(auths) > devconfig.DungeonMaxPlayers {
		return Fail(p.CodeInvalidArgument, "讨伐链成员超过上限")
	}
	accounts := make(map[string]Auth, len(auths))
	groups := make([]uint64, 0, max(1, len(auths)))
	for _, auth := range auths {
		if !lowerHex(auth.AccountID, 16) || !lowerHex(auth.RoleID, 16) || auth.AuthVersion <= 0 || auth.SessionEpoch <= 0 {
			return Fail(p.CodeInvalidArgument, "讨伐链成员授权标识无效")
		}
		if other, exists := accounts[auth.AccountID]; exists && auth.AuthVersion != other.AuthVersion {
			return Fail(p.CodeUnauthenticated, "同账号队员的授权代次不一致")
		}
		accounts[auth.AccountID] = auth
		groups = append(groups, runGuardGroup(auth.AccountID))
	}
	if len(groups) == 0 {
		groups = append(groups, 0) // 无账号请求的旧场次恢复使用固定运行服组。
	}
	accountIDs := slices.Sorted(maps.Keys(accounts))
	slices.Sort(groups)
	groups = slices.Compact(groups)
	ctx, cancel := context.WithTimeout(ctx, transactionTimeout)
	defer cancel()
	// 所有多方事务逐一按固定组序占用围栏，不会与另一支队伍互等更加靠前的组。
	for _, group := range groups {
		slot, err := s.runGuardSlots.acquire(ctx, group)
		if err != nil {
			return Fail(p.CodeOperationPending, "讨伐链原事务结果待查证")
		}
		defer func() { <-slot }()
	}
	return s.tx(ctx, func(tc context.Context) error {
		writes := make([]mongo.ClientBulkWrite, 0, len(groups)+len(accounts))
		for _, group := range groups {
			guard, err := s.runFenceWrite(tc, group)
			if err != nil {
				return err
			}
			writes = append(writes, guard)
		}
		for _, id := range accountIDs {
			auth := accounts[id]
			writes = append(writes, mongo.ClientBulkWrite{Database: s.DB.Name(), Collection: "accounts", Model: mongo.NewClientUpdateOneModel().
				SetFilter(accountGuardFilter(auth)).SetUpdate(bson.M{"$inc": bson.M{"auth_guard_seq": 1}})})
		}
		written, err := s.Client.BulkWrite(tc, writes, options.ClientBulkWrite().SetVerboseResults(true))
		if err != nil {
			return transactionBulkWriteError(err)
		}
		if !written.Acknowledged {
			return Fail(p.CodeOperationPending, "未确认讨伐链写屏障")
		}
		for index := range writes {
			if written.UpdateResults[index].MatchedCount != 1 {
				if index < len(groups) {
					return Fail(p.CodeRouteChanged, "讨伐链写屏障已失效")
				}
				return Fail(p.CodeUnauthenticated, "讨伐链成员账号授权已失效")
			}
		}
		return fn(tc)
	})
}
