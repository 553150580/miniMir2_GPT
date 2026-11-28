// 本文件为角色商城生成原报价，在角色资产事务内同时扣款、发货并确认日限量。
package persistence

import (
	"cmp"
	"fmt"
	"math"
	"slices"
	"time"

	"google.golang.org/protobuf/proto"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	p "minimir/internal/protocol"
)

// AssetGrowthShopCommit 对应沿原经济流水购买一次商城绑定材料。
const AssetGrowthShopCommit = "growth_shop_commit"

// growthShopTerms 记录玩家确认的价格、份数、限额以及原日界。
type growthShopTerms struct {
	ProductID   uint32 `bson:"product_id"`          // 当前币种对应的原商品。
	ItemID      uint32 `bson:"item_id"`             // 实际到账的原材料。
	Currency    uint32 `bson:"currency"`            // 一次只使用所选币种。
	UnitPrice   int64  `bson:"unit_price_units"`    // 每份元宝最小单位。
	Count       uint32 `bson:"count"`               // 玩家确认的份数。
	Quantity    int64  `bson:"item_quantity"`       // 这次到账的材料个数。
	Total       int64  `bson:"total_price_units"`   // 实际扣除的全部最小单位。
	Day         string `bson:"operating_day"`       // 原报价所在上海日。
	DailyLimit  uint32 `bson:"daily_limit"`         // 该材料跨币种共享上限。
	DailyBought uint32 `bson:"daily_bought_before"` // 生成报价前本角色的原水位。
}

// GrowthShopQuote 是角色分片签发的短期确认条款，只有原场景和现行规则可首次使用。
type GrowthShopQuote struct {
	ID        string          `bson:"quote_id"`         // 原随机报价编号。
	RoleID    string          `bson:"role_id"`          // 扣款与到账的永久角色。
	Origin    uint64          `bson:"origin_server_id"` // 该角色的永久入口服。
	Scene     int64           `bson:"scene_epoch"`      // 原场景编号的有符号位模式。
	Revision  int64           `bson:"role_revision"`    // 报价时的角色资产修订。
	Rule      uint64          `bson:"rule_version"`     // 原报价使用的规则版本。
	Config    string          `bson:"config_hash"`      // 原配置全文摘要。
	IssuedAt  time.Time       `bson:"issued_at"`        // 报价实际生成时刻。
	ExpiresAt time.Time       `bson:"expires_at"`       // 首次确认的有效期上界。
	Terms     growthShopTerms `bson:"terms"`            // 原费用和发货条款。
}

// GrowthShopReceipt 包含不可变的首次成交条款和发出的原背包实例。
type GrowthShopReceipt struct {
	Quote    GrowthShopQuote `bson:"quote"`         // 原配置、商品、金额和限量事实。
	Items    []domain.Item   `bson:"items"`         // 首次成交时创建的实际绑定物品。
	Balance  int64           `bson:"balance_units"` // 原成交后的所选钱包余额。
	BoughtAt time.Time       `bson:"bought_at"`     // 数据库内确认成交的时刻。
}

// Proto 投影原报价的确切金额和购买前限量。
func (t growthShopTerms) Proto() *pb.GrowthShopTerms {
	return &pb.GrowthShopTerms{ProductId: t.ProductID, ItemId: t.ItemID, Currency: t.Currency, UnitPriceUnits: t.UnitPrice,
		Count: t.Count, ItemQuantity: t.Quantity, TotalPriceUnits: t.Total, OperatingDay: t.Day, DailyLimit: t.DailyLimit, DailyBoughtBefore: t.DailyBought}
}

// Proto 从房间持有的条款生成确认页。
func (q GrowthShopQuote) Proto() *pb.GrowthShopQuote {
	return &pb.GrowthShopQuote{QuoteId: domain.Bytes(q.ID), ExpiresAtUnixMs: q.ExpiresAt.UnixMilli(), Terms: q.Terms.Proto()}
}

// Proto 用原成交事实还原回执，重连时不取当前余额或原物品当前数量。
func (r *GrowthShopReceipt) Proto() *pb.GrowthShopReceipt {
	if r == nil {
		return nil
	}
	out := &pb.GrowthShopReceipt{QuoteId: domain.Bytes(r.Quote.ID), Terms: r.Quote.Terms.Proto(), BalanceUnits: r.Balance}
	for _, item := range r.Items {
		out.Items = append(out.Items, item.Proto())
	}
	return out
}

// ValidGrowthShopCommit 校验原操作身份和形状；价格和币种只从可信报价取得。
func ValidGrowthShopCommit(q *pb.GrowthShopCommitRequest, op *pb.OperationContext) bool {
	return q != nil && op != nil && len(q.QuoteId) == 16 && q.ProductId != 0 && q.Count > 0 && q.Count <= devconfig.GrowthShopMaxCount &&
		len(op.OpId) == 16 && len(q.ProtoReflect().GetUnknown()) == 0 && len(op.ProtoReflect().GetUnknown()) == 0 && proto.Equal(q.Operation, op)
}

