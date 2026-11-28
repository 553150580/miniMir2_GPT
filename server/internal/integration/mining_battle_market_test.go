// 本文件从完成矿战样本的普通角色取一块实采非绑矿，核对同服直购、原纯度和非绑定元宝守恒。
package integration

import (
	"slices"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
	"google.golang.org/protobuf/proto"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/domain"
)

// miningBattleMarket 保存已证实的成交与两角色各四钱包的增减。
type miningBattleMarket struct {
	Sale         marketDirectSaleProof `json:"sale"`          // 矿石原件、交付及回执。
	SellerBefore domain.Wallet         `json:"seller_before"` // 原矿战角色市场前四钱包。
	SellerAfter  domain.Wallet         `json:"seller_after"`  // 原卖家实际领取后的四钱包。
	BuyerBefore  domain.Wallet         `json:"buyer_before"`  // 隔离买家实际付款前四钱包。
	BuyerAfter   domain.Wallet         `json:"buyer_after"`   // 买家确认原矿到包后的四钱包。
}

// tradeOre 优先使用采样账本中原有的实矿和同一赛道角色，不预置货物。
func (trial *miningBattleTrial) tradeOre(t *testing.T, players []miningBattlePlayer, reports []miningBattleReport) miningBattleMarket {
	t.Helper()
	index := slices.IndexFunc(reports, func(report miningBattleReport) bool {
		return report.Scenario.Privilege && len(report.Mining.Retained) != 0
	})
	if index < 0 {
		t.Fatal("四个持特权样本未实采可供成交的矿石")
	}
	item := reports[index].Mining.Retained[0]
	seller := players[index]
	sellerConn := seller.attach(t)
	if item.Bound || !proto.Equal(item.Proto(), marketBagItem(t, sellerConn, item.ID)) {
		t.Fatal("市场样本缺少实采原件或原绑定已变化", item)
	}
	const buyerFunds = int64(300) // 只给隔离买家准备本次交易本金，卖家资金全部从真实动作产生。
	buyer := trial.newPlayer(t, 1, miningBattleScenario{Privilege: true}, buyerFunds)
	proof := miningBattleMarket{SellerBefore: seller.role(t).Wallet, BuyerBefore: buyer.role(t).Wallet}
	if proof.SellerBefore != reports[index].Closing || proof.BuyerBefore.Yuanbao != buyerFunds {
		t.Fatal("成交前的实际四钱包与矿战期末或买家初态不符", proof.SellerBefore, proof.BuyerBefore)
	}
	const price = int64(29) // 隔离实付价格以 0.1 元宝为整数单位。
	listed := marketActionWS(sellerConn, &pb.ItemMarketAction{Action: marketListAction, ItemInstanceId: domain.Bytes(item.ID),
		ItemVersion: uint64(item.Version), Quantity: 1, UnitPriceYuanbaoUnits: price}, 0)
	if owner := seller.role(t); owner.HasItemInstance(item.ID) || owner.Wallet != proof.SellerBefore {
		t.Fatal("原矿石上架时未准确从背包转到原托管，或提前到账", listed)
	}
	proof.Sale = settleMarketDirectForTest(t, trial.ctx, trial.store, sellerConn, buyer.c, listed.OrderId, item, 1, price, 0)
	fact := proof.Sale.Fact
	if fact.Seller.RoleID != reports[index].RoleID || fact.Buyer.RoleID != domain.Hex(buyer.key.RoleId) || fact.Item.SourceKind != "mining" || fact.Item.OrePurity != item.OrePurity {
		t.Fatal("真实成交未保留矿石的原来源、纯度和正确买卖角色", fact)
	}
	sellerFinal, buyerFinal := seller.role(t), buyer.role(t)
	proof.SellerAfter, proof.BuyerAfter = sellerFinal.Wallet, buyerFinal.Wallet
	if proof.SellerAfter != addMiningBattleWallet(proof.SellerBefore, domain.Wallet{Yuanbao: fact.Gross - fact.Tax}) ||
		proof.BuyerAfter != minusMiningBattleWallet(proof.BuyerBefore, domain.Wallet{Yuanbao: fact.Gross}) {
		t.Fatal("实采矿物成交在原买卖双方与市场佣金间未守恒", proof)
	}
	// 核对最终持有人，每个原矿物事件只有一块未绑定矿在所有采样背包中。
	count, err := trial.store.DB.Collection("roles").CountDocuments(trial.ctx, bson.M{"items": bson.M{"$elemMatch": bson.M{
		"source_kind": "mining", "source_id": item.SourceID, "item_id": item.ItemID, "ore_purity": item.OrePurity, "bound": false,
	}}})
	must(t, err)
	instances := int64(0)
	for _, stock := range buyerFinal.Items {
		if stock.SourceKind == "mining" && stock.SourceID == item.SourceID {
			instances += stock.Quantity
		}
	}
	if count != 1 || instances != 1 || sellerFinal.HasItemInstance(item.ID) {
		t.Fatal("同一原次出矿在交易后缺少唯一持有人和单块原件", item.ID, count, instances)
	}
	return proof
}
