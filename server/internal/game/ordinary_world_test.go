// 本文件回归普通地图直接切换、原槽复活截止继承及旧现场释放。
package game

import (
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	p "minimir/internal/protocol"
	"minimir/internal/wire"
	"slices"
	"testing"
	"time"
)

// ordinaryTravelFixture 添加一个槽位对应不同战斗模板和外观编号的普通图。
func ordinaryTravelFixture(t *testing.T) (*worker, *room, uint64, time.Time) {
	t.Helper()
	w, r, _, now := mapTravelWorker(t)
	f := &w.e.Store.Cfg.Fixture
	f.RuleVersion = devconfig.OrdinaryMonsterRosterRule
	mainDefinition := f.Monsters[0]
	otherDefinition := mainDefinition
	otherDefinition.ID, otherDefinition.DefinitionID, otherDefinition.HP = 50602, 50603, mainDefinition.HP+19
	f.Monsters = append(f.Monsters, otherDefinition)
	f.Map.MonsterSpawnIDs = make([]uint32, devconfig.OrdinaryMonsterSlots)
	for i := range f.Map.MonsterSpawnIDs {
		f.Map.MonsterSpawnIDs[i] = mainDefinition.ID
	}
	f.Map.InitialMonsterCount = 1
	target := f.Map
	target.ID, target.SceneKind = 50601, devconfig.SceneKindPersonal
	target.MonsterSpawnIDs = slices.Clone(f.Map.MonsterSpawnIDs)
	target.MonsterSpawnIDs[0] = otherDefinition.ID
	f.Maps = append(f.Maps, target)
	r.PersonalWorld = r.currentWorld()
	r.PetWorld = r.currentWorld().Ref
	if !populateOrdinaryMonsters(f, r.currentWorld(), &f.Map, nil, now) {
		t.Fatal("主普通地图初始化失败")
	}
	return w, r, target.ID, now
}

// prepareOrdinaryTravel 发出含明确目录编号的普通或城镇换图请求。
func prepareOrdinaryTravel(w *worker, r *room, sequence int, destination uint32, mapID uint64, now time.Time) {
	request := &pb.MapTravelPrepare{SceneEpoch: r.Scene, TravelId: travelID(sequence), Destination: destination, TargetMapId: mapID}
	w.prepareMapTravelAt(r, wire.Message(p.MsgC2SMapTravelPrepare, uint32(sequence), uint64(r.Auth.SessionEpoch), request), now)
}

// acceptOrdinaryTravel 确认测试地图收到完整入场基线。
func acceptOrdinaryTravel(t *testing.T, w *worker, r *room, now time.Time) {
	t.Helper()
	if !w.finishMapTravelAck(r, now) {
		t.Fatal("旅行场景确认失败")
	}
	r.Ready = true
}

// TestOrdinaryWorldTravelKeepsSelectedMonsterCount 验证直达和经过城镇均沿用原普通图选择。
func TestOrdinaryWorldTravelKeepsSelectedMonsterCount(t *testing.T) {
	w, r, otherMapID, now := ordinaryTravelFixture(t)
	f := &w.e.Store.Cfg.Fixture
	if !w.setVisibleMonsterCount(r, &f.Map, 8) {
		t.Fatal("原普通图无法切换到八个活跃槽")
	}
	prepareOrdinaryTravel(w, r, 1, travelToPersonal, otherMapID, now)
	confirmTravelAt(w, r, 1, r.Scene, false, now)
	acceptOrdinaryTravel(t, w, r, now)
	if r.MapID != otherMapID || r.MonsterCount != 8 {
		t.Fatal("直达新普通图丢失已选择的八个槽")
	}
	other, ok := f.MapByID(otherMapID)
	if !ok || !w.setVisibleMonsterCount(r, other, 4) {
		t.Fatal("新普通图无法切换到四个活跃槽")
	}
	town := f.TownTravel.DestinationMapIDs[0]
	now = now.Add(time.Second)
	prepareOrdinaryTravel(w, r, 2, travelToTown, town, now)
	confirmTravelAt(w, r, 2, r.Scene, false, now)
	acceptOrdinaryTravel(t, w, r, now)
	if r.MapID != town || r.MonsterCount != 0 || r.SavedPersonal.MonsterCount != 4 {
		t.Fatal("进入城镇没有独立保留原普通图的四个槽")
	}
	now = now.Add(time.Second)
	prepareOrdinaryTravel(w, r, 3, travelToPersonal, f.Map.ID, now)
	confirmTravelAt(w, r, 3, r.Scene, false, now)
	acceptOrdinaryTravel(t, w, r, now)
	if r.MapID != f.Map.ID || r.MonsterCount != 4 {
		t.Fatal("从城镇去另一张普通图丢失已选择的四个槽")
	}
}

