// 本文件按运行服扫描已核验的广告，在线和离线角色共用同一个资产投递入口。
package game

import (
	"log/slog"
	"time"

	"minimir/internal/persistence"
)

const (
	rewardedAdIdleDelay   = 2 * time.Second        // 已开放渠道的队列为空时按运行服重查。
	rewardedAdClosedDelay = 15 * time.Second       // 暂停新观看时低频履行已有核验结果。
	rewardedAdNextDelay   = 200 * time.Millisecond // 两笔发放之间让出数据库队列。
	rewardedAdFailDelay   = 5 * time.Second        // 数据库查询失败时退避。
)

// rewardedAdJob 保存一个运行服的扫描游标，不为每个角色创建定时器。
type rewardedAdJob struct {
	store    *persistence.Store            // 源角色必须路由到该运行服。
	after    persistence.RewardedAdAttempt // 上次扫描已越过的原凭据。
	next     time.Time                     // 允许再次查队列的时间。
	inFlight bool                          // 查询或角色分片投递仍在处理。
	result   chan error                    // 原角色分片的第一笔事务结果。
}

// initializeRewardedAdJobs 保留已确认到账资格，渠道停发新观看也继续处理原凭据。
func (e *Engine) initializeRewardedAdJobs() {
	for run, store := range e.runStores {
		owner := e.workers[run%uint64(len(e.workers))]
		owner.rewardedAdJobs = append(owner.rewardedAdJobs, rewardedAdJob{store: store, result: make(chan error, 1)})
	}
}

// tickRewardedAdJobs 使用服务端现有业务队列读取一笔到期凭据。
func (w *worker) tickRewardedAdJobs(now time.Time) {
	if w.e.stopped.Load() {
		return
	}
	for i := range w.rewardedAdJobs {
		state := &w.rewardedAdJobs[i]
		if state.inFlight {
			select {
			case err := <-state.result:
				state.inFlight = false
				state.next = now.Add(rewardedAdNextDelay)
				if err != nil {
					slog.Warn("广告奖励履约待确认", "run", state.store.Cfg.RunID, "code", persistence.Code(err))
				}
			default:
			}
			continue
		}
		if now.Before(state.next) {
			continue
		}
		after := state.after
		state.next = now.Add(time.Second)
		state.inFlight = w.submit(job{target: w, run: func() func(*worker) {
			attempt, err := state.store.NextRewardedAdGrant(w.e.ctx, after)
			return func(owner *worker) { owner.finishRewardedAdScan(state, attempt, err) }
		}})
	}
}

// finishRewardedAdScan 越过单个凭据后继续扫描其他角色，下一轮仍可确认繁忙的原凭据。
func (w *worker) finishRewardedAdScan(state *rewardedAdJob, attempt persistence.RewardedAdAttempt, err error) {
	if err != nil {
		state.inFlight = false
		state.next = time.Now().Add(rewardedAdFailDelay)
		slog.Warn("广告奖励队列查询待确认", "run", state.store.Cfg.RunID, "code", persistence.Code(err))
		return
	}
	if attempt.ID == "" {
		state.inFlight = false
		state.after = persistence.RewardedAdAttempt{}
		delay := rewardedAdIdleDelay
		if state.store.Cfg.Fixture.RewardedAd == nil {
			delay = rewardedAdClosedDelay
		}
		state.next = time.Now().Add(delay)
		return
	}
	state.after = persistence.RewardedAdAttempt{ID: attempt.ID, VerifiedAt: attempt.VerifiedAt}
	w.postExternal(state.store, externalDelivery{externalRewardedAd, attempt.ID, attempt.RoleID, attempt.AccountID, attempt.OriginServerID}, state.result)
}
