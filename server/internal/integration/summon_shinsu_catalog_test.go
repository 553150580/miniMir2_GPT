// 本文件验证当前目录的神兽购书、升阶、城镇购符、召回和健康重连。
package integration

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"google.golang.org/protobuf/proto"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/bootstrap"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
	"minimir/internal/wire"
)

// TestSummonPetShinsuCatalogMongoWS 使用实际目录交易，再走同一单宠名额的召唤与召回。
func TestSummonPetShinsuCatalogMongoWS(t *testing.T) {
	if os.Getenv("MINIMIR_MONGO_E2E") != "1" {
		t.Skip("神兽目录验收需要专用 MongoDB 副本集")
	}
	cfg, err := loadIntegrationConfig()
	must(t, err)
	f := &cfg.Fixture
	shinsu, exists := f.Skill(devconfig.SummonShinsuSkillID)
	if !exists || shinsu.Summon == nil || shinsu.MaterialQuantity != 5 || f.PetRuntime.BootstrapMode != "none" {
		t.Fatal("正式目录缺少独立神兽或有免费宠物")
	}
	definition, exists := f.PetRuntime.DefinitionByID(shinsu.Summon.DefinitionID)
	if !exists || definition.ResourceKey != "pet_shinsu" {
		t.Fatal("神兽需要独立身体和外观")
	}
	upgrade, exists := f.Upgrade(shinsu.ID, shinsu.LearnedLevel+1)
	if !exists {
		t.Fatal("神兽缺少升阶规则")
	}
	cfg.Database = "minimir_dev_game_test_shinsu_" + domain.ID()
	cfg.AccountAddress, cfg.GameAddress = address(t), address(t)
	ctx, cancel := context.WithCancel(context.Background())
	store, err := persistence.Open(ctx, cfg)
	must(t, err)
	var servers sync.WaitGroup
	for _, mode := range []string{"account", "game"} {
		servers.Go(func() {
			if err := bootstrap.Run(ctx, cfg, mode); err != nil && ctx.Err() == nil {
				t.Errorf("%s 节点启动失败 %v", mode, err)
			}
		})
	}
	t.Cleanup(func() {
		cancel()
		servers.Wait()
		cleanup, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
		must(t, store.DB.Drop(cleanup))
		store.Close(cleanup)
	})
	healthy(t, cfg.AccountAddress)
	healthy(t, cfg.GameAddress)
	endpoint := "http://" + cfg.AccountAddress + "/rpc"
	login, role := &pb.LoginResult{}, &pb.RoleSummary{}
	rpc(t, endpoint, "", p.MsgC2SLocalRegister, &pb.LocalLoginRequest{Account: "m1_shinsu_" + domain.ID()[:8], Password: "test_123456"}, login, 0)
	rpc(t, endpoint, login.AccountToken, p.MsgC2SCreateRole, &pb.CreateRoleRequest{OriginServerId: 1, Slot: 1, Job: 2, Name: "神兽" + domain.ID()[:7], OpId: domain.Bytes(domain.ID())}, role, 0)
	// 仅预置隔离角色的等级、金币和书页。技能书和护符均通过当前商店交易。
	pages := domain.Item{ID: domain.ID(), ItemID: upgrade.PageItemID, Quantity: upgrade.PageCost, Version: 1, Bound: true, SourceKind: "skill_recycle", SourceID: domain.ID()}
	_, err = store.DB.Collection("roles").UpdateOne(ctx, bson.M{"_id": domain.Hex(role.Key.RoleId)}, bson.M{
		"$set":  bson.M{"level": max(shinsu.LearnLevel, upgrade.RequiredRoleLevel), "wallet.bound_gold": 1_000_000},
		"$push": bson.M{"items": pages},
	})
	must(t, err)
	c := join(t, cfg.GameAddress, issue(t, endpoint, login.AccountToken, role.Key))
	defer func() { c.close() }()
	c.auto(false, 1)
	if c.scene.OwnedPet != nil {
		t.Fatal("入场出现免费宠物")
	}
	c.buyAndLearnSkill(*f, shinsu.ID)
	c.buyAndLearnSkill(*f, devconfig.SummonSkeletonSkillID)
	upgradeShinsuFromCatalog(c, f, shinsu, upgrade)
	buyTaoistSupplyAtVendor(c, f, shinsu.MaterialItemID, 6)
	equipTaoistVendorSupply(c, shinsu.MaterialItemID)
	travelToBaseline(c, 2, f.Map.ID)
	travelAcknowledge(c, false)
	time.Sleep(time.Duration(c.player().MoveIntervalMs)*time.Millisecond + 100*time.Millisecond)
	cast := func(skillID uint32, direction uint32) *pb.PetBorn {
		material := summonWSEquipped(t, c)
		result := c.asset(p.MsgC2SCastWithMaterial, &pb.CastWithMaterialRequest{
			SceneEpoch: c.entry.SceneEpoch, CasterLife: c.player().Life, SkillId: skillID, Direction: direction,
			MaterialInstanceId: material.InstanceId, MaterialVersion: material.Version, Operation: c.operation(),
		})
		if result.ErrorCode != 0 {
			t.Fatalf("使用已购护符施放 %d 失败：%v", skillID, result)
		}
		c.inventoryAt(result.NewRevision)
		frame := travelUntilWithin(c, 4*time.Second, func(frame wire.Frame) bool { return frame.ID == p.MsgS2CPetBorn })
		born := &pb.PetBorn{}
		must(t, proto.Unmarshal(frame.Payload, born))
		return born
	}
	born := cast(shinsu.ID, 2)
	owned := c.scene.OwnedPet
	if born.SourceSkillId != shinsu.ID || born.DefinitionId != definition.Base.DefinitionID || born.Direction != 2 ||
		owned.GetLife() != born.EntityLife || owned.GetPetTraining().GetTrainingLevel() != 1 || owned.GetPetTraining().GetMaxTrainingLevel() != 3 ||
		summonWSEquipped(t, c).Quantity != 1 {
		t.Fatal("神兽独立出场、升阶或五张护符扣耗不符", born, owned)
	}
	// 等两次施放之间的原技能冷却，再用无材料字段的召回命令确认同一出生身份。
	time.Sleep(time.Duration(shinsu.CooldownBaseMS) * time.Millisecond)
	c.send(p.MsgC2SCastIntent, &pb.CastIntent{SceneEpoch: c.entry.SceneEpoch, SkillId: shinsu.ID, Direction: 7})
	travelUntilWithin(c, 4*time.Second, func(frame wire.Frame) bool {
		accepted := &pb.CastAccepted{}
		return frame.ID == p.MsgS2CCastAccepted && proto.Unmarshal(frame.Payload, accepted) == nil && accepted.SkillId == shinsu.ID
	})
	time.Sleep(time.Duration(max(shinsu.DelayBaseMS, shinsu.GlobalSpellMS, shinsu.ActionMS)) * time.Millisecond)
	if c.scene.OwnedPet.GetLife() != born.EntityLife || summonWSEquipped(t, c).Quantity != 1 {
		t.Fatal("召回错误扣护符或替换出生身份")
	}
	c.close()
	c = join(t, cfg.GameAddress, issue(t, endpoint, login.AccountToken, role.Key))
	if c.scene.OwnedPet.GetLife() != born.EntityLife || c.scene.OwnedPet.GetEntityHandle() != born.EntityHandle {
		t.Fatal("健康重连未恢复当前神兽")
	}
	skeleton := cast(devconfig.SummonSkeletonSkillID, 4)
	if skeleton.EntityLife <= born.EntityLife || skeleton.DefinitionId == born.DefinitionId || travelCount(c.scene, 2) != 1 {
		t.Fatal("骷髅未替换神兽或出现多只活动宠物")
	}
	t.Log("真实技能书购入、神兽升级、城镇护符购买与装备、五符新生、免费召回、健康重连及单宠切换均通过")
}

