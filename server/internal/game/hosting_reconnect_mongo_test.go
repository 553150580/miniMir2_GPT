// 本文件使用隔离 MongoDB 测试库验证健康重连保留亚秒余量和提交确认丢失后的幂等结算。
package game

import (
	"context"
	"os"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
	"minimir/internal/wire"
)

// hostingMongoFixture 保存离线托管MongoDB开发测试配置所需的状态。
type hostingMongoFixture struct {
	// ctx 向下传递当前请求的截止时间和取消信号。
	ctx context.Context
	// account 记录账号，使用持久层存储表示。
	account *persistence.Store
	// owner 记录所有者，使用持久层存储表示。
	owner *persistence.Store
	// accountAuth 记录账号授权。
	accountAuth persistence.Auth
	// roleKey 记录角色键。
	roleKey *pb.RoleKey
	// role 记录角色。
	role domain.Role
	// roleAuth 记录角色授权。
	roleAuth persistence.Auth
}

// newHostingMongoFixture 构造离线托管MongoDB开发测试配置。
// t 提供断言失败报告、子用例和清理注册，cfg 记录服务配置。
func newHostingMongoFixture(t *testing.T, cfg devconfig.Config) hostingMongoFixture {
	t.Helper()
	if os.Getenv("MINIMIR_MONGO_E2E") != "1" {
		t.Skip("显式启用本任务副本集中的独立托管计量测试库")
	}
	cfg.Database = "minimir_dev_game_test_host_" + domain.ID()
	// ctx 传递从调用方派生的上下文，供取消或控制截止时间，cancel 取消对应上下文或停止计时，释放其关联资源。
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	t.Cleanup(cancel)
	// account 保存由打开取得的账号，err 接收打开过程中可能产生的错误，供后续失败分支处理。
	account, err := persistence.Open(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	// owner 记录所有者，使用持久层存储表示。
	var owner *persistence.Store
	t.Cleanup(func() {
		// cleanup 传递从调用方派生的上下文，供取消或控制截止时间，stop 取消对应上下文或停止计时，释放其关联资源。
		cleanup, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
		// err 接收掉落过程中可能产生的错误，供后续失败分支处理。
		if err := account.DB.Drop(cleanup); err != nil {
			t.Error(err)
		}
		if owner != nil {
			owner.Close(cleanup)
		}
		account.Close(cleanup)
	})
	owner, err = persistence.Open(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err = account.Singleton(ctx, "account"); err != nil {
		t.Fatal(err)
	}
	// grant 保存由批准游戏取得的授予。
	grant, err := account.ApproveGame(ctx, cfg.NodeID, owner.Instance, cfg.RunID)
	if err != nil {
		t.Fatal(err)
	}
	owner.Epoch = grant.Epoch
	// login 接收登录供后续步骤使用。
	login, err := account.Login(ctx, "frac_"+domain.ID()[:10], "test_123456", true)
	if err != nil {
		t.Fatal(err)
	}
	// auth 保存由“authenticate”取得的授权。
	auth, err := account.Authenticate(ctx, login.AccountToken)
	if err != nil {
		t.Fatal(err)
	}
	// key 保存由创建角色取得的键。
	key, err := account.CreateRole(ctx, auth, &pb.CreateRoleRequest{OriginServerId: cfg.OriginID, Slot: 1, Job: 0, Name: "余时回归" + domain.ID()[:5], OpId: domain.Bytes(domain.ID())})
	if err != nil {
		t.Fatal(err)
	}
	// id 区分当前步骤指向的目标对象。
	id := domain.Hex(key.Key.RoleId)
	if _, err = account.DB.Collection("roles").UpdateOne(ctx, bson.M{"_id": id}, bson.M{"$set": bson.M{"hosting_seconds": int64(5)}}); err != nil {
		t.Fatal(err)
	}
	// ticket 保存由签发入场票据取得的入场票据。
	ticket, err := account.IssueTicket(ctx, auth, key.Key)
	if err != nil {
		t.Fatal(err)
	}
	// role 保存由入场取得的角色，roleAuth 保存由入场取得的角色授权。
	role, roleAuth, err := owner.Enter(ctx, ticket.Ticket)
	if err != nil {
		t.Fatal(err)
	}
	if role.HostingSeconds != 5 {
		t.Fatal("独立测试托管余额前态未生效")
	}
	return hostingMongoFixture{ctx: ctx, account: account, owner: owner, accountAuth: auth, roleKey: key.Key, role: role, roleAuth: roleAuth}
}

// 独立随机库仅提供正常签票、健康Enter和真实计费；固定非致死战斗不代表自然成长速度。
// t 提供断言失败报告、子用例和清理注册。
func TestHostingFractionAcrossHealthyReconnectMongo(t *testing.T) {
	// w 接收完整或满额背包策略房间执行器供后续步骤使用，r 接收完整或满额背包策略房间执行器供后续步骤使用。
	w, r, _ := fullBagPolicyWorker(t)
	// fixture 保存由创建离线托管MongoDB开发测试配置取得的开发测试配置。
	fixture := newHostingMongoFixture(t, w.e.Store.Cfg)
	// ctx 保存当前处理所用的上下文，account 保存当前处理所用的账号，owner 保存当前处理所用的所有者。
	ctx, account, owner := fixture.ctx, fixture.account, fixture.owner
	// role 保存当前处理所用的角色，roleAuth 保存当前处理所用的角色授权。
	role, roleAuth := fixture.role, fixture.roleAuth
	// id 区分当前步骤指向的目标对象。
	id := role.ID
	w.e.Store, w.e.ctx = owner, ctx
	r.Role, r.Auth = role, roleAuth
	r.Conn, r.Ready = &travelConnection{alive: true, binding: uint64(roleAuth.SessionEpoch)}, true
	w.rooms = map[string]*room{id: r}
	// monster 保存从怪物集合取出的怪物。
	monster := r.Monsters[0]
	monster.HP, monster.MaxHP = 1000, 1000
	// hpBefore 保存生命值，用于当前步骤中的生命值操作前的状态。
	hpBefore := monster.HP
	// simulatedMS 记录“simulated”毫秒，使用数值表示，committedSeconds 记录已提交秒集合，使用数值表示。
	var simulatedMS, committedSeconds int64
	// now 保存由新增取得的当前时间。
	now := time.Now().Add(5 * time.Second)
	// segment 规定分段的初始取值。
	for segment := 0; segment < 3; segment++ {
		// 走正常断线分支；八个100ms非致死离线tick，健康接管冻结整秒并沿原SourceSeq提交。
		w.command(command{role: id, conn: r.Conn, disconnect: true, at: now})
		// session 保存托管会话标识，用于当前步骤中的会话。
		session := r.HostSession
		if session == "" {
			t.Fatal("个人离线未建立当前托管会话")
		}
		r.LastTick = now
		// tick 规定模拟帧的初始取值。
		for tick := 0; tick < 8; tick++ {
			now = now.Add(100 * time.Millisecond)
			w.step(r, now)
			if r.Pending || len(w.e.jobs) != 0 {
				t.Fatal("未到检查点且无击杀，不能提前提交")
			}
		}
		simulatedMS += 800
		// fraction 保存根据“simulated”毫秒与当前数据计算的“fraction”。
		fraction := simulatedMS % 1000
		if r.HostMS != fraction || (5-r.Role.HostingSeconds+r.hostSecondsOpen)*1000+r.HostMS != simulatedMS {
			t.Fatalf("累计模拟和计量不等: simulated=%d balance=%d fraction=%d", simulatedMS, r.Role.HostingSeconds, r.HostMS)
		}
		// ticket 保存由签发入场票据取得的入场票据，err 接收签发入场票据过程中可能产生的错误，供后续失败分支处理。
		ticket, err := account.IssueTicket(ctx, fixture.accountAuth, fixture.roleKey)
		if err != nil {
			t.Fatal(err)
		}
		verified, err := owner.TicketRole(ctx, ticket.Ticket)
		if err != nil {
			t.Fatal(err)
		}
		// next 保存当前处理所用的下一状态。
		next := &travelConnection{alive: true}
		// debit 保存已累计且尚未归入流水的托管整秒，用于当前步骤中的扣减，oldBinding 保存授权，用于当前步骤中的原状态会话绑定。
		debit, oldBinding := r.hostSecondsOpen, r.Auth
		w.command(command{role: id, conn: next, ticket: &verified, frame: wire.Frame{Request: uint32(segment + 1)}})
		if debit != 0 {
			if len(r.Waiting) != 1 || len(w.e.jobs) != 1 || r.Auth != oldBinding || r.Conn != nil || !r.Pending {
				t.Fatal("未确认的旧会话时长需阻塞接管")
			}
			// j 接收任务或职业编号集合通道送来的当前位置的下标。
			j := <-w.e.jobs
			j.run()(w)
			if r.Isolated || r.Role.Report.SessionID != session || r.Role.Report.UsedSeconds != debit || r.Auth != oldBinding ||
				r.Role.HostingSeconds != 5-committedSeconds-debit || r.hostSecondsOpen != 0 {
				t.Fatal("计费需在新票前确认，且保持原时长来源和区间")
			}
			committedSeconds += debit
		}
		if len(w.e.jobs) != 1 {
			t.Fatal("结清后应继续处理原入场")
		}
		// j 接收任务或职业编号集合通道送来的当前位置的下标。
		j := <-w.e.jobs
		j.run()(w)
		if r.Pending || r.Conn != next || r.Isolated || r.HostMS != fraction {
			t.Fatal("真实健康接管丢失计量或未完成")
		}
		w.command(command{role: id, conn: next, frame: wire.Message(p.MsgC2SEnterWorldAck, 10, next.Binding(), &pb.EnterWorldAck{SceneEpoch: r.Scene, BaselineId: r.Baseline})})
		if !r.Ready || r.HostSession != "" {
			t.Fatal("正常匹配ACK未就绪，或在线仍附着旧托管会话")
		}
		// 在线真实tick仍运行战斗，但不增加托管计量。
		now = now.Add(100 * time.Millisecond)
		r.LastTick = now.Add(-100 * time.Millisecond)
		w.step(r, now)
		if r.HostMS != fraction || r.Pending {
			t.Fatal("健康上线误计费")
		}
		t.Logf("第%d段: 实际离线%dms，余额%d秒，余量%dms，会话代次%d", segment+1, simulatedMS, r.Role.HostingSeconds, r.HostMS, r.Auth.SessionEpoch)
		now = now.Add(time.Second)
	}
	if monster.HP >= hpBefore || r.Player.AttackSequence == 0 || committedSeconds != 2 || r.Role.HostingSeconds != 3 || r.HostMS != 400 {
		t.Fatalf("真实AI或累计扣时错误: hp=%d attacks=%d committed=%d balance=%d fraction=%d", monster.HP, r.Player.AttackSequence, committedSeconds, r.Role.HostingSeconds, r.HostMS)
	}
	// persisted 保存由角色取得的“persisted”，err 接收角色过程中可能产生的错误，供后续失败分支处理。
	persisted, err := owner.Role(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.HostingSeconds != 3 || persisted.SourceSeq != 2 {
		t.Fatal("内存计费与原永久来源水位不一致")
	}

	// t 提供断言失败报告、子用例和清理注册。
	t.Run("击杀和扣时共用原来源重试", func(t *testing.T) {
		w.command(command{role: id, conn: r.Conn, disconnect: true, at: now})
		r.LastTick = now
		// tick 规定模拟帧的初始取值。
		for tick := 0; tick < 6; tick++ {
			now = now.Add(100 * time.Millisecond)
			w.step(r, now)
		}
		// session 保存托管会话标识，用于当前步骤中的会话。
		session := r.HostSession
		if r.Pending || r.hostSecondsOpen != 1 || r.HostMS != 0 {
			t.Fatal("击杀前应累计一整秒且没有独立写入")
		}
		r.KillFacts = append(r.KillFacts, killFact{ID: domain.ID(), World: r.currentWorld().Ref, At: now, HostSession: session, Monster: *monster.Monster})
		// q 接收击杀结算供后续步骤使用。
		q := w.killSettlement(r, r.KillFacts[0])
		q.HostSeconds = 1
		w.flushKillFacts(r)
		assertHostingJob(t, w, r, q)
		// binding 初始化用作会话绑定的结算会话绑定。
		binding := settlementBinding{room: r, auth: r.Auth, world: r.currentWorld().Ref, generation: r.SettlementGeneration, id: r.PendingOperationID, fingerprint: r.PendingFingerprint}
		// j 接收任务或职业编号集合通道送来的当前位置的下标。
		j := <-w.e.jobs
		j.run() // 真实写入成功，模拟游戏服未收到第一次确认。
		w.finishSettlement(binding, q, domain.Role{}, persistence.Fail(p.CodeOperationPending, "未收到原确认"), &j)
		if !r.Pending || r.hostSecondsOpen != 0 || r.Role.HostingSeconds != 3 || len(r.KillFacts) != 1 {
			t.Fatal("确认未知时修改余额、丢失击杀或重新累计原时长")
		}
		w.retryPendingRoom(r, r.RetryAt)
		assertHostingJob(t, w, r, q)
		// retry 接收任务或职业编号集合通道送来的重试。
		retry := <-w.e.jobs
		retry.run()(w)
		if r.Pending || r.Isolated || len(r.KillFacts) != 0 || r.Role.SourceSeq != q.SourceSeq || r.Role.HostingSeconds != 2 ||
			r.Role.Report.SessionID != session || r.Role.Report.UsedSeconds != 1 || r.Role.Report.Kills != 1 || r.Role.Report.RawExperience != q.Experience {
			t.Fatalf("重试未准确恢复同一次托管扣时与击杀收益: %+v", r.Role.Report)
		}
		// filter 初始化用作筛选条件的MongoDB BSON 文档。
		filter := bson.M{"role_id": id, "source_seq": q.SourceSeq, "host_seconds": int64(1), "hosting_session": session, "kills": int64(1), "experience": q.Experience}
		// count 保存由数量文档取得的数量，err 接收数量文档过程中可能产生的错误，供后续失败分支处理。
		count, err := account.DB.Collection("audit_outbox").CountDocuments(ctx, filter)
		if err != nil || count != 1 {
			t.Fatalf("击杀与扣时应只留下同一份永久审计: count=%d err=%v", count, err)
		}
	})
}
