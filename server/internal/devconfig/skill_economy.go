// 本文件建立职业技能学书、升级材料、金币费用与技能书商店之间的版本合同。

package devconfig

import (
	"errors"
	"math"
)

// ItemCost 保存道具消耗所需的状态。
type ItemCost struct {
	// ItemID 标识当前业务使用的道具。
	ItemID uint32 `json:"item_id"`
	// Quantity 记录数量，使用数值表示。
	Quantity int64 `json:"quantity"`
}

// SkillUpgrade 保存技能升级所需的状态。
type SkillUpgrade struct {
	// SkillID 标识当前业务使用的技能。
	SkillID uint32 `json:"skill_id"`
	// Level 记录等级，使用数值表示。
	Level uint32 `json:"level"`
	// RequiredRoleLevel 记录“required”角色等级，使用数值表示。
	RequiredRoleLevel uint32 `json:"required_role_level"`
	// BookCosts 记录技能书“costs”，使用道具消耗列表表示。
	BookCosts []ItemCost `json:"book_costs"`
	// PageItemID 标识当前业务使用的页面或分页道具。
	PageItemID uint32 `json:"page_item_id"`
	// PageCost 记录页面或分页消耗，使用数值表示。
	PageCost int64 `json:"page_cost"`
}

// SkillShop 保存技能商店所需的状态。
type SkillShop struct {
	// ID 区分当前步骤指向的目标对象。
	ID uint32 `json:"id"`
	// ItemID 标识当前业务使用的道具。
	ItemID uint32 `json:"item_id"`
	// Quantity 记录数量，使用数值表示。
	Quantity int64 `json:"quantity"`
	// Currency 记录“currency”，使用文本表示。
	Currency string `json:"currency"`
	// PriceUnits 记录“price”单位集合，使用数值表示。
	PriceUnits int64 `json:"price_units"`
	// RequiredRoleLevel 记录“required”角色等级，使用数值表示。
	RequiredRoleLevel uint32 `json:"required_role_level"`
	// OutputBound 标记是否处于输出绑定状态。
	OutputBound bool `json:"output_bound"`
	// MaxPurchaseQuantity 记录最大“purchase”数量，使用数值表示。
	MaxPurchaseQuantity int64 `json:"max_purchase_quantity"`
}

// SkillBookRecycle 保存技能技能书回收所需的状态。
type SkillBookRecycle struct {
	// ItemID 标识当前业务使用的道具。
	ItemID uint32 `json:"item_id"`
	// BookQuality 记录技能书品质，使用数值表示。
	BookQuality uint32 `json:"book_quality"`
	// PageItemID 标识当前业务使用的页面或分页道具。
	PageItemID uint32 `json:"page_item_id"`
	// PagesPerBook 记录“pages”“per”技能书，使用数值表示。
	PagesPerBook int64 `json:"pages_per_book"`
	// AllowedSourceKinds 记录允许来源“kinds”，使用文本列表表示。
	AllowedSourceKinds []string `json:"allowed_source_kinds"`
}

// SkillEconomy 保存技能经济所需的状态。
type SkillEconomy struct {
	// PageItemID 标识当前业务使用的页面或分页道具。
	PageItemID uint32 `json:"page_item_id"`
	// PreviewTTLMS 记录预览“ttlms”，使用数值表示。
	PreviewTTLMS int64 `json:"preview_ttl_ms"`
	// PreviewMaterialLimit 记录预览材料限额，使用数值表示。
	PreviewMaterialLimit int `json:"preview_material_limit"`
}

// Upgrade 处理升级。
// f 是调用该方法的开发测试配置实例，id 区分当前步骤指向的目标对象，level 记录等级，使用数值表示。
func (f *Fixture) Upgrade(id, level uint32) (SkillUpgrade, bool) {
	// u 接收遍历技能“upgrades”时当前的元素。
	for _, u := range f.SkillUpgrades {
		if u.SkillID == id && u.Level == level {
			return u, true
		}
	}
	return SkillUpgrade{}, false
}

// ShopOffer 处理商店尝试提交。
// f 是调用该方法的开发测试配置实例，id 区分当前步骤指向的目标对象。
func (f *Fixture) ShopOffer(id uint32) (SkillShop, bool) {
	// s 接收遍历技能商店时当前的元素。
	for _, s := range f.SkillShop {
		if s.ID == id {
			return s, true
		}
	}
	return SkillShop{}, false
}

// BookRecycle 处理技能书回收。
// f 是调用该方法的开发测试配置实例，id 区分当前步骤指向的目标对象，quality 记录品质，使用数值表示。
func (f *Fixture) BookRecycle(id, quality uint32) (SkillBookRecycle, bool) {
	// r 接收遍历技能技能书回收时当前的元素。
	for _, r := range f.SkillBookRecycle {
		if r.ItemID == id && r.BookQuality == quality {
			return r, true
		}
	}
	return SkillBookRecycle{}, false
}

