// 本文件保存跨赛季的可兑换积分与绑定羽毛的原子成交。
package persistence

import (
	"context"
	"errors"
	"fmt"
	"math"
	"slices"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/domain"
	p "minimir/internal/protocol"
)

const (
	// AssetMirrorArenaExchange 走玩家持久操作序号、角色修订及原回执。
	AssetMirrorArenaExchange              = "mirror_arena_exchange"
	mirrorExchangeCollection              = "mirror_arena_exchange_wallets" // 永久入口与角色共同限定的余额。
	mirrorExchangeCreditCollection        = "mirror_arena_exchange_credits" // 每个原赛季、原角色最多一条结转证明。
	maxMirrorExchangePacks         uint32 = 100                             // 限定单笔操作的份数，避免超量入包。
)

type mirrorExchangeBalance struct {
	ID        string    `bson:"_id"`        // 永久入口与角色组成的资产键，不随运行服改变。
	Origin    uint64    `bson:"origin"`     // 冗余身份用于拒绝错误历史归属。
	RoleID    string    `bson:"role_id"`    // 原角色的全局编号。
	Points    int64     `bson:"points"`     // 可消费的当前余额，兑换从不修改原赛季积分。
	UpdatedAt time.Time `bson:"updated_at"` // 最新结转或消费的服务端时点。
}

type mirrorExchangeCredit struct {
	ID        string    `bson:"_id"`        // 赛季与永久兑换资产键。
	BalanceID string    `bson:"balance_id"` // 本次结转对应的永久角色钱包。
	SeasonID  string    `bson:"season_id"`  // 原运行服、永久入口和赛季起点。
	Points    int64     `bson:"points"`     // 原赛季成绩按照 1:1 转出的原值。
	At        time.Time `bson:"at"`         // 完成原赛季结转的服务端时刻。
}

// MirrorExchangeReceipt 记录首次成交后的余额和发放的原绑定实例，重试沿用原值。
type MirrorExchangeReceipt struct {
	SpentPoints     int64       `bson:"spent_points"`     // 首次成交扣除的可兑换积分。
	RemainingPoints int64       `bson:"remaining_points"` // 首次成交后的可兑换积分余额。
	Feather         domain.Item `bson:"feather"`          // 同一资产事务确认的绑定羽毛实例。
}

func (r *MirrorExchangeReceipt) Proto() *pb.MirrorArenaExchangeReceipt {
	if r == nil {
		return nil
	}
	return &pb.MirrorArenaExchangeReceipt{SpentPoints: r.SpentPoints, RemainingPoints: r.RemainingPoints,
		ItemInstanceId: domain.Bytes(r.Feather.ID), ItemId: r.Feather.ItemID, Quantity: r.Feather.Quantity}
}

func (s *Store) mirrorExchangeBalance(ctx context.Context, origin uint64, roleID string) (mirrorExchangeBalance, bool, error) {
	key := mirrorRoleKey(origin, roleID)
	var balance mirrorExchangeBalance
	err := s.DB.Collection(mirrorExchangeCollection).FindOne(ctx, bson.M{"_id": key}).Decode(&balance)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return mirrorExchangeBalance{ID: key, Origin: origin, RoleID: roleID}, false, nil
	}
	if err != nil {
		return balance, false, err
	}
	if balance.ID != key || balance.Origin != origin || balance.RoleID != roleID || balance.Points < 0 {
		return balance, false, Fail(p.CodeOperationConflict, "镜像兑换余额的永久归属或数值异常")
	}
	return balance, true, nil
}

// ReadMirrorExchangePoints 在调用方已经核对玩家身份的读取事务中取永久兑换余额。
func (s *Store) ReadMirrorExchangePoints(ctx context.Context, origin uint64, roleID string) (int64, error) {
	balance, _, err := s.mirrorExchangeBalance(ctx, origin, roleID)
	return balance.Points, err
}

