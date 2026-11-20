package bootstrap

import (
	"context"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"net/url"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/event"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
)

const leaseTestDBPrefix = "minimir_dev_game_test_lease_"

// 代理只接随机库游戏实例的续租连接，只丢弃由驱动匹配的一帧run_control成功响应。
type leaseReplyLoss struct {
	armed   atomic.Bool
	request atomic.Int64
	dropped chan struct{}
}

func newLeaseReplyLoss(t *testing.T, cfg devconfig.Config) (*leaseReplyLoss, *mongo.Client) {
	t.Helper()
	if cfg.MongoURI != devconfig.DevelopmentMongoURI() || !strings.HasPrefix(cfg.Database, leaseTestDBPrefix) ||
		len(cfg.Database) != len(leaseTestDBPrefix)+32 {
		t.Fatal("续租代理只允许当前平台专属副本集和独立随机游戏库")
	}
	endpoint, err := url.Parse(cfg.MongoURI)
	if err != nil || endpoint.Hostname() != "127.0.0.1" {
		t.Fatal("续租代理上游必须是本项目回环Mongo", err)
	}
	upstream := endpoint.Host
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	loss := &leaseReplyLoss{dropped: make(chan struct{}, 1)}
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go loss.forward(conn, upstream)
		}
	}()
	monitor := &event.CommandMonitor{Started: func(_ context.Context, command *event.CommandStartedEvent) {
		if command.DatabaseName != cfg.Database || command.CommandName != "update" {
			return
		}
		collection, ok := command.Command.Lookup("update").StringValueOK()
		if ok && collection == "run_control" && loss.armed.CompareAndSwap(true, false) {
			loss.request.Store(command.RequestID)
		}
	}}
	endpoint.Host = listener.Addr().String()
	client, err := mongo.Connect(options.Client().ApplyURI(endpoint.String()).SetMaxPoolSize(3).SetMonitor(monitor))
	if err != nil {
		t.Fatal(err)
	}
	return loss, client
}

func (l *leaseReplyLoss) forward(client net.Conn, upstream string) {
	defer client.Close()
	mongoConn, err := net.DialTimeout("tcp4", upstream, time.Second)
	if err != nil {
		return
	}
	defer mongoConn.Close()
	go func() {
		_, _ = io.Copy(mongoConn, client)
		_ = mongoConn.Close()
	}()
	for {
		var header [16]byte
		if _, err := io.ReadFull(mongoConn, header[:]); err != nil {
			return
		}
		size := int(binary.LittleEndian.Uint32(header[:4]))
		if size < len(header) || size > 1<<20 {
			return
		}
		body := make([]byte, size-len(header))
		if _, err := io.ReadFull(mongoConn, body); err != nil {
			return
		}
		responseTo := int64(int32(binary.LittleEndian.Uint32(header[8:12])))
		if responseTo != 0 && l.request.CompareAndSwap(responseTo, 0) {
			l.dropped <- struct{}{}
			continue
		}
		if _, err := client.Write(header[:]); err != nil {
			return
		}
		if _, err := client.Write(body); err != nil {
			return
		}
	}
}

