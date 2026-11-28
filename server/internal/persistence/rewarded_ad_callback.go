// 本文件记录可信广告回调、上海运营日和原子配额结果。
package persistence

import (
	"context"
	"errors"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	p "minimir/internal/protocol"
	"minimir/internal/rewardedad"
)

const rewardedAdQuotas = "rewarded_ad_quotas" // 按运营日和受限身份隔离的计数。

// rewardedAdQuotaID 把原维度和上海运营日固定到唯一主键。
func rewardedAdQuotaID(scope, id, day string) string { return scope + ":" + id + ":" + day }

// rewardedAdQuotaUsed 返回当日该身份真实已经确认的广告数。
func (s *Store) rewardedAdQuotaUsed(ctx context.Context, scope, id, day string) (uint32, error) {
	var row struct {
		Used uint32 `bson:"used"` // 原日已经分配的成功次数。
	}
	err := s.DB.Collection(rewardedAdQuotas).FindOne(ctx, bson.M{"_id": rewardedAdQuotaID(scope, id, day)}).Decode(&row)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return 0, nil
	}
	return row.Used, err
}

// consumeRewardedAdQuotas 先检查所有维度，只有全部满足才在同一事务增加次数。
func (s *Store) consumeRewardedAdQuotas(ctx context.Context, attempt RewardedAdAttempt, proof rewardedad.Facts, day string) (uint32, error) {
	type quota struct {
		scope string // 记账维度。
		key   string // 该维度原身份，设备和网络只写摘要。
		limit uint32 // 签票时冻结的该维度上限。
		code  uint32 // 可向本账号显示的额度结果。
	}
	checks := [...]quota{
		{"account", attempt.AccountID, attempt.Rules.AccountCap, p.CodeRewardedAdAccountLimit},
		{"device", proof.DeviceKey, attempt.Rules.DeviceCap, p.CodeRewardedAdDeviceLimit},
		{"ip", proof.NetworkKey, attempt.Rules.NetworkCap, p.CodeRewardedAdNetworkLimit},
	}
	for _, row := range checks {
		used, err := s.rewardedAdQuotaUsed(ctx, row.scope, row.key, day)
		if err != nil {
			return 0, err
		}
		if used >= row.limit {
			return row.code, nil
		}
	}
	local := proof.CompletedAt.In(shanghaiOperatingZone)
	// 三个上海日后回收计数。合法回调最迟在同次观看窗口与次日确认期内送达。
	purge := time.Date(local.Year(), local.Month(), local.Day()+3, 0, 0, 0, 0, shanghaiOperatingZone)
	for _, row := range checks {
		_, err := s.DB.Collection(rewardedAdQuotas).UpdateByID(ctx, rewardedAdQuotaID(row.scope, row.key, day), bson.M{
			"$setOnInsert": bson.M{"purge_at": purge}, "$inc": bson.M{"used": 1},
		}, options.UpdateOne().SetUpsert(true))
		if err != nil {
			return 0, err
		}
	}
	return 0, nil
}

// RecordRewardedAd 保存已验证的渠道事件，结束临时凭据并预约原游戏服发奖。
func (s *Store) RecordRewardedAd(ctx context.Context, id string, verified rewardedad.Verified, at time.Time) (RewardedAdAttempt, error) {
	at = bossTime(at)
	ctx, cancel := context.WithTimeout(ctx, transactionTimeout)
	defer cancel()
	var attempt RewardedAdAttempt // 提交成功后的原核验结果。
	var err error
	// 首次同时建立共享设备或 IP 计数可能遇到唯一键竞争。重新读原事件与水位后裁决。
	for range 3 {
		err = s.tx(ctx, func(tc context.Context) error {
			current, err := s.RewardedAdAttempt(tc, id)
			if err != nil {
				return err
			}
			proof, matches := verified.For(current.Expected())
			if !matches {
				return Fail(p.CodeOperationConflict, "广告证明不属于原观看凭据")
			}
			if current.Stage != RewardedAdAwaiting {
				if current.Proof == nil || *current.Proof != proof {
					return Fail(p.CodeOperationConflict, "本观看凭据已绑定不同的完成事件")
				}
				attempt = current
				return nil
			}
			if !at.Before(current.CallbackEnd) {
				return Fail(p.CodeRewardedAdExpired, "渠道证明抵达时原观看凭据已过期")
			}
			// 与原账号的新观看申请串行，防止换角色叠加同一时段的待核验凭据。
			guard, err := s.DB.Collection("accounts").UpdateByID(tc, current.AccountID, bson.M{"$inc": bson.M{"auth_guard_seq": 1}})
			if err != nil {
				return err
			}
			if guard.MatchedCount != 1 {
				return Fail(p.CodeOperationConflict, "激励视频原账号不存在")
			}
			collection := s.DB.Collection(rewardedAdAttempts)
			used := collection.FindOne(tc, bson.M{"proof.event_key": proof.EventKey}).Err()
			if used == nil {
				return Fail(p.CodeOperationConflict, "该渠道事件已核验给其他观看凭据")
			}
			if !errors.Is(used, mongo.ErrNoDocuments) {
				return used
			}
			current.Proof, current.VerifiedAt, current.PurgeAt = &proof, at, time.Time{}
			current.OperatingDay = shanghaiOperatingDay(proof.CompletedAt)
			current.ResultCode, err = s.consumeRewardedAdQuotas(tc, current, proof, current.OperatingDay)
			if err != nil {
				return err
			}
			current.Stage = RewardedAdVerified
			if current.ResultCode != 0 {
				current.Stage = RewardedAdDenied
			}
			result, err := collection.ReplaceOne(tc, bson.M{"_id": id, "stage": RewardedAdAwaiting}, current)
			if err != nil {
				return err
			}
			if result.MatchedCount != 1 {
				return Fail(p.CodeOperationConflict, "原观看凭据已确认")
			}
			attempt = current
			return nil
		})
		if !mongo.IsDuplicateKeyError(err) {
			break
		}
	}
	return attempt, err
}
