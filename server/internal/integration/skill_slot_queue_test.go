// 本文件在真实隔离 MongoDB 与 WebSocket 核对持久槽位、轮转攻击和新票后的纯普攻。
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
	p "minimir/internal/protocol"
	"minimir/internal/wire"
)

// TestSkillAttackSlotsAlternateAndReconnectMongoWS 使用已学法师的隔离前态实测四槽和原攻击协议。
func TestSkillAttackSlotsAlternateAndReconnectMongoWS(t *testing.T) {
	cfg := m1FieldConfig(t)
	fire, known := cfg.Fixture.Skill(1)
	if !known {
		t.Fatal("缺少现行火球术")
	}
	thunder, known := cfg.Fixture.Skill(11)
	if !known || !fire.AttackSlotEligible() || !thunder.AttackSlotEligible() {
		t.Fatal("缺少现行可配置雷电术或火球术")
	}
	mapTemplate := &cfg.Fixture.Map
	mapTemplate.MonsterSpawns[0] = devconfig.Point{X: mapTemplate.PlayerSpawn.X + 1, Y: mapTemplate.PlayerSpawn.Y}
	monster := &cfg.Fixture.Monsters[0]
	// 保留原法术效果和行动时间，只延长隔离观察靶的存活并阻止怪物反击。
	monster.HP, monster.AttackMin, monster.AttackMax, monster.AttackMS, monster.MoveMS = 5000, 0, 0, 60_000, 60_000
	cfg, ctx, store := m1FieldServices(t, cfg)
	endpoint, token, key := m1FieldRole(t, cfg, 1)
	roleID := domain.Hex(key.RoleId)
	learned := []domain.LearnedSkill{{SkillID: fire.ID, Level: fire.LearnedLevel}, {SkillID: thunder.ID, Level: thunder.LearnedLevel}}
	_, err := store.DB.Collection("roles").UpdateOne(ctx, bson.M{"_id": roleID}, bson.M{"$set": bson.M{
		"level": max(fire.LearnLevel, thunder.LearnLevel), "learned_skills": learned}})
	must(t, err)
	before, err := store.Role(ctx, roleID)
	must(t, err)
	c := join(t, cfg.GameAddress, issue(t, endpoint, token, key))
	defer c.close()
	c.auto(false, 1)
	if len(before.SkillSlotIDs) != 0 || !slices.Equal(c.inventory.SkillSlotIds, []uint32{11, 1, 0, 0}) {
		t.Fatal("未设置的老角色没有四槽默认视图", before.SkillSlotIDs, c.inventory.SkillSlotIds)
	}
	setSkillAttackSlots(c, []uint32{1, 11, 0, 0})
	target := travelEntity(c.scene, 3)
	if target == nil || target.GetLife() != 0 {
		t.Fatal("缺少实际零代次普通怪", target)
	}
	wrong := c.send(p.MsgC2SAttackIntent, &pb.AttackIntent{SceneEpoch: c.entry.SceneEpoch, TargetHandle: target.EntityHandle, TargetLife: 1})
	travelExpectError(c, p.MsgC2SAttackIntent, wrong, p.CodeSkillTarget)
	c.send(p.MsgC2SAttackIntent, &pb.AttackIntent{SceneEpoch: c.entry.SceneEpoch, TargetHandle: target.EntityHandle, TargetLife: target.Life})
	first := awaitSkillSlotCast(c)
	if first.SkillId != 1 || first.TargetHandle != target.EntityHandle || first.TargetLife != 0 || first.InputSequence == 0 {
		t.Fatal("已选初生命未按持久首槽实际释放火球", first)
	}
	c.auto(true, 1)
	second := awaitSkillSlotCast(c)
	if second.SkillId != 11 || second.TargetHandle != target.EntityHandle || second.InputSequence != 0 {
		t.Fatal("自动没有轮到较早就绪的原雷电或改变了目标", second)
	}
	c.auto(false, 1)
	setSkillAttackSlots(c, []uint32{0, 0, 0, 0})
	c.close()
	rejoined := join(t, cfg.GameAddress, issue(t, endpoint, token, key))
	defer rejoined.close()
	if !slices.Equal(rejoined.inventory.SkillSlotIds, []uint32{0, 0, 0, 0}) {
		t.Fatal("新票丢失显式四空配置", rejoined.inventory.SkillSlotIds)
	}
	time.Sleep(time.Duration(thunder.ActionMS) * time.Millisecond)
	target = travelEntity(rejoined.scene, 3)
	rejoined.send(p.MsgC2SAttackIntent, &pb.AttackIntent{SceneEpoch: rejoined.entry.SceneEpoch, TargetHandle: target.EntityHandle, TargetLife: target.Life})
	frame := travelUntilWithin(rejoined, 4*time.Second, func(frame wire.Frame) bool { return frame.ID == p.MsgS2CAttackAccepted })
	var attack pb.AttackAccepted
	must(t, proto.Unmarshal(frame.Payload, &attack))
	if attack.SkillId != 0 || attack.TargetHandle != target.EntityHandle || attack.InputSequence == 0 {
		t.Fatal("重连四空仍触发技能或攻击错了实体", &attack)
	}
	after, err := store.Role(ctx, roleID)
	must(t, err)
	if !slices.Equal(after.SkillSlotIDs, []uint32{0, 0, 0, 0}) || after.Wallet != before.Wallet || after.SourceSeq != before.SourceSeq || after.EconomicSeq != before.EconomicSeq+2 {
		t.Fatal("四槽持久化或攻击产生非预期永久资产变动", after.SkillSlotIDs, after.Wallet, after.SourceSeq, after.EconomicSeq)
	}
}

// setSkillAttackSlots 提交真实槽位操作，等到完整权威快照回传。
func setSkillAttackSlots(c *client, slots []uint32) {
	c.t.Helper()
	result := c.asset(p.MsgC2SSetSkillSlots, &pb.SetSkillSlotsRequest{SkillSlotIds: slots, Operation: c.operation()})
	if result.ErrorCode != 0 {
		c.t.Fatalf("设置技能槽失败：%v", result)
	}
	c.inventoryAt(result.NewRevision)
	if !slices.Equal(c.inventory.SkillSlotIds, slots) {
		c.t.Fatal("确认后的技能槽位和设置不一致", c.inventory.SkillSlotIds, slots)
	}
}

// awaitSkillSlotCast 取得下一次真实施法确认，用于区分手动序号和服务端挂机。
func awaitSkillSlotCast(c *client) *pb.CastAccepted {
	c.t.Helper()
	frame := travelUntilWithin(c, 7*time.Second, func(frame wire.Frame) bool { return frame.ID == p.MsgS2CCastAccepted })
	accepted := &pb.CastAccepted{}
	must(c.t, proto.Unmarshal(frame.Payload, accepted))
	return accepted
}
