//go:build m1_natural_upgrade

// 本文件在真实联机环境中验证道士从正常建角到自然取得一阶灵魂火符。
package integration

import (
	"context"
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

// TestLevelGrowthNaturalM1TaoistSoulFireMongoWS 核对正常所得书、页和护符，以及实际一阶手动、自动消耗和重连。
func TestLevelGrowthNaturalM1TaoistSoulFireMongoWS(t *testing.T) {
	cfg, ctx, store := m1FieldServices(t, m1FieldConfig(t))
	fixture := &cfg.Fixture
	skill, known := fixture.Skill(13)
	if !known || skill.JobID != 2 || !skill.AutoEnabled || skill.LearnedLevel != 0 || skill.Behavior != "soul_fire_ball" || skill.MaterialQuantity != 1 {
		t.Fatal("现行目录缺少可手动及自动使用的一张符灵魂火符")
	}
	upgrade, known := fixture.Upgrade(skill.ID, 1)
	if !known || len(upgrade.BookCosts) != 1 || upgrade.BookCosts[0].ItemID != skill.BookItemID || upgrade.PageCost <= 0 {
		t.Fatal("现行目录缺少火符一阶消耗")
	}
	books := upgrade.BookCosts[0].Quantity
	bookTown, bookNPC, bookOffer := naturalEquipmentVendor(t, fixture, skill.BookItemID)
	weaponTown, weaponNPC, weaponOffer := naturalEquipmentVendor(t, fixture, 223)
	amuletTown, amuletNPC, amuletOffer := naturalEquipmentVendor(t, fixture, skill.MaterialItemID)
	const amuletCount = 4
	if bookOffer.MaxPurchaseQuantity < books+1 || amuletOffer.MaxPurchaseQuantity < amuletCount {
		t.Fatal("现行店铺不支持本次正常购买数量")
	}
	skillSpend := (books+1)*bookOffer.PriceUnits + amuletCount*amuletOffer.PriceUnits
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
		t.Fatal("新道士存在正常建角外的资源")
	}
	started := time.Now()
	var kills, deaths int
	var manual *pb.CastAccepted
	c := join(t, cfg.GameAddress, issue(t, endpoint, token, key), func(_ *client, frame wire.Frame) {
		switch frame.ID {
		case p.MsgS2CExperienceGrantResult:
			var grant pb.ExperienceGrantResult
			must(t, proto.Unmarshal(frame.Payload, &grant))
			if !grant.Migrated && grant.AcceptedExperience > 0 {
				kills++
			}
		case p.MsgS2CCastAccepted:
			cast := &pb.CastAccepted{}
			must(t, proto.Unmarshal(frame.Payload, cast))
			if cast.SkillId == skill.ID && cast.InputSequence != 0 {
				manual = cast
			}
		}
	})
	c.stop()
	c.ctx, c.stop = context.WithTimeout(ctx, 49*time.Minute)
	t.Cleanup(c.close)
	t.Cleanup(func() {
		if t.Failed() {
			role := stored()
			t.Logf("自然火符停点：%d级、%d杀、%d次阵亡、绑金%d、背包%d/%d、用时%s", role.Level, kills, deaths, role.Wallet.BoundGold,
				len(role.Items), role.Capacity, time.Since(started).Round(time.Second))
		}
	})
	if c.entry.ResourceVersion != fixture.RuleVersion || c.entry.RoomId != fixture.Map.ID {
		t.Fatal("新道士未进入现行普通地图")
	}
	c.auto(false, 1)
	equipNaturalUpgradeStarter(c, fixture)
	spare := naturalGrowthItem(c, 218, "")
	if spare == nil || spare.SourceKind != "" {
		t.Fatal("原建角赠送的备用木剑缺失")
	}
	lock := c.asset(p.MsgC2SSetItemLock, &pb.SetItemLockRequest{ItemInstanceId: spare.InstanceId, ItemVersion: spare.Version, Locked: true, Operation: c.operation()})
	if lock.ErrorCode != 0 {
		t.Fatal("无法保留初赠木剑作为非资金来源", lock)
	}
	c.inventoryAt(lock.NewRevision)
	policy := &pb.AutoRecyclePolicy{Enabled: true, QualityIds: []uint32{0, 1, 2, 3, 4}}
	recycle := c.asset(p.MsgC2SSetAutoRecycle, &pb.SetAutoRecycleRequest{Policy: policy, Operation: c.operation()})
	if recycle.ErrorCode != 0 {
		t.Fatal("正常自动回收开关失败", recycle)
	}
	c.inventoryAt(recycle.NewRevision)
	starter, known := fixture.Skill(2)
	if !known || starter.JobID != skill.JobID {
		t.Fatal("缺少道士原始治愈术")
	}
	c.auto(true, 4)
	awaitNaturalEquipmentFunds(t, c, stored, starter.LearnLevel, 0, 4, &kills, &deaths)
	c.auto(false, 4)
	naturalEquipmentLearnStarter(t, c, fixture, skill.JobID)

	// 先用普通收益购入青铜剑协助后续实战，保留原商人、金币和武器实例关联。
	c.auto(true, 8)
	awaitNaturalEquipmentFunds(t, c, stored, starter.LearnLevel, weaponOffer.PriceUnits, 8, &kills, &deaths)
	c.auto(false, 1)
	buyTaoistSupplyOnMap(c, weaponTown, weaponNPC, weaponOffer, 1)
	weapon := naturalGrowthItem(c, weaponOffer.ItemID, "npc_shop")
	definition, known := fixture.Item(weaponOffer.ItemID)
	if !known || weapon == nil {
		t.Fatal("正常商店武器未进入当前背包")
	}
	originalWeapon := naturalEquipmentInSlot(stored(), definition.EquipmentSlot)
	if originalWeapon.ItemID != 218 || originalWeapon.SourceKind != "" || originalWeapon.ID == "" {
		t.Fatal("换装前没有正常穿戴的原初赠木剑")
	}
	assertNaturalTaoistPurchase(t, ctx, store, roleID, weapon, weaponNPC.ID, weaponOffer.PriceUnits)
	worn := c.asset(p.MsgC2SEquip, &pb.EquipRequest{ItemInstanceId: weapon.InstanceId, ItemVersion: weapon.Version, Slot: definition.EquipmentSlot, Operation: c.operation()})
	if worn.ErrorCode != 0 {
		t.Fatal("正常达到穿戴门槛后更换青铜剑失败", worn)
	}
	c.inventoryAt(worn.NewRevision)
	// 原装备返回背包后同样加锁，避免本次自然收益把初赠木剑计为击杀回收。
	index := slices.IndexFunc(c.inventory.Items, func(item *pb.ItemView) bool { return domain.Hex(item.InstanceId) == originalWeapon.ID })
	if index < 0 {
		t.Fatal("换装后未保留原初赠木剑实例")
	}
	old := c.inventory.Items[index]
	lock = c.asset(p.MsgC2SSetItemLock, &pb.SetItemLockRequest{ItemInstanceId: old.InstanceId, ItemVersion: old.Version, Locked: true, Operation: c.operation()})
	if lock.ErrorCode != 0 {
		t.Fatal("未保护换下的初赠装备", lock)
	}
	c.inventoryAt(lock.NewRevision)
	travelToBaseline(c, 2, fixture.Map.ID)
	travelAcknowledge(c, false)
	t.Logf("道士自然成长中间检查：%d级、%d杀，从武器商花%d金币购入青铜剑，已用时%s", c.inventory.Level, kills, weaponOffer.PriceUnits, time.Since(started).Round(time.Second))
	c.auto(true, 8)
	level := max(skill.LearnLevel, upgrade.RequiredRoleLevel)
	role := awaitNaturalEquipmentFunds(t, c, stored, level, skillSpend, 8, &kills, &deaths)
	dropped, pageRule := naturalUpgradeDroppedBook(role, fixture, upgrade.PageCost)
	for dropped.ID == "" {
		awaitNaturalEquipmentFunds(t, c, stored, level, role.Wallet.BoundGold+1, 8, &kills, &deaths)
		role = stored()
		dropped, pageRule = naturalUpgradeDroppedBook(role, fixture, upgrade.PageCost)
	}
	c.auto(false, 1)
	reachTownVendor(c, bookTown, bookNPC)
	reserveNaturalTaoistBag(c, fixture, int(books+2))
	before := stored()
	direct, recycled := naturalUpgradeIncome(t, ctx, store, before, dropped, fixture.Map.ID)
	if before.Wallet.Gold != 0 || direct+recycled != before.Wallet.BoundGold+weaponOffer.PriceUnits || direct == 0 || recycled == 0 {
		t.Fatalf("升级前钱包不能据真实原野外收益对账：直金%d、回收%d、钱包%+v", direct, recycled, before.Wallet)
	}
	t.Logf("自然火符材料齐备：%d级、%d杀、%d死、直金%d＋回收%d、剩余绑金%d、原掉书%d/%s、用时%s",
		before.Level, kills, deaths, direct, recycled, before.Wallet.BoundGold, dropped.ItemID, dropped.SourceID, time.Since(started).Round(time.Second))
	learnNaturalTaoistSoul(t, c, ctx, store, roleID, skill, upgrade, bookNPC, bookOffer, dropped, pageRule)
	buyTaoistSupplyOnMap(c, amuletTown, amuletNPC, amuletOffer, amuletCount)
	amulet := equipTaoistVendorSupply(c, skill.MaterialItemID)
	assertNaturalTaoistPurchase(t, ctx, store, roleID, amulet, amuletNPC.ID, amuletCount*amuletOffer.PriceUnits)
	qualified := stored()
	if rank, ok := qualified.Skill(skill.ID); !ok || rank != 1 || qualified.Wallet.BoundGold != before.Wallet.BoundGold-skillSpend || qualified.Wallet.Gold != 0 {
		t.Fatal("永久已学阶与正常购书购符扣账不符")
	}
	travelToBaseline(c, 2, fixture.Map.ID)
	travelAcknowledge(c, false)
	time.Sleep(time.Duration(c.player().MoveIntervalMs)*time.Millisecond + 150*time.Millisecond)
	target := m1LiveMonster(t, c, skill.RangeCells, nil)
	use := &pb.CastWithMaterialRequest{SceneEpoch: c.entry.SceneEpoch, CasterLife: c.player().Life, SkillId: skill.ID, TargetHandle: target.EntityHandle,
		MaterialInstanceId: amulet.InstanceId, MaterialVersion: amulet.Version, Operation: c.operation()}
	cast := c.asset(p.MsgC2SCastWithMaterial, use)
	if cast.ErrorCode != 0 {
		t.Fatal("一阶手动火符未成功扣号并受理", cast)
	}
	c.inventoryAt(cast.NewRevision)
	if left := summonWSEquipped(t, c); left.Quantity != amuletCount-1 {
		t.Fatalf("手动施法未只消费一张护符：%v", left)
	}
	hit := c.combatImpact(skill.ID, target.EntityHandle)
	if manual == nil || manual.SkillLevel != 1 || manual.TargetLife != target.Life || hit.TargetLife != target.Life || hit.CastId != manual.CastId || hit.Outcome != 1 || hit.Amount <= 0 {
		t.Fatalf("手动真实火符未按一阶命中原目标生命：%v %v", manual, hit)
	}
	if replay := c.asset(p.MsgC2SCastWithMaterial, use); replay.ErrorCode != 0 || summonWSEquipped(t, c).Quantity != amuletCount-1 {
		t.Fatal("同一手动施法重传重复消耗护符", replay)
	}
	manualReceipt := assertNaturalTaoistCast(t, ctx, store, roleID,
		bson.M{"_id": "asset:" + roleID + ":" + domain.Hex(use.Operation.OpId)}, amulet, skill.ID, "manual")
	if manualReceipt.CommandSeq != int64(cast.CommandSeq) {
		t.Fatal("原手动序号与确权回执不一致")
	}
	// 手动火符可能击倒首只怪物，等它按原地图的自然复活后，再触发并计量一次自动火符。
	travelUntilWithin(c, 14*time.Second, func(frame wire.Frame) bool {
		return frame.ID == p.MsgS2CSceneBaseline && c.entity(target.EntityHandle).GetHp() > 0
	})
	time.Sleep(time.Duration(skill.CooldownBaseMS)*time.Millisecond + 100*time.Millisecond)
	applyAutoSkillPreferences(c, &pb.AutoSkillSettings{Enabled: true, DisabledSkillIds: []uint32{starter.ID}})
	automatic := expectTaoistCatalogAutoCast(c, skill.ID)
	if automatic.SkillLevel != 1 || automatic.CastId == manual.CastId || summonWSEquipped(t, c).Quantity != amuletCount-2 {
		t.Fatal("自动火符没有定点沿用一阶，或未恰好再消耗一张原护符")
	}
	autoReceipt := assertNaturalTaoistCast(t, ctx, store, roleID, bson.M{"role_id": roleID,
		"casting_use.scope": "auto", "casting_use.skill_id": skill.ID}, amulet, skill.ID, "auto")
	if autoReceipt.SourceSeq <= 0 {
		t.Fatal("自动火符缺少真实自动材料源编号")
	}
	c.close()
	rejoined := join(t, cfg.GameAddress, issue(t, endpoint, token, key))
	t.Cleanup(rejoined.close)
	if rejoined.entry.RoomId != fixture.Map.ID || !slices.ContainsFunc(rejoined.inventory.LearnedSkills, func(row *pb.LearnedSkill) bool {
		return row.SkillId == skill.ID && row.Level == 1
	}) {
		t.Fatal("新票没有返回原普通地图或永久一阶火符")
	}
	if left := summonWSEquipped(t, rejoined); left.Quantity != amuletCount-2 || domain.Hex(left.InstanceId) != domain.Hex(amulet.InstanceId) || left.SourceId != amulet.SourceId {
		t.Fatal("健康重连没有恢复 NPC 原护符实例、来源与剩余数量", left)
	}
	t.Logf("自然火符完成：规则%d，最终%d级、%d杀，书费%d、%d本同书＋%d掉书页、符费%d，原NPC堆叠%d→%d，手动流水%d、自动来源%d，健康重连保留一阶与原槽，用时%s",
		fixture.RuleVersion, qualified.Level, kills, (books+1)*bookOffer.PriceUnits, books+1, upgrade.PageCost, amuletCount*amuletOffer.PriceUnits, amuletCount,
		amuletCount-2, manualReceipt.CommandSeq, autoReceipt.SourceSeq, time.Since(started).Round(time.Millisecond))
}

