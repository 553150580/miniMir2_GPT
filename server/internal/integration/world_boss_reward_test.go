// 本文件使用真实 MongoDB 和 WebSocket 验证世界首领击杀后的独立待领、绑定与重放。
package integration

import (
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
	"google.golang.org/protobuf/proto"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/domain"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
)

// TestSharedWorldBossRewardsMongoWS 真实共同击杀 5 点生命的首领，再从原角色待领领取各自冻结的附件。
func TestSharedWorldBossRewardsMongoWS(t *testing.T) {
	trial := prepareWorldBossRewardTrial(t, true)
	ctx, store := trial.ctx, trial.store
	first, second := trial.winner.client, trial.partner.client
	firstReward := trial.winner.reward
	// 他人的附件编号不能转成自己可领的请求，也不能改变他人的原奖。
	foreign := second.asset(p.MsgC2SClaimChallengeReward, &pb.ClaimChallengeReward{RewardId: firstReward.RewardId, Operation: second.operation()})
	if foreign.ErrorCode != p.CodeOperationConflict {
		t.Fatal("角色可领取他人的永久附件", foreign)
	}
	second.inventoryAt(foreign.NewRevision)
	full := first.asset(p.MsgC2SClaimChallengeReward, &pb.ClaimChallengeReward{RewardId: firstReward.RewardId, Operation: first.operation()})
	first.inventoryAt(full.NewRevision)
	retained := reconnectBossInfo(t, first).GetWorldPendingRewards()
	if full.ErrorCode != p.CodeEquipmentBagFull || len(retained) != 1 || !proto.Equal(retained[0], firstReward) {
		t.Fatal("装不下整批奖励时未原样保留附件", full)
	}
	// 关闭原连接后领同一份奖包，期间真实分解一件原背包装备腾出一个格。
	first.close()
	first = trial.attach(t, &trial.winner, false)
	if rewards := reconnectBossInfo(t, first).WorldPendingRewards; len(rewards) != 1 || !proto.Equal(rewards[0], firstReward) {
		t.Fatal("掉线后分配了新的奖励实例", rewards)
	}
	var cloth []byte
	for _, item := range first.inventory.Items {
		if item.ItemId == 193 {
			cloth = item.InstanceId
		}
	}
	if len(cloth) != 16 {
		t.Fatal("满包测试需要原库存里的布衣")
	}
	cleared := first.asset(p.MsgC2SRecycle, &pb.RecycleRequest{ItemInstanceIds: [][]byte{cloth}, Operation: first.operation()})
	if cleared.ErrorCode != 0 {
		t.Fatal("未能正常清出领奖空间", cleared)
	}
	first.inventoryAt(cleared.NewRevision)
	claim := &pb.ClaimChallengeReward{RewardId: firstReward.RewardId, Operation: first.operation()}
	accepted := first.asset(p.MsgC2SClaimChallengeReward, claim)
	first.inventoryAt(accepted.NewRevision)
	if accepted.ErrorCode != 0 || len(reconnectBossInfo(t, first).WorldPendingRewards) != 0 || first.asset(p.MsgC2SClaimChallengeReward, claim).ErrorCode != 0 {
		t.Fatal("清包后未确认为同一笔可重查的领取", accepted)
	}
	var receipt persistence.Operation
	roleID := domain.Hex(trial.winner.summary.Key.RoleId)
	must(t, store.DB.Collection("operations").FindOne(ctx, bson.M{"_id": "asset:" + roleID + ":" + domain.Hex(claim.Operation.OpId)}).Decode(&receipt))
	if receipt.ChallengeReward == nil || receipt.ChallengeReward.RewardID != domain.Hex(firstReward.RewardId) || len(receipt.ChallengeReward.Items) != 2 {
		t.Fatal("未原样持久化领取及回执", receipt)
	}
	stored, err := store.Role(ctx, roleID)
	must(t, err)
	for i, original := range firstReward.Items {
		instances := 0
		for _, item := range stored.Items {
			if item.ID == domain.Hex(original.InstanceId) && item.Quantity == original.Quantity && item.Bound == original.Bound {
				instances++
			}
		}
		if instances != 1 || !proto.Equal(receipt.ChallengeReward.Items[i].Proto(), original) {
			t.Fatal("原确认附件未按原实例、数量和绑定只到账一次", original, instances)
		}
	}
}