func TestNodeLeaseRecoversRealMongoReplyLossWithoutChangingOwner(t *testing.T) {
	if os.Getenv("MINIMIR_MONGO_E2E") != "1" {
		t.Skip("显式启用独立随机Mongo库的续租回包故障")
	}
	cfg, err := devconfig.Load("../../../conf/dev/server.json")
	if err != nil {
		t.Fatal(err)
	}
	cfg.Database = leaseTestDBPrefix + domain.ID()
	ctx, cancel := context.WithTimeout(t.Context(), 25*time.Second)
	defer cancel()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	authority, err := persistence.Open(ctx, cfg)
	must(err)
	t.Cleanup(func() {
		cleanup, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		defer authority.Close(cleanup)
		must(authority.DB.Drop(cleanup))
	})
	game, err := persistence.Open(ctx, cfg)
	must(err)
	t.Cleanup(func() {
		cleanup, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		game.Close(cleanup)
	})
	must(authority.Singleton(ctx, "account"))
	grant, err := authority.ApproveGame(ctx, cfg.NodeID, game.Instance, cfg.RunID)
	must(err)
	game.Epoch = grant.Epoch
	registered, err := authority.Login(ctx, "lease_"+domain.ID()[:8], "test_123456", true)
	must(err)
	account, err := authority.Authenticate(ctx, registered.AccountToken)
	must(err)
	role, err := authority.CreateRole(ctx, account, &pb.CreateRoleRequest{
		OriginServerId: cfg.OriginID, Slot: 1, Job: 0, Name: "续租" + domain.ID()[:8], OpId: domain.Bytes(domain.ID()),
	})
	must(err)
	ticket, err := authority.IssueTicket(ctx, account, role.Key)
	must(err)
	before, auth, err := game.Enter(ctx, ticket.Ticket)
	must(err)

	loss, control := newLeaseReplyLoss(t, cfg)
	must(game.Control.Disconnect(ctx))
	game.Control = control
	renew := func(attempt context.Context) error { return game.Renew(attempt, "game") }
	confirmed, err := confirmInitialNodeLease(ctx, renew)
	must(err)
	readRun := func() persistence.Run {
		t.Helper()
		var run persistence.Run
		must(authority.DB.Collection("run_control").FindOne(ctx, bson.M{"_id": cfg.RunID}).Decode(&run))
		return run
	}
	initial := readRun()
	loss.armed.Store(true)
	leaseCtx, stopLease := context.WithCancel(ctx)
	defer stopLease()
	events := make(chan nodeLeaseEvent, 4)
	stopped := make(chan error, 1)
	go func() { stopped <- watchNodeLease(leaseCtx, confirmed, renew, events) }()
	select {
	case <-loss.dropped:
	case err := <-stopped:
		t.Fatal("故障注入前退出", err)
	case <-ctx.Done():
		t.Fatal("未截获独立续租回包", ctx.Err())
	}
	unknown := readRun()
	if !unknown.LeaseUntil.After(initial.LeaseUntil) {
		t.Fatal("未确认回包没有对应Mongo中真实已写入的续租")
	}
	request := persistence.Settlement{SourceSeq: before.SourceSeq + 1, Gold: 30, Bound: true}
	during, err := game.Settle(ctx, auth, request)
	must(err)
	if during.SourceSeq != request.SourceSeq || during.Wallet.BoundGold != before.Wallet.BoundGold+30 {
		t.Fatal("续租响应未知时阻塞了仍持有围栏的正常永久结算")
	}
	pickEvent := func() nodeLeaseEvent {
		t.Helper()
		select {
		case event := <-events:
			return event
		case err := <-stopped:
			t.Fatal("单次续租读超时导致节点退出", err)
		case <-ctx.Done():
			t.Fatal("未收到续租事件", ctx.Err())
		}
		return nodeLeaseEvent{}
	}
	first, recovered := pickEvent(), pickEvent()
	if first.err == nil || !mongo.IsTimeout(first.err) || recovered.err != nil ||
		recovered.elapsed < nodeLeaseTimeout || recovered.elapsed >= nodeLeaseWindow-nodeLeaseInterval {
		t.Fatalf("真实驱动读超时未在已确认窗口内恢复：%+v，%+v", first, recovered)
	}
	after := readRun()
	if after.Owner != initial.Owner || after.Epoch != initial.Epoch || after.ControlVersion != initial.ControlVersion ||
		after.State != "active" || !after.LeaseUntil.After(unknown.LeaseUntil) {
		t.Fatal("续租恢复丢失原owner/epoch/控制水位", initial, unknown, after)
	}
	replay, err := game.Settle(ctx, auth, request)
	must(err)
	if replay.SourceSeq != during.SourceSeq || replay.Wallet != during.Wallet || replay.Revision != during.Revision {
		t.Fatal("续租恢复重复触发已确认的原结算", during, replay)
	}
	stopLease()
	select {
	case err := <-stopped:
		must(err)
	case <-ctx.Done():
		t.Fatal("停止后仍在续租", ctx.Err())
	}
	must(game.Release(ctx, "game"))
	next, err := authority.ApproveGame(ctx, cfg.NodeID, domain.ID(), cfg.RunID)
	must(err)
	lost := renew(ctx)
	if next.Epoch != grant.Epoch+1 || persistence.Code(lost) != p.CodeRouteChanged || errors.Is(lost, context.DeadlineExceeded) ||
		readRun().Owner != next.Owner {
		t.Fatal("旧实例的续租越过下一owner围栏", next, lost)
	}
	t.Logf("真实Mongo回包丢失触发读超时，约%dms恢复，同owner/epoch持续永久写入，旧epoch继续被拒绝", recovered.elapsed.Milliseconds())
}
