// 本文件经实际区服、NPC与永久操作验证集体隐身的购书、升阶、耗材和施放。
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

// TestTaoistMassHidingCatalogMongoWS 使用默认开发目录，与实际城镇 NPC 交易后验证升级效果。
func TestTaoistMassHidingCatalogMongoWS(t *testing.T) {
	if os.Getenv("MINIMIR_MONGO_E2E") != "1" {
		t.Skip("集体隐身目录验收需要专用 MongoDB 副本集")
	}
	cfg, err := loadIntegrationConfig()
	must(t, err)
	f := &cfg.Fixture
	skill, ok := f.Skill(19)
	if !ok || !f.CastingSupply.Enabled {
		t.Fatal("默认开发目录缺少集体隐身术或未启用材料专槽")
	}
	upgrade, ok := f.Upgrade(skill.ID, skill.LearnedLevel+1)
	if !ok {
		t.Fatal("集体隐身术缺少首阶升级配置")
	}
	cfg.Database = "minimir_dev_game_test_m1_tao_" + domain.ID()
	cfg.AccountAddress, cfg.GameAddress, cfg.GMAddress = address(t), address(t), address(t)
	ctx, cancel := context.WithCancel(context.Background())
	store, err := persistence.Open(ctx, cfg)
	must(t, err)
	var servers sync.WaitGroup
	for _, mode := range []string{"account", "game"} {
		servers.Go(func() {
			if err := bootstrap.Run(ctx, cfg, mode); err != nil && ctx.Err() == nil {
				t.Errorf("%s 启动失败: %v", mode, err)
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
	rpc(t, endpoint, "", p.MsgC2SLocalRegister, &pb.LocalLoginRequest{Account: "m1_tao_" + domain.ID()[:8], Password: "test_123456"}, login, 0)
	rpc(t, endpoint, login.AccountToken, p.MsgC2SCreateRole, &pb.CreateRoleRequest{OriginServerId: 1, Slot: 1, Job: 2, Name: "道隐" + domain.ID()[:7], OpId: domain.Bytes(domain.ID())}, role, 0)
	roleID := domain.Hex(role.Key.RoleId)
	// 限定隔离角色的等级、购买预算和书页前态；所用技能书和三种耗材均通过当前运营目录购买。
	pages := domain.Item{ID: domain.ID(), ItemID: upgrade.PageItemID, Quantity: upgrade.PageCost, Version: 1, Bound: true, SourceKind: "skill_recycle", SourceID: domain.ID()}
	_, err = store.DB.Collection("roles").UpdateOne(ctx, bson.M{"_id": roleID}, bson.M{
		"$set":  bson.M{"level": max(upgrade.RequiredRoleLevel, skill.LearnLevel), "wallet.bound_gold": 1_000_000},
		"$push": bson.M{"items": pages},
	})
	must(t, err)
	t.Log("为隔离道士准备适龄角色、测试金币和书页；学习、同书购买、升级和三种施法耗材均走真实游戏协议")
	c := join(t, cfg.GameAddress, issue(t, endpoint, login.AccountToken, role.Key))
	defer c.close()
	c.auto(false, 1)
	c.buyAndLearnSkill(*f, skill.ID)
	c.buyAndLearnSkill(*f, 13)
	c.buyAndLearnSkill(*f, 6)
	// 升阶从当前技能商店购买所需同书，并按权威物品实例和版本提交。
	need := map[uint32]int64{upgrade.PageItemID: upgrade.PageCost}
	for _, cost := range upgrade.BookCosts {
		need[cost.ItemID] += cost.Quantity
		for _, offer := range f.SkillShop {
			if offer.ItemID == cost.ItemID && offer.Currency == "gold" {
				q := c.skillPreview(&pb.SkillAssetPreviewRequest{Kind: 1, OfferId: offer.ID, Quantity: cost.Quantity})
				if result := c.skillConfirm(q); result.ErrorCode != 0 {
					t.Fatal("升级用技能书购买失败", result)
				}
				break
			}
		}
	}
	upgradeRequest := &pb.SkillAssetPreviewRequest{Kind: 2, SkillId: skill.ID}
	for _, item := range c.inventory.Items {
		if count := min(item.Quantity, need[item.ItemId]); count > 0 {
			upgradeRequest.Materials = append(upgradeRequest.Materials, &pb.ItemConsumption{InstanceId: item.InstanceId, Version: item.Version, Quantity: count})
			need[item.ItemId] -= count
		}
	}
	if result := c.skillConfirm(c.skillPreview(upgradeRequest)); result.ErrorCode != 0 {
		t.Fatal("集体隐身术升阶失败", result)
	}
	// 护符、灰粉和黄粉逐项查实际城镇目录并在其 NPC 相邻格完成报价交易。
	buyTaoistSupplyAtVendor(c, f, skill.MaterialItemID, 2)
	buyTaoistSupplyAtVendor(c, f, 181, 1)
	buyTaoistSupplyAtVendor(c, f, 182, 1)
	equipped := equipTaoistVendorSupply(c, skill.MaterialItemID)
	travelToBaseline(c, 2, f.Map.ID)
	travelAcknowledge(c, false)
	// 旅行保留进城购物的剩余行走时钟，等待一个普通走格间隔再提交新动作。
	time.Sleep(time.Duration(c.player().MoveIntervalMs) * time.Millisecond)
	cast := &pb.CastWithMaterialRequest{SceneEpoch: c.entry.SceneEpoch, CasterLife: c.player().Life,
		SkillId: skill.ID, TargetGrid: c.player().Position, MaterialInstanceId: equipped.InstanceId,
		MaterialVersion: equipped.Version, Operation: c.operation()}
	result := c.asset(p.MsgC2SCastWithMaterial, cast)
	if result.ErrorCode != 0 {
		t.Fatal("升阶后的地格施法扣护符失败", result, c.player())
	}
	c.inventoryAt(result.NewRevision)
	travelUntilWithin(c, 4*time.Second, func(frame wire.Frame) bool {
		if frame.ID == p.MsgS2CSceneBaseline {
			for _, status := range c.player().Statuses {
				if status.StatusId == skill.BuffID && status.SourceSkillId == skill.ID && status.SkillLevel == upgrade.Level && status.RemainingMs > 0 {
					return true
				}
			}
		}
		return false
	})
	// 同一原施法请求重放仍取回首次确认，已经改变的材料物品和技能时钟不会重新扣除。
	replay := c.asset(p.MsgC2SCastWithMaterial, cast)
	if replay.ErrorCode != 0 || replay.CommandSeq != result.CommandSeq || replay.NewRevision != result.NewRevision {
		t.Fatal("集体隐身术原请求重放未返回首次回执", replay)
	}
	var receipt persistence.Operation
	must(t, store.DB.Collection("operations").FindOne(ctx, bson.M{"_id": "asset:" + roleID + ":" + domain.Hex(cast.Operation.OpId)}).Decode(&receipt))
	if receipt.Casting == nil || receipt.Casting.InstanceID != domain.Hex(equipped.InstanceId) || receipt.Casting.Quantity != 1 || receipt.Casting.SkillID != skill.ID {
		t.Fatalf("持久回执没有只消费一个原护符单位：%+v", receipt)
	}
	// 同批 NPC 购入的护符、灰粉分别用于定点自动施法，校验原施法点和实际命中。
	applyAutoSkillPreferences(c, &pb.AutoSkillSettings{Enabled: true})
	fire := expectTaoistCatalogAutoCast(c, 13)
	green := equipTaoistVendorSupply(c, 181)
	poison := expectTaoistCatalogAutoCast(c, 6)
	for _, used := range []struct {
		skill  uint32
		castID uint64
		item   *pb.ItemView
	}{
		{skill: 13, castID: fire.CastId, item: equipped},
		{skill: 6, castID: poison.CastId, item: green},
	} {
		var automatic persistence.Operation
		must(t, store.DB.Collection("operations").FindOne(ctx, bson.M{"role_id": roleID, "casting_use.scope": "auto", "casting_use.skill_id": used.skill}).Decode(&automatic))
		if used.castID == 0 || automatic.SourceSeq <= 0 || automatic.Casting == nil ||
			automatic.Casting.InstanceID != domain.Hex(used.item.InstanceId) || automatic.Casting.Quantity != 1 {
			t.Fatalf("自动技能 %d 没有消费一单位从 NPC 购买的原材料：%+v", used.skill, automatic)
		}
	}
}

// buyTaoistSupplyAtVendor 选择真实城镇的供货 NPC，按当前地图授权和原报价完成购买。
func buyTaoistSupplyAtVendor(c *client, f *devconfig.Fixture, itemID uint32, quantity int64) {
	c.t.Helper()
	for _, mapID := range f.TownTravel.DestinationMapIDs {
		town, _ := f.MapByID(mapID)
		for _, npc := range town.NPCs {
			shop := f.NPCShopByRole(npc.Role)
			if shop == nil {
				continue
			}
			for _, offer := range shop.Offers {
				if offer.ItemID == itemID && offer.Currency == "gold" {
					buyTaoistSupplyOnMap(c, town, npc, offer, quantity)
					return
				}
			}
		}
	}
	c.t.Fatalf("当前城镇没有购买道士耗材 %d 的 NPC", itemID)
}

// naturalEquipmentVendor 从真实城镇职责及商店目录定位原装备和报价。
func naturalEquipmentVendor(t *testing.T, fixture *devconfig.Fixture, itemID uint32) (*devconfig.MapTemplate, devconfig.TownNPC, devconfig.SkillShop) {
	t.Helper()
	for _, mapID := range fixture.TownTravel.DestinationMapIDs {
		town, found := fixture.MapByID(mapID)
		if !found {
			continue
		}
		for _, npc := range town.NPCs {
			shop := fixture.NPCShopByRole(npc.Role)
			if shop == nil {
				continue
			}
			for _, offer := range shop.Offers {
				if offer.ItemID == itemID && offer.Currency == "gold" && offer.Quantity == 1 && offer.PriceUnits > 0 {
					return town, npc, offer
				}
			}
		}
	}
	t.Fatalf("现行城镇没有装备%d的金币渠道", itemID)
	return nil, devconfig.TownNPC{}, devconfig.SkillShop{}
}

// reachTownVendor 从当前场景走到指定城镇商人的相邻可走格。
func reachTownVendor(c *client, town *devconfig.MapTemplate, npc devconfig.TownNPC) {
	c.t.Helper()
	if c.entry.RoomId != town.ID {
		travelToBaseline(c, 1, town.ID)
		travelAcknowledge(c, true)
	}
	// 共享城镇的第一位玩家可能属于其他连接，使用当前附着在首包确认的原实体。
	position := challengePlayerDeathBody(c).Position
	near := func(point *pb.GridPoint) bool {
		x, y := int(point.X)-npc.Position.X, int(point.Y)-npc.Position.Y
		return x >= -1 && x <= 1 && y >= -1 && y <= 1
	}
	if !near(position) {
		for _, offset := range []devconfig.Point{{X: 1}, {X: -1}, {Y: 1}, {Y: -1}, {X: 1, Y: 1}, {X: -1, Y: 1}, {X: 1, Y: -1}, {X: -1, Y: -1}} {
			point := devconfig.Point{X: npc.Position.X + offset.X, Y: npc.Position.Y + offset.Y}
			if !town.Walkable(point) {
				continue
			}
			grid := &pb.GridPoint{X: int32(point.X), Y: int32(point.Y)}
			occupied := false
			for _, entity := range c.scene.GetEntities() {
				if entity.EntityHandle != c.entry.PlayerHandle && proto.Equal(entity.Position, grid) {
					occupied = true
					break
				}
			}
			if occupied {
				continue
			}
			c.send(p.MsgC2SMoveIntent, &pb.MoveIntent{SceneEpoch: c.entry.SceneEpoch, Destination: grid})
			// 心跳保持同一连接并读入基线。一个相邻格动态受阻时，可在原附着内改试其它格子。
			deadline := time.Now().Add(10 * time.Second)
			for time.Now().Before(deadline) {
				heartbeat := c.send(p.MsgC2SHeartbeat, &pb.Heartbeat{})
				travelUntilWithin(c, 4*time.Second, func(frame wire.Frame) bool {
					return frame.ID == p.MsgS2CHeartbeat && frame.Request == heartbeat
				})
				nav := c.scene.GetNavigationState()
				if nav.GetInputSequence() == c.input {
					if nav.GetStatus() == 2 && proto.Equal(challengePlayerDeathBody(c).Position, grid) {
						return
					}
					if nav.GetStatus() == 5 {
						break
					}
				}
				time.Sleep(150 * time.Millisecond)
			}
		}
	}
	if !near(challengePlayerDeathBody(c).Position) {
		c.t.Fatalf("没有到达商店 NPC %d 的相邻地格", npc.ID)
	}
}

// buyTaoistSupplyOnMap 到 NPC 相邻地格后验证报价、原操作和商品来源。
func buyTaoistSupplyOnMap(c *client, town *devconfig.MapTemplate, npc devconfig.TownNPC, offer devconfig.SkillShop, quantity int64) {
	c.t.Helper()
	reachTownVendor(c, town, npc)
	request := c.send(p.MsgC2SVendorQuote, &pb.VendorQuoteRequest{SceneEpoch: c.entry.SceneEpoch, NpcId: npc.ID, Action: 1, OfferId: offer.ID, Quantity: quantity})
	frame := travelUntilWithin(c, 3*time.Second, func(frame wire.Frame) bool { return frame.ID == p.MsgS2CVendorQuote && frame.Request == request })
	var quote pb.VendorQuoteResult
	must(c.t, proto.Unmarshal(frame.Payload, &quote))
	if quote.ItemId != offer.ItemID || quote.BoundGold != offer.PriceUnits*quantity {
		c.t.Fatalf("耗材 %d 的实际金币报价错误：%+v", offer.ItemID, &quote)
	}
	result := c.asset(p.MsgC2SVendorCommit, &pb.VendorCommitRequest{SceneEpoch: c.entry.SceneEpoch, NpcId: npc.ID, QuoteId: quote.QuoteId, Operation: c.operation()})
	if result.ErrorCode != 0 {
		c.t.Fatal("NPC 购买耗材失败", result)
	}
	c.inventoryAt(result.NewRevision)
	for _, item := range c.inventory.Items {
		if item.ItemId == offer.ItemID && item.SourceKind == "npc_shop" && item.Bound && item.Quantity == offer.Quantity*quantity {
			return
		}
	}
	c.t.Fatalf("NPC 未按约定发放耗材 %d", offer.ItemID)
}

// equipTaoistVendorSupply 使用本例 NPC 交付的物品授权穿戴。
func equipTaoistVendorSupply(c *client, itemID uint32) *pb.ItemView {
	c.t.Helper()
	for _, item := range c.inventory.Items {
		if item.ItemId == itemID && item.SourceKind == "npc_shop" {
			result := c.asset(p.MsgC2SSetCastingSupply, &pb.SetCastingSupplyRequest{ItemInstanceId: item.InstanceId, ItemVersion: item.Version, Operation: c.operation()})
			if result.ErrorCode != 0 {
				c.t.Fatalf("装备 NPC 购入的材料 %d 失败：%v", itemID, result)
			}
			c.inventoryAt(result.NewRevision)
			equipped := summonWSEquipped(c.t, c)
			if equipped.ItemId != itemID {
				c.t.Fatalf("材料专槽未同步 NPC 购入的物品 %d：%v", itemID, equipped)
			}
			return equipped
		}
	}
	c.t.Fatalf("实际 NPC 购买没有交付材料 %d", itemID)
	return nil
}

// expectTaoistCatalogAutoCast 启用定点自动，确认技能从原位置释放并命中原怪物。
func expectTaoistCatalogAutoCast(c *client, skillID uint32) *pb.CastAccepted {
	c.t.Helper()
	origin := proto.Clone(c.player().Position).(*pb.GridPoint)
	c.auto(true, 1)
	cast := &pb.CastAccepted{}
	travelUntilWithin(c, 10*time.Second, func(frame wire.Frame) bool {
		if frame.ID != p.MsgS2CCastAccepted {
			return false
		}
		must(c.t, proto.Unmarshal(frame.Payload, cast))
		if cast.SkillId != skillID || cast.InputSequence != 0 || cast.TargetHandle == 0 || !proto.Equal(cast.Origin, origin) {
			c.t.Fatal("自动施法技能、目标或原地等待不符", cast)
		}
		return true
	})
	travelUntilWithin(c, 5*time.Second, func(frame wire.Frame) bool {
		if frame.ID != p.MsgS2CCombatImpact {
			return false
		}
		hit := &pb.CombatImpact{}
		must(c.t, proto.Unmarshal(frame.Payload, hit))
		return hit.CastId == cast.CastId && hit.SkillId == skillID && hit.TargetHandle == cast.TargetHandle && hit.Outcome == 1
	})
	c.auto(false, 1)
	return cast
}
