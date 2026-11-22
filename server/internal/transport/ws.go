// 本文件处理 WebSocket 升级、来源与连接配额、二进制帧收发、待入场超时和连接代次。

package transport

import (
	"context"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"
	"google.golang.org/protobuf/proto"
	pb "minimir/api/generated/minimirv1"
	p "minimir/internal/protocol"
	"minimir/internal/wire"
)

// initialAdmissionTimeout 从 HTTP 升级成功起限制尚无角色绑定的连接寿命。
const initialAdmissionTimeout = 10 * time.Second

const (
	admissionWaiting uint32 = iota // 已升级，等待可用的角色绑定。
	admissionBound                 // 已取得角色连接代次，关闭初始准入计时。
	admissionExpired               // 超过初始准入截止，不再收发普通帧。
)

// connection 保存传输状态和队列，房间中的角色状态只由游戏协程修改。
type connection struct {
	socket         *websocket.Conn    // 已升级的底层连接。
	ctx            context.Context    // 连接收发共用的生命周期。
	cancel         context.CancelFunc // 通知读写和等待者退出。
	queue          chan []byte        // 等待单个写协程发送的普通帧。
	terminal       chan []byte        // 入场拒绝等必须优先写出的最后一帧。
	queued         atomic.Int64       // 已排队普通帧的字节预算。
	role           atomic.Value       // 从票据识别出的角色标识，可先于绑定写入。
	binding        atomic.Uint64      // 持久层确认的角色会话代次。
	closed         atomic.Bool        // 底层连接关闭是否已启动。
	ending         atomic.Bool        // 是否已承诺写最后一帧后关闭。
	entry          atomic.Bool        // 本连接是否已发过入场请求。
	admission      atomic.Uint32      // 连接的初始准入状态。
	admissionTimer *time.Timer        // 尚未绑定时负责主动释放连接的定时器。
}

// Alive 判断连接是否还能接收新的普通业务帧。
// c 是调用该方法的连接实例。
func (c *connection) Alive() bool {
	return c.admission.Load() != admissionExpired && !c.closed.Load() && !c.ending.Load() && c.ctx.Err() == nil
}

