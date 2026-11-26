// 本文件定义可显式装配的金币撮合参数及安全的定点报价区间。
package devconfig

import (
	"errors"
	"math"
)

// GoldMarketConfig 保存确认的成交规则和单独装配的隔离市场参数。
type GoldMarketConfig struct {
	GoldPerLot                 int64  `json:"gold_per_lot"`                    // 每手冻结或交付的非绑定金币数量。
	PriceStepYuanbaoUnits      int64  `json:"price_step_yuanbao_units"`        // 每手报价的离散步长，单位为 0.1 元宝。
	MaxBuyOrdersPerAccountRun  uint32 `json:"max_buy_orders_per_account_run"`  // 同账号同运行服的活跃买单上限。
	MaxSellOrdersPerAccountRun uint32 `json:"max_sell_orders_per_account_run"` // 同账号同运行服的活跃卖单上限。
	TaxDivisor                 int64  `json:"tax_divisor"`                     // 两市场共用的卖方 10% 累计税余数除数。
	OrderExpirySeconds         uint32 `json:"order_expiry_seconds"`            // 隔离运行的显式挂单期限，单位秒。
	ReferencePriceYuanbaoUnits int64  `json:"reference_price_yuanbao_units"`   // 无足量成交时隔离运行使用的显式参考价。
	PriceWindowLots            int64  `json:"price_window_lots"`               // 参考价候选的最近有效成交手数。
	PriceLimitBP               uint32 `json:"price_limit_bp"`                  // 以参考价为中心的单侧价格限幅，万分比。
}

// PriceBounds 以正整数参考价计算允许的闭区间，将上下界向区间内对齐步长。
func (m *GoldMarketConfig) PriceBounds(reference int64) (minimum, maximum int64, ok bool) {
	const basis = int64(10_000)
	if m == nil || reference <= 0 || m.PriceStepYuanbaoUnits <= 0 || m.PriceLimitBP == 0 || int64(m.PriceLimitBP) >= basis {
		return 0, 0, false
	}
	// 商余拆分避免用大额参考价直接乘万分比。
	bp := int64(m.PriceLimitBP)
	delta := reference/basis*bp + reference%basis*bp/basis
	if reference > math.MaxInt64-delta {
		return 0, 0, false
	}
	minimum, maximum = reference-delta, reference+delta
	step := m.PriceStepYuanbaoUnits
	maximum -= maximum % step
	if tail := minimum % step; tail != 0 {
		adjust := step - tail
		if adjust > maximum-minimum {
			return 0, 0, false
		}
		minimum += adjust
	}
	if minimum > maximum {
		return 0, 0, false
	}
	return minimum, maximum, true
}

// validateGoldMarket 允许历史夹具省略，启用时必须装配期限、参考价和原共税范围。
func (f *Fixture) validateGoldMarket() error {
	m := f.GoldMarket
	if m == nil {
		return nil
	}
	if m.GoldPerLot != 10_000 || m.PriceStepYuanbaoUnits != 1 || m.MaxBuyOrdersPerAccountRun != 5 ||
		m.MaxSellOrdersPerAccountRun != 5 || m.TaxDivisor != 10 {
		return errors.New("金币撮合的手数单位、定点步长、委托上限或共税除数不符已确认规则")
	}
	if m.OrderExpirySeconds == 0 || m.PriceWindowLots <= 0 {
		return errors.New("金币撮合缺少显式期限或有效成交窗口")
	}
	if _, _, ok := m.PriceBounds(m.ReferencePriceYuanbaoUnits); !ok {
		return errors.New("金币撮合参考价、定点步长或报价区间无效")
	}
	return nil
}
