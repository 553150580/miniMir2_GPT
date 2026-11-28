// 本文件核对已确认的支付只交给原运行服、角色和会话，并按原到账时刻更新权益。
package game

import (
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	"minimir/internal/payment"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
)

// paymentTestRoom 保存到账前房间、原订单和即将接收的已付回执。
type paymentTestRoom struct {
	w       *worker
	r       *room
	conn    *capturedConnection
	order   persistence.PaymentOrder
	next    domain.Role
	receipt persistence.Operation
	now     time.Time
}

func setupPaymentTestRoom(t *testing.T) paymentTestRoom {
	t.Helper()
	w, r, conn := stageBWorker(t, 0)
	now := time.Now().UTC().Truncate(time.Millisecond)
	store := w.e.Store
	store.Instance, store.Epoch, store.Cfg.RunID = domain.ID(), 3, 17
	r.Role.ID, r.Role.AccountID, r.Role.Origin = domain.ID(), domain.ID(), 5
	r.Role.Owner, r.Role.RunEpoch, r.Role.RunServerID = store.Instance, store.Epoch, store.Cfg.RunID
	r.Role.Revision, r.Role.SessionEpoch, r.Role.EconomicSeq, r.Role.SourceSeq = 12, 4, 7, 2
	r.Role.Wallet = domain.Wallet{Gold: 20, BoundGold: 30, Yuanbao: 80, BoundYuanbao: 90}
	r.Auth = persistence.Auth{RoleID: r.Role.ID, AccountID: r.Role.AccountID, SessionEpoch: 4, AuthVersion: 1}
	r.Conn, r.Ready, r.Pending = conn, true, true
	w.rooms = map[string]*room{r.Role.ID: r}
	order := persistence.PaymentOrder{ID: domain.ID(), RoleID: r.Role.ID, AccountID: r.Role.AccountID, OriginServerID: r.Role.Origin}
	r.pendingExternal = paymentDelivery(order)
	next := r.Role
	next.Revision++
	next.Wallet.Yuanbao += 1_000
	receipt := persistence.Operation{RoleID: r.Role.ID, Revision: next.Revision,
		PaymentGrant: &persistence.PaymentGrantReceipt{OrderID: order.ID, AccountID: r.Role.AccountID, OriginServerID: r.Role.Origin,
			Kind: payment.KindRecharge, YuanbaoAdded: 1_000, WalletBefore: r.Role.Wallet, WalletAfter: next.Wallet, GrantedAt: now}}
	return paymentTestRoom{w: w, r: r, conn: conn, order: order, next: next, receipt: receipt, now: now}
}

func TestPaymentWorkerPublishesOnlyConfirmedAssets(t *testing.T) {
	test := setupPaymentTestRoom(t)
	w, r, conn := test.w, test.r, test.conn
	life, hp, mp, pos, scene := r.Player.Life, r.Player.HP, r.Player.MP, r.Player.Pos, r.Scene
	test.next.Level++ // 部分字段不会被支付通知拷入现场。
	w.finishExternal(w.e.Store, paymentDelivery(test.order), r, r.Auth, test.next, test.receipt, nil, test.now)
	if r.Pending || r.pendingExternal != (externalDelivery{}) || r.Role.Revision != 13 || r.Role.EconomicSeq != 7 || r.Role.SourceSeq != 2 ||
		r.Role.Wallet != test.next.Wallet || r.Role.Level == test.next.Level || r.Player.HP != hp || r.Player.MP != mp ||
		r.Player.Life != life || r.Player.Pos != pos || r.Scene != scene {
		t.Fatal("到账通知没有保存原玩家流水、财务与现场", r.Role, r.Player)
	}
	walletFrames := 0
	for _, frame := range conn.frames {
		switch frame.ID {
		case p.MsgS2CWallet:
			var wallet pb.WalletState
			if err := proto.Unmarshal(frame.Payload, &wallet); err != nil || frame.Binding != 4 || frame.Request != 0 ||
				wallet.Revision != 13 || wallet.YuanbaoUnits != 1_080 || wallet.BoundYuanbaoUnits != 90 {
				t.Fatal("财务同步缺少原附着或已提交钱包", &wallet, frame, err)
			}
			walletFrames++
		case p.MsgS2COperationResult, p.MsgS2CSceneBaseline:
			t.Fatal("现金履约混入玩家命令回执或切换普通场景", frame)
		}
	}
	if walletFrames != 1 {
		t.Fatal("原到账未通过已有关联的 WebSocket 钱包消息发送", walletFrames)
	}
	before := len(conn.frames)
	w.finishExternal(w.e.Store, paymentDelivery(test.order), nil, persistence.Auth{}, test.next, test.receipt, nil, test.now)
	if len(conn.frames) != before || r.Role.Wallet.Yuanbao != 1_080 {
		t.Fatal("已到账的原回执对当前房间重复生效")
	}
}

