// 使用正常账号和 WebSocket 验证小地图的原点地目标与权威行走路径。
package integration

import (
	"slices"
	"testing"
	"time"

	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	p "minimir/internal/protocol"
	"minimir/internal/wire"
)

// TestMapTravelPersonalGroundShortestMongoWS 使用可复现的两处静态阻挡测试绕路。
func TestMapTravelPersonalGroundShortestMongoWS(t *testing.T) {
	cfg := m1FieldConfig(t)
	start := devconfig.Point{X: 6, Y: 9}
	turn := devconfig.Point{X: 6, Y: 11}
	goal := devconfig.Point{X: 6, Y: 5}
	cfg.Fixture.Map.PlayerSpawn = start
	cfg.Fixture.Map.PetSpawn = devconfig.Point{X: 5, Y: 9}
	cfg.Fixture.Map.Blocked = []devconfig.Point{{X: 6, Y: 6}, {X: 7, Y: 5}}
	must(t, cfg.Fixture.Validate())
	cfg, _, _ = m1FieldServices(t, cfg)
	endpoint, token, key := m1FieldRole(t, cfg, 0)
	c := join(t, cfg.GameAddress, issue(t, endpoint, token, key))
	t.Cleanup(c.close)
	c.auto(false, 1)

	// playerPoint 只读取服务端场景基线的权威行走位置。
	playerPoint := func() devconfig.Point {
		position := c.player().GetPosition()
		return devconfig.Point{X: int(position.GetX()), Y: int(position.GetY())}
	}
	// walk 通过真实移动意图发起一次完整导航，逐个收集新落格。
	walk := func(destination devconfig.Point) []devconfig.Point {
		t.Helper()
		wanted := &pb.GridPoint{X: int32(destination.X), Y: int32(destination.Y)}
		input := &pb.MoveIntent{SceneEpoch: c.entry.SceneEpoch, Destination: wanted}
		route := []devconfig.Point{playerPoint()}
		c.send(p.MsgC2SMoveIntent, input)
		travelUntilWithin(c, 9*time.Second, func(frame wire.Frame) bool {
			if frame.ID != p.MsgS2CSceneBaseline || c.scene == nil || c.scene.SceneEpoch != c.entry.SceneEpoch {
				return false
			}
			nav := c.scene.GetNavigationState()
			if nav == nil || nav.GetInputSequence() != input.InputSequence ||
				nav.GetDestination().GetX() != wanted.GetX() || nav.GetDestination().GetY() != wanted.GetY() {
				return false
			}
			if position := playerPoint(); position != route[len(route)-1] {
				route = append(route, position)
			}
			return nav.GetStatus() == 2 && playerPoint() == destination
		})
		return route
	}
	if current := playerPoint(); current != start {
		t.Fatalf("测试角色没有从预定地格出生：%+v", current)
	}
	if actual := walk(turn); !slices.Equal(actual, []devconfig.Point{start, {X: 6, Y: 10}, turn}) {
		t.Fatalf("无遮挡直走发生偏移：%+v", actual)
	}
	if actual := walk(start); !slices.Equal(actual, []devconfig.Point{turn, {X: 6, Y: 10}, start}) {
		t.Fatalf("反向直走发生偏移：%+v", actual)
	}
	detour := walk(goal)
	if len(detour) != 6 {
		t.Fatalf("两处阻挡的权威最短路径应为五步：%+v", detour)
	}
	for i, next := range detour[1:] {
		prior := detour[i]
		dx, dy := next.X-prior.X, next.Y-prior.Y
		if dx < -1 || dx > 1 || dy < -1 || dy > 1 || !cfg.Fixture.Map.Walkable(next) ||
			dx != 0 && dy != 0 && (!cfg.Fixture.Map.Walkable(devconfig.Point{X: prior.X, Y: next.Y}) ||
				!cfg.Fixture.Map.Walkable(devconfig.Point{X: next.X, Y: prior.Y})) {
			t.Fatalf("权威步非法或穿过斜角：%+v→%+v", prior, next)
		}
	}
	t.Logf("两处静态阻挡的五步真实路线：%+v", detour)
}
