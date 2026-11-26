// 本文件用隔离 MongoDB 和真实 WebSocket 验证道具竞购、交付、退款及定点佣金。
package integration

import (
	"bytes"
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

const (
	marketListAction      uint32 = iota + 1 // 背包原版本道具上架。
	marketBidAction                         // 固定原显示价格的整单预购。
	marketRaiseAction                       // 已有预购者确认服务器加价。
	marketDirectBuyAction                   // 直购期按数量成交。
	marketCancelAction                      // 卖家撤回当前订单。
	marketClaimAction                       // 领取完整托管项目。
)

// marketTestRole 同一隔离用例中可通过新票重新进入的真实角色。
type marketTestRole struct {
	key    *pb.RoleKey // 原建角响应返回的永久身份。
	client *client     // 当前真实 WebSocket 游戏连接。
	attach func()      // 使用同一账号和角色签发新票。
}

// TestItemMarketLifecycleMongoWS 验证上架、竞购、加价、到期结算、直购和回执重放守恒。
func TestItemMarketLifecycleMongoWS(t *testing.T) {
	const marketItem = uint32(35)
	ctx, store, endpoint, route := startChallengePlayerDeathServer(t, func(*devconfig.Config) {})
	rules := store.Cfg.Fixture.ItemMarket
	if rules == nil || rules.AuctionSeconds != 7200 || rules.BuyoutSeconds != 86400 || rules.BidRaiseBP != 1000 ||
		rules.ExtendWhenRemainingBelowSeconds != 1200 || rules.ExtendToRemainingSeconds != 1200 || rules.TaxDivisor != 10 {
		t.Fatalf("实际开发规则未载入已确认市场参数：%+v", rules)
	}
	definition, known := store.Cfg.Fixture.Item(marketItem)
	if !known || !definition.Stackable || definition.MaxStack < 8 || definition.IsEquipment {
		t.Fatal("隔离验收需要真实已装配且可堆叠的普通药水", definition)
	}
	newAccount := func() string {
		t.Helper()
		login := &pb.LoginResult{}
		rpc(t, endpoint, "", p.MsgC2SLocalRegister,
			&pb.LocalLoginRequest{Account: "marketws_" + domain.ID()[:8], Password: "test_123456"}, login, 0)
		return login.AccountToken
	}
	seed := func(token string, slot uint32, yuanbao int64, items ...domain.Item) *marketTestRole {
		t.Helper()
		role := &pb.RoleSummary{}
		rpc(t, endpoint, token, p.MsgC2SCreateRole, &pb.CreateRoleRequest{OriginServerId: route.origin.OriginID,
			Slot: slot, Job: 0, Name: "市集" + domain.ID()[:6], OpId: domain.Bytes(domain.ID())}, role, 0)
		id := domain.Hex(role.Key.RoleId)
		// 只有隔离资金、交易前提和源道具在第一次附着前准备。此后所有经济动作通过正常协议提交。
		stored, err := store.Role(ctx, id)
		must(t, err)
		wallet := domain.Wallet{Yuanbao: yuanbao, BoundYuanbao: 9_999, Gold: 11_111, BoundGold: 12_222}
		result, err := store.DB.Collection("roles").UpdateByID(ctx, id, bson.M{"$set": bson.M{
			"wallet": wallet, "items": append(stored.Items, items...), "privilege_until": time.Now().Add(time.Hour),
		}})
		must(t, err)
		if result.ModifiedCount != 1 {
			t.Fatal("未准备此隔离角色的市场输入", role.Name)
		}
		prepared := &marketTestRole{key: role.Key}
		prepared.attach = func() {
			if prepared.client != nil {
				prepared.client.close()
			}
			ticket := directoryTicket(t, endpoint, token, prepared.key, route)
			connected := join(t, directoryTicketAddress(t, ticket), ticket.Ticket)
			t.Cleanup(connected.close)
			connected.auto(false, 1)
			prepared.client = connected
		}
		prepared.attach()
		return prepared
	}
	readRole := func(role *marketTestRole) domain.Role {
		t.Helper()
		value, err := store.Role(ctx, domain.Hex(role.key.RoleId))
		must(t, err)
		return value
	}
	readOrder := func(orderID []byte) persistence.ItemMarketOrder {
		t.Helper()
		var value persistence.ItemMarketOrder
		must(t, store.DB.Collection("item_orders").FindOne(ctx, bson.M{"_id": domain.Hex(orderID)}).Decode(&value))
		return value
	}
	shorten := func(orderID []byte, field string) {
		t.Helper()
		order := readOrder(orderID)
		// 先验证正式配置生成的截止；只缩短此独立历史订单的真实截止以触发实际排程。
		result, err := store.DB.Collection("item_orders").UpdateOne(ctx, bson.M{"_id": order.ID, "revision": order.Revision},
			bson.M{"$set": bson.M{field: time.Now().Add(3 * time.Second).UTC().Truncate(time.Millisecond)}})
		must(t, err)
		if result.ModifiedCount != 1 {
			t.Fatal("隔离订单的原阶段或修订发生变化", order.ID, order.Revision)
		}
	}

	stock := domain.Item{ID: domain.ID(), ItemID: marketItem, Quantity: 8, Version: 1, SourceKind: "monster_drop", SourceID: domain.ID()}
	bound := domain.Item{ID: domain.ID(), ItemID: marketItem, Quantity: 1, Bound: true, Version: 1, SourceKind: "monster_drop", SourceID: domain.ID()}
	returned := domain.Item{ID: domain.ID(), ItemID: marketItem, Quantity: 1, Version: 1, SourceKind: "monster_drop", SourceID: domain.ID()}
	limitStock := domain.Item{ID: domain.ID(), ItemID: marketItem, Quantity: 4, Version: 1, SourceKind: "monster_drop", SourceID: domain.ID()}
	sellerLogin, buyerLogin := newAccount(), newAccount()
	seller := seed(sellerLogin, 1, 1_000, stock, bound, returned)
	sellerAlt := seed(sellerLogin, 2, 1_000, limitStock)
	firstBuyer := seed(buyerLogin, 1, 1_000)
	buyerAlt := seed(buyerLogin, 2, 5)
	winner := seed(newAccount(), 1, 1_000)
	if state := marketStateWS(seller.client, 1, marketItem); !state.Enabled || !state.CanTrade || len(state.Orders) != 0 {
		t.Fatal("初始独立市场目录错误", state)
	}
	list := func(item *pb.ItemView, quantity, unitPrice int64, code uint32) *pb.ItemMarketReceipt {
		t.Helper()
		return marketActionWS(seller.client, &pb.ItemMarketAction{Action: marketListAction, ItemInstanceId: item.InstanceId,
			ItemVersion: item.Version, Quantity: quantity, UnitPriceYuanbaoUnits: unitPrice}, code)
	}
	list(bound.Proto(), 1, 1, p.CodeVendorItemBlocked)
	auction := list(stock.Proto(), 3, 11, 0)
	listed := readOrder(auction.OrderId)
	if auction.Quantity != 3 || listed.AuctionEndsAt.Sub(listed.ListedAt) != 2*time.Hour || listed.BuyoutDuration != 24*time.Hour ||
		listed.TaxDivisor != 10 || listed.Item.Bound || listed.Item.SourceID != stock.SourceID {
		t.Fatalf("上架没有冻结原物品、来源或正式期限：回执=%v，订单=%+v", auction, listed)
	}
	view := marketOrderWS(sellerAlt.client, auction.OrderId, persistence.ItemMarketAuction)
	if !view.SameAccount || view.Owned || view.NextUnitPriceYuanbaoUnits != 13 {
		t.Fatal("账号关系或 0.1 元宝步长的加价确认不正确", view)
	}
	marketActionWS(sellerAlt.client, termsForMarketAction(view, marketBidAction, 3), p.CodeSelfTradeForbidden)
	marketActionWS(buyerAlt.client, termsForMarketAction(marketOrderWS(buyerAlt.client, auction.OrderId, persistence.ItemMarketAuction), marketBidAction, 3), p.CodeVendorFunds)
	marketActionWS(firstBuyer.client, termsForMarketAction(marketOrderWS(firstBuyer.client, auction.OrderId, persistence.ItemMarketAuction), marketBidAction, 3), 0)
	marketActionWS(buyerAlt.client, termsForMarketAction(marketOrderWS(buyerAlt.client, auction.OrderId, persistence.ItemMarketAuction), marketBidAction, 3), p.CodeOperationConflict)
	winnerTerms := marketOrderWS(winner.client, auction.OrderId, persistence.ItemMarketAuction)
	marketActionWS(winner.client, termsForMarketAction(winnerTerms, marketRaiseAction, 3), p.CodeOperationConflict)
	marketActionWS(winner.client, termsForMarketAction(winnerTerms, marketBidAction, 3), 0)
	marketActionWS(seller.client, termsForMarketAction(marketOrderWS(seller.client, auction.OrderId, persistence.ItemMarketAuction), marketCancelAction, 3), p.CodeOperationConflict)
	if got := readRole(firstBuyer).Wallet.Yuanbao; got != 967 || readRole(winner).Wallet.Yuanbao != 967 {
		t.Fatal("两笔预购没有各按 3×11 全额冻结", got)
	}
	marketActionWS(winner.client, termsForMarketAction(winnerTerms, marketRaiseAction, 3), p.CodeOperationConflict)
	shorten(auction.OrderId, "auction_ends_at")
	raised := marketActionWS(winner.client, termsForMarketAction(marketOrderWS(winner.client, auction.OrderId, persistence.ItemMarketAuction), marketRaiseAction, 3), 0)
	afterRaise := readOrder(auction.OrderId)
	if raised.UnitPriceYuanbaoUnits != 13 || raised.YuanbaoDeltaUnits != -6 || afterRaise.ActiveBidCount != 1 ||
		time.Until(afterRaise.AuctionEndsAt) < 19*time.Minute+55*time.Second || time.Until(afterRaise.AuctionEndsAt) > 20*time.Minute {
		t.Fatalf("加价未只冻结差价、清除其他预购并延至二十分钟：回执=%v，订单=%+v", raised, afterRaise)
	}
	refund := marketEscrowWS(firstBuyer.client, auction.OrderId, persistence.ItemMarketClaimRefund)
	if refund.YuanbaoUnits != 33 {
		t.Fatal("失败预购未保留整笔原冻结退款", refund)
	}
	refundRequest := itemMarketClaimForTest(refund)
	refundReceipt := marketActionWS(firstBuyer.client, refundRequest, 0)
	if refundReceipt.YuanbaoDeltaUnits != 33 || readRole(firstBuyer).Wallet.Yuanbao != 1_000 {
		t.Fatal("原买家未取回完整预购资金", refundReceipt)
	}

	// 真实到期由市场运行服自动推进，已退出活跃资格的赢家仍有原物品可领。
	shorten(auction.OrderId, "auction_ends_at")
	expireMarketTestPrivilege(t, ctx, store, winner)
	prize := marketEscrowWS(winner.client, auction.OrderId, persistence.ItemMarketClaimItem)
	if prize.GetItem().GetQuantity() != 3 || prize.GetItem().GetSourceId() != stock.SourceID || prize.GetItem().GetBound() {
		t.Fatal("竞购未保留原非绑定整单与来源", prize)
	}
	marketActionWS(winner.client, itemMarketClaimForTest(prize), 0)
	proceeds := marketEscrowWS(seller.client, auction.OrderId, persistence.ItemMarketClaimProceeds)
	if proceeds.YuanbaoUnits != 36 || readOrder(auction.OrderId).Phase != persistence.ItemMarketSold {
		t.Fatal("竞购未按 39 扣 3 结算原卖方净收", proceeds)
	}
	marketActionWS(seller.client, itemMarketClaimForTest(proceeds), 0)

	// 无人预购的订单自动转直购，买掉两件后剩余原物品归原卖方。
	left := marketBagItem(t, seller.client, stock.ID)
	direct := list(left, 5, 7, 0)
	cancellable := list(returned.Proto(), 1, 3, 0)
	shorten(direct.OrderId, "auction_ends_at")
	directView := marketOrderWS(firstBuyer.client, direct.OrderId, persistence.ItemMarketDirect)
	opened := readOrder(direct.OrderId)
	if opened.DirectEndsAt.Sub(opened.AuctionEndsAt) != 24*time.Hour {
		t.Fatal("直购截止没有采用原订单冻结的二十四小时期限", opened)
	}
	marketActionWS(winner.client, termsForMarketAction(marketOrderWS(winner.client, direct.OrderId, persistence.ItemMarketDirect), marketDirectBuyAction, 1), p.CodeLineForbidden)
	bought := marketActionWS(firstBuyer.client, termsForMarketAction(directView, marketDirectBuyAction, 2), 0)
	if bought.Quantity != 2 || bought.YuanbaoDeltaUnits != -14 || bought.TaxYuanbaoUnits != 2 || readOrder(direct.OrderId).Item.Quantity != 3 {
		t.Fatal("部分直购未保留余量或延续原税余数", bought)
	}
	if !marketBagContainsSource(firstBuyer.client, stock.SourceID, 2) {
		t.Fatal("部分直购成交时没有原子交付原来源物品")
	}
	shorten(direct.OrderId, "direct_ends_at")
	expireMarketTestPrivilege(t, ctx, store, seller)
	if directCash := marketEscrowWS(seller.client, direct.OrderId, persistence.ItemMarketClaimProceeds); directCash.YuanbaoUnits != 12 {
		t.Fatal("卖家到期后未保留部分直购净收", directCash)
	} else {
		marketActionWS(seller.client, itemMarketClaimForTest(directCash), 0)
	}
	unsold := marketEscrowWS(seller.client, direct.OrderId, persistence.ItemMarketClaimItem)
	if unsold.GetItem().GetSourceId() != stock.SourceID || unsold.GetItem().GetQuantity() != 3 || readOrder(direct.OrderId).Phase != persistence.ItemMarketExpired {
		t.Fatal("直购到期没有退回原剩余道具", unsold)
	}
	marketActionWS(seller.client, itemMarketClaimForTest(unsold), 0)
	marketActionWS(seller.client, termsForMarketAction(marketOrderWS(seller.client, cancellable.OrderId, persistence.ItemMarketAuction), marketCancelAction, 1), 0)
	cancelled := marketEscrowWS(seller.client, cancellable.OrderId, persistence.ItemMarketClaimItem)
	if cancelled.GetItem().GetSourceId() != returned.SourceID || readOrder(cancellable.OrderId).Phase != persistence.ItemMarketCancelled {
		t.Fatal("特权过期后撤销无有效预购订单没有返还原资产", cancelled)
	}
	marketActionWS(seller.client, itemMarketClaimForTest(cancelled), 0)

	beforeReplay := readRole(firstBuyer)
	oldBinding := firstBuyer.client.binding
	firstBuyer.attach()
	if firstBuyer.client.binding == oldBinding {
		t.Fatal("健康重连没有签发新会话围栏")
	}
	if replay := marketActionWS(firstBuyer.client, refundRequest, 0); !proto.Equal(replay, refundReceipt) {
		t.Fatalf("原操作重放没有返回首次退款回执：%v / %v", replay, refundReceipt)
	}
	marketActionWS(firstBuyer.client, itemMarketClaimForTest(refund), p.CodeOperationConflict)
	if actual := readRole(firstBuyer); actual.Wallet != beforeReplay.Wallet {
		t.Fatal("新会话或新操作重复退款", actual.Wallet)
	}
	var durable persistence.Operation
	must(t, store.DB.Collection("operations").FindOne(ctx,
		bson.M{"_id": "asset:" + beforeReplay.ID + ":" + domain.Hex(refundRequest.Operation.OpId)}).Decode(&durable))
	if !proto.Equal(durable.ItemMarket.Proto(), refundReceipt) {
		t.Fatal("Mongo 原永久操作与 WebSocket 的确认退款回执不一致", durable.ItemMarket)
	}
	assertMarketConservation(t, ctx, store, stock.SourceID, []domain.Role{
		readRole(seller), readRole(sellerAlt), readRole(firstBuyer), readRole(buyerAlt), readRole(winner),
	})

	// 两个仍持特权的卖家角色共用一个账号的五个名额，终态历史不占名额。
	secondStock := domain.Item{ID: domain.ID(), ItemID: marketItem, Quantity: 3, Version: 1, SourceKind: "monster_drop", SourceID: domain.ID()}
	otherSeller := seed(sellerLogin, 3, 1_000, secondStock)
	listMore := func(c *client, itemID string, quantity int64, code uint32) *pb.ItemMarketReceipt {
		t.Helper()
		item := marketBagItem(t, c, itemID)
		return marketActionWS(c, &pb.ItemMarketAction{Action: marketListAction, ItemInstanceId: item.InstanceId,
			ItemVersion: item.Version, Quantity: quantity, UnitPriceYuanbaoUnits: 5}, code)
	}
	firstOrders := [][]byte{
		listMore(sellerAlt.client, limitStock.ID, 1, 0).OrderId,
		listMore(sellerAlt.client, limitStock.ID, 2, 0).OrderId,
		listMore(sellerAlt.client, limitStock.ID, 1, 0).OrderId,
	}
	secondOrders := [][]byte{
		listMore(otherSeller.client, secondStock.ID, 1, 0).OrderId,
		listMore(otherSeller.client, secondStock.ID, 1, 0).OrderId,
	}
	listMore(otherSeller.client, secondStock.ID, 1, p.CodeOperationConflict)
	if remaining := marketBagItem(t, otherSeller.client, secondStock.ID); remaining.Quantity != 1 {
		t.Fatal("第六笔订单被拒后仍移走道具", remaining)
	}
	assertMarketPages(firstBuyer.client, 1, marketItem, 2, append(firstOrders, secondOrders...))
	assertMarketPages(sellerAlt.client, 2, marketItem, 2, firstOrders)
	assertMarketPages(otherSeller.client, 2, marketItem, 1, secondOrders)
}

// marketStateWS 只接受当前场景和请求编号关联的市场目录响应。
func marketStateWS(c *client, scope, item uint32) *pb.ItemMarketState {
	return marketPageWS(c, scope, item, 20, nil)
}

// marketPageWS 使用服务端返回的游标读取同一范围的下一页。
func marketPageWS(c *client, scope, item, size uint32, cursor []byte) *pb.ItemMarketState {
	c.t.Helper()
	request := c.send(p.MsgC2SItemMarketQuery, &pb.ItemMarketQuery{SceneEpoch: c.entry.SceneEpoch, Scope: scope, ItemId: item, PageSize: size, Cursor: cursor})
	frame := travelUntilWithin(c, 4*time.Second, func(f wire.Frame) bool { return f.ID == p.MsgS2CItemMarketState && f.Request == request })
	state := &pb.ItemMarketState{}
	must(c.t, proto.Unmarshal(frame.Payload, state))
	if state.ErrorCode != 0 || state.SceneEpoch != c.entry.SceneEpoch || state.Scope != scope {
		c.t.Fatal("市场查询未返回当前场景、范围的有效结果", state)
	}
	return state
}

// assertMarketPages 核对公开或本人订单按原身份稳定分页，最后一页没有续查游标。
func assertMarketPages(c *client, scope, item uint32, size int, ids [][]byte) {
	c.t.Helper()
	want := slices.Clone(ids)
	slices.SortFunc(want, bytes.Compare)
	var cursor []byte
	for from := 0; from < len(want); from += size {
		to := min(from+size, len(want))
		page := marketPageWS(c, scope, item, uint32(size), cursor)
		if len(page.Orders) != to-from {
			c.t.Fatalf("市场范围 %d 第 %d 页订单数=%d，预期 %d", scope, from/size+1, len(page.Orders), to-from)
		}
		for index, order := range page.Orders {
			if !bytes.Equal(order.OrderId, want[from+index]) || order.State != persistence.ItemMarketAuction || scope == 2 && !order.Owned {
				c.t.Fatalf("市场范围 %d 出现重单、漏单或跨角色订单：%v", scope, order)
			}
		}
		if to < len(want) && !bytes.Equal(page.NextCursor, want[to-1]) || to == len(want) && len(page.NextCursor) != 0 {
			c.t.Fatalf("市场范围 %d 的下一页游标不匹配末条订单：%x", scope, page.NextCursor)
		}
		cursor = page.NextCursor
	}
}

// marketOrderWS 等待原公开订单出现于目标阶段；只通过正常查询观察推进。
func marketOrderWS(c *client, id []byte, phase uint32) *pb.ItemMarketOrderView {
	c.t.Helper()
	for end := time.Now().Add(15 * time.Second); time.Now().Before(end); time.Sleep(100 * time.Millisecond) {
		for _, order := range marketStateWS(c, 1, 0).Orders {
			if bytes.Equal(order.OrderId, id) && order.State == phase {
				return order
			}
		}
	}
	c.t.Fatalf("未观察到市场订单 %x 进入阶段 %d", id, phase)
	return nil
}

// marketEscrowWS 从当前角色的独立托管仓等待指定资产。
func marketEscrowWS(c *client, id []byte, kind uint32) *pb.ItemMarketEscrowView {
	c.t.Helper()
	for end := time.Now().Add(15 * time.Second); time.Now().Before(end); time.Sleep(100 * time.Millisecond) {
		for _, asset := range marketStateWS(c, 3, 0).Escrows {
			if bytes.Equal(asset.OrderId, id) && asset.Kind == kind {
				return asset
			}
		}
	}
	c.t.Fatalf("未观察到市场订单 %x 的托管类型 %d", id, kind)
	return nil
}

// marketActionWS 等待实际资产终态，并在角色修订推进时接收对应完整库存。
func marketActionWS(c *client, request *pb.ItemMarketAction, code uint32) *pb.ItemMarketReceipt {
	c.t.Helper()
	if request.Operation == nil {
		request.Operation = c.operation()
	}
	result := c.asset(p.MsgC2SItemMarketAction, request)
	if result.ErrorCode != code || result.Pending || !bytes.Equal(result.OpId, request.Operation.OpId) ||
		(code == 0) != (result.ItemMarket != nil) {
		c.t.Fatalf("市场动作 %d 返回错误结果，预期错误码 %d：%v", request.Action, code, result)
	}
	if result.NewRevision > c.inventory.Revision {
		c.inventoryAt(result.NewRevision)
	}
	return result.ItemMarket
}

func termsForMarketAction(order *pb.ItemMarketOrderView, action uint32, quantity int64) *pb.ItemMarketAction {
	return &pb.ItemMarketAction{Action: action, OrderId: order.OrderId, OrderVersion: order.Version,
		Quantity: quantity, UnitPriceYuanbaoUnits: order.UnitPriceYuanbaoUnits}
}

func itemMarketClaimForTest(asset *pb.ItemMarketEscrowView) *pb.ItemMarketAction {
	return &pb.ItemMarketAction{Action: marketClaimAction, EscrowId: asset.EscrowId, EscrowVersion: asset.Version}
}

func marketBagItem(t *testing.T, c *client, id string) *pb.ItemView {
	t.Helper()
	for _, item := range c.inventory.Items {
		if domain.Hex(item.InstanceId) == id {
			return item
		}
	}
	t.Fatal("原堆叠余量没有通过库存同步", id)
	return nil
}

// marketBagContainsSource 核对直购同步的同来源原物和实际数量。
func marketBagContainsSource(c *client, source string, quantity int64) bool {
	for _, item := range c.inventory.Items {
		if item.SourceId == source && item.Quantity == quantity && !item.Bound {
			return true
		}
	}
	return false
}

// expireMarketTestPrivilege 只推进隔离卡的结束时刻，再通过真实新票加载退出资格的角色。
func expireMarketTestPrivilege(t *testing.T, ctx context.Context, store *persistence.Store, role *marketTestRole) {
	t.Helper()
	result, err := store.DB.Collection("roles").UpdateByID(ctx, domain.Hex(role.key.RoleId),
		bson.M{"$set": bson.M{"privilege_until": time.Now().Add(-time.Second)}})
	must(t, err)
	if result.ModifiedCount != 1 {
		t.Fatal("未定位隔离角色的原卡期限")
	}
	role.attach()
	if state := marketStateWS(role.client, 1, 0); state.CanTrade {
		t.Fatal("新票没有反映原角色的已到期交易资格", state)
	}
}

// assertMarketConservation 核对总资产、两次成交事实、永久入口的累计税额及原物品来源。
func assertMarketConservation(t *testing.T, ctx context.Context, store *persistence.Store, source string, roles []domain.Role) {
	t.Helper()
	balances, stocks := []int64{1_048, 1_000, 986, 5, 961}, []int64{3, 0, 2, 0, 3}
	for index, role := range roles {
		var quantity int64
		for _, item := range role.Items {
			if item.SourceID == source {
				if item.Bound {
					t.Fatal("市场转移不得改变原资产绑定", item)
				}
				quantity += item.Quantity
			}
		}
		wallet := role.Wallet
		if wallet.Yuanbao != balances[index] || wallet.BoundYuanbao != 9_999 || wallet.Gold != 11_111 || wallet.BoundGold != 12_222 || quantity != stocks[index] {
			t.Fatalf("角色市场账本不守恒：下标=%d，钱包=%+v，道具数量=%d", index, wallet, quantity)
		}
	}
	cursor, err := store.DB.Collection("item_market_settlements").Find(ctx, bson.M{})
	must(t, err)
	var facts []persistence.ItemMarketSettlement
	must(t, cursor.All(ctx, &facts))
	if len(facts) != 2 || facts[0].Gross+facts[1].Gross != 53 || facts[0].Tax+facts[1].Tax != 5 ||
		facts[0].Item.Quantity+facts[1].Item.Quantity != 5 {
		t.Fatal("实际成交额、商品或税存在重复或缺漏", facts)
	}
	var tax struct {
		Remainder int64 `bson:"remainder"` // 两笔成交后未满 0.1 元宝的累计税余数。
	}
	must(t, store.DB.Collection("seller_tax_remainders").FindOne(ctx,
		bson.M{"account_id": roles[0].AccountID, "origin_server_id": roles[0].Origin}).Decode(&tax))
	if tax.Remainder != 3 {
		t.Fatalf("按卖方永久入口保留的税余数错误：%d", tax.Remainder)
	}
}
