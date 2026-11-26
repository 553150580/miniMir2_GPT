// 本文件在原角色事务内确认预购、加价、直购、撤单和待领资产。
package persistence

import (
	"context"
	"errors"
	"math"
	"slices"
	"strconv"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	p "minimir/internal/protocol"
)

func (s *Store) bidItemMarketOrder(ctx context.Context, role domain.Role, req *pb.ItemMarketAction, order ItemMarketOrder, rules *devconfig.ItemMarketConfig, now time.Time) (domain.Role, *ItemMarketReceipt, uint32, error) {
	if order.Phase != ItemMarketAuction || order.ActiveBidCount < 0 || order.ActiveBidCount >= math.MaxUint32 ||
		order.Round < 1 || order.Revision >= math.MaxInt64 || order.WinnerBidID != "" {
		return role, nil, p.CodeOperationConflict, nil
	}
	gross, valid := marketTotal(order.UnitPrice, order.Item.Quantity)
	if !valid {
		return role, nil, p.CodeOperationConflict, nil
	}
	var existing ItemMarketBid
	err := s.DB.Collection(itemMarketBids).FindOne(ctx, bson.M{
		"order_id": order.ID, "round": order.Round, "buyer.account_id": role.AccountID,
	}).Decode(&existing)
	if err != nil && !errors.Is(err, mongo.ErrNoDocuments) {
		return role, nil, 0, err
	}
	if req.Action == itemMarketBid && err == nil {
		return role, nil, p.CodeOperationConflict, nil
	}

	next := order
	next.Revision++
	debit := gross
	if req.Action == itemMarketRaise {
		if err != nil || existing.Buyer != marketParty(role) || existing.State != itemBidFrozen ||
			existing.Amount != gross || order.ActiveBidCount == 0 || order.Round >= math.MaxInt64 {
			return role, nil, p.CodeOperationConflict, nil
		}
		price, canRaise := marketNextPrice(order.UnitPrice, rules)
		newGross, canFreeze := marketTotal(price, order.Item.Quantity)
		if !canRaise || !canFreeze {
			return role, nil, p.CodeInvalidArgument, nil
		}
		debit, gross = newGross-gross, newGross
		next.UnitPrice, next.Round, next.ActiveBidCount = price, order.Round+1, 0
		if order.AuctionEndsAt.Sub(now) < time.Duration(rules.ExtendWhenRemainingBelowSeconds)*time.Second {
			next.AuctionEndsAt = now.Add(time.Duration(rules.ExtendToRemainingSeconds) * time.Second)
		}
	}
	if role.Wallet.Yuanbao < debit {
		return role, nil, p.CodeVendorFunds, nil
	}
	next.ActiveBidCount++
	bid := ItemMarketBid{
		ID: marketFactID("market-bid", order.ID, strconv.FormatInt(next.Round, 10), role.AccountID), OrderID: order.ID,
		RunID: order.RunID, Buyer: marketParty(role), Round: next.Round, Ordinal: next.ActiveBidCount,
		Amount: gross, State: itemBidFrozen, CreatedAt: now,
	}
	if err = s.replaceItemMarketOrder(ctx, next, order.Revision); err != nil {
		return role, nil, 0, err
	}
	if req.Action == itemMarketRaise {
		patch := bson.M{"$set": bson.M{"state": itemBidCarried, "ended_at": now}}
		change, err := s.DB.Collection(itemMarketBids).UpdateOne(ctx, bson.M{"_id": existing.ID, "state": itemBidFrozen}, patch)
		if err != nil {
			return role, nil, 0, err
		}
		if change.ModifiedCount != 1 {
			return role, nil, 0, Fail(p.CodeOperationConflict, "原冻结款不再可加价")
		}
		if order.ActiveBidCount > 1 {
			if err = s.queueItemMarketRefund(ctx, order, now); err != nil {
				return role, nil, 0, err
			}
		}
	}
	if _, err = s.DB.Collection(itemMarketBids).InsertOne(ctx, bid); err != nil {
		return role, nil, 0, err
	}
	role.Wallet.Yuanbao -= debit
	receipt := next.receipt(req.Action, order.Item.Quantity)
	receipt.YuanbaoDelta = -debit
	return role, receipt, 0, nil
}

func (s *Store) buyItemMarketOrder(ctx context.Context, role domain.Role, req *pb.ItemMarketAction, order ItemMarketOrder, now time.Time) (domain.Role, *ItemMarketReceipt, uint32, error) {
	if order.Phase != ItemMarketDirect || order.Revision >= math.MaxInt64 {
		return role, nil, p.CodeOperationConflict, nil
	}
	gross, valid := marketTotal(order.UnitPrice, req.Quantity)
	if !valid {
		return role, nil, p.CodeInvalidArgument, nil
	}
	if role.Wallet.Yuanbao < gross {
		return role, nil, p.CodeVendorFunds, nil
	}
	op := domain.Hex(req.Operation.OpId)
	item, remainder, valid := warehouseSplit(order.Item, req.Quantity, marketFactID("market-buy-item", role.ID, op))
	if !valid {
		return role, nil, p.CodeOperationConflict, nil
	}
	if code := marketReceiveItemCode(role, item); code != 0 {
		return role, nil, code, nil
	}
	next := order
	next.Revision++
	if remainder == nil {
		next.Phase, next.ClosedAt, next.Item.Quantity = ItemMarketSold, now, 0
	} else {
		next.Item = *remainder
	}
	if err := s.replaceItemMarketOrder(ctx, next, order.Revision); err != nil {
		return role, nil, 0, err
	}
	tax, err := s.chargeMarketSellerTax(ctx, order.Seller, gross, order.TaxDivisor)
	if err != nil {
		return role, nil, 0, err
	}
	fact := ItemMarketSettlement{ID: marketFactID("market-direct-sale", role.ID, op),
		OrderID: order.ID, RunID: order.RunID, Seller: order.Seller, Buyer: marketParty(role), Item: item,
		UnitPrice: order.UnitPrice, Gross: gross, Tax: tax, RuleVersion: order.RuleVersion,
		ConfigHash: order.ConfigHash, CreatedAt: now}
	if err = s.insertItemMarketSettlement(ctx, fact); err != nil {
		return role, nil, 0, err
	}
	if net := gross - tax; net > 0 {
		claim := ItemMarketClaim{ID: marketFactID("market-claim", fact.ID, "proceeds"), OrderID: order.ID,
			Owner: order.Seller, Kind: ItemMarketClaimProceeds, Revision: 1, Yuanbao: net, CreatedAt: now}
		if err = s.insertItemMarketClaim(ctx, claim); err != nil {
			return role, nil, 0, err
		}
	}
	role.Wallet.Yuanbao -= gross
	role.Items = append(slices.Clone(role.Items), item)
	receipt := next.receipt(req.Action, req.Quantity)
	receipt.YuanbaoDelta, receipt.Tax = -gross, tax
	return role, receipt, 0, nil
}

