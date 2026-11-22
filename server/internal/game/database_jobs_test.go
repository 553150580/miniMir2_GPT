package game

import (
	"context"
	"testing"
	"time"

	p "minimir/internal/protocol"
)

func TestDatabaseAdmissionMakesProgressWithBusyBusinessWorkers(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	e := &Engine{
		ctx: ctx, jobs: make(chan job, 1), settlementJobs: make(chan job, 1),
		admissionTickets: make(chan job, 1), admissionEntries: make(chan job, 1),
	}
	e.startDatabaseWorkers()
	t.Cleanup(func() { cancel(); e.wg.Wait() })
	busy := make(chan struct{}, databaseBusinessWorkers)
	until := time.NewTimer(4 * time.Second)
	defer until.Stop()
	long := job{run: func() func(*worker) {
		busy <- struct{}{}
		<-ctx.Done()
		return nil
	}}
	for range databaseBusinessWorkers {
		select {
		case e.jobs <- long:
		case <-until.C:
			t.Fatal("业务执行协程未能接收占用任务")
		}
	}
	for range databaseBusinessWorkers {
		select {
		case <-busy:
		case <-until.C:
			t.Fatal("未能占用全部业务执行协程")
		}
	}
	if !offerDatabaseJob(e.jobs, long) || !offerDatabaseJob(e.settlementJobs, long) {
		t.Fatal("无法建立满队列前态")
	}
	validated := make(chan struct{})
	joined := make(chan struct{})
	w := &worker{e: e, done: make(chan func(*worker), 1)}
	if !offerDatabaseJob(e.admissionTickets, job{run: func() func(*worker) {
		close(validated)
		return nil
	}}) || !e.submitEntry(job{target: w, run: func() func(*worker) {
		return func(*worker) { close(joined) }
	}}) {
		t.Fatal("业务队列饱和阻塞了入场调度")
	}
	select {
	case <-validated:
	case <-until.C:
		t.Fatal("票据校验被业务积压阻塞")
	}
	select {
	case callback := <-w.done:
		callback(w)
	case <-until.C:
		t.Fatal("入场事务被业务积压阻塞")
	}
	select {
	case <-joined:
	default:
		t.Fatal("入场回调未送达自己的房间")
	}
}

func TestDatabaseSettlementQueuePreservesOrderAndCapacity(t *testing.T) {
	e := &Engine{jobs: make(chan job, 1), settlementJobs: make(chan job, 2)}
	w := &worker{e: e}
	first := &worker{}
	second := &worker{}
	if !w.submit(job{target: w}) ||
		!w.submit(job{target: first, settlement: true}) ||
		!w.submit(job{target: second, settlement: true}) {
		t.Fatal("普通业务占满时应仍可接收结算任务")
	}
	if w.submit(job{target: w, settlement: true}) || w.submit(job{target: w}) {
		t.Fatal("有界队列可以继续接收超过容量的任务")
	}
	if (<-e.settlementJobs).target != first || (<-e.settlementJobs).target != second || (<-e.jobs).target != w {
		t.Fatal("结算任务的先进先出顺序或普通业务保留槽位被破坏")
	}
}

func TestDatabaseAdmissionOverloadReturnsProtocolError(t *testing.T) {
	e := &Engine{admissionTickets: make(chan job, 1), admissionEntries: make(chan job, 1)}
	e.admissionTickets <- job{}
	e.admissionEntries <- job{}
	conn := &capturedConnection{}
	e.Enter("待校验票据", conn, 10)
	r := &room{}
	w := &worker{e: e, rooms: map[string]*room{"role": r}}
	w.join(r, command{role: "role", conn: conn, ticket: "已校验票据"})
	if len(conn.frames) != 2 ||
		conn.frames[0].ID != p.MsgS2CEnterLine || conn.frames[0].Request != 10 || conn.frames[0].Error != p.CodeServerBusy ||
		conn.frames[1].ID != p.MsgS2CEnterLine || conn.frames[1].Error != p.CodeServerBusy ||
		r.Pending || w.rooms["role"] != nil {
		t.Fatalf("入场队列满时协议错误或房间状态不正确：%+v", conn.frames)
	}
}

func TestDatabaseAdmissionSkipsDisconnectedWaitingEntry(t *testing.T) {
	e := &Engine{admissionEntries: make(chan job, 1)}
	r := &room{}
	w := &worker{e: e, rooms: map[string]*room{"role": r}}
	conn := &travelConnection{alive: true}
	w.join(r, command{role: "role", conn: conn, ticket: "已校验票据"})
	if !r.Pending || len(e.admissionEntries) != 1 {
		t.Fatal("入场任务没有进入待处理状态")
	}
	conn.Close("测试断开")
	// 数据库未装配。断开的排队连接必须在调用数据库之前返回。
	j := <-e.admissionEntries
	j.run()(w)
	if r.Pending || w.rooms["role"] != nil || len(conn.frames) != 0 {
		t.Fatal("等待入场时断开应释放临时房间而不返回业务帧")
	}
}
