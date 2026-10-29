package devconfig

import (
	"errors"
	"math"
)

type ItemCost struct {
	ItemID   uint32 `json:"item_id"`
	Quantity int64  `json:"quantity"`
}
type SkillUpgrade struct {
	SkillID           uint32     `json:"skill_id"`
	Level             uint32     `json:"level"`
	RequiredRoleLevel uint32     `json:"required_role_level"`
	BookCosts         []ItemCost `json:"book_costs"`
	PageItemID        uint32     `json:"page_item_id"`
	PageCost          int64      `json:"page_cost"`
}
type SkillShop struct {
	ID                  uint32 `json:"id"`
	ItemID              uint32 `json:"item_id"`
	Quantity            int64  `json:"quantity"`
	Currency            string `json:"currency"`
	PriceUnits          int64  `json:"price_units"`
	RequiredRoleLevel   uint32 `json:"required_role_level"`
	OutputBound         bool   `json:"output_bound"`
	MaxPurchaseQuantity int64  `json:"max_purchase_quantity"`
}
type SkillBookRecycle struct {
	ItemID             uint32   `json:"item_id"`
	BookQuality        uint32   `json:"book_quality"`
	PageItemID         uint32   `json:"page_item_id"`
	PagesPerBook       int64    `json:"pages_per_book"`
	AllowedSourceKinds []string `json:"allowed_source_kinds"`
}
type SkillEconomy struct {
	PageItemID           uint32 `json:"page_item_id"`
	PreviewTTLMS         int64  `json:"preview_ttl_ms"`
	PreviewMaterialLimit int    `json:"preview_material_limit"`
}

func (f *Fixture) Upgrade(id, level uint32) (SkillUpgrade, bool) {
	for _, u := range f.SkillUpgrades {
		if u.SkillID == id && u.Level == level {
			return u, true
		}
	}
	return SkillUpgrade{}, false
}
func (f *Fixture) ShopOffer(id uint32) (SkillShop, bool) {
	for _, s := range f.SkillShop {
		if s.ID == id {
			return s, true
		}
	}
	return SkillShop{}, false
}
func (f *Fixture) BookRecycle(id, quality uint32) (SkillBookRecycle, bool) {
	for _, r := range f.SkillBookRecycle {
		if r.ItemID == id && r.BookQuality == quality {
			return r, true
		}
	}
	return SkillBookRecycle{}, false
}
func (f *Fixture) ValidateSkillEconomy() error {
	e := f.SkillEconomy
	page, ok := f.Item(e.PageItemID)
	if !ok || !page.IsSkillPage || page.MaxStack <= 0 || e.PreviewTTLMS <= 0 || e.PreviewTTLMS > math.MaxInt32 || e.PreviewMaterialLimit < 1 || e.PreviewMaterialLimit > f.InventoryCapacity {
		return errors.New("技能经济书页/预览预算无效")
	}
	for _, s := range f.Skills {
		for rank := s.LearnedLevel + 1; rank <= s.MaxLevel; rank++ {
			u, exists := f.Upgrade(s.ID, rank)
			if !exists || u.PageItemID != e.PageItemID || u.PageCost < 0 || u.RequiredRoleLevel < s.LearnLevel || len(u.BookCosts) == 0 {
				return errors.New("逐技能升级等级/成本不完整")
			}
		}
	}
	seen := map[[2]uint32]bool{}
	for _, u := range f.SkillUpgrades {
		s, exists := f.Skill(u.SkillID)
		key := [2]uint32{u.SkillID, u.Level}
		if !exists || seen[key] || u.Level <= s.LearnedLevel || u.Level > s.MaxLevel || u.PageCost < 0 || len(u.BookCosts) != 1 {
			return errors.New("升级行重复或等级越界")
		}
		seen[key] = true
		items := map[uint32]bool{}
		for _, c := range u.BookCosts {
			b, ok := f.Item(c.ItemID)
			if !ok || b.BookSkillID != s.ID || c.ItemID != s.BookItemID || c.Quantity != int64(u.Level)+1 || items[c.ItemID] {
				return errors.New("升级书本引用或数量无效")
			}
			items[c.ItemID] = true
		}
	}
	shops := map[uint32]bool{}
	for _, s := range f.SkillShop {
		i, exists := f.Item(s.ItemID)
		if !exists || s.ID == 0 || shops[s.ID] || (!i.IsSkillPage && i.BookSkillID == 0 && !f.IsCastingSupply(i.ID)) || f.IsCastingSupply(i.ID) && !f.CastingSupply.Enabled || s.Quantity <= 0 || s.PriceUnits <= 0 || s.MaxPurchaseQuantity <= 0 || !s.OutputBound || (s.Currency != "gold" && s.Currency != "ingot" && s.Currency != "bound_ingot") {
			return errors.New("技能商店商品/币种/绑定无效")
		}
		shops[s.ID] = true
	}
	recycle := map[[2]uint32]bool{}
	for _, r := range f.SkillBookRecycle {
		b, exists := f.Item(r.ItemID)
		p, found := f.Item(r.PageItemID)
		key := [2]uint32{r.ItemID, r.BookQuality}
		if !exists || b.BookSkillID == 0 || !found || !p.IsSkillPage || r.PagesPerBook <= 0 || len(r.AllowedSourceKinds) == 0 || recycle[key] {
			return errors.New("技能书唯一分解规则无效")
		}
		for _, kind := range r.AllowedSourceKinds {
			if kind != "monster_drop" {
				return errors.New("未审定来源禁止拆页")
			}
		}
		recycle[key] = true
	}
	return nil
}
