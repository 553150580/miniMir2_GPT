// 本文件验证矿石自动回收只引用配置明确开放的商店和矿种。
package devconfig

import "testing"

func TestMiningAutoRecycleRequiresPricedOre(t *testing.T) {
	newFixture := func() Fixture {
		f := miningConfigTestFixture()
		f.NPCShops[0].MineralBuyback = []NPCMineralBuyback{
			{ItemID: 920001, Numerator: 1, Denominator: 2, Rounding: "floor_per_ore"},
			{ItemID: 920004, Numerator: 4, Denominator: 1, Rounding: "exact"},
		}
		return f
	}
	f := newFixture()
	if _, ok := f.MiningAutoRecyclePrice(920001); ok {
		t.Fatal("仅开启人工出售时错误开放了自动回收")
	}
	f.Mining.AutoRecycle = &MiningAutoRecycleConfig{ShopKey: "grocery", ItemIDs: []uint32{920001}}
	if err := f.validateMining(); err != nil {
		t.Fatal(err)
	}
	price, ok := f.MiningAutoRecyclePrice(920001)
	if !ok || price != f.NPCShops[0].MineralBuyback[0] {
		t.Fatal("自动回收没有引用原商店的铜矿价格")
	}
	if _, ok := f.MiningAutoRecyclePrice(920004); ok {
		t.Fatal("人工收购的黑铁未明确开放自动回收")
	}
	for _, row := range []struct {
		name string
		edit func(*Fixture)
	}{
		{"未知商店", func(x *Fixture) { x.Mining.AutoRecycle.ShopKey = "unknown" }},
		{"空矿种范围", func(x *Fixture) { x.Mining.AutoRecycle.ItemIDs = nil }},
		{"重复矿种", func(x *Fixture) { x.Mining.AutoRecycle.ItemIDs = []uint32{920001, 920001} }},
		{"锄头混入矿种", func(x *Fixture) { x.Mining.AutoRecycle.ItemIDs = []uint32{920011} }},
		{"矿种无对应价格", func(x *Fixture) { x.Mining.AutoRecycle.ItemIDs = []uint32{920002} }},
		{"所有纯度均为零价", func(x *Fixture) { x.NPCShops[0].MineralBuyback[0].Denominator = 101 }},
	} {
		t.Run(row.name, func(t *testing.T) {
			bad := newFixture()
			bad.Mining.AutoRecycle = &MiningAutoRecycleConfig{ShopKey: "grocery", ItemIDs: []uint32{920001}}
			row.edit(&bad)
			if err := bad.validateMining(); err == nil {
				t.Fatal("错误的矿石自动回收配置通过启动检查")
			}
		})
	}
}
