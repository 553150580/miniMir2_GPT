// 本文件核验精确成交量加权报价的定点边界。
package persistence

import (
	"math"
	"math/big"
	"testing"

	"minimir/internal/devconfig"
)

func TestGoldMarketWeightedBoundsUseExactAverage(t *testing.T) {
	rules := &devconfig.GoldMarketConfig{PriceWindowLots: 2, PriceStepYuanbaoUnits: 1, PriceLimitBP: 2000}
	ref, low, high, ok := goldMarketWeightedBounds(big.NewInt(21), rules)
	if !ok || ref != 10 || low != 9 || high != 12 {
		t.Fatalf("每手 10.5 的精确加权限幅应为 [9, 12]，实际 %d [%d, %d]", ref, low, high)
	}
	extreme := new(big.Int).Mul(big.NewInt(math.MaxInt64), big.NewInt(rules.PriceWindowLots))
	ref, low, high, ok = goldMarketWeightedBounds(extreme, rules)
	if !ok || ref != math.MaxInt64 || high != math.MaxInt64 || low <= 0 {
		t.Fatalf("高额参考价不得造成定点溢出或反向新区间，实际 %d [%d, %d]", ref, low, high)
	}
}
