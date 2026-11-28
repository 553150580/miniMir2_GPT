// 本文件把当前账号额度、原视频凭据和已到账结果投影到只读面板。
package persistence

import (
	"context"
	"errors"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/domain"
	p "minimir/internal/protocol"
)

// ReadRewardedAd 查询原角色最新结果或明确指定的历史回执，不产生新申请或到账。
func (s *Store) ReadRewardedAd(ctx context.Context, auth Auth, role *pb.RoleKey, id string, playable bool, at time.Time) (*pb.RewardedAdState, error) {
	rules := s.Cfg.Fixture.RewardedAd
	if rules == nil {
		return nil, Fail(p.CodeRewardedAdUnavailable, "激励视频当前未开放")
	}
	r, err := s.adAttemptOwner(ctx, auth, role)
	if err != nil {
		return nil, err
	}
	var attempt RewardedAdAttempt
	if id != "" {
		attempt, err = s.RewardedAdAttempt(ctx, id)
	} else {
		err = s.DB.Collection(rewardedAdAttempts).FindOne(ctx, bson.M{"role_id": r.ID, "account_id": r.AccountID, "origin_server_id": r.Origin},
			options.FindOne().SetSort(bson.D{{Key: "issued_at", Value: -1}, {Key: "_id", Value: -1}})).Decode(&attempt)
	}
	if err != nil && !errors.Is(err, mongo.ErrNoDocuments) {
		return nil, err
	}
	if id != "" && attempt.ID == "" {
		return nil, Fail(p.CodeRewardedAdExpired, "观看凭据已过期或不存在")
	}
	if attempt.ID != "" && (attempt.RoleID != r.ID || attempt.AccountID != r.AccountID || attempt.OriginServerID != r.Origin) {
		return nil, Fail(p.CodeUnauthenticated, "观看凭据属于其他角色")
	}
	day := shanghaiOperatingDay(at)
	used, err := s.rewardedAdQuotaUsed(ctx, "account", r.AccountID, day)
	if err != nil {
		return nil, err
	}
	active, err := s.activeRewardedAd(ctx, r.AccountID, at)
	if err != nil {
		return nil, err
	}
	state := &pb.RewardedAdState{Role: role, OperatingDay: day, RewardSeconds: rules.Seconds, AccountDailyLimit: rules.AccountCap,
		AccountRemaining: rules.AccountCap - min(used, rules.AccountCap), ChannelAvailable: playable,
		OtherRolePending: active.ID != "" && active.RoleID != r.ID}
	if attempt.ID != "" {
		state.AttemptId, state.Stage, state.ResultCode = domain.Bytes(attempt.ID), attempt.Stage, attempt.ResultCode
		state.Provider, state.Placement = attempt.Rules.Provider, attempt.Rules.Placement
		state.Beneficiary = attempt.Expected().BeneficiaryReference()
		state.WatchUntilUnixMs, state.CallbackUntilUnixMs = attempt.WatchEnd.UnixMilli(), attempt.CallbackEnd.UnixMilli()
		state.Receipt = attempt.Receipt.Proto()
		if attempt.Stage == RewardedAdAwaiting && !at.Before(attempt.CallbackEnd) {
			state.Stage, state.ResultCode = RewardedAdExpired, p.CodeRewardedAdExpired
		}
	}
	return state, nil
}
