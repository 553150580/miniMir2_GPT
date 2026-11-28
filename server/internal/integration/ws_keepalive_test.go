// 本文件为真实联机采样提供不会占用普通资产请求序号的心跳。
package integration

import (
	"context"
	"time"

	"github.com/coder/websocket"
	pb "minimir/api/generated/minimirv1"
	p "minimir/internal/protocol"
	"minimir/internal/wire"
)

// sampleHeartbeat 使用独立传输层探针，保证缺少业务事件时仍能按时完成固定窗。
func sampleHeartbeat(c *client, end time.Time) func() {
	ctx, cancel := context.WithCancel(c.ctx)
	ws, binding := c.ws, c.binding
	done := make(chan struct{})
	go func() {
		defer close(done)
		interval := time.NewTicker(5 * time.Second)
		defer interval.Stop()
		last := time.NewTimer(time.Until(end))
		defer last.Stop()
		send := func() error {
			write, stop := context.WithTimeout(ctx, 3*time.Second)
			defer stop()
			return ws.Write(write, websocket.MessageBinary, wire.Encode(wire.Message(p.MsgC2SHeartbeat, 0, binding, &pb.Heartbeat{})))
		}
		for {
			select {
			case <-ctx.Done():
				return
			case <-interval.C:
				if send() != nil {
					return
				}
			case <-last.C:
				_ = send()
				return
			}
		}
	}()
	return func() { cancel(); <-done }
}
