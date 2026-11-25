// 本文件通过真实账号、游戏 WebSocket 和隔离 MongoDB 验证镜像竞技原名单、格战与唯一终态。
package integration

import (
	"bytes"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"google.golang.org/protobuf/proto"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	p "minimir/internal/protocol"
	"minimir/internal/wire"
)

const (
	mirrorTestMap     = 86
	mirrorTestFighter = 1
	mirrorTestMage    = 2
)

// TestMirrorArenaFrozenOpponentMongoWS 的原守方已下线，场内仍能施放原快照的火球。
func TestMirrorArenaFrozenOpponentMongoWS(t *testing.T) {
	cfg := m1FieldConfig(t)
	configureMirrorArenaTrial(t, &cfg.Fixture)
	// 原装备加法进入固定快照。提高隔离木剑输出以在数个真实移动和攻击帧内观察击杀。
	for i := range cfg.Fixture.Items {
		if cfg.Fixture.Items[i].ID == 218 {
			cfg.Fixture.Items[i].AttackBonus = 18
		}
	}
	cfg.Fixture.Monsters[0].HP, cfg.Fixture.Monsters[0].AttackMin, cfg.Fixture.Monsters[0].AttackMax = 1, 0, 0
	spawn := cfg.Fixture.Map.PlayerSpawn
	cfg.Fixture.Map.MonsterSpawns[0] = devconfig.Point{X: spawn.X + 1, Y: spawn.Y}
	cfg, ctx, store := m1FieldServices(t, cfg)
	endpoint, defenderToken, defenderKey := m1FieldRole(t, cfg, 1)
	_, attackerToken, attackerKey := m1FieldRole(t, cfg, 0)
	defenderID, attackerID := domain.Hex(defenderKey.RoleId), domain.Hex(attackerKey.RoleId)
	fire, ok := cfg.Fixture.Skill(1)
	if !ok || fire.LearnLevel == 0 {
		t.Fatal("隔离样本缺少现有火球术")
	}
	_, err := store.DB.Collection("roles").UpdateByID(ctx, attackerID, bson.M{"$set": bson.M{"level": fire.LearnLevel}})
	must(t, err)
	// 已学技能是隔离前态。发布、换槽、战斗和结果全部走真实 WS。
	_, err = store.DB.Collection("roles").UpdateByID(ctx, defenderID, bson.M{"$set": bson.M{"level": fire.LearnLevel,
		"learned_skills": []domain.LearnedSkill{{SkillID: fire.ID, Level: fire.LearnedLevel}}, "skill_slot_ids": []uint32{1, 0, 0, 0}}})
	must(t, err)
	defender := join(t, cfg.GameAddress, issue(t, endpoint, defenderToken, defenderKey))
	defer func() { defender.close() }()
	defender.auto(false, 1)
	attacker := join(t, cfg.GameAddress, issue(t, endpoint, attackerToken, attackerKey))
	defer func() { attacker.close() }()
	attacker.auto(false, 1)
	var weapon *pb.ItemView
	for _, item := range attacker.inventory.Items {
		if item.ItemId == 218 {
			weapon = item
			break
		}
	}
	if weapon == nil {
		t.Fatal("攻方正常新角色缺少木剑")
	}
	equipped := attacker.asset(p.MsgC2SEquip, &pb.EquipRequest{Operation: attacker.operation(), ItemInstanceId: weapon.InstanceId,
		Slot: devconfig.WeaponSlot, ItemVersion: weapon.Version})
	if equipped.ErrorCode != 0 {
		t.Fatal("无法正常穿戴用于隔离战斗观察的木剑", equipped)
	}
	attacker.inventoryAt(equipped.NewRevision)
	first := mirrorArenaCall(defender, &pb.MirrorArenaRequest{Action: 2})
	if first.ErrorCode != 0 || !first.Open || first.PublishedDefenseVersion != 1 {
		t.Fatal("原守方没有发布首份防守快照", first)
	}
	lobby := mirrorArenaCall(attacker, &pb.MirrorArenaRequest{Action: 1})
	if lobby.ErrorCode != 0 || lobby.DailyLimit != 5 || lobby.EntriesUsed != 0 || len(lobby.Offers) != 1 ||
		lobby.Offers[0].DefenderJob != 1 || lobby.Offers[0].DefenseVersion != 1 {
		t.Fatal("系统没有发放原跨职业候选", lobby)
	}
	offer := lobby.Offers[0]
	setSkillAttackSlots(defender, []uint32{0, 0, 0, 0})
	newPlan := mirrorArenaCall(defender, &pb.MirrorArenaRequest{Action: 2})
	if newPlan.ErrorCode != 0 || newPlan.PublishedDefenseVersion != 2 {
		t.Fatal("守方新技能槽没有产生新版本", newPlan)
	}
	defender.close()
	beforeDefender, err := store.Role(ctx, defenderID)
	must(t, err)
	beforeAttacker, err := store.Role(ctx, attackerID)
	must(t, err)
	forged := mirrorArenaCall(attacker, &pb.MirrorArenaRequest{Action: 3, OfferId: domain.Bytes(domain.ID()), ChallengeId: domain.Bytes(domain.ID())})
	if forged.ErrorCode == 0 || forged.Challenge != nil {
		t.Fatal("开战接受了服务端未分配的任意候选", forged)
	}
	challenge := domain.Bytes(domain.ID())
	enter := &pb.MirrorArenaRequest{Action: 3, OfferId: offer.OfferId, ChallengeId: challenge}
	reserved := mirrorArenaCall(attacker, enter)
	if reserved.ErrorCode != 0 || reserved.BattleState != 1 || reserved.BattleMapId != mirrorTestMap || reserved.BattleSceneEpoch == 0 ||
		reserved.Challenge.GetDefenseVersion() != 1 || !bytes.Equal(reserved.Challenge.GetChallengeId(), challenge) {
		t.Fatal("原场未准确预占或守方旧版本被替换", reserved)
	}
	if mismatched := mirrorArenaCall(attacker, &pb.MirrorArenaRequest{Action: 4, ChallengeId: challenge,
		BattleSceneEpoch: reserved.BattleSceneEpoch + 1}); mismatched.ErrorCode == 0 {
		t.Fatal("错误的镜像战斗观察身份被接受", mismatched)
	}
	ready := &pb.MirrorArenaRequest{Action: 4, ChallengeId: challenge, BattleSceneEpoch: reserved.BattleSceneEpoch}
	if started := mirrorArenaCall(attacker, ready); started.ErrorCode != 0 || started.BattleState != 2 {
		t.Fatal("同一连接无法确认原场并启动战斗", started)
	}
	firstCast := waitMirrorDefenseFireball(t, attacker, challenge, reserved.BattleSceneEpoch, fire.ID)
	t.Logf("守方离线后原版本仍在场帧 %d 施放火球", firstCast)
	priorBinding := attacker.binding
	attacker.close()
	attacker = join(t, cfg.GameAddress, issue(t, endpoint, attackerToken, attackerKey))
	if attacker.binding <= priorBinding || attacker.entry.RoomId == mirrorTestMap {
		t.Fatal("健康新票未换绑定，或普通挂机世界被镜像替换", attacker.entry)
	}
	// 等新附着上更晚的原场帧，确认即使战斗时重连也沿同一进攻与守方身体推进。
	travelUntilWithin(attacker, 6*time.Second, func(raw wire.Frame) bool {
		var frame pb.MirrorArenaCombatFrame
		return raw.ID == p.MsgS2CMirrorArenaCombatFrame && proto.Unmarshal(raw.Payload, &frame) == nil &&
			frame.BattleSceneEpoch == reserved.BattleSceneEpoch && frame.SceneEpoch == attacker.entry.SceneEpoch &&
			frame.Sequence > firstCast && bytes.Equal(frame.ChallengeId, challenge)
	})
	t.Log("健康新票已看到原场后续实体帧，等待格战终态")
	finished := waitMirrorFinish(t, attacker, challenge, 70*time.Second)
	wantPoints, wantStreak := int64(1), uint32(0)
	switch finished.Challenge.Outcome {
	case 1:
		wantPoints, wantStreak = 5, 1
	case 2:
	default:
		t.Fatal("已开战的双方未形成原胜负", finished)
	}
	if finished.Challenge.AwardedPoints != wantPoints || finished.SeasonPoints != wantPoints || finished.Streak != wantStreak {
		t.Fatal("真实胜负未形成一次正确的进攻积分", finished)
	}
	replayed := mirrorArenaCall(attacker, enter)
	if replayed.ErrorCode != 0 || replayed.BattleState != 4 || !proto.Equal(replayed.Challenge, finished.Challenge) {
		t.Fatal("原开始编号重放没有返回既有终态", replayed, finished)
	}
	second := domain.Bytes(domain.ID())
	prepared := mirrorArenaCall(attacker, &pb.MirrorArenaRequest{Action: 3, OfferId: offer.OfferId, ChallengeId: second})
	if prepared.ErrorCode != 0 || prepared.BattleState != 1 {
		t.Fatal("无法预占用于首帧超时观察的场次", prepared)
	}
	interrupted := waitMirrorFinish(t, attacker, second, 15*time.Second)
	if interrupted.Challenge.Outcome != 3 || interrupted.Challenge.AwardedPoints != 0 || interrupted.SeasonPoints != wantPoints || interrupted.Streak != wantStreak {
		t.Fatal("未确认加载的中断污染了前次积分", interrupted)
	}
	lobby = mirrorArenaCall(attacker, &pb.MirrorArenaRequest{Action: 1})
	if lobby.EntriesUsed != 1 || lobby.SeasonPoints != wantPoints || lobby.Streak != wantStreak || lobby.Challenge != nil {
		t.Fatal("最终次数和原赛季积分没有守恒", lobby)
	}
	defender = join(t, cfg.GameAddress, issue(t, endpoint, defenderToken, defenderKey))
	defender.auto(false, 1)
	other := mirrorArenaCall(defender, &pb.MirrorArenaRequest{Action: 1})
	if other.EntriesUsed != 0 || other.SeasonPoints != 0 || other.Streak != 0 || other.PublishedDefenseVersion != 2 || len(other.DefenseHistory) == 0 {
		t.Fatal("原守方未收到战报或被扣分、扣次数", other)
	}
	afterDefender, err := store.Role(ctx, defenderID)
	must(t, err)
	afterAttacker, err := store.Role(ctx, attackerID)
	must(t, err)
	for _, pair := range [][2]domain.Role{{beforeDefender, afterDefender}, {beforeAttacker, afterAttacker}} {
		if pair[0].Wallet != pair[1].Wallet || pair[0].Experience != pair[1].Experience || pair[0].EconomicSeq != pair[1].EconomicSeq || pair[0].SourceSeq != pair[1].SourceSeq {
			t.Fatal("镜像临时属性产生普通角色资产变动", pair[0].ID)
		}
	}

	// 另一个原角色在同一连接保留普通挂机，同时独立观察自己的镜像战斗。
	_, concurrentToken, concurrentKey := m1FieldRole(t, cfg, 0)
	concurrentID := domain.Hex(concurrentKey.RoleId)
	_, err = store.DB.Collection("roles").UpdateByID(ctx, concurrentID, bson.M{"$set": bson.M{"level": fire.LearnLevel}})
	must(t, err)
	concurrent := join(t, cfg.GameAddress, issue(t, endpoint, concurrentToken, concurrentKey))
	defer concurrent.close()
	concurrent.auto(false, 1)
	thirdLobby := mirrorArenaCall(concurrent, &pb.MirrorArenaRequest{Action: 1})
	if len(thirdLobby.Offers) != 1 {
		t.Fatal("无法获得隔离并行测试的守方新版本", thirdLobby)
	}
	third := domain.Bytes(domain.ID())
	thirdOffer := thirdLobby.Offers[0].OfferId
	thirdReserved := mirrorArenaCall(concurrent, &pb.MirrorArenaRequest{Action: 3, OfferId: thirdOffer, ChallengeId: third})
	if thirdReserved.BattleState != 1 {
		t.Fatal("普通挂机角色未能预占镜像", thirdReserved)
	}
	if active := mirrorArenaCall(concurrent, &pb.MirrorArenaRequest{Action: 4, ChallengeId: third,
		BattleSceneEpoch: thirdReserved.BattleSceneEpoch}); active.BattleState != 2 || active.ErrorCode != 0 {
		t.Fatal("并行样本未进入真实战斗", active)
	}
	otherChallenge := concurrent.send(p.MsgC2SPersonalBossEnter, &pb.PersonalBossEnter{SceneEpoch: concurrent.entry.SceneEpoch,
		TemplateId: cfg.Fixture.PersonalBoss.TemplateID, ChallengeId: domain.ID()})
	travelExpectError(concurrent, p.MsgC2SPersonalBossEnter, otherChallenge, p.CodeLineRecovering)
	beforeHunt, err := store.Role(ctx, concurrentID)
	must(t, err)
	concurrent.auto(true, 1)
	for deadline := time.Now().Add(8 * time.Second); ; {
		current, err := store.Role(ctx, concurrentID)
		must(t, err)
		if current.SourceSeq > beforeHunt.SourceSeq {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("真实镜像期间普通角色没有完成挂机结算")
		}
		time.Sleep(100 * time.Millisecond)
	}
	state := mirrorArenaCall(concurrent, &pb.MirrorArenaRequest{Action: 6, ChallengeId: third, BattleSceneEpoch: thirdReserved.BattleSceneEpoch})
	if state.BattleState != 2 || state.ErrorCode != 0 {
		t.Fatal("普通收益确认时原镜像已不在真实战斗", state)
	}
	t.Log("原角色的正常挂机收益与独立镜像格战实际并行")
	concurrent.auto(false, 1)
	if quitting := mirrorArenaCall(concurrent, &pb.MirrorArenaRequest{Action: 5, ChallengeId: third, BattleSceneEpoch: thirdReserved.BattleSceneEpoch}); quitting.BattleState != 3 || quitting.ErrorCode != 0 {
		t.Fatal("主动认输没有进入原终态事务", quitting)
	}
	if lost := waitMirrorFinish(t, concurrent, third, 6*time.Second); lost.Challenge.Outcome != 2 || lost.Challenge.AwardedPoints != 1 {
		t.Fatal("主动认输未按普通失败记分", lost)
	}
}

