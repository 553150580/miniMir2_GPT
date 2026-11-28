// 本文件保存激励视频的原观看请求、固定条款和永久核验结果。
package persistence

import (
	"context"
	"errors"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	p "minimir/internal/protocol"
	"minimir/internal/rewardedad"
)

const rewardedAdAttempts = "rewarded_ad_attempts" // 未核验的临时凭据与已核验永久事件使用原身份。

const (
	RewardedAdAwaiting uint32 = iota + 1 // 等待可信渠道证明。
	RewardedAdVerified                   // 已核验且三种日限量均已记入。
	RewardedAdGranted                    // 原角色托管时长事务已确认。
	RewardedAdDenied                     // 播放已核验但达到某类运营日上限。
	RewardedAdExpired                    // 只用于展示，原临时凭据到期后由索引回收。
)

// RewardedAdAttempt 同时固定原角色、期限、奖励秒数和唯一核验事实。
type RewardedAdAttempt struct {
	ID             string                    `bson:"_id"`                     // 由原账号和客户端请求号导出的唯一凭据。
	AccountID      string                    `bson:"account_id"`              // 同一账号的不同角色共用限量。
	RoleID         string                    `bson:"role_id"`                 // 唯一获得此奖励的原角色。
	OriginServerID uint64                    `bson:"origin_server_id"`        // 角色永久入口，合服后仍沿用。
	Rules          devconfig.RewardedAdRules `bson:"rules"`                   // 申请观看时冻结的渠道、奖励和限量。
	RuleVersion    uint64                    `bson:"rule_version"`            // 原玩法版本。
	ConfigHash     string                    `bson:"config_hash"`             // 原规则摘要。
	IssuedAt       time.Time                 `bson:"issued_at"`               // 签发时刻。
	WatchEnd       time.Time                 `bson:"watch_until"`             // 渠道播放必须在此之前完成。
	CallbackEnd    time.Time                 `bson:"callback_until"`          // 新证明最晚到达时刻，不含端点。
	PurgeAt        time.Time                 `bson:"purge_at,omitempty"`      // 只有尚未核验的临时凭据保留回收时间。
	Stage          uint32                    `bson:"stage"`                   // 当前生命周期阶段。
	Proof          *rewardedad.Facts         `bson:"proof,omitempty"`         // 已核验完播的原证明及隐私摘要。
	VerifiedAt     time.Time                 `bson:"verified_at,omitempty"`   // 按此在原运行服排队投递。
	OperatingDay   string                    `bson:"operating_day,omitempty"` // 真实完成时刻对应的上海日。
	ResultCode     uint32                    `bson:"result_code,omitempty"`   // 某类配额用尽时保留原决定。
	Receipt        *RewardedAdGrantReceipt   `bson:"receipt,omitempty"`       // 已按同一事件实际增加的时长。
}

// Expected 只从持久凭据读取可信渠道的角色绑定和播放窗口。
func (a RewardedAdAttempt) Expected() rewardedad.Expected {
	return rewardedad.Expected{AttemptID: a.ID, AccountID: a.AccountID, RoleID: a.RoleID, Origin: a.OriginServerID,
		Provider: a.Rules.Provider, Placement: a.Rules.Placement, IssuedAt: a.IssuedAt, WatchEnd: a.WatchEnd}
}

// RewardedAdAttempt 由可信回调入口读取原观看条款，玩家查询另做账号与角色核对。
func (s *Store) RewardedAdAttempt(ctx context.Context, id string) (RewardedAdAttempt, error) {
	var attempt RewardedAdAttempt
	if !lowerHex(id, 16) {
		return attempt, Fail(p.CodeInvalidArgument, "观看凭据身份无效")
	}
	err := s.DB.Collection(rewardedAdAttempts).FindOne(ctx, bson.M{"_id": id}).Decode(&attempt)
	return attempt, err
}

// rewardedAdAttemptIndexes 只清理未核验的原凭据，永久事件按渠道身份去重。
func rewardedAdAttemptIndexes() []mongo.IndexModel {
	return []mongo.IndexModel{
		{Keys: bson.D{{Key: "purge_at", Value: 1}}, Options: options.Index().SetExpireAfterSeconds(0)},
		{Keys: bson.D{{Key: "proof.event_key", Value: 1}}, Options: options.Index().SetName("rewarded_ad_provider_event").SetUnique(true).
			SetPartialFilterExpression(bson.M{"proof.event_key": bson.M{"$type": "string"}})},
		{Keys: bson.D{{Key: "account_id", Value: 1}, {Key: "stage", Value: 1}, {Key: "callback_until", Value: 1}},
			Options: options.Index().SetName("rewarded_ad_account_pending")},
		{Keys: bson.D{{Key: "role_id", Value: 1}, {Key: "issued_at", Value: -1}, {Key: "_id", Value: -1}},
			Options: options.Index().SetName("rewarded_ad_role_history")},
		{Keys: bson.D{{Key: "stage", Value: 1}, {Key: "origin_server_id", Value: 1}, {Key: "verified_at", Value: 1}, {Key: "_id", Value: 1}},
			Options: options.Index().SetName("rewarded_ad_pending_delivery")},
	}
}

