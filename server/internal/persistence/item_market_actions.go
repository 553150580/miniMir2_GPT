// 本文件处理角色资产所有者受理的道具市场操作。
package persistence

import (
	"context"
	"errors"
	"math"
	"slices"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	p "minimir/internal/protocol"
)

const AssetItemMarket = "item_market" // 按角色经济序号确认的一笔道具市场操作。

const (
	itemMarketList uint32 = iota + 1
	itemMarketBid
	itemMarketRaise
	itemMarketBuy
	itemMarketCancel
	itemMarketClaim
)

// ItemMarketReceipt 固定首次确认的结果，已成交的原回执先于当前条件返回。
type ItemMarketReceipt struct {
	Action        uint32 `bson:"action"`
	OrderID       string `bson:"order_id"`
	OrderVersion  int64  `bson:"order_version"`
	ItemID        uint32 `bson:"item_id,omitempty"`
	Quantity      int64  `bson:"quantity,omitempty"`
	UnitPrice     int64  `bson:"unit_price,omitempty"`
	YuanbaoDelta  int64  `bson:"yuanbao_delta,omitempty"`
	Tax           int64  `bson:"tax,omitempty"`
	EscrowID      string `bson:"escrow_id,omitempty"`
	EscrowVersion int64  `bson:"escrow_version,omitempty"`
}

func (r *ItemMarketReceipt) Proto() *pb.ItemMarketReceipt {
	if r == nil {
		return nil
	}
	return &pb.ItemMarketReceipt{
		Action: r.Action, OrderId: domain.Bytes(r.OrderID), OrderVersion: uint64(r.OrderVersion),
		ItemId: r.ItemID, Quantity: r.Quantity, UnitPriceYuanbaoUnits: r.UnitPrice,
		YuanbaoDeltaUnits: r.YuanbaoDelta, TaxYuanbaoUnits: r.Tax,
		EscrowId: domain.Bytes(r.EscrowID), EscrowVersion: uint64(r.EscrowVersion),
	}
}

// validItemMarketAction 拒绝在动作之外夹带数量、身份或报价字段。
func validItemMarketAction(q *pb.ItemMarketAction) bool {
	if q == nil || q.Operation == nil || len(q.Operation.OpId) != 16 || len(q.ProtoReflect().GetUnknown()) != 0 || len(q.Operation.ProtoReflect().GetUnknown()) != 0 {
		return false
	}
	emptyOrder := len(q.OrderId) == 0 && q.OrderVersion == 0
	emptyItem := len(q.ItemInstanceId) == 0 && q.ItemVersion == 0
	emptyEscrow := len(q.EscrowId) == 0 && q.EscrowVersion == 0
	switch q.Action {
	case itemMarketList:
		return emptyOrder && emptyEscrow && len(q.ItemInstanceId) == 16 && q.ItemVersion > 0 && q.ItemVersion < math.MaxInt64 &&
			q.Quantity > 0 && q.UnitPriceYuanbaoUnits > 0
	case itemMarketBid, itemMarketRaise, itemMarketBuy, itemMarketCancel:
		return emptyItem && emptyEscrow && len(q.OrderId) == 16 && q.OrderVersion > 0 && q.OrderVersion < math.MaxInt64 &&
			q.Quantity > 0 && q.UnitPriceYuanbaoUnits > 0
	case itemMarketClaim:
		return emptyOrder && emptyItem && len(q.EscrowId) == 16 && q.EscrowVersion > 0 &&
			q.EscrowVersion < math.MaxInt64 && q.UnitPriceYuanbaoUnits == 0 && q.Quantity >= 0
	}
	return false
}

