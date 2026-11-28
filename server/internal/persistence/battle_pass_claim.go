// 本文件计算原档差额并与角色背包、绑定钱包和领取回执原子结算。
package persistence

import (
	"context"
	"time"

	"google.golang.org/protobuf/proto"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	p "minimir/internal/protocol"
)

const AssetBattlePassClaim = "battle_pass_claim" // 角色主动确认的每日或周期一次奖。

// BattlePassReceipt 保存实际发放的原差额和每个绑定实例。
type BattlePassReceipt struct {
	Start        string        `bson:"season_start"`    // 首个运营日，限定原领取周期。
	Day          uint32        `bson:"day"`             // 相对日，一次奖励使用零。
	PreviousTier uint32        `bson:"previous_tier"`   // 本日或本周期原已领的最高档。
	Tier         uint32        `bson:"tier"`            // 本笔成功后已发到的最高档。
	Gold         int64         `bson:"bound_gold"`      // 本笔实际增加的绑定金币。
	Yuanbao      int64         `bson:"bound_yuanbao"`   // 本笔实际增加的绑定元宝最小单位。
	Items        []domain.Item `bson:"items,omitempty"` // 按原来源入包的材料实例。
}

// Proto 原样返回成功时的到账，重放不取角色当前余额和档位。
func (receipt *BattlePassReceipt) Proto() *pb.BattlePassReceipt {
	if receipt == nil {
		return nil
	}
	out := &pb.BattlePassReceipt{SeasonStart: receipt.Start, Day: receipt.Day, PreviousTier: receipt.PreviousTier,
		Tier: receipt.Tier, BoundGold: receipt.Gold, BoundYuanbaoUnits: receipt.Yuanbao}
	for _, item := range receipt.Items {
		out.Items = append(out.Items, item.Proto())
	}
	return out
}

// ValidBattlePassClaim 检查原日期和经济请求形状，服务器按持久资格计算奖励。
func ValidBattlePassClaim(req *pb.BattlePassClaimRequest, op *pb.OperationContext) bool {
	if req == nil || op == nil || len(op.OpId) != 16 || len(op.ProtoReflect().GetUnknown()) != 0 ||
		len(req.ProtoReflect().GetUnknown()) != 0 || !proto.Equal(req.Operation, op) || req.Day > devconfig.BattlePassDays ||
		req.Tier < devconfig.BattlePassFree || req.Tier > devconfig.BattlePassMaxTier || req.ClaimedTier >= req.Tier {
		return false
	}
	_, valid := battlePassDate(req.SeasonStart)
	return valid
}

// battlePassRewardDelta 对齐相同物品模板，升档只补双方总奖之间的差额。
func battlePassRewardDelta(tiers []devconfig.BattlePassTier, day, tier, prior uint32) (devconfig.TaskCodexReward, bool) {
	reward := tiers[tier-1].Daily
	var previous devconfig.TaskCodexReward
	if day == 0 {
		reward = tiers[tier-1].Season
	}
	if prior > 0 {
		previous = tiers[prior-1].Daily
		if day == 0 {
			previous = tiers[prior-1].Season
		}
	}
	delta := devconfig.TaskCodexReward{BoundGold: reward.BoundGold - previous.BoundGold, BoundYuanbaoUnits: reward.BoundYuanbaoUnits - previous.BoundYuanbaoUnits}
	if delta.BoundGold < 0 || delta.BoundYuanbaoUnits < 0 {
		return delta, false
	}
	priorIndex := 0 // 上一档已发物品按模板升序，对齐当前总奖。
	for _, grant := range reward.Items {
		if priorIndex < len(previous.Items) && previous.Items[priorIndex].ItemID == grant.ItemID {
			grant.Quantity -= previous.Items[priorIndex].Quantity
			priorIndex++
		}
		if grant.Quantity < 0 {
			return delta, false
		}
		if grant.Quantity > 0 {
			delta.Items = append(delta.Items, grant)
		}
	}
	return delta, priorIndex == len(previous.Items)
}

// claimBattlePass 从真实资格和原总奖发货，拒绝猜测日期、陈旧档位或容量不足。
func (s *Store) claimBattlePass(ctx context.Context, r domain.Role, q AssetRequest, at time.Time, opID string) (domain.Role, *BattlePassReceipt, uint32, error) {
	req := q.BattlePass
	if !ValidBattlePassClaim(req, q.Context) {
		return r, nil, p.CodeInvalidArgument, nil
	}
	if q.BattlePassGate || s.Cfg.Fixture.BattlePass == nil {
		return r, nil, p.CodeBattlePassUnavailable, nil
	}
	state, err := s.loadBattlePassBoard(ctx, r, req.SeasonStart, at)
	if err != nil {
		if code := Code(err); code == p.CodeBattlePassUnavailable {
			return r, nil, code, nil
		}
		return r, nil, 0, err
	}
	previous := state.tierClaimed(req.Day)
	if req.Tier != state.Tier || req.ClaimedTier != previous || previous == state.Tier {
		return r, nil, p.CodeBattlePassChanged, nil
	}
	if req.Day != 0 && state.Eligible&battlePassMask(req.Day) == 0 {
		return r, nil, p.CodeBattlePassUnavailable, nil
	}
	delta, valid := battlePassRewardDelta(state.Tiers, req.Day, state.Tier, previous)
	if !valid {
		return r, nil, p.CodeOperationConflict, nil
	}
	if len(delta.Items) == 0 && delta.BoundGold == 0 && delta.BoundYuanbaoUnits == 0 {
		return r, nil, p.CodeBattlePassUnavailable, nil
	}
	next, items, code := grantBoundRewards(r, &s.Cfg.Fixture, delta, "battle_pass", opID)
	if code != 0 {
		return r, nil, code, nil
	}
	state.setClaimed(req.Day, state.Tier)
	if err := s.persistBattlePassSeason(ctx, state); err != nil {
		return r, nil, 0, err
	}
	receipt := &BattlePassReceipt{Start: req.SeasonStart, Day: req.Day, PreviousTier: previous, Tier: state.Tier,
		Gold: delta.BoundGold, Yuanbao: delta.BoundYuanbaoUnits, Items: items}
	return next, receipt, 0, nil
}
