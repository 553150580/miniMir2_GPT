// 本文件核验发布候选所引用的地图、怪物以及角色资格。
package devconfig

import (
	"encoding/json"
	"testing"
	"time"
)

// TestAcceptsPersonalBossQualification 核对当前版本以及已逐字段核对夹具之间的单向兼容。
func TestAcceptsPersonalBossQualification(t *testing.T) {
	const before = "4bb20bf3e8d235db1061e3223b8b28169f662d3e7e334291f6f65c09400acb1a"
	const after = "6a421806cf5d45a8ead84bfda80be4a57eebcac00d3344140825fc860bfec9a6"
	const market = "8cf30edfdf4db1cffbd2c0fe5688652a96496569f00aa19f26581f83a94fb266"
	const unrelated = "af53a2647dab60517d43e75bba2b3a3c0cfc25e3b7295b5a931c330222adee65"
	for _, tc := range []struct {
		name      string   // 说明当前规则与原证明的关系。
		fixture   *Fixture // 当前加载的主夹具身份。
		proofRule uint64   // 持久证明保存的规则版本。
		proofHash string   // 持久证明保存的完整配置摘要。
		want      bool     // 可继续沿用这条原实战证明。
	}{
		{"双卡配置继承旧证明", &Fixture{RuleVersion: 30, Hash: after}, 30, before, true},
		{"市场配置继承最早证明", &Fixture{RuleVersion: 30, Hash: market}, 30, before, true},
		{"市场配置继承双卡证明", &Fixture{RuleVersion: 30, Hash: market}, 30, after, true},
		{"市场配置接受现有证明", &Fixture{RuleVersion: 30, Hash: market}, 30, market, true},
		{"双卡配置不反向接受市场证明", &Fixture{RuleVersion: 30, Hash: after}, 30, market, false},
		{"市场配置不接受其他摘要", &Fixture{RuleVersion: 30, Hash: market}, 30, unrelated, false},
		{"市场规则版本变化拒绝沿用", &Fixture{RuleVersion: 31, Hash: market}, 31, after, false},
		{"当前配置接受当前证明", &Fixture{RuleVersion: 30, Hash: unrelated}, 30, unrelated, true},
		{"配置摘要不相干", &Fixture{RuleVersion: 30, Hash: unrelated}, 30, before, false},
		{"旧配置不反向接受新证明", &Fixture{RuleVersion: 30, Hash: before}, 30, after, false},
		{"旧证明版本不同", &Fixture{RuleVersion: 30, Hash: after}, 29, before, false},
		{"规则版本变更", &Fixture{RuleVersion: 31, Hash: after}, 31, before, false},
		{"相同摘要也须同规则版本", &Fixture{RuleVersion: 30, Hash: after}, 31, after, false},
		{"空身份不算证据", &Fixture{RuleVersion: 30}, 30, "", false},
		{"无当前配置", nil, 30, before, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.fixture.AcceptsPersonalBossQualification(tc.proofRule, tc.proofHash); got != tc.want {
				t.Fatalf("通关证明是否允许沿用=%v，预期 %v", got, tc.want)
			}
		})
	}
}

// TestPersonalBossReleaseResolvesDedicatedSource 验证独立装配不会向普通刷怪和通用地图导入挑战。
func TestPersonalBossReleaseResolvesDedicatedSource(t *testing.T) {
	data := []byte(`{
		"map":{"id":1}, "monsters":[{"id":1}],
		"challenge_monsters":[{"id":90,"hp":40,"attack_min":1,"attack_max":2,"move_ms":500,"attack_ms":500}],
		"personal_boss":{"enabled":true,"template_id":9,"display_name":"隔离候选",
			"map":{"id":91,"scene_kind":3,"width":13,"height":13,"player_spawn":{"x":2,"y":2},"monster_spawns":[{"x":4,"y":4}]},
			"monster_id":90,"minimum_level":7,"duration_ms":30000,"success_cooldown_ms":60000,"retry_cooldown_ms":1000,
			"daily_success_limit":4,"privilege_success_bonus":2,"history_limit":20}
	}`)
	var fixture Fixture
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	if err := fixture.validatePersonalBossRelease(); err != nil {
		t.Fatal(err)
	}
	cfg := fixture.PersonalBoss
	if cfg.TemplateID != 9 || cfg.Monster.ID != 90 || cfg.Map.ID != 91 || cfg.Duration != 30*time.Second || cfg.MinimumLevel != 7 || cfg.DailySuccessLimit != 4 || cfg.PrivilegeSuccessBonus != 2 {
		t.Fatalf("发布定义未完整解析游戏配置：%+v", cfg)
	}
	if len(fixture.Monsters) != 1 || fixture.Monsters[0].ID != 1 {
		t.Fatal("首领进入了普通刷怪来源")
	}
	if _, exists := fixture.MapByID(cfg.Map.ID); exists {
		t.Fatal("挑战地图出现在常规换图目录")
	}
	fixture.PersonalBossRelease.Map.Blocked = []Point{{X: 4, Y: 3}, {X: 4, Y: 5}, {X: 3, Y: 4}, {X: 5, Y: 4}}
	if err := fixture.validatePersonalBossRelease(); err == nil {
		t.Fatal("首领出生格完全隔离时仍可开放")
	}
	fixture.PersonalBossRelease.Enabled = false
	if err := fixture.validatePersonalBossRelease(); err != nil || fixture.PersonalBoss != nil {
		t.Fatalf("显式关闭没有清除运行时挑战：%v", err)
	}
	fixture.ChallengeMonsters[0].ID = fixture.Monsters[0].ID
	if err := fixture.validatePersonalBossRelease(); err == nil {
		t.Fatal("允许挑战怪物占用普通怪物编号")
	}
}