func TestPaymentWorkerDoesNotOverwriteDifferentAttachmentOrRevision(t *testing.T) {
	for _, tc := range []struct {
		name string
		edit func(*paymentTestRoom)
	}{
		{"返回原会话", func(s *paymentTestRoom) { s.next.SessionEpoch-- }},
		{"返回原运行服所有权", func(s *paymentTestRoom) { s.next.RunEpoch-- }},
		{"返回不同账号", func(s *paymentTestRoom) { s.next.AccountID = domain.ID() }},
		{"回执原入口不符", func(s *paymentTestRoom) { s.receipt.PaymentGrant.OriginServerID++ }},
		{"原单不符", func(s *paymentTestRoom) { s.receipt.PaymentGrant.OrderID = domain.ID() }},
		{"已经有更高版本", func(s *paymentTestRoom) { s.r.Role.Revision = 14 }},
		{"同时夹带未观察资产更改", func(s *paymentTestRoom) { s.next.Revision++; s.receipt.Revision++ }},
		{"当前房间已属于另一运行服", func(s *paymentTestRoom) { s.r.runStore = &persistence.Store{} }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			test := setupPaymentTestRoom(t)
			tc.edit(&test)
			wallet, revision, frames := test.r.Role.Wallet, test.r.Role.Revision, len(test.conn.frames)
			test.w.finishExternal(test.w.e.Store, paymentDelivery(test.order), test.r, test.r.Auth, test.next, test.receipt, nil, test.now)
			if test.r.Role.Wallet != wallet || test.r.Role.Revision != revision || len(test.conn.frames) != frames {
				t.Fatal("错配或较旧结果覆盖了目前附着")
			}
		})
	}
	t.Run("接管后不解除新附着的在途任务", func(t *testing.T) {
		test := setupPaymentTestRoom(t)
		original := test.r.Auth
		test.r.Auth.SessionEpoch++
		test.w.finishExternal(test.w.e.Store, paymentDelivery(test.order), test.r, original, test.next, test.receipt, nil, test.now)
		if !test.r.Pending || test.r.pendingExternal != paymentDelivery(test.order) || test.r.Role.Revision != 12 {
			t.Fatal("原支付回调释放了重连后的在途任务")
		}
	})
}