// configureMirrorArenaTrial 使用工作簿已有计次和积分，只给隔离样本指定开场、网格与候选范围。
func configureMirrorArenaTrial(t *testing.T, fixture *devconfig.Fixture) {
	t.Helper()
	local := time.Now().In(time.FixedZone("Asia/Shanghai", 8*60*60))
	fixture.MirrorArenaRelease = &devconfig.MirrorArenaDefinition{Enabled: true, TemplateID: 6001, DisplayName: "隔离镜像",
		Map: devconfig.MapTemplate{ID: mirrorTestMap, DisplayName: "隔离镜像", ResourceKey: "development/maps/personal-map",
			Width: 13, Height: 13, SceneKind: devconfig.SceneKindMirrorArena, Blocked: []devconfig.Point{{X: 6, Y: 7}}},
		AttackerSpawn: devconfig.Point{X: 4, Y: 6}, DefenderSpawn: devconfig.Point{X: 8, Y: 6},
		AttackerPetSpawn: devconfig.Point{X: 4, Y: 7}, DefenderPetSpawn: devconfig.Point{X: 8, Y: 7},
		SeasonAnchor: time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, local.Location()).AddDate(0, 0, -1),
		SeasonDays:   15, DailyLimit: 5, OpponentLimit: 3, AttackHistoryLimit: 20, DefenseHistoryLimit: 20,
		DurationMS: 60_000, WinBasePoints: 5, LossPoints: 1,
		StreakPoints:  []devconfig.MirrorArenaStreakPoints{{Streak: 1, Points: 5}, {Streak: 3, Points: 6}, {Streak: 5, Points: 8}},
		TimeoutPolicy: devconfig.MirrorArenaTimeoutEffectiveHPRatio, MatchPolicy: devconfig.MirrorArenaMatchCrossJobLevelWindow,
		PetDeathPolicy: devconfig.MirrorArenaPetFollowsOwnerDeath, MaxLevelGap: 3}
	must(t, fixture.ValidateMirrorArena())
}

