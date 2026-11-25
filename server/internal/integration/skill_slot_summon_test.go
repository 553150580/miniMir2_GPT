// 本文件通过真实 MongoDB 和 WebSocket 验证召唤写入技能槽、优先和实际扣料。
package integration

import (
	"slices"
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

// TestSkillAttackSlotsSummonMongoWS 核对道士两个召唤的真实持久配置、扣料、场景和健康重连。
func TestSkillAttackSlotsSummonMongoWS(t *testing.T) {
	cfg := m1FieldConfig(t)
	f := &cfg.Fixture
	skeleton, ok := f.Skill(devconfig.SummonSkeletonSkillID)
	if !ok || !skeleton.AttackSlotSummon() || skeleton.AutoEnabled {
		t.Fatal("召唤骷髅应能显式配置，同时保留原默认自动目录")
	}
	shinsu, ok := f.Skill(devconfig.SummonShinsuSkillID)
	if !ok || !shinsu.AttackSlotSummon() || shinsu.AutoEnabled {
		t.Fatal("召唤神兽应能显式配置，同时保留原默认自动目录")
	}
	// 隔离观察靶放在前格、延长存活，仅用于验证实际普通攻击反馈，技能和宠物规则沿用原配置。
	f.Map.MonsterSpawns[0] = devconfig.Point{X: f.Map.PlayerSpawn.X + 1, Y: f.Map.PlayerSpawn.Y}
	monster := &f.Monsters[0]
	monster.HP, monster.AttackMin, monster.AttackMax, monster.MoveMS, monster.AttackMS = 10000, 0, 0, 60000, 60000
	boss := f.PersonalBoss
	boss.Map.MonsterSpawns[0] = devconfig.Point{X: boss.Map.PlayerSpawn.X + 1, Y: boss.Map.PlayerSpawn.Y}
	boss.Monster.HP, boss.Monster.AttackMin, boss.Monster.AttackMax, boss.Monster.MoveMS, boss.Monster.AttackMS = 10000, 0, 0, 60000, 60000
	cfg, ctx, store := m1FieldServices(t, cfg)
	wantSlots := []uint32{skeleton.ID, shinsu.ID, 0, 0}
	for _, route := range []struct {
		name      string
		skill     devconfig.Skill
		automatic bool
	}{{"手动攻击走骷髅并跳过活宠", skeleton, false}, {"定点自动跳过禁用的骷髅并召神兽", shinsu, true}} {
		t.Run(route.name, func(t *testing.T) {
			endpoint, token, key := m1FieldRole(t, cfg, skeleton.JobID)
			roleID := domain.Hex(key.RoleId)
			// 本用例只预置适龄及金币，召唤书和护符通过实际学习和两个商店接入。
			_, err := store.DB.Collection("roles").UpdateOne(ctx, bson.M{"_id": roleID}, bson.M{"$set": bson.M{
				"level": max(shinsu.LearnLevel, boss.MinimumLevel), "wallet.bound_gold": int64(100000)}})
			must(t, err)
			c := join(t, cfg.GameAddress, issue(t, endpoint, token, key))
			t.Cleanup(c.close)
			c.auto(false, 1)
			unlearned := c.asset(p.MsgC2SSetSkillSlots, &pb.SetSkillSlotsRequest{SkillSlotIds: wantSlots, Operation: c.operation()})
			if unlearned.ErrorCode != p.CodeSkillNotLearned {
				t.Fatalf("未学召唤未按永久技能门槛拒绝：%v", unlearned)
			}
			c.inventoryAt(unlearned.NewRevision)
			c.buyAndLearnSkill(cfg.Fixture, skeleton.ID)
			c.buyAndLearnSkill(cfg.Fixture, shinsu.ID)
			if !slices.Equal(c.inventory.SkillSlotIds, []uint32{0, 0, 0, 0}) {
				t.Fatal("学习召唤偷偷填入缺省技能槽", c.inventory.SkillSlotIds)
			}
			setSkillAttackSlots(c, wantSlots)
			// 无护符的召唤候选会跳过，当前前格敌人仍收到普通攻击。
			assertSummonSlotBasicAttack(c)
			const bought = 12
			buyTaoistSupplyAtVendor(c, &cfg.Fixture, skeleton.MaterialItemID, bought)
			material := equipTaoistVendorSupply(c, skeleton.MaterialItemID)
			travelToBaseline(c, 2, cfg.Fixture.Map.ID)
			travelAcknowledge(c, false)
			time.Sleep(time.Duration(c.player().MoveIntervalMs)*time.Millisecond + 100*time.Millisecond)
			if route.automatic {
				// 首领场继承槽位，但其独立战斗没有原背包永久写权限，只能落到普通攻击。
				challenge, _ := enterReconnectBoss(t, c, boss.TemplateID, false)
				assertSummonSlotBasicAttack(c)
				if c.scene.OwnedPet != nil || summonWSEquipped(t, c).Quantity != bought {
					t.Fatal("个人首领内的槽位召唤改变了原宠物或护符")
				}
				c.send(p.MsgC2SPersonalBossLeave, &pb.PersonalBossLeave{SceneEpoch: c.entry.SceneEpoch, ChallengeId: challenge.ChallengeId})
				returnFromWorldBossTest(c, challenge.ChallengeId)
				applyAutoSkillPreferences(c, &pb.AutoSkillSettings{Enabled: true, DisabledSkillIds: []uint32{skeleton.ID}})
			}
			target := m1LiveMonster(t, c, 1, nil)
			invalid := c.send(p.MsgC2SAttackIntent, &pb.AttackIntent{SceneEpoch: c.entry.SceneEpoch, TargetHandle: target.EntityHandle, TargetLife: target.Life + 1})
			travelExpectError(c, p.MsgC2SAttackIntent, invalid, p.CodeSkillTarget)
			if summonWSEquipped(t, c).Quantity != bought || c.scene.OwnedPet != nil {
				t.Fatal("无效目标导致宠物出场或扣符")
			}
			origin := proto.Clone(c.player().Position).(*pb.GridPoint)
			if route.automatic {
				c.auto(true, 1)
			} else {
				c.send(p.MsgC2SAttackIntent, &pb.AttackIntent{SceneEpoch: c.entry.SceneEpoch, TargetHandle: target.EntityHandle, TargetLife: target.Life})
			}
			accepted := awaitSkillSlotCast(c)
			if accepted.SkillId != route.skill.ID || (accepted.InputSequence == 0) != route.automatic || accepted.TargetHandle != 0 || !proto.Equal(accepted.Origin, origin) {
				t.Fatalf("战斗槽位未按原优先、个人偏好和模式触发自身召唤：%v", accepted)
			}
			born := &pb.PetBorn{}
			frame := travelUntilWithin(c, 8*time.Second, func(frame wire.Frame) bool { return frame.ID == p.MsgS2CPetBorn })
			must(t, proto.Unmarshal(frame.Payload, born))
			pet := c.scene.OwnedPet
			want := int64(bought) - route.skill.MaterialQuantity
			if born.SourceSkillId != route.skill.ID || pet.GetLife() != born.EntityLife || pet.GetEntityHandle() != born.EntityHandle ||
				travelCount(c.scene, 2) != 1 || summonWSEquipped(t, c).Quantity != want {
				t.Fatalf("实际召唤没有保留唯一宠物或正确消耗：出生%v，宠物%v", born, pet)
			}
			// 活宠在槽位中始终跳过。无论另一种也在槽中还是原召唤冷却已结束，都继续落回普通攻击。
			time.Sleep(time.Duration(max(skeleton.CooldownBaseMS, shinsu.CooldownBaseMS))*time.Millisecond + 100*time.Millisecond)
			if !route.automatic {
				assertSummonSlotBasicAttack(c)
			} else {
				travelUntilWithin(c, 6*time.Second, func(frame wire.Frame) bool { return frame.ID == p.MsgS2CAttackAccepted })
				c.auto(false, 1)
			}
			if c.scene.OwnedPet.GetLife() != born.EntityLife || c.scene.OwnedPet.GetEntityHandle() != born.EntityHandle || summonWSEquipped(t, c).Quantity != want {
				t.Fatal("有活宠时重复消耗或被另一槽的宠物替换")
			}
			filter := bson.M{"role_id": roleID, "casting_use.scope": "auto", "casting_use.skill_id": bson.M{"$in": []uint32{skeleton.ID, shinsu.ID}}}
			count, err := store.DB.Collection("operations").CountDocuments(ctx, filter)
			must(t, err)
			var receipt persistence.Operation
			must(t, store.DB.Collection("operations").FindOne(ctx, filter).Decode(&receipt))
			if count != 1 || receipt.Code != 0 || receipt.SourceSeq <= 0 || receipt.Casting.InstanceID != domain.Hex(material.InstanceId) ||
				receipt.Casting.SkillID != route.skill.ID || receipt.Casting.Quantity != route.skill.MaterialQuantity {
				t.Fatalf("原 MongoDB 回执未只扣同一护符堆叠一次：%d、%+v", count, receipt)
			}
			c.close()
			rejoined := join(t, cfg.GameAddress, issue(t, endpoint, token, key))
			t.Cleanup(rejoined.close)
			if !slices.Equal(rejoined.inventory.SkillSlotIds, wantSlots) || rejoined.scene.OwnedPet.GetLife() != born.EntityLife ||
				rejoined.scene.OwnedPet.GetEntityHandle() != born.EntityHandle || summonWSEquipped(t, rejoined).Quantity != want {
				t.Fatal("健康重连未恢复四槽、原活宠和已扣护符")
			}
		})
	}
}

// assertSummonSlotBasicAttack 核对当前明确选择的近格原怪仍收到普通攻击。
func assertSummonSlotBasicAttack(c *client) {
	c.t.Helper()
	target := m1LiveMonster(c.t, c, 1, nil)
	c.send(p.MsgC2SAttackIntent, &pb.AttackIntent{SceneEpoch: c.entry.SceneEpoch, TargetHandle: target.EntityHandle, TargetLife: target.Life})
	frame := travelUntilWithin(c, 4*time.Second, func(frame wire.Frame) bool { return frame.ID == p.MsgS2CAttackAccepted })
	var attack pb.AttackAccepted
	must(c.t, proto.Unmarshal(frame.Payload, &attack))
	if attack.SkillId != 0 || attack.TargetHandle != target.EntityHandle || attack.InputSequence == 0 {
		c.t.Fatalf("不可用召唤未回落原目标的普通攻击：%v", &attack)
	}
}
