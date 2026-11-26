// 本文件在原角色资产事务内冻结委托、撤销余量和领取货币。
package persistence

import (
	"context"
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

const AssetGoldMarket = "gold_market" // 走原角色经济序号并保留永久回执。

const (
	goldMarketPlace uint32 = iota + 1
	goldMarketCancel
	goldMarketClaim
)

// GoldMarketReceipt 保存首次已确认的原操作结果，只报告本角色实际钱包变更。
type GoldMarketReceipt struct {
	Action        uint32    `bson:"action"`
	OrderID       string    `bson:"order_id"`
	OrderVersion  int64     `bson:"order_version,omitempty"`
	Side          uint32    `bson:"side,omitempty"`
	State         uint32    `bson:"state,omitempty"`
	Lots          int64     `bson:"lots,omitempty"`
	FilledLots    int64     `bson:"filled_lots,omitempty"`
	RemainingLots int64     `bson:"remaining_lots,omitempty"`
	UnitPrice     int64     `bson:"unit_price,omitempty"`
	GoldDelta     int64     `bson:"gold_delta,omitempty"`
	YuanbaoDelta  int64     `bson:"yuanbao_delta,omitempty"`
	EscrowID      string    `bson:"escrow_id,omitempty"`
	EscrowVersion int64     `bson:"escrow_version,omitempty"`
	ExpiresAt     time.Time `bson:"expires_at,omitempty"`
}

func (r *GoldMarketReceipt) Proto() *pb.GoldMarketReceipt {
	if r == nil {
		return nil
	}
	return &pb.GoldMarketReceipt{Action: r.Action, OrderId: domain.Bytes(r.OrderID), OrderVersion: uint64(r.OrderVersion),
		Side: r.Side, State: r.State, QuantityLots: r.Lots, FilledLots: r.FilledLots, RemainingLots: r.RemainingLots,
		UnitPriceYuanbaoUnits: r.UnitPrice, GoldDelta: r.GoldDelta, YuanbaoDeltaUnits: r.YuanbaoDelta,
		EscrowId: domain.Bytes(r.EscrowID), EscrowVersion: uint64(r.EscrowVersion), ExpiresAtUnixMs: cardUnixMillis(r.ExpiresAt)}
}

func validGoldMarketAction(q *pb.GoldMarketAction) bool {
	if q == nil || q.Operation == nil || len(q.Operation.OpId) != 16 || len(q.ProtoReflect().GetUnknown()) != 0 ||
		len(q.Operation.ProtoReflect().GetUnknown()) != 0 {
		return false
	}
	emptyOrder := len(q.OrderId) == 0 && q.OrderVersion == 0
	emptyEscrow := len(q.EscrowId) == 0 && q.EscrowVersion == 0
	emptyTerms := q.Side == 0 && q.Lots == 0 && q.UnitPriceYuanbaoUnits == 0 && q.TermsHash == "" && q.ExpectedDurationSeconds == 0
	switch q.Action {
	case goldMarketPlace:
		return emptyOrder && emptyEscrow && (q.Side == GoldMarketBuy || q.Side == GoldMarketSell) &&
			q.Lots > 0 && q.UnitPriceYuanbaoUnits > 0 && lowerHex(q.TermsHash, 32) && q.ExpectedDurationSeconds > 0
	case goldMarketCancel:
		return emptyTerms && emptyEscrow && len(q.OrderId) == 16 && q.OrderVersion > 0 && q.OrderVersion < math.MaxInt64
	case goldMarketClaim:
		return emptyTerms && emptyOrder && len(q.EscrowId) == 16 && q.EscrowVersion > 0 && q.EscrowVersion < math.MaxInt64
	}
	return false
}

// applyGoldMarket 仅限制新委托的权益，先前形成的待领和可撤余额仍可原角色处理。
func (s *Store) applyGoldMarket(ctx context.Context, role domain.Role, q *pb.GoldMarketAction, gate uint32) (domain.Role, *GoldMarketReceipt, uint32, error) {
	if !validGoldMarketAction(q) {
		return role, nil, p.CodeInvalidArgument, nil
	}
	if gate != 0 {
		return role, nil, gate, nil
	}
	now := bossTime(time.Now())
	switch q.Action {
	case goldMarketPlace:
		return s.placeGoldMarketOrder(ctx, role, q, now)
	case goldMarketCancel:
		return s.cancelGoldMarketOrder(ctx, role, q, now)
	default:
		return s.claimGoldMarketAsset(ctx, role, q, now)
	}
}

func (s *Store) placeGoldMarketOrder(ctx context.Context, role domain.Role, q *pb.GoldMarketAction, now time.Time) (domain.Role, *GoldMarketReceipt, uint32, error) {
	rules := s.Cfg.Fixture.GoldMarket
	if rules == nil || !role.PrivilegeActiveAt(now) {
		return role, nil, p.CodeLineForbidden, nil
	}
	terms, err := s.goldMarketTerms(ctx)
	if err != nil {
		return role, nil, 0, err
	}
	price := q.UnitPriceYuanbaoUnits
	if q.TermsHash != terms.TermsHash || q.ExpectedDurationSeconds != terms.OrderDurationSeconds ||
		price < terms.MinUnitPriceYuanbaoUnits || price > terms.MaxUnitPriceYuanbaoUnits || price%rules.PriceStepYuanbaoUnits != 0 {
		return role, nil, p.CodeOperationConflict, nil
	}
	gold, validGold := marketTotal(rules.GoldPerLot, q.Lots)
	yuanbao, validYuanbao := marketTotal(price, q.Lots)
	if !validGold || !validYuanbao {
		return role, nil, p.CodeInvalidArgument, nil
	}
	capacity := int64(rules.MaxBuyOrdersPerAccountRun)
	if q.Side == GoldMarketSell {
		capacity = int64(rules.MaxSellOrdersPerAccountRun)
		if role.Wallet.Gold < gold {
			return role, nil, p.CodeVendorFunds, nil
		}
	} else if role.Wallet.Yuanbao < yuanbao {
		return role, nil, p.CodeVendorFunds, nil
	}
	count, err := s.DB.Collection(goldMarketOrders).CountDocuments(ctx, bson.M{
		"run_server_id": s.Cfg.RunID, "owner.account_id": role.AccountID, "side": q.Side, "state": GoldMarketActive,
		"expires_at": bson.M{"$gt": now},
	}, options.Count().SetLimit(capacity))
	if err != nil {
		return role, nil, 0, err
	}
	if count >= capacity {
		return role, nil, p.CodeOperationConflict, nil
	}
	sequence, err := s.nextGoldMarketSequence(ctx, "order_sequence")
	if err != nil {
		return role, nil, 0, err
	}
	id := marketFactID("gold-order", role.ID, domain.Hex(q.Operation.OpId))
	order := GoldMarketOrder{ID: id, RunID: s.Cfg.RunID, Owner: marketParty(role), Side: q.Side, State: GoldMarketActive,
		Sequence: sequence, LimitPrice: price, InitialLots: q.Lots, RemainingLots: q.Lots, Revision: 1,
		TaxDivisor: rules.TaxDivisor, GoldPerLot: rules.GoldPerLot, AcceptedAt: now,
		ExpiresAt: now.Add(time.Duration(rules.OrderExpirySeconds) * time.Second), PendingMatch: true,
		RuleVersion: terms.RuleVersion, ConfigHash: terms.ConfigHash}
	if order.Side == GoldMarketBuy {
		order.FrozenYuanbao = yuanbao
		role.Wallet.Yuanbao -= yuanbao
	} else {
		order.FrozenGold = gold
		role.Wallet.Gold -= gold
	}
	if _, err := s.DB.Collection(goldMarketOrders).InsertOne(ctx, order); err != nil {
		return role, nil, 0, err
	}
	receipt := order.assetReceipt(q.Action)
	receipt.GoldDelta, receipt.YuanbaoDelta = -order.FrozenGold, -order.FrozenYuanbao
	return role, receipt, 0, nil
}

func (s *Store) cancelGoldMarketOrder(ctx context.Context, role domain.Role, q *pb.GoldMarketAction, now time.Time) (domain.Role, *GoldMarketReceipt, uint32, error) {
	filter := bson.M{"_id": domain.Hex(q.OrderId), "run_server_id": s.Cfg.RunID, "owner.role_id": role.ID,
		"owner.account_id": role.AccountID, "owner.origin_server_id": role.Origin, "revision": int64(q.OrderVersion),
		"state": GoldMarketActive, "expires_at": bson.M{"$gt": now}}
	var order GoldMarketOrder
	err := s.DB.Collection(goldMarketOrders).FindOne(ctx, filter).Decode(&order)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return role, nil, p.CodeOperationConflict, nil
	}
	if err != nil {
		return role, nil, 0, err
	}
	claim := goldMarketReturnClaim(order, GoldMarketReasonCancel, now)
	if !validGoldMarketClaim(claim) || order.RemainingLots <= 0 {
		return role, nil, p.CodeOperationConflict, nil
	}
	revision := order.Revision
	order.State, order.ClosedAt, order.PendingMatch, order.Revision = GoldMarketCancelled, now, false, revision+1
	order.FrozenGold, order.FrozenYuanbao = 0, 0
	if err = s.replaceGoldMarketOrder(ctx, order, revision); err != nil {
		return role, nil, 0, err
	}
	if err = s.insertGoldMarketClaim(ctx, claim); err != nil {
		return role, nil, 0, err
	}
	receipt := order.assetReceipt(q.Action)
	receipt.EscrowID, receipt.EscrowVersion = claim.ID, claim.Revision
	return role, receipt, 0, nil
}

