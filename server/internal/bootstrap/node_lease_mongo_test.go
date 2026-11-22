// 本文件在专用 MongoDB 测试库对照续租、所有权换代和永久屏障行为。

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

// leaseTestDBPrefix 保存租约测试数据库前缀所使用的文本。
const leaseTestDBPrefix = "minimir_dev_game_test_lease_"

// 代理只接随机库游戏实例的续租连接，只丢弃由驱动匹配的一帧run_control成功响应。
type leaseReplyLoss struct {
	// armed 记录“armed”，使用“bool”表示。
	armed atomic.Bool
	// request 记录请求，使用整数“64”表示。
	request atomic.Int64
	// dropped 记录“dropped”，使用临时结构通道表示。
	dropped chan struct{}
}

// newLeaseReplyLoss 构造租约“reply”损失。
// t 提供断言失败报告、子用例和清理注册，cfg 记录服务配置。
func newLeaseReplyLoss(t *testing.T, cfg devconfig.Config) (*leaseReplyLoss, *mongo.Client) {
	t.Helper()
	if cfg.MongoURI != devconfig.DevelopmentMongoURI() || !strings.HasPrefix(cfg.Database, leaseTestDBPrefix) ||
		len(cfg.Database) != len(leaseTestDBPrefix)+32 {
		t.Fatal("续租代理只允许当前平台专属副本集和独立随机游戏库")
	}
	// endpoint 保存由“parse”取得的端点，err 接收“parse”过程中可能产生的错误，供后续失败分支处理。
	endpoint, err := url.Parse(cfg.MongoURI)
	if err != nil || endpoint.Hostname() != "127.0.0.1" {
		t.Fatal("续租代理上游必须是本项目回环Mongo", err)
	}
	// upstream 保存网络主机或托管，用于当前步骤中的“upstream”。
	upstream := endpoint.Host
	// listener 保存由监听取得的监听器。
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	// loss 保存当前处理所用的损失。
	loss := &leaseReplyLoss{dropped: make(chan struct{}, 1)}
	go func() {
		for {
			// conn 保存由“accept”取得的连接，err 接收“accept”过程中可能产生的错误，供后续失败分支处理。
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go loss.forward(conn, upstream)
		}
	}()
	// monitor 保存当前处理所用的“monitor”，command 记录命令，使用命令已起点或开始时间事件表示。
	monitor := &event.CommandMonitor{Started: func(_ context.Context, command *event.CommandStartedEvent) {
		if command.DatabaseName != cfg.Database || command.CommandName != "update" {
			return
		}
		// collection 保存由文本值成功标记取得的集合，ok 表明查询、校验或类型转换是否成功。
		collection, ok := command.Command.Lookup("update").StringValueOK()
		if ok && collection == "run_control" && loss.armed.CompareAndSwap(true, false) {
			loss.request.Store(command.RequestID)
		}
	}}
	endpoint.Host = listener.Addr().String()
	// client 保存由“connect”取得的客户端。
	client, err := mongo.Connect(options.Client().ApplyURI(endpoint.String()).SetMaxPoolSize(3).SetMonitor(monitor))
	if err != nil {
		t.Fatal(err)
	}
	return loss, client
}

// forward 处理向前。
// l 是调用该方法的租约“reply”损失实例，client 记录客户端，使用连接表示，upstream 记录“upstream”，使用文本表示。
func (l *leaseReplyLoss) forward(client net.Conn, upstream string) {
	defer client.Close()
	// mongoConn 保存由发起连接超时取得的MongoDB连接，err 接收发起连接超时过程中可能产生的错误，供后续失败分支处理。
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
		// header 记录头部，使用字节序列表示。
		var header [16]byte
		// err 接收读取完整或满额过程中可能产生的错误，供后续失败分支处理。
		if _, err := io.ReadFull(mongoConn, header[:]); err != nil {
			return
		}
		// size 保存由整数取得的大小。
		size := int(binary.LittleEndian.Uint32(header[:4]))
		if size < len(header) || size > 1<<20 {
			return
		}
		// body 创建用于维护消息正文的容器或通道。
		body := make([]byte, size-len(header))
		// err 接收读取完整或满额过程中可能产生的错误，供后续失败分支处理。
		if _, err := io.ReadFull(mongoConn, body); err != nil {
			return
		}
		// responseTo 保存由整数“64”取得的响应目标。
		responseTo := int64(int32(binary.LittleEndian.Uint32(header[8:12])))
		if responseTo != 0 && l.request.CompareAndSwap(responseTo, 0) {
			l.dropped <- struct{}{}
			continue
		}
		// err 接收写入过程中可能产生的错误，供后续失败分支处理。
		if _, err := client.Write(header[:]); err != nil {
			return
		}
		// err 接收写入过程中可能产生的错误，供后续失败分支处理。
		if _, err := client.Write(body); err != nil {
			return
		}
	}
}

