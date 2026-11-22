// 本文件根据最后一次确认的租约计算本地安全截止，重试暂态失败并隔离失权进程。

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
	nodeLeaseInterval = 3 * time.Second                               // 健康节点的常规续租间隔。
	nodeLeaseTimeout  = 2 * time.Second                               // 单次确认或释放的等待上限。
	nodeLeaseWindow   = persistence.NodeLeaseDuration - 3*time.Second // 距最近确认续租开始时间的本地安全窗口。
	nodeLeaseRetryMin = 250 * time.Millisecond                        // 暂态失败后的最小退避。
	nodeLeaseRetryMax = time.Second                                   // 同次故障的退避上限。
)

// errNodeLeaseUnconfirmed 标记本地安全截止已经用尽，节点应立即隔离。
var errNodeLeaseUnconfirmed = errors.New("达到最近一次已确认续租的安全截止")

// nodeLeaseEvent 保存节点租约事件所需的状态。
type nodeLeaseEvent struct {
	// err 保存当前操作的失败原因，供调用方检查或返回。
	err error
	// elapsed 记录已经经过的时长。
	elapsed time.Duration
}

// nodeLeaseResult 保存节点租约结果所需的状态。
type nodeLeaseResult struct {
	// err 保存当前操作的失败原因，供调用方检查或返回。
	err error
	// completed 记录“completed”，使用时间点表示。
	completed time.Time
}

// releaseNodeLease 释放节点租约。
// release 记录释放，使用供当前流程调用的函数表示。
func releaseNodeLease(release func(context.Context) error) {
	// ctx 传递从调用方派生的上下文，供取消或控制截止时间，stop 取消对应上下文或停止计时，释放其关联资源。
	ctx, stop := context.WithTimeout(context.Background(), nodeLeaseTimeout)
	defer stop()
	// done 创建用于维护完成信号的容器或通道。
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

// confirmInitialNodeLease 在对外监听前确认首个短时续租，返回可用于计算保守截止的请求起点。
// ctx 向下传递当前请求的截止时间和取消信号，renew 记录续租，使用供当前流程调用的函数表示。
func confirmInitialNodeLease(ctx context.Context, renew func(context.Context) error) (time.Time, error) {
	// started 记录本次处理开始时的时间。
	started := time.Now()
	// deadline 保存由新增取得的截止时间。
	deadline := started.Add(nodeLeaseTimeout)
	// attempt 传递从调用方派生的上下文，供取消或控制截止时间，stop 取消对应上下文或停止计时，释放其关联资源。
	attempt, stop := context.WithDeadline(ctx, deadline)
	defer stop()
	// err 接收续租过程中可能产生的错误，供后续失败分支处理。
	if err := renew(attempt); err != nil {
		return time.Time{}, err
	}
	// err 保存当前调用返回的错误，供后续失败分支处理。
	if err := ctx.Err(); err != nil {
		return time.Time{}, err
	}
	if !time.Now().Before(deadline) {
		return time.Time{}, context.DeadlineExceeded
	}
	return started, nil
}

// transientNodeLeaseError 判断未知确认、网络和上下文超时能否在剩余租期内继续核对。
// err 保存当前操作的失败原因，供调用方检查或返回。
func transientNodeLeaseError(err error) bool {
	// write 记录写入，使用写入“exception”表示。
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

// reportNodeLease 处理报告或战报节点租约。
// events 记录事件集合，使用节点租约事件通道表示，event 记录事件，使用节点租约事件表示。
func reportNodeLease(events chan<- nodeLeaseEvent, event nodeLeaseEvent) {
	// 日志I/O不能阻塞安全截止；每轮恢复只有首次暂态与恢复两条事件。
	select {
	case events <- event:
	default:
	}
}

// logNodeLeaseEvents 处理日志节点租约事件集合。
// ctx 向下传递当前请求的截止时间和取消信号，mode 记录模式，使用文本表示，events 记录事件集合，使用节点租约事件通道表示。
func logNodeLeaseEvents(ctx context.Context, mode string, events <-chan nodeLeaseEvent) {
	for {
		select {
		case <-ctx.Done():
			return
		// event 接收事件集合通道送来的事件。
		case event := <-events:
			if event.err != nil {
				slog.Warn("节点续租遇到暂态错误，限时重试", "mode", mode, "error", event.err)
			} else {
				slog.Info("节点续租已恢复", "mode", mode, "elapsed_ms", event.elapsed.Milliseconds())
			}
		}
	}
}

// watchNodeLease 按已确认的租约窗口推进续租，越过本地截止前始终保留退出通道。
// ctx 向下传递当前请求的截止时间和取消信号，confirmedStarted 记录已确认已起点或开始时间，使用时间点表示，renew 记录续租，使用供当前流程调用的函数表示。
// events 记录事件集合，使用节点租约事件通道表示。
func watchNodeLease(ctx context.Context, confirmedStarted time.Time, renew func(context.Context) error, events chan<- nodeLeaseEvent) error {
	// Mongo写入时刻不可能早于发送本次请求的单调时刻。首个和后续租期都按该保守下界计算。
	// deadline 保存由新增取得的截止时间。
	deadline := confirmedStarted.Add(nodeLeaseWindow)
	// expiry 保存由创建定时器取得的过期时间。
	expiry := time.NewTimer(time.Until(deadline))
	defer expiry.Stop()
	// next 保存由创建定时器取得的下一状态。
	next := time.NewTimer(time.Until(confirmedStarted.Add(nodeLeaseInterval)))
	defer next.Stop()
	// retry 保存当前处理所用的重试。
	retry := nodeLeaseRetryMin
	// lastUnknown 记录上一次状态未知，使用错误表示。
	var lastUnknown error
	// incidentStarted 记录“incident”已起点或开始时间，使用时间点表示。
	var incidentStarted time.Time
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-expiry.C:
			return errors.Join(errNodeLeaseUnconfirmed, lastUnknown)
		case <-next.C:
		}
		// started 记录本次处理开始时的时间。
		started := time.Now()
		if !started.Before(deadline) {
			return errors.Join(errNodeLeaseUnconfirmed, lastUnknown)
		}
		// attemptUntil 保存由新增取得的“attempt”有效截止时间。
		attemptUntil := started.Add(nodeLeaseTimeout)
		if deadline.Before(attemptUntil) {
			attemptUntil = deadline
		}
		// attempt 传递从调用方派生的上下文，供取消或控制截止时间，stop 取消对应上下文或停止计时，释放其关联资源。
		attempt, stop := context.WithDeadline(ctx, attemptUntil)
		// 每次只发一个请求。即使驱动迟迟未返回，独立截止也能停止服务。
		// done 创建用于维护完成信号的容器或通道。
		done := make(chan nodeLeaseResult, 1)
		go func() {
			// err 保存当前调用返回的错误，供后续失败分支处理。
			err := attempt.Err()
			if err == nil {
				err = renew(attempt)
			}
			done <- nodeLeaseResult{err: err, completed: time.Now()}
		}()
		// result 记录结果，使用节点租约结果表示。
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
		// now 记录本次处理开始时的时间。
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
