//go:build m1_natural_upgrade

// 本文件用独立真实联机链路验证法师从正常创建到学习、升阶和捕获的完整路径。
package integration

import (
	"context"
	"slices"
	"strconv"
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

// TestLevelGrowthNaturalM1TameMongoWS 用两个独立零资产法师检验真实掉落、购书和捕获的可重复性。
func TestLevelGrowthNaturalM1TameMongoWS(t *testing.T) {
	cfg, ctx, store := m1FieldServices(t, m1FieldConfig(t))
	fixture := &cfg.Fixture
	skill, known := fixture.Skill(20)
	if !known || skill.JobID != 1 || skill.AutoEnabled || skill.LearnedLevel != 0 || skill.Behavior != "electric_shock" {
		t.Fatal("现行规则缺少法师手动诱惑之光")
	}
	row, known := fixture.Upgrade(skill.ID, 1)
	if !known || len(row.BookCosts) != 1 || row.BookCosts[0].ItemID != skill.BookItemID || row.PageCost <= 0 {
		t.Fatal("现行规则缺少诱惑之光一阶材料")
	}
	books := row.BookCosts[0].Quantity
	var offer devconfig.SkillShop
	for _, candidate := range fixture.SkillShop {
		if candidate.ItemID == skill.BookItemID && candidate.Currency == "gold" && candidate.Quantity == 1 && candidate.MaxPurchaseQuantity >= books {
			offer = candidate
			break
		}
	}
	if offer.ID == 0 || offer.PriceUnits <= 0 || len(fixture.TownTravel.DestinationMapIDs) == 0 {
		t.Fatal("现行规则缺少金币书店报价或正常城镇入口")
	}
	price := (books + 1) * offer.PriceUnits
	for _, label := range []string{"法师一", "法师二"} {
		t.Run(label, func(t *testing.T) {
			t.Parallel()
			endpoint, token, key := m1FieldRole(t, cfg, skill.JobID)
			roleID := domain.Hex(key.RoleId)
			stored := func() domain.Role {
				t.Helper()
				role, err := store.Role(ctx, roleID)
				must(t, err)
				return role
			}
			created := stored()
			if created.Level != 1 || created.Experience != 0 || created.SourceSeq != 0 || len(created.Skills) != 0 ||
				created.Wallet != (domain.Wallet{}) || len(created.Items) != len(fixture.InitialInventory)+1 {
				t.Fatal("角色应只有正常建角初始物品和零钱包")
			}
			started := time.Now()
			var kills, deaths int
			c := join(t, cfg.GameAddress, issue(t, endpoint, token, key), func(_ *client, frame wire.Frame) {
				if frame.ID == p.MsgS2CExperienceGrantResult && frame.Error == 0 {
					var grant pb.ExperienceGrantResult
					must(t, proto.Unmarshal(frame.Payload, &grant))
					if !grant.Migrated && grant.AcceptedExperience > 0 {
						kills++
					}
				}
			})
			c.stop()
			c.ctx, c.stop = context.WithTimeout(ctx, 24*time.Minute)
			t.Cleanup(c.close)
			t.Cleanup(func() {
				if t.Failed() {
					role := stored()
					t.Logf("自然诱惑停点：等级%d，经验%d，绑定金币%d，累计%d杀，背包%d/%d，用时%s",
						role.Level, role.Experience, role.Wallet.BoundGold, kills, len(role.Items), role.Capacity, time.Since(started).Round(time.Second))
				}
			})
			if c.entry.RoomId != fixture.Map.ID || c.entry.ResourceVersion != fixture.RuleVersion || c.scene.OwnedPet != nil {
				t.Fatal("正常新角应进入现行默认普通图，初始无宠")
			}
			c.auto(false, 1)
			equipNaturalUpgradeStarter(c, fixture)
			spare := naturalGrowthItem(c, 218, "")
			if spare == nil || spare.SourceKind != "" {
				t.Fatal("正常建角备用木剑缺失")
			}
			locked := c.asset(p.MsgC2SSetItemLock, &pb.SetItemLockRequest{ItemInstanceId: spare.InstanceId,
				ItemVersion: spare.Version, Locked: true, Operation: c.operation()})
			if locked.ErrorCode != 0 {
				t.Fatalf("锁定原赠木剑失败：%v", locked)
			}
			c.inventoryAt(locked.NewRevision)
			policy := &pb.AutoRecyclePolicy{Enabled: true, QualityIds: []uint32{0, 1, 2, 3, 4}}
			result := c.asset(p.MsgC2SSetAutoRecycle, &pb.SetAutoRecycleRequest{Policy: policy, Operation: c.operation()})
			if result.ErrorCode != 0 {
				t.Fatalf("开启普通自动回收策略失败：%v", result)
			}
			c.inventoryAt(result.NewRevision)
			starter, known := fixture.Skill(1)
			if !known || starter.JobID != skill.JobID {
				t.Fatal("缺少自然成长使用的法师初始火球术")
			}
			c.auto(true, 4)
			awaitNaturalEquipmentFunds(t, c, stored, starter.LearnLevel, 0, 4, &kills, &deaths)
			c.auto(false, 4)
			naturalEquipmentLearnStarter(t, c, fixture, skill.JobID)
			c.auto(true, 8)
			level := max(skill.LearnLevel, row.RequiredRoleLevel)
			role := awaitNaturalEquipmentFunds(t, c, stored, level, price, 8, &kills, &deaths)
			drop, sourceRule := naturalUpgradeDroppedBook(role, fixture, row.PageCost)
			for drop.ID == "" || naturalGrowthItem(c, 36, "monster_drop") == nil {
				if time.Since(started) >= 17*time.Minute {
					t.Fatal("正常成长在观察窗内未同时提供可拆掉书和自然蓝药")
				}
				awaitNaturalEquipmentFunds(t, c, stored, role.Level, role.Wallet.BoundGold+1, 8, &kills, &deaths)
				role = stored()
				drop, sourceRule = naturalUpgradeDroppedBook(role, fixture, row.PageCost)
			}
			c.auto(false, 1)
			// 等待停手前已发出的火球与资产结算结束，再从真实战斗账本核对首笔购书资金。
			time.Sleep(4 * time.Second)
			c.expectError(c.send(p.MsgC2SMoveIntent, &pb.MoveIntent{SceneEpoch: c.entry.SceneEpoch, Direction: 8}), p.CodeInvalidArgument)
			before := stored()
			if direct, recycled := naturalUpgradeIncome(t, ctx, store, before, drop, fixture.Map.ID); direct+recycled != before.Wallet.Gold+before.Wallet.BoundGold {
				t.Fatalf("购书资金与自然直接金币%d＋回收金币%d不符：%+v", direct, recycled, before.Wallet)
			}
			t.Logf("自然材料就绪：等级%d，%d杀，%d次复活，金币%d，蓝药与掉书已入包，用时%s",
				before.Level, kills, deaths, before.Wallet.Gold+before.Wallet.BoundGold, time.Since(started).Round(time.Second))
			c.buyAndLearnSkill(*fixture, skill.ID)
			pages := c.skillPreview(&pb.SkillAssetPreviewRequest{Kind: 3,
				Materials: []*pb.ItemConsumption{{InstanceId: domain.Bytes(drop.ID), Version: uint64(drop.Version), Quantity: 1}}})
			if len(pages.Outputs) != 1 || pages.Outputs[0].ItemId != row.PageItemID || pages.Outputs[0].Quantity != sourceRule.PagesPerBook || !pages.Outputs[0].Bound {
				t.Fatalf("真实掉书的拆页报价错误：%v", pages)
			}
			pageCommit := &pb.SkillAssetCommitRequest{PreviewId: pages.PreviewId, Operation: c.operation()}
			if got := c.skillCommit(pageCommit); got.ErrorCode != 0 {
				t.Fatalf("正常拆页失败：%v", got)
			}
			purchase := c.skillPreview(&pb.SkillAssetPreviewRequest{Kind: 1, OfferId: offer.ID, Quantity: books})
			if purchase.Spend.GetGold()+purchase.Spend.GetBoundGold() != offer.PriceUnits*books {
				t.Fatalf("一阶额外书本报价错误：%v", purchase)
			}
			bookCommit := &pb.SkillAssetCommitRequest{PreviewId: purchase.PreviewId, Operation: c.operation()}
			if got := c.skillCommit(bookCommit); got.ErrorCode != 0 {
				t.Fatalf("普通书店购买一阶材料失败：%v", got)
			}
			materials := naturalUpgradeMaterials(c, skill.BookItemID, books, row.PageItemID, row.PageCost, bookCommit, pageCommit)
			quote := c.skillPreview(&pb.SkillAssetPreviewRequest{Kind: 2, SkillId: skill.ID, Materials: materials})
			if quote.FromLevel != 0 || quote.ToLevel != 1 {
				t.Fatalf("一阶报价错误：%v", quote)
			}
			upgrade := &pb.SkillAssetCommitRequest{PreviewId: quote.PreviewId, Operation: c.operation()}
			if got := c.skillCommit(upgrade); got.ErrorCode != 0 {
				t.Fatalf("自然一阶提交失败：%v", got)
			}
			upgraded := stored()
			if rank, learned := upgraded.Skill(skill.ID); !learned || rank != 1 ||
				before.Wallet.Gold+before.Wallet.BoundGold-upgraded.Wallet.Gold-upgraded.Wallet.BoundGold != price {
				t.Fatal("正常学习与一阶升阶未对应实际书费与技能等级")
			}
			if replay := c.skillCommit(upgrade); replay.ErrorCode != 0 || stored().Revision != upgraded.Revision {
				t.Fatal("一阶同操作重放发生重复扣耗")
			}

			pet, attempts, blueUses := naturalTameCombat(t, ctx, store, c, fixture, skill, roleID)
			town := fixture.TownTravel.DestinationMapIDs[0]
			travelToBaseline(c, 1, town)
			travelAcknowledge(c, true)
			travelToBaseline(c, 2, fixture.Map.ID)
			travelAcknowledge(c, false)
			assertNaturalTamedPet(t, c, pet)
			c.close()
			reconnected := join(t, cfg.GameAddress, issue(t, endpoint, token, key))
			t.Cleanup(reconnected.close)
			assertNaturalTamedPet(t, reconnected, pet)
			if reconnected.entry.RoomId != fixture.Map.ID || !slices.ContainsFunc(reconnected.inventory.LearnedSkills, func(row *pb.LearnedSkill) bool {
				return row.SkillId == skill.ID && row.Level == 1
			}) {
				t.Fatal("健康重连未恢复原普通图和已学习的一阶技能")
			}
			t.Logf("自然诱惑完成：规则%d，金币花费%d，%d本原书＋%d张原掉书页，%d次合法施放，%d次自动补蓝，同一活宠%d/%d，用时%s",
				fixture.RuleVersion, price, books+1, row.PageCost, attempts, blueUses, pet.EntityHandle, pet.Life, time.Since(started).Round(time.Millisecond))
		})
	}
}

// naturalTameCombat 仅使用原始 RNG、真实冷却和普通背包，通过命中与场景两路核对捕获。
func naturalTameCombat(t *testing.T, ctx context.Context, store *persistence.Store, c *client, fixture *devconfig.Fixture, skill devconfig.Skill, roleID string) (*pb.OwnedPetState, int, int) {
	t.Helper()
	// 只保留一个活怪，避免捕获成功后的宠物自主攻击混入本次零伤害及零收益验证。
	if travelCount(c.scene, 3) != 1 {
		travelUntilWithin(c, 12*time.Second, func(frame wire.Frame) bool {
			return frame.ID == p.MsgS2CSceneBaseline && travelCount(c.scene, 3) == 1
		})
	}
	filter := bson.M{"role_id": roleID, "kind": "settlement", "kills": 1}
	killsBefore, err := store.DB.Collection("audit_outbox").CountDocuments(ctx, filter)
	must(t, err)
	cooldown := time.Duration(max(skill.ActionMS, skill.GlobalSpellMS, skill.CooldownBaseMS-skill.CooldownReductionMS))*time.Millisecond + 150*time.Millisecond
	blueUses := 0
	for attempt := 1; attempt <= 160; attempt++ {
		target := m1LiveMonster(t, c, skill.RangeCells, func(entity *pb.EntityState) bool {
			return slices.ContainsFunc(fixture.Monsters, func(definition devconfig.Monster) bool {
				return definition.DefinitionID == entity.DefinitionId && definition.CanTame
			})
		})
		accepted, code := naturalTameCast(c, skill.ID, target)
		if code == p.CodeSkillResource {
			verifyNaturalTameMana(t, ctx, store, c, roleID)
			blueUses++
			accepted, code = naturalTameCast(c, skill.ID, target)
		}
		if code != 0 || accepted.SkillLevel != 1 || accepted.InputSequence == 0 || accepted.TargetLife != target.Life {
			t.Fatalf("第%d次一阶手动捕获未受理原目标：错误码%d，%v", attempt, code, accepted)
		}
		hit := c.combatImpact(skill.ID, target.EntityHandle)
		if hit.CastId != accepted.CastId || hit.Amount != 0 || hit.Outcome == 3 {
			t.Fatalf("诱惑造成伤害、打到旧生命或串入别次事件：%v", hit)
		}
		travelUntilWithin(c, 4*time.Second, func(frame wire.Frame) bool { return frame.ID == p.MsgS2CSceneBaseline })
		if pet := c.scene.OwnedPet; pet != nil {
			if hit.Outcome != 1 || pet.DefinitionId != target.DefinitionId || pet.EntityHandle == target.EntityHandle || c.entity(target.EntityHandle) != nil {
				t.Fatalf("原怪没有被准确替换为同模板宠物：原怪%v，现宠%v", target, pet)
			}
			assertNaturalTamedPet(t, c, pet)
			killsAfter, err := store.DB.Collection("audit_outbox").CountDocuments(ctx, filter)
			must(t, err)
			if killsAfter != killsBefore {
				t.Fatal("捕获过程产生普通击杀收益")
			}
			return pet, attempt, blueUses
		}
		time.Sleep(cooldown)
	}
	t.Fatal("160次合法一阶诱惑未观察到成功，保留真实随机结果")
	return nil, 0, 0
}

// naturalTameCast 读取同一手动施放的接受或拒绝，保留缺蓝回执供正常补给后重试。
func naturalTameCast(c *client, skillID uint32, target *pb.EntityState) (*pb.CastAccepted, uint32) {
	c.t.Helper()
	request := c.send(p.MsgC2SCastIntent, &pb.CastIntent{SceneEpoch: c.entry.SceneEpoch, SkillId: skillID, TargetHandle: target.EntityHandle})
	for {
		frame := c.readFrame()
		if frame.ID == p.MsgC2SCastIntent && frame.Request == request && frame.Error != 0 {
			return nil, frame.Error
		}
		if frame.Error != 0 {
			c.t.Fatalf("施放期间出现非预期的联机拒绝：%v", frame)
		}
		if frame.ID == p.MsgS2CCastAccepted {
			event := &pb.CastAccepted{}
			must(c.t, proto.Unmarshal(frame.Payload, event))
			if event.SkillId == skillID {
				return event, 0
			}
		}
	}
}

// verifyNaturalTameMana 对账缺蓝后真正扣除的合法蓝药实例、原场景和自动用药来源序号。
func verifyNaturalTameMana(t *testing.T, ctx context.Context, store *persistence.Store, c *client, roleID string) {
	t.Helper()
	before := c.inventory
	if !slices.ContainsFunc(before.Items, func(item *pb.ItemView) bool { return item.ItemId == 36 && item.Quantity > 0 }) {
		t.Fatal("自然背包蓝药已耗尽，无法继续正常捕获")
	}
	travelUntilWithin(c, 4*time.Second, func(frame wire.Frame) bool {
		return frame.ID == p.MsgS2CInventory && c.inventory.Revision > before.Revision
	})
	var receipt struct {
		ItemInstanceID string                    `bson:"item_instance_id"`
		Potion         persistence.PotionReceipt `bson:"potion"`
		SourceSeq      int64                     `bson:"source_seq"`
	}
	must(t, store.DB.Collection("audit_outbox").FindOne(ctx, bson.M{
		"role_id": roleID, "revision": c.inventory.Revision, "kind": persistence.AssetAutoUsePotion, "result_code": 0,
	}).Decode(&receipt))
	index := slices.IndexFunc(before.Items, func(item *pb.ItemView) bool { return domain.Hex(item.InstanceId) == receipt.ItemInstanceID })
	if index < 0 || receipt.Potion.ItemID != before.Items[index].ItemId || receipt.Potion.HP != 0 || receipt.Potion.MP <= 0 ||
		receipt.Potion.Scene != strconv.FormatUint(c.entry.SceneEpoch, 10) || receipt.SourceSeq <= 0 {
		t.Fatalf("自动蓝药回执未关联捕获场景的原库存：%+v", receipt)
	}
	item := before.Items[index]
	remaining := int64(0)
	for _, current := range c.inventory.Items {
		if domain.Hex(current.InstanceId) == receipt.ItemInstanceID {
			remaining = current.Quantity
		}
	}
	if remaining != item.Quantity-1 || item.SourceKind != "monster_drop" && item.SourceKind != "initial_grant" && item.SourceKind != "" {
		t.Fatalf("自动用药没有恰好扣除一瓶原蓝药：%v，剩余%d", item, remaining)
	}
	travelUntilWithin(c, 3*time.Second, func(frame wire.Frame) bool {
		return frame.ID == p.MsgS2CSceneBaseline && c.player().Mp >= 4
	})
	t.Logf("真实缺蓝自动用药：原实例%s，来路%s/%s，来源序号%d，库存%d→%d，观察MP%d",
		receipt.ItemInstanceID, item.SourceKind, item.SourceId, receipt.SourceSeq, item.Quantity, remaining, c.player().Mp)
}

// assertNaturalTamedPet 对同一角色在普通图中的单只活宠核对模板、出生代次和实体句柄。
func assertNaturalTamedPet(t *testing.T, c *client, original *pb.OwnedPetState) {
	t.Helper()
	pet := c.scene.OwnedPet
	if pet == nil || pet.Hp <= 0 || pet.DefinitionId != original.DefinitionId || pet.EntityHandle != original.EntityHandle || pet.Life != original.Life ||
		c.entity(pet.EntityHandle).GetEntityKind() != 2 || travelCount(c.scene, 2) != 1 {
		t.Fatalf("场景没有保留同一单槽活宠：原宠%v，现宠%v", original, pet)
	}
}
