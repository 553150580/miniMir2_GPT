// 本文件用默认首发候选和独立真实 MongoDB、WebSocket 核验职业技能的书店、学习、升阶与战斗。
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

// TestCombatM1DefaultSkillsMongoWS 覆盖两个职业的手动和定点自动，并核对诱惑与启示的真实场景同步。
func TestCombatM1DefaultSkillsMongoWS(t *testing.T) {
	if os.Getenv("MINIMIR_MONGO_E2E") != "1" || os.Getenv("MINIMIR_INTEGRATION_TEST_MONGO_URI") == "" {
		t.Skip("本用例必须使用独立 MongoDB 副本集与真实 WebSocket")
	}
	cfg, err := loadIntegrationConfig()
	must(t, err)
	if cfg.Fixture.RuleVersion < 29 {
		t.Skip("本组只覆盖已生成首发技能的规则29及后续版本")
	}
	cfg.Database = "minimir_dev_game_test_m1_" + domain.ID()
	cfg.AccountAddress, cfg.GameAddress, cfg.GMAddress = address(t), address(t), address(t)
	ctx, stop := context.WithCancel(context.Background())
	store, err := persistence.Open(ctx, cfg)
	must(t, err)
	var services sync.WaitGroup
	for _, mode := range []string{"account", "game"} {
		services.Go(func() {
			if err := bootstrap.Run(ctx, cfg, mode); err != nil && ctx.Err() == nil {
				t.Errorf("启动 %s 服失败：%v", mode, err)
			}
		})
	}
	t.Cleanup(func() {
		stop()
		services.Wait()
		cleanup, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		must(t, store.DB.Drop(cleanup))
		store.Close(cleanup)
	})
	healthy(t, cfg.AccountAddress)
	healthy(t, cfg.GameAddress)
	f := cfg.Fixture
	endpoint := "http://" + cfg.AccountAddress + "/rpc"
	joinRole := func(t *testing.T, job uint32) *client {
		t.Helper()
		login, role := &pb.LoginResult{}, &pb.RoleSummary{}
		rpc(t, endpoint, "", p.MsgC2SLocalRegister, &pb.LocalLoginRequest{Account: "m1_" + domain.ID()[:10], Password: "test_123456"}, login, 0)
		rpc(t, endpoint, login.AccountToken, p.MsgC2SCreateRole, &pb.CreateRoleRequest{OriginServerId: 1, Slot: 1, Job: job, Name: "验证" + domain.ID()[:6], OpId: domain.Bytes(domain.ID())}, role, 0)
		// 只准备等级、资金和书页前态。技能书从实际书店购买，已学技能和等级由正常业务写入。
		pages := domain.Item{ID: domain.ID(), ItemID: f.SkillEconomy.PageItemID, Quantity: 64, Version: 1, Bound: true, SourceKind: "book_recycle", SourceID: domain.ID()}
		_, err := store.DB.Collection("roles").UpdateOne(ctx, bson.M{"_id": domain.Hex(role.Key.RoleId)}, bson.M{
			"$set": bson.M{"level": 45, "wallet": domain.Wallet{Gold: 10000000, Yuanbao: 1000000, BoundYuanbao: 1000000}}, "$push": bson.M{"items": pages},
		})
		must(t, err)
		c := join(t, cfg.GameAddress, issue(t, endpoint, login.AccountToken, role.Key))
		t.Cleanup(c.close)
		c.auto(false, 1)
		return c
	}
	t.Run("战士购书升阶与手动自动半月", func(t *testing.T) {
		c := joinRole(t, 0)
		c.buyAndLearnSkill(f, 12)
		c.buyAndLearnSkill(f, 25)
		m1UpgradePurchasedSkill(t, c, f, 25, 1)
		c.send(p.MsgC2SSkillModeIntent, &pb.SkillModeIntent{SceneEpoch: c.entry.SceneEpoch, SkillId: 25, Enabled: true})
		c.modesUntil(func(state *pb.SkillModeState) bool { return modeEntry(state, 25).GetEnabled() })
		target := m1LiveMonster(t, c, 1, nil)
		origin := proto.Clone(c.player().Position).(*pb.GridPoint)
		direction := m1AdjacentDirection(c.player().Position, target.Position)
		c.send(p.MsgC2SAttackIntent, &pb.AttackIntent{SceneEpoch: c.entry.SceneEpoch, Direction: direction})
		readAttack := func() *pb.AttackAccepted {
			attack := &pb.AttackAccepted{}
			travelUntilWithin(c, 8*time.Second, func(frame wire.Frame) bool {
				if frame.ID != p.MsgS2CAttackAccepted {
					return false
				}
				must(t, proto.Unmarshal(frame.Payload, attack))
				return attack.SkillId == 25
			})
			return attack
		}
		if manual := readAttack(); manual.InputSequence == 0 || manual.SkillLevel != 1 {
			t.Fatalf("半月手动受理未使用已升阶技能：%v", manual)
		}
		c.auto(true, 2)
		if automatic := readAttack(); automatic.InputSequence != 0 || automatic.SkillLevel != 1 || !proto.Equal(c.player().Position, origin) {
			t.Fatalf("半月自动未使用已升阶技能或改变了挂机坐标：%v", automatic)
		}
	})
	t.Run("法师火球定点施法与诱惑宠物", func(t *testing.T) {
		c := joinRole(t, 1)
		c.buyAndLearnSkill(f, 1)
		m1UpgradePurchasedSkill(t, c, f, 1, 1)
		c.buyAndLearnSkill(f, 20)
		m1UpgradePurchasedSkill(t, c, f, 20, 3)
		fireball, _ := f.Skill(1)
		target := m1LiveMonster(t, c, fireball.RangeCells, nil)
		origin := proto.Clone(c.player().Position).(*pb.GridPoint)
		if manual := c.castAccepted(1, target.EntityHandle, nil); manual.InputSequence == 0 || manual.SkillLevel != 1 {
			t.Fatalf("火球手动受理未使用已升阶技能：%v", manual)
		}
		c.auto(true, 2)
		automatic := &pb.CastAccepted{}
		travelUntilWithin(c, 8*time.Second, func(frame wire.Frame) bool {
			if frame.ID != p.MsgS2CCastAccepted {
				return false
			}
			must(t, proto.Unmarshal(frame.Payload, automatic))
			return automatic.SkillId == 1 && automatic.InputSequence == 0
		})
		if automatic.SkillLevel != 1 || !proto.Equal(c.player().Position, origin) {
			t.Fatalf("火球自动未使用已升阶技能或改变了挂机坐标：%v", automatic)
		}
		c.auto(false, 3)
		// 已发出的自动火球会继续命中。先观察它结算和基线刷新，避免点中已死亡但仍在旧基线上的目标。
		c.combatImpact(1, automatic.TargetHandle)
		travelUntilWithin(c, 3*time.Second, func(frame wire.Frame) bool { return frame.ID == p.MsgS2CSceneBaseline })
		tame, _ := f.Skill(20)
		// 捕获本身随机，使用真实三阶成功路径并给冷却充足的重复尝试上限。
		wait := time.Duration(max(fireball.GlobalSpellMS, tame.ActionMS)+150) * time.Millisecond
		for attempt := 0; attempt < 48; attempt++ {
			time.Sleep(wait)
			target := m1LiveMonster(t, c, tame.RangeCells, func(entity *pb.EntityState) bool {
				for _, monster := range f.Monsters {
					if monster.DefinitionID == entity.DefinitionId && monster.CanTame {
						return true
					}
				}
				return false
			})
			c.castAccepted(20, target.EntityHandle, nil)
			if hit := c.combatImpact(20, target.EntityHandle); hit.Amount != 0 {
				t.Fatal("诱惑错误产生伤害", hit)
			}
			travelUntilWithin(c, 3*time.Second, func(frame wire.Frame) bool { return frame.ID == p.MsgS2CSceneBaseline })
			if pet := c.scene.OwnedPet; pet != nil {
				if pet.DefinitionId != target.DefinitionId || pet.EntityHandle == target.EntityHandle || c.entity(target.EntityHandle) != nil || c.entity(pet.EntityHandle).GetEntityKind() != 2 {
					t.Fatalf("捕获的原怪身份和主人宠物同步不一致：怪物=%v 宠物=%v", target, pet)
				}
				return
			}
			wait = time.Duration(max(tame.GlobalSpellMS, tame.CooldownBaseMS-tame.CooldownReductionMS*int64(tame.MaxLevel))+150) * time.Millisecond
		}
		t.Fatal("连续48次合法三阶诱惑仍未观察到活宠")
	})
	t.Run("道士普通怪物生命启示", func(t *testing.T) {
		c := joinRole(t, 2)
		c.buyAndLearnSkill(f, 28)
		m1UpgradePurchasedSkill(t, c, f, 28, 3)
		skill, _ := f.Skill(28)
		target := m1LiveMonster(t, c, skill.RangeCells, nil)
		accepted := c.castAccepted(28, target.EntityHandle, nil)
		if hit := c.combatImpact(28, target.EntityHandle); hit.Outcome != 1 || hit.Amount != 0 || hit.CastId != accepted.CastId {
			t.Fatal("三阶心灵启示没有在原怪物上成功使用", hit)
		}
		travelUntilWithin(c, 3*time.Second, func(frame wire.Frame) bool {
			if frame.ID != p.MsgS2CSceneBaseline {
				return false
			}
			for _, state := range c.entity(target.EntityHandle).GetStatuses() {
				if state.StatusId == 28 && state.SourceSkillId == 28 && state.SkillLevel == 3 && state.RemainingMs > 0 {
					return true
				}
			}
			return false
		})
	})
}