// CanTrade 核对道具来源、绑定、占用状态和当前已确认的装备交易品质。
func CanTrade(item domain.Item, definition devconfig.Item) bool {
	if item.ItemID == 0 || item.ItemID != definition.ID || item.Bound || item.Locked || equipmentOccupied(item) ||
		item.SourceID == "" || item.Version < 1 || item.Version == math.MaxInt64 || !lowerHex(item.ID, 16) ||
		item.Quantity <= 0 || item.Quality > 4 {
		return false
	}
	if definition.IsEquipment && item.Quality < 2 {
		return false
	}
	if item.SourceKind == "mining" {
		// 保留单块的原纯度。非绑定矿石可申请上架，任意纯度都适用同一交易规则。
		return validSingleMiningOre(item, definition)
	}
	if item.OrePurity != 0 {
		return false
	}
	switch item.SourceKind {
	case "monster_drop", "personal_boss", "world_boss", "world_boss_timeout", "dungeon", "npc_shop",
		"skill_recycle", "task_reward", "codex_reward":
		return item.Quantity == 1 || definition.Stackable && item.Quantity <= definition.MaxStack
	}
	return false
}

func marketTotal(unitPrice, quantity int64) (int64, bool) {
	if unitPrice <= 0 || quantity <= 0 || unitPrice > math.MaxInt64/quantity {
		return 0, false
	}
	return unitPrice * quantity, true
}

// marketNextPrice 使用原定点单位计算加价，余数向上对齐。
func marketNextPrice(unitPrice int64, rules *devconfig.ItemMarketConfig) (int64, bool) {
	if rules == nil || unitPrice <= 0 {
		return 0, false
	}
	const basis = int64(10_000)
	bp, step := int64(rules.BidRaiseBP), rules.PriceStepYuanbaoUnits
	if bp <= 0 || bp > basis || step <= 0 {
		return 0, false
	}
	increase := unitPrice/basis*bp + (unitPrice%basis*bp+basis-1)/basis
	next, err := add(unitPrice, max(increase, rules.MinimumRaiseYuanbaoUnits))
	if err != nil {
		return 0, false
	}
	if remainder := next % step; remainder != 0 {
		next, err = add(next, step-remainder)
	}
	return next, err == nil && next > unitPrice
}

func (s *Store) applyItemMarket(ctx context.Context, role domain.Role, request *pb.ItemMarketAction, gate uint32) (domain.Role, *ItemMarketReceipt, uint32, error) {
	if !validItemMarketAction(request) || !equipmentRoleUnique(role) {
		return role, nil, p.CodeInvalidArgument, nil
	}
	if gate != 0 {
		return role, nil, gate, nil
	}
	if err := s.requireOpenMarkets(ctx); err != nil {
		return role, nil, 0, err
	}
	now := bossTime(time.Now()) // 业务实际受理时以统一毫秒时钟检查截止。
	if request.Action == itemMarketClaim {
		return s.claimItemMarketAsset(ctx, role, request, now)
	}
	rules := s.Cfg.Fixture.ItemMarket
	if request.Action != itemMarketCancel && (rules == nil || !role.PrivilegeActiveAt(now)) {
		return role, nil, p.CodeLineForbidden, nil
	}
	if request.Action == itemMarketList {
		return s.listItemMarketItem(ctx, role, request, rules, now)
	}
	var order ItemMarketOrder
	err := s.DB.Collection(itemMarketOrders).FindOne(ctx, bson.M{
		"_id": domain.Hex(request.OrderId), "run_server_id": s.Cfg.RunID,
	}).Decode(&order)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return role, nil, p.CodeOperationConflict, nil
	}
	if err != nil {
		return role, nil, 0, err
	}
	if order.Revision != int64(request.OrderVersion) || order.UnitPrice != request.UnitPriceYuanbaoUnits ||
		order.Item.Quantity < request.Quantity || request.Action != itemMarketBuy && order.Item.Quantity != request.Quantity ||
		!order.activeAt(now) {
		return role, nil, p.CodeOperationConflict, nil
	}
	if request.Action == itemMarketCancel {
		return s.cancelItemMarketOrder(ctx, role, request, order, now)
	}
	if order.Seller.AccountID == role.AccountID {
		return role, nil, p.CodeSelfTradeForbidden, nil
	}
	if request.Action == itemMarketBuy {
		return s.buyItemMarketOrder(ctx, role, request, order, now)
	}
	return s.bidItemMarketOrder(ctx, role, request, order, rules, now)
}