func TestPaymentWorkerPreservesPriorHostingCardForInFlightHit(t *testing.T) {
	test := setupPaymentTestRoom(t)
	r := test.r
	oldSince, oldUntil := test.now.Add(-time.Hour), test.now.Add(time.Hour)
	r.Role.HostingCardSince, r.Role.HostingCardUntil = oldSince, oldUntil
	r.EventBatch = []dueEvent{{At: test.now.Add(-time.Millisecond)}}
	test.next.Wallet = r.Role.Wallet
	test.next.HostingCardSince, test.next.HostingCardUntil = oldSince, oldUntil.Add(90*24*time.Hour)
	grant := test.receipt.PaymentGrant
	grant.Kind, grant.YuanbaoAdded, grant.WalletAfter = payment.KindHostingCard, 0, test.next.Wallet
	grant.Card = &persistence.PaymentCardGrant{Kind: devconfig.RoleCardKindHosting, Days: 90, PreviousSince: oldSince,
		PreviousUntil: oldUntil, ValidSince: oldSince, ValidUntil: test.next.HostingCardUntil}
	// 离线托管期间只更新权威房间，后续附着会读取已确认的卡期。
	r.Conn = nil
	test.w.finishExternal(test.w.e.Store, paymentDelivery(test.order), r, r.Auth, test.next, test.receipt, nil, test.now.Add(time.Second))
	if r.Role.Wallet != test.next.Wallet || !r.Role.HostingCardUntil.Equal(test.next.HostingCardUntil) ||
		!r.hostingCardUntilAt(test.now.Add(-time.Millisecond)).Equal(oldUntil) ||
		!r.hostingCardUntilAt(test.now).Equal(test.next.HostingCardUntil) || len(r.hostingCardHistory) != 1 {
		t.Fatal("现金托管卡没有正确区分旧在途命中和首次到账生效后的卡期", r.hostingCardHistory)
	}
}

func TestPaymentWorkerSkipsUnavailableDispatchAndAdvancesCursor(t *testing.T) {
	e := &Engine{jobs: make(chan job, 1)}
	for range workerCount {
		e.workers = append(e.workers, &worker{e: e, commands: make(chan command, 1)})
	}
	order := persistence.PaymentOrder{ID: domain.ID(), RoleID: domain.ID(), VerifiedAt: time.Now()}
	target := e.worker(order.RoleID)
	scheduler := e.workers[0]
	if target == scheduler {
		scheduler = e.workers[1]
	}
	target.commands <- command{} // 模拟角色已饱和，不能跳过分片直接写资产。
	state := &paymentJob{inFlight: true, result: make(chan error, 1)}
	scheduler.finishPaymentScan(state, order, nil)
	if state.after.ID != order.ID || !state.after.VerifiedAt.Equal(order.VerifiedAt) || len(state.result) != 1 || len(e.jobs) != 0 {
		t.Fatal("分片暂时繁忙时未让出原单，或绕过原角色分片提交了资产")
	}
	<-state.result
	scheduler.finishPaymentScan(state, persistence.PaymentOrder{}, nil)
	if state.inFlight || state.after.ID != "" || time.Until(state.next) <= time.Second {
		t.Fatal("查到队尾后未回绕或没有降低空轮询频率", state.after, state.next)
	}
}

func TestPaymentWorkerRechecksSameOrderAfterTwoUnknownResults(t *testing.T) {
	test := setupPaymentTestRoom(t)
	w, r := test.w, test.r
	w.e.jobs = make(chan job, 2)
	completed := make(chan error, 1)
	dispatch := &externalDispatch{store: w.e.Store, delivery: paymentDelivery(test.order), room: r, auth: r.Auth, completed: completed, attempts: 1}
	w.externalDispatches = map[string]*externalDispatch{r.Role.ID: dispatch}
	unknown := persistence.Fail(p.CodeOperationPending, "隔离用例暂未确认原提交")
	w.finishExternalDispatch(dispatch, domain.Role{}, persistence.Operation{}, unknown, test.now)
	if !r.Pending || w.externalDispatches[r.Role.ID] != dispatch || dispatch.retryAt.IsZero() ||
		len(completed) != 1 || persistence.Code(<-completed) != p.CodeOperationPending {
		t.Fatal("未知提交未保留原单，或阻塞运行服继续扫描")
	}
	secondAt := dispatch.retryAt
	w.tickExternalConfirmations(secondAt)
	if len(w.e.jobs) != 1 || dispatch.attempts != 2 {
		t.Fatal("首次待确认未从原角色分片重新投递", dispatch.attempts)
	}
	<-w.e.jobs // 本用例从可信持久入口返回第二次原单待确认。
	w.finishExternalDispatch(dispatch, domain.Role{}, persistence.Operation{}, unknown, secondAt)
	if !r.Pending || w.externalDispatches[r.Role.ID] != dispatch || dispatch.retryAt.IsZero() || len(completed) != 0 {
		t.Fatal("连续两次待确认丢失原单，或重复占用运行服扫描权")
	}
	thirdAt := dispatch.retryAt
	w.tickExternalConfirmations(thirdAt)
	if len(w.e.jobs) != 1 || dispatch.attempts != 3 || dispatch.delivery.id != test.order.ID {
		t.Fatal("原支付已不在队列时没有按同一身份继续确认", dispatch.attempts)
	}
	<-w.e.jobs
	w.finishExternalDispatch(dispatch, test.next, test.receipt, nil, thirdAt)
	if r.Pending || len(w.externalDispatches) != 0 || r.Role.Revision != 13 || r.Role.Wallet.Yuanbao != 1_080 {
		t.Fatal("原回执在第三次确认后未应用给匹配的在线房间", r.Role)
	}
}

