// 区分认证读失败、账号凭据失效、票据拒绝与关闭前终态消息，并核查本次请求对应的错误码。

package integration

import (
	"context"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
	"go.mongodb.org/mongo-driver/v2/bson"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/bootstrap"
	"minimir/internal/domain"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
	"minimir/internal/wire"
)

// assertEntryRejectionFrame 请求一次无效入场，验证服务端先回对应错误帧再关闭连接。
// t 提供断言失败报告、子用例和清理注册，address 记录地址，使用文本表示，ticket 记录入场票据，使用文本表示，code 记录结果码，使用数值表示。
func assertEntryRejectionFrame(t *testing.T, address, ticket string, code uint32) {
	t.Helper()
	// ctx 传递从调用方派生的上下文，供取消或控制截止时间，cancel 取消对应上下文或停止计时，释放其关联资源。
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	// ws 保存由发起连接取得的WebSocket，err 接收发起连接过程中可能产生的错误，供后续失败分支处理。
	ws, _, err := websocket.Dial(ctx, "ws://"+address+"/ws", nil)
	must(t, err)
	defer ws.CloseNow()
	// request 关联客户端请求与服务端的对应响应。
	const request uint32 = 7
	must(t, ws.Write(ctx, websocket.MessageBinary, wire.Encode(wire.Message(p.MsgC2SEnterLine, request, 0, &pb.EnterLineRequest{AccountTicket: ticket}))))
	// body 保存由读取取得的消息正文。
	_, body, err := ws.Read(ctx)
	must(t, err)
	// frame 保存由解码取得的协议帧。
	frame, err := wire.Decode(body)
	must(t, err)
	if frame.ID != p.MsgS2CEnterLine || frame.Request != request || frame.Error != code {
		t.Fatal("入场拒绝未发送20002原错误", frame.ID, frame.Request, frame.Error)
	}
	if _, _, err = ws.Read(ctx); err == nil || ctx.Err() != nil {
		t.Fatal("入场拒绝后连接未在期限内关闭")
	}
}