func (o ItemMarketOrder) activeAt(at time.Time) bool {
	return o.Phase == ItemMarketAuction && at.Before(o.AuctionEndsAt) ||
		o.Phase == ItemMarketDirect && at.Before(o.DirectEndsAt)
}

func (s *Store) listItemMarketItem(ctx context.Context, role domain.Role, request *pb.ItemMarketAction, rules *devconfig.ItemMarketConfig, now time.Time) (domain.Role, *ItemMarketReceipt, uint32, error) {
	itemID := domain.Hex(request.ItemInstanceId)
	slot := slices.IndexFunc(role.Items, func(item domain.Item) bool { return item.ID == itemID })
	if slot < 0 || uint64(role.Items[slot].Version) != request.ItemVersion {
		return role, nil, p.CodeOperationConflict, nil
	}
	original := role.Items[slot]
	definition, known := s.Cfg.Fixture.Item(original.ItemID)
	if !known || !CanTrade(original, definition) {
		return role, nil, p.CodeVendorItemBlocked, nil
	}
	// 整件转入托管时保障后续拆卖、返还或交付仍有足够的实例版本。
	if request.Quantity == original.Quantity && original.Version >= math.MaxInt64-request.Quantity {
		return role, nil, p.CodeVendorItemBlocked, nil
	}
	if _, valid := marketTotal(request.UnitPriceYuanbaoUnits, request.Quantity); !valid ||
		request.Quantity > original.Quantity || request.UnitPriceYuanbaoUnits%rules.PriceStepYuanbaoUnits != 0 {
		return role, nil, p.CodeInvalidArgument, nil
	}
	active, err := s.DB.Collection(itemMarketOrders).CountDocuments(ctx, bson.M{
		"run_server_id": s.Cfg.RunID, "seller.account_id": role.AccountID,
		"phase": bson.M{"$in": []uint32{ItemMarketAuction, ItemMarketDirect}},
	}, options.Count().SetLimit(int64(rules.MaxListingsPerAccountRun)))
	if err != nil {
		return role, nil, 0, err
	}
	if active >= int64(rules.MaxListingsPerAccountRun) {
		return role, nil, p.CodeOperationConflict, nil
	}
	op := domain.Hex(request.Operation.OpId)
	transferred, remaining, valid := warehouseSplit(original, request.Quantity, marketFactID("market-list-item", role.ID, op))
	if !valid {
		return role, nil, p.CodeOperationConflict, nil
	}
	order := ItemMarketOrder{
		ID: marketFactID("market-order", role.ID, op), RunID: s.Cfg.RunID, Seller: marketParty(role),
		Item: transferred, UnitPrice: request.UnitPriceYuanbaoUnits, Revision: 1, Round: 1, Phase: ItemMarketAuction,
		ListedAt: now, AuctionEndsAt: now.Add(time.Duration(rules.AuctionSeconds) * time.Second),
		BuyoutDuration: time.Duration(rules.BuyoutSeconds) * time.Second, TaxDivisor: rules.TaxDivisor,
		RuleVersion: s.Cfg.Fixture.RuleVersion, ConfigHash: s.Cfg.Fixture.Hash,
	}
	if _, err = s.DB.Collection(itemMarketOrders).InsertOne(ctx, order); err != nil {
		return role, nil, 0, err
	}
	role.Items = slices.Clone(role.Items)
	if remaining == nil {
		role.Items = slices.Delete(role.Items, slot, slot+1)
	} else {
		role.Items[slot] = *remaining
	}
	return role, order.receipt(request.Action, transferred.Quantity), 0, nil
}

func (o ItemMarketOrder) receipt(action uint32, quantity int64) *ItemMarketReceipt {
	return &ItemMarketReceipt{Action: action, OrderID: o.ID, OrderVersion: o.Revision,
		ItemID: o.Item.ItemID, Quantity: quantity, UnitPrice: o.UnitPrice}
}
