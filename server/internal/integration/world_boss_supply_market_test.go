// 本文件从普通参与者实收的世界首领材料追踪市场成交、佣金、买家培养和期末库存。
package integration

import (
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
)

// worldBossSupplySale 关联卖家原材料实例、实际成交以及可重查的原买入和收款操作。
type worldBossSupplySale struct {
	SourceItemID string                           `json:"source_item_instance_id"` // 普通参与者原奖励堆的实例。
	BuyOpID      string                           `json:"buy_operation_id"`        // 真实买入操作编号。
	PayoutOpID   string                           `json:"payout_operation_id"`     // 卖家净收原领取操作。
	Fact         persistence.ItemMarketSettlement `json:"settlement"`              // 原成交、物品、买卖双方、费用和时间。
}

// worldBossSupplyStock 使用独立物品号和绑定类别核算该来源的实收、消耗及结余。
type worldBossSupplyStock struct {
	ItemID      uint32 `json:"item_id"`       // 配置物品号。
	IssuedBound int64  `json:"issued_bound"`  // 从原附件真实领取的绑定数量。
	IssuedFree  int64  `json:"issued_free"`   // 从原附件真实领取的非绑定数量。
	SpentFree   int64  `json:"spent_free"`    // 实际由买家用于永久培养的非绑定材料。
	InBagsBound int64  `json:"in_bags_bound"` // 四人角色库存中的实有绑定材料。
	InBagsFree  int64  `json:"in_bags_free"`  // 四人角色库存中的实有非绑定材料。
}

// worldBossSupplyMarket 保存原交易链和双方各四个钱包的实际快照。
type worldBossSupplyMarket struct {
	Sales        []worldBossSupplySale  `json:"sales"`         // 每次唯一成交。
	Stocks       []worldBossSupplyStock `json:"stocks"`        // 材料最终守恒。
	SellerBefore domain.Wallet          `json:"seller_before"` // 首次成交前卖方钱包。
	SellerAfter  domain.Wallet          `json:"seller_after"`  // 全部收款后的卖方钱包。
	BuyerBefore  domain.Wallet          `json:"buyer_before"`  // 首次成交前买方钱包。
	BuyerAfter   domain.Wallet          `json:"buyer_after"`   // 购入、培养后的买方钱包。
}

// buyWorldBossSupplyPrivilege 使用实际月卡商品给原无资格交易者开卡，核对支付发生在原击杀之后。
func buyWorldBossSupplyPrivilege(t *testing.T, c *client, killedAt time.Time) *pb.RoleCardPurchaseReceipt {
	t.Helper()
	catalog := roleCardStateWS(c, 0)
	index := slices.IndexFunc(catalog.Products, func(product *pb.RoleCardProductView) bool {
		return product.Kind == devconfig.RoleCardKindPrivilege
	})
	if index < 0 || catalog.PrivilegeUntilUnixMs > killedAt.UnixMilli() {
		t.Fatal("开卡前应有已过期的特权与可购目录", catalog)
	}
	product := catalog.Products[index]
	quote := roleCardStateWS(c, product.ProductId).GetQuote()
	if len(quote.GetQuoteId()) != 16 {
		t.Fatal("无法取得普通非绑元宝月卡报价", quote)
	}
	request := &pb.BuyRoleCardRequest{ProductId: product.ProductId, QuoteId: quote.QuoteId, Operation: c.operation()}
	result := roleCardAssetWS(c, p.MsgC2SBuyRoleCard, request)
	receipt := result.GetRoleCardPurchase()
	if result.ErrorCode != 0 || receipt.GetEffectiveAtUnixMs() <= killedAt.UnixMilli() || !proto.Equal(receipt.GetQuote(), quote) ||
		!marketStateWS(c, 1, 0).GetCanTrade() {
		t.Fatal("真实开卡未生效，或世界首领击杀与购卡顺序错误", result)
	}
	return receipt
}