func goldMarketReturnClaim(order GoldMarketOrder, reason uint32, now time.Time) GoldMarketClaim {
	kind := GoldMarketClaimYuanbaoRefund
	if order.Side == GoldMarketSell {
		kind = GoldMarketClaimGoldReturn
	}
	return GoldMarketClaim{ID: marketFactID("gold-claim", order.ID, "remainder"), OrderID: order.ID, Owner: order.Owner,
		Kind: kind, Reason: reason, Revision: 1, Gold: order.FrozenGold, Yuanbao: order.FrozenYuanbao, CreatedAt: now}
}

func validGoldMarketClaim(claim GoldMarketClaim) bool {
	switch claim.Kind {
	case GoldMarketClaimGold, GoldMarketClaimGoldReturn:
		return claim.Gold > 0 && claim.Yuanbao == 0
	case GoldMarketClaimProceeds, GoldMarketClaimYuanbaoRefund:
		return claim.Yuanbao > 0 && claim.Gold == 0
	}
	return false
}

func goldMarketClaimOwner(role domain.Role) bson.M {
	return bson.M{"owner.role_id": role.ID, "owner.account_id": role.AccountID,
		"owner.origin_server_id": role.Origin, "claimed_at": nil}
}

func (s *Store) claimGoldMarketAsset(ctx context.Context, role domain.Role, q *pb.GoldMarketAction, now time.Time) (domain.Role, *GoldMarketReceipt, uint32, error) {
	filter := goldMarketClaimOwner(role)
	filter["_id"], filter["revision"] = domain.Hex(q.EscrowId), int64(q.EscrowVersion)
	var claim GoldMarketClaim
	err := s.DB.Collection(goldMarketClaims).FindOne(ctx, filter).Decode(&claim)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return role, nil, p.CodeOperationConflict, nil
	}
	if err != nil {
		return role, nil, 0, err
	}
	if !validGoldMarketClaim(claim) {
		return role, nil, p.CodeOperationConflict, nil
	}
	if role.Wallet.Gold, err = add(role.Wallet.Gold, claim.Gold); err != nil {
		return role, nil, 0, err
	}
	if role.Wallet.Yuanbao, err = add(role.Wallet.Yuanbao, claim.Yuanbao); err != nil {
		return role, nil, 0, err
	}
	result, err := s.DB.Collection(goldMarketClaims).UpdateOne(ctx, filter, bson.M{
		"$set": bson.M{"claimed_at": now, "claim_op": domain.Hex(q.Operation.OpId)}, "$inc": bson.M{"revision": 1},
	})
	if err != nil {
		return role, nil, 0, err
	}
	if result.ModifiedCount != 1 {
		return role, nil, 0, Fail(p.CodeOperationConflict, "金币市场原托管款已领取")
	}
	return role, &GoldMarketReceipt{Action: q.Action, OrderID: claim.OrderID, EscrowID: claim.ID, EscrowVersion: claim.Revision + 1,
		GoldDelta: claim.Gold, YuanbaoDelta: claim.Yuanbao}, 0, nil
}

func (o GoldMarketOrder) assetReceipt(action uint32) *GoldMarketReceipt {
	return &GoldMarketReceipt{Action: action, OrderID: o.ID, OrderVersion: o.Revision, Side: o.Side, State: o.State,
		Lots: o.InitialLots, FilledLots: o.InitialLots - o.RemainingLots, RemainingLots: o.RemainingLots,
		UnitPrice: o.LimitPrice, ExpiresAt: o.ExpiresAt}
}
