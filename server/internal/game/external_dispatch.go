// 本文件协调已核验外部权益、普通资产写入与冷入场，保留原凭据的有界重试。
package game

import (
	"time"

	"minimir/internal/domain"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
)

const (
	externalConfirmationDelay  = 750 * time.Millisecond // 按原凭据退避重查。
	externalConfirmationWindow = 8 * time.Second        // 原角色暂留未知写入的最长时限。
	maxExternalAttempts        = 3                      // 首次失败后还能按原身份确认两次。
	maxExternalDispatches      = 8                      // 每分片同时保留的外部确认数。
)

type externalSource uint8 // 区分已核验的外部来源，避免相同编号混到另一路回执。

const (
	externalPayment    externalSource = iota + 1 // 已核验原现金支付。
	externalRewardedAd                           // 已核验原激励视频。
)

// externalDelivery 携带来源及不可变的原受益人。
type externalDelivery struct {
	source    externalSource // 本次发放所属来源。
	id        string         // 来源内唯一的原凭据。
	roleID    string         // 原永久角色。
	accountID string         // 原授权账号。
	origin    uint64         // 原入口服，不因账号换角或合服变化。
}

// externalDispatch 保存分片正在处理的一笔原发放，重试沿用同一来源和凭据。
type externalDispatch struct {
	store     *persistence.Store // 当前运行服的租约视图。
	delivery  externalDelivery   // 已核验且固定了受益角色的原凭据。
	room      *room              // 派送前已存在的有效原房间，冷角色为空。
	auth      persistence.Auth   // 房间的原附着授权，用于确认回调。
	completed chan<- error       // 首次尝试后归还运行服扫描权。
	attempts  int                // 已投递的履约或查证次数。
	retryAt   time.Time          // 非零时等待按原单重查；清零后有查询在途。
	deadline  time.Time          // 原分片允许暂留未决结果的最晚时间。
	entries   []command          // 冷角色在此期间到达的有限入场请求。
}

// postExternal 交给原角色分片。如果队列已满，归还扫描权并保留原凭据供下轮重查。
func (w *worker) postExternal(store *persistence.Store, delivery externalDelivery, completed chan<- error) {
	dispatch := func(target *worker) { target.submitExternal(store, delivery, completed) }
	target := w.e.worker(delivery.roleID)
	if target == w {
		dispatch(w)
	} else if !w.e.post(target, command{done: dispatch}) {
		completed <- nil
	}
}

// submitExternal 由原角色分片排队，保留该角色已有资产和冷入场的顺序。
func (w *worker) submitExternal(store *persistence.Store, delivery externalDelivery, completed chan<- error) {
	if w.e.stopped.Load() || len(w.externalDispatches) >= maxExternalDispatches || w.externalDispatches[delivery.roleID] != nil {
		completed <- nil
		return
	}
	r := w.externalRoom(store, delivery)
	current := w.rooms[delivery.roleID]
	if r == nil && current != nil && !current.Isolated && w.roomStore(current) == store {
		// 当前分片已有同一角色的冷入场，必须让其先确认新会话并创建可用的首快照。
		completed <- nil
		return
	}
	dispatch := &externalDispatch{store: store, delivery: delivery, room: r, completed: completed}
	if r != nil {
		if r.Pending || r.entryAttempt != "" || r.dungeonStart != nil {
			completed <- nil
			return
		}
		dispatch.auth = r.Auth
	}
	if !w.queueExternalDispatch(dispatch) {
		completed <- nil
		return
	}
	if w.externalDispatches == nil {
		w.externalDispatches = make(map[string]*externalDispatch)
	}
	w.externalDispatches[delivery.roleID] = dispatch
	if r != nil {
		r.Pending, r.pendingExternal = true, delivery
	}
}

// deferEntryForExternal 让已经排入的冷角色发放优先完成，再读取首个资产快照。
func (w *worker) deferEntryForExternal(entry command) bool {
	dispatch := w.externalDispatches[entry.role]
	if dispatch == nil || dispatch.room != nil {
		return false
	}
	if len(dispatch.entries) >= 2 {
		rejectEntry(entry.conn, entry.frame.Request, p.CodeServerBusy, "asset confirmation in progress")
		return true
	}
	dispatch.entries = append(dispatch.entries, entry)
	return true
}