func (s *Store) cancelItemMarketOrder(ctx context.Context, role domain.Role, req *pb.ItemMarketAction, order ItemMarketOrder, now time.Time) (domain.Role, *ItemMarketReceipt, uint32, error) {
	if order.Seller != marketParty(role) {
		return role, nil, p.CodeLineForbidden, nil
	}
	if order.ActiveBidCount != 0 || order.WinnerBidID != "" || order.Revision >= math.MaxInt64 {
		return role, nil, p.CodeOperationConflict, nil
	}
	item := order.Item
	claim := ItemMarketClaim{ID: marketFactID("market-claim", order.ID, "cancel"), OrderID: order.ID,
		Owner: order.Seller, Kind: ItemMarketClaimItem, Revision: 1, Item: &item, CreatedAt: now}
	next := order
	next.Phase, next.ClosedAt, next.Item.Quantity = ItemMarketCancelled, now, 0
	next.Revision++
	if err := s.replaceItemMarketOrder(ctx, next, order.Revision); err != nil {
		return role, nil, 0, err
	}
	if err := s.insertItemMarketClaim(ctx, claim); err != nil {
		return role, nil, 0, err
	}
	receipt := next.receipt(req.Action, item.Quantity)
	receipt.EscrowID, receipt.EscrowVersion = claim.ID, claim.Revision
	return role, receipt, 0, nil
}

func (s *Store) claimItemMarketAsset(ctx context.Context, role domain.Role, req *pb.ItemMarketAction, now time.Time) (domain.Role, *ItemMarketReceipt, uint32, error) {
	filter := itemMarketOwnerFilter(role)
	filter["_id"], filter["revision"] = domain.Hex(req.EscrowId), int64(req.EscrowVersion)
	var claim ItemMarketClaim
	err := s.DB.Collection(itemMarketClaims).FindOne(ctx, filter).Decode(&claim)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return role, nil, p.CodeOperationConflict, nil
	}
	if err != nil {
		return role, nil, 0, err
	}
	receipt := &ItemMarketReceipt{Action: req.Action, OrderID: claim.OrderID, EscrowID: claim.ID, EscrowVersion: claim.Revision + 1}
	switch claim.Kind {
	case ItemMarketClaimItem:
		if claim.Item == nil || claim.Yuanbao != 0 || req.Quantity != 0 && req.Quantity != claim.Item.Quantity {
			return role, nil, p.CodeInvalidArgument, nil
		}
		item, _, valid := warehouseSplit(*claim.Item, claim.Item.Quantity, "")
		if !valid {
			return role, nil, p.CodeOperationConflict, nil
		}
		if code := marketReceiveItemCode(role, item); code != 0 {
			return role, nil, code, nil
		}
		role.Items = append(slices.Clone(role.Items), item)
		receipt.ItemID, receipt.Quantity = item.ItemID, item.Quantity
	case ItemMarketClaimProceeds, ItemMarketClaimRefund:
		if claim.Item != nil || claim.Yuanbao <= 0 || req.Quantity != 0 {
			return role, nil, p.CodeInvalidArgument, nil
		}
		role.Wallet.Yuanbao, err = add(role.Wallet.Yuanbao, claim.Yuanbao)
		if err != nil {
			return role, nil, 0, err
		}
		receipt.YuanbaoDelta = claim.Yuanbao
	default:
		return role, nil, p.CodeOperationConflict, nil
	}
	result, err := s.DB.Collection(itemMarketClaims).UpdateOne(ctx, filter, bson.M{
		"$set": bson.M{"claimed_at": now, "claim_op": domain.Hex(req.Operation.OpId)}, "$inc": bson.M{"revision": 1},
	})
	if err == nil && result.ModifiedCount != 1 {
		err = Fail(p.CodeOperationConflict, "托管原件已经领取")
	}
	return role, receipt, 0, err
}

func marketReceiveItemCode(role domain.Role, item domain.Item) uint32 {
	if len(role.Items) >= role.Capacity {
		return p.CodeEquipmentBagFull
	}
	for _, stored := range role.Items {
		if stored.ID == item.ID {
			return p.CodeOperationConflict
		}
	}
	for _, equipped := range role.Equipped {
		if equipped.Item.ID == item.ID {
			return p.CodeOperationConflict
		}
	}
	return 0
}
