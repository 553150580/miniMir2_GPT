package bootstrap

import (
	"context"
	"errors"
	"slices"
	"testing"
	"testing/synctest"
	"time"

	"go.mongodb.org/mongo-driver/v2/mongo"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
)

func TestNodeLeaseRetriesUnknownThenReturnsToNormalCadence(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		started := time.Now()
		events := make(chan nodeLeaseEvent, 4)
		var attempts []time.Duration
		err := watchNodeLease(ctx, started, func(attempt context.Context) error {
			attempts = append(attempts, time.Since(started))
			switch len(attempts) {
			case 1:
				<-attempt.Done()
				return attempt.Err()
			case 2:
				return nil
			default:
				cancel()
				return nil
			}
		}, events)
		synctest.Wait()
		if err != nil || !slices.Equal(attempts, []time.Duration{3 * time.Second, 5250 * time.Millisecond, 8250 * time.Millisecond}) {
			t.Fatalf("单次超时未按有界退避恢复原节奏：%v，%v", attempts, err)
		}
		if len(events) != 2 {
			t.Fatalf("暂态与恢复事件应各一条，实际%d", len(events))
		}
		first, recovered := <-events, <-events
		if !errors.Is(first.err, context.DeadlineExceeded) || recovered.err != nil || recovered.elapsed != 2250*time.Millisecond {
			t.Fatalf("暂态与恢复耗时错误：%+v，%+v", first, recovered)
		}
	})
}

func TestNodeLeasePersistentUnknownCannotMoveConfirmedDeadline(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		started := time.Now()
		events := make(chan nodeLeaseEvent, 4)
		var attempts []time.Duration
		err := watchNodeLease(t.Context(), started, func(attempt context.Context) error {
			attempts = append(attempts, time.Since(started))
			<-attempt.Done()
			return attempt.Err()
		}, events)
		synctest.Wait()
		if !errors.Is(err, errNodeLeaseUnconfirmed) || time.Since(started) != 12*time.Second ||
			!slices.Equal(attempts, []time.Duration{3 * time.Second, 5250 * time.Millisecond, 7750 * time.Millisecond, 10750 * time.Millisecond}) {
			t.Fatalf("未知续租修改了已确认截止或重复刷新：%v，%v，%v", time.Since(started), attempts, err)
		}
		if len(events) != 1 {
			t.Fatalf("持续错误不应反复报首次暂态或冒报恢复：%d", len(events))
		}
	})
}

func TestNodeLeaseLateSuccessCannotReopenOldWindow(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		started := time.Now()
		release := make(chan struct{})
		events := make(chan nodeLeaseEvent, 4)
		calls := 0
		err := watchNodeLease(t.Context(), started, func(context.Context) error {
			calls++
			<-release
			return nil
		}, events)
		if !errors.Is(err, errNodeLeaseUnconfirmed) || time.Since(started) != 12*time.Second {
			t.Fatalf("挂起确认突破安全截止：%v，%v", time.Since(started), err)
		}
		close(release)
		synctest.Wait()
		if calls != 1 || len(events) != 0 {
			t.Fatalf("驱动未返回时发生重叠，或迟到回包被认作恢复：调用%d，事件%d", calls, len(events))
		}
	})
}

func TestNodeLeaseSuccessAfterRequestBudgetStillUnknown(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		started := time.Now()
		events := make(chan nodeLeaseEvent, 4)
		attempts := 0
		err := watchNodeLease(t.Context(), started, func(attempt context.Context) error {
			attempts++
			<-attempt.Done()
			if attempts == 1 {
				time.Sleep(100 * time.Millisecond)
				return nil
			}
			return attempt.Err()
		}, events)
		synctest.Wait()
		if !errors.Is(err, errNodeLeaseUnconfirmed) || time.Since(started) != 12*time.Second || len(events) != 1 {
			t.Fatalf("单次预算之外的响应延长了旧确认窗口：%v，%v，事件%d", time.Since(started), err, len(events))
		}
	})
}

func TestNodeLeaseConfirmsOwnershipLossWithoutRetry(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		started := time.Now()
		attempts := 0
		lost := persistence.Fail(p.CodeRouteChanged, "运行服租约失效")
		err := watchNodeLease(t.Context(), started, func(context.Context) error {
			attempts++
			return lost
		}, nil)
		if !errors.Is(err, lost) || persistence.Code(err) != p.CodeRouteChanged || attempts != 1 || time.Since(started) != nodeLeaseInterval {
			t.Fatalf("明确失权没有立即隔离：%d，%v，%v", attempts, time.Since(started), err)
		}
	})
}

func TestNodeLeaseWriteConcernIsUnknownWithoutExplicitWriteError(t *testing.T) {
	wc := &mongo.WriteConcernError{Code: 64, Message: "waiting for replication timed out"}
	if !transientNodeLeaseError(mongo.WriteException{WriteConcernError: wc}) {
		t.Fatal("写确认超时被错误认定为已失权")
	}
	if transientNodeLeaseError(mongo.WriteException{WriteConcernError: wc, WriteErrors: mongo.WriteErrors{{Code: 11000}}}) ||
		transientNodeLeaseError(mongo.CommandError{Code: 13, Message: "Unauthorized"}) {
		t.Fatal("写入/权限的明确失败被错误重试")
	}
}

func TestInitialNodeLeaseReturnsConservativeRequestStart(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		start := time.Now()
		confirmed, err := confirmInitialNodeLease(t.Context(), func(context.Context) error {
			time.Sleep(time.Second)
			return nil
		})
		if err != nil || !confirmed.Equal(start) || time.Since(confirmed) != time.Second {
			t.Fatalf("首次已确认窗口从返回时间错误扩大：%v，%v", confirmed, err)
		}
		confirmed, err = confirmInitialNodeLease(t.Context(), func(context.Context) error {
			time.Sleep(nodeLeaseTimeout + time.Millisecond)
			return nil
		})
		if !confirmed.IsZero() || !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("超时的首次确认允许开始服务：%v，%v", confirmed, err)
		}
	})
}

func TestNodeLeaseShutdownDoesNotWaitForBlockedTransactionCleanup(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		start := time.Now()
		endDriverWork := make(chan struct{})
		var request context.Context
		releaseNodeLease(func(ctx context.Context) error {
			request = ctx
			<-endDriverWork
			return nil
		})
		synctest.Wait()
		if time.Since(start) != nodeLeaseTimeout || request == nil || !errors.Is(request.Err(), context.DeadlineExceeded) {
			t.Fatalf("驱动事务清理绕过退出等待上限：%v，%v", time.Since(start), request)
		}
		close(endDriverWork)
	})
}