// TestOrdinaryWorldTravelKeepsSlotCooldownAndReleasesOldScene 验证多模板、旧收益屏障和单现场上限。
func TestOrdinaryWorldTravelKeepsSlotCooldownAndReleasesOldScene(t *testing.T) {
	w, r, targetID, now := ordinaryTravelFixture(t)
	old := r.currentWorld()
	old.Monsters[0].HP, old.Monsters[0].RespawnAt = 0, now.Add(10*time.Second)
	old.Loot = []*entity{{Handle: 3000, Kind: 4}}
	r.KillFacts = []killFact{{World: old.Ref}}
	prepareOrdinaryTravel(w, r, 1, travelToPersonal, targetID, now)
	confirmTravelAt(w, r, 1, r.Scene, false, now)
	if r.Travel == nil || r.Travel.Phase != travelQuiescing || r.currentWorld() != old {
		t.Fatal("未结算原收益时提前离开")
	}
	r.KillFacts = nil
	if !w.progressMapTravel(r, now.Add(time.Millisecond)) {
		t.Fatal("完成原收益后未提交")
	}
	world := r.currentWorld()
	monster := world.Monsters[0]
	if r.MapID != targetID || world != r.PersonalWorld || r.world(old.Ref) != nil || len(r.worlds()) != 1 || len(world.Loot) != 0 ||
		world.MonsterCount != 1 || monster.Monster.ID != 50602 || monster.Def != 50603 || monster.HP != 0 ||
		!monster.DeferredUntilRespawn || !monster.RespawnAt.Equal(old.Monsters[0].RespawnAt) {
		t.Fatal("新地图未取得独立怪物或跳过旧复活截止", r.MapID, world.MonsterCount, monster)
	}
	baseline, err := w.buildBaseline(r, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, chunk := range baseline {
		for _, entity := range chunk.Entities {
			if entity.EntityKind == 3 {
				t.Fatal("等待原截止的槽位在新图显示虚假尸体")
			}
		}
	}
	acceptOrdinaryTravel(t, w, r, now.Add(time.Millisecond))
	townID := w.e.Store.Cfg.Fixture.TownTravel.DestinationMapIDs[0]
	prepareOrdinaryTravel(w, r, 2, travelToTown, townID, now)
	confirmTravelAt(w, r, 2, r.Scene, false, now.Add(time.Millisecond))
	acceptOrdinaryTravel(t, w, r, now.Add(2*time.Millisecond))
	if r.SavedPersonal != world {
		t.Fatal("进城丢失原普通现场")
	}
	prepareOrdinaryTravel(w, r, 3, travelToPersonal, 0, now.Add(time.Second))
	confirmTravelAt(w, r, 3, r.Scene, false, now.Add(time.Second))
	acceptOrdinaryTravel(t, w, r, now.Add(time.Second))
	if r.currentWorld() != world || r.Monsters[0] != monster || monster.HP != 0 {
		t.Fatal("返回未复用普通图、怪物实体和计时")
	}
	w.roll = func(n int64) int64 { return n - 1 }
	before := monster.Pos
	w.worldDeadlines(r, world, now.Add(10*time.Second-time.Nanosecond))
	if monster.HP != 0 {
		t.Fatal("入场后提前恢复怪物")
	}
	w.worldDeadlines(r, world, now.Add(10*time.Second))
	if monster.Pos == before || monster.HP != monster.MaxHP || monster.DeferredUntilRespawn || monster.Def != 50603 || monster.Life != 1 {
		t.Fatal("新普通图的异模板怪物未按原计时随机空格复活", monster)
	}
	baseline, err = w.buildBaseline(r, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, chunk := range baseline {
		for _, entity := range chunk.Entities {
			if entity.EntityHandle == monster.Handle && entity.DefinitionId == 50603 && entity.Hp == monster.HP {
				return
			}
		}
	}
	t.Fatal("基线没有同步复活怪物的外观定义编号")
}

// TestOrdinaryWorldRepeatedHopsKeepOneSceneAndParkPet 验证反复换图后只保留当前现场及禁宠驻留引用。
func TestOrdinaryWorldRepeatedHopsKeepOneSceneAndParkPet(t *testing.T) {
	w, r, targetID, now := ordinaryTravelFixture(t)
	target, _ := w.e.Store.Cfg.Fixture.MapByID(targetID)
	noPets := true
	target.NoPets = &noPets
	pet := r.Pet
	original := r.currentWorld().Ref
	for i := 1; i <= 8; i++ {
		id := targetID
		if i%2 == 0 {
			id = w.e.Store.Cfg.Fixture.Map.ID
		}
		at := now.Add(time.Duration(i) * time.Second)
		prepareOrdinaryTravel(w, r, i, travelToPersonal, id, at)
		confirmTravelAt(w, r, i, r.Scene, false, at)
		acceptOrdinaryTravel(t, w, r, at)
		if r.MapID != id || r.PersonalWorld != r.currentWorld() || r.TravelPersonalWorld != nil || r.Pet != pet {
			t.Fatal("反复换图丢失角色现场或宠物身份", i)
		}
		if i%2 != 0 {
			held := r.PetHoldingWorld
			if held == nil || r.petResidentWorld() != held || len(held.Monsters) != 0 || len(held.Loot) != 0 || len(r.worlds()) != 2 {
				t.Fatal("禁宠仅可保留宠物驻留引用", i)
			}
		} else if r.PetHoldingWorld != nil || r.petResidentWorld() != r.currentWorld() || len(r.worlds()) != 1 || r.world(original) != nil {
			t.Fatal("正常回迁后保留了废弃场景", i)
		}
	}
}
