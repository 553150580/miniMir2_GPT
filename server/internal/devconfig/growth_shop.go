// 本文件校验元宝商城的商品、分币报价和每日材料限额。
package devconfig

import (
	"errors"
	"fmt"
	"strings"
)

const (
	GrowthShopYuanbao      uint32 = 1    // 只用普通元宝钱包。
	GrowthShopBoundYuanbao uint32 = 2    // 只用绑定元宝钱包。
	GrowthShopMaxCount     uint32 = 1000 // 单次请求能产生的最大购买份数。
)

// GrowthShop 只允许绑定成长材料进入独立的元宝商城。
type GrowthShop struct {
	Offers []GrowthShopOffer `json:"offers"` // 按原工作簿编号排列的可见报价。
}

// GrowthShopOffer 表示单个商品与币种。没有价格的原商品可在目录中显示待定状态。
type GrowthShopOffer struct {
	ID          uint32 `json:"id"`            // 唯一商品编号，可区分同物品的两个币种。
	ItemID      uint32 `json:"item_id"`       // 原材料的库存模板。
	Name        string `json:"name"`          // 工作簿内商品名称。
	Currency    uint32 `json:"currency"`      // 只允许普通元宝或绑定元宝，为零时商品待开放。
	PriceUnits  int64  `json:"price_units"`   // 每份最小货币单位。
	ItemPerPack uint32 `json:"item_per_pack"` // 每份包含的绑定材料数量。
	MaxCount    uint32 `json:"max_count"`     // 单次确认最多购买的份数。
	DailyLimit  uint32 `json:"daily_limit"`   // 同一角色、同种材料的日限量，零代表不限购。
}

// Offer 返回已经登记的原报价定义。
func (s *GrowthShop) Offer(id uint32) (GrowthShopOffer, bool) {
	if s != nil {
		for _, offer := range s.Offers {
			if offer.ID == id {
				return offer, true
			}
		}
	}
	return GrowthShopOffer{}, false
}

// validateGrowthShop 禁止引入装备或与旧书店重复售卖同一材料。
func (f *Fixture) validateGrowthShop() error {
	s := f.GrowthShop
	if s == nil {
		return nil
	}
	if f.RuleVersion < 30 || len(s.Offers) == 0 || len(s.Offers) > 16 {
		return errors.New("商城目录缺少受支持的版本或商品")
	}
	var previousID uint32
	var active int
	limits := make(map[uint32]uint32, len(s.Offers))
	currencies := make(map[[2]uint32]bool, len(s.Offers))
	for _, offer := range s.Offers {
		item, known := f.Item(offer.ItemID)
		isOil := f.LuckOil != nil && offer.ItemID == f.LuckOil.ItemID
		isPage := item.IsSkillPage && offer.ItemID == f.SkillEconomy.PageItemID
		isStone := f.EquipmentRefine != nil && offer.ItemID == f.EquipmentRefine.StoneItemID
		if !known || (!isPage && !isOil && !isStone) || !item.Stackable || item.MaxStack <= 0 || item.IsEquipment ||
			item.EquipmentSlot != 0 || item.BookSkillID != 0 || item.RecycleGold != 0 || f.IsCastingSupply(item.ID) ||
			offer.ID <= previousID || strings.TrimSpace(offer.Name) == "" || offer.ItemPerPack == 0 || int64(offer.ItemPerPack) > item.MaxStack {
			return fmt.Errorf("商城商品 %d 须引用现有可堆叠的独立成长材料", offer.ID)
		}
		previousID = offer.ID
		key := [2]uint32{offer.ItemID, offer.Currency}
		limit, exists := limits[offer.ItemID]
		if currencies[key] || exists && limit != offer.DailyLimit {
			return errors.New("同一种材料的报价币种须唯一，且共同使用每日上限")
		}
		limits[offer.ItemID], currencies[key] = offer.DailyLimit, true
		if offer.Currency == 0 {
			if offer.PriceUnits != 0 || offer.MaxCount != 0 || offer.DailyLimit != 0 {
				return errors.New("待开放商品不能携带可结算的条款")
			}
			continue
		}
		if (offer.Currency != GrowthShopYuanbao && offer.Currency != GrowthShopBoundYuanbao) || offer.PriceUnits <= 0 ||
			offer.PriceUnits > 1_000_000_000 || offer.MaxCount == 0 || offer.MaxCount > GrowthShopMaxCount {
			return errors.New("商城价格须明确一种元宝钱包，且数量有上限")
		}
		active++
	}
	if active == 0 {
		return errors.New("商城至少须有一个可购买的商品")
	}
	for _, offer := range f.SkillShop {
		if _, exists := limits[offer.ItemID]; exists {
			return errors.New("商城材料的商城价格和限额须使用同一个购买入口")
		}
	}
	for _, shop := range f.NPCShops {
		for _, offer := range shop.Offers {
			if _, exists := limits[offer.ItemID]; exists {
				return errors.New("普通 NPC 商店不能另售商城专供材料")
			}
		}
	}
	return nil
}
