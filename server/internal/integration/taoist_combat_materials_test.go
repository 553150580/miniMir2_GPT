// 本文件通过真实 MongoDB 和 WebSocket 验证道士弹道、毒粉与围困的原材料扣除和幂等重放。
package integration

import (
	"bytes"
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

// TestTaoistCombatMaterialsMongoWS 在隔离服实测灵魂火符、绿毒、错材料拒绝与成功回执重放。
func TestTaoistCombatMaterialsMongoWS(t *testing.T) {
	if os.Getenv("MINIMIR_MONGO_E2E") != "1" {
		t.Skip("道士材料WS用例要求专用 MongoDB 副本集")
	}
	// cfg 仍使用本任务专属副本集，真实服务只访问本用例随机分配的子数据库。
	cfg, err := loadIntegrationConfig()
	must(t, err)
	cfg.Database = "minimir_dev_game_test_taoist_" + domain.ID()
	cfg.AccountAddress, cfg.GameAddress, cfg.GMAddress = address(t), address(t), address(t)
	// fixture 显式开启用例材料和技能，不推断正式材料或修改唯一工作簿。
	fixture := &cfg.Fixture
	fixture.CastingSupply.Enabled = true
	fixture.CastingSupply.ItemIDs = append(fixture.CastingSupply.ItemIDs, 181, 182)
	fixture.Items = append(fixture.Items, devconfig.Item{ID: 181, EquipmentSlot: 100, Stackable: true, MaxStack: 3},
		devconfig.Item{ID: 182, EquipmentSlot: 100, Stackable: true, MaxStack: 3})
	for i := range fixture.Items {
		if fixture.Items[i].ID == 187 {
			fixture.Items[i].MaxStack = 3
		}
	}
	// base 复用法术的普通动作和时钟，再分别设置道术弹道与两色药粉规则。
	base, _ := fixture.Skill(1)
	base.JobID, base.AutoEnabled, base.MPLevelCost, base.MaterialQuantity = 2, false, 1, 1
	base.TargetKind, base.DelayBaseMS, base.RangeCells = "enemy_entity", 500, 9
	soul, poison, trap := base, base, base
	soul.ID, soul.Behavior, soul.MaterialItemID, soul.MPBaseCost = 13, "soul_fire_ball", 187, 3
	soul.PowerBase, soul.MPowerBase, soul.PowerBonus, soul.MPowerBonus = 3, 8, 0, 0
	poison.ID, poison.Behavior, poison.MaterialItemID, poison.MPBaseCost = 6, "poisoning", 0, 2
	poison.RequireLineOfSight, poison.ProjectileMSPerCell = false, 0
	poison.Poison = &devconfig.PoisonMaterials{GreenItemIDs: []uint32{181}, RedItemIDs: []uint32{182}}
	trap.ID, trap.Behavior, trap.MaterialItemID, trap.MPBaseCost, trap.MPLevelCost = 16, devconfig.TrapHexagonBehavior, 187, 7, 3
	trap.TargetKind, trap.RequireLineOfSight, trap.ProjectileMSPerCell, trap.AreaRadius = "enemy_grid", false, 0, 1
	fixture.Skills = append(fixture.Skills, soul, poison, trap)
	must(t, fixture.ValidateCombat())
	must(t, fixture.ValidateBuffs())
	for i := range fixture.Monsters {
		monster := &fixture.Monsters[i]
		monster.AttackMin, monster.AttackMax, monster.MoveMS, monster.AttackMS = 0, 0, 600000, 600000
		monster.MagicDefenceMin, monster.MagicDefenceMax, monster.MagicResist = 0, 0, 0
		monster.HP = 2000
	}
	// ctx 管理专属服务生命周期，store 独立查看已持久化的材料事实。
	ctx, stop := context.WithCancel(context.Background())
	store, err := persistence.Open(ctx, cfg)
	must(t, err)
	var servers sync.WaitGroup
	for _, mode := range []string{"account", "game"} {
		servers.Go(func() {
			if err := bootstrap.Run(ctx, cfg, mode); err != nil && ctx.Err() == nil {
				t.Errorf("%s启动失败: %v", mode, err)
			}
		})
	}
	t.Cleanup(func() {
		stop()
		servers.Wait()
		cleanup, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		must(t, store.DB.Drop(cleanup))
		store.Close(cleanup)
	})
	healthy(t, cfg.AccountAddress)
	healthy(t, cfg.GameAddress)
	// login、role 是通过原认证入口建立的隔离道士。
	endpoint := "http://" + cfg.AccountAddress + "/rpc"
	login, role := &pb.LoginResult{}, &pb.RoleSummary{}
	rpc(t, endpoint, "", p.MsgC2SLocalRegister, &pb.LocalLoginRequest{Account: "tao_" + domain.ID()[:9], Password: "test_123456"}, login, 0)
	rpc(t, endpoint, login.AccountToken, p.MsgC2SCreateRole, &pb.CreateRoleRequest{OriginServerId: 1, Slot: 1, Job: 2, Name: "道术" + domain.ID()[:6], OpId: domain.Bytes(domain.ID())}, role, 0)
	roleID := domain.Hex(role.Key.RoleId)
	// amulet 和 powder 是明确发放的前态，后续装槽、校验、消耗和重试都走真实协议。
	amulet := domain.Item{ID: domain.ID(), ItemID: 187, Quantity: 2, Version: 1, Bound: true, SourceKind: "development_fixture", SourceID: "explicit_count"}
	powder := domain.Item{ID: domain.ID(), ItemID: 181, Quantity: 2, Version: 1, Bound: true, SourceKind: "development_fixture", SourceID: "explicit_count"}
	_, err = store.DB.Collection("roles").UpdateOne(ctx, bson.M{"_id": roleID}, bson.M{
		"$set":  bson.M{"level": 30, "learned_skills": []domain.LearnedSkill{{SkillID: 13}, {SkillID: 6}, {SkillID: 16}}},
		"$push": bson.M{"items": bson.M{"$each": []domain.Item{amulet, powder}}},
	})
	must(t, err)
	c := join(t, cfg.GameAddress, issue(t, endpoint, login.AccountToken, role.Key))
	defer func() { c.close() }()
	c.auto(false, 1)
	// target 是真实地图同步内仍存活的敌人句柄。
	target := uint32(10)
	if c.entity(target) == nil {
		t.Fatal("候选无可施法怪物")
	}
	equip := func(item domain.Item) *pb.ItemView {
		result := c.asset(p.MsgC2SSetCastingSupply, &pb.SetCastingSupplyRequest{ItemInstanceId: domain.Bytes(item.ID), ItemVersion: uint64(item.Version), Operation: c.operation()})
		if result.ErrorCode != 0 {
			t.Fatalf("装入原材料 %d 失败: %+v", item.ItemID, result)
		}
		c.inventoryAt(result.NewRevision)
		return summonWSEquipped(t, c)
	}
	cast := func(skill uint32, item *pb.ItemView) *pb.CastWithMaterialRequest {
		return &pb.CastWithMaterialRequest{SceneEpoch: c.entry.SceneEpoch, CasterLife: c.player().Life, TargetHandle: target,
			SkillId: skill, Direction: 2, MaterialInstanceId: item.InstanceId, MaterialVersion: item.Version, Operation: c.operation()}
	}
	impact := func(skill uint32) {
		travelUntilWithin(c, 4*time.Second, func(frame wire.Frame) bool {
			if frame.ID != p.MsgS2CCombatImpact {
				return false
			}
			hit := &pb.CombatImpact{}
			must(t, proto.Unmarshal(frame.Payload, hit))
			return hit.SkillId == skill && hit.TargetHandle == target && hit.Outcome == 1 && hit.Amount > 0
		})
	}
	// wrong 使用真实符纸尝试施毒，应持久化拒绝且保留原耗材单位。
	selected := equip(amulet)
	wrong := c.asset(p.MsgC2SCastWithMaterial, cast(6, selected))
	c.inventoryAt(wrong.NewRevision)
	if wrong.ErrorCode != p.CodeSkillMaterialRequired || summonWSEquipped(t, c).Quantity != 2 {
		t.Fatalf("施毒接受符纸或误扣材料: 回执 %v，剩余 %d", wrong, summonWSEquipped(t, c).Quantity)
	}
	// request 记录原灵魂火符操作，之后以相同内容重放。
	request := cast(13, selected)
	fired := c.asset(p.MsgC2SCastWithMaterial, request)
	c.inventoryAt(fired.NewRevision)
	if fired.ErrorCode != 0 || summonWSEquipped(t, c).Quantity != 1 {
		t.Fatal("灵魂火符未恰好消费一个原材料单位", fired)
	}
	impact(13)
	// 等待附着动作冷却，再换到明确发放的原灰粉。
	time.Sleep(2 * time.Second)
	selected = equip(powder)
	poisoned := c.asset(p.MsgC2SCastWithMaterial, cast(6, selected))
	c.inventoryAt(poisoned.NewRevision)
	if poisoned.ErrorCode != 0 || summonWSEquipped(t, c).Quantity != 1 {
		t.Fatal("灰粉未恰好消费一个原材料单位", poisoned)
	}
	impact(6)
	// replay 在另一种粉装槽后仍查回原成功回执，不得扣除当前药粉或重用原弹道。
	castSequence := c.player().CastSequence
	replay := c.asset(p.MsgC2SCastWithMaterial, proto.Clone(request).(*pb.CastWithMaterialRequest))
	c.inventoryAt(replay.NewRevision)
	travelUntilWithin(c, 4*time.Second, func(frame wire.Frame) bool { return frame.ID == p.MsgS2CSceneBaseline })
	if replay.ErrorCode != 0 || !bytes.Equal(replay.OpId, request.Operation.OpId) || replay.CommandSeq != poisoned.CommandSeq ||
		summonWSEquipped(t, c).Quantity != 1 || c.player().CastSequence != castSequence {
		t.Fatal("原护符操作没有幂等重查", replay)
	}
	// original 验证持久回执仍是首次成功的消耗，响应业务水位已经推进到最近操作。
	var original persistence.Operation
	must(t, store.DB.Collection("operations").FindOne(ctx, bson.M{"_id": "asset:" + roleID + ":" + domain.Hex(request.Operation.OpId)}).Decode(&original))
	if original.CommandSeq < 0 || uint64(original.CommandSeq) != fired.CommandSeq || original.Casting == nil || original.Casting.InstanceID != amulet.ID || original.Casting.Quantity != 1 {
		t.Fatalf("原材料扣除回执错配: %+v", original)
	}
	// 困魔咒从当前背包找回同一个原护符并使用明确的地格，不信任旧物品版本或旧目标坐标。
	time.Sleep(2 * time.Second)
	for _, item := range c.inventory.Items {
		if bytes.Equal(item.InstanceId, domain.Bytes(amulet.ID)) {
			selected = equip(domain.Item{ID: amulet.ID, ItemID: amulet.ItemID, Version: int64(item.Version)})
			break
		}
	}
	if selected.ItemId != amulet.ItemID || selected.Quantity != 1 || c.entity(target) == nil {
		t.Fatal("原护符或围困目标没有从实际背包与场景恢复")
	}
	trapRequest := cast(16, selected)
	trapRequest.TargetHandle, trapRequest.TargetGrid = 0, proto.Clone(c.entity(target).Position).(*pb.GridPoint)
	trapped := c.asset(p.MsgC2SCastWithMaterial, trapRequest)
	c.inventoryAt(trapped.NewRevision)
	if trapped.ErrorCode != 0 {
		t.Fatal("真实扣料未批准原地格围困", trapped)
	}
	travelUntilWithin(c, 4*time.Second, func(frame wire.Frame) bool {
		if frame.ID != p.MsgS2CCombatImpact {
			return false
		}
		hit := &pb.CombatImpact{}
		must(t, proto.Unmarshal(frame.Payload, hit))
		return hit.SkillId == 16 && hit.TargetHandle == target && hit.Outcome == 1 && hit.Amount == 0
	})
	trapReplay := c.asset(p.MsgC2SCastWithMaterial, proto.Clone(trapRequest).(*pb.CastWithMaterialRequest))
	if trapReplay.ErrorCode != 0 || !bytes.Equal(trapReplay.OpId, trapRequest.Operation.OpId) || trapReplay.CommandSeq != trapped.CommandSeq || trapReplay.NewRevision != trapped.NewRevision {
		t.Fatal("围困重放更改了原确认结果", trapReplay)
	}
	var trapReceipt persistence.Operation
	must(t, store.DB.Collection("operations").FindOne(ctx, bson.M{"_id": "asset:" + roleID + ":" + domain.Hex(trapRequest.Operation.OpId)}).Decode(&trapReceipt))
	if trapReceipt.Casting == nil || trapReceipt.Casting.SkillID != 16 || trapReceipt.Casting.InstanceID != amulet.ID || trapReceipt.Casting.Quantity != 1 {
		t.Fatalf("原围困耗材没有唯一落账：%+v", trapReceipt)
	}
	// receipts 是数据库中成功扣料的真实回执，三次正向操作各有且仅有一个。
	count, err := store.DB.Collection("operations").CountDocuments(ctx, bson.M{"role_id": roleID, "kind": "cast_material", "casting_use": bson.M{"$exists": true}})
	must(t, err)
	if count != 3 {
		t.Fatalf("成功材料扣除记录数 %d", count)
	}
}