// learnNaturalTaoistSoul 从当前物理书店收齐三本书，随后只用实际掉书分解出的页交付一阶。
func learnNaturalTaoistSoul(t *testing.T, c *client, ctx context.Context, store *persistence.Store, roleID string, skill devconfig.Skill, row devconfig.SkillUpgrade,
	npc devconfig.TownNPC, offer devconfig.SkillShop, dropped domain.Item, pageRule devconfig.SkillBookRecycle) {
	t.Helper()
	books := row.BookCosts[0].Quantity
	request := c.send(p.MsgC2SVendorQuote, &pb.VendorQuoteRequest{SceneEpoch: c.entry.SceneEpoch, NpcId: npc.ID, Action: 1, OfferId: offer.ID, Quantity: books + 1})
	frame := travelUntilWithin(c, 3*time.Second, func(frame wire.Frame) bool { return frame.ID == p.MsgS2CVendorQuote && frame.Request == request })
	var quote pb.VendorQuoteResult
	must(t, proto.Unmarshal(frame.Payload, &quote))
	if quote.ItemId != skill.BookItemID || quote.BoundGold != offer.PriceUnits*(books+1) {
		t.Fatal("真实书店报价没有按所需技能书和数量定价", &quote)
	}
	buy := &pb.VendorCommitRequest{SceneEpoch: c.entry.SceneEpoch, NpcId: npc.ID, QuoteId: quote.QuoteId, Operation: c.operation()}
	purchased := c.asset(p.MsgC2SVendorCommit, buy)
	if purchased.ErrorCode != 0 {
		t.Fatal("实际技能书店交付失败", purchased)
	}
	c.inventoryAt(purchased.NewRevision)
	first := naturalGrowthItem(c, skill.BookItemID, "npc_shop")
	if first == nil || first.SourceId != domain.Hex(buy.Operation.OpId) || !first.Bound {
		t.Fatal("第一本待学习技能书没有真实书店来路")
	}
	assertNaturalTaoistPurchase(t, ctx, store, roleID, first, npc.ID, offer.PriceUnits*(books+1))
	learn := c.asset(p.MsgC2SLearnSkill, &pb.LearnSkillRequest{ItemInstanceId: first.InstanceId, SkillId: skill.ID, Operation: c.operation()})
	if learn.ErrorCode != 0 {
		t.Fatal("正常达级购得原书后未能学习火符", learn)
	}
	c.inventoryAt(learn.NewRevision)
	pages := c.skillPreview(&pb.SkillAssetPreviewRequest{Kind: 3, Materials: []*pb.ItemConsumption{{
		InstanceId: domain.Bytes(dropped.ID), Version: uint64(dropped.Version), Quantity: 1,
	}}})
	if len(pages.Outputs) != 1 || pages.Outputs[0].ItemId != row.PageItemID || pages.Outputs[0].Quantity != pageRule.PagesPerBook || !pages.Outputs[0].Bound {
		t.Fatal("普通击杀所得技能书拆页报价错误", pages)
	}
	pageCommit := &pb.SkillAssetCommitRequest{PreviewId: pages.PreviewId, Operation: c.operation()}
	if disassembled := c.skillCommit(pageCommit); disassembled.ErrorCode != 0 {
		t.Fatal("原掉书分解失败", disassembled)
	}
	materials := naturalUpgradeMaterials(c, skill.BookItemID, books, row.PageItemID, row.PageCost, &pb.SkillAssetCommitRequest{Operation: buy.Operation}, pageCommit)
	forecast := c.skillPreview(&pb.SkillAssetPreviewRequest{Kind: 2, SkillId: skill.ID, Materials: materials})
	if forecast.FromLevel != 0 || forecast.ToLevel != 1 || c.skillConfirm(forecast).ErrorCode != 0 {
		t.Fatal("真实书和掉书页未完成一阶", forecast)
	}
}

