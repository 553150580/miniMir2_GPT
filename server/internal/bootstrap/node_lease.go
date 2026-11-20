package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"go.mongodb.org/mongo-driver/v2/mongo"
	"minimir/internal/persistence"
)

const (
	nodeLeaseInterval = 3 * time.Second
	nodeLeaseTimeout  = 2 * time.Second
	nodeLeaseWindow   = persistence.NodeLeaseDuration - 3*time.Second
	nodeLeaseRetryMin = 250 * time.Millisecond
	nodeLeaseRetryMax = time.Second
)

var errNodeLeaseUnconfirmed = errors.New("达到最近一次已确认续租的安全截止")

type nodeLeaseEvent struct {
	err     error
	elapsed time.Duration
}

type nodeLeaseResult struct {
	err       error
	completed time.Time
}

func releaseNodeLease(release func(context.Context) error) {
	ctx, stop := context.WithTimeout(context.Background(), nodeLeaseTimeout)
	defer stop()
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = release(ctx)
	}()
	// 驱动可能在独立上下文完成事务清理；旧实例不等待它，无确认时由原租约自然到期。
	select {
	case <-done:
	case <-ctx.Done():
	}
}

func confirmInitialNodeLease(ctx context.Context, renew func(context.Context) error) (time.Time, error) {
	started := time.Now()
	deadline := started.Add(nodeLeaseTimeout)
	attempt, stop := context.WithDeadline(ctx, deadline)
	defer stop()
	if err := renew(attempt); err != nil {
		return time.Time{}, err
	}
	if err := ctx.Err(); err != nil {
		return time.Time{}, err
	}
	if !time.Now().Before(deadline) {
		return time.Time{}, context.DeadlineExceeded
	}
	return started, nil
}

func transientNodeLeaseError(err error) bool {
	var write mongo.WriteException
	if errors.As(err, &write) && len(write.WriteErrors) != 0 {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) || mongo.IsTimeout(err) || mongo.IsNetworkError(err) {
		return true
	}
	// 写确认超时不证明是否写入；显式写失败或业务失权仍立即停止。
	return write.WriteConcernError != nil && write.WriteConcernError.Code == 64
}

func reportNodeLease(events chan<- nodeLeaseEvent, event nodeLeaseEvent) {
	// 日志I/O不能阻塞安全截止；每轮恢复只有首次暂态与恢复两条事件。
	select {
	case events <- event:
	default:
	}
}

func logNodeLeaseEvents(ctx context.Context, mode string, events <-chan nodeLeaseEvent) {
	for {
		select {
		case <-ctx.Done():
			return
		case event := <-events:
			if event.err != nil {
				slog.Warn("节点续租遇到暂态错误，限时重试", "mode", mode, "error", event.err)
			} else {
				slog.Info("节点续租已恢复", "mode", mode, "elapsed_ms", event.elapsed.Milliseconds())
			}
		}
	}
}

func watchNodeLease(ctx context.Context, confirmedStarted time.Time, renew func(context.Context) error, events chan<- nodeLeaseEvent) error {
	// Mongo写入时刻不可能早于发送本次请求的单调时刻。首个和后续租期都按该保守下界计算。
	deadline := confirmedStarted.Add(nodeLeaseWindow)
	expiry := time.NewTimer(time.Until(deadline))
	defer expiry.Stop()
	next := time.NewTimer(time.Until(confirmedStarted.Add(nodeLeaseInterval)))
	defer next.Stop()
	retry := nodeLeaseRetryMin
	var lastUnknown error
	var incidentStarted time.Time
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-expiry.C:
			return errors.Join(errNodeLeaseUnconfirmed, lastUnknown)
		case <-next.C:
		}
		started := time.Now()
		if !started.Before(deadline) {
			return errors.Join(errNodeLeaseUnconfirmed, lastUnknown)
		}
		attemptUntil := started.Add(nodeLeaseTimeout)
		if deadline.Before(attemptUntil) {
			attemptUntil = deadline
		}
		attempt, stop := context.WithDeadline(ctx, attemptUntil)
		// 每次只发一个请求。即使驱动迟迟未返回，独立截止也能停止服务。
		done := make(chan nodeLeaseResult, 1)
		go func() {
			err := attempt.Err()
			if err == nil {
				err = renew(attempt)
			}
			done <- nodeLeaseResult{err: err, completed: time.Now()}
		}()
		var result nodeLeaseResult
		select {
		case <-ctx.Done():
			stop()
			return nil
		case <-expiry.C:
			stop()
			return errors.Join(errNodeLeaseUnconfirmed, lastUnknown)
		case result = <-done:
			stop()
		}
		if ctx.Err() != nil {
			return nil
		}
		now := time.Now()
		if !now.Before(deadline) {
			return errors.Join(errNodeLeaseUnconfirmed, lastUnknown, result.err)
		}
		if result.err == nil && !result.completed.Before(attemptUntil) {
			result.err = context.DeadlineExceeded
		}
		if result.err == nil {
			deadline = started.Add(nodeLeaseWindow)
			expiry.Reset(time.Until(deadline))
			next.Reset(time.Until(started.Add(nodeLeaseInterval)))
			if lastUnknown != nil {
				reportNodeLease(events, nodeLeaseEvent{elapsed: now.Sub(incidentStarted)})
			}
			lastUnknown, retry = nil, nodeLeaseRetryMin
			continue
		}
		if !transientNodeLeaseError(result.err) {
			return fmt.Errorf("节点续租确认失败：%w", result.err)
		}
		if lastUnknown == nil {
			incidentStarted = started
			reportNodeLease(events, nodeLeaseEvent{err: result.err})
		}
		lastUnknown = result.err
		next.Reset(retry)
		retry = min(retry*2, nodeLeaseRetryMax)
	}
}
