// 本文件验证当前紧凑城镇的实际碰撞图和点地路线的转向。
package townnav

import (
	"slices"
	"testing"

	"minimir/internal/devconfig"
)

// TestCompactTownRouteDoesNotOvershootAroundWarehouse 验证六步绕仓库时不先越过目标横坐标再折回。
func TestCompactTownRouteDoesNotOvershootAroundWarehouse(t *testing.T) {
	config, err := devconfig.Load("../../../conf/dev/server.json")
	if err != nil {
		t.Fatal(err)
	}
	goal := Point{X: 6, Y: 8}
	want := []Point{{8, 12}, {7, 11}, {7, 10}, {7, 9}, {7, 8}, goal}
	dynamic := Dynamic{Revision: 1}
	count := 0
	for _, town := range config.Fixture.Maps {
		if !town.CompactTown || !config.Fixture.TownDestination(town.ID) {
			continue
		}
		count++
		grid, err := NewGrid(testKey().MapID, testKey().MapVersion, town.Width, town.Height, town.TownCollisionBits())
		if err != nil {
			t.Fatal(err)
		}
		start := Point{X: town.PlayerSpawn.X, Y: town.PlayerSpawn.Y}
		task, _ := testTask(t, grid, start, goal, testLimits(grid.Cells()), dynamic)
		finish(t, task, 4)
		var steps []Point
		position := start
		for task.State() == Ready {
			step, ok, err := task.Next(testKey(), position, dynamic)
			if err != nil || !ok || !grid.StepAllowed(position, step) {
				t.Fatalf("城镇%d 的第%d步穿过阻挡或斜角: %v %v", town.ID, len(steps)+1, step, err)
			}
			if err := task.CommitStep(testKey(), position, step); err != nil {
				t.Fatal(err)
			}
			steps = append(steps, step)
			position = step
		}
		if task.State() != Arrived || !slices.Equal(steps, want) {
			t.Errorf("城镇%d 从%v点%v：实际路线%v，预期贴近直线的六步%v", town.ID, start, goal, steps, want)
		}
	}
	if count != 3 {
		t.Fatalf("发布城镇覆盖不全：%d", count)
	}
}
