// 本文件计算金币委托的参考价和可接受的定点报价边界。
package persistence

import (
	"context"
	"fmt"
	"math"
	"math/big"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	p "minimir/internal/protocol"
)

// goldMarketTerms 以最近足量的有效成交手数计算精确成交量加权边界。
func (s *Store) goldMarketTerms(ctx context.Context) (*pb.GoldMarketRulesView, error) {
	fixture := &s.Cfg.Fixture
	rules := fixture.GoldMarket
	if rules == nil {
		return nil, nil
	}
	filter := bson.M{"run_server_id": s.Cfg.RunID, "reference_excluded": false}
	cursor, err := s.DB.Collection(goldMarketFills).Find(ctx, filter, options.Find().
		SetSort(bson.D{{Key: "sequence", Value: -1}}).
		SetProjection(bson.M{"lots": 1, "unit_price": 1, "_id": 0}).SetLimit(rules.PriceWindowLots))
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	remaining := rules.PriceWindowLots
	var weighted big.Int
	for remaining > 0 && cursor.Next(ctx) {
		var trade GoldMarketFill
		if err := cursor.Decode(&trade); err != nil {
			return nil, err
		}
		if trade.Lots <= 0 || trade.UnitPrice <= 0 {
			return nil, Fail(p.CodeOperationConflict, "参考成交保存了无效的价格或手数")
		}
		count := min(remaining, trade.Lots)
		weighted.Add(&weighted, new(big.Int).Mul(big.NewInt(count), big.NewInt(trade.UnitPrice)))
		remaining -= count
	}
	if err = cursor.Err(); err != nil {
		return nil, err
	}
	reference := rules.ReferencePriceYuanbaoUnits
	low, high, ok := rules.PriceBounds(reference)
	if remaining == 0 {
		reference, low, high, ok = goldMarketWeightedBounds(&weighted, rules)
	}
	if !ok {
		return nil, Fail(p.CodeLineForbidden, "金币委托缺少有效参考价")
	}
	terms := &pb.GoldMarketRulesView{
		RuleVersion: fixture.RuleVersion, ConfigHash: fixture.Hash,
		ReferenceUnitPriceYuanbaoUnits: reference, MinUnitPriceYuanbaoUnits: low, MaxUnitPriceYuanbaoUnits: high,
		PriceStepYuanbaoUnits: rules.PriceStepYuanbaoUnits, GoldPerLot: rules.GoldPerLot,
		OrderDurationSeconds: rules.OrderExpirySeconds, SellerCommissionBp: uint32(10_000 / rules.TaxDivisor),
		MaxOrdersPerSide: min(rules.MaxBuyOrdersPerAccountRun, rules.MaxSellOrdersPerAccountRun),
	}
	terms.TermsHash = Hash([]byte(fmt.Sprintf("gold-terms:%d:%d:%s:%d:%d:%d:%d:%d:%d:%d:%d",
		s.Cfg.RunID, terms.RuleVersion, terms.ConfigHash, reference, low, high,
		rules.PriceStepYuanbaoUnits, rules.GoldPerLot, rules.OrderExpirySeconds,
		rules.MaxBuyOrdersPerAccountRun, rules.MaxSellOrdersPerAccountRun)))
	return terms, nil
}

// goldMarketWeightedBounds 报价向精确区间内取整，避免先舍入参考价扩大允许区间。
func goldMarketWeightedBounds(weighted *big.Int, rules *devconfig.GoldMarketConfig) (reference, low, high int64, ok bool) {
	const basis = int64(10_000)
	lots, step, bp := rules.PriceWindowLots, rules.PriceStepYuanbaoUnits, int64(rules.PriceLimitBP)
	if weighted.Sign() <= 0 || lots <= 0 || step <= 0 || bp <= 0 || bp >= basis {
		return 0, 0, 0, false
	}
	average := new(big.Int).Quo(weighted, big.NewInt(lots))
	if !average.IsInt64() || average.Sign() <= 0 {
		return 0, 0, 0, false
	}
	// 分母乘入步长，下界向上、上界向下取整后再还原到最小账本单位。
	denominator := new(big.Int).Mul(big.NewInt(lots), big.NewInt(basis))
	denominator.Mul(denominator, big.NewInt(step))
	lower := new(big.Int).Mul(weighted, big.NewInt(basis-bp))
	lower.Add(lower, new(big.Int).Sub(denominator, big.NewInt(1)))
	lower.Quo(lower, denominator).Mul(lower, big.NewInt(step))
	upper := new(big.Int).Mul(weighted, big.NewInt(basis+bp))
	upper.Quo(upper, denominator).Mul(upper, big.NewInt(step))
	if !upper.IsInt64() {
		upper.SetInt64(math.MaxInt64 / step * step)
	}
	if !lower.IsInt64() || lower.Sign() <= 0 || lower.Cmp(upper) > 0 {
		return 0, 0, 0, false
	}
	return average.Int64(), lower.Int64(), upper.Int64(), true
}
