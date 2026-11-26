// 本文件定义非绑定元宝购买角色月卡和特权卡每日领取绑定元宝的配置。
package devconfig

import "errors"

const (
	RoleCardKindPrivilege uint32 = 1  // 特权月卡。
	RoleCardKindHosting   uint32 = 2  // 托管月卡。
	roleCardMonthDays     uint32 = 30 // 元宝渠道仅提供固定天数的月卡。
)

// RoleCardConfig 仅装配角色级非绑定元宝月卡和特权卡每日领取额度。
type RoleCardConfig struct {
	Products            []RoleCardProduct `json:"products"`              // 可在游戏内购买的两种月卡。
	DailyPrivilegeUnits int64             `json:"daily_privilege_units"` // 有效特权卡每日手动领取的绑定元宝定点单位。
}

// RoleCardProduct 表示唯一工作簿中已确认的角色月卡商品。
type RoleCardProduct struct {
	ID                uint32 `json:"id"`                  // 工作簿商品编号。
	Kind              uint32 `json:"kind"`                // 特权卡或托管卡。
	Days              uint32 `json:"days"`                // 支付成功后增加的有效天数。
	PriceYuanbaoUnits int64  `json:"price_yuanbao_units"` // 非绑定元宝定点单位，单位为 0.1 元宝。
}

// RoleCardProduct 用已装配编号查找商品，旧规则包和未开放编号均返回未找到。
func (f *Fixture) RoleCardProduct(id uint32) (RoleCardProduct, bool) {
	if f == nil || f.RoleCards == nil || id == 0 {
		return RoleCardProduct{}, false
	}
	for _, product := range f.RoleCards.Products {
		if product.ID == id {
			return product, true
		}
	}
	return RoleCardProduct{}, false
}

// validateRoleCards 要求两种月卡各一条、商品身份唯一，并校验每日领取与牌价。
func (f *Fixture) validateRoleCards() error {
	cards := f.RoleCards
	if cards == nil {
		return nil
	}
	if len(cards.Products) != 2 || cards.DailyPrivilegeUnits <= 0 {
		return errors.New("角色月卡配置缺少完整商品或每日领取额度")
	}
	for _, product := range cards.Products {
		if product.ID == 0 || product.Kind != RoleCardKindPrivilege && product.Kind != RoleCardKindHosting ||
			product.Days != roleCardMonthDays || product.PriceYuanbaoUnits <= 0 {
			return errors.New("角色月卡商品的编号、类型、期限或价格无效")
		}
	}
	if first, second := cards.Products[0], cards.Products[1]; first.ID == second.ID || first.Kind == second.Kind {
		return errors.New("角色月卡的商品编号和卡种必须唯一")
	}
	return nil
}
