// 本文件回归数据库入场队列隔离、结算队列顺序、过载错误和忽略已断连入场。
package game

import (
	"context"
	"testing"
	"time"

	"minimir/internal/persistence"
	p "minimir/internal/protocol"
)

// TestDatabaseAdmissionMakesProgressWithBusyBusinessWorkers 核对「业务执行协程未能接收占用任务」与「业务队列饱和阻塞了入场调度」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestDatabaseAdmissionMakesProgressWithBusyBusinessWorkers(t *testing.T) {
	// ctx 传递从调用方派生的上下文，供取消或控制截止时间，cancel 取消对应上下文或停止计时，释放其关联资源。
	ctx, cancel := context.WithCancel(t.Context())
	// e 保存当前处理所用的游戏引擎。
	e := &Engine{
		ctx: ctx, jobs: make(chan job, 1), settlementJobs: make(chan job, 1),
		admissionTickets: make(chan job, 1), admissionEntries: make(chan job, 1),
	}
	e.startDatabaseWorkers()
	t.Cleanup(func() { cancel(); e.wg.Wait() })
	// busy 创建用于维护“busy”的容器或通道。
	busy := make(chan struct{}, databaseBusinessWorkers)
	// until 保存由创建定时器取得的有效截止时间。
	until := time.NewTimer(4 * time.Second)
	defer until.Stop()
	// long 建立带初始值的任务或职业编号，供本段继续处理。
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
	// validated 创建用于维护“validated”的容器或通道。
	validated := make(chan struct{})
	// joined 创建用于维护“joined”的容器或通道。
	joined := make(chan struct{})
	// w 保存当前处理所用的房间执行器。
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
	// callback 接收完成信号通道送来的完成回调。
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

// TestDatabaseSettlementQueuePreservesOrderAndCapacity 核对「结算任务的先进先出顺序或普通业务保留槽位被破坏」与「有界队列可以继续接收超过容量的任务」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestDatabaseSettlementQueuePreservesOrderAndCapacity(t *testing.T) {
	// e 保存当前处理所用的游戏引擎。
	e := &Engine{jobs: make(chan job, 1), settlementJobs: make(chan job, 2)}
	// w 保存当前处理所用的房间执行器。
	w := &worker{e: e}
	// first 保存当前处理所用的第一项。
	first := &worker{}
	// second 保存当前处理所用的秒。
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

// TestDatabaseAdmissionOverloadReturnsProtocolError 核对「入场队列满时协议错误或房间状态不正确」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestDatabaseAdmissionOverloadReturnsProtocolError(t *testing.T) {
	// e 保存当前处理所用的游戏引擎。
	e := &Engine{admissionTickets: make(chan job, 1), admissionEntries: make(chan job, 1)}
	e.admissionTickets <- job{}
	e.admissionEntries <- job{}
	// conn 保存当前处理所用的连接。
	conn := &capturedConnection{}
	e.Enter("待校验票据", conn, 10)
	// r 保存当前处理所用的房间。
	r := &room{}
	// w 保存当前处理所用的房间执行器。
	w := &worker{e: e, rooms: map[string]*room{"role": r}}
	w.join(r, command{role: "role", conn: conn, ticket: &persistence.Ticket{}})
	if len(conn.frames) != 2 ||
		conn.frames[0].ID != p.MsgS2CEnterLine || conn.frames[0].Request != 10 || conn.frames[0].Error != p.CodeServerBusy ||
		conn.frames[1].ID != p.MsgS2CEnterLine || conn.frames[1].Error != p.CodeServerBusy ||
		r.Pending || w.rooms["role"] != nil {
		t.Fatalf("入场队列满时协议错误或房间状态不正确：%+v", conn.frames)
	}
}

// TestDatabaseAdmissionSkipsDisconnectedWaitingEntry 核验入场任务应进入待处理状态，用诊断「等待入场时断开应释放临时房间而不返回业务帧」定位对应断言。
// t 提供断言失败报告、子用例和清理注册。
func TestDatabaseAdmissionSkipsDisconnectedWaitingEntry(t *testing.T) {
	// e 保存当前处理所用的游戏引擎。
	e := &Engine{admissionEntries: make(chan job, 1)}
	// r 保存当前处理所用的房间。
	r := &room{}
	// w 保存当前处理所用的房间执行器。
	w := &worker{e: e, rooms: map[string]*room{"role": r}}
	// conn 保存当前处理所用的连接。
	conn := &travelConnection{alive: true}
	w.join(r, command{role: "role", conn: conn, ticket: &persistence.Ticket{}})
	if !r.Pending || len(e.admissionEntries) != 1 {
		t.Fatal("入场任务没有进入待处理状态")
	}
	conn.Close("测试断开")
	// 数据库未装配。断开的排队连接必须在调用数据库之前返回。
	// j 接收入场“entries”通道送来的当前位置的下标。
	j := <-e.admissionEntries
	j.run()(w)
	if r.Pending || w.rooms["role"] != nil || len(conn.frames) != 0 {
		t.Fatal("等待入场时断开应释放临时房间而不返回业务帧")
	}
}