// tradeWorldBossSupply 让第二名在正常市场卖出两种实际材料，分别验证资金和物品两套账。
func tradeWorldBossSupply(t *testing.T, trial worldBossRewardTrial, players []worldBossRewardPlayer, claims []worldBossSupplyClaim, occurrence string) worldBossSupplyMarket {
	t.Helper()
	fixture := &trial.store.Cfg.Fixture
	seller, restricted, buyer := players[1].client, players[2].client, players[3].client
	sellerID, buyerID := claims[1].RoleID, worldBossSupplyRoleID(players[3])
	readRole := func(roleID string) domain.Role {
		t.Helper()
		role, err := trial.store.Role(trial.ctx, roleID)
		must(t, err)
		return role
	}
	ledger := worldBossSupplyMarket{SellerBefore: readRole(sellerID).Wallet, BuyerBefore: readRole(buyerID).Wallet}
	var gross, fees int64
	for _, line := range []struct {
		itemID    uint32 // 原奖励中的候选物品号。
		quantity  int64  // 本次成交的确定数量，留有卖方余量。
		unitPrice int64  // 隔离测试挂单价，使用 0.1 元宝单位。
	}{{fixture.Wings.FeatherItemID, 2, 17}, {fixture.Medal.FragmentItemID, 3, 11}} {
		from := slices.IndexFunc(claims[1].Reward.Items, func(item domain.Item) bool { return item.ItemID == line.itemID })
		blocked := slices.IndexFunc(claims[2].Reward.Items, func(item domain.Item) bool { return item.ItemID == line.itemID })
		if from < 0 || blocked < 0 {
			t.Fatal("原普通参与者缺少本次交易对应的实收材料", line.itemID)
		}
		bound := marketBagItem(t, restricted, claims[2].Reward.Items[blocked].ID)
		marketActionWS(restricted, &pb.ItemMarketAction{Action: marketListAction, ItemInstanceId: bound.InstanceId,
			ItemVersion: bound.Version, Quantity: 1, UnitPriceYuanbaoUnits: line.unitPrice}, p.CodeVendorItemBlocked)
		original := claims[1].Reward.Items[from]
		listed := marketActionWS(seller, &pb.ItemMarketAction{Action: marketListAction, ItemInstanceId: domain.Bytes(original.ID),
			ItemVersion: uint64(original.Version), Quantity: line.quantity, UnitPriceYuanbaoUnits: line.unitPrice}, 0)
		remaining := marketBagItem(t, seller, original.ID)
		if remaining.Quantity != original.Quantity-line.quantity || remaining.Bound || remaining.SourceId != occurrence {
			t.Fatal("上架未从原领取堆精确扣除拍品", listed, remaining)
		}
		sale := buyWorldBossSupplyOrder(t, trial, seller, buyer, listed.OrderId, original, line.quantity, line.unitPrice, gross)
		if sale.Fact.Seller.RoleID != sellerID || sale.Fact.Buyer.RoleID != buyerID {
			t.Fatal("成交原永久归属错误", sale.Fact)
		}
		ledger.Sales = append(ledger.Sales, sale)
		gross, fees = gross+sale.Fact.Gross, fees+sale.Fact.Tax
	}
	feather, fragment := marketBagItem(t, buyer, ledger.Sales[0].Fact.Item.ID), marketBagItem(t, buyer, ledger.Sales[1].Fact.Item.ID)
	wingCost, medalLevel := fixture.Wings.Ranks[0].Feathers, fixture.Medal.Levels[0]
	growWingWS(t, buyer, nil, feather, domain.Bytes(domain.ID()), wingCost, 1, 1, false)
	medal := requestMedalWS(t, buyer, domain.Bytes(domain.ID()), fragment, medalLevel, false)
	expectMedalReceipt(t, buyer, medal, buyer.asset(p.MsgC2SMedalCommit, medalCommitRequest(buyer, medal)))
	checkMedalFragmentLeft(t, buyer, fragment, medalLevel.FragmentCost)
	ledger.SellerAfter, ledger.BuyerAfter = readRole(sellerID).Wallet, readRole(buyerID).Wallet
	expectedSeller, expectedBuyer := ledger.SellerBefore, ledger.BuyerBefore
	expectedSeller.Yuanbao += gross - fees
	expectedBuyer.Yuanbao -= gross
	if ledger.SellerAfter != expectedSeller || ledger.BuyerAfter != expectedBuyer {
		t.Fatal("两笔成交及培养后的原钱包不守恒", ledger, gross, fees)
	}
	var roles []domain.Role
	for _, player := range players {
		roles = append(roles, readRole(worldBossSupplyRoleID(player)))
	}
	ledger.Stocks = []worldBossSupplyStock{
		reconcileWorldBossSupplyStock(t, claims, roles, occurrence, feather.ItemId, wingCost),
		reconcileWorldBossSupplyStock(t, claims, roles, occurrence, fragment.ItemId, medalLevel.FragmentCost),
	}
	return ledger
}

