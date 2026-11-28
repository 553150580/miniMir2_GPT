// 本文件按运行服发现已核验的支付，并通过原角色分片有界派送。
package game

import (
	"log/slog"
	"time"

	"minimir/internal/persistence"
)

const (
	paymentIdleDelay = 15 * time.Second       // 没有可处理订单时，仅低频检查仍未履约的欠款。
	paymentNextDelay = 200 * time.Millisecond // 两笔待支付派送之间让出数据库队列。
	paymentFailDelay = 5 * time.Second        // 无法确认查询时退避，保留原游标。
)

// paymentJob 由固定分片单独推进当前运行服；跨角色分片只通过完成通道回传。
type paymentJob struct {
	store    *persistence.Store       // 已批准运行服的持久视图。
	after    persistence.PaymentOrder // 原查询游标，只保留核验时间和订单号。
	next     time.Time                // 可以再次扫描的时刻。
	inFlight bool                     // 当前运行服已有查询或履约在途。
	result   chan error               // 单笔派送的完成结果，由本运行服调度分片回收。
}

// initializePaymentJobs 无条件保留已核验欠款的扫描能力，当前渠道开关不影响历史债务。
func (e *Engine) initializePaymentJobs() {
	for run, store := range e.runStores {
		owner := e.workers[run%uint64(len(e.workers))]
		owner.paymentJobs = append(owner.paymentJobs, paymentJob{store: store, result: make(chan error, 1)})
	}
}

// tickPaymentJobs 使用已有业务队列，无支付时不为每个角色单独轮询。
func (w *worker) tickPaymentJobs(now time.Time) {
	if w.e.stopped.Load() {
		return
	}
	for i := range w.paymentJobs {
		state := &w.paymentJobs[i]
		if state.inFlight {
			select {
			case err := <-state.result:
				state.inFlight = false
				state.next = now.Add(paymentNextDelay)
				logPaymentJobError(state.store, err)
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
			order, err := state.store.NextVerifiedPayment(w.e.ctx, after)
			return func(owner *worker) { owner.finishPaymentScan(state, order, err) }
		}})
	}
}

// finishPaymentScan 先把游标移出该订单，再让角色分片接手；忙或异常订单下一轮仍会重查。
func (w *worker) finishPaymentScan(state *paymentJob, order persistence.PaymentOrder, err error) {
	if err != nil || order.ID == "" {
		state.inFlight = false
		if err != nil {
			state.next = time.Now().Add(paymentFailDelay)
			logPaymentJobError(state.store, err)
		} else {
			state.after = persistence.PaymentOrder{}
			state.next = time.Now().Add(paymentIdleDelay)
		}
		return
	}
	state.after = persistence.PaymentOrder{ID: order.ID, VerifiedAt: order.VerifiedAt}
	w.postExternal(state.store, paymentDelivery(order), state.result)
}

// paymentDelivery 把原订单身份交给原角色分片，支付金额始终从持久订单读取。
func paymentDelivery(order persistence.PaymentOrder) externalDelivery {
	return externalDelivery{externalPayment, order.ID, order.RoleID, order.AccountID, order.OriginServerID}
}

// logPaymentJobError 仅记录运行服与分类，不把原商户或渠道数据写入日志。
func logPaymentJobError(store *persistence.Store, err error) {
	if err != nil {
		slog.Warn("支付履约待确认", "run", store.Cfg.RunID, "code", persistence.Code(err))
	}
}
