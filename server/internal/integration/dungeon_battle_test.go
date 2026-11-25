// 本文件通过真实 MongoDB 和三条 WebSocket 验证同队副本握手、四段实战、健康接回及终态返场。
package integration

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/coder/websocket"
	"go.mongodb.org/mongo-driver/v2/bson"
	"google.golang.org/protobuf/proto"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
	"minimir/internal/wire"
)

const dungeonBattleFireball uint32 = 1 // 使用法师原火球术，不向服务器提交伤害数值。

// TestDungeonBattleThreeRolesMongoWS 验证唯一开战回执、同场技能命中、记账和三个独立附着返场。
func TestDungeonBattleThreeRolesMongoWS(t *testing.T) {
	store, endpoint, route, _ := startDungeonRecruitmentServer(t, true, func(cfg *devconfig.Config) {
		// 仅隔离模板缩短对局，不跳过实际寻路、施法时钟、伤害或分段。
		monster := &cfg.Fixture.ChallengeMonsters[0]
		monster.HP, monster.AttackMin, monster.AttackMax, monster.DefenceMin, monster.DefenceMax = 1, 0, 0, 0, 0
		monster.MagicDefenceMin, monster.MagicDefenceMax, monster.MagicResist = 0, 0, 0
		monster.MoveMS, monster.AttackMS = 60_000, 60_000
		// 仅隔离样本，固定单个候选包以便逐一比对两种绑定规则和原物品实例。
		cfg.Fixture.DungeonReleases[0].Rewards = &devconfig.DungeonRewards{Options: []devconfig.DungeonRewardOption{
			{Weight: 1, Items: []devconfig.DungeonRewardItem{
				{ItemID: 35, Quantity: 2, BindPolicy: devconfig.DungeonRewardBindOnKill},
				{ItemID: 36, Quantity: 1, BindPolicy: devconfig.DungeonRewardAlwaysBound},
			}},
		}}
	})
	recruits := []dungeonRecruit{newDungeonRecruit(t, endpoint, route, 0), newDungeonRecruit(t, endpoint, route, 1), newDungeonRecruit(t, endpoint, route, 2)}
	mageID := domain.Hex(recruits[1].summary.Key.RoleId)
	// 技能学习与技能槽另有专项。这里仅准备能使用原火球的法师，不写次数或战斗结果。
	_, err := store.DB.Collection("roles").UpdateByID(t.Context(), mageID, bson.M{"$set": bson.M{"level": 15, "privilege_until": time.Now().Add(time.Hour),
		"learned_skills": []domain.LearnedSkill{{SkillID: dungeonBattleFireball, Level: 0}}}})
	must(t, err)
	connections := make([]*client, len(recruits))
	originals := make([]*pb.EnterLineResult, len(recruits))
	for i, recruit := range recruits {
		connections[i] = recruit.attach(t)
		originals[i] = proto.Clone(connections[i].entry).(*pb.EnterLineResult)
	}
	leader, mage := connections[0], connections[1]
	partyID := domain.Bytes(domain.ID())
	party := requireDungeonParty(t, dungeonPartyRPC(leader, &pb.DungeonPartyRequest{Action: 2, PartyId: partyID, TemplateId: 3001}), partyID, 1)
	for i := 1; i < len(connections); i++ {
		party = requireDungeonParty(t, dungeonPartyRPC(connections[i], &pb.DungeonPartyRequest{Action: 3, PartyId: partyID,
			ExpectedRevision: party.Revision}), partyID, i+1)
	}
	start := &pb.DungeonPartyRequest{Action: 7, PartyId: partyID, ExpectedRevision: party.Revision}
	if rejected := dungeonPartyRPC(mage, proto.Clone(start).(*pb.DungeonPartyRequest)); rejected.ErrorCode == 0 || len(rejected.ChallengeId) != 0 {
		t.Fatal("普通成员被错误授予原队开战权限", rejected)
	}
	started := dungeonPartyRPC(leader, start)
	if started.ErrorCode != 0 || len(started.ChallengeId) != 16 || started.Starting {
		t.Fatal("开战未返回唯一已确认扣次场次", started)
	}
	challenge := domain.Hex(started.ChallengeId)
	for i, c := range connections {
		confirmDungeonBattleArrival(c, challenge, originals[i])
		if travelCount(c.scene, 3) != 1 || dungeonOwnBody(c).DefinitionId != uint32(i) {
			t.Fatal("未观察到原己方身体及共享敌人", c.entry, c.scene)
		}
		if i == 0 {
			// 有人尚未确认场景时，已经载入的队长也不能提前进入战斗。
			beforeTeamReady := c.send(p.MsgC2SMoveIntent, &pb.MoveIntent{SceneEpoch: c.entry.SceneEpoch, Direction: 2})
			travelExpectError(c, p.MsgC2SMoveIntent, beforeTeamReady, p.CodeLineRecovering)
		}
	}
	for _, c := range connections {
		waitDungeonBattleState(c, challenge, 1, 2)
		if travelCount(c.scene, 1) != len(connections) {
			travelUntilWithin(c, 5*time.Second, func(frame wire.Frame) bool {
				return frame.ID == p.MsgS2CSceneBaseline && travelCount(c.scene, 1) == len(connections)
			})
		}
	}
	t.Log("三职业各自确认首基线并看见原三席与同一段敌人")
	var previousEnemy uint32
	for stage := uint32(1); stage <= 4; stage++ {
		if stage > 1 {
			waitDungeonBattleState(mage, challenge, stage, 2)
			destination := &pb.GridPoint{X: int32(4 * (stage - 1)), Y: 4}
			mage.send(p.MsgC2SMoveIntent, &pb.MoveIntent{SceneEpoch: mage.entry.SceneEpoch, Destination: destination})
			travelUntilWithin(mage, 8*time.Second, func(frame wire.Frame) bool {
				return frame.ID == p.MsgS2CSceneBaseline && proto.Equal(dungeonOwnBody(mage).GetPosition(), destination)
			})
			time.Sleep(time.Duration(dungeonOwnBody(mage).GetMoveIntervalMs())*time.Millisecond + 100*time.Millisecond)
		}
		target := travelEntity(mage.scene, 3)
		if target == nil || target.Hp <= 0 || target.EntityHandle == previousEnemy || target.Life != uint64(stage) {
			t.Fatal("原分段没有生成独立的活怪身体", stage, target)
		}
		previousEnemy = target.EntityHandle
		cast := dungeonCastAt(mage, target)
		if stage == 1 {
			travelUntilWithin(leader, 4*time.Second, func(frame wire.Frame) bool {
				if frame.ID != p.MsgS2CCastAccepted {
					return false
				}
				var observed pb.CastAccepted
				must(t, proto.Unmarshal(frame.Payload, &observed))
				return observed.SceneEpoch == leader.entry.SceneEpoch && observed.CastId == cast.CastId && observed.CasterHandle == mage.entry.PlayerHandle &&
					observed.TargetHandle == target.EntityHandle && observed.SkillId == dungeonBattleFireball
			})
			waitDungeonBattleState(leader, challenge, 2, 2)
			old := connections[2]
			waitDungeonBattleState(old, challenge, 2, 2)
			oldEntry, oldBody, oldBinding := old.entry, dungeonOwnBody(old), old.binding
			old.close()
			resumed := reconnectDungeonForBattle(t, recruits[2])
			confirmDungeonBattleArrival(resumed, challenge, oldEntry)
			waitDungeonBattleState(resumed, challenge, 2, 2)
			body := dungeonOwnBody(resumed)
			if resumed.binding <= oldBinding || body.EntityHandle != oldBody.EntityHandle || body.Life != oldBody.Life || body.Hp != oldBody.Hp ||
				!proto.Equal(body.Position, oldBody.Position) || resumed.entry.SceneEpoch <= oldEntry.SceneEpoch {
				t.Fatal("健康新票没有接回原场原身体", oldEntry, resumed.entry, oldBody, body)
			}
			connections[2] = resumed
			t.Log("道士在第二段用原区服、账号及角色新票接回原身体")
		}
	}
	for i, c := range connections {
		waitDungeonBattleState(c, challenge, 4, 4)
		waitDungeonSceneBaseline(c, challenge, devconfig.SceneKindPersonal)
		travelAcknowledge(c, false)
		if c.entry.RoomId != originals[i].RoomId || c.entry.SceneEpoch <= originals[i].SceneEpoch || c.entry.LineId != originals[i].LineId {
			t.Fatal("通关后没有返回原普通地图及永久入口", originals[i], c.entry)
		}
		roleID := domain.Hex(recruits[i].summary.Key.RoleId)
		role, err := store.Role(t.Context(), roleID)
		must(t, err)
		if role.Dungeon == nil || role.Dungeon.Entries != 1 || role.Dungeon.ActiveChallengeID != "" || role.SourceSeq != 0 || role.Origin != route.origin.OriginID {
			t.Fatal("原角色重复扣次、遗留活动场次或临时击杀串入普通收益", roleID, role.Dungeon, role.SourceSeq)
		}
	}
	var record persistence.DungeonChallenge
	must(t, store.DB.Collection("dungeon_challenges").FindOne(t.Context(), bson.M{"_id": challenge}).Decode(&record))
	number, err := store.DB.Collection("dungeon_challenges").CountDocuments(t.Context(), bson.M{})
	must(t, err)
	if record.RunID != route.assignment.RunID || record.Outcome != persistence.DungeonSucceeded || record.Refunded || len(record.Members) != 3 ||
		!record.FinishedAt.After(record.StartedAt) || !record.FinishedAt.Before(record.ExpiresAt) || record.RewardAt.IsZero() ||
		record.RewardAt.Before(record.StartedAt) || record.RewardAt.After(record.FinishedAt) || number != 1 {
		t.Fatal("原挑战未提交同队唯一通关结果", record)
	}
	for i, recruit := range recruits {
		roleID := domain.Hex(recruit.summary.Key.RoleId)
		found := false
		for _, member := range record.Members {
			if member.RoleID == roleID {
				found = member.EntriesAfter == 1 && member.SessionEpoch == int64(connections[i].binding)
				break
			}
		}
		if !found {
			t.Fatal("永久席位未记录原一次扣次和最终会话围栏", roleID, record.Members)
		}
	}
	verifyDungeonRewardClaims(t, connections, record)
	t.Log("四段通过真实命中推进，三席确认原地图，永久回执唯一且各扣一次")
}

