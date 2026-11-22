package devconfig

import (
	"errors"
	"slices"
	"strings"
)

type NPCSellPrice struct {
	ItemID    uint32 `json:"item_id"`
	PriceGold int64  `json:"price_gold"`
}

type NPCShop struct {
	Key        string         `json:"key"`
	Roles      []string       `json:"roles"`
	Offers     []SkillShop    `json:"offers"`
	SellPrices []NPCSellPrice `json:"sell_prices"`
	CanRepair  bool           `json:"can_repair"`
}

func (f *Fixture) NPCShopByRole(role string) *NPCShop {
	for i := range f.NPCShops {
		if slices.Contains(f.NPCShops[i].Roles, role) {
			return &f.NPCShops[i]
		}
	}
	return nil
}

func (f *Fixture) NPCShopByKey(key string) *NPCShop {
	for i := range f.NPCShops {
		if f.NPCShops[i].Key == key {
			return &f.NPCShops[i]
		}
	}
	return nil
}

func (s *NPCShop) Offer(id uint32) (SkillShop, bool) {
	for _, offer := range s.Offers {
		if offer.ID == id {
			return offer, true
		}
	}
	return SkillShop{}, false
}

func (s *NPCShop) SellPrice(id uint32) int64 {
	for _, price := range s.SellPrices {
		if price.ItemID == id {
			return price.PriceGold
		}
	}
	return 0
}

func (f *Fixture) validateNPCShops() error {
	if f.RuleVersion < 25 {
		if len(f.NPCShops) != 0 {
			return errors.New("NPC买卖配置须使用规则25")
		}
		return nil
	}
	rolesInTown := map[string]bool{}
	for _, m := range f.Maps {
		for _, npc := range m.NPCs {
			rolesInTown[npc.Role] = true
		}
	}
	keys, roles := map[string]bool{}, map[string]bool{}
	if len(f.NPCShops) == 0 {
		return errors.New("规则25缺少城镇商店配置")
	}
	for _, shop := range f.NPCShops {
		if strings.TrimSpace(shop.Key) == "" || keys[shop.Key] || len(shop.Roles) == 0 ||
			(len(shop.Offers) == 0 && len(shop.SellPrices) == 0 && !shop.CanRepair) {
			return errors.New("城镇商店须配置唯一标识和有效服务")
		}
		keys[shop.Key] = true
		for _, role := range shop.Roles {
			if roles[role] || !rolesInTown[role] {
				return errors.New("商店角色必须对应一个已配置NPC职位且不能重叠")
			}
			roles[role] = true
		}
		offers, sellItems := map[uint32]bool{}, map[uint32]bool{}
		for _, offer := range shop.Offers {
			if _, ok := f.Item(offer.ItemID); !ok || offer.ID == 0 || offers[offer.ID] || offer.Quantity <= 0 ||
				offer.PriceUnits <= 0 || offer.RequiredRoleLevel == 0 || offer.MaxPurchaseQuantity <= 0 ||
				(offer.Currency != "gold" && offer.Currency != "bound_ingot" && offer.Currency != "ingot") {
				return errors.New("城镇商店商品、价格、币种、数量或级别不合法")
			}
			offers[offer.ID] = true
		}
		for _, price := range shop.SellPrices {
			if _, ok := f.Item(price.ItemID); !ok || sellItems[price.ItemID] || price.PriceGold <= 0 {
				return errors.New("城镇商店收购物品须存在且有唯一正整数金币单价")
			}
			for _, offer := range shop.Offers {
				if offer.ItemID == price.ItemID && offer.Currency == "gold" && price.PriceGold > offer.PriceUnits/offer.Quantity {
					return errors.New("城镇商店同物品收购价超过金币出售价")
				}
			}
			sellItems[price.ItemID] = true
		}
	}
	return nil
}
