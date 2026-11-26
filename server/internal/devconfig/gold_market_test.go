// 本文件核对金币市场默认关闭、隔离装配与定点区间的极值。
package devconfig

import (
	"math"
	"testing"
)

// TestGoldMarketDefaultDisabled 验证真实开发发布没有启用未审定的市场参数。
func TestGoldMarketDefaultDisabled(t *testing.T) {
	cfg, err := Load("../../../conf/dev/server.json")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Fixture.GoldMarket != nil {
		t.Fatal("默认开发规则提前开放未审定的金币市场")
	}
}

// TestGoldMarketPriceBounds 核对闭区间、最小整数报价、步长舍入和确定的溢出拒绝。
func TestGoldMarketPriceBounds(t *testing.T) {
	for _, tc := range []struct {
		name     string // 样本覆盖的边界来源。
		ref      int64  // 运行时传入的整数参考价。
		step     int64  // 实际报价的最小步长。
		minimum  int64  // 向上对齐后的区间下限。
		maximum  int64  // 向下对齐后的区间上限。
		accepted bool   // 区间是否可用。
	}{
		{"常用定点价", 100, 1, 80, 120, true},
		{"单单位参考价", 1, 1, 1, 1, true},
		{"非整二成在上下界舍入", 11, 1, 9, 13, true},
		{"按步长向内对齐", 103, 5, 85, 120, true},
		{"窄价区间内无可用报价", 2, 3, 0, 0, false},
		{"参考价为零", 0, 1, 0, 0, false},
		{"上限溢出", math.MaxInt64, 1, 0, 0, false},
		{"不直接将大额参考价乘万分比", math.MaxInt64 / 2, 1, 3689348814741910323, 5534023222112865483, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			market := GoldMarketConfig{PriceStepYuanbaoUnits: tc.step, PriceLimitBP: 2000}
			minimum, maximum, ok := market.PriceBounds(tc.ref)
			if minimum != tc.minimum || maximum != tc.maximum || ok != tc.accepted {
				t.Fatalf("报价区间为 %d 到 %d，允许=%v，预期 %d 到 %d，允许=%v", minimum, maximum, ok, tc.minimum, tc.maximum, tc.accepted)
			}
		})
	}
}

// TestValidateGoldMarket 拒绝不完整的隔离输入、不同税除数和无效定点上下界。
func TestValidateGoldMarket(t *testing.T) {
	valid := GoldMarketConfig{GoldPerLot: 10_000, PriceStepYuanbaoUnits: 1, MaxBuyOrdersPerAccountRun: 5,
		MaxSellOrdersPerAccountRun: 5, TaxDivisor: 10, OrderExpirySeconds: 120, ReferencePriceYuanbaoUnits: 100,
		PriceWindowLots: 5_000, PriceLimitBP: 2_000}
	if err := (&Fixture{GoldMarket: &valid}).validateGoldMarket(); err != nil {
		t.Fatal(err)
	}
	for name, change := range map[string]func(*GoldMarketConfig){
		"每手金币不同": func(m *GoldMarketConfig) { m.GoldPerLot = 10_001 },
		"缺期限":    func(m *GoldMarketConfig) { m.OrderExpirySeconds = 0 },
		"缺参考价":   func(m *GoldMarketConfig) { m.ReferencePriceYuanbaoUnits = 0 },
		"额度不同":   func(m *GoldMarketConfig) { m.MaxSellOrdersPerAccountRun = 4 },
		"缺窗口":    func(m *GoldMarketConfig) { m.PriceWindowLots = 0 },
		"限幅超界":   func(m *GoldMarketConfig) { m.PriceLimitBP = 10000 },
		"定点步长不同": func(m *GoldMarketConfig) { m.PriceStepYuanbaoUnits = 3 },
		"税除数不符":  func(m *GoldMarketConfig) { m.TaxDivisor = 20 },
	} {
		t.Run(name, func(t *testing.T) {
			market := valid
			change(&market)
			if err := (&Fixture{GoldMarket: &market}).validateGoldMarket(); err == nil {
				t.Fatal("无效的金币市场配置通过校验")
			}
		})
	}
}
