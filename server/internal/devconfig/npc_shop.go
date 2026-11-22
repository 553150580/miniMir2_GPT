// 本文件装配 NPC 商品目录、收购类型和维修能力，并核对地图与物品引用。

package devconfig

import (
	"errors"
	"slices"
	"strings"
)

// NPCSellPrice 保存NPC“sell”“price”所需的状态。
type NPCSellPrice struct {
	// ItemID 标识当前业务使用的道具。
	ItemID uint32 `json:"item_id"`
	// PriceGold 记录“price”金币，使用数值表示。
	PriceGold int64 `json:"price_gold"`
}

// NPCShop 保存NPC商店所需的状态。
type NPCShop struct {
	// Key 记录键，使用文本表示。
	Key string `json:"key"`
	// Roles 记录角色集合，使用文本列表表示。
	Roles []string `json:"roles"`
	// Offers 记录“offers”，使用技能商店列表表示。
	Offers []SkillShop `json:"offers"`
	// SellPrices 记录“sell”“prices”，使用NPC“sell”“price”列表表示。
	SellPrices []NPCSellPrice `json:"sell_prices"`
	// CanRepair 标记是否处于“can”修理状态。
	CanRepair bool `json:"can_repair"`
}

// NPCShopByRole 返回NPC商店“by”角色供调用方使用。
// f 是调用该方法的开发测试配置实例，role 记录角色，使用文本表示。
func (f *Fixture) NPCShopByRole(role string) *NPCShop {
	// i 标记遍历NPC商店集合时访问的下标。
	for i := range f.NPCShops {
		if slices.Contains(f.NPCShops[i].Roles, role) {
			return &f.NPCShops[i]
		}
	}
	return nil
}

// NPCShopByKey 返回NPC商店“by”键供调用方使用。
// f 是调用该方法的开发测试配置实例，key 记录键，使用文本表示。
func (f *Fixture) NPCShopByKey(key string) *NPCShop {
	// i 标记遍历NPC商店集合时访问的下标。
	for i := range f.NPCShops {
		if f.NPCShops[i].Key == key {
			return &f.NPCShops[i]
		}
	}
	return nil
}

// Offer 尝试投递当前请求或状态，并把结果交给调用方。
// s 是调用该方法的NPC商店实例，id 区分当前步骤指向的目标对象。
func (s *NPCShop) Offer(id uint32) (SkillShop, bool) {
	// offer 接收遍历“offers”时当前的元素。
	for _, offer := range s.Offers {
		if offer.ID == id {
			return offer, true
		}
	}
	return SkillShop{}, false
}

// SellPrice 返回“sell”“price”供调用方使用。
// s 是调用该方法的NPC商店实例，id 区分当前步骤指向的目标对象。
func (s *NPCShop) SellPrice(id uint32) int64 {
	// price 接收遍历“sell”“prices”时当前的元素。
	for _, price := range s.SellPrices {
		if price.ItemID == id {
			return price.PriceGold
		}
	}
	return 0
}

// validateNPCShops 校验NPC商店集合。
// f 是调用该方法的开发测试配置实例。
func (f *Fixture) validateNPCShops() error {
	if f.RuleVersion < 25 {
		if len(f.NPCShops) != 0 {
			return errors.New("NPC买卖配置须使用规则25")
		}
		return nil
	}
	// rolesInTown 初始化用作角色集合输入城镇的按文本索引的布尔状态映射。
	rolesInTown := map[string]bool{}
	// m 接收遍历地图集合时当前的元素。
	for _, m := range f.Maps {
		// npc 接收遍历“np”“cs”时当前的元素。
		for _, npc := range m.NPCs {
			rolesInTown[npc.Role] = true
		}
	}
	// keys 初始化用作键集合的按文本索引的布尔状态映射，roles 初始化用作角色集合的按文本索引的布尔状态映射。
	keys, roles := map[string]bool{}, map[string]bool{}
	if len(f.NPCShops) == 0 {
		return errors.New("规则25缺少城镇商店配置")
	}
	// shop 接收遍历NPC商店集合时当前的元素。
	for _, shop := range f.NPCShops {
		if strings.TrimSpace(shop.Key) == "" || keys[shop.Key] || len(shop.Roles) == 0 ||
			(len(shop.Offers) == 0 && len(shop.SellPrices) == 0 && !shop.CanRepair) {
			return errors.New("城镇商店须配置唯一标识和有效服务")
		}
		keys[shop.Key] = true
		// role 接收遍历角色集合时当前的元素。
		for _, role := range shop.Roles {
			if roles[role] || !rolesInTown[role] {
				return errors.New("商店角色必须对应一个已配置NPC职位且不能重叠")
			}
			roles[role] = true
		}
		// offers 建立带初始值的按数值索引的布尔状态映射，供本段继续处理，sellItems 建立带初始值的按数值索引的布尔状态映射，供本段继续处理。
		offers, sellItems := map[uint32]bool{}, map[uint32]bool{}
		// offer 接收遍历“offers”时当前的元素。
		for _, offer := range shop.Offers {
			// ok 表明查询、校验或类型转换是否成功。
			if _, ok := f.Item(offer.ItemID); !ok || offer.ID == 0 || offers[offer.ID] || offer.Quantity <= 0 ||
				offer.PriceUnits <= 0 || offer.RequiredRoleLevel == 0 || offer.MaxPurchaseQuantity <= 0 ||
				(offer.Currency != "gold" && offer.Currency != "bound_ingot" && offer.Currency != "ingot") {
				return errors.New("城镇商店商品、价格、币种、数量或级别不合法")
			}
			offers[offer.ID] = true
		}
		// price 接收遍历“sell”“prices”时当前的元素。
		for _, price := range shop.SellPrices {
			// ok 表明查询、校验或类型转换是否成功。
			if _, ok := f.Item(price.ItemID); !ok || sellItems[price.ItemID] || price.PriceGold <= 0 {
				return errors.New("城镇商店收购物品须存在且有唯一正整数金币单价")
			}
			// offer 接收遍历“offers”时当前的元素。
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