func TestPaymentWorkerSerializesColdAdmissionInBothOrders(t *testing.T) {
	store := &persistence.Store{}
	e := &Engine{Store: store, jobs: make(chan job, 2)}
	w := &worker{e: e, rooms: make(map[string]*room)}
	order := persistence.PaymentOrder{ID: domain.ID(), RoleID: domain.ID()}
	completed := make(chan error, 1)
	w.submitExternal(store, paymentDelivery(order), completed)
	dispatch := w.externalDispatches[order.RoleID]
	if len(e.jobs) != 1 || dispatch == nil || dispatch.room != nil {
		t.Fatal("无房间时没有保存原支付派送")
	}
	entry := command{role: order.RoleID, ticket: &persistence.Ticket{}, runStore: store, conn: &travelConnection{alive: true}}
	w.command(entry)
	if w.rooms[order.RoleID] != nil || len(e.jobs) != 1 || len(dispatch.entries) != 1 {
		t.Fatal("支付尚未确认时提前发起冷入场，可能向玩家发送旧钱包")
	}
	<-e.jobs
	w.finishExternalDispatch(dispatch, domain.Role{}, persistence.Operation{}, nil, time.Now())
	r := w.rooms[order.RoleID]
	if r == nil || r.entryAttempt == "" || !r.Pending || len(e.jobs) != 1 || len(w.externalDispatches) != 0 {
		t.Fatal("已派送原支付没有放行正常入场事务")
	}
	if err := <-completed; err != nil {
		t.Fatal(err)
	}
	w.submitExternal(store, paymentDelivery(order), completed)
	if len(e.jobs) != 1 || len(w.externalDispatches) != 0 || len(completed) != 1 {
		t.Fatal("先进入的冷入场仍在读取持久角色时被支付越过")
	}
}

func TestPaymentWorkerReleasesPendingRetryDuringShutdown(t *testing.T) {
	test := setupPaymentTestRoom(t)
	w, r := test.w, test.r
	w.e.stopped.Store(true)
	dispatch := &externalDispatch{store: w.e.Store, delivery: paymentDelivery(test.order), room: r, auth: r.Auth, attempts: 1}
	w.externalDispatches = map[string]*externalDispatch{r.Role.ID: dispatch}
	if !w.drainExternalDispatches(test.now) || !r.Pending {
		t.Fatal("停服未等待已提交的支付任务")
	}
	dispatch.retryAt = test.now.Add(time.Second)
	if w.drainExternalDispatches(test.now) || r.Pending || len(w.externalDispatches) != 0 || r.Role.Revision != 12 {
		t.Fatal("等待重试的原单未释放内存槽位，或停服当地再次发放了资产")
	}
}
