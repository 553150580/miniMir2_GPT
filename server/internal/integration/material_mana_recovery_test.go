// 本文件通过真实材料施法和自动补蓝回执验证低蓝时的顺序与防重。
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

func TestManaPotionManualMaterialMongoWS(t *testing.T) {
	cfg := m1FieldConfig(t)
	fixture := &cfg.Fixture
	skill, ok := fixture.Skill(13)
	if !ok || !fixture.CastingSupply.Enabled {
		t.Fatal("当前开发规则必须正常开放道士灵魂火符和护符槽")
	}
	stats, ok := fixture.PlayerAtLevel(skill.JobID, 30)
	if !ok {
		t.Fatal("缺少隔离道士的初始魔法上限")
	}
	for index := range fixture.Skills {
		if fixture.Skills[index].ID == skill.ID {
			// 只在独立试验台使首发消耗刚过半条蓝，第二发会差少量蓝。
			fixed := &fixture.Skills[index]
			fixed.MPBaseCost, fixed.MPLevelCost = stats.MP/2+2, 0
			skill = *fixed
		}
	}
	fixture.Map.PlayerSpawn, fixture.Map.MonsterSpawns[0] = devconfig.Point{X: 5, Y: 5}, devconfig.Point{X: 8, Y: 5}
	fixture.Monsters[0].HP, fixture.Monsters[0].MoveMS = 10000, 600000
	fixture.Monsters[0].AttackMin, fixture.Monsters[0].AttackMax = 0, 0
	cfg, ctx, store := m1FieldServices(t, cfg)
	endpoint, token, key := m1FieldRole(t, cfg, skill.JobID)
	roleID := domain.Hex(key.RoleId)
	amulet := domain.Item{ID: domain.ID(), ItemID: skill.MaterialItemID, Quantity: 3, Version: 1, Bound: true}
	blue := domain.Item{ID: domain.ID(), ItemID: 36, Quantity: 2, Version: 1, Bound: true}
	_, err := store.DB.Collection("roles").UpdateOne(ctx, bson.M{"_id": roleID}, bson.M{"$set": bson.M{
		"level": 30, "learned_skills": []domain.LearnedSkill{{SkillID: skill.ID, Level: skill.LearnedLevel}},
		"items": []domain.Item{amulet, blue},
	}})
	must(t, err)
	c := join(t, cfg.GameAddress, issue(t, endpoint, token, key))
	defer c.close()
	c.auto(false, 1)
	equip := c.asset(p.MsgC2SSetCastingSupply, &pb.SetCastingSupplyRequest{ItemInstanceId: domain.Bytes(amulet.ID), ItemVersion: 1, Operation: c.operation()})
	if equip.ErrorCode != 0 {
		t.Fatal("护符正常装槽失败", equip)
	}
	c.inventoryAt(equip.NewRevision)
	makeCast := func(target uint32) *pb.CastWithMaterialRequest {
		current := summonWSEquipped(t, c)
		return &pb.CastWithMaterialRequest{SceneEpoch: c.entry.SceneEpoch, CasterLife: c.player().Life, SkillId: skill.ID,
			TargetHandle: target, Direction: 2, MaterialInstanceId: current.InstanceId, MaterialVersion: current.Version, Operation: c.operation()}
	}
	first := c.asset(p.MsgC2SCastWithMaterial, makeCast(10))
	if first.ErrorCode != 0 {
		t.Fatal("第一发实际扣蓝施法失败", first)
	}
	c.inventoryAt(first.NewRevision)
	travelUntilWithin(c, 4*time.Second, func(frame wire.Frame) bool {
		return frame.ID == p.MsgS2CSceneBaseline && c.player().CastSequence == 1
	})
	before := c.player().Mp
	if before >= skill.MPBaseCost || summonWSEquipped(t, c).Quantity != 2 {
		t.Fatal("第一次技能未建立缺蓝且有符的前态")
	}
	wait := max(skill.GlobalSpellMS, skill.CooldownBaseMS, skill.ActionMS)
	time.Sleep(time.Duration(wait+60) * time.Millisecond)
	wrongTarget := c.asset(p.MsgC2SCastWithMaterial, makeCast(99))
	c.inventoryAt(wrongTarget.NewRevision)
	wrongMaterial := makeCast(10)
	wrongMaterial.MaterialInstanceId = domain.Bytes(blue.ID)
	deniedSupply := c.asset(p.MsgC2SCastWithMaterial, wrongMaterial)
	c.inventoryAt(deniedSupply.NewRevision)
	if wrongTarget.ErrorCode != p.CodeSkillTarget || deniedSupply.ErrorCode != p.CodeSkillMaterialChanged {
		t.Fatalf("不合法意图进入补蓝判断：目标 %d，材料 %d", wrongTarget.ErrorCode, deniedSupply.ErrorCode)
	}
	checkCount := func(want int64) {
		t.Helper()
		count, err := store.DB.Collection("operations").CountDocuments(ctx, bson.M{"role_id": roleID, "kind": persistence.AssetAutoUsePotion})
		must(t, err)
		if count != want {
			t.Fatalf("原用药事实数量=%d，预期%d", count, want)
		}
	}
	checkCount(0)
	low := makeCast(10)
	result := c.asset(p.MsgC2SCastWithMaterial, low)
	if result.ErrorCode != p.CodeSkillResource {
		t.Fatal("合法但缺蓝的材料施法未确认拒绝", result)
	}
	c.inventoryAt(result.NewRevision + 1)
	travelUntilWithin(c, 6*time.Second, func(frame wire.Frame) bool {
		return frame.ID == p.MsgS2CSceneBaseline && c.player().Mp >= skill.MPBaseCost
	})
	if c.player().CastSequence != 1 || summonWSEquipped(t, c).Quantity != 2 {
		t.Fatal("补蓝重播了原输入或误耗护符")
	}
	checkCount(1)
	persisted, err := store.Role(ctx, roleID)
	must(t, err)
	if !slices.ContainsFunc(persisted.Items, func(item domain.Item) bool { return item.ID == blue.ID && item.Quantity == 1 }) {
		t.Fatal("实际库存未恰好扣一单位蓝药")
	}
	replayed := c.asset(p.MsgC2SCastWithMaterial, proto.Clone(low).(*pb.CastWithMaterialRequest))
	if replayed.ErrorCode != result.ErrorCode || replayed.NewRevision != uint64(persisted.Revision) {
		t.Fatal("原缺蓝操作不能被重算成已成功施法", replayed)
	}
	checkCount(1)
	second := c.asset(p.MsgC2SCastWithMaterial, makeCast(10))
	if second.ErrorCode != 0 {
		t.Fatal("回复魔法后，新手动输入未获得正常施法资格", second)
	}
	c.inventoryAt(second.NewRevision)
	if summonWSEquipped(t, c).Quantity != 1 {
		t.Fatal("新手动施法未按原材料扣一")
	}
}
