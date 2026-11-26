// 本文件校验唯一玩法表中的角色现金卡商品及其人民币分金额。
package devconfig

import (
	"bytes"
	"encoding/json"
	"errors"
)

// PaymentConfig 装配供服务端原单冻结使用的现金卡商品。渠道凭据和充值档位不属于该配置。
type PaymentConfig struct {
	CashCardProducts []CashCardProduct `json:"cash_card_products"` // 两种角色卡分别提供月、季、年三种期限。
}

// CashCardProduct 定义一份角色现金卡商品的原价和完整有效期。
type CashCardProduct struct {
	ID       uint32 `json:"id"`        // 唯一玩法工作簿中的商品编号。
	Kind     uint32 `json:"kind"`      // 特权卡或托管卡，沿用原角色权益编号。
	Days     uint32 `json:"days"`      // 付款核验后增加的完整有效天数。
	PriceFen int64  `json:"price_fen"` // 人民币整数分，供核对原单及渠道金额。
}

// UnmarshalJSON 校验独立目录结构，防止隔离装配误带额外资产、渠道或付款币种字段。
func (c *PaymentConfig) UnmarshalJSON(body []byte) error {
	type cashCatalog PaymentConfig
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	return decoder.Decode((*cashCatalog)(c))
}

// CashCardProduct 只按现金目录查询原商品，缺省配置或游戏内元宝商品均返回未找到。
func (f *Fixture) CashCardProduct(id uint32) (CashCardProduct, bool) {
	if f == nil || f.Payment == nil || id == 0 {
		return CashCardProduct{}, false
	}
	for _, product := range f.Payment.CashCardProducts {
		if product.ID == id {
			return product, true
		}
	}
	return CashCardProduct{}, false
}

// validatePayment 校验两种卡的月、季、年各一份，并禁止现金与元宝商品共享编号。
func (f *Fixture) validatePayment() error {
	if f.Payment == nil {
		return nil
	}
	products := f.Payment.CashCardProducts
	if len(products) != 6 {
		return errors.New("现金卡目录须为两种角色各提供月卡、季卡和年卡")
	}
	ids := make(map[uint32]bool, len(products))
	terms := make(map[[2]uint32]bool, len(products))
	for _, product := range products {
		if product.ID == 0 || product.PriceFen <= 0 || (product.Kind != RoleCardKindPrivilege && product.Kind != RoleCardKindHosting) ||
			(product.Days != roleCardMonthDays && product.Days != 90 && product.Days != 365) {
			return errors.New("现金卡商品的编号、卡种、期限或人民币分金额无效")
		}
		key := [2]uint32{product.Kind, product.Days}
		_, isYuanbaoProduct := f.RoleCardProduct(product.ID)
		if ids[product.ID] || terms[key] || isYuanbaoProduct {
			return errors.New("现金卡商品编号和卡种期限须唯一，且编号不能与元宝商品重叠")
		}
		ids[product.ID], terms[key] = true, true
	}
	return nil
}
