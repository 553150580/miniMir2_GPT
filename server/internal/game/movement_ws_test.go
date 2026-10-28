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
	socket  *websocket.Conn
	ctx     context.Context
	send    sync.Mutex
	closed  atomic.Bool
	binding atomic.Uint64
}

func (c *movementWSConnection) Alive() bool { return !c.closed.Load() && c.ctx.Err() == nil }
func (c *movementWSConnection) Deliver(frame wire.Frame) bool {
	c.send.Lock()
	defer c.send.Unlock()
	if !c.Alive() {
		return false
	}
	return c.socket.Write(c.ctx, websocket.MessageBinary, wire.Encode(frame)) == nil
}
func (c *movementWSConnection) Close(string) {
	if c.closed.CompareAndSwap(false, true) {
		c.socket.CloseNow()
	}
}
func (c *movementWSConnection) SetBinding(binding uint64) { c.binding.Store(binding) }
func (c *movementWSConnection) Binding() uint64           { return c.binding.Load() }
func (*movementWSConnection) SetRole(string)              {}
func (*movementWSConnection) RoleID() string              { return "role" }

func TestPushedRealWebSocketObservationAndReentryWithoutPublicTrigger(t *testing.T) {
	w, r, _ := pushedWorker(t)
	r.Role.ID = "role"
	r.Player.Handle = 43
	ownerHandle := r.Player.Handle
	r.Scene = uint64(1)<<63 | 11
	r.Player.Life = uint64(1)<<63 | 13
	r.Player.MovementRevision = uint64(1)<<53 | 17
	goal := devconfig.Point{X: 9, Y: 8}
	r.Destination = &goal
	r.Navigation = navigationState{Revision: uint64(1)<<63 | 19, InputSequence: 77, Destination: &goal, Status: navigationActive}
	r.MonsterCount = 0
	r.Pet.HP = 0
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	commands := make(chan func(), 16)
	ownerDone := make(chan struct{})
	go func() {
		defer close(ownerDone)
		for {
			select {
			case <-ctx.Done():
				return
			case fn := <-commands:
				fn()
			}
		}
	}()
	owner := func(fn func()) bool {
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
	attached := make(chan *movementWSConnection, 3)
	var bindings atomic.Uint64
	bindings.Store(3)
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		socket, err := websocket.Accept(response, request, nil)
		if err != nil {
			return
		}
		defer socket.CloseNow()
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
			_, raw, err := socket.Read(ctx)
			if err != nil {
				owner(func() { w.command(command{role: "role", conn: connection, disconnect: true, at: time.Now()}) })
				return
			}
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
	dial := func() (*websocket.Conn, *movementWSConnection) {
		socket, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http"), nil)
		if err != nil {
			t.Fatal(err)
		}
		select {
		case connection := <-attached:
			return socket, connection
		case <-ctx.Done():
			t.Fatal(ctx.Err())
			return nil, nil
		}
	}
	read := func(socket *websocket.Conn) wire.Frame {
		_, raw, err := socket.Read(ctx)
		if err != nil {
			t.Fatal(err)
		}
		frame, err := wire.Decode(raw)
		if err != nil {
			t.Fatal(err)
		}
		return frame
	}
	socket, connection := dial()
	defer socket.CloseNow()
	owner(func() { w.send(r, p.MsgS2CEnterLine, 0, w.enterLineResult(r)) })
	entryFrame := read(socket)
	entry := &pb.EnterLineResult{}
	if entryFrame.ID != p.MsgS2CEnterLine || proto.Unmarshal(entryFrame.Payload, entry) != nil || entry.PlayerHandle != ownerHandle || entry.PlayerHandle == 0 {
		t.Fatal("实际入场未显式声明真实本人句柄")
	}
	// 同连接按帧顺序送出释放和只读状态屏障，证明释放摇杆不会取消地面目标。
	for _, outgoing := range []wire.Frame{
		wire.Message(p.MsgC2SMoveIntent, 801, connection.Binding(), &pb.MoveIntent{SceneEpoch: r.Scene, InputSequence: 1, ReleaseJoystick: true}),
		wire.Message(p.MsgC2SAutoBattle, 802, connection.Binding(), &pb.AutoBattleIntent{SceneEpoch: r.Scene}),
	} {
		if err := socket.Write(ctx, websocket.MessageBinary, wire.Encode(outgoing)); err != nil {
			t.Fatal(err)
		}
	}
	if released := read(socket); released.ID != p.MsgS2CHostingState || released.Request != 802 {
		t.Fatal("摇杆释放观察屏障没有按帧序返回")
	}
	releasedScene := &pb.SceneBaselineChunk{}
	if released := read(socket); released.ID != p.MsgS2CSceneBaseline || proto.Unmarshal(released.Payload, releasedScene) != nil || releasedScene.NavigationState.Status != navigationActive || releasedScene.NavigationState.Revision != uint64(1)<<63|19 {
		t.Fatal("真实WS释放摇杆错误取消原目标")
	}
	before := r.Player.Pos
	// 使用真实网络输入S2C消息号，生产worker的默认拒绝分支不得执行任何移动。
	malicious := wire.Message(p.MsgS2CEntityDisplacement, 7, connection.Binding(), &pb.EntityDisplacement{SceneEpoch: r.Scene, EntityHandle: ownerHandle, Kind: movementTeleport, Path: []*pb.GridPoint{{X: 16, Y: 16}}})
	if err := socket.Write(ctx, websocket.MessageBinary, wire.Encode(malicious)); err != nil {
		t.Fatal(err)
	}
	rejected := read(socket)
	if rejected.Request != 7 || rejected.Error != p.CodeInvalidArgument {
		t.Fatal("客户端伪造S2C位移未拒绝")
	}
	owner(func() {
		if r.Player.Pos != before {
			t.Error("伪造S2C位移修改现场")
		}
	})
	now := time.Unix(1800000000, 0)
	var first pushOutcome
	owner(func() {
		first = w.sourcePushed(r, reference(r, &r.Player), 2, 1, now, displacementSource{CastID: uint64(1)<<63 | 23})
	})
	eventFrame, baselineFrame := read(socket), read(socket)
	if first.Steps != 1 || eventFrame.ID != p.MsgS2CEntityDisplacement || baselineFrame.ID != p.MsgS2CSceneBaseline {
		t.Fatal("真实WS路径事件未先于最终基线")
	}
	event := &pb.EntityDisplacement{}
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
	if frame := read(socket); frame.ID != p.MsgS2CSceneBaseline {
		t.Fatal("NotReady仍向WS发送旧瞬时位移")
	}
	socket.CloseNow()
	owner(func() {
		w.command(command{role: "role", conn: connection, disconnect: true, at: now})
		w.sourcePushed(r, reference(r, &r.Player), 2, 1, now, displacementSource{})
	})
	next, nextConnection := dial()
	defer next.CloseNow()
	owner(func() { w.send(r, p.MsgS2CEnterLine, 0, w.enterLineResult(r)); w.baseline(r) })
	entryFrame = read(next)
	if entryFrame.ID != p.MsgS2CEnterLine || proto.Unmarshal(entryFrame.Payload, entry) != nil || entry.PlayerHandle != ownerHandle {
		t.Fatal("健康重入丢失显式本人句柄")
	}
	frame := read(next)
	if frame.ID != p.MsgS2CSceneBaseline || proto.Unmarshal(frame.Payload, scene) != nil || movementEntity(scene, ownerHandle).MovementRevision != uint64(1)<<53|20 || scene.NavigationState.Status != navigationActive || scene.NavigationState.Revision != uint64(1)<<63|19 || frame.Binding != nextConnection.Binding() {
		t.Fatal("健康WS重入丢失当前水位/目标或补播旧路径")
	}
	owner(func() {
		r.Ready = true
		old := reference(r, &r.Player)
		w.revivePlayer(r)
		if outcome := w.sourcePushed(r, old, 2, 1, now, displacementSource{}); outcome.Stop != pushStale {
			t.Error("旧life force作用新life")
		}
		w.snapshot(r)
	})
	frame = read(next)
	if frame.ID != p.MsgS2CSceneBaseline || proto.Unmarshal(frame.Payload, scene) != nil || movementEntity(scene, ownerHandle).Life != uint64(1)<<63|14 || movementEntity(scene, ownerHandle).MovementRevision != 0 || scene.NavigationState.Revision != uint64(1)<<63|19 {
		t.Fatal("新life首全量补播旧瞬时/丢弃同图导航")
	}
	for _, outgoing := range []wire.Frame{
		wire.Message(p.MsgC2SMoveIntent, 809, nextConnection.Binding(), &pb.MoveIntent{SceneEpoch: r.Scene, InputSequence: 1, ReleaseJoystick: true}),
		wire.Message(p.MsgC2SAutoBattle, 810, nextConnection.Binding(), &pb.AutoBattleIntent{SceneEpoch: r.Scene}),
	} {
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
	var beforeCancel entity
	owner(func() { beforeCancel = r.Player; r.Manual = true })
	for index, rejected := range []struct {
		binding uint64
		intent  *pb.MoveIntent
		code    uint32
	}{
		{nextConnection.Binding(), &pb.MoveIntent{SceneEpoch: r.Scene + 1, InputSequence: 2, CancelNavigation: true}, p.CodeInvalidArgument},
		{connection.Binding(), &pb.MoveIntent{SceneEpoch: r.Scene, InputSequence: 2, CancelNavigation: true}, p.CodeStaleSession},
		{nextConnection.Binding(), &pb.MoveIntent{SceneEpoch: r.Scene, InputSequence: 1, CancelNavigation: true}, p.CodeStaleInput},
		{nextConnection.Binding(), &pb.MoveIntent{SceneEpoch: r.Scene, InputSequence: 2, CancelNavigation: true, Destination: pointPB(goal)}, p.CodeInvalidArgument},
		{nextConnection.Binding(), &pb.MoveIntent{SceneEpoch: r.Scene, InputSequence: 2, CancelNavigation: true, ReleaseJoystick: true}, p.CodeInvalidArgument},
	} {
		request := uint32(811 + index)
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
