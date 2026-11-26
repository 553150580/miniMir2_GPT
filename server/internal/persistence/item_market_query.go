// 本文件分页读取公开拍品、本人订单及待领资产，不更改角色或市场。
package persistence

import (
	"cmp"
	"context"
	"errors"
	"math"
	"slices"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/domain"
	p "minimir/internal/protocol"
)

const itemMarketPageLimit = 20

// QueryItemMarket 用当前会话和运行服读屏障保护三个分页范围。
func (s *Store) QueryItemMarket(ctx context.Context, auth Auth, q *pb.ItemMarketQuery) (*pb.ItemMarketState, error) {
	if q == nil || q.SceneEpoch == 0 || q.Scope < 1 || q.Scope > 3 || q.PageSize == 0 ||
		q.PageSize > itemMarketPageLimit || len(q.Cursor) != 0 && len(q.Cursor) != 16 ||
		q.Scope == 3 && q.ItemId != 0 || len(q.ProtoReflect().GetUnknown()) != 0 {
		return nil, Fail(p.CodeInvalidArgument, "道具市场分页参数无效")
	}
	ctx, stop := context.WithTimeout(ctx, transactionTimeout)
	defer stop()
	var out *pb.ItemMarketState
	err := s.tx(ctx, func(tc context.Context) error {
		if err := s.warehouseReadFence(tc, auth); err != nil {
			return err
		}
		role, err := s.Role(tc, auth.RoleID)
		if err != nil {
			return err
		}
		if err = s.checkRole(role, auth); err != nil {
			return err
		}
		now := bossTime(time.Now())
		rules := s.Cfg.Fixture.ItemMarket
		out = &pb.ItemMarketState{SceneEpoch: q.SceneEpoch, Scope: q.Scope, Enabled: rules != nil,
			CanTrade: rules != nil && role.PrivilegeActiveAt(now), RoleRevision: uint64(role.Revision)}
		if out.CanTrade {
			var frozen bool
			frozen, err = s.marketMergeFrozen(tc)
			if err != nil {
				return err
			}
			out.CanTrade = !frozen
		}
		if !out.Enabled {
			out.Enabled, err = s.hasItemMarketObligations(tc, role)
			if err != nil || !out.Enabled {
				return err
			}
		}
		if q.Scope == 3 {
			return s.readItemMarketClaims(tc, role, q, out)
		}
		return s.readItemMarketOrders(tc, role, q, now, out)
	})
	return out, err
}

// hasItemMarketObligations 保留停开新单后的老订单与原角色待领入口。
func (s *Store) hasItemMarketObligations(ctx context.Context, role domain.Role) (bool, error) {
	err := s.DB.Collection(itemMarketOrders).FindOne(ctx, bson.M{
		"run_server_id": s.Cfg.RunID, "phase": bson.M{"$in": []uint32{ItemMarketAuction, ItemMarketDirect}},
	}).Err()
	if !errors.Is(err, mongo.ErrNoDocuments) {
		return err == nil, err
	}
	err = s.DB.Collection(itemMarketClaims).FindOne(ctx, itemMarketOwnerFilter(role)).Err()
	if errors.Is(err, mongo.ErrNoDocuments) {
		return false, nil
	}
	return err == nil, err
}

func itemMarketOwnerFilter(role domain.Role) bson.M {
	return bson.M{"owner.role_id": role.ID, "owner.account_id": role.AccountID,
		"owner.origin_server_id": role.Origin, "claimed_at": nil}
}

func (s *Store) readItemMarketClaims(ctx context.Context, role domain.Role, q *pb.ItemMarketQuery, state *pb.ItemMarketState) error {
	filter := itemMarketOwnerFilter(role)
	if len(q.Cursor) != 0 {
		filter["_id"] = bson.M{"$gt": domain.Hex(q.Cursor)}
	}
	cursor, err := s.DB.Collection(itemMarketClaims).Find(ctx, filter,
		options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}).SetLimit(int64(q.PageSize+1)))
	if err != nil {
		return err
	}
	var rows []ItemMarketClaim
	if err = cursor.All(ctx, &rows); err != nil {
		return err
	}
	if len(rows) > int(q.PageSize) {
		rows = rows[:q.PageSize]
		state.NextCursor = domain.Bytes(rows[len(rows)-1].ID)
	}
	for _, row := range rows {
		item := &pb.ItemMarketEscrowView{EscrowId: domain.Bytes(row.ID), OrderId: domain.Bytes(row.OrderID),
			Version: uint64(row.Revision), Kind: row.Kind, YuanbaoUnits: row.Yuanbao, CreatedAtUnixMs: row.CreatedAt.UnixMilli()}
		if row.Item != nil {
			item.Item = row.Item.Proto()
		}
		state.Escrows = append(state.Escrows, item)
	}
	return nil
}