// m1UpgradePurchasedSkill 逐阶购买所需书本，用广播中实际实例、数量和版本升级。
func m1UpgradePurchasedSkill(t *testing.T, c *client, f devconfig.Fixture, id, last uint32) {
	t.Helper()
	for level := uint32(1); level <= last; level++ {
		row, ok := f.Upgrade(id, level)
		if !ok {
			t.Fatalf("技能 %d 缺少第 %d 阶报价", id, level)
		}
		need := map[uint32]int64{row.PageItemID: row.PageCost}
		for _, book := range row.BookCosts {
			for _, offer := range f.SkillShop {
				if offer.ItemID != book.ItemID || offer.Currency != "gold" {
					continue
				}
				purchase := c.skillPreview(&pb.SkillAssetPreviewRequest{Kind: 1, OfferId: offer.ID, Quantity: book.Quantity})
				if result := c.skillConfirm(purchase); result.ErrorCode != 0 {
					t.Fatalf("书店升阶材料购买失败：%v", result)
				}
				break
			}
			need[book.ItemID] += book.Quantity
		}
		request := &pb.SkillAssetPreviewRequest{Kind: 2, SkillId: id}
		for _, item := range c.inventory.Items {
			n := min(need[item.ItemId], item.Quantity)
			if n > 0 {
				request.Materials = append(request.Materials, &pb.ItemConsumption{InstanceId: item.InstanceId, Version: item.Version, Quantity: n})
				need[item.ItemId] -= n
			}
		}
		preview := c.skillPreview(request)
		if preview.FromLevel != level-1 || preview.ToLevel != level {
			t.Fatalf("技能 %d 学习、逐阶报价结果不符：%v", id, preview)
		}
		if result := c.skillConfirm(preview); result.ErrorCode != 0 {
			t.Fatalf("技能 %d 正常升到第 %d 阶失败：%v", id, level, result)
		}
	}
}

