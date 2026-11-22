// 本文件回归停止收束时按房间处理健康结算，并报告已隔离房间。
package game

import (
	"context"
	"strings"
	"testing"
	"time"

	"minimir/internal/domain"
	"minimir/internal/persistence"
)

// TestShutdownDrainsHealthyRoomBeforeReportingIsolation 验证停服时先结清健康房间已发生的整秒，再报告无法安全结清的隔离房间。
// t 提供断言失败报告、子用例和清理注册。
func TestShutdownDrainsHealthyRoomBeforeReportingIsolation(t *testing.T) {
	// w 接收完整或满额背包策略房间执行器供后续步骤使用，healthy 保存由完整或满额背包策略房间执行器取得的健康。
	w, healthy, _ := fullBagPolicyWorker(t)
	// ctx 传递从调用方派生的上下文，供取消或控制截止时间，cancel 取消对应上下文或停止计时，释放其关联资源。
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	// e 保存当前处理所用的e。
	e := w.e
	e.ctx, e.workers = ctx, []*worker{w}
	w.commands, w.done = make(chan command, 8), make(chan func(*worker), 8)
	healthy.Conn, healthy.hostSecondsOpen = nil, 1
	// isolated 保存当前处理所用的已隔离。
	isolated := &room{Role: domain.Role{ID: domain.ID()}, Isolated: true, hostSecondsOpen: 2}
	w.rooms[isolated.Role.ID] = isolated
	// workerDone 创建用于维护房间执行器完成信号的容器或通道。
	workerDone := make(chan struct{})
	go func() {
		defer close(workerDone)
		w.loop()
	}()
	t.Cleanup(func() {
		cancel()
		<-workerDone
	})
	// drainDone 创建用于维护收束完成信号的容器或通道。
	drainDone := make(chan error, 1)
	go func() { drainDone <- e.DrainSettlements(ctx) }()
	// settled 记录“settled”，使用任务或职业编号表示。
	var settled job
	select {
	case settled = <-e.jobs:
	// err 保存当前操作的失败原因，供调用方检查或返回。
	case err := <-drainDone:
		t.Fatalf("健康房间未提交即返回停服结果: %v", err)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	// w 记录房间执行器。
	w.done <- func(w *worker) {
		// q 组装结算所需的数据。
		q := persistence.Settlement{SourceSeq: healthy.Role.SourceSeq + 1, HostSeconds: 1, HostingSession: healthy.HostSession}
		// binding 初始化用作会话绑定的结算会话绑定。
		binding := settlementBinding{room: healthy, auth: healthy.Auth, world: healthy.currentWorld().Ref, generation: healthy.SettlementGeneration, id: healthy.PendingOperationID, fingerprint: healthy.PendingFingerprint}
		// next 保存角色，用于当前步骤中的下一状态。
		next := healthy.Role
		next.Revision++
		next.SourceSeq, next.HostingSeconds = q.SourceSeq, next.HostingSeconds-q.HostSeconds
		w.finishSettlement(binding, q, next, nil, &settled)
	}
	select {
	// err 保存当前操作的失败原因，供调用方检查或返回。
	case err := <-drainDone:
		if err == nil || !strings.Contains(err.Error(), "1 个已隔离房间") {
			t.Fatalf("停服应报告隔离房间未结清: %v", err)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	if healthy.Pending || healthy.Role.HostingSeconds != 2 || isolated.hostSecondsOpen != 2 || len(e.jobs) != 0 {
		t.Fatal("健康房间未确认或改写了隔离房间")
	}
}
