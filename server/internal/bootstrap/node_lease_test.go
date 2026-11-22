// 本文件驱动续租监视器的初始确认、暂态恢复、截止和取消边界。

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

// TestNodeLeaseRetriesUnknownThenReturnsToNormalCadence 核对「单次超时未按有界退避恢复原节奏」与「暂态与恢复事件应各一条」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestNodeLeaseRetriesUnknownThenReturnsToNormalCadence(t *testing.T) {
	// t 提供断言失败报告、子用例和清理注册。
	synctest.Test(t, func(t *testing.T) {
		// ctx 传递从调用方派生的上下文，供取消或控制截止时间，cancel 取消对应上下文或停止计时，释放其关联资源。
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		// started 记录本次处理开始时的时间。
		started := time.Now()
		// events 创建用于维护事件集合的容器或通道。
		events := make(chan nodeLeaseEvent, 4)
		// attempts 记录“attempts”，使用时长列表表示。
		var attempts []time.Duration
		// err 接收监视节点租约过程中可能产生的错误，供后续失败分支处理，attempt 记录“attempt”，使用请求或事务上下文表示。
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
		// first 接收事件集合通道送来的第一项，recovered 接收事件集合通道送来的“recovered”。
		first, recovered := <-events, <-events
		if !errors.Is(first.err, context.DeadlineExceeded) || recovered.err != nil || recovered.elapsed != 2250*time.Millisecond {
			t.Fatalf("暂态与恢复耗时错误：%+v，%+v", first, recovered)
		}
	})
}

// TestNodeLeasePersistentUnknownCannotMoveConfirmedDeadline 核验持续错误不应反复报首次暂态或冒报恢复，用诊断「未知续租修改了已确认截止或重复刷新」定位对应断言。
// t 提供断言失败报告、子用例和清理注册。
func TestNodeLeasePersistentUnknownCannotMoveConfirmedDeadline(t *testing.T) {
	// t 提供断言失败报告、子用例和清理注册。
	synctest.Test(t, func(t *testing.T) {
		// started 记录本次处理开始时的时间。
		started := time.Now()
		// events 创建用于维护事件集合的容器或通道。
		events := make(chan nodeLeaseEvent, 4)
		// attempts 记录“attempts”，使用时长列表表示。
		var attempts []time.Duration
		// err 接收监视节点租约过程中可能产生的错误，供后续失败分支处理，attempt 记录“attempt”，使用请求或事务上下文表示。
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

// TestNodeLeaseLateSuccessCannotReopenOldWindow 核对「驱动未返回时发生重叠，或迟到回包被认作恢复」与「挂起确认突破安全截止」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestNodeLeaseLateSuccessCannotReopenOldWindow(t *testing.T) {
	// t 提供断言失败报告、子用例和清理注册。
	synctest.Test(t, func(t *testing.T) {
		// started 记录本次处理开始时的时间。
		started := time.Now()
		// release 创建用于维护释放的容器或通道。
		release := make(chan struct{})
		// events 创建用于维护事件集合的容器或通道。
		events := make(chan nodeLeaseEvent, 4)
		// calls 规定调用集合的初始取值。
		calls := 0
		// err 接收监视节点租约过程中可能产生的错误，供后续失败分支处理。
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

// TestNodeLeaseSuccessAfterRequestBudgetStillUnknown 核对「单次预算之外的响应延长了旧确认窗口」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestNodeLeaseSuccessAfterRequestBudgetStillUnknown(t *testing.T) {
	// t 提供断言失败报告、子用例和清理注册。
	synctest.Test(t, func(t *testing.T) {
		// started 记录本次处理开始时的时间。
		started := time.Now()
		// events 创建用于维护事件集合的容器或通道。
		events := make(chan nodeLeaseEvent, 4)
		// attempts 规定“attempts”的初始取值。
		attempts := 0
		// err 接收监视节点租约过程中可能产生的错误，供后续失败分支处理，attempt 记录“attempt”，使用请求或事务上下文表示。
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

// TestNodeLeaseConfirmsOwnershipLossWithoutRetry 核验明确失权应立即隔离。
// t 提供断言失败报告、子用例和清理注册。
func TestNodeLeaseConfirmsOwnershipLossWithoutRetry(t *testing.T) {
	// t 提供断言失败报告、子用例和清理注册。
	synctest.Test(t, func(t *testing.T) {
		// started 记录本次处理开始时的时间。
		started := time.Now()
		// attempts 规定“attempts”的初始取值。
		attempts := 0
		// lost 保存由失败取得的“lost”。
		lost := persistence.Fail(p.CodeRouteChanged, "运行服租约失效")
		// err 接收监视节点租约过程中可能产生的错误，供后续失败分支处理。
		err := watchNodeLease(t.Context(), started, func(context.Context) error {
			attempts++
			return lost
		}, nil)
		if !errors.Is(err, lost) || persistence.Code(err) != p.CodeRouteChanged || attempts != 1 || time.Since(started) != nodeLeaseInterval {
			t.Fatalf("明确失权没有立即隔离：%d，%v，%v", attempts, time.Since(started), err)
		}
	})
}

// TestNodeLeaseWriteConcernIsUnknownWithoutExplicitWriteError 核对「写入/权限的明确失败被错误重试」与「写确认超时被错误认定为已失权」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestNodeLeaseWriteConcernIsUnknownWithoutExplicitWriteError(t *testing.T) {
	// wc 保存当前处理所用的“wc”。
	wc := &mongo.WriteConcernError{Code: 64, Message: "waiting for replication timed out"}
	if !transientNodeLeaseError(mongo.WriteException{WriteConcernError: wc}) {
		t.Fatal("写确认超时被错误认定为已失权")
	}
	if transientNodeLeaseError(mongo.WriteException{WriteConcernError: wc, WriteErrors: mongo.WriteErrors{{Code: 11000}}}) ||
		transientNodeLeaseError(mongo.CommandError{Code: 13, Message: "Unauthorized"}) {
		t.Fatal("写入/权限的明确失败被错误重试")
	}
}

// TestInitialNodeLeaseReturnsConservativeRequestStart 核对「首次已确认窗口从返回时间错误扩大」与「超时的首次确认允许开始服务」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestInitialNodeLeaseReturnsConservativeRequestStart(t *testing.T) {
	// t 提供断言失败报告、子用例和清理注册。
	synctest.Test(t, func(t *testing.T) {
		// start 记录本次处理开始时的时间。
		start := time.Now()
		// confirmed 保存由确认初始值节点租约取得的已确认，err 接收确认初始值节点租约过程中可能产生的错误，供后续失败分支处理。
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

// TestNodeLeaseShutdownDoesNotWaitForBlockedTransactionCleanup 核对「驱动事务清理绕过退出等待上限」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestNodeLeaseShutdownDoesNotWaitForBlockedTransactionCleanup(t *testing.T) {
	// t 提供断言失败报告、子用例和清理注册。
	synctest.Test(t, func(t *testing.T) {
		// start 记录本次处理开始时的时间。
		start := time.Now()
		// endDriverWork 创建用于维护结束位置或时间“driver”工作量的容器或通道。
		endDriverWork := make(chan struct{})
		// request 记录请求，使用请求或事务上下文表示。
		var request context.Context
		// ctx 向下传递当前请求的截止时间和取消信号。
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
