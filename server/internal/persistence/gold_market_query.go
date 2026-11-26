// 本文件读取两个价格簿、原角色委托与专属金币托管资产。
package persistence

import (
	"context"
	"encoding/binary"
	"errors"
	"math"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/domain"
	p "minimir/internal/protocol"
)

// QueryGoldMarket 以一致读快照核对原账号、角色及运行服，不占永久事务写槽。
func (s *Store) QueryGoldMarket(ctx context.Context, auth Auth, q *pb.GoldMarketQuery) (*pb.GoldMarketState, error) {
	if q == nil || q.SceneEpoch == 0 || q.Scope < 1 || q.Scope > 4 || q.PageSize == 0 ||
		q.PageSize > itemMarketPageLimit || len(q.Cursor) != 0 && len(q.Cursor) != 16 ||
		len(q.ProtoReflect().GetUnknown()) != 0 {
		return nil, Fail(p.CodeInvalidArgument, "金币市场分页参数无效")
	}
	ctx, cancel := context.WithTimeout(ctx, transactionTimeout)
	defer cancel()
	var out *pb.GoldMarketState
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
		out = &pb.GoldMarketState{SceneEpoch: q.SceneEpoch, Scope: q.Scope, RoleRevision: uint64(role.Revision)}
		out.Rules, err = s.goldMarketTerms(tc)
		if err != nil && Code(err) != p.CodeLineForbidden {
			return err
		}
		if out.Rules == nil {
			out.Enabled, err = s.hasGoldMarketObligation(tc, role)
			if err != nil || !out.Enabled {
				return err
			}
		} else {
			out.Enabled, out.CanTrade = true, role.PrivilegeActiveAt(now)
		}
		if q.Scope == 4 {
			return s.readGoldMarketClaims(tc, role, q, out)
		}
		return s.readGoldMarketOrders(tc, role, q, now, out)
	})
	return out, err
}

func (s *Store) hasGoldMarketObligation(ctx context.Context, role domain.Role) (bool, error) {
	filter := bson.M{"run_server_id": s.Cfg.RunID, "owner.role_id": role.ID, "owner.origin_server_id": role.Origin, "state": GoldMarketActive}
	err := s.DB.Collection(goldMarketOrders).FindOne(ctx, filter).Err()
	if !errors.Is(err, mongo.ErrNoDocuments) {
		return err == nil, err
	}
	err = s.DB.Collection(goldMarketClaims).FindOne(ctx, goldMarketClaimOwner(role)).Err()
	if errors.Is(err, mongo.ErrNoDocuments) {
		return false, nil
	}
	return err == nil, err
}

func (s *Store) readGoldMarketClaims(ctx context.Context, role domain.Role, q *pb.GoldMarketQuery, state *pb.GoldMarketState) error {
	filter := goldMarketClaimOwner(role)
	if len(q.Cursor) != 0 {
		filter["_id"] = bson.M{"$gt": domain.Hex(q.Cursor)}
	}
	cursor, err := s.DB.Collection(goldMarketClaims).Find(ctx, filter,
		options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}).SetLimit(int64(q.PageSize+1)))
	if err != nil {
		return err
	}
	var rows []GoldMarketClaim
	if err = cursor.All(ctx, &rows); err != nil {
		return err
	}
	if len(rows) > int(q.PageSize) {
		rows = rows[:q.PageSize]
		state.NextCursor = domain.Bytes(rows[len(rows)-1].ID)
	}
	for _, row := range rows {
		state.Escrows = append(state.Escrows, &pb.GoldMarketEscrowView{EscrowId: domain.Bytes(row.ID),
			OrderId: domain.Bytes(row.OrderID), Version: uint64(row.Revision), Kind: row.Kind, Reason: row.Reason,
			Gold: row.Gold, YuanbaoUnits: row.Yuanbao, CreatedAtUnixMs: row.CreatedAt.UnixMilli()})
	}
	return nil
}

func (s *Store) readGoldMarketOrders(ctx context.Context, role domain.Role, q *pb.GoldMarketQuery, now time.Time, state *pb.GoldMarketState) error {
	filter := bson.M{"run_server_id": s.Cfg.RunID}
	sortBy := bson.D{{Key: "_id", Value: 1}}
	if q.Scope == 3 {
		filter["owner.role_id"], filter["owner.origin_server_id"] = role.ID, role.Origin
		if len(q.Cursor) != 0 {
			filter["_id"] = bson.M{"$gt": domain.Hex(q.Cursor)}
		}
	} else {
		filter["side"], filter["state"], filter["expires_at"] = q.Scope, GoldMarketActive, bson.M{"$gt": now}
		priceSort, priceOp := 1, "$gt"
		if q.Scope == GoldMarketBuy {
			priceSort, priceOp = -1, "$lt"
		}
		sortBy = bson.D{{Key: "limit_price", Value: priceSort}, {Key: "sequence", Value: 1}}
		if len(q.Cursor) != 0 {
			price, sequence := binary.BigEndian.Uint64(q.Cursor[:8]), binary.BigEndian.Uint64(q.Cursor[8:])
			if price == 0 || price > math.MaxInt64 || sequence == 0 || sequence > math.MaxInt64 {
				return Fail(p.CodeInvalidArgument, "金币价格簿原游标无效")
			}
			filter["$or"] = bson.A{bson.M{"limit_price": bson.M{priceOp: int64(price)}},
				bson.M{"limit_price": int64(price), "sequence": bson.M{"$gt": int64(sequence)}}}
		}
	}
	cursor, err := s.DB.Collection(goldMarketOrders).Find(ctx, filter, options.Find().SetSort(sortBy).SetLimit(int64(q.PageSize+1)))
	if err != nil {
		return err
	}
	var rows []GoldMarketOrder
	if err = cursor.All(ctx, &rows); err != nil {
		return err
	}
	if len(rows) > int(q.PageSize) {
		rows = rows[:q.PageSize]
		last := rows[len(rows)-1]
		if q.Scope == 3 {
			state.NextCursor = domain.Bytes(last.ID)
		} else {
			state.NextCursor = binary.BigEndian.AppendUint64(nil, uint64(last.LimitPrice))
			state.NextCursor = binary.BigEndian.AppendUint64(state.NextCursor, uint64(last.Sequence))
		}
	}
	for _, row := range rows {
		state.Orders = append(state.Orders, &pb.GoldMarketOrderView{OrderId: domain.Bytes(row.ID),
			Version: uint64(row.Revision), State: row.State, Side: row.Side, UnitPriceYuanbaoUnits: row.LimitPrice,
			InitialLots: row.InitialLots, RemainingLots: row.RemainingLots, ExpiresAtUnixMs: row.ExpiresAt.UnixMilli(),
			CreatedAtUnixMs: row.AcceptedAt.UnixMilli(), Owned: row.Owner == marketParty(role), SameAccount: row.Owner.AccountID == role.AccountID})
	}
	return nil
}
