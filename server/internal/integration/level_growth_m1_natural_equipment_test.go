//go:build m1_natural_upgrade

// 本文件以真实新角实测三职业自然获得并穿戴现行装备。只读原配置和审计，使用隔离数据库。
package integration

import (
	"context"
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

// naturalEquipmentCase 指定每职业使用的既有物品和正常产出地图。
type naturalEquipmentCase struct {
	name   string // 职业和装备的观察名。
	job    uint32 // 建角使用的职业。
	itemID uint32 // 现行目录中实际购买的模板。
	mapID  uint64 // 获得足够主动战斗能力后前往的现行普通图。
}

// TestLevelGrowthNaturalM1EquipmentMongoWS 从真实零金币创角跑到购买、属性生效和新票恢复。
func TestLevelGrowthNaturalM1EquipmentMongoWS(t *testing.T) {
	cfg, ctx, store := m1FieldServices(t, m1FieldConfig(t))
	fixture := &cfg.Fixture
	for _, test := range []naturalEquipmentCase{{"战士青铜剑", 0, 223, 1}, {"法师乌木剑", 1, 220, 6}, {"道士白色虎齿项链", 2, 261, 6}} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			definition, known := fixture.Item(test.itemID)
			if !known || !definition.IsEquipment || !definition.EquipmentStatsKnown() || definition.RequiredLevel == nil {
				t.Fatal("当前候选缺少已验证的装备定义")
			}
			town, npc, offer := naturalEquipmentVendor(t, fixture, test.itemID)
			endpoint, token, key := m1FieldRole(t, cfg, test.job)
			roleID := domain.Hex(key.RoleId)
			stored := func() domain.Role {
				t.Helper()
				role, err := store.Role(ctx, roleID)
				must(t, err)
				return role
			}
			created := stored()
			if created.Level != 1 || created.Experience != 0 || created.SourceSeq != 0 || created.Wallet != (domain.Wallet{}) || len(created.Equipped) != 0 {
				t.Fatal("新角色有测试预置属性、金币或来源流水")
			}
			var kills int
			c := join(t, cfg.GameAddress, issue(t, endpoint, token, key), func(_ *client, frame wire.Frame) {
				if frame.ID != p.MsgS2CExperienceGrantResult || frame.Error != 0 {
					return
				}
				var grant pb.ExperienceGrantResult
				must(t, proto.Unmarshal(frame.Payload, &grant))
				if !grant.Migrated && grant.AcceptedExperience > 0 {
					kills++
				}
			})
			c.stop() // 首次入场的短超时已结束，后续自然战斗走独立观察窗。
			c.ctx, c.stop = context.WithTimeout(ctx, 27*time.Minute)
			t.Cleanup(c.close)
			if c.entry.RoomId != fixture.Map.ID || c.entry.ResourceVersion != fixture.RuleVersion {
				t.Fatal("真实建角未进入当前规则默认普通图")
			}
			c.auto(false, 1)
			equipNaturalUpgradeStarter(c, fixture)
			spare := naturalGrowthItem(c, 218, "")
			if spare == nil || spare.SourceKind != "" {
				t.Fatal("原新角备用木剑缺失")
			}
			if result := c.asset(p.MsgC2SSetItemLock, &pb.SetItemLockRequest{ItemInstanceId: spare.InstanceId,
				ItemVersion: spare.Version, Locked: true, Operation: c.operation()}); result.ErrorCode != 0 {
				t.Fatalf("正常锁定备用木剑失败：%v", result)
			} else {
				c.inventoryAt(result.NewRevision)
			}
			policy := &pb.AutoRecyclePolicy{Enabled: true, QualityIds: []uint32{0, 1, 2, 3, 4}}
			if result := c.asset(p.MsgC2SSetAutoRecycle, &pb.SetAutoRecycleRequest{Policy: policy, Operation: c.operation()}); result.ErrorCode != 0 {
				t.Fatalf("未能通过客户端策略开启自动回收：%v", result)
			} else {
				c.inventoryAt(result.NewRevision)
			}

			started := time.Now()
			t.Cleanup(func() {
				if t.Failed() {
					role := stored()
					t.Logf("自然输出停点：%d杀、等级%d、绑金%d、来源%d，观察时长%s", kills, role.Level, role.Wallet.BoundGold, role.SourceSeq, time.Since(started).Round(time.Second))
				}
			})
			var deaths int
			c.auto(true, 4)
			awaitNaturalEquipmentFunds(t, c, stored, 7, 0, 4, &kills, &deaths)
			c.auto(false, 4)
			naturalEquipmentLearnStarter(t, c, fixture, test.job)
			battleCount := uint32(8)
			if test.mapID != fixture.Map.ID {
				// 谷地比林间伤害高，先在林间学习技能并自然提升到 T2。
				c.auto(true, battleCount)
				ready := awaitNaturalEquipmentFunds(t, c, stored, 11, 0, battleCount, &kills, &deaths)
				if ready.Wallet.BoundGold < offer.PriceUnits {
					c.auto(false, battleCount)
					travelToBaseline(c, 2, test.mapID)
					travelAcknowledge(c, false)
					battleCount = 2
				}
			}
			c.auto(true, battleCount)
			role := awaitNaturalEquipmentFunds(t, c, stored, *definition.RequiredLevel, offer.PriceUnits, battleCount, &kills, &deaths)
			c.auto(false, battleCount)
			t.Logf("自然资金达到现行报价：地图%d、等级%d、绑金%d、%d杀、%d次复活、用时%s", c.entry.RoomId, role.Level, role.Wallet.BoundGold, kills, deaths, time.Since(started).Round(time.Second))

			// 切至商店后普通战斗已暂停，再对拍最后一笔实际收益并使用同一钱包购买。
			travelToBaseline(c, 1, town.ID)
			travelAcknowledge(c, true)
			reachTownVendor(c, town, npc)
			before := stored()
			direct, recycled := naturalOrdinaryKillIncome(t, ctx, store, before)
			if direct+recycled != before.Wallet.BoundGold || before.Wallet.Gold != 0 || recycled <= 0 || direct <= 0 || before.SourceSeq == 0 {
				t.Fatalf("自然钱包和击杀收益不一致：直金%d 回收%d 钱包%+v", direct, recycled, before.Wallet)
			}
			buyTaoistSupplyOnMap(c, town, npc, offer, 1)
			bought := naturalGrowthItem(c, test.itemID, "npc_shop")
			if bought == nil || !bought.Bound || bought.Quantity != 1 || bought.Quality != definition.DropQuality || bought.DurabilityU != uint64(definition.OriginalDurabilityU) ||
				bought.OriginalDurabilityU != bought.DurabilityU || bought.MaxDurabilityU != bought.DurabilityU {
				t.Fatalf("现行NPC装备未保存原来源、品质和满耐久：%v", bought)
			}
			acquired := stored()
			wallet := before.Wallet
			wallet.BoundGold -= offer.PriceUnits
			if acquired.Wallet != wallet || acquired.SourceSeq != before.SourceSeq || acquired.EconomicSeq != before.EconomicSeq+1 {
				t.Fatalf("NPC扣款或原流水不符：钱包 %+v→%+v，期望 %+v；来源%d→%d，经济%d→%d",
					before.Wallet, acquired.Wallet, wallet, before.SourceSeq, acquired.SourceSeq, before.EconomicSeq, acquired.EconomicSeq)
			}
			var purchase persistence.Operation
			purchaseID := "asset:" + roleID + ":" + bought.SourceId
			must(t, store.DB.Collection("operations").FindOne(ctx, bson.M{"_id": purchaseID, "role_id": roleID}).Decode(&purchase))
			if purchase.Code != 0 || purchase.Vendor == nil || purchase.Vendor.NPC != npc.ID || len(purchase.Vendor.Plan.Outputs) != 1 ||
				purchase.Vendor.Plan.Outputs[0].ID != domain.Hex(bought.InstanceId) || purchase.Vendor.Plan.Wallet.BoundGold != offer.PriceUnits {
				t.Fatal("实际购买缺少同实例、同NPC和同扣款的原回执")
			}
			must(t, store.DB.Collection("audit_outbox").FindOne(ctx, bson.M{"_id": purchaseID, "role_id": roleID}).Err())

			originalStats := proto.Clone(c.player().CombatAttributes).(*pb.CombatAttributes)
			old := naturalEquipmentInSlot(acquired, definition.EquipmentSlot)
			oldDefinition, _ := fixture.Item(old.ItemID)
			expected := naturalEquippedStats(originalStats, oldDefinition, definition)
			if proto.Equal(originalStats, expected) {
				t.Fatal("当前装备对实际职业没有可观察属性变化")
			}
			wearRequest := &pb.EquipRequest{ItemInstanceId: bought.InstanceId, ItemVersion: bought.Version, Slot: definition.EquipmentSlot, Operation: c.operation()}
			wore := c.asset(p.MsgC2SEquip, wearRequest)
			if wore.ErrorCode != 0 {
				t.Fatalf("实际达到装备等级后穿戴失败：%v", wore)
			}
			c.inventoryAt(wore.NewRevision)
			travelUntilWithin(c, 5*time.Second, func(frame wire.Frame) bool {
				return frame.ID == p.MsgS2CSceneBaseline && c.player() != nil && proto.Equal(c.player().CombatAttributes, expected)
			})
			worn := stored()
			originalGear := naturalEquipmentInSlot(worn, definition.EquipmentSlot)
			if originalGear.ID != domain.Hex(bought.InstanceId) || originalGear.SourceID != bought.SourceId || originalGear.DurabilityU != definition.OriginalDurabilityU ||
				worn.Wallet != wallet || worn.SourceSeq != before.SourceSeq || worn.EconomicSeq != acquired.EconomicSeq+1 {
				t.Fatal("持久穿戴、首次耐久或经济序号与真实原件不符")
			}
			if old.ID != "" && !hasNaturalEquipmentInstance(worn.Items, old.ID) {
				t.Fatal("换装时原新手装备未返回背包")
			}
			c.close()
			rejoined := join(t, cfg.GameAddress, issue(t, endpoint, token, key))
			t.Cleanup(rejoined.close)
			after := stored()
			if naturalEquipmentInSlot(after, definition.EquipmentSlot) != originalGear || after.SourceSeq != worn.SourceSeq ||
				after.EconomicSeq != worn.EconomicSeq || after.Wallet != worn.Wallet || !proto.Equal(rejoined.player().CombatAttributes, expected) {
				t.Fatal("健康新票没有返回原持久装备、属性、耐久、钱包或游标")
			}
			if replay := rejoined.asset(p.MsgC2SEquip, wearRequest); replay.ErrorCode != 0 || stored().Revision != after.Revision {
				t.Fatal("重连后原穿戴编号重放重复改变了资产")
			}
			t.Logf("自然换装完成：%s T%d，等级%d，%d杀，%d次复活，直金%d，回收%d，购价%d，耐久%d，物攻%d–%d→%d–%d，魔法%d–%d→%d–%d，道术%d–%d→%d–%d，耗时%s",
				definition.Name, definition.EquipmentStage+1, before.Level, kills, deaths, direct, recycled, offer.PriceUnits, originalGear.DurabilityU,
				originalStats.PhysicalAttackMin, originalStats.PhysicalAttackMax, expected.PhysicalAttackMin, expected.PhysicalAttackMax,
				originalStats.MagicAttackMin, originalStats.MagicAttackMax, expected.MagicAttackMin, expected.MagicAttackMax,
				originalStats.SpiritAttackMin, originalStats.SpiritAttackMax, expected.SpiritAttackMin, expected.SpiritAttackMax, time.Since(started).Round(time.Second))
		})
	}
}

