// 本文件核验发布候选所引用的地图、怪物以及角色资格。
package devconfig

import (
	"encoding/json"
	"testing"
	"time"
)

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
