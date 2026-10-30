package game

import (
	"context"
	"os"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/domain"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
	"minimir/internal/wire"
)

// 独立随机库仅提供正常签票、健康Enter和真实计费；固定非致死战斗不代表自然成长速度。
func TestHostingFractionAcrossHealthyReconnectMongo(t *testing.T) {
	if os.Getenv("MINIMIR_MONGO_E2E") != "1" {
		t.Skip("显式启用本任务副本集中的独立托管计量测试库")
	}
	w, r, _ := fullBagPolicyWorker(t)
	cfg := w.e.Store.Cfg
	cfg.Database = "minimir_dev_game_test_frac_" + domain.ID()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
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
		// 走正常断线分支；八个100ms非致死离线tick，整秒沿原SourceSeq真实提交。
		w.command(command{role: id, conn: r.Conn, disconnect: true, at: now})
		session := r.HostSession
		if session == "" {
			t.Fatal("个人离线未建立当前托管会话")
		}
		r.LastTick = now
		for tick := 0; tick < 8; tick++ {
			now = now.Add(100 * time.Millisecond)
			w.step(r, now)
			if r.Pending {
				if len(w.e.jobs) != 1 {
					t.Fatal("整秒未占原唯一事务槽")
				}
				j := <-w.e.jobs
				j.run()(w)
				if r.Pending || r.Isolated || r.Role.Report.SessionID != session || r.Role.Report.UsedSeconds != 1 {
					t.Fatal("计费未提交到跨过整秒的当前会话，或误写旧战报")
				}
				committedSeconds++
			}
		}
		simulatedMS += 800
		fraction := simulatedMS % 1000
		if r.HostMS != fraction || (5-r.Role.HostingSeconds)*1000+r.HostMS != simulatedMS {
			t.Fatalf("累计模拟和计量不等: simulated=%d balance=%d fraction=%d", simulatedMS, r.Role.HostingSeconds, r.HostMS)
		}
		ticket, err = account.IssueTicket(ctx, auth, key.Key)
		if err != nil {
			t.Fatal(err)
		}
		next := &travelConnection{alive: true}
		w.join(r, command{role: id, conn: next, ticket: ticket.Ticket, frame: wire.Frame{Request: uint32(segment + 1)}})
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
}
