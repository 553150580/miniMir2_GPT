// 本文件验证真实连接的入场超时、终止回执与连接收发边界。

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

// TestWebSocketInitialAdmissionTimeoutReleasesSlot 验证未入场连接仍可收到无绑定的心跳回执，到达初始入场截止后关闭并释放连接槽位。
// t 提供断言失败报告、子用例和清理注册。
func TestWebSocketInitialAdmissionTimeoutReleasesSlot(t *testing.T) {
	// s 保存当前处理所用的服务端。
	s := &Server{Engine: &game.Engine{}}
	// server 保存由创建服务端取得的服务端，w 记录响应写入器，r 记录请求。
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.serveWebSocket(w, r, 600*time.Millisecond)
	}))
	defer server.Close()
	// ctx 传递从调用方派生的上下文，供取消或控制截止时间，cancel 取消对应上下文或停止计时，释放其关联资源。
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	// socket 保存由发起连接取得的“socket”，err 接收发起连接过程中可能产生的错误，供后续失败分支处理。
	socket, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http"), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer socket.CloseNow()
	// heartbeat 保存由消息取得的“heartbeat”。
	heartbeat := wire.Message(p.MsgC2SHeartbeat, 1, 0, &pb.Heartbeat{})
	if err = socket.Write(ctx, websocket.MessageBinary, wire.Encode(heartbeat)); err != nil {
		t.Fatal(err)
	}
	// data 保存由读取取得的数据。
	_, data, err := socket.Read(ctx)
	if err != nil {
		t.Fatal(err)
	}
	// response 保存由解码取得的响应。
	response, err := wire.Decode(data)
	if err != nil || response.ID != p.MsgS2CHeartbeat || response.Binding != 0 {
		t.Fatalf("未入场心跳回执不匹配：%+v, %v", response, err)
	}
	if _, _, err = socket.Read(ctx); err == nil || ctx.Err() != nil {
		t.Fatalf("未附着连接没有按服务端截止关闭：%v", err)
	}
	// deadline 保存由新增取得的截止时间。
	deadline := time.Now().Add(time.Second)
	for s.active.Load() != 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	// active 保存由加载取得的“active”。
	if active := s.active.Load(); active != 0 {
		t.Fatalf("超时连接仍占用 %d 个槽位", active)
	}
}
