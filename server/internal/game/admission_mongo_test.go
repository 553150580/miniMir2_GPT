// 验证热接管确认丢失后的原票终态、后续新票和旧会话恢复。
package game

import (
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

func TestAdmissionReconcilesOriginalTakeoverBeforeNewTicketMongo(t *testing.T) {
	w, r, oldConn, fixture := admissionMongoRoom(t)
	r.Player.HP--
	r.Player.MP--
	scene, health, mana, position, life := r.Scene, r.Player.HP, r.Player.MP, r.Player.Pos, r.Player.Life

	first := admissionTestCommand(t, fixture, 10)
	attempt := domain.ID()
	r.Pending, r.entryAttempt = true, attempt
	// 实际提交接管，丢弃应用层成功回调，模拟房间只收到待确认结果。
	w.entryJob(r, first, attempt, false).run()
	committed, err := fixture.owner.Role(fixture.ctx, r.Role.ID)
	if err != nil || committed.SessionEpoch != fixture.roleAuth.SessionEpoch+1 || committed.LastEntryAttempt == "" {
		t.Fatalf("首次接管没有提交新会话和核对身份：%+v %v", committed, err)
	}
	w.finishEntry(r, first, attempt, false, domain.Role{}, persistence.Auth{}, persistence.Fail(p.CodeOperationPending, "回调确认丢失"))
	if !r.Pending || r.PendingRetry == nil || r.Conn != oldConn || r.Auth != fixture.roleAuth {
		t.Fatal("未知接管提前释放房间或更换连接")
	}
	stopAuto := wire.Message(p.MsgC2SAutoBattle, 9, oldConn.Binding(), &pb.AutoBattleIntent{SceneEpoch: r.Scene, MonsterCount: 2})
	w.command(command{role: r.Role.ID, conn: oldConn, frame: stopAuto})
	if !r.Auto || r.MonsterCount != 1 || len(oldConn.frames) != 1 || oldConn.frames[0].Error != p.CodeLineRecovering {
		t.Fatal("接管待确认时旧连接仍可改变原房间的战斗状态")
	}
	if _, err = fixture.owner.DB.Collection("entry_tickets").DeleteOne(fixture.ctx, bson.M{"_id": first.ticket.ID}); err != nil {
		t.Fatal(err)
	}
	first.conn.Close("测试断开")
	next := admissionTestCommand(t, fixture, 11)
	w.command(next)
	if len(r.Waiting) != 1 || len(w.e.admissionEntries) != 0 {
		t.Fatal("后续新票没有等待首次接管查证")
	}

	w.retryPendingRoom(r, r.RetryAt)
	if len(w.e.admissionEntries) != 1 || len(w.e.jobs) != 0 {
		t.Fatal("原接管查证没有使用独立入场队列")
	}
	(<-w.e.admissionEntries).run()(w)
	if r.Isolated || !r.Pending || oldConn.Alive() || r.Conn != nil || r.Auth.SessionEpoch != committed.SessionEpoch || len(w.e.admissionEntries) != 1 {
		t.Fatal("原接管未正确确认，或后续新票未继续入场")
	}
	(<-w.e.admissionEntries).run()(w)
	if r.Isolated || r.Pending || r.Conn != next.conn || r.Auth.SessionEpoch != committed.SessionEpoch+1 || r.Role.Revision != committed.Revision+1 {
		t.Fatalf("新票未精确推进一次绑定：隔离=%t 待确认=%t 新连接=%t 代次=%d 版本=%d", r.Isolated, r.Pending, r.Conn == next.conn, r.Auth.SessionEpoch, r.Role.Revision)
	}
	if r.Scene != scene || r.Player.HP != health || r.Player.MP != mana || r.Player.Pos != position || r.Player.Life != life {
		t.Fatalf("新票丢失原健康现场：场景=%d/%d 生命=%d/%d 魔法=%d/%d 坐标=%v/%v 生命代次=%d/%d", r.Scene, scene, r.Player.HP, health, r.Player.MP, mana, r.Player.Pos, position, r.Player.Life, life)
	}
	if _, _, err := fixture.owner.EnterTicket(fixture.ctx, *first.ticket, attempt, false); persistence.Code(err) != p.CodeUnauthenticated {
		t.Fatalf("旧尝试越过更新的角色会话：%v", err)
	}
	if _, _, err := fixture.owner.EnterTicket(fixture.ctx, *next.ticket, domain.ID(), true); persistence.Code(err) != p.CodeUnauthenticated {
		t.Fatalf("同一张已消费票被另一接管尝试接受：%v", err)
	}
}

func TestAdmissionCancelledUnknownAttemptResumesPreviousBindingMongo(t *testing.T) {
	w, r, oldConn, fixture := admissionMongoRoom(t)
	first := admissionTestCommand(t, fixture, 10)

	// 已断开的原尝试没有提交。查证时应保留旧会话，也不消费原票。
	attempt := domain.ID()
	r.Pending, r.entryAttempt = true, attempt
	w.finishEntry(r, first, attempt, false, domain.Role{}, persistence.Auth{}, persistence.Fail(p.CodeOperationPending, "回调确认丢失"))
	first.conn.Close("测试断开")
	w.retryPendingRoom(r, r.RetryAt.Add(time.Millisecond))
	if len(w.e.admissionEntries) != 1 {
		t.Fatal("断开连接后未查证未知尝试")
	}
	(<-w.e.admissionEntries).run()(w)
	persisted, err := fixture.owner.Role(fixture.ctx, r.Role.ID)
	if err != nil || r.Pending || r.entryAttempt != "" || r.Isolated || r.Conn != oldConn || !oldConn.Alive() || r.Auth != fixture.roleAuth || persisted.SessionEpoch != fixture.roleAuth.SessionEpoch {
		t.Fatalf("未提交的原尝试破坏了旧会话：%v", err)
	}
	stopAuto := wire.Message(p.MsgC2SAutoBattle, 9, oldConn.Binding(), &pb.AutoBattleIntent{SceneEpoch: r.Scene, MonsterCount: 2})
	w.command(command{role: r.Role.ID, conn: oldConn, frame: stopAuto})
	if r.Auto || r.MonsterCount != 2 {
		t.Fatal("确认原尝试未提交后未恢复旧连接的正常输入")
	}
	if _, err := fixture.owner.DB.Collection("entry_tickets").FindOne(fixture.ctx, bson.M{"_id": first.ticket.ID, "used": false}).Raw(); err != nil {
		t.Fatalf("已断开的未提交尝试消费了票据：%v", err)
	}
}

// admissionMongoRoom 用独立库首次入场的同一份持久角色初始化健康现场。
func admissionMongoRoom(t *testing.T) (*worker, *room, *travelConnection, hostingMongoFixture) {
	t.Helper()
	cfg, err := devconfig.Load("../../../conf/dev/server.json")
	if err != nil {
		t.Fatal(err)
	}
	fixture := newHostingMongoFixture(t, cfg)
	e := &Engine{Store: fixture.owner, ctx: fixture.ctx, jobs: make(chan job, 1), admissionEntries: make(chan job, 2)}
	w := &worker{e: e}
	r := &room{Auth: fixture.roleAuth}
	w.initialize(r, fixture.role)
	conn := &travelConnection{alive: true, binding: uint64(r.Auth.SessionEpoch)}
	r.Conn, r.Ready = conn, true
	w.rooms = map[string]*room{r.Role.ID: r}
	return w, r, conn, fixture
}

func admissionTestCommand(t *testing.T, fixture hostingMongoFixture, request uint32) command {
	t.Helper()
	ticket, err := fixture.account.IssueTicket(fixture.ctx, fixture.accountAuth, fixture.roleKey)
	if err != nil {
		t.Fatal(err)
	}
	verified, err := fixture.owner.TicketRole(fixture.ctx, ticket.Ticket)
	if err != nil {
		t.Fatal(err)
	}
	return command{role: fixture.role.ID, conn: &travelConnection{alive: true}, ticket: &verified, frame: wire.Frame{Request: request}}
}
