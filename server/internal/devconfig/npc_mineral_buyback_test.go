// 本文件检查收购配置在缺省、显式开放和错误矿种映射下的启动边界。
package devconfig

import "testing"

func TestMineralBuybackRequiresExplicitValidOrePrice(t *testing.T) {
	fixture := miningConfigTestFixture()
	shop := &fixture.NPCShops[0]
	if _, ok := shop.MineralBuybackFor(920001); ok {
		t.Fatal("缺省商店开启了矿石收购")
	}
	// 显式配置可收不同矿种，未配置的矿种继续保留在原库存中。
	shop.MineralBuyback = []NPCMineralBuyback{
		{ItemID: 920001, Numerator: 1, Denominator: 2, Rounding: "floor_per_ore"},
		{ItemID: 920002, Numerator: 1, Denominator: 1, Rounding: "exact"},
	}
	if err := fixture.validateMineralBuyback(*shop); err != nil {
		t.Fatal(err)
	}
	if price, ok := shop.MineralBuybackFor(920001); !ok || price != shop.MineralBuyback[0] {
		t.Fatal("没有读回本商店配置的比价")
	}
	if _, ok := shop.MineralBuybackFor(920004); ok {
		t.Fatal("未配置的黑铁矿进入了收购目录")
	}
	for _, row := range []struct {
		name string
		edit func(*Fixture, *NPCShop)
	}{
		{"未装配矿种", func(f *Fixture, _ *NPCShop) { f.Mining = nil }},
		{"锄头被当成矿石", func(_ *Fixture, s *NPCShop) { s.MineralBuyback[0].ItemID = 920011 }},
		{"矿种重复", func(_ *Fixture, s *NPCShop) { s.MineralBuyback[0].ItemID = 920002 }},
		{"通用价绕开纯度", func(_ *Fixture, s *NPCShop) { s.SellPrices = []NPCSellPrice{{ItemID: 920001, PriceGold: 3}} }},
		{"比价分子为零", func(_ *Fixture, s *NPCShop) { s.MineralBuyback[0].Numerator = 0 }},
		{"比价分母为零", func(_ *Fixture, s *NPCShop) { s.MineralBuyback[0].Denominator = 0 }},
		{"整数价含分母", func(_ *Fixture, s *NPCShop) { s.MineralBuyback[1].Denominator = 2 }},
		{"未定义取整规则", func(_ *Fixture, s *NPCShop) { s.MineralBuyback[0].Rounding = "ceiling" }},
	} {
		t.Run(row.name, func(t *testing.T) {
			copyFixture := fixture
			copyShop := *shop
			copyShop.MineralBuyback = append([]NPCMineralBuyback(nil), shop.MineralBuyback...)
			row.edit(&copyFixture, &copyShop)
			if err := copyFixture.validateMineralBuyback(copyShop); err == nil {
				t.Fatal("错误的矿石收购配置通过检查")
			}
		})
	}
}
