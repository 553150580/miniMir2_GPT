// 本文件核对可选讨伐链奖表的 JSON 装配、来源判定与候选包边界。
package devconfig

import (
	"encoding/json"
	"math"
	"reflect"
	"slices"
	"strings"
	"testing"
)

// TestDungeonRewardsSnapshot 验证空包、来源策略与入场模板的装配快照。
func TestDungeonRewardsSnapshot(t *testing.T) {
	fixture := dungeonRewardFixture()
	source := fixture.DungeonReleases[0]
	options := []DungeonRewardOption{
		{Weight: 3, Items: []DungeonRewardItem{{ItemID: 1, Quantity: 1, BindPolicy: DungeonRewardBindOnKill},
			{ItemID: 2, Quantity: 4, BindPolicy: DungeonRewardAlwaysBound}}},
		{Weight: math.MaxUint32 - 3, Items: []DungeonRewardItem{}},
	}
	source.Rewards = &DungeonRewards{Options: options}
	wire, err := json.Marshal(source)
	if err != nil {
		t.Fatal(err)
	}
	var decoded DungeonDefinition
	if err = json.Unmarshal(wire, &decoded); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(wire), `"rewards":{"options":[{"weight":3,"items":[{"item_id":1,"quantity":1,"bind_policy":"kill_privilege"}`) ||
		!strings.Contains(string(wire), `"bind_policy":"always_bound"`) || !strings.Contains(string(wire), `"items":[]`) {
		t.Fatalf("奖表未投影为约定的字段及绑定枚举：%s", wire)
	}
	noReward := source
	noReward.TemplateID++
	noReward.Map.ID++
	noReward.Rewards = nil
	closed := decoded
	closed.Enabled = false
	closed.Rewards = &DungeonRewards{}
	fixture.DungeonReleases = []DungeonDefinition{decoded, noReward, closed}
	if err = fixture.validateDungeonReleases(); err != nil {
		t.Fatal(err)
	}
	snapshot, found := fixture.DungeonTemplateByID(source.TemplateID)
	unrewarded, foundUnrewarded := fixture.DungeonTemplateByID(noReward.TemplateID)
	if !found || !foundUnrewarded || !reflect.DeepEqual(snapshot.Rewards.Options, options) || unrewarded.Rewards != nil || len(fixture.Dungeons) != 2 {
		t.Fatal("装配遗漏了原始奖表、明确空包或兼容的无奖模板")
	}
	decoded.Rewards.Options[0].Weight++
	decoded.Rewards.Options[0].Items[0].ItemID++
	if !reflect.DeepEqual(snapshot.Rewards.Options, options) {
		t.Fatal("修改装配前奖表后，运行中的选项权重或附件随之改变")
	}
}

// TestDungeonRewardsRejectInvalidRelease 确认一张不合法的奖表不会部分发布其它模板。
func TestDungeonRewardsRejectInvalidRelease(t *testing.T) {
	tests := []struct {
		name   string                          // 对应的失败边界。
		change func(*DungeonRewards, *Fixture) // 改动独立副本以触发该错误。
	}{
		{"无选项", func(r *DungeonRewards, _ *Fixture) { r.Options = nil }},
		{"全为空包", func(r *DungeonRewards, _ *Fixture) { r.Options[0].Items = nil }},
		{"选项超限", func(r *DungeonRewards, _ *Fixture) { r.Options = slices.Repeat(r.Options, maxDungeonRewardOptions+1) }},
		{"零权重", func(r *DungeonRewards, _ *Fixture) { r.Options[0].Weight = 0 }},
		{"总权重超限", func(r *DungeonRewards, _ *Fixture) {
			r.Options = append(r.Options, DungeonRewardOption{Weight: math.MaxUint32})
		}},
		{"附件超限", func(r *DungeonRewards, _ *Fixture) {
			r.Options[0].Items = slices.Repeat(r.Options[0].Items, maxChallengeRewardItems+1)
		}},
		{"未知物品", func(r *DungeonRewards, _ *Fixture) { r.Options[0].Items[0].ItemID = 100 }},
		{"零物品编号", func(r *DungeonRewards, f *Fixture) {
			r.Options[0].Items[0].ItemID = 0
			f.Items = append(f.Items, Item{ID: 0})
		}},
		{"单件装备多件数量", func(r *DungeonRewards, _ *Fixture) { r.Options[0].Items[0].Quantity = 2 }},
		{"堆叠零数量", func(r *DungeonRewards, _ *Fixture) { r.Options[0].Items[1].Quantity = 0 }},
		{"堆叠超过上限", func(r *DungeonRewards, _ *Fixture) { r.Options[0].Items[1].Quantity = 5 }},
		{"未知绑定策略", func(r *DungeonRewards, _ *Fixture) { r.Options[0].Items[1].BindPolicy = "unknown" }},
		{"缺少绑定策略", func(r *DungeonRewards, _ *Fixture) { r.Options[0].Items[1].BindPolicy = "" }},
		{"掉落装备使用固定绑定", func(r *DungeonRewards, _ *Fixture) { r.Options[0].Items[0].BindPolicy = DungeonRewardAlwaysBound }},
		{"稀有装备缺少来源额度", func(r *DungeonRewards, _ *Fixture) { r.Options[0].Items[0].ItemID = 3 }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := dungeonRewardFixture()
			invalid := fixture.DungeonReleases[0]
			invalid.TemplateID++
			invalid.Map.ID++
			invalid.Rewards = &DungeonRewards{Options: []DungeonRewardOption{{Weight: 1, Items: []DungeonRewardItem{
				{ItemID: 1, Quantity: 1, BindPolicy: DungeonRewardBindOnKill},
				{ItemID: 2, Quantity: 1, BindPolicy: DungeonRewardAlwaysBound},
			}}}}
			test.change(invalid.Rewards, &fixture)
			fixture.DungeonReleases = append(fixture.DungeonReleases, invalid)
			if err := fixture.validateDungeonReleases(); err == nil || !strings.Contains(err.Error(), "讨伐链奖励") || len(fixture.Dungeons) != 0 {
				t.Fatalf("非法奖表应使整批装配失败：%v", err)
			}
		})
	}
}

// dungeonRewardFixture 提供隔离怪物与物品目录，奖表仅由测试用例添加。
func dungeonRewardFixture() Fixture {
	monster := Monster{ID: 92_001, Fighter: Fighter{HP: 24, MoveMS: 700, AttackMS: 900}}
	return Fixture{Map: MapTemplate{ID: 1}, ChallengeMonsters: []Monster{monster}, DungeonReleases: []DungeonDefinition{dungeonDefinitionForTest(monster.ID)},
		Items: []Item{{ID: 1, IsEquipment: true, DropQuality: 1}, {ID: 2, Stackable: true, MaxStack: 4}, {ID: 3, IsEquipment: true, DropQuality: 2}}}
}
