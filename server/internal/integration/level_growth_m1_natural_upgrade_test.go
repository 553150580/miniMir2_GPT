//go:build m1_natural_upgrade

// 本文件慢速验收三职业首个技能的自然一阶材料供给，使用独立真实联机链路。
package integration

import (
	"context"
	"fmt"
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

// TestLevelGrowthNaturalM1FirstUpgradeMongoWS 从原新手钱包和背包开始，真实掉落、回收、购书并提交一阶。
func TestLevelGrowthNaturalM1FirstUpgradeMongoWS(t *testing.T) {
	cfg, ctx, store := m1FieldServices(t, m1FieldConfig(t))
	fixture := &cfg.Fixture
	for _, skillID := range []uint32{3, 1, 2} {
		skill, found := fixture.Skill(skillID)
		if !found || skill.LearnedLevel != 0 {
			t.Fatalf("技能%d缺少正常的首个习得阶段", skillID)
		}
		row, found := fixture.Upgrade(skillID, 1)
		if !found || len(row.BookCosts) != 1 || row.BookCosts[0].ItemID != skill.BookItemID || row.PageCost <= 0 {
			t.Fatalf("技能%d缺少真实的一阶材料规则", skillID)
		}
		bookCount := row.BookCosts[0].Quantity
		t.Run(skill.Name, func(t *testing.T) {
			t.Parallel()
			var offer devconfig.SkillShop
			for _, candidate := range fixture.SkillShop {
				if candidate.ItemID == skill.BookItemID && candidate.Currency == "gold" && candidate.Quantity == 1 && candidate.MaxPurchaseQuantity >= bookCount {
					offer = candidate
					break
				}
			}
			if offer.ID == 0 {
				t.Fatal("缺少可正常一次买足一阶材料的金币书店商品")
			}
			skillPrice := offer.PriceUnits * bookCount
			endpoint, token, key := m1FieldRole(t, cfg, skill.JobID)
			roleID := domain.Hex(key.RoleId)
			stored := func() domain.Role {
				t.Helper()
				role, err := store.Role(ctx, roleID)
				must(t, err)
				return role
			}
			created := stored()
			if created.Level != 1 || created.Experience != 0 || created.SourceSeq != 0 || len(created.Skills) != 0 || created.Wallet != (domain.Wallet{}) {
				t.Fatal("角色不是未注入资产的正常新角")
			}
			var killCount int
			c := join(t, cfg.GameAddress, issue(t, endpoint, token, key), func(_ *client, frame wire.Frame) {
				if frame.Error == 0 && frame.ID == p.MsgS2CExperienceGrantResult {
					var grant pb.ExperienceGrantResult
					must(t, proto.Unmarshal(frame.Payload, &grant))
					if !grant.Migrated && grant.AcceptedExperience > 0 {
						killCount++
					}
				}
			})
			c.stop() // 入场确认的80秒已经结束，长线经济在独立时限内执行。
			c.ctx, c.stop = context.WithTimeout(ctx, 14*time.Minute)
			t.Cleanup(c.close)
			if c.entry.ResourceVersion != fixture.RuleVersion || c.entry.RoomId != fixture.Map.ID || c.inventory.Level != 1 {
				t.Fatal("真实新角未进入指定规则默认普通图")
			}
			c.auto(false, 1)
			equipNaturalUpgradeStarter(c, fixture)
			// 建角还会送一把备用木剑。通过普通锁定保留它，只把战斗掉落计入升阶资金。
			spare := naturalGrowthItem(c, 218, "")
			if spare == nil || spare.SourceKind != "" {
				t.Fatal("初赠备用装备缺失")
			}
			locked := c.asset(p.MsgC2SSetItemLock, &pb.SetItemLockRequest{ItemInstanceId: spare.InstanceId, ItemVersion: spare.Version,
				Locked: true, Operation: c.operation()})
			if locked.ErrorCode != 0 {
				t.Fatalf("正常锁定备用木剑失败：%v", locked)
			}
			c.inventoryAt(locked.NewRevision)
			policy := &pb.AutoRecyclePolicy{Enabled: true, QualityIds: []uint32{0, 1, 2, 3, 4}}
			if result := c.asset(p.MsgC2SSetAutoRecycle, &pb.SetAutoRecycleRequest{Policy: policy, Operation: c.operation()}); result.ErrorCode != 0 {
				t.Fatalf("普通自动回收策略未生效：%v", result)
			} else {
				c.inventoryAt(result.NewRevision)
			}
			started := time.Now()
			t.Cleanup(func() {
				if t.Failed() {
					actual := stored()
					t.Logf("实测停点：%d杀、%d级、角色钱包=%+v、背包%d/%d、用时%s",
						killCount, actual.Level, actual.Wallet, len(actual.Items), actual.Capacity, time.Since(started).Round(time.Second))
				}
			})
			c.auto(true, 4)
			c.until(func(frame wire.Frame) bool {
				return frame.ID == p.MsgS2CExperienceGrantResult && c.inventory.Level >= skill.LearnLevel
			})
			c.auto(false, 4)
			book := naturalGrowthItem(c, skill.BookItemID, "initial_grant")
			if book == nil {
				t.Fatal("没有同一角色的原初赠书")
			}
			learn := c.asset(p.MsgC2SLearnSkill, &pb.LearnSkillRequest{ItemInstanceId: book.InstanceId, SkillId: skill.ID, Operation: c.operation()})
			if learn.ErrorCode != 0 {
				t.Fatalf("实际达级仍无法习得：%v", learn)
			}
			c.inventoryAt(learn.NewRevision)
			t.Logf("正常习得，角色等级=%d，击杀=%d，距开始=%s", c.inventory.Level, killCount, time.Since(started).Round(time.Second))

			c.auto(true, 8) // 习得后使用客户端正常开放的八怪档，保持原位置。
			role := stored()
			drop, sourceRule := naturalUpgradeDroppedBook(role, fixture, row.PageCost)
			for role.Level < row.RequiredRoleLevel || role.Wallet.Gold+role.Wallet.BoundGold < skillPrice || drop.ID == "" {
				if time.Since(started) >= 13*time.Minute {
					t.Fatalf("自然供给未在观察窗内齐备：需金币%d、需可拆%d页，现为%v，合格原书=%t", skillPrice, row.PageCost, role.Wallet, drop.ID != "")
				}
				frame := c.until(func(frame wire.Frame) bool {
					return frame.ID == p.MsgS2CExperienceGrantResult || frame.ID == p.MsgS2CDeathNotice
				})
				if frame.ID == p.MsgS2CDeathNotice {
					t.Fatal("真实角色定点经济观察期间死亡")
				}
				role = stored()
				drop, sourceRule = naturalUpgradeDroppedBook(role, fixture, row.PageCost)
				if killCount%10 == 0 {
					t.Logf("自然进度：击杀%d、等级%d、金币%d、绑定金币%d、合格原书=%t、用时%s",
						killCount, role.Level, role.Wallet.Gold, role.Wallet.BoundGold, drop.ID != "", time.Since(started).Round(time.Second))
				}
			}
			readyAt, readyKills := time.Since(started), killCount
			c.auto(false, 8)
			// 等原普攻和磨损来源确认，准备报价时使用最后实际资产版本。
			time.Sleep(4 * time.Second)
			barrier := c.send(p.MsgC2SMoveIntent, &pb.MoveIntent{SceneEpoch: c.entry.SceneEpoch, Direction: 8})
			c.expectError(barrier, p.CodeInvalidArgument)
			before := stored()
			direct, recycled := naturalUpgradeIncome(t, ctx, store, before, drop, fixture.Map.ID)
			if before.Wallet.Gold+before.Wallet.BoundGold != direct+recycled {
				t.Fatal("自然金币来源对账不一致")
			}
			t.Logf("材料齐备：%d杀、%d级、自然直接金币%d、自动回收金币%d、原掉书%d@来源%s、用时%s",
				readyKills, before.Level, direct, recycled, drop.ItemID, drop.SourceID, readyAt.Round(time.Millisecond))

			pages := c.skillPreview(&pb.SkillAssetPreviewRequest{Kind: 3,
				Materials: []*pb.ItemConsumption{{InstanceId: domain.Bytes(drop.ID), Version: uint64(drop.Version), Quantity: 1}}})
			if len(pages.Outputs) != 1 || pages.Outputs[0].ItemId != row.PageItemID || pages.Outputs[0].Quantity != sourceRule.PagesPerBook || !pages.Outputs[0].Bound {
				t.Fatalf("原掉落技能书拆页不符合正式报价：%v", pages)
			}
			pageCommit := &pb.SkillAssetCommitRequest{PreviewId: pages.PreviewId, Operation: c.operation()}
			if result := c.skillCommit(pageCommit); result.ErrorCode != 0 {
				t.Fatalf("正常拆书失败：%v", result)
			}
			purchase := c.skillPreview(&pb.SkillAssetPreviewRequest{Kind: 1, OfferId: offer.ID, Quantity: bookCount})
			if purchase.Spend.GetGold()+purchase.Spend.GetBoundGold() != skillPrice {
				t.Fatalf("用实赚金币购书的报价不符：%v", purchase)
			}
			bookCommit := &pb.SkillAssetCommitRequest{PreviewId: purchase.PreviewId, Operation: c.operation()}
			if result := c.skillCommit(bookCommit); result.ErrorCode != 0 {
				t.Fatalf("普通书店购买一阶材料失败：%v", result)
			}
			materials := naturalUpgradeMaterials(c, skill.BookItemID, bookCount, row.PageItemID, row.PageCost, bookCommit, pageCommit)
			quote := c.skillPreview(&pb.SkillAssetPreviewRequest{Kind: 2, SkillId: skill.ID, Materials: materials})
			if quote.FromLevel != 0 || quote.ToLevel != 1 || len(quote.Materials) != len(materials) {
				t.Fatalf("真实自然材料未产生正确一阶报价：%v", quote)
			}
			upgrade := &pb.SkillAssetCommitRequest{PreviewId: quote.PreviewId, Operation: c.operation()}
			if result := c.skillCommit(upgrade); result.ErrorCode != 0 {
				t.Fatalf("正常提交一阶失败：%v", result)
			}
			finished := stored()
			if rank, ok := finished.Skill(skill.ID); !ok || rank != 1 ||
				finished.Wallet.Gold+finished.Wallet.BoundGold != before.Wallet.Gold+before.Wallet.BoundGold-skillPrice ||
				!slices.ContainsFunc(c.inventory.LearnedSkills, func(row *pb.LearnedSkill) bool { return row.SkillId == skill.ID && row.Level == 1 }) {
				t.Fatal("永久技能、钱包扣账与真实WS同步不一致")
			}
			if replay := c.skillCommit(upgrade); replay.ErrorCode != 0 || stored().Revision != finished.Revision {
				t.Fatal("同一自然升阶提交重放重复修改资产")
			}
			t.Logf("自然一阶完成：%s，规则%d，材料%d本同书＋%d书页，耗%d个自然金币，累计%d杀、用时%s",
				skill.Name, fixture.RuleVersion, bookCount, row.PageCost, skillPrice, killCount, time.Since(started).Round(time.Millisecond))
		})
	}
}

// equipNaturalUpgradeStarter 只让原角色常规穿戴建角得到的木剑与布衣。
func equipNaturalUpgradeStarter(c *client, fixture *devconfig.Fixture) {
	c.t.Helper()
	for _, itemID := range []uint32{218, 193} {
		item := naturalGrowthItem(c, itemID, "")
		definition, exists := fixture.Item(itemID)
		if item == nil || !exists {
			c.t.Fatal("缺少新角原装")
		}
		result := c.asset(p.MsgC2SEquip, &pb.EquipRequest{ItemInstanceId: item.InstanceId,
			Slot: definition.EquipmentSlot, ItemVersion: item.Version, Operation: c.operation()})
		if result.ErrorCode != 0 {
			c.t.Fatalf("原装穿戴失败：%v", result)
		}
		c.inventoryAt(result.NewRevision)
	}
}

// naturalUpgradeDroppedBook 只选择来源与品质均通过正式规则、单本足以析出一阶书页的真实掉落。
func naturalUpgradeDroppedBook(role domain.Role, fixture *devconfig.Fixture, pageCost int64) (domain.Item, devconfig.SkillBookRecycle) {
	for _, item := range role.Items {
		if item.SourceKind == "monster_drop" && item.Quantity >= 1 {
			if row, ok := fixture.BookRecycle(item.ItemID, item.Quality); ok && row.PagesPerBook >= pageCost && slices.Contains(row.AllowedSourceKinds, item.SourceKind) {
				return item, row
			}
		}
	}
	return domain.Item{}, devconfig.SkillBookRecycle{}
}

// naturalUpgradeIncome 核对造成金币增长的真实自动回收回执及带原地图的掉书收益。
func naturalUpgradeIncome(t *testing.T, ctx context.Context, store *persistence.Store, role domain.Role, drop domain.Item, mapID uint64) (int64, int64) {
	t.Helper()
	var bookReceipt persistence.Operation
	source, err := strconv.ParseInt(drop.SourceID, 10, 64)
	must(t, err)
	must(t, store.DB.Collection("operations").FindOne(ctx, bson.M{"_id": fmt.Sprintf("settle:%s:%d", role.ID, source)}).Decode(&bookReceipt))
	if proof := bookReceipt.Settlement; proof == nil || proof.MapID != mapID || proof.Kills != 1 || !proof.GrantedItem || !drop.Bound {
		t.Fatal("拆书前真实掉落缺少原野外收益的入包回执")
	}
	return naturalOrdinaryKillIncome(t, ctx, store, role)
}

// naturalOrdinaryKillIncome 仅统计真实击杀账本的直接金币及同次掉落兑现的自动回收金币。
func naturalOrdinaryKillIncome(t *testing.T, ctx context.Context, store *persistence.Store, role domain.Role) (int64, int64) {
	t.Helper()
	cursor, err := store.DB.Collection("audit_outbox").Find(ctx, bson.M{"role_id": role.ID, "kind": "settlement", "kills": 1})
	must(t, err)
	var rows []struct {
		SourceSeq  int64                         `bson:"source_seq"`
		Gold       int64                         `bson:"gold"`
		Settlement persistence.SettlementReceipt `bson:"settlement"`
	}
	must(t, cursor.All(ctx, &rows))
	var direct, recycled int64
	for _, row := range rows {
		for _, item := range row.Settlement.Recycling.Recycled {
			if item.SourceKind != "monster_drop" || item.SourceID != strconv.FormatInt(row.SourceSeq, 10) {
				t.Fatalf("自动回收金币不属于同次击杀掉落：来源=%d，物品%d 来源=%s/%s", row.SourceSeq, item.ItemID, item.SourceKind, item.SourceID)
			}
		}
		direct += row.Gold
		recycled += row.Settlement.Recycling.Gold + row.Settlement.Recycling.BoundGold
	}
	return direct, recycled
}

// naturalUpgradeMaterials 仅用本次书店和本次拆页两个操作真实生成的实例构造一阶请求。
func naturalUpgradeMaterials(c *client, bookID uint32, books int64, pageID uint32, pages int64,
	bookCommit, pageCommit *pb.SkillAssetCommitRequest) []*pb.ItemConsumption {
	c.t.Helper()
	needed := map[uint32]int64{bookID: books, pageID: pages}
	materials := []*pb.ItemConsumption{}
	for _, item := range c.inventory.Items {
		quantity := min(needed[item.ItemId], item.Quantity)
		if quantity <= 0 || !item.Bound {
			continue
		}
		book := item.ItemId == bookID && item.SourceKind == "npc_shop" && item.SourceId == domain.Hex(bookCommit.Operation.OpId)
		page := item.ItemId == pageID && item.SourceKind == "skill_recycle" && item.SourceId == domain.Hex(pageCommit.Operation.OpId)
		if book || page {
			materials = append(materials, &pb.ItemConsumption{InstanceId: item.InstanceId, Version: item.Version, Quantity: quantity})
			needed[item.ItemId] -= quantity
		}
	}
	if needed[bookID] != 0 || needed[pageID] != 0 {
		c.t.Fatalf("已确认的实际来路材料不足：%v", needed)
	}
	return materials
}
