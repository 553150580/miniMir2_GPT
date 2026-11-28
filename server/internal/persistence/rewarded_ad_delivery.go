// 本文件按永久入口发现原广告事件，并在角色资产事务内确认实际托管到账。
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

const rewardedAdGrantKind = "rewarded_ad_grant" // 激励视频仅增加托管秒数，单独记录来源。

// RewardedAdGrantReceipt 固定原广告实际增加的秒数，后续使用余额不改原回执。
type RewardedAdGrantReceipt struct {
	Attempt   string    `bson:"attempt_id"`       // 游戏签发的原观看凭据。
	Event     string    `bson:"event_key"`        // 已核验渠道事件的稳定摘要。
	Account   string    `bson:"account_id"`       // 原受益账号。
	Origin    uint64    `bson:"origin_server_id"` // 原角色永久入口。
	Day       string    `bson:"operating_day"`    // 实际播放完成的上海日。
	Seconds   uint32    `bson:"granted_seconds"`  // 原广告实际到账秒数。
	Before    int64     `bson:"before_seconds"`   // 资产事务读取的原余额。
	Balance   int64     `bson:"balance_seconds"`  // 原发放完成时的余额。
	GrantedAt time.Time `bson:"granted_at"`       // 原发放可被外部观察的时刻。
}

// Proto 只向原角色返回必要的到账信息。
func (r *RewardedAdGrantReceipt) Proto() *pb.RewardedAdReceipt {
	if r == nil {
		return nil
	}
	return &pb.RewardedAdReceipt{AttemptId: domain.Bytes(r.Attempt), GrantedSeconds: r.Seconds, BalanceSeconds: r.Balance, OperatingDay: r.Day}
}

// NextRewardedAdGrant 在原运行服的可领取队列按核验时间和凭据排序。
func (s *Store) NextRewardedAdGrant(ctx context.Context, after RewardedAdAttempt) (RewardedAdAttempt, error) {
	ctx, stop := context.WithTimeout(ctx, transactionTimeout)
	defer stop()
	origins, err := s.externalDeliveryOrigins(ctx)
	if err != nil || len(origins) == 0 {
		return RewardedAdAttempt{}, err
	}
	filter := bson.M{"stage": RewardedAdVerified, "origin_server_id": bson.M{"$in": origins}}
	if after.ID != "" {
		if !lowerHex(after.ID, 16) || after.VerifiedAt.IsZero() {
			return RewardedAdAttempt{}, Fail(p.CodeInvalidArgument, "广告发放扫描游标无效")
		}
		filter["$or"] = bson.A{bson.M{"verified_at": bson.M{"$gt": after.VerifiedAt}},
			bson.M{"verified_at": after.VerifiedAt, "_id": bson.M{"$gt": after.ID}}}
	}
	var attempt RewardedAdAttempt
	err = s.DB.Collection(rewardedAdAttempts).FindOne(ctx, filter,
		options.FindOne().SetSort(bson.D{{Key: "verified_at", Value: 1}, {Key: "_id", Value: 1}})).Decode(&attempt)
	if errors.Is(err, mongo.ErrNoDocuments) {
		err = nil
	}
	return attempt, err
}

// FulfillRewardedAd 由原角色分片调用，支持冷角色、离线托管和未知提交之后重查。
func (s *Store) FulfillRewardedAd(ctx context.Context, id string) (domain.Role, Operation, error) {
	ctx, stop := context.WithTimeout(ctx, transactionTimeout)
	defer stop()
	if s.Cfg.RunID == 0 || s.Epoch <= 0 || s.runGuardSlots == nil {
		return domain.Role{}, Operation{}, Fail(p.CodeRouteChanged, "广告奖励发放需要当前游戏运行服")
	}
	initial, err := s.RewardedAdAttempt(ctx, id)
	if err != nil {
		return domain.Role{}, Operation{}, err
	}
	var out domain.Role
	var op Operation
	err = s.guardedTx(ctx, Auth{AccountID: initial.AccountID}, func(tc context.Context) error {
		current, err := s.RewardedAdAttempt(tc, id)
		if err != nil {
			return err
		}
		if current.Expected() != initial.Expected() || current.Proof == nil || current.OperatingDay != shanghaiOperatingDay(current.Proof.CompletedAt) ||
			current.ResultCode != 0 || current.Rules.Seconds == 0 || (current.Stage != RewardedAdVerified && current.Stage != RewardedAdGranted) {
			return Fail(p.CodeOperationConflict, "广告奖励缺少原核验或已失去资格")
		}
		fence, err := s.externalDeliveryFence(tc, current.OriginServerID, current.AccountID)
		if err != nil {
			return err
		}
		role, err := s.Role(tc, current.RoleID)
		if err != nil {
			return err
		}
		if role.AccountID != current.AccountID || role.Origin != current.OriginServerID {
			return Fail(p.CodeOperationConflict, "广告奖励与原角色归属不一致")
		}
		opID := "rewarded_ad:" + current.ID
		if current.Stage == RewardedAdGranted {
			out = role
			err := s.DB.Collection("operations").FindOne(tc, bson.M{"_id": opID, "role_id": role.ID, "kind": rewardedAdGrantKind}).Decode(&op)
			if err != nil {
				return err
			}
			if op.RewardedAd == nil || current.Receipt == nil || *op.RewardedAd != *current.Receipt || op.RewardedAd.Event != current.Proof.EventKey {
				return Fail(p.CodeOperationConflict, "原广告发放回执不一致")
			}
			return nil
		}
		balance, err := add(role.HostingSeconds, int64(current.Rules.Seconds))
		if err != nil || role.HostingSeconds < 0 || balance > math.MaxUint32 || role.Revision == math.MaxInt64 {
			return Fail(p.CodeOperationConflict, "托管余额或角色修订超出当前协议范围")
		}
		at := bossTime(time.Now())
		grant := &RewardedAdGrantReceipt{Attempt: current.ID, Event: current.Proof.EventKey, Account: role.AccountID, Origin: role.Origin,
			Day: current.OperatingDay, Seconds: current.Rules.Seconds, Before: role.HostingSeconds, Balance: balance, GrantedAt: at}
		next := role
		next.HostingSeconds, next.Revision = balance, role.Revision+1
		op = Operation{ID: opID, Digest: Hash([]byte(current.Proof.EventKey)), RoleID: role.ID, Revision: next.Revision,
			CommandSeq: role.EconomicSeq, SourceSeq: role.SourceSeq, Kind: rewardedAdGrantKind, Created: at, RewardedAd: grant}
		audit := bson.M{"_id": opID, "kind": rewardedAdGrantKind, "role_id": role.ID, "account_id": role.AccountID,
			"origin_server_id": role.Origin, "run_server_id": s.Cfg.RunID, "at": at, "rewarded_ad": grant,
			"rule_version": current.RuleVersion, "config_hash": current.ConfigHash, "revision": next.Revision}
		if err := s.writeSettlement(tc, fence, next, role.Revision, op, audit); err != nil {
			return err
		}
		result, err := s.DB.Collection(rewardedAdAttempts).UpdateOne(tc, bson.M{"_id": id, "stage": RewardedAdVerified},
			bson.M{"$set": bson.M{"stage": RewardedAdGranted, "receipt": grant}})
		if err == nil && result.MatchedCount != 1 {
			err = Fail(p.CodeOperationConflict, "待发放的广告事件已变更")
		}
		out = next
		return err
	})
	return out, op, err
}