// verifyDungeonRewardClaims 逐连接观察同场的独立原奖包，再通过已有的领取入口核对实际到包实例。
func verifyDungeonRewardClaims(t *testing.T, players []*client, record persistence.DungeonChallenge) {
	t.Helper()
	seen := make(map[string]bool)
	for index, player := range players {
		request := player.send(p.MsgC2SPersonalBossQuery, &pb.PersonalBossQuery{})
		response := travelUntilWithin(player, 6*time.Second, func(frame wire.Frame) bool {
			return frame.ID == p.MsgS2CPersonalBossInfo && frame.Request == request
		})
		info := &pb.PersonalBossInfo{}
		must(t, proto.Unmarshal(response.Payload, info))
		if info.ErrorCode != 0 || len(info.DungeonPendingRewards) != 1 || info.DungeonPendingRewardsMore ||
			len(info.PendingRewards) != 0 || len(info.WorldPendingRewards) != 0 {
			t.Fatalf("讨伐奖包与其它挑战串账或遗漏原队员：%v", info)
		}
		reward := info.DungeonPendingRewards[0]
		id := domain.Hex(reward.RewardId)
		if len(reward.RewardId) != 16 || seen[id] || reward.SourceKind != "dungeon" || reward.TemplateId != record.TemplateID ||
			reward.EarnedAtUnixMs != record.RewardAt.UnixMilli() || len(reward.Items) != 2 {
			t.Fatalf("领取前的原讨伐附件无效：%v", reward)
		}
		seen[id] = true
		first, second := reward.Items[0], reward.Items[1]
		if first.ItemId != 35 || first.Quantity != 2 || first.Bound != (index != 1) || second.ItemId != 36 || !second.Bound ||
			first.SourceId != record.ChallengeID || second.SourceId != record.ChallengeID {
			t.Fatalf("原击杀时特权或来源固定绑定未正确应用：%v", reward.Items)
		}
		result := player.asset(p.MsgC2SClaimChallengeReward, &pb.ClaimChallengeReward{RewardId: reward.RewardId, Operation: player.operation()})
		if result.ErrorCode != 0 || result.Pending {
			t.Fatalf("原奖包未进入真实角色资产事务：%v", result)
		}
		player.inventoryAt(result.NewRevision)
		for _, item := range reward.Items {
			owned := 0
			for _, current := range player.inventory.Items {
				if proto.Equal(current, item) {
					owned++
				}
			}
			if owned != 1 {
				t.Fatalf("原附件实例未按一次原样进包：%v", item)
			}
		}
	}
}