// naturalEquipmentLearnStarter 学习角色正常得到的赠书，沿用每职业的实际自动施法策略。
func naturalEquipmentLearnStarter(t *testing.T, c *client, fixture *devconfig.Fixture, job uint32) {
	t.Helper()
	for _, initial := range fixture.InitialSkillBooks {
		if initial.JobID != job {
			continue
		}
		book := naturalGrowthItem(c, initial.ItemID, "initial_grant")
		definition, known := fixture.Item(initial.ItemID)
		if book == nil || !known || definition.BookSkillID == 0 {
			t.Fatal("新角未收到原赠书")
		}
		result := c.asset(p.MsgC2SLearnSkill, &pb.LearnSkillRequest{ItemInstanceId: book.InstanceId, SkillId: definition.BookSkillID, Operation: c.operation()})
		if result.ErrorCode != 0 {
			t.Fatalf("自然达级后学习原技能失败：%v", result)
		}
		c.inventoryAt(result.NewRevision)
		return
	}
	t.Fatal("当前职业没有现行新手赠书")
}

// awaitNaturalEquipmentFunds 等待真实击杀后达级和积累实际商店报价。
func awaitNaturalEquipmentFunds(t *testing.T, c *client, read func() domain.Role, level uint32, price int64, count uint32, kills, deaths *int) domain.Role {
	t.Helper()
	role := read()
	for role.Level < level || role.Wallet.BoundGold < price {
		frame := c.until(func(frame wire.Frame) bool {
			return frame.ID == p.MsgS2CExperienceGrantResult || frame.ID == p.MsgS2CDeathNotice
		})
		if frame.ID == p.MsgS2CDeathNotice {
			(*deaths)++
			if *deaths > 3 {
				t.Fatal("正常复活后仍重复阵亡，无法持续累积装备资金")
			}
			result := c.asset(p.MsgC2SReviveChoice, &pb.ReviveChoice{DeathId: c.death.DeathId, Action: 1, Operation: c.operation()})
			if result.ErrorCode != 0 {
				t.Fatalf("实际阵亡后的普通复活失败：%v", result)
			}
			c.inventoryAt(result.NewRevision)
			c.until(func(f wire.Frame) bool { return f.ID == p.MsgS2CSceneBaseline && c.player().GetHp() > 0 })
			c.auto(true, count)
		}
		role = read()
		if *kills > 0 && *kills%20 == 0 {
			t.Logf("自然装备进度：%d杀，等级%d，绑金%d，目标等级%d和报价%d", *kills, role.Level, role.Wallet.BoundGold, level, price)
		}
	}
	return role
}

