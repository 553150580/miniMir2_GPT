// 本文件在角色分片协调支付确认、已有资产写入与冷入场，确保原已付订单可独立重查。
package game

import (
	"time"

	"minimir/internal/domain"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
)

const (
	paymentConfirmationDelay  = 750 * time.Millisecond // 以原单退避查证，不反复占用同一数据库执行协程。
	paymentConfirmationWindow = 8 * time.Second        // 只在有限窗口为同一角色保留原支付顺序。
	maxPaymentAttempts        = 3                      // 首次提交待确认后，原角色仍可继续查证两次。
	maxPaymentDispatches      = 8                      // 每分片暂留的支付确认有限，余下订单继续持久排队。
)

// paymentDispatch 保存当前分片受理的一笔原支付，重试只沿用原持久订单身份。
type paymentDispatch struct {
	store     *persistence.Store       // 发现此订单的运行服租约视图。
	order     persistence.PaymentOrder // 已核验并固定了受益角色的原单。
	room      *room                    // 派送前已存在的有效原房间，冷角色为空。
	auth      persistence.Auth         // 房间的原附着授权，用于确认回调。
	completed chan<- error             // 首次尝试后归还运行服扫描权。
	attempts  int                      // 已投递的履约或查证次数。
	retryAt   time.Time                // 非零时等待按原单重查；清零后有查询在途。
	deadline  time.Time                // 原分片允许暂留未决结果的最晚时间。
	entries   []command                // 冷角色在此期间到达的有限入场请求。
}

// submitPayment 由原角色分片和已有资产、冷入场保持顺序；确认中的其他角色可独立继续派送。
func (w *worker) submitPayment(store *persistence.Store, order persistence.PaymentOrder, completed chan<- error) {
	if w.e.stopped.Load() || len(w.paymentDispatches) >= maxPaymentDispatches || w.paymentDispatches[order.RoleID] != nil {
		completed <- nil
		return
	}
	r := w.paymentRoom(store, order)
	current := w.rooms[order.RoleID]
	if r == nil && current != nil && !current.Isolated && w.roomStore(current) == store {
		// 当前分片已有同一角色的冷入场，必须让其先确认新会话并创建可用的首快照。
		completed <- nil
		return
	}
	dispatch := &paymentDispatch{store: store, order: order, room: r, completed: completed}
	if r != nil {
		if r.Pending || r.entryAttempt != "" || r.dungeonStart != nil {
			completed <- nil
			return
		}
		dispatch.auth = r.Auth
	}
	if !w.queuePaymentDispatch(dispatch) {
		completed <- nil
		return
	}
	if w.paymentDispatches == nil {
		w.paymentDispatches = make(map[string]*paymentDispatch)
	}
	w.paymentDispatches[order.RoleID] = dispatch
	if r != nil {
		r.Pending, r.pendingPaymentID = true, order.ID
	}
}

// deferEntryForPayment 让先进入分片的冷角色支付完成后再开始账号接管，避免沿用旧钱包首快照。
func (w *worker) deferEntryForPayment(entry command) bool {
	dispatch := w.paymentDispatches[entry.role]
	if dispatch == nil || dispatch.room != nil {
		return false
	}
	if len(dispatch.entries) >= 2 {
		rejectEntry(entry.conn, entry.frame.Request, p.CodeServerBusy, "payment confirmation in progress")
		return true
	}
	dispatch.entries = append(dispatch.entries, entry)
	return true
}

// queuePaymentDispatch 只提交原单编号，由持久层查询已提交的原回执或执行剩余履约。
func (w *worker) queuePaymentDispatch(dispatch *paymentDispatch) bool {
	task := job{target: w, run: func() func(*worker) {
		next, receipt, err := dispatch.store.FulfillPaymentOrder(w.e.ctx, dispatch.order.ID)
		return func(target *worker) { target.finishPaymentDispatch(dispatch, next, receipt, err, time.Now()) }
	}}
	if !w.submit(task) {
		return false
	}
	dispatch.attempts++
	dispatch.retryAt = time.Time{}
	return true
}