// buyWorldBossSupplyOrder 等真实市场进入直购并核对原成交、税、交付、到账和重放。
func buyWorldBossSupplyOrder(t *testing.T, trial worldBossRewardTrial, seller, buyer *client, orderID []byte, original domain.Item, quantity, price, priorGross int64) worldBossSupplySale {
	t.Helper()
	collection := trial.store.DB.Collection("item_orders")
	key := bson.M{"_id": domain.Hex(orderID)}
	var order persistence.ItemMarketOrder
	must(t, collection.FindOne(trial.ctx, key).Decode(&order))
	rules := trial.store.Cfg.Fixture.ItemMarket
	if order.Phase != persistence.ItemMarketAuction || order.Item.Quantity != quantity || order.Item.Bound || order.Item.SourceID != original.SourceID ||
		order.UnitPrice != price || order.AuctionEndsAt.Sub(order.ListedAt) != time.Duration(rules.AuctionSeconds)*time.Second || order.TaxDivisor != rules.TaxDivisor {
		t.Fatal("原订单未保存实际来源、价格、佣金与配置期限", order)
	}
	// 仅在隔离库缩短确认过的竞购截止。由正常市场排程转直购，成交和收款均提交真实请求。
	key["revision"] = order.Revision
	changed, err := collection.UpdateOne(trial.ctx, key, bson.M{"$set": bson.M{"auction_ends_at": time.Now().Add(time.Second).UTC().Truncate(time.Millisecond)}})
	must(t, err)
	if changed.ModifiedCount != 1 {
		t.Fatal("原竞购订单在隔离截止调整前已变化", order.ID)
	}
	view := marketOrderWS(buyer, orderID, persistence.ItemMarketDirect)
	buy := termsForMarketAction(view, marketDirectBuyAction, quantity)
	bought := marketActionWS(buyer, buy, 0)
	gross := quantity * price
	tax := (priorGross+gross)/order.TaxDivisor - priorGross/order.TaxDivisor
	if bought.Quantity != quantity || bought.YuanbaoDeltaUnits != -gross || bought.TaxYuanbaoUnits != tax ||
		!proto.Equal(bought, marketActionWS(buyer, buy, 0)) {
		t.Fatal("买入回执、原佣金或重放不一致", bought)
	}
	payout := itemMarketClaimForTest(marketEscrowWS(seller, orderID, persistence.ItemMarketClaimProceeds))
	paid := marketActionWS(seller, payout, 0)
	if paid.YuanbaoDeltaUnits != gross-tax || !proto.Equal(paid, marketActionWS(seller, payout, 0)) {
		t.Fatal("卖家净收或领取重放不一致", paid)
	}
	var fact persistence.ItemMarketSettlement
	facts := trial.store.DB.Collection("item_market_settlements")
	filter := bson.M{"order_id": order.ID}
	must(t, facts.FindOne(trial.ctx, filter).Decode(&fact))
	count, err := facts.CountDocuments(trial.ctx, filter)
	must(t, err)
	delivered := marketBagItem(t, buyer, fact.Item.ID)
	if count != 1 || fact.Item.ItemID != original.ItemID || fact.Item.Quantity != quantity || fact.Item.Bound ||
		fact.Item.SourceKind != original.SourceKind || fact.Item.SourceID != original.SourceID || fact.Item.Quality != original.Quality ||
		fact.UnitPrice != price || fact.Gross != gross || fact.Tax != tax || !proto.Equal(delivered, fact.Item.Proto()) {
		t.Fatal("真实物品和成交资金缺少唯一的永久证据", fact, delivered, count)
	}
	return worldBossSupplySale{SourceItemID: original.ID, BuyOpID: domain.Hex(buy.Operation.OpId), PayoutOpID: domain.Hex(payout.Operation.OpId), Fact: fact}
}

// reconcileWorldBossSupplyStock 按原场次对账，成交归属变化不会增加总量。
func reconcileWorldBossSupplyStock(t *testing.T, claims []worldBossSupplyClaim, roles []domain.Role, occurrence string, itemID uint32, consumed int64) worldBossSupplyStock {
	t.Helper()
	stock := worldBossSupplyStock{ItemID: itemID, SpentFree: consumed}
	for _, claim := range claims {
		for _, item := range claim.Reward.Items {
			if item.ItemID != itemID {
				continue
			}
			if item.Bound {
				stock.IssuedBound += item.Quantity
			} else {
				stock.IssuedFree += item.Quantity
			}
		}
	}
	for _, role := range roles {
		for _, item := range role.Items {
			if item.ItemID != itemID || item.SourceKind != "world_boss" || item.SourceID != occurrence {
				continue
			}
			if item.Bound {
				stock.InBagsBound += item.Quantity
			} else {
				stock.InBagsFree += item.Quantity
			}
		}
	}
	if stock.IssuedBound != stock.InBagsBound || stock.IssuedFree != stock.InBagsFree+stock.SpentFree {
		t.Fatal("原实收量、买家消耗和累计库存不守恒", stock)
	}
	return stock
}