// TestNodeLeaseRecoversRealMongoReplyLossWithoutChangingOwner 核验未确认回包应对应Mongo中真实已写入的续租，用诊断「续租响应未知时阻塞了仍持有围栏的正常永久结算」定位对应断言。
// t 提供断言失败报告、子用例和清理注册。
func TestNodeLeaseRecoversRealMongoReplyLossWithoutChangingOwner(t *testing.T) {
	if os.Getenv("MINIMIR_MONGO_E2E") != "1" {
		t.Skip("显式启用独立随机Mongo库的续租回包故障")
	}
	// cfg 保存由加载取得的服务配置，err 接收加载过程中可能产生的错误，供后续失败分支处理。
	cfg, err := devconfig.Load("../../../conf/dev/server.json")
	if err != nil {
		t.Fatal(err)
	}
	cfg.Database = leaseTestDBPrefix + domain.ID()
	// ctx 传递从调用方派生的上下文，供取消或控制截止时间，cancel 取消对应上下文或停止计时，释放其关联资源。
	ctx, cancel := context.WithTimeout(t.Context(), 25*time.Second)
	defer cancel()
	// must 封装“must”对应的后续处理，err 保存当前操作的失败原因，供调用方检查或返回。
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	// authority 保存由打开取得的权限围栏。
	authority, err := persistence.Open(ctx, cfg)
	must(err)
	t.Cleanup(func() {
		// cleanup 传递从调用方派生的上下文，供取消或控制截止时间，stop 取消对应上下文或停止计时，释放其关联资源。
		cleanup, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		defer authority.Close(cleanup)
		must(authority.DB.Drop(cleanup))
	})
	// game 保存由打开取得的游戏。
	game, err := persistence.Open(ctx, cfg)
	must(err)
	t.Cleanup(func() {
		// cleanup 传递从调用方派生的上下文，供取消或控制截止时间，stop 取消对应上下文或停止计时，释放其关联资源。
		cleanup, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		game.Close(cleanup)
	})
	must(authority.Singleton(ctx, "account"))
	// grant 保存由批准游戏取得的授予。
	grant, err := authority.ApproveGame(ctx, cfg.NodeID, game.Instance, cfg.RunID)
	must(err)
	game.Epoch = grant.Epoch
	// registered 保存由登录取得的“registered”。
	registered, err := authority.Login(ctx, "lease_"+domain.ID()[:8], "test_123456", true)
	must(err)
	// account 保存由“authenticate”取得的账号。
	account, err := authority.Authenticate(ctx, registered.AccountToken)
	must(err)
	// role 保存由创建角色取得的角色。
	role, err := authority.CreateRole(ctx, account, &pb.CreateRoleRequest{
		OriginServerId: cfg.OriginID, Slot: 1, Job: 0, Name: "续租" + domain.ID()[:8], OpId: domain.Bytes(domain.ID()),
	})
	must(err)
	// ticket 保存由签发入场票据取得的入场票据。
	ticket, err := authority.IssueTicket(ctx, account, role.Key)
	must(err)
	// before 保存由入场取得的操作前的状态，auth 保存由入场取得的授权。
	before, auth, err := game.Enter(ctx, ticket.Ticket)
	must(err)

	// loss 保存由创建租约“reply”损失取得的损失，control 保存由创建租约“reply”损失取得的控制。
	loss, control := newLeaseReplyLoss(t, cfg)
	must(game.Control.Disconnect(ctx))
	game.Control = control
	// renew 封装续租对应的后续处理，attempt 记录“attempt”，使用请求或事务上下文表示。
	renew := func(attempt context.Context) error { return game.Renew(attempt, "game") }
	// confirmed 保存由确认初始值节点租约取得的已确认。
	confirmed, err := confirmInitialNodeLease(ctx, renew)
	must(err)
	// readRun 封装读取运行批次对应的后续处理。
	readRun := func() persistence.Run {
		t.Helper()
		// run 记录运行批次。
		var run persistence.Run
		must(authority.DB.Collection("run_control").FindOne(ctx, bson.M{"_id": cfg.RunID}).Decode(&run))
		return run
	}
	// initial 保存由读取运行批次取得的初始值。
	initial := readRun()
	loss.armed.Store(true)
	// leaseCtx 传递从调用方派生的上下文，供取消或控制截止时间，stopLease 传递从调用方派生的上下文，供取消或控制截止时间。
	leaseCtx, stopLease := context.WithCancel(ctx)
	defer stopLease()
	// events 创建用于维护事件集合的容器或通道。
	events := make(chan nodeLeaseEvent, 4)
	// stopped 创建用于维护“stopped”的容器或通道。
	stopped := make(chan error, 1)
	go func() { stopped <- watchNodeLease(leaseCtx, confirmed, renew, events) }()
	select {
	case <-loss.dropped:
	// err 保存当前操作的失败原因，供调用方检查或返回。
	case err := <-stopped:
		t.Fatal("故障注入前退出", err)
	case <-ctx.Done():
		t.Fatal("未截获独立续租回包", ctx.Err())
	}
	// unknown 保存由读取运行批次取得的未知。
	unknown := readRun()
	if !unknown.LeaseUntil.After(initial.LeaseUntil) {
		t.Fatal("未确认回包没有对应Mongo中真实已写入的续租")
	}
	// request 初始化用作请求的结算。
	request := persistence.Settlement{SourceSeq: before.SourceSeq + 1, Gold: 30, Bound: true}
	// during 保存由结算取得的“during”。
	during, err := game.Settle(ctx, auth, request)
	must(err)
	if during.SourceSeq != request.SourceSeq || during.Wallet.BoundGold != before.Wallet.BoundGold+30 {
		t.Fatal("续租响应未知时阻塞了仍持有围栏的正常永久结算")
	}
	// pickEvent 封装“pick”事件对应的后续处理。
	pickEvent := func() nodeLeaseEvent {
		t.Helper()
		select {
		// event 接收事件集合通道送来的事件。
		case event := <-events:
			return event
		// err 保存当前操作的失败原因，供调用方检查或返回。
		case err := <-stopped:
			t.Fatal("单次续租读超时导致节点退出", err)
		case <-ctx.Done():
			t.Fatal("未收到续租事件", ctx.Err())
		}
		return nodeLeaseEvent{}
	}
	// first 保存由“pick”事件取得的第一项，recovered 保存由“pick”事件取得的“recovered”。
	first, recovered := pickEvent(), pickEvent()
	if first.err == nil || !mongo.IsTimeout(first.err) || recovered.err != nil ||
		recovered.elapsed < nodeLeaseTimeout || recovered.elapsed >= nodeLeaseWindow-nodeLeaseInterval {
		t.Fatalf("真实驱动读超时未在已确认窗口内恢复：%+v，%+v", first, recovered)
	}
	// after 保存由读取运行批次取得的操作后的状态。
	after := readRun()
	if after.Owner != initial.Owner || after.Epoch != initial.Epoch || after.ControlVersion != initial.ControlVersion ||
		after.State != "active" || !after.LeaseUntil.After(unknown.LeaseUntil) {
		t.Fatal("续租恢复丢失原owner/epoch/控制水位", initial, unknown, after)
	}
	// replay 保存由结算取得的重放。
	replay, err := game.Settle(ctx, auth, request)
	must(err)
	if replay.SourceSeq != during.SourceSeq || replay.Wallet != during.Wallet || replay.Revision != during.Revision {
		t.Fatal("续租恢复重复触发已确认的原结算", during, replay)
	}
	stopLease()
	select {
	// err 保存当前操作的失败原因，供调用方检查或返回。
	case err := <-stopped:
		must(err)
	case <-ctx.Done():
		t.Fatal("停止后仍在续租", ctx.Err())
	}
	must(game.Release(ctx, "game"))
	// next 保存由批准游戏取得的下一状态。
	next, err := authority.ApproveGame(ctx, cfg.NodeID, domain.ID(), cfg.RunID)
	must(err)
	// lost 保存由续租取得的“lost”。
	lost := renew(ctx)
	if next.Epoch != grant.Epoch+1 || persistence.Code(lost) != p.CodeRouteChanged || errors.Is(lost, context.DeadlineExceeded) ||
		readRun().Owner != next.Owner {
		t.Fatal("旧实例的续租越过下一owner围栏", next, lost)
	}
	t.Logf("真实Mongo回包丢失触发读超时，约%dms恢复，同owner/epoch持续永久写入，旧epoch继续被拒绝", recovered.elapsed.Milliseconds())
}