// tickPaymentConfirmations 按原单单独重查，运行服游标可同时越过坏单继续后面的角色。
func (w *worker) tickPaymentConfirmations(now time.Time) {
	for _, dispatch := range w.paymentDispatches {
		if dispatch.retryAt.IsZero() || now.Before(dispatch.retryAt) {
			continue
		}
		if !now.Before(dispatch.deadline) {
			w.finishPaymentDispatch(dispatch, domain.Role{}, persistence.Operation{},
				persistence.Fail(p.CodeOperationPending, "支付确认暂超出分片等候窗口"), now)
		} else if !w.queuePaymentDispatch(dispatch) {
			dispatch.retryAt = now.Add(paymentConfirmationDelay)
		}
	}
}

// drainPaymentDispatches 等待已投递事务结束，待再次派送的原单保留给下一代游戏进程。
func (w *worker) drainPaymentDispatches(now time.Time) bool {
	pending := false
	for _, dispatch := range w.paymentDispatches {
		if dispatch.retryAt.IsZero() {
			pending = true
			continue
		}
		w.finishPaymentDispatch(dispatch, domain.Role{}, persistence.Operation{},
			persistence.Fail(p.CodeOperationPending, "原支付由下一次启动继续确认"), now)
	}
	return pending
}

// finishPaymentDispatch 首次尝试即释放游标；待确认保留原身份，不依赖仍位于已核验队列内。
func (w *worker) finishPaymentDispatch(dispatch *paymentDispatch, next domain.Role, receipt persistence.Operation, err error, now time.Time) {
	if w.paymentDispatches[dispatch.order.RoleID] != dispatch {
		return
	}
	if dispatch.completed != nil {
		dispatch.completed <- err
		dispatch.completed = nil
	}
	if persistence.Code(err) == p.CodeOperationPending && dispatch.attempts < maxPaymentAttempts && !w.e.stopped.Load() {
		if dispatch.deadline.IsZero() {
			dispatch.deadline = now.Add(paymentConfirmationWindow)
		}
		if now.Before(dispatch.deadline) {
			dispatch.retryAt = now.Add(paymentConfirmationDelay)
			return
		}
	}
	delete(w.paymentDispatches, dispatch.order.RoleID)
	w.finishPayment(dispatch.store, dispatch.order, dispatch.room, dispatch.auth, next, receipt, err, now)
	for _, entry := range dispatch.entries {
		w.command(entry)
	}
}

// finishPayment 按原账号、入口服、运行服及附着核对已确认的支付，只释放自己受理的在途任务。
func (w *worker) finishPayment(store *persistence.Store, order persistence.PaymentOrder, bound *room, auth persistence.Auth,
	next domain.Role, receipt persistence.Operation, err error, now time.Time) {
	r := w.paymentRoom(store, order)
	if bound != nil {
		if r != bound || r.Auth != auth || r.pendingPaymentID != order.ID {
			return
		}
		r.pendingPaymentID = ""
		r.resumeAfterPending(now)
		defer w.after(r)
	}
	if err == nil && r != nil {
		w.applyPaymentGrant(r, store, order.ID, next, receipt, now)
	}
}

// paymentRoom 选择同一运行服视图下仍有原账号授权的角色房间；冷角色在履约完成后再开始入场。
func (w *worker) paymentRoom(store *persistence.Store, order persistence.PaymentOrder) *room {
	r := w.rooms[order.RoleID]
	if r == nil || r.Isolated || r.Scene == 0 || w.roomStore(r) != store || r.Role.ID != order.RoleID ||
		r.Role.Origin != order.OriginServerID || r.Role.AccountID != order.AccountID || r.Auth.AccountID != order.AccountID ||
		r.Auth.RoleID != order.RoleID || r.Auth.SessionEpoch <= 0 {
		return nil
	}
	return r
}
