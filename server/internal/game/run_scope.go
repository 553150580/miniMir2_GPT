// 本文件装配同一进程承载的运行服视图，并在游戏事务前限定票据及房间的运行服归属。
package game

import (
	"context"

	"minimir/internal/persistence"
)

// NewRunEngine 让一组已批准的运行服共用房间分片、数据库任务队列和逻辑时钟。
// ctx 控制整个游戏引擎的生命周期，stores 是当前游戏实例获得的运行服视图。
func NewRunEngine(ctx context.Context, stores []*persistence.Store) *Engine {
	if len(stores) == 0 || stores[0] == nil {
		panic("游戏引擎至少需要一个运行服视图")
	}
	// common 提供进程内所有运行服一致的玩法规则和全局短票预查。
	common := stores[0]
	// runs 启动前建表，之后的票据查询只读该表。
	runs := make(map[uint64]*persistence.Store, len(stores))
	// store 是待纳入同一进程的一个已批准运行服视图。
	for _, store := range stores {
		if store == nil || store.Cfg.NodeID != common.Cfg.NodeID || store.Instance != common.Instance ||
			store.Cfg.Fixture.Hash != common.Cfg.Fixture.Hash ||
			store.Cfg.Fixture.RuleVersion != common.Cfg.Fixture.RuleVersion {
			panic("游戏运行服视图的节点、实例或玩法规则不一致")
		}
		if runs[store.Cfg.RunID] != nil {
			panic("游戏引擎不能重复承载同一运行服")
		}
		runs[store.Cfg.RunID] = store
	}
	return newEngine(ctx, common, runs)
}

// storeForTicket 按原票据的运行服、节点、实例和代次选取持久化视图。
// e 是当前游戏引擎，ticket 来自安装范围的持久票据预查。正式扣票仍由所选视图的事务核验。
func (e *Engine) storeForTicket(ticket persistence.Ticket) *persistence.Store {
	// store 仅包含当前实例已获准承载的一个运行服。
	store := e.runStores[ticket.Run]
	if store == nil || ticket.Owner != store.Instance || ticket.Epoch != store.Epoch || ticket.GameNodeID != store.Cfg.NodeID {
		return nil
	}
	return store
}

// roomStore 返回当前房间终生绑定的运行服视图。
// w 持有房间 r。未通过准入装配的单服测试房间沿用该引擎原有的默认视图。
func (w *worker) roomStore(r *room) *persistence.Store {
	if r.runStore != nil {
		return r.runStore
	}
	return w.e.Store
}