// shopDayFor 校验现有水位，跨日时只读出一个空的新日且保留原历史回执。
func shopDayFor(role domain.Role, day string) (domain.GrowthShopDay, bool) {
	state := role.GrowthShop
	if state == nil || state.Day < day {
		return domain.GrowthShopDay{Day: day}, true
	}
	if state.Day != day || len(state.Counts) > 16 {
		return domain.GrowthShopDay{}, false
	}
	var previous uint32
	for _, count := range state.Counts {
		if count.ItemID <= previous || count.Bought == 0 {
			return domain.GrowthShopDay{}, false
		}
		previous = count.ItemID
	}
	return *state, true
}

// shopMaterialIndex 查找一个材料的位置，未购买时返回保持升序的插入点。
func shopMaterialIndex(day domain.GrowthShopDay, itemID uint32) (int, bool) {
	return slices.BinarySearchFunc(day.Counts, itemID, func(row domain.GrowthShopCount, id uint32) int { return cmp.Compare(row.ItemID, id) })
}

// GrowthShopCatalog 从当前角色读取可观察目录和当日限购；这里不发货和扣款。
func GrowthShopCatalog(role domain.Role, f *devconfig.Fixture, scene uint64, at time.Time) (*pb.GrowthShopState, uint32) {
	if f.GrowthShop == nil {
		return nil, p.CodeGrowthShopUnavailable
	}
	today, valid := shopDayFor(role, shanghaiOperatingDay(at))
	if !valid {
		return nil, p.CodeOperationConflict
	}
	state := &pb.GrowthShopState{SceneEpoch: scene, RoleRevision: uint64(role.Revision), RuleVersion: f.RuleVersion, OperatingDay: today.Day}
	for _, offer := range f.GrowthShop.Offers {
		definition, known := f.Item(offer.ItemID)
		if !known || len(definition.AllowedJobs) != 0 && !slices.Contains(definition.AllowedJobs, role.Job) {
			continue
		}
		view := &pb.GrowthShopOfferView{ProductId: offer.ID, ItemId: offer.ItemID, Name: offer.Name, Currency: offer.Currency,
			PriceUnits: offer.PriceUnits, ItemPerPack: offer.ItemPerPack, MaxCount: offer.MaxCount, DailyLimit: offer.DailyLimit}
		if index, exists := shopMaterialIndex(today, offer.ItemID); exists {
			view.DailyBought = today.Counts[index].Bought
		}
		state.Offers = append(state.Offers, view)
	}
	return state, 0
}

// shopTerms 复用同一套报价条件，检查原 SKU、日限量、明确钱包及实际需要的背包格。
func shopTerms(role domain.Role, f *devconfig.Fixture, today domain.GrowthShopDay, product, count uint32) (growthShopTerms, devconfig.Item, uint32) {
	offer, known := f.GrowthShop.Offer(product)
	if !known || offer.Currency == 0 {
		return growthShopTerms{}, devconfig.Item{}, p.CodeGrowthShopUnavailable
	}
	if count == 0 || count > offer.MaxCount {
		return growthShopTerms{}, devconfig.Item{}, p.CodeInvalidArgument
	}
	definition, found := f.Item(offer.ItemID)
	quantity, quantityOK := multiply(int64(count), int64(offer.ItemPerPack))
	total, totalOK := multiply(int64(count), offer.PriceUnits)
	if !found || definition.MaxStack <= 0 || len(definition.AllowedJobs) != 0 && !slices.Contains(definition.AllowedJobs, role.Job) ||
		!quantityOK || quantity <= 0 || !totalOK || total <= 0 {
		return growthShopTerms{}, devconfig.Item{}, p.CodeGrowthShopUnavailable
	}
	terms := growthShopTerms{ProductID: product, ItemID: offer.ItemID, Currency: offer.Currency, UnitPrice: offer.PriceUnits,
		Count: count, Quantity: quantity, Total: total, Day: today.Day, DailyLimit: offer.DailyLimit}
	if index, exists := shopMaterialIndex(today, offer.ItemID); exists {
		terms.DailyBought = today.Counts[index].Bought
	} else if len(today.Counts) == 16 {
		return terms, definition, p.CodeGrowthShopLimit
	}
	after := uint64(terms.DailyBought) + uint64(quantity)
	if after > math.MaxUint32 || offer.DailyLimit != 0 && after > uint64(offer.DailyLimit) {
		return terms, definition, p.CodeGrowthShopLimit
	}
	balance := role.Wallet.Yuanbao
	if offer.Currency == devconfig.GrowthShopBoundYuanbao {
		balance = role.Wallet.BoundYuanbao
	}
	if balance < total {
		return terms, definition, p.CodeVendorFunds
	}
	if (quantity-1)/definition.MaxStack+1 > int64(role.Capacity-len(role.Items)) {
		return terms, definition, p.CodeEquipmentBagFull
	}
	return terms, definition, 0
}

