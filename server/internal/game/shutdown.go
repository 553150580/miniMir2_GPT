package game

import (
	"context"
	"fmt"
	"time"
)

type settlementDrainStatus struct {
	pending  bool
	isolated int
}

// DrainSettlements 停止接收新操作，由原房间 worker 结清已计量整秒和已发生的收益。
// 调用方在等待期间继续维持节点租约。失去租约时立即取消等待，MongoDB 围栏继续约束在途写入。
func (e *Engine) DrainSettlements(ctx context.Context) error {
	e.stopped.Store(true)
	interval := time.NewTicker(25 * time.Millisecond)
	defer interval.Stop()
	results := make(chan settlementDrainStatus, len(e.workers))
	check := command{done: func(w *worker) {
		var status settlementDrainStatus
		if w.e.ctx.Err() != nil || ctx.Err() != nil {
			results <- status
			return
		}
		now := time.Now()
		for _, r := range w.rooms {
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
		for _, w := range e.workers {
			select {
			case w.commands <- check:
			case <-ctx.Done():
				return fmt.Errorf("停服排队结算未完成: %w", ctx.Err())
			case <-e.ctx.Done():
				return fmt.Errorf("停服中断结算: %w", e.ctx.Err())
			}
		}
		var status settlementDrainStatus
		for range e.workers {
			select {
			case workerStatus := <-results:
				status.pending = workerStatus.pending || status.pending
				status.isolated += workerStatus.isolated
			case <-ctx.Done():
				return fmt.Errorf("停服等待结算未完成: %w", ctx.Err())
			case <-e.ctx.Done():
				return fmt.Errorf("停服中断结算: %w", e.ctx.Err())
			}
		}
		if !status.pending {
			if err := e.ctx.Err(); err != nil {
				return fmt.Errorf("停服中断结算: %w", err)
			}
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
