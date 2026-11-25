// 本文件验证招募版本、锁定条件、队长恢复的时间边界和迟到断线隔离。
package game

import (
	"testing"
	"time"

	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	p "minimir/internal/protocol"
)

// newDungeonPartyTestMember 模拟原角色分片已经确认的招募身份。
func newDungeonPartyTestMember(job, level uint32) dungeonPartyIdentity {
	watch := &dungeonPartyPresence{}
	watch.current.Store(&dungeonPartyAttachment{conn: &travelConnection{alive: true, binding: 1}, binding: 1})
	return dungeonPartyIdentity{id: domain.ID(), job: job, level: level, binding: 1, presence: watch}
}

func TestDungeonPartyDirectoryRejectsStaleRoleAndRoster(t *testing.T) {
	now := time.Now()
	fixture := &devconfig.Fixture{Dungeons: []devconfig.DungeonConfig{{TemplateID: 31, MinimumLevel: 30, Duration: 5 * time.Minute}}}
	directory := newDungeonPartyDirectory()
	leader := newDungeonPartyTestMember(0, 34)
	mage, tao, below := newDungeonPartyTestMember(1, 35), newDungeonPartyTestMember(2, 36), newDungeonPartyTestMember(2, 29)
	anotherWarrior := newDungeonPartyTestMember(0, 40)
	id := domain.Bytes(domain.ID())
	call := func(member dungeonPartyIdentity, req *pb.DungeonPartyRequest, wantCode uint32) *pb.DungeonPartyState {
		t.Helper()
		req.SceneEpoch = 77
		if !validDungeonPartyRequest(req) {
			t.Fatal("用例的招募请求格式不正确", req)
		}
		state := directory.handle(member, fixture, req, now)
		if state.ErrorCode != wantCode || len(state.Templates) != 1 || state.SceneEpoch != req.SceneEpoch {
			t.Fatalf("招募请求 %d 预期 %d 实际 %+v", req.Action, wantCode, state)
		}
		return state
	}
	request := func(action uint32, version uint64) *pb.DungeonPartyRequest {
		return &pb.DungeonPartyRequest{Action: action, PartyId: id, ExpectedRevision: version}
	}
	created := call(leader, &pb.DungeonPartyRequest{Action: dungeonPartyCreate, PartyId: id, TemplateId: 31, MinimumLevel: 29}, 0).OwnParty
	if created.GetMinimumLevel() != 30 || created.GetAllowedJobsMask() != 7 || len(created.GetMembers()) != 1 || created.GetRevision() == 0 {
		t.Fatal("创建应采用模板等级下限和真实三职业", created)
	}
	call(below, request(dungeonPartyJoin, created.Revision), p.CodeLineForbidden)
	call(anotherWarrior, request(dungeonPartyJoin, created.Revision), p.CodeLineForbidden)
	joined := call(mage, request(dungeonPartyJoin, created.Revision), 0).OwnParty
	call(tao, request(dungeonPartyJoin, created.Revision), p.CodeOperationConflict)
	if again := call(mage, request(dungeonPartyJoin, created.Revision), 0).OwnParty; again.Revision != joined.Revision || len(again.Members) != 2 {
		t.Fatal("断网重发原入队申请不应重复增加席位", again)
	}
	modify := request(dungeonPartyUpdate, joined.Revision)
	modify.AllowedJobsMask = 1 << 2
	call(mage, modify, p.CodeLineForbidden)
	if still := call(leader, modify, p.CodeLineForbidden).OwnParty; still.Revision != joined.Revision {
		t.Fatal("排斥原队友的条件应整体失败", still)
	}
	full := call(tao, request(dungeonPartyJoin, joined.Revision), 0)
	if len(full.OwnParty.Members) != 3 || len(full.Parties) != 0 {
		t.Fatal("满三人后队伍应只在本队视图展示", full)
	}
	kick := request(dungeonPartyKick, full.OwnParty.Revision)
	kick.TargetRoleId = domain.Bytes(mage.id)
	call(tao, kick, p.CodeLineForbidden)
	after := call(leader, kick, 0).OwnParty
	if len(after.Members) != 2 || directory.byRole[mage.id] != nil {
		t.Fatal("队长踢人未同时清除正反索引", after)
	}
	call(leader, request(dungeonPartyLeave, after.Revision), 0)
	if len(directory.parties) != 0 || len(directory.byRole) != 0 {
		t.Fatal("队长主动离开仍留下原队伍")
	}
	// 同一个创建标识可作为显式申请再用；原低版本成员请求始终失效。
	recreated := call(leader, &pb.DungeonPartyRequest{Action: dungeonPartyCreate, PartyId: id, TemplateId: 31}, 0).OwnParty
	if recreated.Revision <= after.Revision {
		t.Fatal("新队伍复用了旧队伍的成员操作版本", recreated)
	}
	call(tao, request(dungeonPartyJoin, after.Revision), p.CodeOperationConflict)
}