// adAttemptOwner 校验玩家真实拥有的角色和永久入口，不以客户端声明的账号决定受益人。
func (s *Store) adAttemptOwner(ctx context.Context, auth Auth, role *pb.RoleKey) (domain.Role, error) {
	if role == nil || len(role.RoleId) != 16 || role.OriginServerId == 0 {
		return domain.Role{}, Fail(p.CodeInvalidArgument, "激励视频需要选择原账号角色")
	}
	r, err := s.Role(ctx, domain.Hex(role.RoleId))
	if err != nil || r.AccountID != auth.AccountID || r.Origin != role.OriginServerId {
		if err != nil && !errors.Is(err, mongo.ErrNoDocuments) {
			return domain.Role{}, err
		}
		return domain.Role{}, Fail(p.CodeUnauthenticated, "激励视频角色不属于原账号与入口服")
	}
	return r, nil
}

// activeRewardedAd 查原账号仍在观看窗口内，或已通过而尚待原服发放的事件。
func (s *Store) activeRewardedAd(ctx context.Context, account string, at time.Time) (RewardedAdAttempt, error) {
	var active RewardedAdAttempt
	filter := bson.M{"account_id": account, "$or": bson.A{
		bson.M{"stage": RewardedAdAwaiting, "callback_until": bson.M{"$gt": at}}, bson.M{"stage": RewardedAdVerified}}}
	err := s.DB.Collection(rewardedAdAttempts).FindOne(ctx, filter).Decode(&active)
	if errors.Is(err, mongo.ErrNoDocuments) {
		err = nil
	}
	return active, err
}

// StartRewardedAd 幂等签发新观看凭据。同一账号等待前一笔原核验时继续沿用原角色。
func (s *Store) StartRewardedAd(ctx context.Context, auth Auth, role *pb.RoleKey, request []byte, at time.Time) (RewardedAdAttempt, error) {
	rules := s.Cfg.Fixture.RewardedAd
	if rules == nil {
		return RewardedAdAttempt{}, Fail(p.CodeRewardedAdUnavailable, "尚无激励视频装配")
	}
	if len(request) != 16 {
		return RewardedAdAttempt{}, Fail(p.CodeInvalidArgument, "缺少原观看请求编号")
	}
	id := Hash([]byte("minimir:rewarded-ad:attempt:v1:" + auth.AccountID + ":" + domain.Hex(request)))[:32]
	var attempt RewardedAdAttempt
	at = bossTime(at)
	err := s.tx(ctx, func(tc context.Context) error {
		if err := s.accountGuard(tc, auth); err != nil {
			return err
		}
		r, err := s.adAttemptOwner(tc, auth, role)
		if err != nil {
			return err
		}
		attempt, err = s.RewardedAdAttempt(tc, id)
		if err == nil {
			if attempt.AccountID != r.AccountID || attempt.RoleID != r.ID || attempt.OriginServerID != r.Origin {
				return Fail(p.CodeOperationConflict, "原视频请求绑定在其他角色")
			}
			return nil
		}
		if !errors.Is(err, mongo.ErrNoDocuments) {
			return err
		}
		active, err := s.activeRewardedAd(tc, auth.AccountID, at)
		if err != nil {
			return err
		}
		if active.ID != "" {
			if active.RoleID != r.ID || active.OriginServerID != r.Origin {
				return Fail(p.CodeRewardedAdPending, "原观看尚在其他角色上")
			}
			attempt = active
			return nil
		}
		used, err := s.rewardedAdQuotaUsed(tc, "account", auth.AccountID, shanghaiOperatingDay(at))
		if err != nil {
			return err
		}
		if used >= rules.AccountCap {
			return Fail(p.CodeRewardedAdAccountLimit, "本账号已达到上海运营日上限")
		}
		watchEnd := at.Add(time.Duration(rules.WatchSec) * time.Second)
		callbackEnd := watchEnd.Add(time.Duration(rules.CallbackSec) * time.Second)
		attempt = RewardedAdAttempt{ID: id, AccountID: auth.AccountID, RoleID: r.ID, OriginServerID: r.Origin,
			Rules: *rules, RuleVersion: s.Cfg.Fixture.RuleVersion, ConfigHash: s.Cfg.Fixture.Hash,
			IssuedAt: at, WatchEnd: watchEnd, CallbackEnd: callbackEnd, PurgeAt: callbackEnd.Add(24 * time.Hour), Stage: RewardedAdAwaiting}
		_, err = s.DB.Collection(rewardedAdAttempts).InsertOne(tc, attempt)
		return err
	})
	return attempt, err
}
