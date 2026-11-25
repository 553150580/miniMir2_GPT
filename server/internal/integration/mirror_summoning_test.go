// 本文件通过真实 MongoDB 和两个角色的 WebSocket 核对镜像首召、死亡补召及原资产边界。
package integration

import (
	"bytes"
	"reflect"
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

// TestMirrorArenaSummoningMongoWS 在真实格战观察双方原技能、独立出生身份和旧尸体。
func TestMirrorArenaSummoningMongoWS(t *testing.T) {
	cfg := m1FieldConfig(t)
	fixture := &cfg.Fixture
	configureMirrorArenaTrial(t, fixture)
	fixture.MirrorArenaRelease.Summoning = &devconfig.MirrorArenaSummoningRule{AllowWithoutEntryPet: true, MaxBirthsPerSide: 2}
	fixture.MirrorArenaRelease.AttackerSpawn = devconfig.Point{X: 4, Y: 6}
	fixture.MirrorArenaRelease.DefenderSpawn = devconfig.Point{X: 8, Y: 6}
	must(t, fixture.ValidateMirrorArena())
	var skills [2]devconfig.Skill
	var bodies [2]uint32
	for side, id := range [...]uint32{devconfig.SummonSkeletonSkillID, devconfig.SummonShinsuSkillID} {
		skill, ok := fixture.Skill(id)
		if !ok || skill.Summon == nil || skill.MaterialItemID == 0 || skill.MaterialQuantity <= 0 {
			t.Fatal("缺失原召唤技能和护符配置", id)
		}
		definition, ok := fixture.PetRuntime.DefinitionByID(skill.Summon.DefinitionID)
		if !ok {
			t.Fatal("缺失原宠物模板", id)
		}
		skills[side], bodies[side] = skill, definition.Base.DefinitionID
	}
	// 隔离属性仅加速真实击杀并保留双方主人，原技能、动画、动作时间和材料配置保持不变。
	for i := range fixture.PlayerGrowth {
		if fixture.PlayerGrowth[i].JobID == 2 {
			fixture.PlayerGrowth[i].HPBase = 9000
			fixture.PlayerGrowth[i].MPBase = 1000
		}
	}
	for i := range fixture.PetRuntime.Definitions {
		definition := &fixture.PetRuntime.Definitions[i]
		switch definition.SourceSkillID {
		case skills[0].ID:
			definition.Base.HP, definition.Base.Agility = 1, 0
			definition.Base.Defence, definition.Base.DefenceMin, definition.Base.DefenceMax = 0, 0, 0
		case skills[1].ID:
			definition.Base.HP, definition.Base.Accuracy = 1000, 100
			definition.Base.AttackMin, definition.Base.AttackMax = 90, 90
		}
	}
	cfg, ctx, store := m1FieldServices(t, cfg)
	townID := fixture.TownTravel.DestinationMapIDs[0]
	level := max(skills[0].LearnLevel, skills[1].LearnLevel)
	joinSide := func(side int) (*client, string) {
		t.Helper()
		skill := skills[side]
		endpoint, token, key := m1FieldRole(t, cfg, skill.JobID)
		roleID := domain.Hex(key.RoleId)
		quantity := skill.MaterialQuantity
		if side == 0 {
			quantity *= 2
		}
		item := domain.Item{ID: domain.ID(), ItemID: skill.MaterialItemID, Bound: true, Version: 1, Quantity: quantity}
		// 只预设已学与材料库存。实际装符和技能槽随后由正常协议确认。
		_, err := store.DB.Collection("roles").UpdateByID(ctx, roleID, bson.M{
			"$set":  bson.M{"level": level, "learned_skills": []domain.LearnedSkill{{SkillID: skill.ID, Level: skill.LearnedLevel}}},
			"$push": bson.M{"items": item},
		})
		must(t, err)
		c := join(t, cfg.GameAddress, issue(t, endpoint, token, key))
		t.Cleanup(c.close)
		c.auto(false, 1)
		travelToBaseline(c, 1, townID)
		travelAcknowledge(c, true)
		supply := c.asset(p.MsgC2SSetCastingSupply, &pb.SetCastingSupplyRequest{ItemInstanceId: domain.Bytes(item.ID), ItemVersion: 1, Operation: c.operation()})
		if supply.ErrorCode != 0 {
			t.Fatal("实际装入原护符失败", supply)
		}
		c.inventoryAt(supply.NewRevision)
		setSkillAttackSlots(c, []uint32{skill.ID, 0, 0, 0})
		if c.scene.GetOwnedPet() != nil {
			t.Fatal("原城镇已有宠物，无法验证空槽首召")
		}
		return c, roleID
	}
	defender, defenderID := joinSide(1)
	attacker, attackerID := joinSide(0)
	if reply := mirrorArenaCall(defender, &pb.MirrorArenaRequest{Action: 2}); reply.ErrorCode != 0 || reply.PublishedDefenseVersion != 1 {
		t.Fatal("道士未能发布只有技能、没有宠物的原快照", reply)
	}
	defender.close()
	before := make(map[string]domain.Role, 2)
	for _, id := range []string{attackerID, defenderID} {
		stored, err := store.Role(ctx, id)
		must(t, err)
		before[id] = stored
	}
	lobby := mirrorArenaCall(attacker, &pb.MirrorArenaRequest{Action: 1})
	if lobby.ErrorCode != 0 || len(lobby.Offers) != 1 || lobby.Offers[0].DefenderJob != skills[1].JobID {
		t.Fatal("未分配真实同职业守方快照", lobby)
	}
	challenge := domain.Bytes(domain.ID())
	booked := mirrorArenaCall(attacker, &pb.MirrorArenaRequest{Action: 3, OfferId: lobby.Offers[0].OfferId, ChallengeId: challenge})
	if booked.BattleState != 1 || booked.ErrorCode != 0 || booked.BattleSceneEpoch == 0 {
		t.Fatal("未获原场的首帧预占", booked)
	}
	query := &pb.MirrorArenaRequest{Action: 6, ChallengeId: challenge, BattleSceneEpoch: booked.BattleSceneEpoch}
	attacker.onFrame = func(_ *client, frame wire.Frame) {
		if frame.ID == p.MsgS2CMirrorArenaCombatFrame {
			t.Fatal("未确认首帧就开始下发镜像格战")
		}
	}
	time.Sleep(650 * time.Millisecond) // 让预占窗口跨过一次原召唤延迟。
	if pending := mirrorArenaCall(attacker, query); pending.BattleState != 1 || pending.ErrorCode != 0 {
		t.Fatal("首帧前原场不应启动临时施法", pending)
	}
	attacker.onFrame = nil
	query.Action = 4
	if ready := mirrorArenaCall(attacker, query); ready.BattleState != 2 || ready.ErrorCode != 0 {
		t.Fatal("未开始真实镜像格战", ready)
	}
	born := observeMirrorReplacement(t, attacker, challenge, booked.BattleSceneEpoch, fixture.MirrorArena.Map.ID, skills, bodies)
	t.Logf("实际出生：进攻骷髅 %d/%d 死亡后补召 %d/%d，防守神兽 %d/%d", born[103].EntityHandle,
		born[103].EntityLife, born[105].EntityHandle, born[105].EntityLife, born[104].EntityHandle, born[104].EntityLife)
	query.Action = 5
	if started := mirrorArenaCall(attacker, query); started.ErrorCode != 0 || started.BattleState != 3 {
		t.Fatal("原场次认输未进入结算", started)
	}
	if settled := waitMirrorFinish(t, attacker, challenge, 6*time.Second); settled.Challenge.GetOutcome() != 2 || settled.Challenge.GetAwardedPoints() != 1 {
		t.Fatal("补召后原场次的独立积分结算失效", settled)
	}
	for id, start := range before {
		after, err := store.Role(ctx, id)
		must(t, err)
		if !reflect.DeepEqual(start, after) {
			t.Fatal("镜像出生和伤害不应改变普通角色的永久资产、护符或绑定修订", id)
		}
	}
}

// observeMirrorReplacement 在 20027 完整帧把两侧施法、实际出生和原尸体逐一对应。
func observeMirrorReplacement(t *testing.T, c *client, challenge []byte, scene, mapID uint64, skills [2]devconfig.Skill, definitions [2]uint32) map[uint32]*pb.PetBorn {
	t.Helper()
	born := make(map[uint32]*pb.PetBorn, 3)
	casts := make(map[uint64]*pb.CastAccepted, 3)
	travelUntilWithin(c, 18*time.Second, func(raw wire.Frame) bool {
		var frame pb.MirrorArenaCombatFrame
		if raw.ID != p.MsgS2CMirrorArenaCombatFrame || proto.Unmarshal(raw.Payload, &frame) != nil ||
			!bytes.Equal(frame.ChallengeId, challenge) || frame.SceneEpoch != c.entry.SceneEpoch || frame.BattleSceneEpoch != scene {
			return false
		}
		entities := make(map[uint32]*pb.EntityState, len(frame.Entities))
		for _, row := range frame.Entities {
			if entities[row.EntityHandle] != nil {
				t.Fatal("补召帧出现重复实体句柄", row.EntityHandle)
			}
			entities[row.EntityHandle] = row
		}
		for _, event := range frame.Observations {
			switch uint16(event.MessageId) {
			case p.MsgS2CCastAccepted:
				cast := &pb.CastAccepted{}
				must(t, proto.Unmarshal(event.Payload, cast))
				casts[cast.CastId] = cast
			case p.MsgS2CPetBorn:
				birth := &pb.PetBorn{}
				must(t, proto.Unmarshal(event.Payload, birth))
				pet := entities[birth.EntityHandle]
				cast := casts[birth.SourceCastId]
				if birth.EntityHandle < 103 || birth.EntityHandle > 105 || born[birth.EntityHandle] != nil || pet == nil ||
					pet.IsPetCorpse || pet.Hp <= 0 || pet.Life != birth.EntityLife || birth.Role != nil ||
					birth.SceneEpoch != scene || birth.BaselineId != scene || birth.StateSequence != frame.Sequence || birth.MapId != mapID || cast == nil {
					t.Fatal("镜像出生缺少同帧原授权、完整活体或原施法", birth)
				}
				side := (birth.EntityHandle - 103) % 2
				if cast.CasterHandle != side+1 || cast.SkillId != skills[side].ID || birth.SourceSkillId != skills[side].ID ||
					pet.DefinitionId != definitions[side] || birth.DefinitionId != pet.DefinitionId {
					t.Fatal("镜像出生串用阵营、宠物模板或技能", birth, cast)
				}
				born[birth.EntityHandle] = birth
			}
		}
		if born[105] == nil {
			return false
		}
		old := entities[103]
		if born[103] == nil || born[104] == nil || old == nil || !old.IsPetCorpse || old.Hp != 0 ||
			old.Life != born[103].EntityLife || born[105].EntityLife <= old.Life {
			t.Fatal("真补召应保留原死亡身体并生成新出生代次", old, born)
		}
		return true
	})
	return born
}