// TestAuthenticationReadErrorsAndTicketRejectionMongoWS 让账号读失败或票据失效，核对实际 HTTP 和 WebSocket 错误与连接关闭顺序。
// t 提供断言失败报告、子用例和清理注册。
func TestAuthenticationReadErrorsAndTicketRejectionMongoWS(t *testing.T) {
	if os.Getenv("MINIMIR_MONGO_E2E") != "1" {
		t.Skip("显式开启独立Mongo验证认证拒绝与真实进线错误帧")
	}
	// cfg 保存由加载取得的服务配置，err 接收加载过程中可能产生的错误，供后续失败分支处理。
	cfg, err := loadIntegrationConfig()
	must(t, err)
	cfg.Database = "minimir_dev_game_test_auth_" + domain.ID()
	cfg.AccountAddress, cfg.GameAddress = address(t), address(t)
	// ctx 传递从调用方派生的上下文，供取消或控制截止时间，cancel 取消对应上下文或停止计时，释放其关联资源。
	ctx, cancel := context.WithCancel(context.Background())
	// store 保存由打开取得的持久层存储。
	store, err := persistence.Open(ctx, cfg)
	must(t, err)
	// nodes 记录节点集合，使用协程汇合计数器表示。
	var nodes sync.WaitGroup
	t.Cleanup(func() {
		cancel()
		nodes.Wait()
		// cleanup 传递从调用方派生的上下文，供取消或控制截止时间，stop 取消对应上下文或停止计时，释放其关联资源。
		cleanup, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
		must(t, store.DB.Drop(cleanup))
		store.Close(cleanup)
	})
	// password 保存密码所使用的文本。
	const password = "test_123456"
	// name 保存根据当前数据与标识中的范围计算的名称。
	name := "auth_" + domain.ID()[:10]
	// login 接收登录供后续步骤使用。
	login, err := store.Login(ctx, name, password, true)
	must(t, err)
	// auth 保存由“authenticate”取得的授权。
	auth, err := store.Authenticate(ctx, login.AccountToken)
	must(t, err)
	// role 保存由创建角色取得的角色。
	role, err := store.CreateRole(ctx, auth, &pb.CreateRoleRequest{OriginServerId: 1, Slot: 1, Name: "认证" + domain.ID()[:6], OpId: domain.Bytes(domain.ID())})
	must(t, err)
	// assertCode 封装“assert”结果码对应的后续处理，err 保存当前操作的失败原因，供调用方检查或返回，want 记录期望值，使用数值表示。
	assertCode := func(err error, want uint32) {
		t.Helper()
		// got 保存由结果码取得的实际结果。
		if got := persistence.Code(err); got != want {
			t.Fatalf("认证错误=%d，期望%d", got, want)
		}
	}
	// interrupted 传递从调用方派生的上下文，供取消或控制截止时间，stop 取消对应上下文或停止计时，释放其关联资源。
	interrupted, stop := context.WithCancel(ctx)
	stop()
	_, err = store.Login(interrupted, name, password, false)
	assertCode(err, p.CodeServerBusy)
	_, err = store.Login(interrupted, name, password, true)
	assertCode(err, p.CodeServerBusy)
	_, err = store.Authenticate(interrupted, login.AccountToken)
	assertCode(err, p.CodeServerBusy)
	_, err = store.TicketRole(interrupted, strings.Repeat("c", 64))
	assertCode(err, p.CodeServerBusy)
	_, err = store.IssueTicket(interrupted, auth, role.Key)
	assertCode(err, p.CodeServerBusy)
	_, err = store.Login(ctx, name, "wrong_123456", false)
	assertCode(err, p.CodeUnauthenticated)
	_, err = store.Authenticate(ctx, strings.Repeat("c", 64))
	assertCode(err, p.CodeUnauthenticated)
	_, err = store.IssueTicket(ctx, auth, &pb.RoleKey{OriginServerId: 1, RoleId: domain.Bytes(domain.ID())})
	assertCode(err, p.CodeInvalidArgument)
	_, err = store.IssueTicket(ctx, auth, role.Key)
	assertCode(err, p.CodeLineRecovering)
	// roleFilter 初始化用作角色筛选条件的MongoDB BSON 文档。
	roleFilter := bson.M{"_id": domain.Hex(role.Key.RoleId)}
	// roles 保存由集合取得的角色集合。
	roles := store.DB.Collection("roles")
	_, err = roles.UpdateOne(ctx, roleFilter, bson.M{"$set": bson.M{"binding": "错误结构"}})
	must(t, err)
	_, err = store.IssueTicket(ctx, auth, role.Key)
	assertCode(err, p.CodeLineForbidden)
	_, err = store.IssueTicket(ctx, persistence.Auth{AccountID: domain.ID()}, role.Key)
	assertCode(err, p.CodeInvalidArgument)
	_, err = roles.UpdateOne(ctx, roleFilter, bson.M{"$unset": bson.M{"binding": ""}})
	must(t, err)
	// mode 接收遍历文本列表时当前的元素。
	for _, mode := range []string{"account", "game"} {
		nodes.Go(func() {
			// err 接收运行批次过程中可能产生的错误，供后续失败分支处理。
			if err := bootstrap.Run(ctx, cfg, mode); err != nil && ctx.Err() == nil {
				t.Errorf("入场拒绝测试%s节点: %v", mode, err)
			}
		})
	}
	healthy(t, cfg.AccountAddress)
	healthy(t, cfg.GameAddress)
	assertEntryRejectionFrame(t, cfg.GameAddress, "invalid", p.CodeUnauthenticated)
	// ticket 保存由签发入场票据取得的入场票据。
	ticket, err := store.IssueTicket(ctx, auth, role.Key)
	must(t, err)
	join(t, cfg.GameAddress, ticket.Ticket).close()
	assertEntryRejectionFrame(t, cfg.GameAddress, ticket.Ticket, p.CodeUnauthenticated)
	_, err = store.DB.Collection("accounts").UpdateOne(ctx, bson.M{"_id": auth.AccountID}, bson.M{"$set": bson.M{"active": false}})
	must(t, err)
	_, err = store.Authenticate(ctx, login.AccountToken)
	assertCode(err, p.CodeUnauthenticated)
	_, err = store.Login(ctx, name, password, false)
	assertCode(err, p.CodeUnauthenticated)
}