// upgradeShinsuFromCatalog 逐项购买当前升阶所需技能书，使用权威版本和书页提交原升级。
func upgradeShinsuFromCatalog(c *client, f *devconfig.Fixture, skill devconfig.Skill, upgrade devconfig.SkillUpgrade) {
	c.t.Helper()
	need := map[uint32]int64{upgrade.PageItemID: upgrade.PageCost}
	for _, cost := range upgrade.BookCosts {
		need[cost.ItemID] += cost.Quantity
		for _, offer := range f.SkillShop {
			if offer.ItemID == cost.ItemID && offer.Currency == "gold" {
				buy := c.skillPreview(&pb.SkillAssetPreviewRequest{Kind: 1, OfferId: offer.ID, Quantity: cost.Quantity})
				if result := c.skillConfirm(buy); result.ErrorCode != 0 {
					c.t.Fatal("NPC 没有交付神兽升级书", result)
				}
				break
			}
		}
	}
	learn := &pb.SkillAssetPreviewRequest{Kind: 2, SkillId: skill.ID}
	for _, item := range c.inventory.Items {
		if quantity := min(need[item.ItemId], item.Quantity); quantity > 0 {
			learn.Materials = append(learn.Materials, &pb.ItemConsumption{InstanceId: item.InstanceId, Version: item.Version, Quantity: quantity})
			need[item.ItemId] -= quantity
		}
	}
	if result := c.skillConfirm(c.skillPreview(learn)); result.ErrorCode != 0 {
		c.t.Fatal("神兽升阶失败", result)
	}
}