// m1LiveMonster 从真实同步中选择射程内的活怪，不固定随机出生点或覆盖默认怪物强度。
func m1LiveMonster(t *testing.T, c *client, maxDistance int, valid func(*pb.EntityState) bool) *pb.EntityState {
	t.Helper()
	find := func() *pb.EntityState {
		origin := c.player().Position
		for _, monster := range c.scene.Entities {
			point := monster.Position
			if monster.EntityKind == 3 && monster.Hp > 0 && point != nil && max(max(point.X-origin.X, origin.X-point.X), max(point.Y-origin.Y, origin.Y-point.Y)) <= int32(maxDistance) && (valid == nil || valid(monster)) {
				return monster
			}
		}
		return nil
	}
	if target := find(); target != nil {
		return target
	}
	var target *pb.EntityState
	travelUntilWithin(c, 8*time.Second, func(frame wire.Frame) bool {
		if frame.ID == p.MsgS2CSceneBaseline {
			target = find()
		}
		return target != nil
	})
	return target
}

// m1AdjacentDirection 使用服务器八方向编号从相邻目标位置生成普攻方向。
func m1AdjacentDirection(origin, target *pb.GridPoint) uint32 {
	deltaX, deltaY := target.X-origin.X, target.Y-origin.Y
	for index, step := range [8][2]int32{{0, -1}, {1, -1}, {1, 0}, {1, 1}, {0, 1}, {-1, 1}, {-1, 0}, {-1, -1}} {
		if step[0] == deltaX && step[1] == deltaY {
			return uint32(index)
		}
	}
	return 0
}
