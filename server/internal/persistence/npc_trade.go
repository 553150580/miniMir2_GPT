package persistence

import (
	"math"
	"math/bits"
	"slices"

	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	p "minimir/internal/protocol"
)

const (
	vendorBuy = 1 + iota
	vendorSell
	vendorOrdinaryRepair
	vendorSpecialRepair
)

type vendorPlan struct {
	Action       uint32        `bson:"action"`
	OfferID      uint32        `bson:"offer_id,omitempty"`
	ItemID       uint32        `bson:"item_id"`
	Quantity     int64         `bson:"quantity"`
	ItemQuantity int64         `bson:"item_quantity"`
	Bound        bool          `bson:"bound"`
	Wallet       domain.Wallet `bson:"wallet"`
	Sold         *domain.Item  `bson:"sold,omitempty"`
	Outputs      []domain.Item `bson:"outputs,omitempty"`
	Repair       *RepairPlan   `bson:"repair,omitempty"`
}

func vendorCanPay(balance, cost domain.Wallet) bool {
	return balance.Gold >= cost.Gold && balance.BoundGold >= cost.BoundGold &&
		balance.Yuanbao >= cost.Yuanbao && balance.BoundYuanbao >= cost.BoundYuanbao
}

func buildVendorPlan(r domain.Role, f *devconfig.Fixture, shop *devconfig.NPCShop, q *pb.VendorQuoteRequest) (vendorPlan, uint32) {
	if shop == nil || q == nil || q.Quantity <= 0 {
		return vendorPlan{}, p.CodeVendorItemBlocked
	}
	switch q.Action {
	case vendorBuy:
		return buildVendorPurchase(r, f, shop, q)
	case vendorSell:
		return buildVendorSale(r, f, shop, q)
	case vendorOrdinaryRepair, vendorSpecialRepair:
		if !shop.CanRepair || q.OfferId != 0 || len(q.InstanceId) != 16 || q.Quantity != 1 {
			return vendorPlan{}, p.CodeVendorItemBlocked
		}
		option, ok := f.EquipmentDurability.Repair(q.Action - vendorSell)
		if !ok {
			return vendorPlan{}, p.CodeVendorItemBlocked
		}
		repair, code := BuildRepairPlan(r, f, option, domain.Hex(q.InstanceId), q.ItemVersion)
		if code != 0 {
			return vendorPlan{}, code
		}
		if !vendorCanPay(r.Wallet, repair.Spend) {
			return vendorPlan{}, p.CodeVendorFunds
		}
		return vendorPlan{Action: q.Action, ItemID: repair.Before.ItemID, Quantity: 1, ItemQuantity: 1,
			Bound: repair.Before.Bound, Wallet: repair.Spend, Repair: &repair}, 0
	default:
		return vendorPlan{}, p.CodeInvalidArgument
	}
}

func buildVendorPurchase(r domain.Role, f *devconfig.Fixture, shop *devconfig.NPCShop, q *pb.VendorQuoteRequest) (vendorPlan, uint32) {
	offer, exists := shop.Offer(q.OfferId)
	definition, known := f.Item(offer.ItemID)
	if !exists || !known || q.Quantity > offer.MaxPurchaseQuantity || r.Level < offer.RequiredRoleLevel || len(q.InstanceId) != 0 || q.ItemVersion != 0 {
		return vendorPlan{}, p.CodeVendorItemBlocked
	}
	quantity, quantityOK := multiply(q.Quantity, offer.Quantity)
	price, priceOK := multiply(q.Quantity, offer.PriceUnits)
	if !quantityOK || !priceOK || quantity <= 0 || price <= 0 {
		return vendorPlan{}, p.CodeVendorItemBlocked
	}
	plan := vendorPlan{Action: vendorBuy, OfferID: offer.ID, ItemID: offer.ItemID, Quantity: q.Quantity, ItemQuantity: quantity, Bound: offer.OutputBound}
	switch offer.Currency {
	case "gold":
		plan.Wallet.BoundGold = min(max(r.Wallet.BoundGold, 0), price)
		plan.Wallet.Gold = price - plan.Wallet.BoundGold
	case "ingot":
		plan.Wallet.Yuanbao = price
	case "bound_ingot":
		plan.Wallet.BoundYuanbao = price
	default:
		return vendorPlan{}, p.CodeInvalidArgument
	}
	if !vendorCanPay(r.Wallet, plan.Wallet) {
		return vendorPlan{}, p.CodeVendorFunds
	}
	// 绑定钱包实际支付的货物继续绑定，出售时不能变成可转移金币。
	plan.Bound = plan.Bound || plan.Wallet.BoundGold > 0 || plan.Wallet.BoundYuanbao > 0
	stack := int64(1)
	if definition.Stackable && definition.MaxStack > 1 {
		stack = definition.MaxStack
	}
	slots := (quantity-1)/stack + 1
	if slots > int64(r.Capacity-len(r.Items)) {
		return vendorPlan{}, p.CodeEquipmentBagFull
	}
	for quantity > 0 {
		item := domain.Item{ItemID: offer.ItemID, Quantity: min(quantity, stack), Bound: plan.Bound,
			Version: 1, SourceKind: "npc_shop", Quality: definition.DropQuality}
		plan.Outputs = append(plan.Outputs, InitializeItemDurability(item, f))
		quantity -= item.Quantity
	}
	return plan, 0
}