func TestDungeonPartyLeaderPresenceRestoresBeforeOriginalDeadline(t *testing.T) {
	now := time.Now().Truncate(time.Millisecond)
	fixture := &devconfig.Fixture{Dungeons: []devconfig.DungeonConfig{{TemplateID: 31, MinimumLevel: 30, Duration: 5 * time.Minute}}}
	directory := newDungeonPartyDirectory()
	leader, teammate := newDungeonPartyTestMember(0, 40), newDungeonPartyTestMember(2, 40)
	original := leader.presence.current.Load().conn
	roomOwner := &room{dungeonPartyPresence: leader.presence}
	id := domain.Bytes(domain.ID())
	created := directory.handle(leader, fixture, &pb.DungeonPartyRequest{Action: dungeonPartyCreate, PartyId: id, TemplateId: 31}, now).OwnParty
	joined := directory.handle(teammate, fixture, &pb.DungeonPartyRequest{Action: dungeonPartyJoin, PartyId: id, ExpectedRevision: created.Revision}, now).OwnParty
	dungeonPartyDetached(roomOwner, original, now)
	check := &pb.DungeonPartyRequest{Action: dungeonPartyList}
	whileOffline := directory.handle(teammate, fixture, check, now.Add(40*time.Second))
	if whileOffline.OwnParty.LeaderOfflineUntilUnixMs != now.Add(time.Minute).UnixMilli() || whileOffline.OwnParty.Members[0].Online ||
		whileOffline.OwnParty.Revision <= joined.Revision || len(whileOffline.Parties) != 0 {
		t.Fatal("队长的精确60秒原时钟与队员观察不一致", whileOffline)
	}
	nextConn := &travelConnection{alive: true, binding: 2}
	leader.presence.current.Store(&dungeonPartyAttachment{conn: nextConn, binding: 2, disconnectedAt: now, resumedAt: now.Add(59 * time.Second)})
	dungeonPartyDetached(roomOwner, original, now.Add(59*time.Second))
	resumed := directory.handle(teammate, fixture, check, now.Add(time.Minute))
	if !resumed.OwnParty.GetMembers()[0].Online || resumed.OwnParty.GetLeaderOfflineUntilUnixMs() != 0 || len(resumed.Parties) != 1 {
		t.Fatal("及时新票被迟到原断线覆盖", resumed)
	}
	// 下一次关闭可能先从连接观察到。不能拿上一次已经恢复的离线截止来解散。
	nextConn.alive = false
	closedAt := now.Add(2 * time.Minute)
	if state := directory.handle(teammate, fixture, check, closedAt); state.OwnParty.GetLeaderOfflineUntilUnixMs() != closedAt.Add(time.Minute).UnixMilli() {
		t.Fatal("第二次关闭误复用上一次离线计时", state)
	}
	dungeonPartyDetached(roomOwner, nextConn, closedAt)
	lateConn := &travelConnection{alive: true, binding: 3}
	leader.presence.current.Store(&dungeonPartyAttachment{conn: lateConn, binding: 3, disconnectedAt: closedAt, resumedAt: closedAt.Add(time.Minute)})
	if state := directory.handle(teammate, fixture, check, closedAt.Add(time.Minute)); state.OwnParty != nil || len(directory.byRole) != 0 {
		t.Fatal("队长到期边界的新票不应恢复原队伍", state)
	}
}
