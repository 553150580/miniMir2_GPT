package persistence

import (
	"math"
	"testing"

	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	p "minimir/internal/protocol"
)

func TestVendorPurchaseSplitsPaymentAndPreservesBoundAssets(t *testing.T) {
	f := &devconfig.Fixture{
		EquipmentDurability: &devconfig.EquipmentDurabilityRules{},
		Items: []devconfig.Item{
			{ID: 35, Stackable: true, MaxStack: 8},
			{ID: 218, IsEquipment: true, OriginalDurabilityU: 4000},
		},
	}
	shop := &devconfig.NPCShop{Offers: []devconfig.SkillShop{
		{ID: 10, ItemID: 35, Quantity: 3, PriceUnits: 7, Currency: "gold", RequiredRoleLevel: 1, MaxPurchaseQuantity: 4},
		{ID: 20, ItemID: 218, Quantity: 1, PriceUnits: 15, Currency: "gold", RequiredRoleLevel: 1, MaxPurchaseQuantity: 1},
	}}
	r := domain.Role{Level: 1, Capacity: 3, Wallet: domain.Wallet{Gold: 50, BoundGold: 15}}
	input := &pb.VendorQuoteRequest{Action: vendorBuy, OfferId: 10, Quantity: 3}
	plan, code := buildVendorPlan(r, f, shop, input)
	if code != 0 || plan.Wallet.Gold != 6 || plan.Wallet.BoundGold != 15 || !plan.Bound ||
		plan.ItemQuantity != 9 || len(plan.Outputs) != 2 || plan.Outputs[0].Quantity != 8 || plan.Outputs[1].Quantity != 1 {
		t.Fatalf("购买数量、分堆或绑定支付预览错误：%+v，%d", plan, code)
	}
	out, receipt, code := applyVendorPlan(r, plan, "operation")
	if code != 0 || len(r.Items) != 0 || len(out.Items) != 2 || out.Wallet.BoundGold != 0 || out.Wallet.Gold != 44 ||
		out.Items[0].ID == out.Items[1].ID || len(out.Items[0].ID) != 32 || !out.Items[0].Bound || receipt.Outputs[0].SourceID != "operation" {
		t.Fatalf("购买提交未归属新实例或改变原快照：%+v，%d", out, code)
	}
	weapon, code := buildVendorPlan(out, f, shop, &pb.VendorQuoteRequest{Action: vendorBuy, OfferId: 20, Quantity: 1})
	if code != 0 || len(weapon.Outputs) != 1 || weapon.Outputs[0].DurabilityU != 4000 || weapon.Bound {
		t.Fatalf("新装备耐久或非绑定支付错误：%+v，%d", weapon, code)
	}
	if _, code = buildVendorPlan(out, f, shop, input); code != p.CodeEquipmentBagFull {
		t.Fatalf("满包购买应失败：%d", code)
	}
	out.Wallet.Gold = 0
	if _, code = buildVendorPlan(out, f, shop, input); code != p.CodeVendorFunds {
		t.Fatalf("余额不足应失败：%d", code)
	}
}

func TestVendorSaleUsesOriginalInstanceAndNeverLaundersBoundGold(t *testing.T) {
	weapon := domain.Item{ID: domain.ID(), ItemID: 218, Quantity: 1, Version: 5, Bound: true,
		OriginalDurabilityU: 4000, MaxDurabilityU: 3800, DurabilityU: 2000}
	input := &pb.VendorQuoteRequest{Action: vendorSell, InstanceId: domain.Bytes(weapon.ID), ItemVersion: 5, Quantity: 1}
	shop := &devconfig.NPCShop{SellPrices: []devconfig.NPCSellPrice{{ItemID: 218, PriceGold: 25}}}
	f := &devconfig.Fixture{}
	r := domain.Role{Items: []domain.Item{weapon}}
	plan, code := buildVendorPlan(r, f, shop, input)
	if code != 0 || plan.Wallet.Gold != 0 || plan.Wallet.BoundGold != 12 {
		t.Fatalf("磨损折价或绑定入账错误：%+v，%d", plan, code)
	}
	out, _, code := applyVendorPlan(r, plan, "operation")
	if code != 0 || len(out.Items) != 0 || out.Wallet.BoundGold != 12 || len(r.Items) != 1 {
		t.Fatalf("出售提交错误：%+v，%d", out, code)
	}
	for _, mutate := range []func(*domain.Role){
		func(r *domain.Role) { r.Items[0].Version++ },
		func(r *domain.Role) { r.Items[0].DurabilityU = 0 },
		func(r *domain.Role) { r.Items[0].Locked = true },
		func(r *domain.Role) { r.Items[0].ReservationID = "reserved" },
		func(r *domain.Role) { r.Wallet.BoundGold = math.MaxInt64 },
		func(r *domain.Role) { r.Equipped, r.Items = []domain.Equipped{{Slot: 0, Item: r.Items[0]}}, nil },
	} {
		r := domain.Role{Items: []domain.Item{weapon}}
		mutate(&r)
		if _, code := buildVendorPlan(r, f, shop, input); code != p.CodeVendorItemBlocked {
			t.Fatalf("过期、锁定、损坏或余额溢出没有拒绝：%+v，%d", r, code)
		}
	}
}
