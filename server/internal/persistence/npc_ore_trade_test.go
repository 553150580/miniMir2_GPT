// 本文件验证矿石回收的单块取整、绑定钱包及原实例保护。
package persistence

import (
	"math"
	"testing"
	"time"

	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	p "minimir/internal/protocol"
)

// TestVendorMineralSale 验证各类矿石按各自纯度和原绑定状态出价，当前权益不放大报价。
func TestVendorMineralSale(t *testing.T) {
	fixture := &devconfig.Fixture{Items: []devconfig.Item{{ID: 1}, {ID: 2}, {ID: 3}, {ID: 4}}}
	shop := &devconfig.NPCShop{MineralBuyback: []devconfig.NPCMineralBuyback{
		{ItemID: 1, Numerator: 1, Denominator: 2, Rounding: "floor_per_ore"},
		{ItemID: 2, Numerator: 1, Denominator: 1, Rounding: "exact"},
		{ItemID: 3, Numerator: 2, Denominator: 1, Rounding: "exact"},
		{ItemID: 4, Numerator: 4, Denominator: 1, Rounding: "exact"},
	}}
	cases := []struct {
		name   string // 纯度、矿种或绑定的边界名称。
		ore    uint32 // 待售矿种的物品模板号。
		purity uint32 // 原矿块的纯度。
		bound  bool   // 售前原矿是否绑定。
		gold   int64  // 单块最终取得的整数金币。
	}{
		{"奇数铜", 1, 5, true, 2}, {"另一块奇数铜", 1, 7, true, 3}, {"偶数铜", 1, 12, false, 6},
		{"铁矿", 2, 79, true, 79}, {"金矿", 3, 23, false, 46}, {"明确开放的黑铁", 4, 100, true, 400},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ore := domain.Item{ID: domain.ID(), ItemID: tc.ore, Quantity: 1, Version: 7,
				SourceKind: "mining", SourceID: domain.ID(), OrePurity: tc.purity, Bound: tc.bound}
			role := domain.Role{Items: []domain.Item{ore}, Wallet: domain.Wallet{Gold: 3, BoundGold: 4},
				PrivilegeUntil: time.Now().Add(time.Hour), HostingCardUntil: time.Now().Add(time.Hour)}
			request := &pb.VendorQuoteRequest{Action: vendorSell, InstanceId: domain.Bytes(ore.ID), ItemVersion: 7, Quantity: 1}
			want := domain.Wallet{Gold: tc.gold}
			if tc.bound {
				want = domain.Wallet{BoundGold: tc.gold}
			}
			plan, code := buildVendorPlan(role, fixture, shop, request)
			if code != 0 || plan.Wallet != want || plan.Bound != tc.bound || plan.Sold == nil || *plan.Sold != ore {
				t.Fatalf("矿价、绑定钱包或原实例改变：%+v，%d", plan, code)
			}
			out, receipt, code := applyVendorPlan(role, plan, "ore-sale")
			if code != 0 || len(out.Items) != 0 || role.Items[0] != ore || *receipt.Sold != ore ||
				out.Wallet != (domain.Wallet{Gold: role.Wallet.Gold + want.Gold, BoundGold: role.Wallet.BoundGold + want.BoundGold}) {
				t.Fatalf("成交没有将原单矿移除并按对应钱包结算：%+v，%d", out, code)
			}
		})
	}
}

// TestVendorMineralSaleRejectsInvalidInstance 核验矿石必须来自原背包且具备合法的单块身份与可执行报价。
func TestVendorMineralSaleRejectsInvalidInstance(t *testing.T) {
	fixture := &devconfig.Fixture{Items: []devconfig.Item{{ID: 1}, {ID: 2}, {ID: 3, Stackable: true}}}
	shop := &devconfig.NPCShop{
		MineralBuyback: []devconfig.NPCMineralBuyback{
			{ItemID: 1, Numerator: 1, Denominator: 2, Rounding: "floor_per_ore"},
			{ItemID: 3, Numerator: 1, Denominator: 1, Rounding: "exact"},
		},
		// 伪造固定收购价，确认无专项牌价时仍拒绝矿石。
		SellPrices: []devconfig.NPCSellPrice{{ItemID: 2, PriceGold: 200}},
	}
	base := domain.Item{ID: domain.ID(), ItemID: 1, Quantity: 1, Version: 8, SourceKind: "mining", SourceID: domain.ID(), OrePurity: 5}
	cases := []struct {
		name   string             // 本例破坏的出售条件。
		change func(*domain.Role) // 在构造请求前构造独立异常角色快照。
	}{
		{"零价保留", func(r *domain.Role) { r.Items[0].OrePurity = 1 }},
		{"无纯度", func(r *domain.Role) { r.Items[0].OrePurity = 0 }},
		{"超出纯度", func(r *domain.Role) { r.Items[0].OrePurity = 101 }},
		{"非挖矿来源", func(r *domain.Role) { r.Items[0].SourceKind = "npc_shop" }},
		{"无原来源号", func(r *domain.Role) { r.Items[0].SourceID = "" }},
		{"伪造来源号", func(r *domain.Role) { r.Items[0].SourceID = "123456789012345678901234567890gh" }},
		{"矿种未收购", func(r *domain.Role) { r.Items[0].ItemID = 2 }},
		{"模板可堆叠", func(r *domain.Role) { r.Items[0].ItemID = 3 }},
		{"矿块叠堆", func(r *domain.Role) { r.Items[0].Quantity = 2 }},
		{"异常耐久", func(r *domain.Role) { r.Items[0].DurabilityU = 1 }},
		{"过期版本", func(r *domain.Role) { r.Items[0].Version++ }},
		{"锁定", func(r *domain.Role) { r.Items[0].Locked = true }},
		{"待确认占用", func(r *domain.Role) { r.Items[0].PendingResultID = "pending" }},
		{"其他资产业务预留", func(r *domain.Role) { r.Items[0].ReservationID = "reserved" }},
		{"钱包溢出", func(r *domain.Role) { r.Wallet.Gold = math.MaxInt64 }},
		{"非当前背包", func(r *domain.Role) { r.Items = nil }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			role := domain.Role{Items: []domain.Item{base}}
			tc.change(&role)
			request := &pb.VendorQuoteRequest{Action: vendorSell, InstanceId: domain.Bytes(base.ID), ItemVersion: 8, Quantity: 1}
			if plan, code := buildVendorPlan(role, fixture, shop, request); code != p.CodeVendorItemBlocked || plan.Sold != nil {
				t.Fatalf("非法矿石被允许出售：%+v，%d", plan, code)
			}
		})
	}
}
