package transport

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/game"
	p "minimir/internal/protocol"
	"minimir/internal/wire"
)

func TestWebSocketInitialAdmissionTimeoutReleasesSlot(t *testing.T) {
	s := &Server{Engine: &game.Engine{}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.serveWebSocket(w, r, 600*time.Millisecond)
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	socket, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http"), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer socket.CloseNow()
	heartbeat := wire.Message(p.MsgC2SHeartbeat, 1, 0, &pb.Heartbeat{})
	if err = socket.Write(ctx, websocket.MessageBinary, wire.Encode(heartbeat)); err != nil {
		t.Fatal(err)
	}
	_, data, err := socket.Read(ctx)
	if err != nil {
		t.Fatal(err)
	}
	response, err := wire.Decode(data)
	if err != nil || response.ID != p.MsgS2CHeartbeat || response.Binding != 0 {
		t.Fatalf("未入场心跳回执不匹配：%+v, %v", response, err)
	}
	if _, _, err = socket.Read(ctx); err == nil || ctx.Err() != nil {
		t.Fatalf("未附着连接没有按服务端截止关闭：%v", err)
	}
	deadline := time.Now().Add(time.Second)
	for s.active.Load() != 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if active := s.active.Load(); active != 0 {
		t.Fatalf("超时连接仍占用 %d 个槽位", active)
	}
}
