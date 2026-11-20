package game

import (
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	p "minimir/internal/protocol"
	"minimir/internal/wire"
)

func TestCompactTownTravelRequiresAllowedTargetsAndKeepsOriginalPersonalWorld(t *testing.T) {
	w, r, c, now := mapTravelWorker(t)
	f := &w.e.Store.Cfg.Fixture
	personal, position, monster, pet := r.currentWorld(), r.Player.Pos, r.Monsters[0], r.Pet
	r.Loot = []*entity{{Handle: 3000, Kind: 4, Pos: position}}
	r.LootUntil = now.Add(time.Minute)
	loot, cutoff := r.Loot[0], r.LootUntil
	step := 0
	prepare := func(destination uint32, target uint64) wire.Frame {
		step++
		return wire.Message(p.MsgC2SMapTravelPrepare, uint32(step), uint64(r.Auth.SessionEpoch), &pb.MapTravelPrepare{
			SceneEpoch: r.Scene, TravelId: travelID(step), Destination: destination, TargetMapId: target,
		})
	}
	reject := func(frame wire.Frame, code uint32) {
		t.Helper()
		world, scene, baseline, player := r.currentWorld(), r.Scene, r.Baseline, r.Player.Pos
		w.prepareMapTravelAt(r, frame, now)
		if got := c.frames[len(c.frames)-1]; got.Request != frame.Request || got.Error != code ||
			r.currentWorld() != world || r.Scene != scene || r.Baseline != baseline || r.Player.Pos != player || !r.Ready {
			t.Fatalf("非法或同图目标改变现场或未正常拒绝：%v", got)
		}
	}
	commit := func(mapID uint64, position devconfig.Point) {
		t.Helper()
		from := r.Scene
		frame := c.frames[len(c.frames)-1]
		var plan pb.MapTravelPlan
		if frame.ID != p.MsgS2CMapTravelPlan || proto.Unmarshal(frame.Payload, &plan) != nil ||
			plan.FromSceneEpoch != from || plan.TargetMapId != mapID || !proto.Equal(plan.Position, pointPB(position)) {
			t.Fatalf("旅行Plan未选中唯一服务端目的地：%v", &plan)
		}
		confirmTravelAt(w, r, step, from, false, now)
		if r.MapID != mapID || r.Scene != from+1 || r.Ready || r.Player.Pos != position || r.Pet != pet {
			t.Fatalf("选择地图%d没有正确切换并等待首视口ACK", mapID)
		}
		w.command(command{role: r.Role.ID, conn: c, frame: wire.Message(p.MsgC2SEnterWorldAck, 900, uint64(r.Auth.SessionEpoch),
			&pb.EnterWorldAck{SceneEpoch: r.Scene, BaselineId: r.Baseline})})
		if !r.Ready || c.frames[len(c.frames)-1].ID != p.MsgS2CSyncReady {
			t.Fatal("正确地图首视口ACK未激活当前地图")
		}
	}
	reject(prepare(travelToTown, 0), p.CodeLineForbidden)
	reject(prepare(travelToTown, 2), p.CodeLineForbidden)
	reject(prepare(travelToTown, 99), p.CodeLineForbidden)
	reject(prepare(travelToPersonal, 1), p.CodeLineForbidden)

	for _, id := range []uint64{3, 4, 5, 3} {
		town, exists := f.MapByID(id)
		if !exists {
			t.Fatalf("当前目录缺少紧凑城镇%d", id)
		}
		frame := prepare(travelToTown, id)
		w.prepareMapTravelAt(r, frame, now)
		if id == 3 {
			conflict := wire.Message(p.MsgC2SMapTravelPrepare, frame.Request, frame.Binding,
				&pb.MapTravelPrepare{SceneEpoch: r.Scene, TravelId: travelID(step), Destination: travelToTown, TargetMapId: 4})
			reject(conflict, p.CodeOperationConflict)
			w.prepareMapTravelAt(r, frame, now)
		}
		commit(id, town.PlayerSpawn)
		if r.SavedPersonal != personal || r.Auto || len(r.Loot) != 0 || r.MonsterCount != 0 || len(r.Monsters) != 0 {
			t.Fatal("城镇之间旅行替换了休眠个人现场，或开启城镇挂机")
		}
		reject(prepare(travelToTown, id), p.CodeLineForbidden)
	}
	if len(r.TownWorlds) != 3 || r.currentWorld() != r.TownWorlds[0] || r.townWorld(2) != nil || len(r.worlds()) != 4 {
		t.Fatal("再次访问重建了原城镇World，或分配了历史绑定地图")
	}
	reject(prepare(travelToPersonal, 4), p.CodeLineForbidden)
	w.prepareMapTravelAt(r, prepare(travelToPersonal, 1), now)
	commit(1, position)
	if r.currentWorld() != personal || r.SavedPersonal != nil || r.Monsters[0] != monster ||
		r.Loot[0] != loot || !r.LootUntil.Equal(cutoff) || !r.Auto {
		t.Fatal("返回时没有恢复原角色脚点、怪物、掉落截止和挂机偏好")
	}
}

