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

// TestAdmissionKeepsOriginalSceneAckMongo 用真实角色和票据，控制原确认到达与接管事务执行的先后。
func TestAdmissionKeepsOriginalSceneAckMongo(t *testing.T) {
	t.Run("待执行接管取消后恢复普通场景", func(t *testing.T) {
		w, r, original, fixture := admissionMongoRoom(t)
		r.Ready, r.AwaitUntil = false, time.Now().Add(10*time.Second)
		candidate := admissionTestCommand(t, fixture, 20)
		w.command(candidate)
		w.command(admissionTestAck(r, r, 21))
		if r.Ready || r.entryAck == nil || len(original.frames) != 0 {
			t.Fatal("候选在途时未保留合法确认，或过早启用原普通场景")
		}
		candidate.conn.Close("测试在事务前断开")
		finishQueuedAdmission(t, w)
		if r.Pending || !r.Ready || r.entryAck != nil || r.Conn != original || r.Auth != fixture.roleAuth || !original.Alive() || !admissionSyncReady(original, 21) {
			t.Fatal("候选未启动时没有保留旧连接并确认原单次基线")
		}
		if err := fixture.owner.DB.Collection("entry_tickets").FindOne(fixture.ctx, bson.M{"_id": candidate.ticket.ID, "used": false}).Err(); err != nil {
			t.Fatalf("事务前关闭错误消费原候选票：%v", err)
		}
	})
	t.Run("数据库拒绝接管后仅重放合法首领确认", func(t *testing.T) {
		w, r, original, fixture := admissionMongoRoom(t, func(cfg *devconfig.Config) {
			cfg.Fixture.PersonalBoss.MinimumLevel = 1
		})
		begin := wire.Message(p.MsgC2SPersonalBossEnter, 22, original.Binding(), &pb.PersonalBossEnter{
			SceneEpoch: r.Scene, ChallengeId: domain.ID(), TemplateId: fixture.owner.Cfg.Fixture.PersonalBoss.TemplateID})
		w.command(command{role: r.Role.ID, conn: original, frame: begin})
		if len(w.e.jobs) != 1 {
			t.Fatal("正常首领预约未提交真实数据库任务")
		}
		(<-w.e.jobs).run()(w)
		run := r.personalBoss
		if run == nil || run.battle == nil || !run.shown || run.started {
			t.Fatal("原预约没有进入等待首次确认的个人首领现场")
		}
		candidate := admissionTestCommand(t, fixture, 23)
		w.command(candidate)
		w.command(admissionTestAck(r, run.battle, 24))
		w.command(admissionTestAck(r, r, 25)) // 仍带旧普通场景号的确认不能覆盖首领 ACK。
		if run.started || r.entryAck == nil || r.entryAck.frame.Request != 24 {
			t.Fatal("旧普通场景确认覆盖了首领首次确认")
		}
		// 票据已通过前置读取，随后过期。事务应读取并确认原持久绑定，拒绝这张候选票。
		_, err := fixture.owner.DB.Collection("entry_tickets").UpdateOne(fixture.ctx, bson.M{"_id": candidate.ticket.ID},
			bson.M{"$set": bson.M{"expires_at": time.Now().Add(-time.Minute)}})
		if err != nil {
			t.Fatal(err)
		}
		finishQueuedAdmission(t, w)
		if r.entryAck != nil || r.Pending || r.Conn != original || !original.Alive() || candidate.conn.Alive() ||
			!run.started || !run.battle.Ready || !admissionSyncReady(original, 24) || admissionSyncReady(original, 25) {
			t.Fatal("候选被真实数据库拒绝后，原首领未正确处理一次原确认")
		}
		stored, err := fixture.owner.Role(fixture.ctx, r.Role.ID)
		if err != nil || stored.SessionEpoch != fixture.roleAuth.SessionEpoch || stored.PersonalBoss == nil || stored.PersonalBoss.ActiveChallengeID != run.grant.ChallengeID {
			t.Fatalf("拒绝接管改写了首领原预约或会话：%v", err)
		}
	})
	t.Run("成功接管和过期确认都丢弃暂存", func(t *testing.T) {
		for _, success := range []bool{true, false} {
			w, r, original, fixture := admissionMongoRoom(t)
			r.Ready, r.AwaitUntil = false, time.Now().Add(10*time.Second)
			candidate := admissionTestCommand(t, fixture, 26)
			w.command(candidate)
			w.command(admissionTestAck(r, r, 27))
			if r.entryAck == nil {
				t.Fatal("合法原确认应暂存，等候接管结果")
			}
			if !success {
				candidate.conn.Close("测试取消")
				r.AwaitUntil = time.Now().Add(-time.Millisecond)
			}
			finishQueuedAdmission(t, w)
			if r.Pending || r.Ready || r.entryAck != nil || admissionSyncReady(original, 27) {
				t.Fatal("成功接管后的旧确认，或超期后的暂存确认，打开了游戏场景")
			}
			if success && (r.Conn != candidate.conn || original.Alive() || r.Auth.SessionEpoch != fixture.roleAuth.SessionEpoch+1) ||
				!success && (r.Conn != original || !original.Alive() || r.Auth != fixture.roleAuth) {
				t.Fatal("原确认错误改变接管结果或原连接身份")
			}
		}
	})
}

// admissionTestAck 按原附着绑定构造指定场景的一次基线确认。
func admissionTestAck(owner, scene *room, request uint32) command {
	frame := wire.Message(p.MsgC2SEnterWorldAck, request, owner.Conn.Binding(), &pb.EnterWorldAck{SceneEpoch: scene.Scene, BaselineId: scene.Baseline})
	return command{role: owner.Role.ID, conn: owner.Conn, frame: frame}
}

// finishQueuedAdmission 在测试选定的时点执行请求已排队的真实接管事务及回调。
func finishQueuedAdmission(t *testing.T, w *worker) {
	t.Helper()
	if len(w.e.admissionEntries) != 1 {
		t.Fatal("应该只排队原接管任务")
	}
	(<-w.e.admissionEntries).run()(w)
}

// admissionSyncReady 核对原观察确认的应用层回执。
func admissionSyncReady(connection *travelConnection, request uint32) bool {
	for _, frame := range connection.frames {
		if frame.ID == p.MsgS2CSyncReady && frame.Request == request && frame.Error == 0 {
			return true
		}
	}
	return false
}

// admissionMongoRoom 用独立库首次入场的同一份持久角色初始化健康现场。
func admissionMongoRoom(t *testing.T, configure ...func(*devconfig.Config)) (*worker, *room, *travelConnection, hostingMongoFixture) {
	t.Helper()
	cfg, err := devconfig.Load("../../../conf/dev/server.json")
	if err != nil {
		t.Fatal(err)
	}
	for _, apply := range configure {
		apply(&cfg)
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
