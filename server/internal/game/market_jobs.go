// 本文件用原运行服和有界数据库队列推进两个市场的已受理资产。
package game

import (
	"context"
	"log/slog"
	"time"

	"minimir/internal/persistence"
	p "minimir/internal/protocol"
)

type marketJob struct {
	runID    uint64                                         // 拥有相应市场的运行服。
	name     string                                         // 日志中的市场名称。
	gold     bool                                           // 金币挂单确认后可唤醒撮合。
	advance  func(context.Context, time.Time) (bool, error) // 持久层的有界推进步骤。
	idle     time.Duration                                  // 未发现可推进订单时的空闲间隔。
	active   time.Duration                                  // 有剩余工作时的让出间隔。
	next     time.Time                                      // 下次投递时间。
	inFlight bool                                           // 同一运行服的同一市场最多一笔后台步骤。
	retired  bool                                           // 本节点的运行服代次已失效。
}

func (e *Engine) initializeMarketJobs() {
	const closed = 15 * time.Second
	for run, store := range e.runStores {
		owner := e.workers[run%uint64(len(e.workers))]
		itemIdle, goldIdle := closed, closed
		if store.Cfg.Fixture.ItemMarket != nil {
			itemIdle = 2 * time.Second
		}
		if store.Cfg.Fixture.GoldMarket != nil {
			goldIdle = 500 * time.Millisecond
		}
		owner.marketJobs = append(owner.marketJobs,
			marketJob{runID: run, name: "道具", advance: store.AdvanceItemMarket, idle: itemIdle, active: 200 * time.Millisecond},
			marketJob{runID: run, name: "金币", gold: true, advance: store.AdvanceGoldMarket, idle: goldIdle, active: 50 * time.Millisecond})
	}
}

// wakeGoldMarket 在挂单冻结后唤醒原分片，定时扫描仍能恢复断线或换代前的委托。
func (w *worker) wakeGoldMarket(run uint64) {
	target := w.e.workers[run%uint64(len(w.e.workers))]
	wake := func(owner *worker) {
		for i := range owner.marketJobs {
			task := &owner.marketJobs[i]
			if task.runID == run && task.gold {
				task.next = time.Time{}
			}
		}
	}
	if w == target {
		wake(w)
		return
	}
	w.e.post(target, command{done: wake})
}

// tickMarketJobs 每次有界推进后让出数据库队列，空闲及关闭态保留旧单清偿扫描。
func (w *worker) tickMarketJobs(now time.Time) {
	if w.e.stopped.Load() {
		return
	}
	for i := range w.marketJobs {
		state := &w.marketJobs[i]
		if state.inFlight || state.retired || now.Before(state.next) {
			continue
		}
		state.next = now.Add(time.Second)
		state.inFlight = w.submit(job{target: w, run: func() func(*worker) {
			more, err := state.advance(w.e.ctx, time.Now())
			return func(*worker) {
				state.inFlight = false
				pause := state.idle
				if more || state.next.IsZero() {
					pause = state.active
				}
				if err != nil {
					code := persistence.Code(err)
					state.retired = code == p.CodeRouteChanged
					pause = 5 * time.Second
					if !w.e.stopped.Load() {
						slog.Warn("市场后台步骤待确认", "market", state.name, "run", state.runID, "code", code, "error", err)
					}
				}
				state.next = time.Now().Add(pause)
			}
		}})
	}
}