func TestCompactTownNavigationUsesNPCCellsAndSkipsHistoricalMap(t *testing.T) {
	w, _, _, _ := mapTravelWorker(t)
	w.e.workers = []*worker{w}
	if err := w.e.initializeTownNavigation(); err != nil {
		t.Fatal(err)
	}
	if len(w.e.TownGrids) != 3 || w.e.TownGrids[2] != nil {
		t.Fatal("寻路给历史绑定地图分配了工作区")
	}
	for _, m := range w.e.Store.Cfg.Fixture.Maps {
		if !m.CompactTown {
			continue
		}
		grid := w.e.TownGrids[m.ID]
		if grid == nil || grid.Cells() != 289 {
			t.Fatalf("地图%d缺少17×17地面寻路格", m.ID)
		}
		for _, npc := range m.NPCs {
			if m.Walkable(npc.Position) || m.TeleportWalkable(npc.Position) || grid.Walkable(townPoint(npc.Position)) {
				t.Fatalf("地图%d的NPC%d占格仍允许行走或传送", m.ID, npc.ID)
			}
		}
	}
}

func TestCompactTownTeleportUsesTrustedLegacyZoneAndKeepsBindingProofUntilCommit(t *testing.T) {
	w, r, c, _, now := boundTeleport21Worker(t)
	f := &w.e.Store.Cfg.Fixture
	personal, oldBinding, oldSource := r.currentWorld(), *r.Role.Binding, r.Role.BindingSourceSeq
	w.roll = func(n int64) int64 { return n / 2 }
	if code := w.cast(r, 21, 0, 1, now); code != 0 {
		t.Fatal("历史可信安全区没有受理21", code)
	}
	w.advanceEvents(r, now.Add(200*time.Millisecond))
	result := teleport21Last(t, c)
	town, _ := f.MapByID(3)
	center := town.SafeZones[0].Center
	if result.Outcome != teleportSucceeded || result.TargetMapId != 3 || result.TransitionId == "" || r.MapID != 3 ||
		r.Player.Pos != center || r.SavedPersonal != personal || r.townWorld(2) != nil {
		t.Fatal("可信历史坐标未进入对应紧凑城镇安全区", result)
	}
	if *r.Role.Binding != oldBinding || r.Role.BindingSourceSeq != oldSource || r.Role.BindingSafeZoneID != 106 {
		t.Fatal("21落点事务提交前改写了历史证明")
	}
	fact := r.BindingCandidate
	if fact == nil || fact.zoneID != town.SafeZones[0].ID ||
		fact.center != (domain.RoleBinding{MapID: 3, X: int32(center.X), Y: int32(center.Y)}) {
		t.Fatal("实际走到新城镇安全区没有生成新地图的合法绑定候选")
	}
}
