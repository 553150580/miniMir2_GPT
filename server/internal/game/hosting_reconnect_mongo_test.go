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

type hostingMongoFixture struct {
	ctx         context.Context
	account     *persistence.Store
	owner       *persistence.Store
	accountAuth persistence.Auth
	roleKey     *pb.RoleKey
	role        domain.Role
	roleAuth    persistence.Auth
}

func newHostingMongoFixture(t *testing.T, cfg devconfig.Config) hostingMongoFixture {
	t.Helper()
	if os.Getenv("MINIMIR_MONGO_E2E") != "1" {
		t.Skip("显式启用本任务副本集中的独立托管计量测试库")
	}
	cfg.Database = "minimir_dev_game_test_host_" + domain.ID()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	t.Cleanup(cancel)
	account, err := persistence.Open(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	var owner *persistence.Store
	t.Cleanup(func() {
		cleanup, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
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
	grant, err := account.ApproveGame(ctx, cfg.NodeID, owner.Instance, cfg.RunID)
	if err != nil {
		t.Fatal(err)
	}
	owner.Epoch = grant.Epoch
	login, err := account.Login(ctx, "frac_"+domain.ID()[:10], "test_123456", true)
	if err != nil {
		t.Fatal(err)
	}
	auth, err := account.Authenticate(ctx, login.AccountToken)
	if err != nil {
		t.Fatal(err)
	}
	key, err := account.CreateRole(ctx, auth, &pb.CreateRoleRequest{OriginServerId: cfg.OriginID, Slot: 1, Job: 0, Name: "余时回归" + domain.ID()[:5], OpId: domain.Bytes(domain.ID())})
	if err != nil {
		t.Fatal(err)
	}
	id := domain.Hex(key.Key.RoleId)
	if _, err = account.DB.Collection("roles").UpdateOne(ctx, bson.M{"_id": id}, bson.M{"$set": bson.M{"hosting_seconds": int64(5)}}); err != nil {
		t.Fatal(err)
	}
	ticket, err := account.IssueTicket(ctx, auth, key.Key)
	if err != nil {
		t.Fatal(err)
	}
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
func TestHostingFractionAcrossHealthyReconnectMongo(t *testing.T) {
	w, r, _ := fullBagPolicyWorker(t)
	fixture := newHostingMongoFixture(t, w.e.Store.Cfg)
	ctx, account, owner := fixture.ctx, fixture.account, fixture.owner
	role, roleAuth := fixture.role, fixture.roleAuth
	id := role.ID
	w.e.Store, w.e.ctx = owner, ctx
	r.Role, r.Auth = role, roleAuth
	r.Conn, r.Ready = &travelConnection{alive: true, binding: uint64(roleAuth.SessionEpoch)}, true
	w.rooms = map[string]*room{id: r}
	monster := r.Monsters[0]
	monster.HP, monster.MaxHP = 1000, 1000
	hpBefore := monster.HP
	var simulatedMS, committedSeconds int64
	now := time.Now().Add(5 * time.Second)
	for segment := 0; segment < 3; segment++ {
		// 走正常断线分支；八个100ms非致死离线tick，健康接管冻结整秒并沿原SourceSeq提交。
		w.command(command{role: id, conn: r.Conn, disconnect: true, at: now})
		session := r.HostSession
		if session == "" {
			t.Fatal("个人离线未建立当前托管会话")
		}
		r.LastTick = now
		for tick := 0; tick < 8; tick++ {
			now = now.Add(100 * time.Millisecond)
			w.step(r, now)
			if r.Pending || len(w.e.jobs) != 0 {
				t.Fatal("未到检查点且无击杀，不能提前提交")
			}
		}
		simulatedMS += 800
		fraction := simulatedMS % 1000
		if r.HostMS != fraction || (5-r.Role.HostingSeconds+r.hostSecondsOpen)*1000+r.HostMS != simulatedMS {
			t.Fatalf("累计模拟和计量不等: simulated=%d balance=%d fraction=%d", simulatedMS, r.Role.HostingSeconds, r.HostMS)
		}
		ticket, err := account.IssueTicket(ctx, fixture.accountAuth, fixture.roleKey)
		if err != nil {
			t.Fatal(err)
		}
		next := &travelConnection{alive: true}
		debit, oldBinding := r.hostSecondsOpen, r.Auth
		w.command(command{role: id, conn: next, ticket: ticket.Ticket, frame: wire.Frame{Request: uint32(segment + 1)}})
		if debit != 0 {
			if len(r.Waiting) != 1 || len(w.e.jobs) != 1 || r.Auth != oldBinding || r.Conn != nil || !r.Pending {
				t.Fatal("未确认的旧会话时长需阻塞接管")
			}
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
	persisted, err := owner.Role(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.HostingSeconds != 3 || persisted.SourceSeq != 2 {
		t.Fatal("内存计费与原永久来源水位不一致")
	}

	t.Run("击杀和扣时共用原来源重试", func(t *testing.T) {
		w.command(command{role: id, conn: r.Conn, disconnect: true, at: now})
		r.LastTick = now
		for tick := 0; tick < 6; tick++ {
			now = now.Add(100 * time.Millisecond)
			w.step(r, now)
		}
		session := r.HostSession
		if r.Pending || r.hostSecondsOpen != 1 || r.HostMS != 0 {
			t.Fatal("击杀前应累计一整秒且没有独立写入")
		}
		r.KillFacts = append(r.KillFacts, killFact{ID: domain.ID(), World: r.currentWorld().Ref, At: now, HostSession: session, Monster: *monster.Monster})
		q := w.killSettlement(r, r.KillFacts[0])
		q.HostSeconds = 1
		w.flushKillFacts(r)
		assertHostingJob(t, w, r, q)
		binding := settlementBinding{room: r, auth: r.Auth, world: r.currentWorld().Ref, generation: r.SettlementGeneration, id: r.PendingOperationID, fingerprint: r.PendingFingerprint}
		j := <-w.e.jobs
		j.run() // 真实写入成功，模拟游戏服未收到第一次确认。
		w.finishSettlement(binding, q, domain.Role{}, persistence.Fail(p.CodeOperationPending, "未收到原确认"), &j)
		if !r.Pending || r.hostSecondsOpen != 0 || r.Role.HostingSeconds != 3 || len(r.KillFacts) != 1 {
			t.Fatal("确认未知时修改余额、丢失击杀或重新累计原时长")
		}
		w.retryPendingRoom(r, r.RetryAt)
		assertHostingJob(t, w, r, q)
		retry := <-w.e.jobs
		retry.run()(w)
		if r.Pending || r.Isolated || len(r.KillFacts) != 0 || r.Role.SourceSeq != q.SourceSeq || r.Role.HostingSeconds != 2 ||
			r.Role.Report.SessionID != session || r.Role.Report.UsedSeconds != 1 || r.Role.Report.Kills != 1 || r.Role.Report.RawExperience != q.Experience {
			t.Fatalf("重试未准确恢复同一次托管扣时与击杀收益: %+v", r.Role.Report)
		}
		filter := bson.M{"role_id": id, "source_seq": q.SourceSeq, "host_seconds": int64(1), "hosting_session": session, "kills": int64(1), "experience": q.Experience}
		count, err := account.DB.Collection("audit_outbox").CountDocuments(ctx, filter)
		if err != nil || count != 1 {
			t.Fatalf("击杀与扣时应只留下同一份永久审计: count=%d err=%v", count, err)
		}
	})
}
