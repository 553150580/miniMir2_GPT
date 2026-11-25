// 本文件通过当前运行服的业务队列推进有界赛季封账，无需参赛角色上线。
package game

import (
	"log/slog"
	"time"

	"minimir/internal/persistence"
	p "minimir/internal/protocol"
)

type mirrorSeasonJob struct {
	store    *persistence.Store // 此运行服的原租约和持久写屏障。
	next     time.Time          // 此分片可以再次投递关闭步骤的时刻。
	inFlight bool               // 每个运行服同一时刻最多一个关闭步骤。
	retired  bool               // 原写所有权失效后停止投递。
}

// initializeMirrorSeasonJobs 只为启用赛季结算的运行服分配任务，默认开发装配没有扫描开销。
func (e *Engine) initializeMirrorSeasonJobs() {
	for run, store := range e.runStores {
		arena := store.Cfg.Fixture.MirrorArena
		if arena == nil || arena.Settlement == nil {
			continue
		}
		owner := e.workers[run%uint64(len(e.workers))]
		owner.mirrorSeasonJobs = append(owner.mirrorSeasonJobs, mirrorSeasonJob{store: store})
	}
}

// tickMirrorSeasons 低频探测到期赛季，有工作时分批让出队列，避免挤占角色入场和收益结算。
func (w *worker) tickMirrorSeasons(now time.Time) {
	if len(w.mirrorSeasonJobs) == 0 || w.e.stopped.Load() {
		return
	}
	for i := range w.mirrorSeasonJobs {
		state := &w.mirrorSeasonJobs[i]
		if state.inFlight || state.retired || now.Before(state.next) {
			continue
		}
		state.next = now.Add(time.Second)
		state.inFlight = w.submit(job{target: w, run: func() func(*worker) {
			more, err := state.store.AdvanceMirrorSeason(w.e.ctx, now)
			return func(*worker) {
				state.inFlight = false
				pause := 30 * time.Second // 无到期工作时避免频繁探测。
				if more {
					pause = time.Second // 仅在确认有下一批时加快推进。
				}
				if err != nil {
					code := persistence.Code(err)
					state.retired = code == p.CodeRouteChanged
					pause = 5 * time.Second
					if !w.e.stopped.Load() {
						slog.Warn("镜像赛季封账待确认", "run", state.store.Cfg.RunID, "code", code, "error", err)
					}
				}
				state.next = time.Now().Add(pause)
			}
		}})
	}
}
