// 本文件验证 AI 报价选择与真人共用席位，并按原队伍修订确认收费。
package game

import (
	"testing"
	"time"

	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
)

func TestDungeonPartyHireCapacityAndOriginalQuote(t *testing.T) {
	f := &devconfig.Fixture{Dungeons: []devconfig.DungeonConfig{{TemplateID: 31, MinimumLevel: 30,
		Hires: []devconfig.DungeonHireOption{{ID: 9, DisplayName: "法师", Job: 1, Level: 35, FeeGold: 31}, {ID: 10, DisplayName: "道士", Job: 2, Level: 34, FeeGold: 43}}}}}
	directory := newDungeonPartyDirectory()
	leader := newDungeonPartyTestMember(0, 34)
	mage := newDungeonPartyTestMember(1, 36)
	tao := newDungeonPartyTestMember(2, 36)
	partyID := domain.Bytes(domain.ID())
	created := directory.handle(leader, f, &pb.DungeonPartyRequest{Action: dungeonPartyCreate, TemplateId: 31, PartyId: partyID}, time.Now()).OwnParty
	call := func(id dungeonPartyIdentity, action uint32, revision uint64, option, slot uint32, want uint32) *pb.DungeonPartyState {
		t.Helper()
		q := &pb.DungeonPartyRequest{Action: action, SceneEpoch: 1, PartyId: partyID, ExpectedRevision: revision, HireOptionId: option, HireSlot: slot}
		if !validDungeonPartyRequest(q) {
			t.Fatalf("专项请求参数不合法：%v", q)
		}
		state := directory.handle(id, f, q, time.Now())
		if state.ErrorCode != want {
			t.Fatalf("操作 %d 预期错误码 %d，实际 %d", action, want, state.ErrorCode)
		}
		return state
	}
	first := call(leader, dungeonPartyHire, created.Revision, 9, 0, 0).OwnParty
	if len(first.Hires) != 1 || first.Hires[0].FeeGold != 31 || first.Revision <= created.Revision {
		t.Fatal("首个可用席、原报价或修订错误", first)
	}
	call(mage, dungeonPartyJoin, first.Revision, 0, 0, p.CodeLineForbidden)
	call(leader, dungeonPartyHire, created.Revision, 10, 0, p.CodeOperationConflict)
	full := call(leader, dungeonPartyHire, first.Revision, 10, 0, 0)
	if len(full.Parties) != 0 || len(full.OwnParty.Hires) != 2 || len(full.OwnParty.Members) != 1 {
		t.Fatal("两个 AI 与一个真人必须用尽三席", full)
	}
	begin := persistence.DungeonBegin{}
	party := directory.byRole[leader.id]
	total, ok := party.freezeDungeonHires(&f.Dungeons[0], &begin)
	if !ok || begin.LeaderRoleID != leader.id || total != 74 ||
		!validDungeonHireReceipt(begin, persistence.DungeonChallenge{HireOptionIDs: []uint32{9, 10}, HireCharge: &persistence.DungeonHireCharge{
			RoleID: leader.id, Gold: 70, BoundGold: 4}}, total) {
		t.Fatal("开战未携带原付款者、原席或原报价", begin)
	}
	f.Dungeons[0].Hires[0].FeeGold++
	if _, ok := party.freezeDungeonHires(&f.Dungeons[0], &persistence.DungeonBegin{}); ok {
		t.Fatal("原确认窗口不应自动接受新报价")
	}
	current := call(leader, dungeonPartyRemoveHire, full.OwnParty.Revision, 0, 2, 0).OwnParty
	current = call(tao, dungeonPartyJoin, current.Revision, 0, 0, 0).OwnParty
	call(tao, dungeonPartyRemoveHire, current.Revision, 0, 1, p.CodeLineForbidden)
	if len(current.Members) != 2 || len(current.Hires) != 1 || current.Hires[0].OptionId != 9 {
		t.Fatal("释放佣兵后未留下原真人可用席", current)
	}
}