func buildVendorSale(r domain.Role, f *devconfig.Fixture, shop *devconfig.NPCShop, q *pb.VendorQuoteRequest) (vendorPlan, uint32) {
	if q.OfferId != 0 || len(q.InstanceId) != 16 || q.ItemVersion == 0 {
		return vendorPlan{}, p.CodeVendorItemBlocked
	}
	id := domain.Hex(q.InstanceId)
	for _, original := range r.Items {
		if original.ID != id {
			continue
		}
		item := InitializeItemDurability(original, f)
		unit := shop.SellPrice(item.ItemID)
		if unit <= 0 || item.Version <= 0 || item.Version == math.MaxInt64 || uint64(item.Version) != q.ItemVersion ||
			item.Quantity < q.Quantity || item.Locked || equipmentOccupied(item) {
			break
		}
		if item.OriginalDurabilityU != 0 {
			if !validItemDurability(item) {
				break
			}
			hi, lo := bits.Mul64(uint64(unit), uint64(item.DurabilityU))
			adjusted, _ := bits.Div64(hi, lo, uint64(item.OriginalDurabilityU))
			unit = int64(adjusted)
		}
		proceeds, ok := multiply(unit, q.Quantity)
		if !ok || proceeds <= 0 {
			break
		}
		plan := vendorPlan{Action: vendorSell, ItemID: item.ItemID, Quantity: q.Quantity, ItemQuantity: q.Quantity, Bound: item.Bound, Sold: &item}
		if item.Bound {
			plan.Wallet.BoundGold = proceeds
		} else {
			plan.Wallet.Gold = proceeds
		}
		if r.Wallet.Gold > math.MaxInt64-plan.Wallet.Gold || r.Wallet.BoundGold > math.MaxInt64-plan.Wallet.BoundGold {
			break
		}
		return plan, 0
	}
	return vendorPlan{}, p.CodeVendorItemBlocked
}

func applyVendorPlan(r domain.Role, plan vendorPlan, operationID string) (domain.Role, vendorPlan, uint32) {
	switch plan.Action {
	case vendorBuy:
		r.Items = slices.Clone(r.Items)
		for index := range plan.Outputs {
			item := &plan.Outputs[index]
			item.ID, item.SourceID = domain.ID(), operationID
			r.Items = append(r.Items, *item)
		}
		r.Wallet.BoundGold -= plan.Wallet.BoundGold
		r.Wallet.Gold -= plan.Wallet.Gold
		r.Wallet.BoundYuanbao -= plan.Wallet.BoundYuanbao
		r.Wallet.Yuanbao -= plan.Wallet.Yuanbao
	case vendorSell:
		r.Items = slices.Clone(r.Items)
		for index, item := range r.Items {
			if item.ID == plan.Sold.ID {
				item.Quantity -= plan.Quantity
				item.Version++
				if item.Quantity == 0 {
					r.Items = slices.Delete(r.Items, index, index+1)
				} else {
					r.Items[index] = item
				}
				break
			}
		}
		r.Wallet.BoundGold += plan.Wallet.BoundGold
		r.Wallet.Gold += plan.Wallet.Gold
	case vendorOrdinaryRepair, vendorSpecialRepair:
		out, code := ApplyRepairPlan(r, *plan.Repair)
		return out, plan, code
	}
	return r, plan, 0
}