func mirrorArenaCall(c *client, input *pb.MirrorArenaRequest) *pb.MirrorArenaState {
	c.t.Helper()
	q := proto.Clone(input).(*pb.MirrorArenaRequest)
	q.SceneEpoch = c.entry.SceneEpoch
	request := c.send(p.MsgC2SMirrorArenaRequest, q)
	frame := travelUntilWithin(c, 5*time.Second, func(raw wire.Frame) bool { return raw.Request == request })
	if frame.ID != p.MsgS2CMirrorArenaState || frame.Error != 0 {
		c.t.Fatalf("镜像请求缺少原有类型的回包：%+v", frame)
	}
	out := &pb.MirrorArenaState{}
	must(c.t, proto.Unmarshal(frame.Payload, out))
	return out
}

func waitMirrorDefenseFireball(t *testing.T, c *client, challenge []byte, scene uint64, skill uint32) uint64 {
	t.Helper()
	sequence := uint64(0)
	travelUntilWithin(c, 8*time.Second, func(raw wire.Frame) bool {
		var frame pb.MirrorArenaCombatFrame
		if raw.ID != p.MsgS2CMirrorArenaCombatFrame || proto.Unmarshal(raw.Payload, &frame) != nil ||
			!bytes.Equal(challenge, frame.ChallengeId) || frame.BattleSceneEpoch != scene || frame.SceneEpoch != c.entry.SceneEpoch {
			return false
		}
		if len(frame.Entities) != 2 || frame.Entities[0].EntityKind != 1 || frame.Entities[1].EntityKind != 1 ||
			frame.Entities[0].EntityHandle != mirrorTestFighter || frame.Entities[1].EntityHandle != mirrorTestMage {
			t.Fatal("镜像格战未保留两侧原角色实体", frame.Entities)
		}
		for _, event := range frame.Observations {
			if event.MessageId != uint32(p.MsgS2CCastAccepted) {
				continue
			}
			var cast pb.CastAccepted
			must(t, proto.Unmarshal(event.Payload, &cast))
			if cast.CasterHandle == mirrorTestMage && cast.TargetHandle == mirrorTestFighter && cast.SkillId == skill && cast.SceneEpoch == scene {
				sequence = frame.Sequence
				return true
			}
		}
		return false
	})
	return sequence
}