// creditMirrorExchangePoints 在原赛季结算的同一事务内做 1:1 结转，重复只核对原证明。
func (s *Store) creditMirrorExchangePoints(ctx context.Context, origin uint64, roleID, seasonID string, points int64, now time.Time) error {
	if origin == 0 || !lowerHex(roleID, 16) || seasonID == "" || points <= 0 || now.IsZero() {
		return Fail(p.CodeInvalidArgument, "镜像兑换积分结转的原季或分数无效")
	}
	walletID := mirrorRoleKey(origin, roleID)
	proof := mirrorExchangeCredit{ID: seasonID + "/" + walletID, BalanceID: walletID, SeasonID: seasonID, Points: points, At: bossTime(now)}
	credits := s.DB.Collection(mirrorExchangeCreditCollection)
	var prior mirrorExchangeCredit
	err := credits.FindOne(ctx, bson.M{"_id": proof.ID}).Decode(&prior)
	if err == nil {
		if prior.BalanceID == proof.BalanceID && prior.SeasonID == proof.SeasonID && prior.Points == proof.Points {
			return nil
		}
		return Fail(p.CodeOperationConflict, "同一原赛季的镜像结转内容发生变化")
	}
	if !errors.Is(err, mongo.ErrNoDocuments) {
		return err
	}
	balance, exists, err := s.mirrorExchangeBalance(ctx, origin, roleID)
	if err != nil {
		return err
	}
	next, err := add(balance.Points, points)
	if err != nil {
		return err
	}
	if err = s.writeMirrorExchangeBalance(ctx, balance, exists, next, proof.At); err != nil {
		return err
	}
	_, err = credits.InsertOne(ctx, proof)
	return err
}

func (s *Store) writeMirrorExchangeBalance(ctx context.Context, balance mirrorExchangeBalance, exists bool, points int64, at time.Time) error {
	wallets := s.DB.Collection(mirrorExchangeCollection)
	if !exists {
		balance.Points, balance.UpdatedAt = points, at
		_, err := wallets.InsertOne(ctx, balance)
		return err
	}
	updated, err := wallets.UpdateOne(ctx, bson.M{"_id": balance.ID, "origin": balance.Origin, "role_id": balance.RoleID, "points": balance.Points},
		bson.M{"$set": bson.M{"points": points, "updated_at": at}})
	if err != nil {
		return err
	}
	if updated.MatchedCount != 1 {
		return Fail(p.CodeOperationPending, "镜像兑换余额待核对")
	}
	return nil
}

// exchangeMirrorFeather 仅在原角色资产事务内调用。满包、费用不足及非法份数不更新余额。
func (s *Store) exchangeMirrorFeather(ctx context.Context, role domain.Role, q AssetRequest, operationID string) (domain.Role, *MirrorExchangeReceipt, uint32, error) {
	if q.MirrorExchangeGate != 0 {
		return role, nil, q.MirrorExchangeGate, nil
	}
	config := s.Cfg.Fixture.MirrorArena
	if config == nil || config.Exchange == nil {
		return role, nil, p.CodeLineForbidden, nil
	}
	offer, request := config.Exchange, q.MirrorExchange
	if request == nil {
		return role, nil, p.CodeInvalidArgument, nil
	}
	if request.ExpectedPrice != offer.Price || request.ExpectedItemId != offer.ItemID || request.ExpectedPackQuantity != offer.Quantity {
		return role, nil, p.CodeOperationConflict, nil
	}
	packs := int64(request.PackCount)
	if packs <= 0 || request.PackCount > maxMirrorExchangePacks || offer.Price <= 0 || offer.Quantity <= 0 ||
		packs > math.MaxInt64/offer.Price || packs > math.MaxInt64/offer.Quantity {
		return role, nil, p.CodeInvalidArgument, nil
	}
	definition, exists := s.Cfg.Fixture.Item(offer.ItemID)
	if !exists || definition.IsEquipment || !definition.Stackable || definition.MaxStack <= 0 {
		return role, nil, p.CodeLineForbidden, nil
	}
	quantity := packs * offer.Quantity
	if quantity > definition.MaxStack {
		return role, nil, p.CodeInvalidArgument, nil
	}
	if len(role.Items) >= role.Capacity {
		return role, nil, p.CodeEquipmentBagFull, nil
	}
	cost := packs * offer.Price
	balance, known, err := s.mirrorExchangeBalance(ctx, role.Origin, role.ID)
	if err != nil {
		return role, nil, 0, err
	}
	if !known || balance.Points < cost {
		return role, nil, p.CodeMirrorExchangeFunds, nil
	}
	remaining := balance.Points - cost
	feather := domain.Item{ID: Hash(fmt.Appendf(nil, "mirror_feather:%s", operationID))[:32], ItemID: offer.ItemID,
		Quantity: quantity, Bound: true, Version: 1, SourceKind: AssetMirrorArenaExchange, SourceID: domain.Hex(q.Context.OpId)}
	if err = s.writeMirrorExchangeBalance(ctx, balance, true, remaining, bossTime(time.Now())); err != nil {
		return role, nil, 0, err
	}
	role.Items = append(slices.Clone(role.Items), feather)
	return role, &MirrorExchangeReceipt{SpentPoints: cost, RemainingPoints: remaining, Feather: feather}, 0, nil
}
