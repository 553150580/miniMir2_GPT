// 本文件通过真实 WebSocket 回归受推的场景通知及重连后的状态，使用测试专用触发。
package game

import (
	"context"
	"github.com/coder/websocket"
	"google.golang.org/protobuf/proto"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	p "minimir/internal/protocol"
	"minimir/internal/wire"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// 此适配器仅在真实WS测试中附着既有worker，不产生生产调试入口或新技能入口。
type movementWSConnection struct {
	// socket 记录“socket”，使用连接表示。
	socket *websocket.Conn
	// ctx 向下传递当前请求的截止时间和取消信号。
	ctx context.Context
	// send 记录发送，使用互斥锁表示。
	send sync.Mutex
	// closed 记录“closed”，使用“bool”表示。
	closed atomic.Bool
	// binding 记录会话绑定，使用无符号整数“64”表示。
	binding atomic.Uint64
}

// Alive 判断存活状态是否满足要求。
// c 是调用该方法的移动WebSocket连接实例。
func (c *movementWSConnection) Alive() bool { return !c.closed.Load() && c.ctx.Err() == nil }

// Deliver 判断“deliver”是否满足要求。
// c 是调用该方法的移动WebSocket连接实例，frame 记录协议帧。
func (c *movementWSConnection) Deliver(frame wire.Frame) bool {
	c.send.Lock()
	defer c.send.Unlock()
	if !c.Alive() {
		return false
	}
	return c.socket.Write(c.ctx, websocket.MessageBinary, wire.Encode(frame)) == nil
}

// Close 关闭当前请求或状态，并把结果交给调用方。
// c 是调用该方法的移动WebSocket连接实例。
func (c *movementWSConnection) Close(string) {
	if c.closed.CompareAndSwap(false, true) {
		c.socket.CloseNow()
	}
}

// SetBinding 设置会话绑定。
// c 是调用该方法的移动WebSocket连接实例，binding 携带连接上的会话代次，用于防止旧连接命令被接受。
func (c *movementWSConnection) SetBinding(binding uint64) { c.binding.Store(binding) }

// Binding 返回会话绑定供调用方使用。
// c 是调用该方法的移动WebSocket连接实例。
func (c *movementWSConnection) Binding() uint64 { return c.binding.Load() }

// SetRole 设置角色。
func (*movementWSConnection) SetRole(string) {}

// RoleID 返回角色标识供调用方使用。
func (*movementWSConnection) RoleID() string { return "role" }

// TestPushedRealWebSocketObservationAndReentryWithoutPublicTrigger 核验真实WS明确取消应同请求确认原目标身份，以及真实WS非法取消应沿原请求明确拒绝。
// t 提供断言失败报告、子用例和清理注册。
func TestPushedRealWebSocketObservationAndReentryWithoutPublicTrigger(t *testing.T) {
	// w 接收被推动位移房间执行器供后续步骤使用，r 接收被推动位移房间执行器供后续步骤使用。
	w, r, _ := pushedWorker(t)
	r.Role.ID = "role"
	r.Player.Handle = 43
	// ownerHandle 保存实体句柄，用于当前步骤中的所有者实体句柄。
	ownerHandle := r.Player.Handle
	r.Scene = uint64(1)<<63 | 11
	r.Player.Life = uint64(1)<<63 | 13
	r.Player.MovementRevision = uint64(1)<<53 | 17
	// goal 建立带初始值的点位，供本段继续处理。
	goal := devconfig.Point{X: 9, Y: 8}
	r.Destination = &goal
	r.Navigation = navigationState{Revision: uint64(1)<<63 | 19, InputSequence: 77, Destination: &goal, Status: navigationActive}
	r.MonsterCount = 0
	r.Pet.HP = 0
	// ctx 传递从调用方派生的上下文，供取消或控制截止时间，cancel 取消对应上下文或停止计时，释放其关联资源。
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	// commands 创建用于维护命令集合的容器或通道。
	commands := make(chan func(), 16)
	// ownerDone 创建用于维护所有者完成信号的容器或通道。
	ownerDone := make(chan struct{})
	go func() {
		defer close(ownerDone)
		for {
			select {
			case <-ctx.Done():
				return
			// fn 接收命令集合通道送来的“fn”。
			case fn := <-commands:
				fn()
			}
		}
	}()
	// owner 封装所有者对应的后续处理，fn 记录“fn”，使用供当前流程调用的函数表示。
	owner := func(fn func()) bool {
		// done 创建用于维护完成信号的容器或通道。
		done := make(chan struct{})
		select {
		case commands <- func() { fn(); close(done) }:
		case <-ctx.Done():
			return false
		}
		select {
		case <-done:
			return true
		case <-ctx.Done():
			return false
		}
	}
	// attached 创建用于维护“attached”的容器或通道。
	attached := make(chan *movementWSConnection, 3)
	// bindings 记录会话绑定集合，使用无符号整数“64”表示。
	var bindings atomic.Uint64
	bindings.Store(3)
	// server 保存由创建服务端取得的服务端，response 记录响应，使用响应写入器表示，request 记录请求。
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		// socket 保存由“accept”取得的“socket”，err 接收“accept”过程中可能产生的错误，供后续失败分支处理。
		socket, err := websocket.Accept(response, request, nil)
		if err != nil {
			return
		}
		defer socket.CloseNow()
		// connection 保存当前处理所用的连接。
		connection := &movementWSConnection{socket: socket, ctx: ctx}
		connection.SetBinding(bindings.Add(1))
		if !owner(func() {
			r.Conn = connection
			r.Auth.SessionEpoch = int64(connection.Binding())
			r.Ready = connection.Binding() == 4
			r.InputSequence = 0
		}) {
			return
		}
		select {
		case attached <- connection:
		case <-ctx.Done():
			return
		}
		for {
			// raw 保存由读取取得的原始值，err 接收读取过程中可能产生的错误，供后续失败分支处理。
			_, raw, err := socket.Read(ctx)
			if err != nil {
				owner(func() { w.command(command{role: "role", conn: connection, disconnect: true, at: time.Now()}) })
				return
			}
			// frame 保存由解码取得的协议帧。
			frame, err := wire.Decode(raw)
			if err != nil {
				return
			}
			if !owner(func() { w.command(command{role: "role", conn: connection, frame: frame}) }) {
				return
			}
		}
	}))
	defer func() { cancel(); server.Close(); <-ownerDone }()
	// dial 封装发起连接对应的后续处理。
	dial := func() (*websocket.Conn, *movementWSConnection) {
		// socket 保存由发起连接取得的“socket”，err 接收发起连接过程中可能产生的错误，供后续失败分支处理。
		socket, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http"), nil)
		if err != nil {
			t.Fatal(err)
		}
		select {
		// connection 接收“attached”通道送来的连接。
		case connection := <-attached:
			return socket, connection
		case <-ctx.Done():
			t.Fatal(ctx.Err())
			return nil, nil
		}
	}
	// read 封装读取对应的后续处理，socket 记录“socket”，使用连接表示。
	read := func(socket *websocket.Conn) wire.Frame {
		// raw 保存由读取取得的原始值，err 接收读取过程中可能产生的错误，供后续失败分支处理。
		_, raw, err := socket.Read(ctx)
		if err != nil {
			t.Fatal(err)
		}
		// frame 保存由解码取得的协议帧。
		frame, err := wire.Decode(raw)
		if err != nil {
			t.Fatal(err)
		}
		return frame
	}
	// socket 保存由发起连接取得的“socket”，connection 保存由发起连接取得的连接。
	socket, connection := dial()
	defer socket.CloseNow()
	owner(func() { w.send(r, p.MsgS2CEnterLine, 0, w.enterLineResult(r)) })
	// entryFrame 保存由读取取得的入场协议帧。
	entryFrame := read(socket)
	// entry 保存当前处理所用的入场。
	entry := &pb.EnterLineResult{}
	if entryFrame.ID != p.MsgS2CEnterLine || proto.Unmarshal(entryFrame.Payload, entry) != nil || entry.PlayerHandle != ownerHandle || entry.PlayerHandle == 0 {
		t.Fatal("实际入场未显式声明真实本人句柄")
	}
	// 同连接按帧顺序送出释放和只读状态屏障，证明释放摇杆不会取消地面目标。
	// outgoing 接收遍历协议帧列表时当前的元素。
	for _, outgoing := range []wire.Frame{
		wire.Message(p.MsgC2SMoveIntent, 801, connection.Binding(), &pb.MoveIntent{SceneEpoch: r.Scene, InputSequence: 1, ReleaseJoystick: true}),
		wire.Message(p.MsgC2SAutoBattle, 802, connection.Binding(), &pb.AutoBattleIntent{SceneEpoch: r.Scene}),
	} {
		// err 接收写入过程中可能产生的错误，供后续失败分支处理。
		if err := socket.Write(ctx, websocket.MessageBinary, wire.Encode(outgoing)); err != nil {
			t.Fatal(err)
		}
	}
	// released 保存由读取取得的“released”。
	if released := read(socket); released.ID != p.MsgS2CHostingState || released.Request != 802 {
		t.Fatal("摇杆释放观察屏障没有按帧序返回")
	}
	// releasedScene 保存当前处理所用的“released”场景。
	releasedScene := &pb.SceneBaselineChunk{}
	// released 保存由读取取得的“released”。
	if released := read(socket); released.ID != p.MsgS2CSceneBaseline || proto.Unmarshal(released.Payload, releasedScene) != nil || releasedScene.NavigationState.Status != navigationActive || releasedScene.NavigationState.Revision != uint64(1)<<63|19 {
		t.Fatal("真实WS释放摇杆错误取消原目标")
	}
	// before 保存位置，用于当前步骤中的操作前的状态。
	before := r.Player.Pos
	// 使用真实网络输入S2C消息号，生产worker的默认拒绝分支不得执行任何移动。
	// malicious 保存由消息取得的“malicious”。
	malicious := wire.Message(p.MsgS2CEntityDisplacement, 7, connection.Binding(), &pb.EntityDisplacement{SceneEpoch: r.Scene, EntityHandle: ownerHandle, Kind: movementTeleport, Path: []*pb.GridPoint{{X: 16, Y: 16}}})
	// err 接收写入过程中可能产生的错误，供后续失败分支处理。
	if err := socket.Write(ctx, websocket.MessageBinary, wire.Encode(malicious)); err != nil {
		t.Fatal(err)
	}
	// rejected 保存由读取取得的拒绝。
	rejected := read(socket)
	if rejected.Request != 7 || rejected.Error != p.CodeInvalidArgument {
		t.Fatal("客户端伪造S2C位移未拒绝")
	}
	owner(func() {
		if r.Player.Pos != before {
			t.Error("伪造S2C位移修改现场")
		}
	})
	// now 保存由Unix 时间取得的当前时间。
	now := time.Unix(1800000000, 0)
	// first 记录第一项，使用推入“outcome”表示。
	var first pushOutcome
	owner(func() {
		first = w.sourcePushed(r, reference(r, &r.Player), 2, 1, now, displacementSource{CastID: uint64(1)<<63 | 23})
	})
	// eventFrame 保存由读取取得的事件协议帧，baselineFrame 保存由读取取得的场景基线协议帧。
	eventFrame, baselineFrame := read(socket), read(socket)
	if first.Steps != 1 || eventFrame.ID != p.MsgS2CEntityDisplacement || baselineFrame.ID != p.MsgS2CSceneBaseline {
		t.Fatal("真实WS路径事件未先于最终基线")
	}
	// event 保存当前处理所用的事件。
	event := &pb.EntityDisplacement{}
	// scene 保存当前处理所用的场景。
	scene := &pb.SceneBaselineChunk{}
	if proto.Unmarshal(eventFrame.Payload, event) != nil || proto.Unmarshal(baselineFrame.Payload, scene) != nil {
		t.Fatal("真实WS PB解码失败")
	}
	if event.EntityHandle != entry.PlayerHandle || event.SceneEpoch != uint64(1)<<63|11 || event.EntityLife != uint64(1)<<63|13 || event.MovementRevision != uint64(1)<<53|18 || movementEntity(scene, ownerHandle).MovementRevision != event.MovementRevision || scene.NavigationState.Status != navigationActive || scene.NavigationState.Revision != uint64(1)<<63|19 {
		t.Fatal("真实WS大位宽身份/路径末基线/导航未保持")
	}
	owner(func() {
		r.Ready = false
		w.sourcePushed(r, reference(r, &r.Player), 2, 1, now, displacementSource{})
		w.baseline(r)
	})
	// frame 保存由读取取得的协议帧。
	if frame := read(socket); frame.ID != p.MsgS2CSceneBaseline {
		t.Fatal("NotReady仍向WS发送旧瞬时位移")
	}
	socket.CloseNow()
	owner(func() {
		w.command(command{role: "role", conn: connection, disconnect: true, at: now})
		w.sourcePushed(r, reference(r, &r.Player), 2, 1, now, displacementSource{})
	})
	// next 保存由发起连接取得的下一状态，nextConnection 保存由发起连接取得的下一状态连接。
	next, nextConnection := dial()
	defer next.CloseNow()
	owner(func() { w.send(r, p.MsgS2CEnterLine, 0, w.enterLineResult(r)); w.baseline(r) })
	entryFrame = read(next)
	if entryFrame.ID != p.MsgS2CEnterLine || proto.Unmarshal(entryFrame.Payload, entry) != nil || entry.PlayerHandle != ownerHandle {
		t.Fatal("健康重入丢失显式本人句柄")
	}
	// frame 保存由读取取得的协议帧。
	frame := read(next)
	if frame.ID != p.MsgS2CSceneBaseline || proto.Unmarshal(frame.Payload, scene) != nil || movementEntity(scene, ownerHandle).MovementRevision != uint64(1)<<53|20 || scene.NavigationState.Status != navigationActive || scene.NavigationState.Revision != uint64(1)<<63|19 || frame.Binding != nextConnection.Binding() {
		t.Fatal("健康WS重入丢失当前水位/目标或补播旧路径")
	}
	owner(func() {
		r.Ready = true
		// old 保存由引用取得的原状态。
		old := reference(r, &r.Player)
		w.revivePlayer(r)
		// outcome 保存由来源被推动位移取得的“outcome”。
		if outcome := w.sourcePushed(r, old, 2, 1, now, displacementSource{}); outcome.Stop != pushStale {
			t.Error("旧life force作用新life")
		}
		w.snapshot(r)
	})
	frame = read(next)
	if frame.ID != p.MsgS2CSceneBaseline || proto.Unmarshal(frame.Payload, scene) != nil || movementEntity(scene, ownerHandle).Life != uint64(1)<<63|14 || movementEntity(scene, ownerHandle).MovementRevision != 0 || scene.NavigationState.Revision != uint64(1)<<63|19 {
		t.Fatal("新life首全量补播旧瞬时/丢弃同图导航")
	}
	// outgoing 接收遍历协议帧列表时当前的元素。
	for _, outgoing := range []wire.Frame{
		wire.Message(p.MsgC2SMoveIntent, 809, nextConnection.Binding(), &pb.MoveIntent{SceneEpoch: r.Scene, InputSequence: 1, ReleaseJoystick: true}),
		wire.Message(p.MsgC2SAutoBattle, 810, nextConnection.Binding(), &pb.AutoBattleIntent{SceneEpoch: r.Scene}),
	} {
		// err 接收写入过程中可能产生的错误，供后续失败分支处理。
		if err := next.Write(ctx, websocket.MessageBinary, wire.Encode(outgoing)); err != nil {
			t.Fatal(err)
		}
	}
	if frame = read(next); frame.ID != p.MsgS2CHostingState || frame.Request != 810 {
		t.Fatal("新绑定释放摇杆没有正常受理")
	}
	if frame = read(next); frame.ID != p.MsgS2CSceneBaseline || proto.Unmarshal(frame.Payload, scene) != nil || scene.NavigationState.Status != navigationActive {
		t.Fatal("新绑定释放摇杆错误取消地面目标")
	}
	// beforeCancel 记录操作前的状态取消函数，使用实体表示。
	var beforeCancel entity
	owner(func() { beforeCancel = r.Player; r.Manual = true })
	// index 定位临时结构列表中正在处理的成员，rejected 接收遍历临时结构列表时当前的元素。
	for index, rejected := range []struct {
		// binding 携带连接上的会话代次，用于防止旧连接命令被接受。
		binding uint64
		// intent 记录意图，使用移动意图表示。
		intent *pb.MoveIntent
		// code 记录结果码，使用数值表示。
		code uint32
	}{
		{nextConnection.Binding(), &pb.MoveIntent{SceneEpoch: r.Scene + 1, InputSequence: 2, CancelNavigation: true}, p.CodeInvalidArgument},
		{connection.Binding(), &pb.MoveIntent{SceneEpoch: r.Scene, InputSequence: 2, CancelNavigation: true}, p.CodeStaleSession},
		{nextConnection.Binding(), &pb.MoveIntent{SceneEpoch: r.Scene, InputSequence: 1, CancelNavigation: true}, p.CodeStaleInput},
		{nextConnection.Binding(), &pb.MoveIntent{SceneEpoch: r.Scene, InputSequence: 2, CancelNavigation: true, Destination: pointPB(goal)}, p.CodeInvalidArgument},
		{nextConnection.Binding(), &pb.MoveIntent{SceneEpoch: r.Scene, InputSequence: 2, CancelNavigation: true, ReleaseJoystick: true}, p.CodeInvalidArgument},
	} {
		// request 保存由无符号整数“32”取得的请求。
		request := uint32(811 + index)
		// err 接收写入过程中可能产生的错误，供后续失败分支处理。
		if err := next.Write(ctx, websocket.MessageBinary, wire.Encode(wire.Message(p.MsgC2SMoveIntent, request, rejected.binding, rejected.intent))); err != nil {
			t.Fatal(err)
		}
		frame = read(next)
		if frame.ID != p.MsgC2SMoveIntent || frame.Request != request || frame.Error != rejected.code {
			t.Fatal("真实WS非法取消没有沿原请求明确拒绝")
		}
		owner(func() {
			if r.InputSequence != 1 || r.Navigation.Status != navigationActive || r.Destination == nil || *r.Destination != goal || !r.Manual || !reflect.DeepEqual(beforeCancel, r.Player) {
				t.Error("真实WS非法取消改变原地面目标或战斗状态")
			}
		})
	}
	// err 接收写入过程中可能产生的错误，供后续失败分支处理。
	if err := next.Write(ctx, websocket.MessageBinary, wire.Encode(wire.Message(p.MsgC2SMoveIntent, 803, nextConnection.Binding(), &pb.MoveIntent{SceneEpoch: r.Scene, InputSequence: 2, CancelNavigation: true}))); err != nil {
		t.Fatal(err)
	}
	frame = read(next)
	if frame.ID != p.MsgS2CSceneBaseline || frame.Request != 803 || frame.Binding != nextConnection.Binding() || proto.Unmarshal(frame.Payload, scene) != nil || scene.NavigationState.Status != navigationCancelled || scene.NavigationState.Revision != uint64(1)<<63|19 || scene.NavigationState.InputSequence != 77 || !proto.Equal(scene.NavigationState.Destination, pointPB(goal)) {
		t.Fatal("真实WS明确取消没有同请求确认原目标身份")
	}
	owner(func() {
		if r.Manual || r.Destination != nil || !reflect.DeepEqual(beforeCancel, r.Player) {
			t.Error("明确取消改变了位置/HP/CD或未释放摇杆")
		}
	})
	// 用户等待取消基线确认后才断线；健康重入仅观察取消状态，不恢复旧地面目标或旧路径。
	next.CloseNow()
	owner(func() { w.command(command{role: "role", conn: nextConnection, disconnect: true, at: now}) })
	// last 保存由发起连接取得的上一次状态，lastConnection 保存由发起连接取得的上一次状态连接。
	last, lastConnection := dial()
	defer last.CloseNow()
	owner(func() { w.send(r, p.MsgS2CEnterLine, 0, w.enterLineResult(r)); w.baseline(r) })
	if entryFrame = read(last); entryFrame.ID != p.MsgS2CEnterLine {
		t.Fatal("取消后健康重入没有正常入场")
	}
	frame = read(last)
	if frame.ID != p.MsgS2CSceneBaseline || frame.Request != 0 || frame.Binding != lastConnection.Binding() || proto.Unmarshal(frame.Payload, scene) != nil || scene.NavigationState.Status != navigationCancelled || scene.NavigationState.Revision != uint64(1)<<63|19 || scene.NavigationState.InputSequence != 77 || !proto.Equal(scene.NavigationState.Destination, pointPB(goal)) {
		t.Fatal("取消后健康重入恢复旧目标/补播旧路径或丢失取消身份")
	}
}
