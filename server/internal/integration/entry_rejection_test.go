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
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
	"minimir/internal/wire"
)

func assertEntryRejectionFrame(t *testing.T, address, ticket string, code uint32) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	ws, _, err := websocket.Dial(ctx, "ws://"+address+"/ws", nil)
	must(t, err)
	defer ws.CloseNow()
	const request uint32 = 7
	must(t, ws.Write(ctx, websocket.MessageBinary, wire.Encode(wire.Message(p.MsgC2SEnterLine, request, 0, &pb.EnterLineRequest{AccountTicket: ticket}))))
	_, body, err := ws.Read(ctx)
	must(t, err)
	frame, err := wire.Decode(body)
	must(t, err)
	if frame.ID != p.MsgS2CEnterLine || frame.Request != request || frame.Error != code {
		t.Fatal("入场拒绝未发送20002原错误", frame.ID, frame.Request, frame.Error)
	}
	if _, _, err = ws.Read(ctx); err == nil || ctx.Err() != nil {
		t.Fatal("入场拒绝后连接未在期限内关闭")
	}
}

func TestAuthenticationReadErrorsAndTicketRejectionMongoWS(t *testing.T) {
	if os.Getenv("MINIMIR_MONGO_E2E") != "1" {
		t.Skip("显式开启独立Mongo验证认证拒绝与真实进线错误帧")
	}
	cfg, err := devconfig.Load("../../../conf/dev/server.json")
	must(t, err)
	cfg.Database = "minimir_dev_game_test_auth_" + domain.ID()
	cfg.AccountAddress, cfg.GameAddress = address(t), address(t)
	ctx, cancel := context.WithCancel(context.Background())
	store, err := persistence.Open(ctx, cfg)
	must(t, err)
	var nodes sync.WaitGroup
	t.Cleanup(func() {
		cancel()
		nodes.Wait()
		cleanup, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
		must(t, store.DB.Drop(cleanup))
		store.Close(cleanup)
	})
	const password = "test_123456"
	name := "auth_" + domain.ID()[:10]
	login, err := store.Login(ctx, name, password, true)
	must(t, err)
	auth, err := store.Authenticate(ctx, login.AccountToken)
	must(t, err)
	role, err := store.CreateRole(ctx, auth, &pb.CreateRoleRequest{OriginServerId: 1, Slot: 1, Name: "认证" + domain.ID()[:6], OpId: domain.Bytes(domain.ID())})
	must(t, err)
	assertCode := func(err error, want uint32) {
		t.Helper()
		if got := persistence.Code(err); got != want {
			t.Fatalf("认证错误=%d，期望%d", got, want)
		}
	}
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
	roleFilter := bson.M{"_id": domain.Hex(role.Key.RoleId)}
	roles := store.DB.Collection("roles")
	_, err = roles.UpdateOne(ctx, roleFilter, bson.M{"$set": bson.M{"binding": "错误结构"}})
	must(t, err)
	_, err = store.IssueTicket(ctx, auth, role.Key)
	assertCode(err, p.CodeLineForbidden)
	_, err = store.IssueTicket(ctx, persistence.Auth{AccountID: domain.ID()}, role.Key)
	assertCode(err, p.CodeInvalidArgument)
	_, err = roles.UpdateOne(ctx, roleFilter, bson.M{"$unset": bson.M{"binding": ""}})
	must(t, err)
	for _, mode := range []string{"account", "game"} {
		nodes.Go(func() {
			if err := bootstrap.Run(ctx, cfg, mode); err != nil && ctx.Err() == nil {
				t.Errorf("入场拒绝测试%s节点: %v", mode, err)
			}
		})
	}
	healthy(t, cfg.AccountAddress)
	healthy(t, cfg.GameAddress)
	assertEntryRejectionFrame(t, cfg.GameAddress, "invalid", p.CodeUnauthenticated)
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
