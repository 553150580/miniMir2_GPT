// 本文件为两个市场的离线持久结算提供相同的运行服及账号分组围栏。
package persistence

import (
	"context"
	"slices"

	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	p "minimir/internal/protocol"
)

// marketSystemTx 按固定顺序占用原账号组，并在同一事务内实际写入当前租约围栏。
func (s *Store) marketSystemTx(ctx context.Context, market string, accounts []string, fn func(context.Context) error) error {
	if s.Cfg.RunID == 0 || s.Epoch <= 0 || s.runGuardSlots == nil {
		return Fail(p.CodeRouteChanged, market+"当前运行服未取得结算授权")
	}
	ctx, stop := context.WithTimeout(ctx, transactionTimeout)
	defer stop()
	var groups []uint64
	for _, account := range accounts {
		if !lowerHex(account, 16) {
			return Fail(p.CodeOperationConflict, market+"原结算账号无效")
		}
		groups = append(groups, runGuardGroup(account))
	}
	if len(groups) == 0 {
		groups = append(groups, 0)
	}
	slices.Sort(groups)
	groups = slices.Compact(groups)
	for _, group := range groups {
		slot, err := s.runGuardSlots.acquire(ctx, group)
		if err != nil {
			return Fail(p.CodeOperationPending, market+"原结算待查证")
		}
		defer func() { <-slot }()
	}
	return s.tx(ctx, func(tc context.Context) error {
		frozen, err := s.marketMergeFrozen(tc)
		if err != nil {
			return err
		}
		if frozen {
			return errMarketMergeFrozen
		}
		// 冻结也会推进全部组的写序列。即使这里读到旧快照，后续写入也会冲突并重读门禁。
		writes := make([]mongo.ClientBulkWrite, 0, len(groups))
		for _, group := range groups {
			write, err := s.runFenceWrite(tc, group)
			if err != nil {
				return err
			}
			writes = append(writes, write)
		}
		result, err := s.Client.BulkWrite(tc, writes, options.ClientBulkWrite().SetVerboseResults(true))
		if err != nil {
			return transactionBulkWriteError(err)
		}
		if !result.Acknowledged {
			return Fail(p.CodeOperationPending, market+"原结算未确认")
		}
		for index := range writes {
			if result.UpdateResults[index].MatchedCount != 1 {
				return Fail(p.CodeRouteChanged, market+"运行服写屏障失效")
			}
		}
		return fn(tc)
	})
}
