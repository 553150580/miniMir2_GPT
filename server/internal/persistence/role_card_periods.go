// 本文件以可索引的卡期变更记录还原实际战斗和领取时刻的权益。
package persistence

import (
	"context"
	"errors"
	"math"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	p "minimir/internal/protocol"
)

// roleCardPeriodCollection 永久保留已确认卡期变更，与购买原回执同事务提交。
const roleCardPeriodCollection = "role_card_periods"

// roleCardPeriodChange 记录本次变更生效前的连续期，供查询生效之前的事件。
type roleCardPeriodChange struct {
	ID            string    `bson:"_id"`                      // 与原购买或充值体系履约操作共同拥有的唯一编号。
	RoleID        string    `bson:"role_id"`                  // 永久角色身份。
	Origin        uint64    `bson:"origin_server_id"`         // 永久入口服，合服不改变。
	Kind          uint32    `bson:"kind"`                     // 所修改的角色卡种。
	Revision      int64     `bson:"revision"`                 // 同毫秒内业务提交的唯一角色次序。
	EffectiveAt   time.Time `bson:"effective_at"`             // 当前卡期变更起效时间。
	PreviousSince time.Time `bson:"previous_since,omitempty"` // 上一期起点，旧角色缺省时沿用旧有效期语义。
	PreviousUntil time.Time `bson:"previous_until,omitempty"` // 上一期明确到期。
}

// roleCardPeriodIndexes 允许以一次定位找出事件之后的第一条同卡种变更。
func roleCardPeriodIndexes() []mongo.IndexModel {
	return []mongo.IndexModel{{Keys: bson.D{{Key: "role_id", Value: 1}, {Key: "origin_server_id", Value: 1},
		{Key: "kind", Value: 1}, {Key: "effective_at", Value: 1}, {Key: "revision", Value: 1}}}}
}

// insertRoleCardPeriod 在原购买卡期与永久回执的同一事务里写入权益历史。
func (s *Store) insertRoleCardPeriod(ctx context.Context, r domain.Role, opID string, kind uint32, at, previousSince, previousUntil time.Time) error {
	_, err := s.DB.Collection(roleCardPeriodCollection).InsertOne(ctx, roleCardPeriodChange{ID: opID,
		RoleID: r.ID, Origin: r.Origin, Kind: kind, Revision: r.Revision, EffectiveAt: at,
		PreviousSince: previousSince, PreviousUntil: previousUntil})
	return err
}

// extendRoleCardPeriod 从当前仍连续的末日续期，到期后采用本次确认时间作为新一期起点。
func extendRoleCardPeriod(r domain.Role, kind, days uint32, at time.Time) (domain.Role, time.Time, time.Time, error) {
	if at.IsZero() || days == 0 || days > 365 || kind != devconfig.RoleCardKindPrivilege && kind != devconfig.RoleCardKindHosting {
		return r, time.Time{}, time.Time{}, Fail(p.CodeInvalidArgument, "双卡商品期限或类别无效")
	}
	previousSince, previousUntil := roleCardPeriod(r, kind)
	if previousSince.After(at) {
		return r, time.Time{}, time.Time{}, Fail(p.CodeOperationConflict, "原卡期的有效时间异常")
	}
	since, until := at, at
	if previousUntil.After(at) {
		since, until = previousSince, previousUntil
	}
	length := time.Duration(days) * 24 * time.Hour
	if until.UnixMilli() > math.MaxInt64-length.Milliseconds() {
		return r, time.Time{}, time.Time{}, Fail(p.CodeOperationConflict, "续期后的卡期限溢出")
	}
	until = until.Add(length)
	if kind == devconfig.RoleCardKindPrivilege {
		r.PrivilegeSince, r.PrivilegeUntil = since, until
	} else {
		r.HostingCardSince, r.HostingCardUntil = since, until
	}
	return r, since, until, nil
}

// roleCardPeriod 读取当前连续卡期，旧角色起点空值维持原资格。
func roleCardPeriod(r domain.Role, kind uint32) (time.Time, time.Time) {
	if kind == devconfig.RoleCardKindPrivilege {
		return r.PrivilegeSince, r.PrivilegeUntil
	}
	return r.HostingCardSince, r.HostingCardUntil
}

// roleCardUntilAt 读到第一条晚于事件的卡期变更时，使用该变更之前的期限。
func (s *Store) roleCardUntilAt(ctx context.Context, r domain.Role, kind uint32, at time.Time) (time.Time, error) {
	at = bossTime(at)
	since, until := roleCardPeriod(r, kind)
	var next roleCardPeriodChange
	err := s.DB.Collection(roleCardPeriodCollection).FindOne(ctx, bson.M{"role_id": r.ID, "origin_server_id": r.Origin,
		"kind": kind, "effective_at": bson.M{"$gt": at}},
		options.FindOne().SetSort(bson.D{{Key: "effective_at", Value: 1}, {Key: "revision", Value: 1}})).Decode(&next)
	if err == nil {
		since, until = next.PreviousSince, next.PreviousUntil
	} else if !errors.Is(err, mongo.ErrNoDocuments) {
		return time.Time{}, err
	}
	if !until.After(at) || !since.IsZero() && since.After(at) {
		return time.Time{}, nil
	}
	return until, nil
}

// RolePrivilegeUntilAt 根据原角色与入口服查询到期时间，结果为空表示事件时特权无效。
func (s *Store) RolePrivilegeUntilAt(ctx context.Context, r domain.Role, at time.Time) (time.Time, error) {
	return s.roleCardUntilAt(ctx, r, devconfig.RoleCardKindPrivilege, at)
}
