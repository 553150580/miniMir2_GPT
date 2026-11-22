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

func TestEquipmentWearAndOrdinaryRepairMongoWS(t *testing.T) {
	if os.Getenv("MINIMIR_MONGO_E2E") != "1" {
		t.Skip("设置MINIMIR_MONGO_E2E=1以验证真实磨损与NPC修理")
	}
	cfg, err := devconfig.Load("../../../conf/dev/server.json")
	must(t, err)
	f := &cfg.Fixture
	generatedVendorCatalog := f.RuleVersion >= 25
	if f.EquipmentDurability == nil {
		// 历史未投影修理的开发版本也可运行该端到端用例；候选只作用于该随机库和进程。
		f.RuleVersion = 25
		f.EquipmentDurability = &devconfig.EquipmentDurabilityRules{
			WeaponLossU: 1, ArmorHitLossU: 1, DeathLossU: 1000, RepairRangeCells: 1,
			Ordinary: devconfig.RepairRule{Kind: 1, UpperLossDivisor: 30, MinMaxDurabilityU: 1},
		}
		f.NPCShops = []devconfig.NPCShop{{Key: "blacksmith", Roles: []string{"铁匠铺"}, CanRepair: true}}
		for i, item := range f.Items {
			switch item.ID {
			case 218:
				f.Items[i].OriginalDurabilityU, f.Items[i].RepairBasePrice = 4000, 5
			case 193:
				f.Items[i].OriginalDurabilityU, f.Items[i].RepairBasePrice = 10000, 100
			}
		}
	}
	must(t, f.Validate())
	weaponDef, _ := f.Item(218)
	// 保留合法怪物调度，让该测试的手动空挥和耐久损耗不受额外来袭干扰。
	f.Monsters[0].AttackMin, f.Monsters[0].AttackMax = 0, 0
	f.Map.MonsterSpawns[0] = devconfig.Point{X: 1, Y: 1}
	cfg.Database = "minimir_dev_game_test_wear_" + domain.ID()
	cfg.AccountAddress, cfg.GameAddress, cfg.GMAddress = address(t), address(t), address(t)

	ctx, cancel := context.WithCancel(context.Background())
	inspection, err := persistence.Open(ctx, cfg)
	must(t, err)
	var wg sync.WaitGroup
	for _, mode := range []string{"account", "game"} {
		wg.Go(func() {
			if err := bootstrap.Run(ctx, cfg, mode); err != nil && ctx.Err() == nil {
				t.Errorf("修理联调%s节点: %v", mode, err)
			}
		})
	}
	t.Cleanup(func() {
		cancel()
		wg.Wait()
		cleanup, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
		must(t, inspection.DB.Drop(cleanup))
		inspection.Close(cleanup)
	})
	for _, a := range []string{cfg.AccountAddress, cfg.GameAddress} {
		healthy(t, a)
	}
	endpoint := "http://" + cfg.AccountAddress + "/rpc"
	login, role := &pb.LoginResult{}, &pb.RoleSummary{}
	rpc(t, endpoint, "", p.MsgC2SLocalRegister, &pb.LocalLoginRequest{Account: "wear_" + domain.ID()[:10], Password: "test_123456"}, login, 0)
	rpc(t, endpoint, login.AccountToken, p.MsgC2SCreateRole, &pb.CreateRoleRequest{OriginServerId: 1, Slot: 1, Job: 0, Name: "持久" + domain.ID()[:6], OpId: domain.Bytes(domain.ID())}, role, 0)
	roleID := domain.Hex(role.Key.RoleId)
	roles := inspection.DB.Collection("roles")
	filter := bson.M{"_id": roleID}
	var created domain.Role
	must(t, roles.FindOne(ctx, filter).Decode(&created))
	var startWeapon domain.Item
	for _, item := range created.Items {
		if item.ItemID == weaponDef.ID {
			startWeapon = item
		}
	}
	if startWeapon.ID == "" || startWeapon.DurabilityU != weaponDef.OriginalDurabilityU || startWeapon.MaxDurabilityU != weaponDef.OriginalDurabilityU || startWeapon.OriginalDurabilityU != weaponDef.OriginalDurabilityU {
		t.Fatal("正常建角未持久化新品完整耐久", startWeapon)
	}
	// 仅在隔离库先置低耐久，从正常的两次已受理挥击观察损坏和维修。
	startWeapon.DurabilityU = 2 * f.EquipmentDurability.WeaponLossU
	_, err = roles.UpdateOne(ctx, filter, bson.M{"$set": bson.M{"items": []domain.Item{}, "equipped": []domain.Equipped{{Slot: 0, Item: startWeapon}}, "wallet": domain.Wallet{BoundGold: weaponDef.RepairBasePrice + 100}}})
	must(t, err)

	c := join(t, cfg.GameAddress, issue(t, endpoint, login.AccountToken, role.Key))
	defer func() { c.close() }()
	c.auto(false, 1)
	initialAttack, initialRevision := c.player().CombatAttributes.PhysicalAttackMax, c.inventory.Revision
	attack := func() {
		t.Helper()
		intent := &pb.AttackIntent{SceneEpoch: c.entry.SceneEpoch, Direction: 4}
		c.send(p.MsgC2SAttackIntent, intent)
		travelUntilWithin(c, 3*time.Second, func(frame wire.Frame) bool {
			if frame.ID != p.MsgS2CAttackAccepted {
				return false
			}
			event := &pb.AttackAccepted{}
			must(t, proto.Unmarshal(frame.Payload, event))
			return event.InputSequence == intent.InputSequence
		})
	}
	attack()
	time.Sleep(time.Duration(c.player().AttackIntervalMs)*time.Millisecond + 40*time.Millisecond)
	attack()
	c.inventoryAt(initialRevision + 1)
	broken := c.inventory.Equipped[0].Item
	if domain.Hex(broken.InstanceId) != startWeapon.ID || broken.Version != uint64(startWeapon.Version+1) || broken.DurabilityU != 0 || !broken.Bound || broken.OriginalDurabilityU != uint64(startWeapon.OriginalDurabilityU) {
		t.Fatal("两次原挥击未在一批中使原装备损坏且保留本体", broken)
	}
	travelUntilWithin(c, 3*time.Second, func(frame wire.Frame) bool {
		return frame.ID == p.MsgS2CSceneBaseline && c.player().CombatAttributes.PhysicalAttackMax < initialAttack
	})
	var wear persistence.Operation
	must(t, inspection.DB.Collection("operations").FindOne(ctx, bson.M{"role_id": roleID, "settlement.equipment_wear.cost.instance_id": startWeapon.ID}).Decode(&wear))
	if costs := wear.Settlement.EquipmentWear; len(costs) != 1 || costs[0].Cost.AttackU != startWeapon.DurabilityU || costs[0].After.DurabilityU != 0 || wear.CommandSeq != 0 {
		t.Fatal("真实持久记录未按原实例与来源确认合批磨损", wear)
	}

	travelToBaseline(c, 1, 4)
	travelAcknowledge(c, true)
	const blacksmithID = 4003
	request := &pb.VendorQuoteRequest{SceneEpoch: c.entry.SceneEpoch, NpcId: blacksmithID, Action: 3, InstanceId: broken.InstanceId, ItemVersion: broken.Version, Quantity: 1}
	travelExpectError(c, p.MsgC2SVendorQuote, c.send(p.MsgC2SVendorQuote, request), p.CodeVendorUnavailable)
	nearby := &pb.GridPoint{X: 11, Y: 5}
	c.send(p.MsgC2SMoveIntent, &pb.MoveIntent{SceneEpoch: c.entry.SceneEpoch, Destination: nearby})
	travelUntilWithin(c, 9*time.Second, func(frame wire.Frame) bool {
		return frame.ID == p.MsgS2CSceneBaseline && proto.Equal(c.player().Position, nearby) && c.scene.NavigationState.GetStatus() == 2
	})
	quoteRequest := c.send(p.MsgC2SVendorQuote, request)
	quoted := travelUntilWithin(c, 3*time.Second, func(frame wire.Frame) bool { return frame.ID == p.MsgS2CVendorQuote && frame.Request == quoteRequest })
	quote := &pb.VendorQuoteResult{}
	must(t, proto.Unmarshal(quoted.Payload, quote))
	upper := max(f.EquipmentDurability.Ordinary.MinMaxDurabilityU, startWeapon.MaxDurabilityU-startWeapon.MaxDurabilityU/f.EquipmentDurability.Ordinary.UpperLossDivisor)
	if len(quote.QuoteId) != 16 || quote.ItemVersion != broken.Version || quote.DurabilityU != 0 || quote.RepairedMaxDurabilityU != uint64(upper) || quote.RepairedDurabilityU != uint64(upper) ||
		quote.OriginalDurabilityU != broken.OriginalDurabilityU || quote.BoundGold != weaponDef.RepairBasePrice || quote.Gold != 0 || quote.Ingot != 0 || quote.BoundIngot != 0 || !quote.Bound {
		t.Fatal("NPC普修报价未包含正确缺额、上限、绑定和完整钱包", quote)
	}
	commit := &pb.VendorCommitRequest{Operation: c.operation(), QuoteId: quote.QuoteId, SceneEpoch: c.entry.SceneEpoch, NpcId: blacksmithID}
	result := c.asset(p.MsgC2SVendorCommit, commit)
	if result.ErrorCode != 0 {
		t.Fatal("真实普通修理提交失败", result)
	}
	c.inventoryAt(result.NewRevision)
	repaired := c.inventory.Equipped[0].Item
	if !repaired.Bound || repaired.Version != broken.Version+1 || repaired.DurabilityU != uint64(upper) || domain.Hex(repaired.InstanceId) != startWeapon.ID {
		t.Fatal("修理没有恢复穿戴中的原装备", repaired)
	}
	travelUntilWithin(c, 3*time.Second, func(frame wire.Frame) bool {
		return frame.ID == p.MsgS2CSceneBaseline && c.player().CombatAttributes.PhysicalAttackMax == initialAttack
	})
	var confirmed domain.Role
	must(t, roles.FindOne(ctx, filter).Decode(&confirmed))
	if repeat := c.asset(p.MsgC2SVendorCommit, commit); repeat.ErrorCode != 0 || repeat.CommandSeq != result.CommandSeq {
		t.Fatal("同一连接重放原维修未返回原结果", repeat)
	}
	c.close()
	c = join(t, cfg.GameAddress, issue(t, endpoint, login.AccountToken, role.Key))
	if replay := c.asset(p.MsgC2SVendorCommit, commit); replay.ErrorCode != 0 || replay.CommandSeq != result.CommandSeq {
		t.Fatal("健康新票无法核对原维修或重复扣款", replay)
	}
	var persisted domain.Role
	must(t, roles.FindOne(ctx, filter).Decode(&persisted))
	if persisted.Wallet.BoundGold != 100 || persisted.Equipped[0].Item.DurabilityU != upper || persisted.EconomicSeq != int64(result.CommandSeq) || persisted.SourceSeq != confirmed.SourceSeq {
		t.Fatal("持久装备、钱包或水位被原请求重放覆盖", persisted.Wallet, persisted.Equipped, persisted.EconomicSeq, persisted.SourceSeq, confirmed.SourceSeq)
	}
	opID := "asset:" + roleID + ":" + domain.Hex(commit.Operation.OpId)
	audits, err := inspection.DB.Collection("audit_outbox").CountDocuments(ctx, bson.M{"_id": opID})
	must(t, err)
	if audits != 1 {
		t.Fatal("同一维修产生了多个确认审计，或缺少原审计", audits)
	}
	t.Log("真实WS覆盖两次合批磨损、损坏失效、相邻铁匠报价、原实例修复、扣款与健康新票防重")
	if !generatedVendorCatalog {
		return
	}
	shop := f.NPCShopByRole("铁匠铺")
	if shop == nil {
		t.Fatal("规则25缺少实际铁匠商店")
	}
	offer, exists := shop.Offer(218)
	resale := shop.SellPrice(218)
	if !exists || offer.ItemID != 218 || offer.Currency != "gold" || offer.OutputBound || offer.Quantity != 1 || resale <= 0 || persisted.Wallet.BoundGold < offer.PriceUnits {
		t.Fatal("规则25木剑报价不满足真实绑定钱包回归的前态", offer, resale)
	}
	redeem := func(q *pb.VendorQuoteRequest, amount int64) *pb.VendorCommitRequest {
		t.Helper()
		q.SceneEpoch, q.NpcId, q.Quantity = c.entry.SceneEpoch, blacksmithID, 1
		sequence := c.send(p.MsgC2SVendorQuote, q)
		f := travelUntilWithin(c, 3*time.Second, func(f wire.Frame) bool { return f.ID == p.MsgS2CVendorQuote && f.Request == sequence })
		var quote pb.VendorQuoteResult
		must(t, proto.Unmarshal(f.Payload, &quote))
		if quote.Action != q.Action || !quote.Bound || quote.BoundGold != amount || quote.Gold != 0 || quote.ItemId != 218 || len(quote.QuoteId) != 16 {
			t.Fatal("铁匠买卖报价或绑定钱包不一致", &quote)
		}
		request := &pb.VendorCommitRequest{Operation: c.operation(), QuoteId: quote.QuoteId, SceneEpoch: q.SceneEpoch, NpcId: q.NpcId}
		result := c.asset(p.MsgC2SVendorCommit, request)
		if result.ErrorCode != 0 || result.CommandSeq != request.Operation.CommandSeq {
			t.Fatal("铁匠买卖提交未沿原操作完成", result)
		}
		c.inventoryAt(result.NewRevision)
		return request
	}
	buy := redeem(&pb.VendorQuoteRequest{Action: 1, OfferId: offer.ID}, offer.PriceUnits)
	if len(c.inventory.Items) != 1 {
		t.Fatal("买一把木剑未恰好交付一件新实例")
	}
	bought := c.inventory.Items[0]
	if domain.Hex(bought.InstanceId) == startWeapon.ID || len(bought.InstanceId) != 16 || !bought.Bound || bought.SourceKind != "npc_shop" || bought.DurabilityU != uint64(weaponDef.OriginalDurabilityU) {
		t.Fatal("买到的木剑身份、绑定、来源或新品耐久错误", bought)
	}
	sell := redeem(&pb.VendorQuoteRequest{Action: 2, InstanceId: bought.InstanceId, ItemVersion: bought.Version}, resale)
	for _, trade := range []*pb.VendorCommitRequest{buy, sell} {
		if repeat := c.asset(p.MsgC2SVendorCommit, trade); repeat.ErrorCode != 0 {
			t.Fatal("买卖已结算后不能按原操作对账", repeat)
		}
		id := "asset:" + roleID + ":" + domain.Hex(trade.Operation.OpId)
		var receipt persistence.Operation
		must(t, inspection.DB.Collection("operations").FindOne(ctx, bson.M{"_id": id}).Decode(&receipt))
		audits, err := inspection.DB.Collection("audit_outbox").CountDocuments(ctx, bson.M{"_id": id})
		must(t, err)
		if audits != 1 || receipt.Vendor == nil || receipt.Vendor.QuoteID != domain.Hex(trade.QuoteId) || receipt.Vendor.NPC != blacksmithID || receipt.CommandSeq != int64(trade.Operation.CommandSeq) {
			t.Fatal("买卖原报价、操作序号和独立审计不一致", receipt)
		}
	}
	must(t, roles.FindOne(ctx, filter).Decode(&persisted))
	if len(persisted.Items) != 0 || persisted.Equipped[0].Item.ID != startWeapon.ID || persisted.Wallet.Gold != 0 || persisted.Wallet.BoundGold != 100-offer.PriceUnits+resale || persisted.EconomicSeq != int64(sell.Operation.CommandSeq) {
		t.Fatal("买卖重放影响了原装备、钱包或经济流水", persisted.Wallet, persisted.Items)
	}
	t.Log("真实WS覆盖铁匠按生成目录出售木剑、绑定金币买入与卖回、买卖分别按原操作防重")
}