// dungeonOwnBody 按入场下发的真实席位句柄取当前己方身体。
func dungeonOwnBody(c *client) *pb.EntityState {
	for _, body := range c.scene.GetEntities() {
		if body.EntityKind == 1 && body.EntityHandle == c.entry.PlayerHandle {
			return body
		}
	}
	return nil
}

// confirmDungeonBattleArrival 先收全基线、验证错误确认仍挡住输入，再确认同一场次的正确首帧。
func confirmDungeonBattleArrival(c *client, challenge string, prior *pb.EnterLineResult) {
	c.t.Helper()
	waitDungeonSceneBaseline(c, challenge, devconfig.SceneKindDungeon)
	if c.entry.ServerTransitionId != challenge || c.entry.RoomId != 30101 || c.entry.SceneEpoch <= prior.SceneEpoch || c.entry.BaselineId <= prior.BaselineId ||
		c.scene.BaselineId != c.entry.BaselineId || c.scene.ChunkCount != 1 || dungeonOwnBody(c) == nil {
		c.t.Fatal("原讨伐场的首基线或观察身份不合法", c.entry, c.scene)
	}
	c.send(p.MsgC2SEnterWorldAck, &pb.EnterWorldAck{SceneEpoch: c.entry.SceneEpoch, BaselineId: c.entry.BaselineId + 1})
	blocked := c.send(p.MsgC2SMoveIntent, &pb.MoveIntent{SceneEpoch: c.entry.SceneEpoch, Direction: 2})
	travelExpectError(c, p.MsgC2SMoveIntent, blocked, p.CodeLineRecovering)
	travelAcknowledge(c, false, false)
}