func (s *Store) readItemMarketOrders(ctx context.Context, role domain.Role, q *pb.ItemMarketQuery, now time.Time, state *pb.ItemMarketState) error {
	filter := bson.M{"run_server_id": s.Cfg.RunID}
	after := domain.Hex(q.Cursor)
	if after != "" {
		filter["_id"] = bson.M{"$gt": after}
	}
	if q.ItemId != 0 {
		filter["item.item_id"] = q.ItemId
	}
	if q.Scope == 1 {
		filter["$or"] = bson.A{
			bson.M{"phase": ItemMarketAuction, "auction_ends_at": bson.M{"$gt": now}},
			bson.M{"phase": ItemMarketDirect, "direct_ends_at": bson.M{"$gt": now}},
		}
	} else {
		filter["seller.role_id"], filter["seller.origin_server_id"] = role.ID, role.Origin
	}
	limit := int64(q.PageSize + 1)
	sortOrder := bson.D{{Key: "_id", Value: 1}}
	pipe := mongo.Pipeline{{{Key: "$match", Value: filter}}, {{Key: "$sort", Value: sortOrder}}, {{Key: "$limit", Value: limit}}}
	cursor, err := s.DB.Collection(itemMarketOrders).Aggregate(ctx, pipe)
	if err != nil {
		return err
	}
	var rows []ItemMarketOrder
	if err = cursor.All(ctx, &rows); err != nil {
		return err
	}
	if q.Scope == 2 {
		// 同一读快照分别取卖方和预购方索引页，仅合并本页有限记录。
		cursor, err = s.DB.Collection(itemMarketBids).Aggregate(ctx, s.marketBidOrderPipeline(role, q.ItemId, after, now, limit))
		if err != nil {
			return err
		}
		var purchased []ItemMarketOrder
		if err = cursor.All(ctx, &purchased); err != nil {
			return err
		}
		rows = append(rows, purchased...)
		slices.SortFunc(rows, func(a, b ItemMarketOrder) int { return cmp.Compare(a.ID, b.ID) })
	}
	if len(rows) > int(q.PageSize) {
		rows = rows[:q.PageSize]
		state.NextCursor = domain.Bytes(rows[len(rows)-1].ID)
	}
	bids, err := s.readPersonalItemBids(ctx, role, rows)
	if err != nil {
		return err
	}
	for _, order := range rows {
		state.Orders = append(state.Orders, order.marketView(role, bids[order.ID], s, now))
	}
	return nil
}

func (s *Store) marketBidOrderPipeline(role domain.Role, itemID uint32, after string, now time.Time, limit int64) mongo.Pipeline {
	filter := bson.M{"run_server_id": s.Cfg.RunID, "buyer.role_id": role.ID,
		"buyer.origin_server_id": role.Origin, "state": itemBidFrozen}
	if after != "" {
		filter["order_id"] = bson.M{"$gt": after}
	}
	matching := bson.M{"order.phase": ItemMarketAuction, "order.auction_ends_at": bson.M{"$gt": now},
		"$expr": bson.M{"$eq": bson.A{"$round", "$order.round"}}}
	if itemID != 0 {
		matching["order.item.item_id"] = itemID
	}
	return mongo.Pipeline{
		{{Key: "$match", Value: filter}},
		{{Key: "$sort", Value: bson.D{{Key: "order_id", Value: 1}}}},
		{{Key: "$lookup", Value: bson.M{"from": itemMarketOrders, "localField": "order_id", "foreignField": "_id", "as": "order"}}},
		{{Key: "$unwind", Value: "$order"}},
		{{Key: "$match", Value: matching}},
		{{Key: "$limit", Value: limit}},
		{{Key: "$replaceRoot", Value: bson.M{"newRoot": "$order"}}},
	}
}

func (s *Store) readPersonalItemBids(ctx context.Context, role domain.Role, orders []ItemMarketOrder) (map[string]int64, error) {
	if len(orders) == 0 {
		return nil, nil
	}
	list := make(bson.A, 0, len(orders))
	for _, row := range orders {
		if row.Phase == ItemMarketAuction && row.ActiveBidCount != 0 && row.Seller.AccountID != role.AccountID {
			list = append(list, bson.M{"order_id": row.ID, "round": row.Round})
		}
	}
	if len(list) == 0 {
		return nil, nil
	}
	cursor, err := s.DB.Collection(itemMarketBids).Find(ctx, bson.M{"$or": list, "buyer.role_id": role.ID, "state": itemBidFrozen})
	if err != nil {
		return nil, err
	}
	var rows []ItemMarketBid
	if err = cursor.All(ctx, &rows); err != nil {
		return nil, err
	}
	found := make(map[string]int64, len(rows))
	for _, row := range rows {
		found[row.OrderID] = row.Round
	}
	return found, nil
}

func (o ItemMarketOrder) marketView(role domain.Role, bidRound int64, store *Store, now time.Time) *pb.ItemMarketOrderView {
	view := o.Item.Proto()
	// 公开订单仅展示道具属性，脱离背包后不暴露原资产编号和来源流水。
	view.InstanceId, view.SourceId, view.SourceKind, view.Version = nil, "", "", 0
	end := o.AuctionEndsAt
	if o.Phase == ItemMarketDirect {
		end = o.DirectEndsAt
	} else if !o.ClosedAt.IsZero() {
		end = o.ClosedAt
	}
	nextPrice := int64(0)
	if o.Phase == ItemMarketAuction && o.activeAt(now) {
		nextPrice, _ = marketNextPrice(o.UnitPrice, store.Cfg.Fixture.ItemMarket)
	}
	return &pb.ItemMarketOrderView{OrderId: domain.Bytes(o.ID), Version: uint64(o.Revision), State: o.Phase, Item: view,
		RemainingQuantity: o.Item.Quantity, UnitPriceYuanbaoUnits: o.UnitPrice, EndAtUnixMs: end.UnixMilli(),
		ActiveBidCount: uint32(min(max(o.ActiveBidCount, 0), math.MaxUint32)), Owned: o.Seller == marketParty(role),
		SameAccount: o.Seller.AccountID == role.AccountID, MyActiveBid: bidRound > 0 && bidRound == o.Round && o.activeAt(now),
		NextUnitPriceYuanbaoUnits: nextPrice}
}
