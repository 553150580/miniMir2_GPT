// NPC 商品购买和物品出售的资产计划。校验库存、货币种类、实例身份和数量，执行时更新钱包、背包与来源。

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
	// vendorBuy 保存根据当前数据与“iota”计算的商人“buy”。
	vendorBuy = 1 + iota
	// vendorSell 记录商人“sell”。
	vendorSell
	// vendorOrdinaryRepair 记录商人“ordinary”修理。
	vendorOrdinaryRepair
	// vendorSpecialRepair 记录商人特殊修理。
	vendorSpecialRepair
)

// vendorPlan 固定买卖或修理对象、数量、钱包变更和对应资产变化。
type vendorPlan struct {
	// Action 记录动作，使用数值表示。
	Action uint32 `bson:"action"`
	// OfferID 标识当前业务使用的“offer”。
	OfferID uint32 `bson:"offer_id,omitempty"`
	// ItemID 标识当前业务使用的道具。
	ItemID uint32 `bson:"item_id"`
	// Quantity 记录数量，使用数值表示。
	Quantity int64 `bson:"quantity"`
	// ItemQuantity 记录道具数量，使用数值表示。
	ItemQuantity int64 `bson:"item_quantity"`
	// Bound 标记是否处于绑定状态。
	Bound bool `bson:"bound"`
	// Wallet 记录钱包。
	Wallet domain.Wallet `bson:"wallet"`
	// Sold 记录“sold”，使用道具表示。
	Sold *domain.Item `bson:"sold,omitempty"`
	// Outputs 记录“outputs”，使用道具列表表示。
	Outputs []domain.Item `bson:"outputs,omitempty"`
	// Repair 记录修理，使用修理计划表示。
	Repair *RepairPlan `bson:"repair,omitempty"`
}

// vendorCanPay 比较各类绑定及普通货币余额，只有每类余额均充足时才允许支付。
// balance 记录余额，使用钱包表示，cost 记录消耗，使用钱包表示。
func vendorCanPay(balance, cost domain.Wallet) bool {
	return balance.Gold >= cost.Gold && balance.BoundGold >= cost.BoundGold &&
		balance.Yuanbao >= cost.Yuanbao && balance.BoundYuanbao >= cost.BoundYuanbao
}

// buildVendorPlan 校验玩法配置和报价动作，将请求分派到购买、出售或修理计划。
// r 记录角色，f 记录开发测试配置，shop 记录商店，使用NPC商店表示，q 记录商人报价请求。
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
		// option 保存由修理取得的选项，ok 表明查询、校验或类型转换是否成功。
		option, ok := f.EquipmentDurability.Repair(q.Action - vendorSell)
		if !ok {
			return vendorPlan{}, p.CodeVendorItemBlocked
		}
		// repair 保存由构建修理计划取得的修理，code 保存由构建修理计划取得的结果码。
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

// buildVendorPurchase 由商品报价和数量计算需要支付的各类货币及要发放的物品。
// r 记录角色，f 记录开发测试配置，shop 记录商店，使用NPC商店表示，q 记录商人报价请求。
func buildVendorPurchase(r domain.Role, f *devconfig.Fixture, shop *devconfig.NPCShop, q *pb.VendorQuoteRequest) (vendorPlan, uint32) {
	// offer 接收尝试提交供后续步骤使用，exists 保存由尝试提交取得的存在。
	offer, exists := shop.Offer(q.OfferId)
	// definition 保存由道具取得的定义，known 保存由道具取得的已知状态。
	definition, known := f.Item(offer.ItemID)
	if !exists || !known || q.Quantity > offer.MaxPurchaseQuantity || r.Level < offer.RequiredRoleLevel || len(q.InstanceId) != 0 || q.ItemVersion != 0 {
		return vendorPlan{}, p.CodeVendorItemBlocked
	}
	// 报价与最终提交共用规则，不能通过手工指定商品号购买其他职业专用物品。
	if len(definition.AllowedJobs) != 0 && !slices.Contains(definition.AllowedJobs, r.Job) {
		return vendorPlan{}, p.CodeVendorItemBlocked
	}
	// quantity 保存由相乘取得的数量，quantityOK 保存由相乘取得的数量成功标记。
	quantity, quantityOK := multiply(q.Quantity, offer.Quantity)
	// price 保存由相乘取得的“price”，priceOK 保存由相乘取得的“price”成功标记。
	price, priceOK := multiply(q.Quantity, offer.PriceUnits)
	if !quantityOK || !priceOK || quantity <= 0 || price <= 0 {
		return vendorPlan{}, p.CodeVendorItemBlocked
	}
	// plan 初始化用作计划的商人计划。
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
	// stack 保存由整数“64”取得的堆叠。
	stack := int64(1)
	if definition.Stackable && definition.MaxStack > 1 {
		stack = definition.MaxStack
	}
	// slots 保存根据数量与当前数据的计算结果与堆叠的计算结果与当前数据计算的槽位集合。
	slots := (quantity-1)/stack + 1
	if slots > int64(r.Capacity-len(r.Items)) {
		return vendorPlan{}, p.CodeEquipmentBagFull
	}
	for quantity > 0 {
		// item 组装道具所需的数据。
		item := domain.Item{ItemID: offer.ItemID, Quantity: min(quantity, stack), Bound: plan.Bound,
			Version: 1, SourceKind: "npc_shop", Quality: definition.DropQuality}
		plan.Outputs = append(plan.Outputs, InitializeItemDurability(item, f))
		quantity -= item.Quantity
	}
	return plan, 0
}