func waitMirrorFinish(t *testing.T, c *client, challenge []byte, timeout time.Duration) *pb.MirrorArenaState {
	t.Helper()
	var out pb.MirrorArenaState
	var lastObservation int64
	travelUntilWithin(c, timeout, func(raw wire.Frame) bool {
		out.Reset()
		if raw.ID == p.MsgS2CMirrorArenaCombatFrame {
			var sample pb.MirrorArenaCombatFrame
			if proto.Unmarshal(raw.Payload, &sample) == nil && bytes.Equal(challenge, sample.ChallengeId) &&
				sample.ServerUnixMs >= lastObservation+5000 && len(sample.Entities) == 2 {
				lastObservation = sample.ServerUnixMs
				a, b := sample.Entities[0], sample.Entities[1]
				t.Logf("原场帧 %d，攻方生命 %d 格 %v 攻击 %d，守方生命 %d 格 %v 施法 %d", sample.Sequence,
					a.Hp, a.Position, a.AttackSequence, b.Hp, b.Position, b.CastSequence)
			}
		}
		return raw.ID == p.MsgS2CMirrorArenaState && proto.Unmarshal(raw.Payload, &out) == nil && out.SceneEpoch == c.entry.SceneEpoch &&
			out.ErrorCode == 0 && out.BattleState == 4 && bytes.Equal(challenge, out.Challenge.GetChallengeId())
	})
	return &out
}
