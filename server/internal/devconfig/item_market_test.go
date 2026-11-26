// 本文件核对工作簿到服务端的道具市场定值及旧夹具装配边界。
package devconfig

import "testing"

// TestItemMarketDevelopmentProjection 通过实际开发启动配置加载确认值。
func TestItemMarketDevelopmentProjection(t *testing.T) {
	cfg, err := Load("../../../conf/dev/server.json")
	if err != nil {
		t.Fatal(err)
	}
	want := ItemMarketConfig{MaxListingsPerAccountRun: 5, AuctionSeconds: 7200, BuyoutSeconds: 86400,
		BidRaiseBP: 1000, PriceStepYuanbaoUnits: 1, MinimumRaiseYuanbaoUnits: 1,
		ExtendWhenRemainingBelowSeconds: 1200, ExtendToRemainingSeconds: 1200, SellerCommissionBP: 1000, TaxDivisor: 10}
	if got := cfg.Fixture.ItemMarket; got == nil || *got != want {
		t.Fatalf("当前道具市场参数未按唯一工作簿装配：%+v", got)
	}
}

// TestValidateItemMarket 拒绝非法期限、报价与双市场佣金组合。
func TestValidateItemMarket(t *testing.T) {
	if err := (&Fixture{}).validateItemMarket(); err != nil {
		t.Fatalf("旧夹具缺省市场应保持可加载：%v", err)
	}
	for name, change := range map[string]func(*ItemMarketConfig){
		"缺少上架额度": func(m *ItemMarketConfig) { m.MaxListingsPerAccountRun = 0 },
		"缺少竞购期":  func(m *ItemMarketConfig) { m.AuctionSeconds = 0 },
		"缺少直购期":  func(m *ItemMarketConfig) { m.BuyoutSeconds = 0 },
		"缩短剩余时间": func(m *ItemMarketConfig) { m.ExtendToRemainingSeconds-- },
		"加价比例无效": func(m *ItemMarketConfig) { m.BidRaiseBP = 10001 },
		"无定点步长":  func(m *ItemMarketConfig) { m.PriceStepYuanbaoUnits = 0 },
		"最小加价未对齐": func(m *ItemMarketConfig) {
			m.PriceStepYuanbaoUnits, m.MinimumRaiseYuanbaoUnits = 2, 3
		},
		"佣金不同": func(m *ItemMarketConfig) { m.SellerCommissionBP = 500 },
		"除数不同": func(m *ItemMarketConfig) { m.TaxDivisor = 20 },
	} {
		t.Run(name, func(t *testing.T) {
			fixture := Fixture{ItemMarket: &ItemMarketConfig{MaxListingsPerAccountRun: 5, AuctionSeconds: 7200, BuyoutSeconds: 86400,
				BidRaiseBP: 1000, PriceStepYuanbaoUnits: 1, MinimumRaiseYuanbaoUnits: 1,
				ExtendWhenRemainingBelowSeconds: 1200, ExtendToRemainingSeconds: 1200, SellerCommissionBP: 1000, TaxDivisor: 10}}
			change(fixture.ItemMarket)
			if err := fixture.validateItemMarket(); err == nil {
				t.Fatal("非法道具市场配置通过校验")
			}
		})
	}
}
