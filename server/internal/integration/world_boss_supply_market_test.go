// 本文件从普通参与者实收的世界首领材料追踪市场成交、佣金、买家培养和期末库存。
package integration

import (
	"slices"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	p "minimir/internal/protocol"
)

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
	Sales        []marketDirectSaleProof `json:"sales"`         // 每次唯一成交。
	Stocks       []worldBossSupplyStock  `json:"stocks"`        // 材料最终守恒。
	SellerBefore domain.Wallet           `json:"seller_before"` // 首次成交前卖方钱包。
	SellerAfter  domain.Wallet           `json:"seller_after"`  // 全部收款后的卖方钱包。
	BuyerBefore  domain.Wallet           `json:"buyer_before"`  // 首次成交前买方钱包。
	BuyerAfter   domain.Wallet           `json:"buyer_after"`   // 购入、培养后的买方钱包。
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
		sale := settleMarketDirectForTest(t, trial.ctx, trial.store, seller, buyer, listed.OrderId, original, line.quantity, line.unitPrice, gross)
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
