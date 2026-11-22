package game

import (
	"context"
	"strings"
	"testing"
	"time"

	"minimir/internal/domain"
	"minimir/internal/persistence"
)

func TestShutdownDrainsHealthyRoomBeforeReportingIsolation(t *testing.T) {
	w, healthy, _ := fullBagPolicyWorker(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	e := w.e
	e.ctx, e.workers = ctx, []*worker{w}
	w.commands, w.done = make(chan command, 8), make(chan func(*worker), 8)
	healthy.Conn, healthy.hostSecondsOpen = nil, 1
	isolated := &room{Role: domain.Role{ID: domain.ID()}, Isolated: true, hostSecondsOpen: 2}
	w.rooms[isolated.Role.ID] = isolated
	workerDone := make(chan struct{})
	go func() {
		defer close(workerDone)
		w.loop()
	}()
	t.Cleanup(func() {
		cancel()
		<-workerDone
	})
	drainDone := make(chan error, 1)
	go func() { drainDone <- e.DrainSettlements(ctx) }()
	var settled job
	select {
	case settled = <-e.jobs:
	case err := <-drainDone:
		t.Fatalf("健康房间未提交即返回停服结果: %v", err)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	w.done <- func(w *worker) {
		q := persistence.Settlement{SourceSeq: healthy.Role.SourceSeq + 1, HostSeconds: 1, HostingSession: healthy.HostSession}
		binding := settlementBinding{room: healthy, auth: healthy.Auth, world: healthy.currentWorld().Ref, generation: healthy.SettlementGeneration, id: healthy.PendingOperationID, fingerprint: healthy.PendingFingerprint}
		next := healthy.Role
		next.Revision++
		next.SourceSeq, next.HostingSeconds = q.SourceSeq, next.HostingSeconds-q.HostSeconds
		w.finishSettlement(binding, q, next, nil, &settled)
	}
	select {
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