// waitDungeonSceneBaseline 核对实际目标 Entry 前已经发布同场、同代次、同地图和基线的转换授权。
func waitDungeonSceneBaseline(c *client, challenge string, kind uint32) {
	c.t.Helper()
	source := c.entry.SceneEpoch
	var transition *pb.DungeonSceneTransition
	travelUntilWithin(c, 7*time.Second, func(frame wire.Frame) bool {
		switch frame.ID {
		case p.MsgS2CDungeonSceneTransition:
			transition = &pb.DungeonSceneTransition{}
			must(c.t, proto.Unmarshal(frame.Payload, transition))
		case p.MsgS2CEnterLine:
			if transition == nil || !bytes.Equal(transition.ChallengeId, domain.Bytes(challenge)) || transition.SourceSceneEpoch != source ||
				transition.TargetSceneEpoch != c.entry.SceneEpoch || transition.TargetBaselineId != c.entry.BaselineId ||
				transition.TargetMapId != c.entry.RoomId || transition.TargetSceneKind != kind || c.entry.SceneKind != kind || c.entry.ServerTransitionId != challenge {
				c.t.Fatal("讨伐 Entry 没有先收到同原场和同目标的转换授权", transition, c.entry)
			}
		case p.MsgS2CSceneBaseline:
			return transition != nil && c.entry.SceneKind == kind && c.scene.SceneEpoch == c.entry.SceneEpoch && c.scene.BaselineId == c.entry.BaselineId
		}
		return false
	})
}

// waitDungeonBattleState 只接受同一新场景、倒计时和挑战编号的指定段状态。
func waitDungeonBattleState(c *client, challenge string, stage, phase uint32) {
	c.t.Helper()
	travelUntilWithin(c, 8*time.Second, func(frame wire.Frame) bool {
		if frame.ID != p.MsgS2CDungeonBattleState {
			return false
		}
		var state pb.DungeonBattleState
		must(c.t, proto.Unmarshal(frame.Payload, &state))
		if state.ErrorCode != 0 || state.SceneEpoch != c.entry.SceneEpoch || !bytes.Equal(state.ChallengeId, domain.Bytes(challenge)) || state.StageCount != 4 {
			c.t.Fatal("收到其他原场次或错误代次的讨伐状态", &state, c.entry)
		}
		return state.Stage == stage && state.State == phase
	})
}

// dungeonCastAt 提交公共施法协议并等待原同段真实法术受理。
func dungeonCastAt(c *client, target *pb.EntityState) *pb.CastAccepted {
	c.t.Helper()
	c.send(p.MsgC2SCastIntent, &pb.CastIntent{SceneEpoch: c.entry.SceneEpoch, SkillId: dungeonBattleFireball, TargetHandle: target.EntityHandle})
	var cast pb.CastAccepted
	travelUntilWithin(c, 4*time.Second, func(frame wire.Frame) bool {
		if frame.ID != p.MsgS2CCastAccepted {
			return false
		}
		must(c.t, proto.Unmarshal(frame.Payload, &cast))
		return cast.SceneEpoch == c.entry.SceneEpoch && cast.SkillId == dungeonBattleFireball && cast.CasterHandle == c.entry.PlayerHandle &&
			cast.TargetHandle == target.EntityHandle && cast.InputSequence != 0
	})
	return &cast
}

// reconnectDungeonForBattle 用原账号和区服实际重新签票，只确认普通首帧，并让服务端自动恢复原副本。
func reconnectDungeonForBattle(t *testing.T, player dungeonRecruit) *client {
	t.Helper()
	ticket := directoryTicket(t, player.endpoint, player.token, player.summary.Key, player.route)
	ctx, stop := context.WithTimeout(t.Context(), 45*time.Second)
	ws, _, err := websocket.Dial(ctx, "ws://"+directoryTicketAddress(t, ticket)+"/ws", nil)
	if err != nil {
		stop()
		must(t, err)
	}
	c := &client{t: t, ws: ws, ctx: ctx, stop: stop}
	t.Cleanup(c.close)
	c.send(p.MsgC2SEnterLine, &pb.EnterLineRequest{AccountTicket: ticket.Ticket})
	travelUntilWithin(c, 5*time.Second, func(frame wire.Frame) bool {
		return frame.ID == p.MsgS2CSceneBaseline && c.entry.SceneKind == devconfig.SceneKindPersonal && c.scene.SceneEpoch == c.entry.SceneEpoch
	})
	travelAcknowledge(c, false)
	return c
}
