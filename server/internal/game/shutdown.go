// 本文件停止接收新操作，调用原房间分片收束已受理的结算，受调用方期限和租约上下文约束。
package game

import (
	"context"
	"fmt"
	"time"
)

// settlementDrainStatus 汇总一个分片的停服结算状态。
type settlementDrainStatus struct {
	pending  bool // 该分片仍有可继续确认的计量、收益或其他资产操作。
	isolated int  // 该分片已因无法安全提交而隔离的房间数。
}

// DrainSettlements 停止接收新操作，由原房间 worker 结清已计量整秒和已发生的收益。
// 调用方在等待期间继续维持节点租约。失去租约时立即取消等待，MongoDB 围栏继续约束在途写入。
// e 是调用该方法的游戏引擎实例，ctx 限定调用方愿意等待停服结清的最晚时点。
func (e *Engine) DrainSettlements(ctx context.Context) error {
	e.stopped.Store(true)
	// interval 以既定周期唤醒下一轮分片检查，避免持续空转。
	interval := time.NewTicker(25 * time.Millisecond)
	defer interval.Stop()
	// results 收集每个房间分片在同一轮生成的结清状态。
	results := make(chan settlementDrainStatus, len(e.workers))
	// 通过原分片命令检查和推进结算，停服发起协程无需直接读取并发房间状态。
	// check 是投递至原分片的收束命令，由房间所有者结清未提交的事实，w 记录房间执行器。
	check := command{done: func(w *worker) {
		// status 记录尚未结清的工作及已隔离房间数，在分片检查或汇总阶段分别使用。
		var status settlementDrainStatus
		if w.e.ctx.Err() != nil || ctx.Err() != nil {
			results <- status
			return
		}
		// now 记录本次处理开始时的时间。
		now := time.Now()
		// r 接收遍历房间集合时当前的元素。
		for _, r := range w.rooms {
			// pending 表示该单个房间仍有计量、收益或资产操作待确认。
			pending := w.drainRoomSettlement(r, now)
			if r.Isolated {
				status.isolated++
				continue
			}
			status.pending = pending || status.pending
		}
		results <- status
	}}
	for {
		// w 接收遍历房间执行器集合时当前的元素。
		for _, w := range e.workers {
			select {
			case w.commands <- check:
			case <-ctx.Done():
				return fmt.Errorf("停服排队结算未完成: %w", ctx.Err())
			case <-e.ctx.Done():
				return fmt.Errorf("停服中断结算: %w", e.ctx.Err())
			}
		}
		// status 记录尚未结清的工作及已隔离房间数，在分片检查或汇总阶段分别使用。
		var status settlementDrainStatus
		for range e.workers {
			select {
			// workerStatus 是一个房间分片的待确认状态与隔离统计。
			case workerStatus := <-results:
				status.pending = workerStatus.pending || status.pending
				status.isolated += workerStatus.isolated
			case <-ctx.Done():
				return fmt.Errorf("停服等待结算未完成: %w", ctx.Err())
			case <-e.ctx.Done():
				return fmt.Errorf("停服中断结算: %w", e.ctx.Err())
			}
		}
		// 等待所有尚可结清的房间完成后，再汇报隔离风险或成功完成。
		if !status.pending {
			// err 保存当前调用返回的错误，供后续失败分支处理。
			if err := e.ctx.Err(); err != nil {
				return fmt.Errorf("停服中断结算: %w", err)
			}
			// err 保存当前调用返回的错误，供后续失败分支处理。
			if err := ctx.Err(); err != nil {
				return fmt.Errorf("停服确认结算未完成: %w", err)
			}
			if status.isolated > 0 {
				return fmt.Errorf("停服无法结清 %d 个已隔离房间", status.isolated)
			}
			return nil
		}
		select {
		case <-interval.C:
		case <-ctx.Done():
			return fmt.Errorf("停服确认结算未完成: %w", ctx.Err())
		case <-e.ctx.Done():
			return fmt.Errorf("停服中断结算: %w", e.ctx.Err())
		}
	}
}
