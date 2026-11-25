// 本文件验证隔离镜像竞技只能使用明确装配的规则和地图。
package devconfig

import (
	"testing"
	"time"
)

func mirrorArenaFixture() Fixture {
	return Fixture{
		Map: MapTemplate{ID: 1, Width: 13, Height: 13},
		MirrorArenaRelease: &MirrorArenaDefinition{
			Enabled: true, TemplateID: 1, DisplayName: "隔离镜像竞技",
			Map: MapTemplate{ID: 90_001, DisplayName: "隔离镜像竞技地图", ResourceKey: "development/maps/personal-map",
				Width: 13, Height: 13, SceneKind: SceneKindMirrorArena, Blocked: []Point{{5, 5}}},
			AttackerSpawn: Point{3, 5}, DefenderSpawn: Point{8, 5}, AttackerPetSpawn: Point{3, 6}, DefenderPetSpawn: Point{8, 6},
			SeasonAnchor: time.Date(2026, 12, 23, 16, 0, 0, 0, time.UTC), SeasonDays: 15,
			DailyLimit: 5, OpponentLimit: 3, AttackHistoryLimit: 20, DefenseHistoryLimit: 20, DurationMS: 60_000,
			WinBasePoints: 5, LossPoints: 1, StreakPoints: []MirrorArenaStreakPoints{{1, 5}, {3, 6}, {5, 8}},
			TimeoutPolicy: MirrorArenaTimeoutEffectiveHPRatio, MatchPolicy: MirrorArenaMatchCrossJobLevelWindow,
			MaxLevelGap: 3, PetDeathPolicy: MirrorArenaPetFollowsOwnerDeath,
		},
	}
}

func TestMirrorArenaFixtureDefaultsClosed(t *testing.T) {
	cfg, err := Load("../../../conf/dev/server.json")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Fixture.MirrorArenaRelease != nil || cfg.Fixture.MirrorArena != nil {
		t.Fatal("规则 30 默认试玩不得开放未审定的镜像竞技")
	}
}

func TestMirrorArenaExplicitFixtureAndLimits(t *testing.T) {
	f := mirrorArenaFixture()
	if err := f.ValidateMirrorArena(); err != nil {
		t.Fatal(err)
	}
	a := f.MirrorArena
	if a.SeasonAnchor.Format(time.RFC3339) != "2026-12-24T00:00:00+08:00" || a.Duration != time.Minute ||
		!a.Map.Walkable(a.AttackerSpawn) || a.Map.Walkable(Point{5, 5}) || a.Map.TeleportWalkable(a.AttackerSpawn) ||
		a.Map.PlayerSpawn != a.AttackerSpawn || a.Map.PetSpawn != a.AttackerPetSpawn ||
		a.Map.NoTeleport == nil || !*a.Map.NoTeleport || a.Map.NoPets == nil || *a.Map.NoPets {
		t.Fatalf("规则未绑定明确上海起点、碰撞和原落点：%+v", a)
	}
	if _, found := f.MapByID(a.Map.ID); found {
		t.Fatal("镜像场景不能混入普通地图目录")
	}
	if _, _, ok := a.SeasonAt(a.SeasonAnchor.Add(-time.Nanosecond)); ok {
		t.Fatal("赛季开放前返回了可参加场次")
	}
	for _, sample := range []struct {
		elapsed time.Duration // 服务端事件距第一日零点的间隔。
		start   string        // 原事件所属赛季起点。
		end     string        // 不包含的下一赛季起点。
	}{
		{0, "2026-12-24", "2027-01-08"},
		{15*24*time.Hour - time.Nanosecond, "2026-12-24", "2027-01-08"},
		{15 * 24 * time.Hour, "2027-01-08", "2027-01-23"},
	} {
		start, end, ok := a.SeasonAt(a.SeasonAnchor.Add(sample.elapsed).UTC())
		if !ok || start.Format(time.DateOnly) != sample.start || end.Format(time.DateOnly) != sample.end {
			t.Fatalf("跨日赛季归属不准确：%+v %s %s", sample, start, end)
		}
	}
	for streak, want := range []int64{5, 5, 5, 6, 6, 8, 8} {
		if got := a.WinPoints(uint32(streak)); got != want {
			t.Fatalf("连胜 %d 场得分 %d，应为 %d", streak, got, want)
		}
	}
	if !a.AllowsMatch(0, 20, 2, 23) || a.AllowsMatch(2, 23, 0, 19) || a.AllowsMatch(3, 20, 2, 20) || a.AllowsMatch(0, 0, 2, 20) {
		t.Fatal("跨职业候选未按真实等级差或正确职业筛选")
	}
	a.MatchPolicy = MirrorArenaMatchSameJobLevelWindow
	if a.AllowsMatch(0, 20, 2, 20) || !a.AllowsMatch(0, 20, 0, 17) {
		t.Fatal("同职业候选未按配置排除其他职业")
	}
	f.MirrorArenaRelease.Map.Blocked[0] = Point{0, 0}
	f.MirrorArenaRelease.StreakPoints[0].Points = 9
	if a.Map.Blocked[0] != (Point{5, 5}) || a.WinPoints(1) != 5 {
		t.Fatal("装配后的原地图和积分档仍共享输入切片")
	}
}