// QuoteGrowthShop 固定原条款，报价的三十秒窗口不能跨过上海日界。
func QuoteGrowthShop(role domain.Role, f *devconfig.Fixture, scene uint64, at time.Time, product, count uint32) (GrowthShopQuote, uint32) {
	today, valid := shopDayFor(role, shanghaiOperatingDay(at))
	if !valid {
		return GrowthShopQuote{}, p.CodeOperationConflict
	}
	terms, _, code := shopTerms(role, f, today, product, count)
	if code != 0 {
		return GrowthShopQuote{}, code
	}
	local := at.In(shanghaiOperatingZone)
	midnight := time.Date(local.Year(), local.Month(), local.Day()+1, 0, 0, 0, 0, shanghaiOperatingZone)
	expires := at.Add(30 * time.Second)
	if midnight.Before(expires) {
		expires = midnight
	}
	return GrowthShopQuote{ID: domain.ID(), RoleID: role.ID, Origin: role.Origin, Scene: int64(scene), Revision: role.Revision,
		Rule: f.RuleVersion, Config: f.Hash, IssuedAt: bossTime(at), ExpiresAt: bossTime(expires), Terms: terms}, 0
}

// commitGrowthShop 核对可信报价和现行原子角色状态，失败不扣款、不发货、不消费日额度。
func commitGrowthShop(role domain.Role, f *devconfig.Fixture, req AssetRequest, at time.Time) (domain.Role, *GrowthShopReceipt, uint32) {
	if f.GrowthShop == nil || req.GrowthShopGate {
		return role, nil, p.CodeGrowthShopUnavailable
	}
	input, quote := req.GrowthShopCommit, req.GrowthShopQuote
	day := shanghaiOperatingDay(at)
	if !ValidGrowthShopCommit(input, req.Context) || quote == nil || quote.ID != domain.Hex(input.QuoteId) ||
		quote.RoleID != role.ID || quote.Origin != role.Origin || uint64(quote.Scene) != req.SceneEpoch || req.SceneEpoch == 0 ||
		quote.Revision <= 0 || quote.Revision > role.Revision || quote.Config != f.Hash || quote.Rule != f.RuleVersion ||
		at.Before(quote.IssuedAt) || !at.Before(quote.ExpiresAt) || day != quote.Terms.Day || input.ProductId != quote.Terms.ProductID || input.Count != quote.Terms.Count {
		return role, nil, p.CodeGrowthShopQuoteChanged
	}
	today, valid := shopDayFor(role, day)
	if !valid {
		return role, nil, p.CodeOperationConflict
	}
	terms, definition, code := shopTerms(role, f, today, input.ProductId, input.Count)
	if code != 0 {
		return role, nil, code
	}
	if terms != quote.Terms {
		return role, nil, p.CodeGrowthShopQuoteChanged
	}
	opID := domain.Hex(req.Context.OpId)
	var items []domain.Item // 每堆含原操作来源，和其他来源的材料独立。
	for remaining := terms.Quantity; remaining > 0; {
		quantity := min(remaining, definition.MaxStack)
		id := Hash(fmt.Appendf(nil, "growth_shop_item:%s:%s:%d", role.ID, opID, len(items)))[:32]
		if role.HasItemInstance(id) {
			return role, nil, p.CodeOperationConflict
		}
		items = append(items, domain.Item{ID: id, ItemID: definition.ID, Quantity: quantity, Bound: true, Version: 1,
			SourceKind: "growth_shop", SourceID: opID, Quality: definition.DropQuality})
		remaining -= quantity
	}
	index, exists := shopMaterialIndex(today, terms.ItemID)
	today.Counts = slices.Clone(today.Counts)
	if exists {
		today.Counts[index].Bought += uint32(terms.Quantity)
	} else {
		today.Counts = slices.Insert(today.Counts, index, domain.GrowthShopCount{ItemID: terms.ItemID, Bought: uint32(terms.Quantity)})
	}
	role.GrowthShop = &today
	role.Items = append(slices.Clone(role.Items), items...)
	balance := &role.Wallet.Yuanbao
	if terms.Currency == devconfig.GrowthShopBoundYuanbao {
		balance = &role.Wallet.BoundYuanbao
	}
	*balance -= terms.Total
	return role, &GrowthShopReceipt{Quote: *quote, Items: items, Balance: *balance, BoughtAt: bossTime(at)}, 0
}