// Deliver 尝试将编码帧排入写队列。长度或积压超限时主动切断慢连接。
// c 是调用该方法的连接实例，f 提供待写入对端的协议帧。
func (c *connection) Deliver(f wire.Frame) bool {
	if !c.Alive() {
		return false
	}
	// b 保存待排队的已编码应用帧，用其长度核算字节预算。
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

// 入场拒绝交给同一个写协程发送，房间线程不等待网络，也不让普通帧占住终止槽。
// c 是调用该方法的连接实例，f 提供待写入对端的协议帧。
func (c *connection) DeliverAndClose(f wire.Frame, _ string) {
	if c.Alive() && c.ending.CompareAndSwap(false, true) {
		c.terminal <- wire.Encode(f)
	}
}

// Close 仅在首次关闭时取消连接上下文并立即关闭底层套接字。
// c 是调用该方法的连接实例。
func (c *connection) Close(_ string) {
	if c.closed.CompareAndSwap(false, true) {
		c.cancel()
		c.socket.CloseNow()
	}
}

// SetRole 设置角色。
// c 是调用该方法的连接实例，id 区分当前步骤指向的目标对象。
func (c *connection) SetRole(id string) { c.role.Store(id) }

// RoleID 返回角色标识供调用方使用。
// c 是调用该方法的连接实例。
func (c *connection) RoleID() string {
	// v 接收加载供后续步骤使用。
	v := c.role.Load()
	if v == nil {
		return ""
	}
	return v.(string)
}

// SetBinding 发布持久层确认的连接代次，并尝试结束初始准入计时。
// c 是调用该方法的连接实例，v 作为这次角色入场确认的新连接代次。
func (c *connection) SetBinding(v uint64) {
	if v != 0 && c.admission.CompareAndSwap(admissionWaiting, admissionBound) {
		c.admissionTimer.Stop()
	}
	c.binding.Store(v)
}

// Binding 返回会话绑定供调用方使用。
// c 是调用该方法的连接实例。
func (c *connection) Binding() uint64 { return c.binding.Load() }

// ws 处理WebSocket。
// s 是调用该方法的服务端实例，w 提供 HTTP 升级所需的响应写入器，r 提供升级请求、来源和生命周期。
func (s *Server) ws(w http.ResponseWriter, r *http.Request) {
	s.serveWebSocket(w, r, initialAdmissionTimeout)
}

// serveWebSocket 设置连接预算，启动单写协程，将准入和已绑定消息交给游戏引擎。
// s 是调用该方法的服务端实例，w 提供 HTTP 升级所需的响应写入器，r 提供升级请求、来源和生命周期，admissionTimeout 限定连接从升级成功到取得角色绑定的最长时间。
func (s *Server) serveWebSocket(w http.ResponseWriter, r *http.Request, admissionTimeout time.Duration) {
	if s.active.Add(1) > 128 {
		s.active.Add(-1)
		http.Error(w, "development connection budget", 503)
		return
	}
	defer s.active.Add(-1)
	// socket 保存由“accept”取得的“socket”，e 接收“accept”供后续步骤使用。
	socket, e := websocket.Accept(w, r, &websocket.AcceptOptions{OriginPatterns: []string{"127.0.0.1:*", "localhost:*"}})
	if e != nil {
		return
	}
	socket.SetReadLimit(wire.MaxFrame)
	// ctx 传递从调用方派生的上下文，供取消或控制截止时间，cancel 取消对应上下文或停止计时，释放其关联资源。
	ctx, cancel := context.WithCancel(r.Context())
	// c 保存当前处理所用的连接。
	c := &connection{socket: socket, ctx: ctx, cancel: cancel, queue: make(chan []byte, 128), terminal: make(chan []byte, 1)}
	// 起算于升级成功，只有有效角色绑定能结束此计时。心跳和提交票据本身均不延长截止。
	c.admissionTimer = time.AfterFunc(admissionTimeout, func() {
		if c.admission.CompareAndSwap(admissionWaiting, admissionExpired) {
			c.Close("initial admission timeout")
		}
	})
	defer c.admissionTimer.Stop()
	defer c.Close("disconnected")
	defer s.Engine.Disconnect(c)
	// writerDone 允许连接关闭时等待写协程退出并回收资源。
	writerDone := make(chan struct{})
	go func() {
		defer close(writerDone)
		for {
			// b 携带供当前流程读取或解析的原始字节。
			var b []byte
			// ending 标记当前写出的帧是最后一帧，写完即可关闭连接。
			ending := true
			// 先检查终止槽，已排队的心跳不会延迟入场拒绝。
			select {
			case b = <-c.terminal:
			default:
				select {
				case <-ctx.Done():
					return
				case b = <-c.terminal:
				case b = <-c.queue:
					ending = false
				}
			}
			// timeout 保存根据当前数据与秒计算的超时。
			timeout := 5 * time.Second
			if ending {
				timeout = time.Second
			} else {
				c.queued.Add(-int64(len(b)))
			}
			// writeCtx 传递从调用方派生的上下文，供取消或控制截止时间，stop 取消对应上下文或停止计时，释放其关联资源。
			writeCtx, stop := context.WithTimeout(ctx, timeout)
			// err 接收写入过程中可能产生的错误，供后续失败分支处理。
			err := socket.Write(writeCtx, websocket.MessageBinary, b)
			stop()
			if ending || err != nil {
				c.Close("writer stopped")
				return
			}
		}
	}()
	defer func() { cancel(); socket.CloseNow(); <-writerDone }()
	// 心跳由传输层直答。单次入场请求走独立入口，其余消息按角色分片投递。
	for {
		// kind 保存由读取取得的类型，b 保存读取返回的结果，err 接收读取过程中可能产生的错误，供后续失败分支处理。
		kind, b, err := socket.Read(ctx)
		if err != nil {
			return
		}
		if c.ending.Load() {
			continue
		}
		if kind != websocket.MessageBinary {
			c.Close("binary frames required")
			return
		}
		// f 接收解码供后续步骤使用。
		f, err := wire.Decode(b)
		if err != nil || f.Error != 0 {
			c.Close("invalid frame")
			return
		}
		if f.ID == p.MsgC2SHeartbeat {
			// q 记录“heartbeat”。
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
			// q 记录入场线路请求。
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
