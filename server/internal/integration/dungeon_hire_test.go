// 本文件通过真实 WebSocket 验证付费补位、同场 AI 战斗及只向真人发放奖励。
package integration

import (
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"google.golang.org/protobuf/proto"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
	"minimir/internal/wire"
)

const dungeonTrialMageID uint32 = 901 // 隔离样本的 AI 法师选项编号。
const dungeonTrialHireFee int64 = 29  // 隔离金消耗，用于同时覆盖两个金币钱包。

// configureDungeonTrialHire 只在独立联机库启用可沿原火球术参战的已学法师。
func configureDungeonTrialHire(cfg *devconfig.Config) {
	cfg.Fixture.DungeonReleases[0].Hires = []devconfig.DungeonHireOption{{
		ID: dungeonTrialMageID, FeeGold: dungeonTrialHireFee, DisplayName: "隔离法师佣兵", Job: 1, Level: 22,
		Skills: []devconfig.DungeonHireSkill{{ID: dungeonBattleFireball}},
	}}
}

func TestDungeonBattleHiresGoldAndRewardsMongoWS(t *testing.T) {
	store, endpoint, route, _ := startDungeonRecruitmentServer(t, true, func(cfg *devconfig.Config) {
		configureDungeonTrialHire(cfg)
		monster := &cfg.Fixture.ChallengeMonsters[0]
		monster.HP, monster.AttackMin, monster.AttackMax, monster.DefenceMin, monster.DefenceMax = 1, 0, 0, 0, 0
		monster.MagicDefenceMin, monster.MagicDefenceMax, monster.MagicResist = 0, 0, 0
		monster.MoveMS, monster.AttackMS = 60_000, 60_000
		cfg.Fixture.DungeonReleases[0].Rewards = &devconfig.DungeonRewards{Options: []devconfig.DungeonRewardOption{{
			Weight: 1, Items: []devconfig.DungeonRewardItem{{ItemID: 35, Quantity: 1, BindPolicy: devconfig.DungeonRewardAlwaysBound}},
		}}}
	})
	leaderProfile := newDungeonRecruit(t, endpoint, route, 0)
	peerProfile := newDungeonRecruit(t, endpoint, route, 2)
	leaderID, peerID := domain.Hex(leaderProfile.summary.Key.RoleId), domain.Hex(peerProfile.summary.Key.RoleId)
	_, err := store.DB.Collection("roles").UpdateByID(t.Context(), leaderID, bson.M{"$set": bson.M{"wallet.gold": int64(70), "wallet.bound_gold": int64(11)}})
	must(t, err)
	leader, peer := leaderProfile.attach(t), peerProfile.attach(t)
	players := []*client{leader, peer}
	original := []*pb.EnterLineResult{proto.Clone(leader.entry).(*pb.EnterLineResult), proto.Clone(peer.entry).(*pb.EnterLineResult)}

	partyID := domain.Bytes(domain.ID())
	created := dungeonPartyRPC(leader, &pb.DungeonPartyRequest{Action: 2, PartyId: partyID, TemplateId: 3001})
	party := requireDungeonParty(t, created, partyID, 1)
	if len(created.Templates) != 1 || len(created.Templates[0].HireOptions) != 1 ||
		created.Templates[0].HireOptions[0].FeeGold != uint64(dungeonTrialHireFee) {
		t.Fatal("原招募接口未返回确切佣兵报价", created)
	}
	party = requireDungeonParty(t, dungeonPartyRPC(peer, &pb.DungeonPartyRequest{Action: 3, PartyId: partyID, ExpectedRevision: party.Revision}), partyID, 2)
	older := party.Revision
	reserved := dungeonPartyRPC(leader, &pb.DungeonPartyRequest{Action: 8, PartyId: partyID, ExpectedRevision: older, HireOptionId: dungeonTrialMageID})
	party = requireDungeonParty(t, reserved, partyID, 2)
	if len(party.Hires) != 1 || party.Hires[0].OptionId != dungeonTrialMageID || len(reserved.Parties) != 0 {
		t.Fatal("预选没有与真人共用第三席", reserved)
	}
	before, err := store.Role(t.Context(), leaderID)
	must(t, err)
	if before.Wallet.Gold != 70 || before.Wallet.BoundGold != 11 || before.Dungeon != nil {
		t.Fatal("招募预选提前写入了费用或入场次数", before.Wallet, before.Dungeon)
	}
	start := &pb.DungeonPartyRequest{Action: 7, PartyId: partyID, ExpectedRevision: older}
	if stale := dungeonPartyRPC(leader, start); stale.ErrorCode != p.CodeOperationConflict || len(stale.ChallengeId) != 0 {
		t.Fatal("旧确认窗口接受了变动后的佣兵费", stale)
	}
	start.ExpectedRevision = party.Revision
	started := dungeonPartyRPC(leader, start)
	if started.ErrorCode != 0 || len(started.ChallengeId) != 16 {
		t.Fatal("带佣兵的原开战事务未确认", started)
	}
	challenge := domain.Hex(started.ChallengeId)
	for i, player := range players {
		confirmDungeonBattleArrival(player, challenge, original[i])
	}
	var hireHandle uint32
	for _, player := range players {
		waitDungeonBattleState(player, challenge, 1, 2)
		travelUntilWithin(player, 5*time.Second, func(frame wire.Frame) bool {
			for _, entity := range player.scene.GetEntities() {
				if entity.EntityKind == 1 && entity.DefinitionId == 1 && entity.DisplayName == "隔离法师佣兵" && entity.Hp > 0 {
					hireHandle = entity.EntityHandle
					return true
				}
			}
			return false
		})
	}
	travelUntilWithin(peer, 8*time.Second, func(frame wire.Frame) bool {
		var cast pb.CastAccepted
		return frame.ID == p.MsgS2CCastAccepted && proto.Unmarshal(frame.Payload, &cast) == nil &&
			cast.CasterHandle == hireHandle && cast.SkillId == dungeonBattleFireball && cast.SceneEpoch == peer.entry.SceneEpoch
	})
	for _, player := range players {
		waitDungeonBattleState(player, challenge, 4, 4)
		waitDungeonSceneBaseline(player, challenge, devconfig.SceneKindPersonal)
		travelAcknowledge(player, false)
	}
	final := dungeonFinishRecord(t, store, challenge)
	if final.Outcome != persistence.DungeonSucceeded || len(final.Members) != 2 || len(final.HireOptionIDs) != 1 ||
		final.HireOptionIDs[0] != dungeonTrialMageID || final.HireCharge == nil || final.HireCharge.RoleID != leaderID ||
		final.HireCharge.Gold != 18 || final.HireCharge.BoundGold != 11 || final.Refunded {
		t.Fatal("永久通关原名单或双钱包扣款不正确", final)
	}
	cached, err := store.Role(t.Context(), leaderID)
	must(t, err)
	if cached.Wallet.Gold != 52 || cached.Wallet.BoundGold != 0 || cached.Dungeon.Entries != 1 || cached.Dungeon.ActiveChallengeID != "" {
		t.Fatal("成功回场应只保留原实付与一次扣次", cached)
	}
	for _, id := range []string{leaderID, peerID} {
		count, err := store.DB.Collection("dungeon_rewards").CountDocuments(t.Context(), bson.M{"source_id": challenge, "role_id": id})
		must(t, err)
		if count != 1 {
			t.Fatal("原真人独立奖励有且仅有一次", id, count)
		}
	}
	count, err := store.DB.Collection("dungeon_rewards").CountDocuments(t.Context(), bson.M{"source_id": challenge})
	must(t, err)
	if count != 2 {
		t.Fatal("虚拟席位不应创建永久奖励份额", count)
	}
	t.Log("两真人与一法师佣兵完成四段，原火球跨端可见，队长两钱包支付，奖励只确认给两真人")
}
