// 本文件核验奖表默认关闭、独立模板冻结及参与和排名的装配边界。
package devconfig

import (
	"encoding/json"
	"testing"
)

// TestWorldBossRewardsReleaseAndSnapshot 只在测试中临时装入奖表，验证每个原模板各用自己的配置。
func TestWorldBossRewardsReleaseAndSnapshot(t *testing.T) {
	cfg, err := Load("../../../conf/dev/server.json")
	if err != nil {
		t.Fatal(err)
	}
	fixture := &cfg.Fixture
	if !fixture.ValidWorldBoss() || fixture.WorldBoss.Rewards != nil || len(fixture.WorldBossVariants) != 1 || fixture.WorldBossVariants[0].Rewards != nil {
		t.Fatal("试玩模板必须保持可战斗且没有未审定的奖励表")
	}
	// 使用真实开发物品模板验证装配，数值仅为隔离测试输入。
	const candidate = `{"minimum_damage":5,"allow_disconnected":true,"allow_withdrawn":false,"allow_defeated":false,` +
		`"participation":[{"item_id":35,"quantity":1}],"ranks":[{"first_rank":1,"last_rank":2,"items":[{"item_id":35,"quantity":1}]}],` +
		`"timeout_participation":[{"item_id":35,"quantity":1}]}`
	var rules WorldBossRewards // 验证单模板的原 JSON 奖表字段名称及三类附件。
	if err := json.Unmarshal([]byte(candidate), &rules); err != nil {
		t.Fatal(err)
	}
	fixture.WorldBossRelease.Rewards = &rules
	if err := fixture.Validate(); err != nil {
		t.Fatal(err)
	}
	frozen := fixture.WorldBoss.Rewards
	if frozen == nil || frozen.MinimumDamage != 5 || !frozen.AllowDisconnected || frozen.AllowWithdrawn || frozen.AllowDefeated ||
		frozen.Participation[0].ItemID != 35 || frozen.Ranks[0].Items[0].ItemID != 35 || frozen.TimeoutParticipation[0].ItemID != 35 ||
		fixture.WorldBossVariants[0].Rewards != nil {
		t.Fatal("已装配的共享首领奖励与原模板不一致")
	}
	rules.Participation[0].ItemID = 0
	rules.Ranks[0].Items[0].ItemID = 0
	rules.TimeoutParticipation[0].ItemID = 0
	if frozen.Participation[0].ItemID == 0 || frozen.Ranks[0].Items[0].ItemID == 0 || frozen.TimeoutParticipation[0].ItemID == 0 {
		t.Fatal("原候选切片仍能更改已装配首领的附件")
	}
	fixture.WorldBossRelease.Rewards = nil
	fixture.WorldBossVariantReleases[0].Rewards = &rules
	if err := fixture.Validate(); err == nil {
		t.Fatal("轮换模板的非法奖表没有在装配时拒绝")
	}
}

// TestWorldBossRewardQualificationAndRankBounds 校验准入门槛、公开榜闭区间和固定物品的边界。
func TestWorldBossRewardQualificationAndRankBounds(t *testing.T) {
	fixture := Fixture{Items: []Item{{ID: 1, Stackable: true, MaxStack: 5}, {ID: 2, IsEquipment: true, DropQuality: 1}, {ID: 3, IsEquipment: true, DropQuality: 2}}}
	config := WorldBossConfig{ParticipantLimit: 8, Monster: Monster{Fighter: Fighter{HP: 100}}}
	valid := &WorldBossRewards{
		MinimumDamage: 5, AllowDisconnected: true, AllowWithdrawn: true, AllowDefeated: true,
		Participation: []ChallengeRewardItem{{ItemID: 1, Quantity: 5}},
		Ranks: []WorldBossRankReward{
			{FirstRank: 1, LastRank: 2, Items: []ChallengeRewardItem{{ItemID: 2, Quantity: 1}}},
			{FirstRank: 4, LastRank: 8, Items: []ChallengeRewardItem{{ItemID: 1, Quantity: 1}}},
		},
		TimeoutParticipation: []ChallengeRewardItem{{ItemID: 1, Quantity: 1}},
	}
	if !fixture.ValidWorldBossRewards(&config) {
		t.Fatal("无奖表时应允许原战斗模板")
	}
	config.Rewards = valid
	if !fixture.ValidWorldBossRewards(&config) {
		t.Fatal("隔离使用的普通材料、低品质装备和未覆盖排名档应可装配")
	}
	for _, row := range []struct {
		name   string                  // 此用例单独改变的条件。
		mutate func(*WorldBossRewards) // 只改写被测试的临时奖表。
	}{
		{"无贡献门槛", func(candidate *WorldBossRewards) { candidate.MinimumDamage = 0 }},
		{"超出首领总生命", func(candidate *WorldBossRewards) { candidate.MinimumDamage = 101 }},
		{"超时不能满足全额伤害", func(candidate *WorldBossRewards) { candidate.MinimumDamage = 100 }},
		{"没有任何奖包", func(candidate *WorldBossRewards) {
			candidate.Participation, candidate.Ranks, candidate.TimeoutParticipation = nil, nil, nil
		}},
		{"排名从零开始", func(candidate *WorldBossRewards) { candidate.Ranks[0].FirstRank = 0 }},
		{"排名档重叠", func(candidate *WorldBossRewards) { candidate.Ranks[1].FirstRank = 2 }},
		{"排名超过席位上限", func(candidate *WorldBossRewards) { candidate.Ranks[1].LastRank = 9 }},
		{"排名末名小于起始名次", func(candidate *WorldBossRewards) { candidate.Ranks[1].LastRank = 3 }},
		{"空排名附件", func(candidate *WorldBossRewards) { candidate.Ranks[0].Items = nil }},
		{"参与与排名合并超过上限", func(candidate *WorldBossRewards) {
			for len(candidate.Ranks[0].Items) < maxChallengeRewardItems {
				candidate.Ranks[0].Items = append(candidate.Ranks[0].Items, ChallengeRewardItem{ItemID: 1, Quantity: 1})
			}
		}},
		{"未知物品", func(candidate *WorldBossRewards) { candidate.Participation[0].ItemID = 4 }},
		{"超出堆叠", func(candidate *WorldBossRewards) { candidate.Participation[0].Quantity = 6 }},
		{"非堆叠多件", func(candidate *WorldBossRewards) { candidate.Ranks[0].Items[0].Quantity = 2 }},
		{"超时零数量", func(candidate *WorldBossRewards) { candidate.TimeoutParticipation[0].Quantity = 0 }},
		{"无预算的稀有装备", func(candidate *WorldBossRewards) { candidate.Ranks[0].Items[0].ItemID = 3 }},
	} {
		t.Run(row.name, func(t *testing.T) {
			config.Rewards = valid.clone()
			row.mutate(config.Rewards)
			if fixture.ValidWorldBossRewards(&config) {
				t.Fatal("无效世界首领奖励配置被放行")
			}
		})
	}
}
