// 通过真实取票和 WebSocket 验证战斗磨损、普通修理、费用和持久装备状态。

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

// TestEquipmentWearAndOrdinaryRepairMongoWS 触发实际战斗磨损并从铁匠修理，核对耐久和钱包的版本变更。
// t 提供断言失败报告、子用例和清理注册。
func TestEquipmentWearAndOrdinaryRepairMongoWS(t *testing.T) {
	if os.Getenv("MINIMIR_MONGO_E2E") != "1" {
		t.Skip("设置MINIMIR_MONGO_E2E=1以验证真实磨损与NPC修理")
	}
	// cfg 保存由加载取得的服务配置，err 接收加载过程中可能产生的错误，供后续失败分支处理。
	cfg, err := devconfig.Load("../../../conf/dev/server.json")
	must(t, err)
	// f 保存当前处理所用的开发测试配置。
	f := &cfg.Fixture
	// generatedVendorCatalog 保存根据规则版本与当前数据计算的“generated”商人目录。
	generatedVendorCatalog := f.RuleVersion >= 25
	if f.EquipmentDurability == nil {
		// 历史未投影修理的开发版本也可运行该端到端用例；候选只作用于该随机库和进程。
		f.RuleVersion = 25
		f.EquipmentDurability = &devconfig.EquipmentDurabilityRules{
			WeaponLossU: 1, ArmorHitLossU: 1, DeathLossU: 1000, RepairRangeCells: 1,
			Ordinary: devconfig.RepairRule{Kind: 1, UpperLossDivisor: 30, MinMaxDurabilityU: 1},
		}
		f.NPCShops = []devconfig.NPCShop{{Key: "blacksmith", Roles: []string{"铁匠铺"}, CanRepair: true}}
		// i 定位道具集合中正在处理的成员，item 接收遍历道具集合时当前的元素。
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
	// weaponDef 保存由道具取得的“weapon”“def”。
	weaponDef, _ := f.Item(218)
	// 保留合法怪物调度，让该测试的手动空挥和耐久损耗不受额外来袭干扰。
	f.Monsters[0].AttackMin, f.Monsters[0].AttackMax = 0, 0
	f.Map.MonsterSpawns[0] = devconfig.Point{X: 1, Y: 1}
	cfg.Database = "minimir_dev_game_test_wear_" + domain.ID()
	cfg.AccountAddress, cfg.GameAddress, cfg.GMAddress = address(t), address(t), address(t)

	// ctx 传递从调用方派生的上下文，供取消或控制截止时间，cancel 取消对应上下文或停止计时，释放其关联资源。
	ctx, cancel := context.WithCancel(context.Background())
	// inspection 保存由打开取得的检查。
	inspection, err := persistence.Open(ctx, cfg)
	must(t, err)
	// wg 记录“wg”，使用协程汇合计数器表示。
	var wg sync.WaitGroup
	// mode 接收遍历文本列表时当前的元素。
	for _, mode := range []string{"account", "game"} {
		wg.Go(func() {
			// err 接收运行批次过程中可能产生的错误，供后续失败分支处理。
			if err := bootstrap.Run(ctx, cfg, mode); err != nil && ctx.Err() == nil {
				t.Errorf("修理联调%s节点: %v", mode, err)
			}
		})
	}
	t.Cleanup(func() {
		cancel()
		wg.Wait()
		// cleanup 传递从调用方派生的上下文，供取消或控制截止时间，stop 取消对应上下文或停止计时，释放其关联资源。
		cleanup, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
		must(t, inspection.DB.Drop(cleanup))
		inspection.Close(cleanup)
	})
	// a 接收遍历文本列表时当前的元素。
	for _, a := range []string{cfg.AccountAddress, cfg.GameAddress} {
		healthy(t, a)
	}
	// endpoint 保存根据当前数据与账号地址的计算结果与当前数据计算的端点。
	endpoint := "http://" + cfg.AccountAddress + "/rpc"
	// login 保存当前处理所用的登录，role 保存当前处理所用的角色。
	login, role := &pb.LoginResult{}, &pb.RoleSummary{}
	rpc(t, endpoint, "", p.MsgC2SLocalRegister, &pb.LocalLoginRequest{Account: "wear_" + domain.ID()[:10], Password: "test_123456"}, login, 0)
	rpc(t, endpoint, login.AccountToken, p.MsgC2SCreateRole, &pb.CreateRoleRequest{OriginServerId: 1, Slot: 1, Job: 0, Name: "持久" + domain.ID()[:6], OpId: domain.Bytes(domain.ID())}, role, 0)
	// roleID 标识当前业务使用的角色。
	roleID := domain.Hex(role.Key.RoleId)
	// roles 保存由集合取得的角色集合。
	roles := inspection.DB.Collection("roles")
	// filter 初始化用作筛选条件的MongoDB BSON 文档。
	filter := bson.M{"_id": roleID}
	// created 记录“created”，使用角色表示。
	var created domain.Role
	must(t, roles.FindOne(ctx, filter).Decode(&created))
	// startWeapon 记录起点或开始时间“weapon”，使用道具表示。
	var startWeapon domain.Item
	// item 接收遍历道具集合时当前的元素。
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

	// c 接收加入供后续步骤使用。
	c := join(t, cfg.GameAddress, issue(t, endpoint, login.AccountToken, role.Key))
	defer func() { c.close() }()
	c.auto(false, 1)
	// initialAttack 保存物理攻击最大，用于当前步骤中的初始值攻击，initialRevision 保存修订号，用于当前步骤中的初始值修订号。
	initialAttack, initialRevision := c.player().CombatAttributes.PhysicalAttackMax, c.inventory.Revision
	// attack 封装攻击对应的后续处理。
	attack := func() {
		t.Helper()
		// intent 保存当前处理所用的意图。
		intent := &pb.AttackIntent{SceneEpoch: c.entry.SceneEpoch, Direction: 4}
		c.send(p.MsgC2SAttackIntent, intent)
		// frame 记录协议帧。
		travelUntilWithin(c, 3*time.Second, func(frame wire.Frame) bool {
			if frame.ID != p.MsgS2CAttackAccepted {
				return false
			}
			// event 保存当前处理所用的事件。
			event := &pb.AttackAccepted{}
			must(t, proto.Unmarshal(frame.Payload, event))
			return event.InputSequence == intent.InputSequence
		})
	}
	attack()
	time.Sleep(time.Duration(c.player().AttackIntervalMs)*time.Millisecond + 40*time.Millisecond)
	attack()
	c.inventoryAt(initialRevision + 1)
	// broken 保存道具，用于当前步骤中的“broken”。
	broken := c.inventory.Equipped[0].Item
	if domain.Hex(broken.InstanceId) != startWeapon.ID || broken.Version != uint64(startWeapon.Version+1) || broken.DurabilityU != 0 || !broken.Bound || broken.OriginalDurabilityU != uint64(startWeapon.OriginalDurabilityU) {
		t.Fatal("两次原挥击未在一批中使原装备损坏且保留本体", broken)
	}
	// frame 记录协议帧。
	travelUntilWithin(c, 3*time.Second, func(frame wire.Frame) bool {
		return frame.ID == p.MsgS2CSceneBaseline && c.player().CombatAttributes.PhysicalAttackMax < initialAttack
	})
	// wear 记录磨损，使用操作表示。
	var wear persistence.Operation
	must(t, inspection.DB.Collection("operations").FindOne(ctx, bson.M{"role_id": roleID, "settlement.equipment_wear.cost.instance_id": startWeapon.ID}).Decode(&wear))
	// costs 保存装备磨损，用于当前步骤中的“costs”。
	if costs := wear.Settlement.EquipmentWear; len(costs) != 1 || costs[0].Cost.AttackU != startWeapon.DurabilityU || costs[0].After.DurabilityU != 0 || wear.CommandSeq != 0 {
		t.Fatal("真实持久记录未按原实例与来源确认合批磨损", wear)
	}

	travelToBaseline(c, 1, 4)
	travelAcknowledge(c, true)
	// blacksmithID 标识当前业务使用的“blacksmith”。
	const blacksmithID = 4003
	// request 保存当前处理所用的请求。
	request := &pb.VendorQuoteRequest{SceneEpoch: c.entry.SceneEpoch, NpcId: blacksmithID, Action: 3, InstanceId: broken.InstanceId, ItemVersion: broken.Version, Quantity: 1}
	travelExpectError(c, p.MsgC2SVendorQuote, c.send(p.MsgC2SVendorQuote, request), p.CodeVendorUnavailable)
	// nearby 保存当前处理所用的“nearby”。
	nearby := &pb.GridPoint{X: 11, Y: 5}
	c.send(p.MsgC2SMoveIntent, &pb.MoveIntent{SceneEpoch: c.entry.SceneEpoch, Destination: nearby})
	// frame 记录协议帧。
	travelUntilWithin(c, 9*time.Second, func(frame wire.Frame) bool {
		return frame.ID == p.MsgS2CSceneBaseline && proto.Equal(c.player().Position, nearby) && c.scene.NavigationState.GetStatus() == 2
	})
	// quoteRequest 保存由发送取得的报价请求。
	quoteRequest := c.send(p.MsgC2SVendorQuote, request)
	// quoted 保存由地图切换有效截止时间“within”取得的“quoted”，frame 记录协议帧。
	quoted := travelUntilWithin(c, 3*time.Second, func(frame wire.Frame) bool { return frame.ID == p.MsgS2CVendorQuote && frame.Request == quoteRequest })
	// quote 保存当前处理所用的报价。
	quote := &pb.VendorQuoteResult{}
	must(t, proto.Unmarshal(quoted.Payload, quote))
	// upper 保存由最大取得的较高值。
	upper := max(f.EquipmentDurability.Ordinary.MinMaxDurabilityU, startWeapon.MaxDurabilityU-startWeapon.MaxDurabilityU/f.EquipmentDurability.Ordinary.UpperLossDivisor)
	if len(quote.QuoteId) != 16 || quote.ItemVersion != broken.Version || quote.DurabilityU != 0 || quote.RepairedMaxDurabilityU != uint64(upper) || quote.RepairedDurabilityU != uint64(upper) ||
		quote.OriginalDurabilityU != broken.OriginalDurabilityU || quote.BoundGold != weaponDef.RepairBasePrice || quote.Gold != 0 || quote.Ingot != 0 || quote.BoundIngot != 0 || !quote.Bound {
		t.Fatal("NPC普修报价未包含正确缺额、上限、绑定和完整钱包", quote)
	}
	// commit 保存当前处理所用的提交。
	commit := &pb.VendorCommitRequest{Operation: c.operation(), QuoteId: quote.QuoteId, SceneEpoch: c.entry.SceneEpoch, NpcId: blacksmithID}
	// result 保存由资产取得的结果。
	result := c.asset(p.MsgC2SVendorCommit, commit)
	if result.ErrorCode != 0 {
		t.Fatal("真实普通修理提交失败", result)
	}
	c.inventoryAt(result.NewRevision)
	// repaired 保存道具，用于当前步骤中的已修理。
	repaired := c.inventory.Equipped[0].Item
	if !repaired.Bound || repaired.Version != broken.Version+1 || repaired.DurabilityU != uint64(upper) || domain.Hex(repaired.InstanceId) != startWeapon.ID {
		t.Fatal("修理没有恢复穿戴中的原装备", repaired)
	}
	// frame 记录协议帧。
	travelUntilWithin(c, 3*time.Second, func(frame wire.Frame) bool {
		return frame.ID == p.MsgS2CSceneBaseline && c.player().CombatAttributes.PhysicalAttackMax == initialAttack
	})
	// confirmed 记录已确认，使用角色表示。
	var confirmed domain.Role
	must(t, roles.FindOne(ctx, filter).Decode(&confirmed))
	// repeat 保存由资产取得的重复。
	if repeat := c.asset(p.MsgC2SVendorCommit, commit); repeat.ErrorCode != 0 || repeat.CommandSeq != result.CommandSeq {
		t.Fatal("同一连接重放原维修未返回原结果", repeat)
	}
	c.close()
	c = join(t, cfg.GameAddress, issue(t, endpoint, login.AccountToken, role.Key))
	// replay 保存由资产取得的重放。
	if replay := c.asset(p.MsgC2SVendorCommit, commit); replay.ErrorCode != 0 || replay.CommandSeq != result.CommandSeq {
		t.Fatal("健康新票无法核对原维修或重复扣款", replay)
	}
	// persisted 记录“persisted”，使用角色表示。
	var persisted domain.Role
	must(t, roles.FindOne(ctx, filter).Decode(&persisted))
	if persisted.Wallet.BoundGold != 100 || persisted.Equipped[0].Item.DurabilityU != upper || persisted.EconomicSeq != int64(result.CommandSeq) || persisted.SourceSeq != confirmed.SourceSeq {
		t.Fatal("持久装备、钱包或水位被原请求重放覆盖", persisted.Wallet, persisted.Equipped, persisted.EconomicSeq, persisted.SourceSeq, confirmed.SourceSeq)
	}
	// opID 标识当前业务使用的操作。
	opID := "asset:" + roleID + ":" + domain.Hex(commit.Operation.OpId)
	// audits 保存由数量文档取得的审计记录集合。
	audits, err := inspection.DB.Collection("audit_outbox").CountDocuments(ctx, bson.M{"_id": opID})
	must(t, err)
	if audits != 1 {
		t.Fatal("同一维修产生了多个确认审计，或缺少原审计", audits)
	}
	t.Log("真实WS覆盖两次合批磨损、损坏失效、相邻铁匠报价、原实例修复、扣款与健康新票防重")
	if !generatedVendorCatalog {
		return
	}
	// shop 保存由NPC商店“by”角色取得的商店。
	shop := f.NPCShopByRole("铁匠铺")
	if shop == nil {
		t.Fatal("规则25缺少实际铁匠商店")
	}
	// offer 接收尝试提交供后续步骤使用，exists 保存由尝试提交取得的存在。
	offer, exists := shop.Offer(218)
	// resale 保存由“sell”“price”取得的“resale”。
	resale := shop.SellPrice(218)
	if !exists || offer.ItemID != 218 || offer.Currency != "gold" || offer.OutputBound || offer.Quantity != 1 || resale <= 0 || persisted.Wallet.BoundGold < offer.PriceUnits {
		t.Fatal("规则25木剑报价不满足真实绑定钱包回归的前态", offer, resale)
	}
	// redeem 封装“redeem”对应的后续处理，q 记录商人报价请求，amount 记录数量，使用数值表示。
	redeem := func(q *pb.VendorQuoteRequest, amount int64) *pb.VendorCommitRequest {
		t.Helper()
		q.SceneEpoch, q.NpcId, q.Quantity = c.entry.SceneEpoch, blacksmithID, 1
		// sequence 保存由发送取得的序号。
		sequence := c.send(p.MsgC2SVendorQuote, q)
		// f 接收地图切换有效截止时间“within”供后续步骤使用，f 记录协议帧。
		f := travelUntilWithin(c, 3*time.Second, func(f wire.Frame) bool { return f.ID == p.MsgS2CVendorQuote && f.Request == sequence })
		// quote 记录报价，使用商人报价结果表示。
		var quote pb.VendorQuoteResult
		must(t, proto.Unmarshal(f.Payload, &quote))
		if quote.Action != q.Action || !quote.Bound || quote.BoundGold != amount || quote.Gold != 0 || quote.ItemId != 218 || len(quote.QuoteId) != 16 {
			t.Fatal("铁匠买卖报价或绑定钱包不一致", &quote)
		}
		// request 保存当前处理所用的请求。
		request := &pb.VendorCommitRequest{Operation: c.operation(), QuoteId: quote.QuoteId, SceneEpoch: q.SceneEpoch, NpcId: q.NpcId}
		// result 保存由资产取得的结果。
		result := c.asset(p.MsgC2SVendorCommit, request)
		if result.ErrorCode != 0 || result.CommandSeq != request.Operation.CommandSeq {
			t.Fatal("铁匠买卖提交未沿原操作完成", result)
		}
		c.inventoryAt(result.NewRevision)
		return request
	}
	// buy 保存由“redeem”取得的“buy”。
	buy := redeem(&pb.VendorQuoteRequest{Action: 1, OfferId: offer.ID}, offer.PriceUnits)
	if len(c.inventory.Items) != 1 {
		t.Fatal("买一把木剑未恰好交付一件新实例")
	}
	// bought 保存从道具集合取出的“bought”。
	bought := c.inventory.Items[0]
	if domain.Hex(bought.InstanceId) == startWeapon.ID || len(bought.InstanceId) != 16 || !bought.Bound || bought.SourceKind != "npc_shop" || bought.DurabilityU != uint64(weaponDef.OriginalDurabilityU) {
		t.Fatal("买到的木剑身份、绑定、来源或新品耐久错误", bought)
	}
	// sell 保存由“redeem”取得的“sell”。
	sell := redeem(&pb.VendorQuoteRequest{Action: 2, InstanceId: bought.InstanceId, ItemVersion: bought.Version}, resale)
	// trade 接收遍历商人提交请求列表时当前的元素。
	for _, trade := range []*pb.VendorCommitRequest{buy, sell} {
		// repeat 保存由资产取得的重复。
		if repeat := c.asset(p.MsgC2SVendorCommit, trade); repeat.ErrorCode != 0 {
			t.Fatal("买卖已结算后不能按原操作对账", repeat)
		}
		// id 区分当前步骤指向的目标对象。
		id := "asset:" + roleID + ":" + domain.Hex(trade.Operation.OpId)
		// receipt 记录回执，使用操作表示。
		var receipt persistence.Operation
		must(t, inspection.DB.Collection("operations").FindOne(ctx, bson.M{"_id": id}).Decode(&receipt))
		// audits 保存由数量文档取得的审计记录集合，err 接收数量文档过程中可能产生的错误，供后续失败分支处理。
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
