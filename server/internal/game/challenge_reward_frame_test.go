// 本文件验证三类挑战奖励同时分页返回时仍满足 WebSocket 完整包上限。
package game

import (
	"math"
	"strings"
	"testing"

	pb "minimir/api/generated/minimirv1"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
	"minimir/internal/wire"
)

func TestChallengeRewardThreeFullPagesFitWire(t *testing.T) {
	bundle := &pb.ChallengePendingReward{RewardId: make([]byte, 16), TemplateId: math.MaxUint32,
		EarnedAtUnixMs: math.MaxInt64, SourceKind: "world_boss_timeout", SourceRank: math.MaxUint32}
	item := &pb.ItemView{InstanceId: make([]byte, 16), ItemId: math.MaxUint32, Quantity: math.MaxInt64, Bound: true, Version: math.MaxInt64,
		// 三类来源都使用原32字符挑战键；这里扩大其它字段来覆盖长整数及包头。
		SourceKind: "world_boss_timeout", SourceId: strings.Repeat("0", 32), Quality: math.MaxUint32, Locked: true, RecycleBlockFlags: math.MaxUint32,
		DurabilityU: math.MaxInt64, MaxDurabilityU: math.MaxInt64, OriginalDurabilityU: math.MaxInt64}
	for range 8 {
		bundle.Items = append(bundle.Items, item)
	}
	info := &pb.PersonalBossInfo{Open: true, DisplayName: "挑战", WorldPendingRewardsMore: true, DungeonPendingRewardsMore: true}
	for range 20 {
		info.PendingRewards = append(info.PendingRewards, bundle)
		info.WorldPendingRewards = append(info.WorldPendingRewards, bundle)
		info.History = append(info.History, &pb.PersonalBossHistoryEntry{ChallengeId: strings.Repeat("0", 32), Damage: math.MaxInt64,
			FinishedAtUnixMs: math.MaxInt64, RewardGold: math.MaxInt64})
	}
	for range persistence.DungeonPendingRewardPageSize {
		info.DungeonPendingRewards = append(info.DungeonPendingRewards, bundle)
	}
	frame := wire.Encode(wire.Message(p.MsgS2CPersonalBossInfo, math.MaxUint32, math.MaxInt64, info))
	if len(frame) >= wire.MaxFrame {
		t.Fatalf("挑战奖励三个有界首屏的真实封包过大：%d / %d", len(frame), wire.MaxFrame)
	}
}
