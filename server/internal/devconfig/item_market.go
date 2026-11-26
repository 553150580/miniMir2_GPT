// 本文件定义由唯一玩法工作簿装配的道具竞购和共享佣金参数。
package devconfig

import "errors"

// ItemMarketConfig 保存道具市场的期限、定点报价和两市场共用的佣金参数。
type ItemMarketConfig struct {
	MaxListingsPerAccountRun        uint32 `json:"max_listings_per_account_run"`        // 同账号同运行服的同时在售上限。
	AuctionSeconds                  uint32 `json:"auction_seconds"`                     // 初始竞购期的长度，单位秒。
	BuyoutSeconds                   uint32 `json:"buyout_seconds"`                      // 无人竞购转入直购后的时限，单位秒。
	BidRaiseBP                      uint32 `json:"bid_raise_bp"`                        // 有效预购者主动加价的比例，万分比。
	PriceStepYuanbaoUnits           int64  `json:"price_step_yuanbao_units"`            // 报价向上对齐的定点步长，单位为 0.1 元宝。
	MinimumRaiseYuanbaoUnits        int64  `json:"minimum_raise_yuanbao_units"`         // 每次加价增加的最小定点金额。
	ExtendWhenRemainingBelowSeconds uint32 `json:"extend_when_remaining_below_seconds"` // 仅剩余时间严格小于该值时才延时。
	ExtendToRemainingSeconds        uint32 `json:"extend_to_remaining_seconds"`         // 延时后的剩余竞购时间，单位秒。
	SellerCommissionBP              uint32 `json:"seller_commission_bp"`                // 道具和金币市场采用的同一卖方佣金比例，万分比。
	TaxDivisor                      int64  `json:"tax_divisor"`                         // 套用共享税余数公式的除数，余数取 0 到本值减一。
}

// validateItemMarket 检查报价和时间可用，并保持共用税余数的原除数和佣金口径。
func (f *Fixture) validateItemMarket() error {
	m := f.ItemMarket
	if m == nil {
		return nil
	}
	if m.MaxListingsPerAccountRun == 0 || m.AuctionSeconds == 0 || m.BuyoutSeconds == 0 ||
		m.ExtendWhenRemainingBelowSeconds == 0 || m.ExtendToRemainingSeconds < m.ExtendWhenRemainingBelowSeconds {
		return errors.New("道具市场缺少有效上架预算、竞购或直购时限")
	}
	if m.BidRaiseBP == 0 || m.BidRaiseBP > 10_000 || m.PriceStepYuanbaoUnits <= 0 ||
		m.MinimumRaiseYuanbaoUnits < m.PriceStepYuanbaoUnits || m.MinimumRaiseYuanbaoUnits%m.PriceStepYuanbaoUnits != 0 {
		return errors.New("道具市场加价比例、报价步长或最小加价无效")
	}
	if m.SellerCommissionBP != 1000 || m.TaxDivisor != 10 {
		return errors.New("道具市场佣金须与双市场共享的 10% 税余数规则一致")
	}
	return nil
}
