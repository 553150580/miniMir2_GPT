// 本文件验证赛季隔离派奖和兑换不会改变既有夹具的启用范围。
package devconfig

import (
	"encoding/json"
	"math"
	"slices"
	"testing"
)

func mirrorSettlementFixture() Fixture {
	f := mirrorArenaFixture()
	f.DevelopmentOnly = true
	f.Items = []Item{{ID: 35, Stackable: true, MaxStack: 200}, {ID: 10, IsEquipment: true}, {ID: 9}}
	f.MirrorArenaRelease.Settlement = &MirrorArenaSeasonSettlement{
		RankingTiebreak: MirrorArenaRankingEarlierLastChallenge, ParticipationMinChallenges: 10,
		Ranks: []MirrorArenaSeasonRank{
			{FirstRank: 1, LastRank: 1, Reward: TaskCodexReward{BoundYuanbaoUnits: 17}},
			{FirstRank: 2, LastRank: 4, Reward: TaskCodexReward{BoundYuanbaoUnits: 2}},
		},
		Participation: TaskCodexReward{BoundYuanbaoUnits: 3},
	}
	f.MirrorArenaRelease.Exchange = &MirrorArenaExchangeRule{ItemID: 35, Price: 250, Quantity: 1}
	return f
}

func TestMirrorArenaSeasonSettlementResolvesDetachedCandidate(t *testing.T) {
	f := mirrorSettlementFixture()
	if err := f.ValidateMirrorArena(); err != nil {
		t.Fatal(err)
	}
	resolved := f.MirrorArena
	rules := f.MirrorArenaRelease
	rules.Settlement.Ranks[0].Reward.BoundYuanbaoUnits = 19
	rules.Settlement.Participation.BoundYuanbaoUnits = 21
	rules.Exchange.Quantity = 8
	if resolved.Settlement.Ranks[0].Reward.BoundYuanbaoUnits != 17 || resolved.Settlement.Participation.BoundYuanbaoUnits != 3 ||
		resolved.Exchange.Quantity != 1 || resolved.Exchange.Price != 250 {
		t.Fatal("已装配的赛季奖表或兑换受原夹具后续改动影响")
	}
	rules.Exchange = nil
	if err := f.ValidateMirrorArena(); err != nil || f.MirrorArena.Settlement == nil || f.MirrorArena.Exchange != nil {
		t.Fatalf("赛季封账与兑换应能分别关闭：%v", err)
	}
	rules.Settlement = nil
	if err := f.ValidateMirrorArena(); err != nil || f.MirrorArena.Settlement != nil || f.MirrorArena.Exchange != nil {
		t.Fatalf("旧镜像夹具缺省时不应切换账本或兑换：%v", err)
	}
}

func TestMirrorArenaSeasonSettlementRejectsUnsafeGrants(t *testing.T) {
	for _, mode := range []struct {
		name   string         // 被拒绝的配置变种。
		mutate func(*Fixture) // 变更单项候选，原有效装配应同时退役。
	}{
		{"正式环境", func(f *Fixture) { f.DevelopmentOnly = false }},
		{"公开就绪标记", func(f *Fixture) { f.RuntimeReady = true }},
		{"未配置结转却配置兑换", func(f *Fixture) { f.MirrorArenaRelease.Settlement = nil }},
		{"同分规则变化", func(f *Fixture) { f.MirrorArenaRelease.Settlement.RankingTiebreak = "later" }},
		{"参与门槛没有配置", func(f *Fixture) { f.MirrorArenaRelease.Settlement.ParticipationMinChallenges = 0 }},
		{"参与门槛不可达", func(f *Fixture) { f.MirrorArenaRelease.Settlement.ParticipationMinChallenges = 76 }},
		{"排名档为空", func(f *Fixture) { f.MirrorArenaRelease.Settlement.Ranks = nil }},
		{"排名档太多", func(f *Fixture) {
			rules := f.MirrorArenaRelease.Settlement
			rules.Ranks = slices.Repeat(rules.Ranks, maxMirrorSeasonRankTiers)
		}},
		{"排名档重叠", func(f *Fixture) { f.MirrorArenaRelease.Settlement.Ranks[1].FirstRank = 1 }},
		{"排名档倒序", func(f *Fixture) { f.MirrorArenaRelease.Settlement.Ranks[1].LastRank = 1 }},
		{"赛季奖超过总量", func(f *Fixture) { f.MirrorArenaRelease.Settlement.Ranks[0].Reward.BoundYuanbaoUnits = math.MaxInt64 }},
		{"参与奖为零", func(f *Fixture) { f.MirrorArenaRelease.Settlement.Participation.BoundYuanbaoUnits = 0 }},
		{"参与奖含金币", func(f *Fixture) { f.MirrorArenaRelease.Settlement.Participation.BoundGold = 3 }},
		{"排名奖含普通物品", func(f *Fixture) {
			f.MirrorArenaRelease.Settlement.Ranks[0].Reward.Items = []TaskCodexRewardItem{{ItemID: 35, Quantity: 1}}
		}},
		{"兑换价格为零", func(f *Fixture) { f.MirrorArenaRelease.Exchange.Price = 0 }},
		{"兑换不存在的物品", func(f *Fixture) { f.MirrorArenaRelease.Exchange.ItemID = 999 }},
		{"兑换装备", func(f *Fixture) { f.MirrorArenaRelease.Exchange.ItemID = 10 }},
		{"兑换不可堆叠物品", func(f *Fixture) { f.MirrorArenaRelease.Exchange.ItemID = 9 }},
		{"超过实际堆叠", func(f *Fixture) { f.MirrorArenaRelease.Exchange.Quantity = 201 }},
	} {
		t.Run(mode.name, func(t *testing.T) {
			f := mirrorSettlementFixture()
			if err := f.ValidateMirrorArena(); err != nil {
				t.Fatal(err)
			}
			mode.mutate(&f)
			if err := f.ValidateMirrorArena(); err == nil || f.MirrorArena != nil {
				t.Fatalf("不合法的赛季装配得到保留：%v", err)
			}
		})
	}
}

func TestMirrorArenaSeasonSettlementJSONRejectsUnsupportedFields(t *testing.T) {
	for _, raw := range []string{
		`{"settlement":{"participation":{"bound_yuanbao_units":3,"title_id":1}}}`,
		`{"exchange":{"item_id":35,"price":250,"quantity":1,"bound":false}}`,
	} {
		var config MirrorArenaDefinition
		if err := json.Unmarshal([]byte(raw), &config); err == nil {
			t.Fatalf("应报告未启用的镜像奖励字段：%s", raw)
		}
	}
}