// naturalEquipmentInSlot 读取该角色实际存档中目标装备槽的原件。
func naturalEquipmentInSlot(role domain.Role, slot uint32) domain.Item {
	for _, entry := range role.Equipped {
		if entry.Slot == slot {
			return entry.Item
		}
	}
	return domain.Item{}
}

// hasNaturalEquipmentInstance 在背包按原实例检查同时换回的旧装备。
func hasNaturalEquipmentInstance(items []domain.Item, id string) bool {
	for _, item := range items {
		if item.ID == id {
			return true
		}
	}
	return false
}

// naturalEquippedStats 用同等级的旧场景属性和工作簿装备差值核对主战斗属性。
func naturalEquippedStats(before *pb.CombatAttributes, old, replacement devconfig.Item) *pb.CombatAttributes {
	expected := proto.Clone(before).(*pb.CombatAttributes)
	expected.PhysicalAttackMin += replacement.AttackMinBonus - old.AttackMinBonus
	expected.PhysicalAttackMax += replacement.AttackMaxBonus - old.AttackMaxBonus
	expected.PhysicalDefenceMin += replacement.DefenceMinBonus - old.DefenceMinBonus
	expected.PhysicalDefenceMax += replacement.DefenceMaxBonus - old.DefenceMaxBonus
	expected.MagicDefenceMin += replacement.MagicDefenceMinBonus - old.MagicDefenceMinBonus
	expected.MagicDefenceMax += replacement.MagicDefenceMaxBonus - old.MagicDefenceMaxBonus
	expected.MagicAttackMin += replacement.MagicMinBonus - old.MagicMinBonus
	expected.MagicAttackMax += replacement.MagicMaxBonus - old.MagicMaxBonus
	expected.SpiritAttackMin += replacement.TaoMinBonus - old.TaoMinBonus
	expected.SpiritAttackMax += replacement.TaoMaxBonus - old.TaoMaxBonus
	return expected
}