// queueExternalDispatch 按已核验的原来源交给持久层，读取原回执或执行首次发放。
func (w *worker) queueExternalDispatch(dispatch *externalDispatch) bool {
	task := job{target: w, run: func() func(*worker) {
		var next domain.Role
		var receipt persistence.Operation
		var err error
		switch dispatch.delivery.source {
		case externalPayment:
			next, receipt, err = dispatch.store.FulfillPaymentOrder(w.e.ctx, dispatch.delivery.id)
		case externalRewardedAd:
			next, receipt, err = dispatch.store.FulfillRewardedAd(w.e.ctx, dispatch.delivery.id)
		default:
			err = persistence.Fail(p.CodeInvalidArgument, "外部资产来源无效")
		}
		return func(target *worker) { target.finishExternalDispatch(dispatch, next, receipt, err, time.Now()) }
	}}
	if !w.submit(task) {
		return false
	}
	dispatch.attempts++
	dispatch.retryAt = time.Time{}
	return true
}

// tickExternalConfirmations 按原凭据单独重查，运行服游标继续扫描其他角色。
func (w *worker) tickExternalConfirmations(now time.Time) {
	if w.e.stopped.Load() {
		return
	}
	for _, dispatch := range w.externalDispatches {
		if dispatch.retryAt.IsZero() || now.Before(dispatch.retryAt) {
			continue
		}
		if !now.Before(dispatch.deadline) {
			w.finishExternalDispatch(dispatch, domain.Role{}, persistence.Operation{},
				persistence.Fail(p.CodeOperationPending, "外部资产确认暂超出分片等候窗口"), now)
		} else if !w.queueExternalDispatch(dispatch) {
			dispatch.retryAt = now.Add(externalConfirmationDelay)
		}
	}
}

// drainExternalDispatches 等待已投递事务结束，待重新发放的原凭据交给下一代进程。
func (w *worker) drainExternalDispatches(now time.Time) bool {
	pending := false
	for _, dispatch := range w.externalDispatches {
		if dispatch.retryAt.IsZero() {
			pending = true
			continue
		}
		w.finishExternalDispatch(dispatch, domain.Role{}, persistence.Operation{},
			persistence.Fail(p.CodeOperationPending, "原外部资产由下一次启动继续确认"), now)
	}
	return pending
}

// finishExternalDispatch 首次尝试即释放扫描游标。原请求即使已退出待发放列表也可继续查证。
func (w *worker) finishExternalDispatch(dispatch *externalDispatch, next domain.Role, receipt persistence.Operation, err error, now time.Time) {
	if w.externalDispatches[dispatch.delivery.roleID] != dispatch {
		return
	}
	if dispatch.completed != nil {
		dispatch.completed <- err
		dispatch.completed = nil
	}
	if persistence.Code(err) == p.CodeOperationPending && dispatch.attempts < maxExternalAttempts && !w.e.stopped.Load() {
		if dispatch.deadline.IsZero() {
			dispatch.deadline = now.Add(externalConfirmationWindow)
		}
		if now.Before(dispatch.deadline) {
			dispatch.retryAt = now.Add(externalConfirmationDelay)
			return
		}
	}
	delete(w.externalDispatches, dispatch.delivery.roleID)
	w.finishExternal(dispatch.store, dispatch.delivery, dispatch.room, dispatch.auth, next, receipt, err, now)
	for _, entry := range dispatch.entries {
		w.command(entry)
	}
}

// finishExternal 按原运行服和角色附着释放自己等待的操作，已确认的新修订交给相应来源处理。
func (w *worker) finishExternal(store *persistence.Store, delivery externalDelivery, bound *room, auth persistence.Auth,
	next domain.Role, receipt persistence.Operation, err error, now time.Time) {
	r := w.externalRoom(store, delivery)
	if bound != nil {
		if r != bound || r.Auth != auth || r.pendingExternal != delivery {
			return
		}
		r.pendingExternal = externalDelivery{}
		r.resumeAfterPending(now)
		defer w.after(r)
	}
	if err == nil && r != nil {
		switch delivery.source {
		case externalPayment:
			w.applyPaymentGrant(r, store, delivery.id, next, receipt, now)
		case externalRewardedAd:
			w.applyRewardedAdGrant(r, store, delivery.id, next, receipt)
		}
	}
}

// externalRoom 选择同一运行服视图下仍属于原账号的角色房间。
func (w *worker) externalRoom(store *persistence.Store, delivery externalDelivery) *room {
	r := w.rooms[delivery.roleID]
	if r == nil || r.Isolated || r.Scene == 0 || w.roomStore(r) != store || r.Role.ID != delivery.roleID ||
		r.Role.Origin != delivery.origin || r.Role.AccountID != delivery.accountID || r.Auth.AccountID != delivery.accountID ||
		r.Auth.RoleID != delivery.roleID || r.Auth.SessionEpoch <= 0 {
		return nil
	}
	return r
}
