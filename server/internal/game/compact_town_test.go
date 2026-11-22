// 本文件回归紧凑城镇换图、NPC导航与旧安全区绑定兼容。
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

// TestCompactTownTravelRequiresAllowedTargetsAndKeepsOriginalPersonalWorld 核验选择地图应正确切换并等待首视口ACK，以及返回时应恢复原角色脚点、怪物、掉落截止和挂机偏好。
// t 提供断言失败报告、子用例和清理注册。
func TestCompactTownTravelRequiresAllowedTargetsAndKeepsOriginalPersonalWorld(t *testing.T) {
	// w 接收地图地图切换房间执行器供后续步骤使用，r 接收地图地图切换房间执行器供后续步骤使用，c 接收地图地图切换房间执行器供后续步骤使用，now 保存由地图地图切换房间执行器取得的当前时间。
	w, r, c, now := mapTravelWorker(t)
	// f 保存当前处理所用的开发测试配置。
	f := &w.e.Store.Cfg.Fixture
	// personal 保存由当前状态场景世界取得的个人，position 保存当前处理所用的位置，monster 保存从怪物集合取出的怪物，pet 保存当前处理所用的宠物。
	personal, position, monster, pet := r.currentWorld(), r.Player.Pos, r.Monsters[0], r.Pet
	r.AutoTarget = reference(r, monster)
	r.Loot = []*entity{{Handle: 3000, Kind: 4, Pos: position}}
	r.LootUntil = now.Add(time.Minute)
	// loot 保存从战利品取出的战利品，cutoff 保存战利品有效截止时间，用于当前步骤中的“cutoff”。
	loot, cutoff := r.Loot[0], r.LootUntil
	// step 规定步骤的初始取值。
	step := 0
	// prepare 封装准备对应的后续处理，destination 记录目的地，使用数值表示，target 记录目标，使用数值表示。
	prepare := func(destination uint32, target uint64) wire.Frame {
		step++
		return wire.Message(p.MsgC2SMapTravelPrepare, uint32(step), uint64(r.Auth.SessionEpoch), &pb.MapTravelPrepare{
			SceneEpoch: r.Scene, TravelId: travelID(step), Destination: destination, TargetMapId: target,
		})
	}
	// reject 封装“reject”对应的后续处理，frame 记录协议帧，code 记录结果码，使用数值表示。
	reject := func(frame wire.Frame, code uint32) {
		t.Helper()
		// world 保存由当前状态场景世界取得的场景世界，scene 保存当前处理所用的场景，baseline 保存当前处理所用的场景基线，player 保存位置，用于当前步骤中的玩家。
		world, scene, baseline, player := r.currentWorld(), r.Scene, r.Baseline, r.Player.Pos
		w.prepareMapTravelAt(r, frame, now)
		// got 保存从协议帧集合取出的实际结果。
		if got := c.frames[len(c.frames)-1]; got.Request != frame.Request || got.Error != code ||
			r.currentWorld() != world || r.Scene != scene || r.Baseline != baseline || r.Player.Pos != player || !r.Ready {
			t.Fatalf("非法或同图目标改变现场或未正常拒绝：%v", got)
		}
	}
	// commit 封装提交对应的后续处理，mapID 标识当前业务使用的地图，position 记录位置，使用点位表示。
	commit := func(mapID uint64, position devconfig.Point) {
		t.Helper()
		// from 保存场景，用于当前步骤中的来源。
		from := r.Scene
		// frame 保存从协议帧集合取出的协议帧。
		frame := c.frames[len(c.frames)-1]
		// plan 记录计划，使用地图地图切换计划表示。
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

	// id 接收遍历数值列表时当前的元素。
	for _, id := range []uint64{3, 4, 5, 3} {
		// town 保存由地图“by”标识取得的城镇，exists 保存由地图“by”标识取得的存在。
		town, exists := f.MapByID(id)
		if !exists {
			t.Fatalf("当前目录缺少紧凑城镇%d", id)
		}
		// frame 保存由准备取得的协议帧。
		frame := prepare(travelToTown, id)
		w.prepareMapTravelAt(r, frame, now)
		if id == 3 {
			// conflict 保存由消息取得的冲突。
			conflict := wire.Message(p.MsgC2SMapTravelPrepare, frame.Request, frame.Binding,
				&pb.MapTravelPrepare{SceneEpoch: r.Scene, TravelId: travelID(step), Destination: travelToTown, TargetMapId: 4})
			reject(conflict, p.CodeOperationConflict)
			w.prepareMapTravelAt(r, frame, now)
		}
		commit(id, town.PlayerSpawn)
		if r.SavedPersonal != personal || r.Auto || r.AutoTarget != (actorRef{}) || len(r.Loot) != 0 || r.MonsterCount != 0 || len(r.Monsters) != 0 {
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
		r.Loot[0] != loot || !r.LootUntil.Equal(cutoff) || !r.Auto || r.AutoTarget != (actorRef{}) {
		t.Fatal("返回时没有恢复原角色脚点、怪物、掉落截止和挂机偏好")
	}
}

// TestCompactTownNavigationUsesNPCCellsAndSkipsHistoricalMap 核对「寻路给历史绑定地图分配了工作区」与「地图的NPC占格仍允许行走或传送」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestCompactTownNavigationUsesNPCCellsAndSkipsHistoricalMap(t *testing.T) {
	// w 接收地图地图切换房间执行器供后续步骤使用。
	w, _, _, _ := mapTravelWorker(t)
	w.e.workers = []*worker{w}
	// err 接收“initialize”城镇导航过程中可能产生的错误，供后续失败分支处理。
	if err := w.e.initializeTownNavigation(); err != nil {
		t.Fatal(err)
	}
	if len(w.e.TownGrids) != 3 || w.e.TownGrids[2] != nil {
		t.Fatal("寻路给历史绑定地图分配了工作区")
	}
	// m 接收遍历地图集合时当前的元素。
	for _, m := range w.e.Store.Cfg.Fixture.Maps {
		if !m.CompactTown {
			continue
		}
		// grid 保存从城镇网格集合取出的网格。
		grid := w.e.TownGrids[m.ID]
		if grid == nil || grid.Cells() != 289 {
			t.Fatalf("地图%d缺少17×17地面寻路格", m.ID)
		}
		// npc 接收遍历“np”“cs”时当前的元素。
		for _, npc := range m.NPCs {
			if m.Walkable(npc.Position) || m.TeleportWalkable(npc.Position) || grid.Walkable(townPoint(npc.Position)) {
				t.Fatalf("地图%d的NPC%d占格仍允许行走或传送", m.ID, npc.ID)
			}
		}
	}
}

// TestCompactTownTeleportUsesTrustedLegacyZoneAndKeepsBindingProofUntilCommit 核验实际走到新城镇安全区应生成新地图的合法绑定候选，以及历史可信安全区应受理21。
// t 提供断言失败报告、子用例和清理注册。
func TestCompactTownTeleportUsesTrustedLegacyZoneAndKeepsBindingProofUntilCommit(t *testing.T) {
	// w 接收绑定瞬移“21”房间执行器供后续步骤使用，r 接收绑定瞬移“21”房间执行器供后续步骤使用，c 接收绑定瞬移“21”房间执行器供后续步骤使用。
	// now 保存由绑定瞬移“21”房间执行器取得的当前时间。
	w, r, c, _, now := boundTeleport21Worker(t)
	// f 保存当前处理所用的开发测试配置。
	f := &w.e.Store.Cfg.Fixture
	// personal 保存由当前状态场景世界取得的个人，oldBinding 保存当前处理所用的原状态会话绑定。
	// oldSource 保存会话绑定来源序号，用于当前步骤中的原状态来源。
	personal, oldBinding, oldSource := r.currentWorld(), *r.Role.Binding, r.Role.BindingSourceSeq
	// n 记录数值。
	w.roll = func(n int64) int64 { return n / 2 }
	// code 保存由施法取得的结果码。
	if code := w.cast(r, 21, 0, 1, now); code != 0 {
		t.Fatal("历史可信安全区没有受理21", code)
	}
	w.advanceEvents(r, now.Add(200*time.Millisecond))
	// result 保存由瞬移“21”上一次状态取得的结果。
	result := teleport21Last(t, c)
	// town 保存由地图“by”标识取得的城镇。
	town, _ := f.MapByID(3)
	// center 保存当前处理所用的中心格。
	center := town.SafeZones[0].Center
	if result.Outcome != teleportSucceeded || result.TargetMapId != 3 || result.TransitionId == "" || r.MapID != 3 ||
		r.Player.Pos != center || r.SavedPersonal != personal || r.townWorld(2) != nil {
		t.Fatal("可信历史坐标未进入对应紧凑城镇安全区", result)
	}
	if *r.Role.Binding != oldBinding || r.Role.BindingSourceSeq != oldSource || r.Role.BindingSafeZoneID != 106 {
		t.Fatal("21落点事务提交前改写了历史证明")
	}
	// fact 保存会话绑定候选，用于当前步骤中的已发生的事实。
	fact := r.BindingCandidate
	if fact == nil || fact.zoneID != town.SafeZones[0].ID ||
		fact.center != (domain.RoleBinding{MapID: 3, X: int32(center.X), Y: int32(center.Y)}) {
		t.Fatal("实际走到新城镇安全区没有生成新地图的合法绑定候选")
	}
}
