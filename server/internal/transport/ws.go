package transport

import (
	"context"
	"github.com/coder/websocket"
	"google.golang.org/protobuf/proto"
	pb "minimir/api/generated/minimirv1"
	p "minimir/internal/protocol"
	"minimir/internal/wire"
	"net/http"
	"sync/atomic"
	"time"
)

type connection struct {
	socket  *websocket.Conn
	ctx     context.Context
	cancel  context.CancelFunc
	queue   chan []byte
	queued  atomic.Int64
	role    atomic.Value
	binding atomic.Uint64
	closed  atomic.Bool
	entry   atomic.Bool
}

func (c *connection) Alive() bool { return !c.closed.Load() && c.ctx.Err() == nil }
func (c *connection) Deliver(f wire.Frame) bool {
	if c.closed.Load() {
		return false
	}
	b := wire.Encode(f)
	if len(b) > wire.MaxFrame || c.queued.Add(int64(len(b))) > 256*1024 {
		c.Close("send budget exceeded")
		return false
	}
	select {
	case c.queue <- b:
		return true
	default:
		c.queued.Add(-int64(len(b)))
		c.Close("slow connection")
		return false
	}
}
func (c *connection) Close(_ string) {
	if c.closed.CompareAndSwap(false, true) {
		c.cancel()
		c.socket.CloseNow()
	}
}
func (c *connection) SetRole(id string) { c.role.Store(id) }
func (c *connection) RoleID() string {
	v := c.role.Load()
	if v == nil {
		return ""
	}
	return v.(string)
}
func (c *connection) SetBinding(v uint64) { c.binding.Store(v) }
func (c *connection) Binding() uint64     { return c.binding.Load() }
func (s *Server) ws(w http.ResponseWriter, r *http.Request) {
	if s.active.Add(1) > 128 {
		s.active.Add(-1)
		http.Error(w, "development connection budget", 503)
		return
	}
	defer s.active.Add(-1)
	socket, e := websocket.Accept(w, r, &websocket.AcceptOptions{OriginPatterns: []string{"127.0.0.1:*", "localhost:*"}})
	if e != nil {
		return
	}
	socket.SetReadLimit(wire.MaxFrame)
	ctx, cancel := context.WithCancel(r.Context())
	c := &connection{socket: socket, ctx: ctx, cancel: cancel, queue: make(chan []byte, 128)}
	defer c.Close("disconnected")
	defer s.Engine.Disconnect(c)
	writerDone := make(chan struct{})
	go func() {
		defer close(writerDone)
		for {
			select {
			case <-ctx.Done():
				return
			case b := <-c.queue:
				c.queued.Add(-int64(len(b)))
				writeCtx, stop := context.WithTimeout(ctx, 5*time.Second)
				err := socket.Write(writeCtx, websocket.MessageBinary, b)
				stop()
				if err != nil {
					c.Close("write failed")
					return
				}
			}
		}
	}()
	defer func() { cancel(); socket.CloseNow(); <-writerDone }()
	for {
		kind, b, err := socket.Read(ctx)
		if err != nil {
			return
		}
		if kind != websocket.MessageBinary {
			c.Close("binary frames required")
			return
		}
		f, err := wire.Decode(b)
		if err != nil || f.Error != 0 {
			c.Close("invalid frame")
			return
		}
		if f.ID == p.MsgC2SHeartbeat {
			var q pb.Heartbeat
			if proto.Unmarshal(f.Payload, &q) != nil {
				return
			}
			c.Deliver(wire.Message(p.MsgS2CHeartbeat, f.Request, c.Binding(), &pb.HeartbeatResult{ServerUnixMs: uint64(time.Now().UnixMilli())}))
			continue
		}
		if f.ID == p.MsgC2SEnterLine {
			if !c.entry.CompareAndSwap(false, true) {
				c.Deliver(wire.Frame{ID: p.MsgS2CEnterLine, Request: f.Request, Error: p.CodeInvalidArgument})
				continue
			}
			var q pb.EnterLineRequest
			if proto.Unmarshal(f.Payload, &q) != nil {
				return
			}
			s.Engine.Enter(q.AccountTicket, c, f.Request)
			continue
		}
		s.Engine.Handle(c, f)
	}
}