// assertNaturalTaoistPurchase 核对原商人、定价、原物品实例和审计事件。
func assertNaturalTaoistPurchase(t *testing.T, ctx context.Context, store *persistence.Store, role string, item *pb.ItemView, npc uint64, price int64) {
	t.Helper()
	var record persistence.Operation
	id := "asset:" + role + ":" + item.SourceId
	must(t, store.DB.Collection("operations").FindOne(ctx, bson.M{"_id": id, "role_id": role}).Decode(&record))
	if record.Vendor == nil || record.Code != 0 || record.Vendor.NPC != npc || record.Vendor.Plan.Wallet.BoundGold != price ||
		!slices.ContainsFunc(record.Vendor.Plan.Outputs, func(output domain.Item) bool { return output.ID == domain.Hex(item.InstanceId) }) {
		t.Fatalf("购买回执未包含原商人、价格或物品：%+v", record)
	}
	must(t, store.DB.Collection("audit_outbox").FindOne(ctx, bson.M{"_id": id, "role_id": role}).Err())
}

// assertNaturalTaoistCast 核对火符消耗与原 NPC 交付实例，以及唯一审计事件。
func assertNaturalTaoistCast(t *testing.T, ctx context.Context, store *persistence.Store, role string, query bson.M,
	item *pb.ItemView, skill uint32, scope string) persistence.Operation {
	t.Helper()
	var result persistence.Operation
	must(t, store.DB.Collection("operations").FindOne(ctx, query).Decode(&result))
	if result.RoleID != role || result.Casting == nil || result.Code != 0 || result.Casting.SkillID != skill || result.Casting.Scope != scope ||
		result.Casting.InstanceID != domain.Hex(item.InstanceId) || result.Casting.Quantity != 1 {
		t.Fatalf("持久火符回执与实际材料、来源或单次消耗不符：%+v", result)
	}
	must(t, store.DB.Collection("audit_outbox").FindOne(ctx, bson.M{"_id": result.ID, "role_id": role}).Err())
	return result
}

// reserveNaturalTaoistBag 仅在槽位不足时使用自然库存药水，为同一笔原书采购腾出格子。
func reserveNaturalTaoistBag(c *client, fixture *devconfig.Fixture, cutoff int) {
	c.t.Helper()
	for used := 0; len(c.inventory.Items)+cutoff > int(c.inventory.Capacity); used++ {
		if used >= 20 {
			c.t.Fatal("普通库存用药后仍无足够购书空间")
		}
		potion := naturalGrowthItem(c, 35, "monster_drop")
		if potion == nil {
			potion = naturalGrowthItem(c, 36, "monster_drop")
		}
		if potion == nil {
			c.t.Fatal("自然满包，未找到原掉落药水整理背包")
		}
		request := &pb.UseItemRequest{SceneEpoch: c.entry.SceneEpoch, ItemInstanceId: potion.InstanceId, ItemId: potion.ItemId, Operation: c.operation()}
		result := c.asset(p.MsgC2SUseItem, request)
		if result.ErrorCode != 0 {
			c.t.Fatalf("不能通过正常用药整理自然库存：%v", result)
		}
		c.inventoryAt(result.NewRevision)
		time.Sleep(time.Duration(fixture.Potions.UseDelayMS+50) * time.Millisecond)
	}
}