// ValidateSkillEconomy 校验技能经济。
// f 是调用该方法的开发测试配置实例。
func (f *Fixture) ValidateSkillEconomy() error {
	// e 保存当前处理所用的技能经济。
	e := f.SkillEconomy
	// page 保存由道具取得的页面或分页，ok 表明查询、校验或类型转换是否成功。
	page, ok := f.Item(e.PageItemID)
	if !ok || !page.IsSkillPage || page.MaxStack <= 0 || e.PreviewTTLMS <= 0 || e.PreviewTTLMS > math.MaxInt32 || e.PreviewMaterialLimit < 1 || e.PreviewMaterialLimit > f.InventoryCapacity {
		return errors.New("技能经济书页/预览预算无效")
	}
	// s 接收遍历技能集合时当前的元素。
	for _, s := range f.Skills {
		// rank 保存根据已学习等级与当前数据计算的排序位次。
		for rank := s.LearnedLevel + 1; rank <= s.MaxLevel; rank++ {
			// u 保存升级返回的结果，exists 保存由升级取得的存在。
			u, exists := f.Upgrade(s.ID, rank)
			if !exists || u.PageItemID != e.PageItemID || u.PageCost < 0 || u.RequiredRoleLevel < s.LearnLevel || len(u.BookCosts) == 0 {
				return errors.New("逐技能升级等级/成本不完整")
			}
		}
	}
	// seen 建立带初始值的按数值列表索引的布尔状态映射，供本段继续处理。
	seen := map[[2]uint32]bool{}
	// u 接收遍历技能“upgrades”时当前的元素。
	for _, u := range f.SkillUpgrades {
		// s 接收技能供后续步骤使用，exists 保存由技能取得的存在。
		s, exists := f.Skill(u.SkillID)
		// key 初始化用作键的数值列表。
		key := [2]uint32{u.SkillID, u.Level}
		if !exists || seen[key] || u.Level <= s.LearnedLevel || u.Level > s.MaxLevel || u.PageCost < 0 || len(u.BookCosts) != 1 {
			return errors.New("升级行重复或等级越界")
		}
		seen[key] = true
		// items 初始化用作道具集合的按数值索引的布尔状态映射。
		items := map[uint32]bool{}
		// c 接收遍历技能书“costs”时当前的元素。
		for _, c := range u.BookCosts {
			// b 保存由道具取得的b，ok 表明查询、校验或类型转换是否成功。
			b, ok := f.Item(c.ItemID)
			if !ok || b.BookSkillID != s.ID || c.ItemID != s.BookItemID || c.Quantity != int64(u.Level)+1 || items[c.ItemID] {
				return errors.New("升级书本引用或数量无效")
			}
			items[c.ItemID] = true
		}
	}
	// shops 初始化用作商店集合的按数值索引的布尔状态映射。
	shops := map[uint32]bool{}
	// s 接收遍历技能商店时当前的元素。
	for _, s := range f.SkillShop {
		// i 保存由道具取得的当前位置的下标，exists 保存由道具取得的存在。
		i, exists := f.Item(s.ItemID)
		if !exists || s.ID == 0 || shops[s.ID] || (!i.IsSkillPage && i.BookSkillID == 0 && !f.IsCastingSupply(i.ID)) || f.IsCastingSupply(i.ID) && !f.CastingSupply.Enabled || s.Quantity <= 0 || s.PriceUnits <= 0 || s.MaxPurchaseQuantity <= 0 || !s.OutputBound || (s.Currency != "gold" && s.Currency != "ingot" && s.Currency != "bound_ingot") {
			return errors.New("技能商店商品/币种/绑定无效")
		}
		shops[s.ID] = true
	}
	// recycle 初始化用作回收的按数值列表索引的布尔状态映射。
	recycle := map[[2]uint32]bool{}
	// r 接收遍历技能技能书回收时当前的元素。
	for _, r := range f.SkillBookRecycle {
		// b 保存道具返回的结果，exists 保存由道具取得的存在。
		b, exists := f.Item(r.ItemID)
		// p 接收道具供后续步骤使用，found 保存由道具取得的已找到标记。
		p, found := f.Item(r.PageItemID)
		// key 初始化用作键的数值列表。
		key := [2]uint32{r.ItemID, r.BookQuality}
		if !exists || b.BookSkillID == 0 || !found || !p.IsSkillPage || r.PagesPerBook <= 0 || len(r.AllowedSourceKinds) == 0 || recycle[key] {
			return errors.New("技能书唯一分解规则无效")
		}
		// kind 接收遍历允许来源“kinds”时当前的元素。
		for _, kind := range r.AllowedSourceKinds {
			if kind != "monster_drop" {
				return errors.New("未审定来源禁止拆页")
			}
		}
		recycle[key] = true
	}
	return nil
}