// buildVendorSale 按玩家选中的原库存实例、数量与商店收购表计算收入。
// r 记录角色，f 记录开发测试配置，shop 记录商店，使用NPC商店表示，q 记录商人报价请求。
func buildVendorSale(r domain.Role, f *devconfig.Fixture, shop *devconfig.NPCShop, q *pb.VendorQuoteRequest) (vendorPlan, uint32) {
	if q.OfferId != 0 || len(q.InstanceId) != 16 || q.ItemVersion == 0 {
		return vendorPlan{}, p.CodeVendorItemBlocked
	}
	// id 区分当前步骤指向的目标对象。
	id := domain.Hex(q.InstanceId)
	// original 接收遍历道具集合时当前的元素。
	for _, original := range r.Items {
		if original.ID != id {
			continue
		}
		// item 保存由“initialize”道具耐久取得的道具。
		item := InitializeItemDurability(original, f)
		if item.Version <= 0 || item.Version == math.MaxInt64 || uint64(item.Version) != q.ItemVersion ||
			item.Quantity < q.Quantity || item.Locked || equipmentOccupied(item) {
			break
		}
		// unit 是本店按原实例计算的每件收购价，矿石已依单块纯度结算。
		unit := vendorSaleUnitPrice(f, shop, item)
		if unit <= 0 {
			break
		}
		if item.OriginalDurabilityU != 0 {
			if !validItemDurability(item) {
				break
			}
			// hi 保存由“mul”“64”取得的“hi”，lo 保存由“mul”“64”取得的“lo”。
			hi, lo := bits.Mul64(uint64(unit), uint64(item.DurabilityU))
			// adjusted 保存由“div”“64”取得的“adjusted”。
			adjusted, _ := bits.Div64(hi, lo, uint64(item.OriginalDurabilityU))
			unit = int64(adjusted)
		}
		// proceeds 保存由相乘取得的“proceeds”，ok 表明查询、校验或类型转换是否成功。
		proceeds, ok := multiply(unit, q.Quantity)
		if !ok || proceeds <= 0 {
			break
		}
		// plan 初始化用作计划的商人计划。
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

// applyVendorPlan 按已复核报价更新钱包、保留绑定关系并修改原库存实例。
// r 记录角色，plan 记录计划，使用商人计划表示，operationID 标识当前业务使用的操作。
func applyVendorPlan(r domain.Role, plan vendorPlan, operationID string) (domain.Role, vendorPlan, uint32) {
	switch plan.Action {
	case vendorBuy:
		r.Items = slices.Clone(r.Items)
		// index 标记遍历“outputs”时访问的下标。
		for index := range plan.Outputs {
			// item 保存当前处理所用的道具。
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
		// index 定位道具集合中正在处理的成员，item 接收遍历道具集合时当前的元素。
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
		// out 保存由应用修理计划取得的输出，code 保存由应用修理计划取得的结果码。
		out, code := ApplyRepairPlan(r, *plan.Repair)
		return out, plan, code
	}
	return r, plan, 0
}