func TestMirrorArenaRejectsUnverifiedOrOverlappingConfiguration(t *testing.T) {
	for _, sample := range []struct {
		name   string         // 应当被拒绝的候选修改。
		mutate func(*Fixture) // 仅修改与本用例对应的原字段。
	}{
		{"没有赛季日期", func(f *Fixture) { f.MirrorArenaRelease.SeasonAnchor = time.Time{} }},
		{"日期未对齐上海零点", func(f *Fixture) { f.MirrorArenaRelease.SeasonAnchor = time.Date(2026, 12, 24, 0, 0, 0, 0, time.UTC) }},
		{"未选匹配策略", func(f *Fixture) { f.MirrorArenaRelease.MatchPolicy = "" }},
		{"未支持的超时算法", func(f *Fixture) { f.MirrorArenaRelease.TimeoutPolicy = "总战力直接判胜" }},
		{"未选宠物策略", func(f *Fixture) { f.MirrorArenaRelease.PetDeathPolicy = "" }},
		{"后段连胜降分", func(f *Fixture) { f.MirrorArenaRelease.StreakPoints[2].Points = 5 }},
		{"目录编号重叠", func(f *Fixture) { f.MirrorArenaRelease.Map.ID = f.Map.ID }},
		{"讨伐地图编号重叠", func(f *Fixture) { f.Dungeons = []DungeonConfig{{Map: f.MirrorArenaRelease.Map}} }},
		{"原地板资源路径越界", func(f *Fixture) { f.MirrorArenaRelease.Map.ResourceKey = "development/maps/../../旧资源" }},
		{"选择旧地图类型", func(f *Fixture) { f.MirrorArenaRelease.Map.SceneKind = SceneKindPersonalBoss }},
		{"落点不可通行", func(f *Fixture) { f.MirrorArenaRelease.DefenderSpawn = Point{5, 5} }},
		{"双方共用落点", func(f *Fixture) { f.MirrorArenaRelease.DefenderPetSpawn = f.MirrorArenaRelease.AttackerSpawn }},
		{"场地被墙隔开", func(f *Fixture) {
			f.MirrorArenaRelease.Map.Blocked = nil
			for y := range 13 {
				f.MirrorArenaRelease.Map.Blocked = append(f.MirrorArenaRelease.Map.Blocked, Point{5, y})
			}
		}},
	} {
		t.Run(sample.name, func(t *testing.T) {
			f := mirrorArenaFixture()
			if err := f.ValidateMirrorArena(); err != nil {
				t.Fatal(err)
			}
			sample.mutate(&f)
			if err := f.ValidateMirrorArena(); err == nil || f.MirrorArena != nil {
				t.Fatalf("不合法的候选被接受，原快照也应退役：%v", err)
			}
		})
	}
}
