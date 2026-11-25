// 本文件验证开战队列拥塞及原扣次回执释放的边界。
package game

import (
	"testing"
	"time"

	"minimir/internal/devconfig"
	"minimir/internal/domain"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
)

func TestDungeonStartDelayedSubmitKeepsOriginalMembers(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Millisecond)
	engine := &Engine{jobs: make(chan job, 1)}
	engine.jobs <- job{}
	actor := &worker{e: engine}
	start := &dungeonStartAttempt{prepareUntil: now.Add(dungeonStartPrepareWindow)}
	for i := range 3 {
		auth := persistence.Auth{RoleID: domain.ID(), SessionEpoch: int64(i + 1)}
		member := &dungeonPreparedMember{auth: auth, conn: &travelConnection{alive: true, binding: uint64(auth.SessionEpoch)},
			durable: persistence.DungeonAdmissionMember{Auth: auth, ExpectedRevision: 10}}
		slot := &dungeonStartSlot{}
		slot.answer.Store(&dungeonStartPreparation{member: member})
		start.slots = append(start.slots, slot)
	}
	directory, party := newDungeonPartyDirectory(), &dungeonParty{start: start}
	actor.advanceDungeonStart(directory, party, now)
	actor.advanceDungeonStart(directory, party, now.Add(dungeonStartRetryInterval))
	if start.submitted || start.inFlight || len(start.prepared) != 3 || len(start.begin.Members) != 3 || !start.begin.StartedAt.Equal(now) {
		t.Fatalf("队列满时不得提交、改动开始时刻或重复记录原成员：已投递=%v，原成员=%d，原时间=%v", start.submitted, len(start.begin.Members), start.begin.StartedAt)
	}
	<-engine.jobs
	actor.advanceDungeonStart(directory, party, now.Add(2*dungeonStartRetryInterval))
	if !start.submitted || !start.inFlight || len(engine.jobs) != 1 || len(start.begin.Members) != 3 || !start.begin.StartedAt.Equal(now) {
		t.Fatal("队列恢复后应实际提交一份完整的原名单")
	}
}

// TestDungeonStartRefundKeepsOrdinaryRoom 验证已退款的角色可以留在原普通房间继续使用连接。
func TestDungeonStartRefundKeepsOrdinaryRoom(t *testing.T) {
	for _, example := range []struct {
		name   string
		active bool
	}{{"已确认退款", false}, {"原场仍占用且未装场", true}} {
		t.Run(example.name, func(t *testing.T) {
			now := time.Now()
			conn := &travelConnection{alive: true, binding: 1}
			store := &persistence.Store{Cfg: devconfig.Config{RunID: 1}, Instance: "游戏进程", Epoch: 2}
			auth := persistence.Auth{RoleID: domain.ID(), AccountID: domain.ID(), AuthVersion: 1, SessionEpoch: 1}
			prior := domain.Role{ID: auth.RoleID, AccountID: auth.AccountID, Origin: 1, Revision: 10, SessionEpoch: auth.SessionEpoch,
				RunServerID: store.Cfg.RunID, RunEpoch: store.Epoch, Owner: store.Instance}
			member := &dungeonPreparedMember{auth: auth, conn: conn, role: prior,
				durable: persistence.DungeonAdmissionMember{Auth: auth, ExpectedRevision: prior.Revision}}
			start := &dungeonStartAttempt{store: store, conn: conn, code: p.CodeLineRecovering, leader: dungeonPartyIdentity{binding: conn.Binding()},
				begin: persistence.DungeonBegin{ChallengeID: domain.ID()}, prepared: []dungeonPreparedMember{*member}}
			world := &worldState{Auto: true}
			owner := &room{Role: prior, worldState: world, PersonalWorld: world, Auth: auth, Conn: conn, Ready: true,
				dungeonStart: start, Pending: true, PendingOperationID: start.ownerOperation()}
			actor := &worker{e: &Engine{Store: store}, rooms: map[string]*room{auth.RoleID: owner}}
			slot := &dungeonStartSlot{owner: actor}
			slot.answer.Store(&dungeonStartPreparation{member: member})
			start.slots = []*dungeonStartSlot{slot}
			confirmed := prior
			confirmed.Revision += 2
			confirmed.Dungeon = &domain.DungeonProgress{Entries: 1}
			if example.active {
				confirmed.Revision = prior.Revision + 1
				confirmed.Dungeon.ActiveChallengeID = start.begin.ChallengeID
				start.confirmed = map[string]domain.Role{confirmed.ID: confirmed}
			} else {
				start.confirmRefund([]domain.Role{confirmed})
			}
			directory, party := newDungeonPartyDirectory(), &dungeonParty{start: start}
			actor.publishDungeonStart(directory, party, now)
			actor.publishDungeonStart(directory, party, now)
			if party.start != nil || owner.Pending || owner.PendingOperationID != "" || owner.dungeonStart != nil || owner.Role.Revision != confirmed.Revision || owner.Role.Dungeon != confirmed.Dungeon {
				t.Fatal("确认回执未回填原修订或原待确认槽未释放", owner.Role, owner.PendingOperationID)
			}
			if owner.Isolated != example.active || conn.Alive() == example.active || owner.Auto == example.active || owner.PersonalWorld != world {
				t.Fatal("仅原扣次仍占用且无法装场时应隔离，已退款的房间应继续普通挂机", owner.Isolated, conn.Alive(), owner.Auto)
			}
		})
	}
}
